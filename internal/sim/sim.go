// Package sim mensimulasikan program Go untuk menghasilkan Trace satu loop:
// nilai variabel, hasil kondisi, output, dan operasi koleksi di setiap langkah.
//
// Simulator TIDAK menjalankan kode user. Ia membaca AST dan menghitung sendiri
// dengan aturan Go. Bila alur loop bergantung pada nilai yang tidak bisa diketahui
// (input user, package yang tidak didukung, goroutine), hasilnya mode "structural".
package sim

import (
	"fmt"
	"go/ast"
	"go/types"
	"math/rand"

	"goloopviz/internal/analyzer"
	"goloopviz/internal/scene"
	"goloopviz/internal/trace"
)

// Options membatasi simulasi supaya selalu cepat dan aman dipanggil di setiap ketikan.
type Options struct {
	MaxSteps int   // langkah trace maksimum (bawaan 1000)
	MaxIter  int   // putaran loop fokus maksimum per eksekusi (bawaan 50)
	Budget   int   // batas total evaluasi interpreter (bawaan 300.000)
	Seed     int64 // benih pengacak urutan map
}

func (o Options) withDefaults() Options {
	if o.MaxSteps <= 0 {
		o.MaxSteps = 1000
	}
	if o.MaxIter <= 0 {
		o.MaxIter = 50
	}
	if o.Budget <= 0 {
		o.Budget = 300_000
	}
	if o.Seed == 0 {
		o.Seed = 1
	}
	return o
}

// Trace mensimulasikan program p dan merekam loop loopID. Tidak pernah panik.
func Trace(p *analyzer.Program, loopID string, opt Options) (tr trace.Trace) {
	opt = opt.withDefaults()
	tr = trace.Trace{
		Version:  trace.Version,
		File:     p.Result.Scene.File,
		Mode:     trace.ModeStructural,
		Steps:    []trace.Step{},
		Warnings: []string{},
	}
	var loop *scene.Loop
	for i := range p.Result.Scene.Loops {
		if p.Result.Scene.Loops[i].ID == loopID {
			loop = &p.Result.Scene.Loops[i]
		}
	}
	if loop == nil {
		tr.Warnings = append(tr.Warnings, "loop "+loopID+" tidak ditemukan")
		return tr
	}
	tr.Loop = trace.LoopInfo{
		ID: loop.ID, Kind: loop.Kind, Line: loop.Line, EndLine: loop.EndLine,
		Label: loop.Label, Vars: loop.Vars, Children: loop.Children,
	}
	node := p.LoopNodes[loopID]
	if node == nil || loop.State == scene.StateIncomplete || len(p.Result.Diagnostics) > 0 {
		tr.Warnings = append(tr.Warnings, "kode belum lengkap atau masih ada error; selesaikan dulu supaya nilainya bisa dihitung")
		return tr
	}

	coll, collKind := findCollection(p, node)
	it := &interp{
		p:       p,
		globals: newEnv(nil, true),
		funcs:   map[string]*ast.FuncDecl{},
		budget:  opt.Budget,
		rng:     rand.New(rand.NewSource(opt.Seed)),
		rec:     &recorder{focus: node, maxSteps: opt.MaxSteps, maxIter: opt.MaxIter, coll: coll, collKind: collKind},
	}
	reason := it.run(node)
	r := it.rec

	tr.Warnings = append(tr.Warnings, r.warnings...)
	switch {
	case reason != "":
		tr.Warnings = append([]string{reason}, tr.Warnings...)
		return tr
	case !r.reached:
		tr.Warnings = append([]string{"loop ini tidak pernah dijalankan (fungsinya tidak dipanggil dari main)"}, tr.Warnings...)
		return tr
	}
	tr.Mode = trace.ModeSimulated
	tr.Steps = r.steps
	tr.Truncated = r.truncated
	if coll != "" {
		for _, s := range r.steps {
			if s.Items != nil {
				tr.Collection = &trace.Collection{Kind: collKind, Items: s.Items}
				break
			}
		}
	}
	return tr
}

// run menjalankan main (atau fungsi tempat loop berada) dan mengembalikan alasan
// bila simulasi harus beralih ke mode struktur.
func (it *interp) run(node ast.Stmt) (structural string) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		a, ok := r.(abort)
		if !ok {
			structural = fmt.Sprintf("kesalahan internal simulator: %v", r)
			return
		}
		switch a.kind {
		case abortStructural:
			structural = "nilai tidak bisa dihitung, jadi yang tampil hanya alurnya: " + a.msg
		case abortBudget:
			if len(it.rec.steps) > 0 {
				it.rec.truncated = true
				it.rec.warn(a.msg)
			} else {
				structural = a.msg
			}
		case abortPanic:
			if !it.rec.reached {
				structural = "program akan panic sebelum sampai ke loop ini: " + a.msg
			}
		}
	}()
	it.initGlobals()
	if main, ok := it.funcs["main"]; ok && main.Body != nil {
		it.callFunc(&funcVal{name: "main", decl: main}, nil, main)
	}
	if !it.rec.reached {
		if fd := enclosingFunc(it.p.File, node); fd != nil && fd.Name.Name != "main" {
			it.callFunc(&funcVal{name: fd.Name.Name, decl: fd}, nil, fd)
		}
	}
	return ""
}

func enclosingFunc(f *ast.File, node ast.Node) *ast.FuncDecl {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil && fd.Pos() <= node.Pos() && node.End() <= fd.End() {
			return fd
		}
	}
	return nil
}

// findCollection memilih slice/array yang paling sering diakses di dalam loop untuk mode data.
func findCollection(p *analyzer.Program, node ast.Node) (name, kind string) {
	score := map[string]int{}
	kinds := map[string]string{}
	consider := func(x ast.Expr, weight int) {
		id, ok := x.(*ast.Ident)
		if !ok {
			return
		}
		tv, ok := p.Info.Types[id]
		if !ok || tv.Type == nil {
			return
		}
		switch tv.Type.Underlying().(type) {
		case *types.Slice:
			kinds[id.Name] = "slice"
		case *types.Array:
			kinds[id.Name] = "array"
		default:
			return
		}
		score[id.Name] += weight
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IndexExpr:
			consider(x.X, 1)
		case *ast.RangeStmt:
			consider(x.X, 2)
		}
		return true
	})
	best := 0
	for n, s := range score {
		if s > best || (s == best && n < name) {
			name, best = n, s
		}
	}
	return name, kinds[name]
}
