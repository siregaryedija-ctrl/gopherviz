package sim

import (
	"fmt"
	"go/ast"

	"gopherviz/internal/trace"
)

// Penjadwal goroutine yang meniru penjadwal Go dengan satu prosesor (GOMAXPROCS=1):
//   - goroutine baru dan goroutine yang dibangunkan masuk ke slot runnext (penghuni lama
//     pindah ke ekor antrean runq), jadi yang terakhir dibuat/dibangunkan jalan duluan;
//   - goroutine yang sedang jalan tidak pernah disela: giliran berpindah hanya saat ia
//     menunggu (channel, WaitGroup.Wait, Mutex.Lock, select), tidur, Gosched, atau selesai;
//   - waktu bersifat virtual: hanya maju saat semua goroutine menunggu dan ada timer.
//
// Setiap goroutine program berjalan di goroutine Go-nya sendiri, tetapi hanya satu yang
// memegang "tongkat" (channel wake) pada satu waktu, jadi hasilnya deterministik.
// Di Go asli dengan banyak prosesor, urutan antar goroutine bisa berbeda.

type gor struct {
	id       int
	name     string
	wake     chan struct{}
	done     bool
	parked   bool   // sedang menunggu sesuatu (bukan di antrean)
	sleeping bool   // menunggu timer time.Sleep
	why      string // alasan menunggu, untuk tampilan
	ctx      gctx
}

// gctx adalah state interpreter yang milik satu goroutine.
type gctx struct {
	calls    []*callFrame
	depth    int
	quiet    int
	frames   []*frame
	recDepth int
	active   int
}

type timer struct {
	at   int64
	seq  int
	fire func()
}

type sched struct {
	gs      []*gor
	cur     *gor
	runq    []*gor
	runnext *gor
	timers  []*timer
	tseq    int
	now     int64         // waktu virtual (ns)
	done    chan any      // hasil akhir program: nil = main selesai normal
	exited  chan struct{} // goroutine yang dihentikan sudah selesai membongkar stack
	killed  bool
	used    bool // pernah ada goroutine selain main
	chans   []*chanVal
	waitq   map[*value][]*gor // goroutine yang menunggu WaitGroup/Mutex, per sel status
}

type killSignal struct{}

// fatal adalah error fatal runtime Go (deadlock, unlock mutex yang tidak terkunci):
// tidak bisa ditangkap recover().
type fatal struct{ msg string }

func (s *sched) newG(name string) *gor {
	g := &gor{id: len(s.gs) + 1, name: name, wake: make(chan struct{})}
	s.gs = append(s.gs, g)
	return g
}

// ready memasukkan g ke runnext; penghuni runnext sebelumnya pindah ke ekor runq.
func (s *sched) ready(g *gor) {
	g.parked, g.sleeping, g.why = false, false, ""
	if s.runnext != nil {
		s.runq = append(s.runq, s.runnext)
	}
	s.runnext = g
}

func (s *sched) addTimer(at int64, fire func()) {
	s.tseq++
	s.timers = append(s.timers, &timer{at: at, seq: s.tseq, fire: fire})
}

// next mengambil goroutine berikutnya. Bila tidak ada yang siap, waktu virtual dimajukan
// ke timer terdekat. nil berarti semua goroutine menunggu selamanya (deadlock).
func (s *sched) next() *gor {
	for {
		if g := s.runnext; g != nil {
			s.runnext = nil
			return g
		}
		if len(s.runq) > 0 {
			g := s.runq[0]
			s.runq = s.runq[1:]
			return g
		}
		if len(s.timers) == 0 {
			return nil
		}
		best := 0
		for i, t := range s.timers {
			if t.at < s.timers[best].at || (t.at == s.timers[best].at && t.seq < s.timers[best].seq) {
				best = i
			}
		}
		t := s.timers[best]
		s.timers = append(s.timers[:best:best], s.timers[best+1:]...)
		if t.at > s.now {
			s.now = t.at
		}
		t.fire()
	}
}

func (it *interp) saveCtx() gctx {
	return gctx{calls: it.calls, depth: it.depth, quiet: it.quiet, frames: it.rec.frames, recDepth: it.rec.depth, active: it.rec.active}
}

func (it *interp) loadCtx(c gctx) {
	it.calls, it.depth, it.quiet = c.calls, c.depth, c.quiet
	it.rec.frames, it.rec.depth, it.rec.active = c.frames, c.recDepth, c.active
}

// runProgram menjalankan body sebagai goroutine main lalu menunggu program selesai.
// Mengembalikan nilai panic yang menghentikan program (nil bila selesai normal).
func (it *interp) runProgram(body func()) any {
	s := &sched{done: make(chan any, 1), exited: make(chan struct{}), waitq: map[*value][]*gor{}}
	if old := it.sch; old != nil {
		s.chans = old.chans // channel dari run sebelumnya tetap ada (mis. global)
	}
	it.sch = s
	g := s.newG("main")
	s.cur = g
	g.ctx = it.saveCtx()
	go it.runG(s, g, body)
	g.wake <- struct{}{}
	res := <-s.done
	// Goroutine yang masih menunggu dihentikan satu per satu: masing-masing membongkar
	// stack-nya dengan state interpreter miliknya sendiri, lalu memberi tahu lewat exited.
	s.killed = true
	for _, x := range s.gs {
		if !x.done && x != s.cur {
			close(x.wake)
			<-s.exited
		}
	}
	return res
}

// runG adalah badan goroutine Go untuk satu goroutine program.
func (it *interp) runG(s *sched, g *gor, body func()) {
	if _, ok := <-g.wake; !ok || s.killed {
		s.exited <- struct{}{}
		return
	}
	it.loadCtx(g.ctx)
	var res any
	func() {
		defer func() { res = recover() }()
		body()
	}()
	if _, killed := res.(killSignal); killed {
		s.exited <- struct{}{}
		return
	}
	g.done = true
	if g.id == 1 || res != nil {
		// main selesai, atau goroutine mana pun panic: seluruh program berakhir (seperti Go)
		s.done <- res
		return
	}
	next := s.next()
	if next == nil {
		s.done <- it.deadlockValue()
		return
	}
	s.cur = next
	next.wake <- struct{}{}
}

// deadlockValue merekam deadlock tanpa panic (dipakai di luar stack interpreter); bila
// perekaman sendiri berhenti (batas langkah), alasan berhentinya yang diteruskan.
func (it *interp) deadlockValue() (res any) {
	defer func() {
		if r := recover(); r != nil {
			res = r
		}
	}()
	return it.deadlock()
}

// yield menyerahkan giliran dari me (yang sudah menunggu atau sudah masuk antrean) dan
// kembali saat me mendapat giliran lagi.
func (it *interp) yield(me *gor) {
	s := it.sch
	next := s.next()
	if next == nil {
		panic(it.deadlock())
	}
	if next == me {
		return
	}
	me.ctx = it.saveCtx()
	s.cur = next
	next.wake <- struct{}{}
	if _, ok := <-me.wake; !ok || s.killed {
		it.loadCtx(me.ctx) // defer interpreter yang ikut terbongkar memakai state goroutine ini
		panic(killSignal{})
	}
	it.loadCtx(me.ctx)
}

// park membuat goroutine yang sedang jalan menunggu sampai dibangunkan lewat ready().
func (it *interp) park(why string, at ast.Node) {
	me := it.sch.cur
	me.parked, me.why = true, why
	if it.recording() && at != nil {
		it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(at), Note: fmt.Sprintf("G%d waits: %s", me.id, why)}, nil)
	}
	it.yield(me)
}

func (it *interp) sleep(ns int64, at ast.Node) {
	if ns <= 0 {
		return
	}
	s := it.sch
	me := s.cur
	me.parked, me.sleeping, me.why = true, true, "sleeping "+durText(ns)
	recorded := it.recording()
	if recorded {
		it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(at), Note: fmt.Sprintf("G%d sleeps %s (simulated time t=%s, wakes at t=%s)", me.id, durText(ns), durText(s.now), durText(s.now+ns))}, nil)
	}
	s.addTimer(s.now+ns, func() { s.ready(me) })
	it.yield(me)
	if c, ok := at.(*ast.CallExpr); ok && recorded {
		it.selfStep = c // langkahnya sudah dicatat di atas; jangan dicatat ulang oleh statement
	}
}

// gosched menaruh goroutine yang sedang jalan di ekor antrean (runtime.Gosched).
func (it *interp) gosched() {
	s := it.sch
	s.runq = append(s.runq, s.cur)
	it.yield(s.cur)
}

func (it *interp) deadlock() fatal {
	msg := "fatal error: all goroutines are asleep - deadlock!"
	err := it.panicInfo(nil, msg)
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseExit, Reason: trace.ReasonPanic, Note: msg, Err: err}, nil)
	}
	return fatal{msg: msg}
}

// goStmt menjalankan `go f(x)`: fungsi dan argumen dievaluasi sekarang, goroutine baru
// masuk runnext, dan goroutine yang membuatnya tetap lanjut.
func (it *interp) goStmt(n *ast.GoStmt, e *env) {
	s := it.sch
	call := n.Call
	var run func()
	f, isSel := unparen(call.Fun).(*ast.SelectorExpr)
	if sel := it.p.Info.Selections[f]; isSel && sel != nil {
		recv, decl := it.receiver(f, e)
		args := it.args(call, e)
		if decl != nil {
			run = func() {
				it.callFunc(&funcVal{name: recvTypeName(decl) + "." + decl.Name.Name, decl: decl, recv: &recv}, args, call)
			}
		}
	} else {
		fv := it.eval(call.Fun, e)
		args := it.args(call, e)
		if fv.k == kFunc && fv.fn != nil {
			run = func() { it.callFunc(fv.fn, args, call) }
		}
	}
	if run == nil {
		it.structural(fmt.Sprintf("go %s is not simulated yet (line %d)", it.text(call), it.line(n)))
	}
	s.used = true
	g := s.newG(it.text(call))
	// Goroutine baru ikut direkam bila yang membuatnya sedang direkam: ia mewarisi
	// salinan blok-blok yang sedang berjalan supaya langkahnya tetap milik blok fokus.
	if it.recording() {
		g.ctx = gctx{frames: append([]*frame(nil), it.rec.frames...), recDepth: it.rec.depth, active: 1}
	}
	go it.runG(s, g, func() {
		it.goEntry = true // badan fungsi goroutine direkam walau ditulis di luar blok fokus
		run()
	})
	s.ready(g)
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(n), Note: fmt.Sprintf("go %s → goroutine G%d created (ready to run)", it.text(call), g.id)}, e)
	}
}

// durText menulis durasi (ns) seperti time.Duration.String untuk nilai umum.
func durText(ns int64) string {
	switch {
	case ns%1e9 == 0:
		return fmt.Sprintf("%ds", ns/1e9)
	case ns%1e6 == 0:
		return fmt.Sprintf("%dms", ns/1e6)
	case ns%1e3 == 0:
		return fmt.Sprintf("%dµs", ns/1e3)
	}
	return fmt.Sprintf("%dns", ns)
}

func (it *interp) curG() int {
	if it.sch == nil || it.sch.cur == nil {
		return 0
	}
	return it.sch.cur.id
}

// gStates memotret semua goroutine untuk tampilan jalur.
func (it *interp) gStates() []trace.GState {
	s := it.sch
	out := make([]trace.GState, 0, len(s.gs))
	for _, g := range s.gs {
		st := trace.GState{ID: g.id, Name: g.name, State: "siap"}
		switch {
		case g.done:
			st.State = "selesai"
		case g.sleeping:
			st.State, st.Why = "tidur", g.why
		case g.parked:
			st.State, st.Why = "menunggu", g.why
		case g == s.cur:
			st.State = "jalan"
		}
		out = append(out, st)
	}
	return out
}

// chanStates memotret semua channel. Nama diambil dari variabel pertama yang terlihat
// memegang channel itu.
func (it *interp) chanStates(e *env) []trace.ChanState {
	s := it.sch
	if len(s.chans) == 0 {
		return nil
	}
	for sc := e; sc != nil; sc = sc.parent {
		for name, cell := range sc.vars {
			if cell.k == kChan && cell.ch != nil && cell.ch.name == "" {
				cell.ch.name = name
			}
		}
	}
	out := make([]trace.ChanState, 0, len(s.chans))
	for _, ch := range s.chans {
		if ch.timer == 2 && len(ch.buf) == 0 && len(ch.inflight) == 0 {
			continue // timer time.After yang sudah berbunyi dan nilainya sudah diambil
		}
		cs := trace.ChanState{ID: fmt.Sprintf("ch%d", ch.id), Name: ch.name, Cap: ch.cap, Closed: ch.closed, Items: []string{}}
		for _, w := range ch.inflight {
			cs.Items = append(cs.Items, format(w.v))
		}
		for _, v := range ch.buf {
			cs.Items = append(cs.Items, format(v))
		}
		for _, w := range ch.sendq {
			if w.live() {
				cs.Items = append(cs.Items, format(w.v)) // pengirim yang menunggu membawa nilainya
				cs.Senders++
			}
		}
		for _, w := range ch.recvq {
			if w.live() {
				cs.Receivers++
			}
		}
		out = append(out, cs)
	}
	return out
}
