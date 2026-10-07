package sim

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"gopherviz/internal/trace"
)

// waiter adalah goroutine yang menunggu di antrean kirim/terima sebuah channel.
// Untuk select, satu goroutine punya waiter di beberapa channel sekaligus; yang pertama
// "diklaim" menang, sisanya basi dan dilewati.
type waiter struct {
	g      *gor
	v      value // kirim: nilai yang dikirim; terima: nilai yang diterima
	zero   value // terima: nilai nol bila channel ditutup
	ok     bool
	closed bool // kirim: channel ditutup saat menunggu → panic
	sel    *selWait
	idx    int
}

type selWait struct{ fired int } // indeks case yang terpilih, -1 = belum

func (w *waiter) live() bool { return w.sel == nil || w.sel.fired < 0 }

func (w *waiter) claim() bool {
	if w.sel == nil {
		return true
	}
	if w.sel.fired >= 0 {
		return false
	}
	w.sel.fired = w.idx
	return true
}

func liveCount(q []*waiter) int {
	n := 0
	for _, w := range q {
		if w.live() {
			n++
		}
	}
	return n
}

func popWaiter(q *[]*waiter) *waiter {
	for len(*q) > 0 {
		w := (*q)[0]
		*q = (*q)[1:]
		if w.claim() {
			return w
		}
	}
	return nil
}

func (it *interp) newChan(capacity int) value {
	s := it.sch
	ch := &chanVal{cap: capacity, id: len(s.chans) + 1}
	s.chans = append(s.chans, ch)
	return value{k: kChan, ch: ch}
}

func (it *interp) chanName(x ast.Expr) string { return it.text(x) }

// trySend mengirim tanpa menunggu: langsung ke penerima yang sedang menunggu (serah
// terima, penerima dibangunkan), atau ke buffer bila masih ada ruang.
func (it *interp) trySend(ch *chanVal, v value) bool {
	if w := popWaiter(&ch.recvq); w != nil {
		w.v, w.ok = v, true
		ch.inflight = append(ch.inflight, w) // tampil di pipa sampai penerimanya jalan
		it.commOp(trace.OpSend, ch, len(ch.inflight)-1, v)
		it.sch.ready(w.g)
		return true
	}
	if len(ch.buf) < ch.cap {
		ch.buf = append(ch.buf, v)
		it.commOp(trace.OpSend, ch, len(ch.inflight)+len(ch.buf)-1, v)
		return true
	}
	return false
}

// commOp mencatat kirim/terima channel untuk animasi bola (slot = posisi di potret channel:
// nilai yang sedang diserahkan, lalu buffer, lalu nilai pengirim yang menunggu).
func (it *interp) commOp(kind trace.OpKind, ch *chanVal, slot int, v value) {
	if it.recording() {
		it.rec.op(trace.Op{Op: kind, A: slot, Key: fmt.Sprintf("ch%d", ch.id), Value: format(v)})
	}
}

// pickup dipanggil penerima yang dibangunkan dengan nilai serahan: nilainya keluar dari pipa.
func (it *interp) pickup(ch *chanVal, w *waiter) {
	for i, x := range ch.inflight {
		if x == w {
			ch.inflight = append(ch.inflight[:i:i], ch.inflight[i+1:]...)
			it.commOp(trace.OpRecv, ch, i, w.v)
			return
		}
	}
}

// tryRecv menerima tanpa menunggu. done=false berarti harus menunggu.
func (it *interp) tryRecv(ch *chanVal, zero value) (v value, ok, done bool) {
	slot := len(ch.inflight)
	if w := popWaiter(&ch.sendq); w != nil {
		if ch.cap == 0 {
			v = w.v // unbuffered: ambil langsung dari tangan pengirim
		} else {
			v = ch.buf[0] // buffer penuh: ambil kepala, nilai pengirim masuk ke ekor
			ch.buf = append(ch.buf[1:], w.v)
		}
		it.commOp(trace.OpRecv, ch, slot, v)
		it.sch.ready(w.g)
		return v, true, true
	}
	if len(ch.buf) > 0 {
		v = ch.buf[0]
		ch.buf = ch.buf[1:]
		it.commOp(trace.OpRecv, ch, slot, v)
		return v, true, true
	}
	if ch.closed {
		return zero, false, true
	}
	return value{}, false, false
}

// chanSend menjalankan `ch <- v`.
func (it *interp) chanSend(chv value, v value, at ast.Node, name string) {
	if chv.k != kChan {
		it.structural(fmt.Sprintf("kirim ke %s belum disimulasikan (baris %d)", name, it.line(at)))
	}
	ch := chv.ch
	if ch == nil {
		it.park("kirim ke channel nil (selamanya)", at)
	}
	if ch.closed {
		it.panicf(at, "send on closed channel")
	}
	if it.trySend(ch, v) {
		return
	}
	w := &waiter{g: it.sch.cur, v: v}
	ch.sendq = append(ch.sendq, w)
	it.commOp(trace.OpSend, ch, len(ch.inflight)+len(ch.buf)+liveCount(ch.sendq)-1, v) // nilai menunggu di pipa
	why := fmt.Sprintf("%s <- %s (menunggu penerima)", name, show(v))
	if ch.cap > 0 {
		why = fmt.Sprintf("%s <- %s (buffer penuh)", name, show(v))
	}
	it.park(why, at)
	if w.closed {
		it.panicf(at, "send on closed channel")
	}
}

// chanRecv menjalankan `<-ch`. ok=false bila channel sudah ditutup dan kosong.
func (it *interp) chanRecv(chv value, at ast.Node, name string, elem types.Type) (value, bool) {
	if chv.k != kChan {
		it.structural(fmt.Sprintf("terima dari %s belum disimulasikan (baris %d)", name, it.line(at)))
	}
	ch := chv.ch
	if ch == nil {
		it.park("terima dari channel nil (selamanya)", at)
	}
	zero := zeroOf(elem)
	if v, ok, done := it.tryRecv(ch, zero); done {
		return v, ok
	}
	w := &waiter{g: it.sch.cur, zero: zero}
	ch.recvq = append(ch.recvq, w)
	it.park("<-"+name+" (menunggu data)", at)
	if w.ok {
		it.pickup(ch, w)
	}
	return w.v, w.ok
}

func chanElem(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	if c, ok := t.Underlying().(*types.Chan); ok {
		return c.Elem()
	}
	return nil
}

// chanClose menutup channel: semua penerima yang menunggu dibangunkan dengan nilai nol,
// semua pengirim yang menunggu akan panic.
func (it *interp) chanClose(chv value, at ast.Node) {
	if chv.k != kChan || chv.ch == nil {
		it.panicf(at, "close of nil channel")
	}
	ch := chv.ch
	if ch.closed {
		it.panicf(at, "close of closed channel")
	}
	ch.closed = true
	for w := popWaiter(&ch.recvq); w != nil; w = popWaiter(&ch.recvq) {
		w.v, w.ok = w.zero, false
		it.sch.ready(w.g)
	}
	for w := popWaiter(&ch.sendq); w != nil; w = popWaiter(&ch.sendq) {
		w.closed = true
		it.sch.ready(w.g)
	}
}

// timeAfter menjalankan time.After(d): channel ber-buffer 1 yang terisi saat timer berbunyi.
func (it *interp) timeAfter(ns int64) value {
	s := it.sch
	chv := it.newChan(1)
	chv.ch.name = "time.After(" + durText(ns) + ")"
	chv.ch.timer = 1
	s.addTimer(s.now+ns, func() {
		chv.ch.timer = 2
		it.trySend(chv.ch, strV("t="+durText(s.now)))
	})
	return chv
}

// ---------- select ----------

type selCase struct {
	cc    *ast.CommClause
	send  bool
	ch    value
	name  string
	v     value
	elem  types.Type
	recvX ast.Expr // ekspresi <-ch untuk case penerima
}

func (it *interp) execSelect(n *ast.SelectStmt, e *env) flow {
	// Semua channel dan nilai kirim dievaluasi sekali, sebelum memilih (aturan Go).
	var cases []*selCase
	var def *ast.CommClause
	for _, s := range n.Body.List {
		cc := s.(*ast.CommClause)
		c := &selCase{cc: cc}
		switch comm := cc.Comm.(type) {
		case nil:
			def = cc
			continue
		case *ast.SendStmt:
			c.send, c.ch, c.v, c.name = true, it.eval(comm.Chan, e), it.eval(comm.Value, e), it.chanName(comm.Chan)
		case *ast.ExprStmt:
			c.recvX = comm.X
		case *ast.AssignStmt:
			c.recvX = comm.Rhs[0]
		}
		if c.recvX != nil {
			u, ok := unparen(c.recvX).(*ast.UnaryExpr)
			if !ok || u.Op != token.ARROW {
				it.structural(fmt.Sprintf("case select di baris %d belum disimulasikan", it.line(cc)))
			}
			c.ch, c.name = it.eval(u.X, e), it.chanName(u.X)
			c.elem = chanElem(it.p.Info.Types[u.X].Type)
		}
		if c.ch.k != kChan {
			it.structural(fmt.Sprintf("channel pada select di baris %d tidak bisa dihitung", it.line(cc)))
		}
		cases = append(cases, c)
	}

	// Coba setiap case dalam urutan acak (seperti Go); yang pertama bisa jalan menang.
	chosen := -1
	var got value
	var gotOK bool
	for _, i := range it.rng.Perm(len(cases)) {
		c := cases[i]
		ch := c.ch.ch
		if ch == nil {
			continue
		}
		if c.send {
			if ch.closed {
				it.panicf(c.cc, "send on closed channel")
			}
			if it.trySend(ch, c.v) {
				chosen = i
				break
			}
			continue
		}
		if v, ok, done := it.tryRecv(ch, zeroOf(c.elem)); done {
			chosen, got, gotOK = i, v, ok
			break
		}
	}

	scope := newEnv(e, false)
	var body []ast.Stmt
	switch {
	case chosen < 0 && def != nil:
		it.step(def, scope, "select: tidak ada case yang siap → default")
		body = def.Body
	case chosen < 0:
		// tidak ada yang siap: tunggu di semua channel sekaligus
		sel := &selWait{fired: -1}
		ws := make([]*waiter, len(cases))
		for i, c := range cases {
			if c.ch.ch == nil {
				continue
			}
			w := &waiter{g: it.sch.cur, sel: sel, idx: i, v: c.v, zero: zeroOf(c.elem)}
			if c.send {
				c.ch.ch.sendq = append(c.ch.ch.sendq, w)
			} else {
				c.ch.ch.recvq = append(c.ch.ch.recvq, w)
			}
			ws[i] = w
		}
		it.park("select (menunggu salah satu case siap)", n)
		chosen = sel.fired
		w := ws[chosen]
		if cases[chosen].send && w.closed {
			it.panicf(cases[chosen].cc, "send on closed channel")
		}
		got, gotOK = w.v, w.ok
		if !cases[chosen].send && w.ok {
			it.pickup(cases[chosen].ch.ch, w)
		}
	}
	if chosen >= 0 {
		c := cases[chosen]
		body = c.cc.Body
		if c.send {
			it.step(c.cc, scope, fmt.Sprintf("select: kirim %s ke %s", show(c.v), c.name))
		} else {
			if as, isAssign := c.cc.Comm.(*ast.AssignStmt); isAssign {
				vals := []value{got, boolV(gotOK)}
				for i, l := range as.Lhs {
					if i < len(vals) {
						it.target(l, scope, as.Tok == token.DEFINE).set(vals[i])
					}
				}
			}
			note := fmt.Sprintf("select: terima %s dari %s", show(got), c.name)
			if !gotOK {
				note = fmt.Sprintf("select: %s sudah ditutup → nilai nol", c.name)
			}
			it.step(c.cc, scope, note)
		}
	}
	f := it.execList(body, scope)
	if f.k == fBreak && f.label == "" {
		return flow{} // break di dalam select hanya keluar dari select
	}
	return f
}

// ---------- sync ----------

// syncCall menjalankan method sync.WaitGroup, sync.Mutex, sync.RWMutex, dan sync.Once.
// Status disimpan di field pertama struct tiruan (lihat stub "sync" di analyzer).
func (it *interp) syncCall(sel *ast.SelectorExpr, call *ast.CallExpr, e *env, typ, name string) []value {
	x := it.eval(sel.X, e)
	var cell *value
	if x.k == kPtr {
		cell = x.ptr
	} else if addressable(sel.X) {
		cell = it.ref(sel.X, e)
	}
	if cell == nil || cell.k != kStruct || cell.st == nil || len(cell.st.fields) == 0 {
		it.structural(fmt.Sprintf("%s belum disimulasikan (baris %d)", it.text(sel), it.line(sel)))
	}
	f := &cell.st.fields[0]
	s := it.sch
	label := it.text(sel.X)
	wakeAll := func() {
		for _, g := range s.waitq[f] {
			s.ready(g)
		}
		delete(s.waitq, f)
	}
	switch typ + "." + name {
	case "WaitGroup.Add", "WaitGroup.Done":
		d := int64(-1)
		if name == "Add" {
			args := it.args(call, e)
			if len(args) == 1 && args[0].k == kInt {
				d = args[0].i
			}
		}
		f.i += d
		if f.i < 0 {
			it.panicf(call, "sync: negative WaitGroup counter")
		}
		it.syncStep(call, e, fmt.Sprintf("%s → hitungan = %d", it.text(call), f.i))
		if f.i == 0 {
			wakeAll()
		}
	case "WaitGroup.Wait":
		for f.i != 0 {
			s.waitq[f] = append(s.waitq[f], s.cur)
			it.park(fmt.Sprintf("%s.Wait() (hitungan %d)", label, f.i), call)
		}
		it.syncStep(call, e, label+".Wait() selesai: hitungan sudah 0")
	case "Mutex.Lock", "RWMutex.Lock", "RWMutex.RLock":
		for f.b {
			s.waitq[f] = append(s.waitq[f], s.cur)
			it.park(label+".Lock() (pintu sedang dikunci)", call)
		}
		f.b = true
		it.syncStep(call, e, label+".Lock() → pintu dikunci")
	case "Mutex.Unlock", "RWMutex.Unlock", "RWMutex.RUnlock":
		if !f.b {
			panic(fatal{msg: "fatal error: sync: unlock of unlocked mutex"})
		}
		f.b = false
		it.syncStep(call, e, label+".Unlock() → pintu dibuka")
		if q := s.waitq[f]; len(q) > 0 {
			s.waitq[f] = q[1:]
			s.ready(q[0])
		}
	case "Once.Do":
		args := it.args(call, e)
		if !f.b && len(args) == 1 {
			f.b = true
			it.callValue(args[0], nil, call)
		}
	default:
		it.structural(fmt.Sprintf("sync.%s.%s belum disimulasikan", typ, name))
	}
	return nil
}

func (it *interp) syncStep(call *ast.CallExpr, e *env, note string) {
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(call), Note: note}, e)
		it.selfStep = call
	}
}

// syncType mengembalikan nama tipe sync (WaitGroup, Mutex, ...) bila sel adalah method-nya.
func syncType(sel *types.Selection) string { return stubType(sel, "sync") }
