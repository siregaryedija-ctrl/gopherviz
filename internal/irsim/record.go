package irsim

import (
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// Fn dan nodeRef hanyalah nama pendek untuk tipe IR.
type (
	Fn      = ir.Func
	nodeRef = ir.Node
)

// Cell adalah tempat satu variabel; pointer C/C++ menunjuk ke sini.
type Cell struct {
	v     Value
	typ   string // tipe statis yang dideklarasikan (C/C++/Java), untuk konversi saat assignment
	konst bool
	alias *Ptr // referensi (C++ int&, PHP &$x, foreach dengan &): baca/tulis diteruskan ke target
}

func (c *Cell) get() Value {
	if c.alias != nil {
		return c.alias.load()
	}
	return c.v
}

func (c *Cell) set(v Value) {
	if c.alias != nil {
		c.alias.store(v)
		return
	}
	c.v = v
}

func (p *Ptr) load() Value {
	switch {
	case p.cell != nil:
		return p.cell.get()
	case p.list != nil && p.idx >= 0 && p.idx < len(p.list.elems):
		return p.list.elems[p.idx]
	case p.obj != nil:
		v, _ := p.obj.fields.get(strV(p.field))
		return v
	case p.dict != nil:
		v, _ := p.dict.get(p.key)
		return v
	}
	return unknown("pointer menunjuk ke luar array")
}

func (p *Ptr) store(v Value) {
	switch {
	case p.cell != nil:
		p.cell.set(v)
	case p.list != nil && p.idx >= 0 && p.idx < len(p.list.elems):
		p.list.elems[p.idx] = v
	case p.obj != nil:
		p.obj.fields.set(strV(p.field), v)
	case p.dict != nil:
		p.dict.set(p.key, v)
	}
}

// Env adalah satu cakupan variabel.
type Env struct {
	vars   map[string]*Cell
	order  []string
	parent *Env
	fn     bool   // batas fungsi (snapshot variabel berhenti di sini)
	this   *Value // this/self/$this di fungsi ini
	class  *Class // kelas pemilik method (untuk super, self::, static)
	global bool
	uses   map[string]*Cell // PHP: variabel global/closure yang dibawa masuk
	arrow  bool             // PHP fn(): cakupan induk tetap terlihat
}

func newEnv(parent *Env, fn bool) *Env {
	return &Env{vars: map[string]*Cell{}, parent: parent, fn: fn}
}

func (e *Env) lookup(name string) *Cell {
	for s := e; s != nil; s = s.parent {
		if c, ok := s.vars[name]; ok {
			return c
		}
	}
	return nil
}

// local mencari hanya sampai batas fungsi (Python/PHP: variabel fungsi tidak bocor).
func (e *Env) local(name string) *Cell {
	for s := e; s != nil; s = s.parent {
		if c, ok := s.vars[name]; ok {
			return c
		}
		if s.fn && !s.arrow {
			return nil
		}
	}
	return nil
}

func (e *Env) define(name string, v Value) *Cell {
	if c, ok := e.vars[name]; ok {
		c.v = v
		return c
	}
	c := &Cell{v: v}
	e.vars[name] = c
	e.order = append(e.order, name)
	return c
}

// fnEnv mengembalikan cakupan fungsi terdekat (tempat var JS / variabel Python dibuat).
func (e *Env) fnEnv() *Env {
	for s := e; s != nil; s = s.parent {
		if s.fn || s.parent == nil {
			return s
		}
	}
	return e
}

func (e *Env) thisVal() *Value {
	for s := e; s != nil; s = s.parent {
		if s.this != nil {
			return s.this
		}
	}
	return nil
}

func (e *Env) owner() *Class {
	for s := e; s != nil; s = s.parent {
		if s.class != nil {
			return s.class
		}
	}
	return nil
}

// ---------- perekam ----------

// recorder mengumpulkan langkah selama blok fokus (dan blok di dalamnya) berjalan.
type recorder struct {
	focus    ir.Node
	maxSteps int
	maxIter  int

	active  int
	runs    int
	reached bool
	frames  []*frame
	depth   int

	steps     []trace.Step
	ops       []trace.Op
	out       strings.Builder
	warnings  []string
	seen      map[string]bool
	truncated bool

	coll     string // nama variabel koleksi untuk mode data
	collKind string // slice | array | map (ditentukan dari nilainya saat pertama terlihat)
	dirty    bool
}

type frame struct {
	id    string
	kind  scene.BlockKind
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

func (r *recorder) loopFrame() *frame {
	for i := len(r.frames) - 1; i >= 0; i-- {
		if r.frames[i].kind == scene.BlockLoop {
			return r.frames[i]
		}
		if r.frames[i].kind == scene.BlockFunc {
			return nil
		}
	}
	return nil
}

func (r *recorder) op(o trace.Op) {
	r.ops = append(r.ops, o)
	switch o.Op {
	case trace.OpSwap, trace.OpSet, trace.OpAppend, trace.OpDelete:
		r.dirty = true
	}
}

func (r *recorder) add(s trace.Step, vars map[string]string, items func() ([]string, []string)) {
	s.N = len(r.steps)
	s.Run = r.runs
	s.Depth = r.depth
	if f := r.top(); f != nil {
		s.Block = f.id
	}
	if f := r.loopFrame(); f != nil {
		s.Loop = f.id
		iter := f.iter
		s.Iteration = &iter
	}
	s.Vars = vars
	s.Ops, r.ops = r.ops, nil
	s.Output = r.out.String()
	r.out.Reset()
	if r.dirty {
		s.Items, s.Keys = items()
		r.dirty = false
	}
	r.steps = append(r.steps, s)
	if len(r.steps) >= r.maxSteps {
		r.truncated = true
		r.warn("trace dipotong: terlalu banyak langkah untuk ditampilkan")
		panic(abort{kind: abortDone})
	}
}

// ---------- abort / flow ----------

type abortKind int

const (
	abortStructural abortKind = iota
	abortBudget
	abortDone
)

type abort struct {
	kind abortKind
	msg  string
}

// thrown adalah exception program (throw / raise) yang bisa ditangkap try/catch.
type thrown struct {
	v   Value
	msg string // teks untuk catatan: "ZeroDivisionError: division by zero"
	at  ir.Node
}

type flowKind int

const (
	fNone flowKind = iota
	fBreak
	fContinue
	fReturn
)

type flow struct {
	k     flowKind
	label string
	count int // PHP: break 2
	ret   Value
}
