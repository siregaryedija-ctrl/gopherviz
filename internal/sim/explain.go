package sim

import (
	"go/ast"
	"strings"

	"gopherviz/internal/explain"
	"gopherviz/internal/trace"
	"gopherviz/internal/userinput"
)

// Penjelasan error (Fase 12b): tempat yang memicu panic menyimpan keterangan penyebabnya di
// it.errCtx; raise/fatal membawanya ke langkah terakhir (Step.Err).

// lastErr adalah panic/fatal terakhir, untuk langkah penutup bila panic terjadi saat tidak
// sedang merekam (mis. di fungsi yang dipanggil dari blok fokus).
type lastErr struct {
	line int
	msg  string
	err  *trace.ErrInfo
}

// panicInfo menjelaskan pesan panic Go ("panic: runtime error: ...", "fatal error: ...").
func (it *interp) panicInfo(at ast.Node, msg string) *trace.ErrInfo {
	c := it.errCtx
	it.errCtx = nil
	if c == nil {
		c = &explain.Ctx{Len: explain.NoLen}
	}
	if c.Expr == "" && at != nil {
		c.Expr = it.text(at)
	}
	name, text := "panic", strings.TrimPrefix(msg, "panic: ")
	if strings.HasPrefix(msg, "fatal error: ") {
		name, text = "fatal error", strings.TrimPrefix(msg, "fatal error: ")
	}
	e := explain.Error("go", name, text, c)
	line := 0
	if at != nil {
		line = it.line(at)
	}
	it.lastErr = &lastErr{line: line, msg: msg, err: e}
	return e
}

// stackOverflow menghentikan program seperti runtime Go saat stack habis (tidak bisa di-recover).
func (it *interp) stackOverflow(at ast.Node) {
	it.errCtx = &explain.Ctx{Len: explain.NoLen, Depth: maxDepth}
	msg := "fatal error: stack overflow"
	err := it.panicInfo(at, msg)
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseExit, Line: it.line(at), Reason: trace.ReasonPanic, Note: msg + " (recursion too deep)", Err: err}, nil)
	}
	panic(fatal{msg: msg})
}

// inputEnded menghentikan simulasi saat program terus membaca sesudah input habis (userinput).
func (it *interp) inputEnded() {
	msg := userinput.Ended + ": " + userinput.EndedMsg
	err := explain.Error("go", userinput.Ended, userinput.EndedMsg, nil)
	line := 0
	if n := len(it.rec.steps); n > 0 {
		line = it.rec.steps[n-1].Line
	}
	it.lastErr = &lastErr{line: line, msg: msg, err: err}
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseExit, Line: line, Reason: trace.ReasonPanic, Note: "user input ran out → the simulation stops here", Err: err}, nil)
	}
	panic(fatal{msg: msg})
}

// closeWithError menambahkan langkah penutup untuk panic/fatal yang tidak terekam saat terjadi.
func (it *interp) closeWithError(msg string) {
	r := it.rec
	if !r.reached || r.truncated || len(r.steps) > 0 && r.steps[len(r.steps)-1].Reason == trace.ReasonPanic {
		return
	}
	defer func() { recover() }() // batas langkah sudah tercapai: abaikan
	s := trace.Step{Phase: trace.PhaseExit, Reason: trace.ReasonPanic, Note: msg}
	if le := it.lastErr; le != nil && le.msg == msg {
		s.Line, s.Err = le.line, le.err
	} else {
		s.Err = it.panicInfo(nil, msg)
	}
	r.add(s, nil, func() ([]string, []string) { return nil, nil }, func() *trace.Memory { return nil })
}
