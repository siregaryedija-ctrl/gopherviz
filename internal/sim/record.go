package sim

import (
	"go/ast"
	"strings"

	"gopherviz/internal/graphs"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// recorder mengumpulkan langkah-langkah selama blok fokus (dan blok di dalamnya) berjalan.
type recorder struct {
	focus    ast.Node // *ast.ForStmt, *ast.RangeStmt, *ast.IfStmt, *ast.SwitchStmt, *ast.FuncDecl, *ast.FuncLit
	maxSteps int
	maxIter  int

	active  int // > 0 selama blok fokus sedang berjalan (rekursi menambah lebih dari 1)
	runs    int
	reached bool
	frames  []*frame
	depth   int // kedalaman panggilan fungsi sejak fokus dimulai

	steps    []trace.Step
	ops      []trace.Op
	out      strings.Builder
	warnings []string
	seen     map[string]bool

	truncated bool

	coll     string                // nama variabel koleksi untuk mode data ("" bila tidak ada)
	collKind string                // slice | array | map
	dirty    bool                  // isi koleksi berubah sejak langkah terakhir
	elems    map[string]trace.Elem // variabel range yang memegang elemen koleksi (for _, v := range arr)

	graphSpec *graphs.Spec // analisis DFS/BFS blok fokus (nil = bukan penelusuran graf)
	graph     *trace.Graph // graf yang sudah terbaca dari nilai program
	graphCur  string       // variabel simpul yang sedang diproses

	obj objects // trace class / method: struct yang digambar

	mem     bool              // sertakan potret memori di setiap langkah
	objIDs  map[*value]string // ID objek heap yang stabil antar langkah
	nextObj int
}

// frame adalah satu blok yang sedang berjalan selama perekaman.
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

// loopFrame mengembalikan loop terdalam yang sedang berjalan.
func (r *recorder) loopFrame() *frame {
	for i := len(r.frames) - 1; i >= 0; i-- {
		if r.frames[i].kind == scene.BlockLoop {
			return r.frames[i]
		}
		if r.frames[i].kind == scene.BlockFunc {
			return nil // loop di luar fungsi ini bukan milik langkah ini
		}
	}
	return nil
}

func (r *recorder) op(o trace.Op) {
	r.ops = append(r.ops, o)
	switch o.Op {
	case trace.OpSwap, trace.OpSet, trace.OpAppend:
		r.dirty = true
	}
}

// add menambahkan satu langkah. vars, isi koleksi, dan memori diambil dari interpreter.
func (r *recorder) add(s trace.Step, vars map[string]string, items func() ([]string, []string), mem func() *trace.Memory) {
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
	if r.mem {
		s.Mem = mem()
	}
	r.steps = append(r.steps, s)
	if len(r.steps) >= r.maxSteps {
		r.truncated = true
		r.warn("trace dipotong: terlalu banyak langkah untuk ditampilkan")
		panic(abort{kind: abortDone})
	}
}
