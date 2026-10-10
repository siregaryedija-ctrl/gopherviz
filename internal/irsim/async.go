package irsim

import (
	"fmt"
	"regexp"
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/trace"
)

// Event loop JavaScript (Fase 12d).
//
// Setiap tiket (microtask: Promise.then dan lanjutan await; timer: setTimeout) membawa label untuk
// panggung event loop. Fungsi async berjalan sebagai coroutine: goroutine Go sungguhan yang
// bergantian KETAT dengan pemanggilnya lewat kanal (tidak pernah berjalan bersamaan), sehingga
// `await` benar-benar menghentikan fungsi itu, pemanggil melanjutkan kode sinkronnya, dan fungsi
// itu dilanjutkan nanti sebagai microtask, persis seperti JS.

// task adalah satu tiket microtask.
type task struct {
	fn    func()
	label string
	rec   bool // dibuat saat blok fokus sedang direkam: isi callback-nya ikut direkam
}

// coro adalah satu pemanggilan fungsi async yang sedang berjalan atau tertunda di await.
type coro struct {
	name   string
	resume chan struct{}
	yield  chan coroMsg
	killed bool
	done   bool
	ctx    asyncCtx // keadaan interpreter milik coroutine saat ia tertunda
	own    []string // frame call stack JS milik coroutine (dari fungsi async ke dalam)
}

type coroMsg struct {
	done bool
	val  any // panic yang diteruskan ke pemanggil (abort, crash), nil bila selesai normal
}

// coroKill menghentikan coroutine yang masih tertunda saat program selesai.
type coroKill struct{}

// asyncCtx: bagian keadaan interpreter yang milik "utas" yang sedang berjalan.
type asyncCtx struct {
	depth, recDepth, active, quiet int
	frames                         []*frame
	cur                            *module
	jsStack                        []string
}

func (it *interp) saveCtx() asyncCtx {
	return asyncCtx{depth: it.depth, recDepth: it.rec.depth, active: it.rec.active, quiet: it.quiet,
		frames: append([]*frame(nil), it.rec.frames...), cur: it.cur, jsStack: append([]string(nil), it.jsStack...)}
}

func (it *interp) restoreCtx(c asyncCtx) {
	it.depth, it.rec.depth, it.rec.active, it.quiet = c.depth, c.recDepth, c.active, c.quiet
	it.rec.frames, it.cur, it.jsStack = c.frames, c.cur, c.jsStack
}

var reAsyncSrc = regexp.MustCompile(`\b(setTimeout|setInterval|Promise|async|await|queueMicrotask)\b|\.then\(`)

// usesAsync: program JS/TS ini memakai event loop (panggung tiga jalur).
func usesAsync(lang, src string) bool {
	return (lang == "javascript" || lang == "typescript") && reAsyncSrc.MatchString(src)
}

// fnLabel: teks pendek callback untuk tiket ("cetak", "() => console.log(...)").
func (it *interp) fnLabel(v Value) string {
	if v.k != kFunc || v.fn == nil {
		return "callback"
	}
	if d := v.fn.decl; d != nil {
		if d.Name != "" {
			return d.Name + "()"
		}
		if t := it.text(d); t != "" {
			return clipLabel(t, 28)
		}
	}
	if v.fn.name != "" {
		return v.fn.name + "()"
	}
	return "callback"
}

func clipLabel(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// queueTask menaruh tiket microtask di belakang antrian.
func (it *interp) queueTask(label string, fn func()) {
	it.tasks = append(it.tasks, task{fn: fn, label: label, rec: it.recording() || it.rec.on() && it.rec.reached})
}

// runTask menjalankan satu tiket. Tiket yang dibuat saat blok fokus direkam ikut direkam: langkah
// "event loop mengambil tiket" lalu isi callback-nya.
func (it *interp) runTask(fn func(), label string, rec bool, kind string) {
	if !rec || !it.asyncUsed {
		prev := it.curTask
		it.curTask = label
		defer func() { it.curTask = prev }()
		fn()
		return
	}
	r := it.rec
	r.active++
	prevDepth, prevTask := r.depth, it.curTask
	r.depth = 0
	it.curTask = label
	defer func() { r.active--; r.depth = prevDepth; it.curTask = prevTask }()
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseBody, Note: fmt.Sprintf("call stack kosong → event loop mengambil tiket %s: %s", kind, label)}, nil)
	}
	fn()
}

// evLoop: potret event loop untuk langkah ini (nil bila program tidak memakai async).
func (it *interp) evLoop() *trace.EventLoop {
	if !it.asyncUsed {
		return nil
	}
	ev := &trace.EventLoop{Stack: append([]string{}, it.jsStack...), Micro: []string{}, Macro: []string{}, Task: it.curTask}
	for _, t := range it.tasks {
		ev.Micro = append(ev.Micro, t.label)
	}
	for _, t := range it.sortedTimers() {
		ev.Macro = append(ev.Macro, t.label)
	}
	return ev
}

func (it *interp) sortedTimers() []timerTask {
	out := append([]timerTask(nil), it.timers...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].at < out[j-1].at || out[j].at == out[j-1].at && out[j].seq < out[j-1].seq); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// runAsync menjalankan isi fungsi async sebagai coroutine dan mengembalikan promise-nya: promise
// itu selesai saat isinya selesai (return) atau ditolak saat isinya melempar exception.
func (it *interp) runAsync(name string, body func(), result func() Value) Value {
	p := it.newPromise()
	c := &coro{name: name, resume: make(chan struct{}), yield: make(chan coroMsg)}
	it.coros = append(it.coros, c)
	c.own = []string{}
	go func() {
		defer func() {
			c.done = true
			c.yield <- coroMsg{done: true, val: recover()}
		}()
		<-c.resume
		if c.killed {
			panic(coroKill{})
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					if t, ok := r.(thrown); ok && !t.fatal {
						it.settle(p, t.v, true)
						return
					}
					panic(r)
				}
			}()
			body()
			it.resolveWith(p, result())
		}()
	}()
	it.enterCoro(c)
	return p
}

// enterCoro menyerahkan giliran ke coroutine c sampai ia tertunda (await) atau selesai.
// Panic dari dalam coroutine (batas langkah, crash) diteruskan ke pemanggil.
func (it *interp) enterCoro(c *coro) {
	if c.done {
		return
	}
	outer := it.saveCtx()
	prev := it.curCoro
	it.curCoro = c
	// frame call stack JS milik coroutine ditumpuk di atas pemanggil yang melanjutkannya
	c.ctx.jsStack = append(append([]string(nil), outer.jsStack...), c.own...)
	c.resume <- struct{}{}
	msg := <-c.yield
	it.restoreCtx(outer)
	it.curCoro = prev
	if msg.done {
		for i, x := range it.coros {
			if x == c {
				it.coros = append(it.coros[:i:i], it.coros[i+1:]...)
				break
			}
		}
	}
	if msg.val != nil {
		if _, kill := msg.val.(coroKill); !kill {
			panic(msg.val)
		}
	}
}

// awaitCoro: await di dalam fungsi async. Fungsi berhenti di sini, pemanggil melanjutkan kodenya;
// saat promise selesai, lanjutannya masuk antrian microtask.
func (it *interp) awaitCoro(c *coro, v Value, at ir.Node) Value {
	if v.k != kDict || v.d.kind != "promise" {
		v = it.resolvedPromise(v)
	}
	var got Value
	var rejected bool
	resume := func(val Value, rej bool) {
		got, rejected = val, rej
		it.enterCoro(c)
	}
	it.thenLabeled(v, bf("lanjut", func(it *interp, this Value, a []Value, at ir.Node) Value { resume(arg(a, 0), false); return undef() }),
		bf("gagal", func(it *interp, this Value, a []Value, at ir.Node) Value { resume(arg(a, 0), true); return undef() }),
		at, "lanjut "+c.name+"() setelah await")
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(at), Note: fmt.Sprintf("await → %s() berhenti di sini; lanjutannya menunggu di antrian microtask", c.name)}, nil)
	}
	// tertunda: simpan keadaan coroutine (frame call stack JS miliknya saja), serahkan giliran
	mine := it.saveCtx()
	c.own = c.ownFrames(mine.jsStack)
	c.yield <- coroMsg{}
	<-c.resume
	if c.killed {
		panic(coroKill{})
	}
	mine.jsStack = c.ctx.jsStack // dilanjutkan di atas call stack yang menjalankan tiketnya
	it.restoreCtx(mine)
	it.curCoro = c
	if rejected {
		panic(thrown{v: got, msg: it.exceptionText(got), at: at, depth: it.rec.depth + 1})
	}
	return got
}

// ownFrames: frame call stack milik coroutine = dari frame fungsi async-nya ke atas.
func (c *coro) ownFrames(stack []string) []string {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == c.name {
			return append([]string(nil), stack[i:]...)
		}
	}
	return []string{c.name}
}

// resolveWith menyelesaikan p dengan v; bila v promise, p mengikuti hasilnya.
func (it *interp) resolveWith(p, v Value) {
	if v.k == kDict && v.d.kind == "promise" {
		it.then(v, bf("ok", func(it *interp, this Value, a []Value, at ir.Node) Value {
			it.settle(p, arg(a, 0), false)
			return undef()
		}),
			bf("err", func(it *interp, this Value, a []Value, at ir.Node) Value {
				it.settle(p, arg(a, 0), true)
				return undef()
			}), nil)
		return
	}
	it.settle(p, v, false)
}

// killCoros menghentikan coroutine yang masih tertunda (promise yang tidak pernah selesai) saat
// program selesai, supaya goroutine-nya tidak tertinggal.
func (it *interp) killCoros() {
	it.rec.hold++ // langkah dari defer coroutine yang dihentikan tidak direkam
	defer func() { it.rec.hold-- }()
	saved := it.saveCtx()
	for len(it.coros) > 0 {
		c := it.coros[0]
		it.coros = it.coros[1:]
		if c.done {
			continue
		}
		c.killed = true
		c.resume <- struct{}{}
		<-c.yield
	}
	it.restoreCtx(saved)
}

// asyncLabel: label tiket timer.
func (it *interp) timerLabel(f Value, delay float64) string {
	return strings.TrimSpace(fmt.Sprintf("setTimeout %gms: %s", delay, it.fnLabel(f)))
}
