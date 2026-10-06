package sim

import (
	"go/ast"
	"strings"

	"goloopviz/internal/trace"
)

// recorder mengumpulkan langkah-langkah selama loop fokus (dan loop di dalamnya) berjalan.
type recorder struct {
	focus    ast.Stmt
	maxSteps int
	maxIter  int

	active  int // > 0 selama loop fokus sedang berjalan
	runs    int
	reached bool
	frames  []*frame

	steps    []trace.Step
	ops      []trace.Op
	out      strings.Builder
	warnings []string
	seen     map[string]bool

	truncated bool

	coll       string // nama variabel koleksi untuk mode data ("" bila tidak ada)
	collKind   string
	firstItems []string
	dirty      bool // isi koleksi berubah sejak langkah terakhir
}

type frame struct {
	id    string
	iter  int
	focus bool
}

func (r *recorder) on() bool { return r.active > 0 }

func (r *recorder) warn(msg string) {
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	if !r.seen[msg] {
		r.seen[msg] = true
		r.warnings = append(r.warnings, msg)
	}
}

func (r *recorder) top() *frame {
	if len(r.frames) == 0 {
		return nil
	}
	return r.frames[len(r.frames)-1]
}

func (r *recorder) op(o trace.Op) {
	r.ops = append(r.ops, o)
	switch o.Op {
	case trace.OpSwap, trace.OpSet, trace.OpAppend:
		r.dirty = true
	}
}

// add menambahkan satu langkah. vars dan items diambil dari interpreter.
func (r *recorder) add(s trace.Step, vars map[string]string, items func() []string) {
	s.N = len(r.steps)
	s.Run = r.runs
	if f := r.top(); f != nil {
		s.Loop = f.id
		iter := f.iter
		s.Iteration = &iter
	}
	s.Vars = vars
	s.Ops, r.ops = r.ops, nil
	s.Output = r.out.String()
	r.out.Reset()
	if r.dirty {
		s.Items = items()
		r.dirty = false
	}
	r.steps = append(r.steps, s)
	if len(r.steps) >= r.maxSteps {
		r.truncated = true
		r.warn("trace dipotong: terlalu banyak langkah untuk ditampilkan")
		panic(abort{kind: abortDone})
	}
}
