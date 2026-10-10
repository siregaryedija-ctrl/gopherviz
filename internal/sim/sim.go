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
	"sort"

	"gopherviz/internal/analyzer"
	"gopherviz/internal/graphs"
	"gopherviz/internal/irsim"
	"gopherviz/internal/project"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
	"gopherviz/internal/userinput"
)

// Options membatasi simulasi supaya selalu cepat dan aman dipanggil di setiap ketikan.
type Options struct {
	MaxSteps int   // langkah trace maksimum (bawaan 1000)
	MaxIter  int   // putaran loop fokus maksimum per eksekusi (bawaan 50)
	Budget   int   // batas total evaluasi interpreter (bawaan 300.000)
	Seed     int64 // benih pengacak urutan map

	Input     []string // nilai input user dari panel; sisanya angka acak
	InputSeed int64    // benih angka acak input (0 = bawaan)

	Files project.Files // akses baca file lain (modul, file lain di package yang sama); nil = hanya file ini
}

func (o Options) withDefaults() Options {
	if o.MaxSteps <= 0 {
		o.MaxSteps = 5000
	}
	if o.MaxIter <= 0 {
		o.MaxIter = 1000
	}
	if o.Budget <= 0 {
		o.Budget = 300_000
	}
	if o.Seed == 0 {
		o.Seed = 1
	}
	return o
}

// Trace mensimulasikan program p dan merekam blok blockID (loop, if, switch, atau
// fungsi). Fokus pada fungsi main = timeline seluruh program. Tidak pernah panik.
func Trace(p *analyzer.Program, blockID string, opt Options) (tr trace.Trace) {
	opt = opt.withDefaults()
	tr = trace.Trace{
		Version:  trace.Version,
		Language: p.Result.Scene.Language,
		File:     p.Result.Scene.File,
		Mode:     trace.ModeStructural,
		Steps:    []trace.Step{},
		Warnings: []string{},
	}
	blk, ok := p.Result.Scene.Block(blockID)
	if !ok {
		tr.Warnings = append(tr.Warnings, "blok "+blockID+" tidak ditemukan")
		return tr
	}
	if p.Lang != "" && p.Lang != "go" {
		return irsim.Trace(p.Other, blockID, irsim.Options{MaxSteps: opt.MaxSteps, MaxIter: opt.MaxIter, Budget: opt.Budget, Seed: opt.Seed, Input: opt.Input, InputSeed: opt.InputSeed, Files: opt.Files})
	}
	if opt.Files != nil {
		p = analyzer.WithPackage(p, opt.Files) // file lain di package yang sama (Fase 10b)
	}
	tr.Block = trace.BlockInfo{ID: blk.ID, Kind: blk.Kind, Line: blk.Line, EndLine: blk.EndLine, Name: blk.Name}
	for _, loop := range p.Result.Scene.Loops {
		if loop.ID == blockID {
			tr.Loop = trace.LoopInfo{
				ID: loop.ID, Kind: loop.Kind, Line: loop.Line, EndLine: loop.EndLine,
				Label: loop.Label, Vars: loop.Vars, Children: loop.Children,
			}
		}
	}
	node := p.BlockNodes[blockID]
	if node == nil || blk.State == scene.StateIncomplete || len(p.Result.Diagnostics) > 0 {
		tr.Warnings = append(tr.Warnings, "kode belum lengkap atau masih ada error; selesaikan dulu supaya nilainya bisa dihitung")
		return tr
	}

	tr.Block.Names = blockNames(node)
	coll, collKind := findCollection(p, node)
	grid := findGrid(node)
	tree := findTree(node)
	var obj objects
	switch x := node.(type) {
	case *ast.TypeSpec: // trace class: struct beserta method-methodnya
		coll, collKind, grid, tree = "", "", nil, nil
		obj.typ, obj.className = x.Name.Name, x.Name.Name
		obj.methods = focusMethods(p.Files(), x.Name.Name)
		tr.Class = classInfo(p.Files(), x.Name.Name)
	case *ast.FuncDecl:
		if x.Recv != nil && recvTypeName(x) != "" {
			obj.method, obj.className = true, recvTypeName(x)
			tr.Class = classInfo(p.Files(), obj.className)
		}
	}
	it := &interp{
		p:       p,
		globals: newEnv(nil, true),
		funcs:   map[string]*ast.FuncDecl{},
		budget:  opt.Budget,
		rng:     rand.New(rand.NewSource(opt.Seed)),
		input:   userinput.New(opt.Input, opt.InputSeed),
		rec: &recorder{focus: node, maxSteps: opt.MaxSteps, maxIter: opt.MaxIter, coll: coll, collKind: collKind,
			mem: usesMemory(infoTypes{p.Info}, node), objIDs: map[*value]string{}, graphSpec: graphUnlessGrid(node, grid), gridSpec: grid, treeSpec: tree, obj: obj},
	}
	reason := it.run(node)
	r := it.rec

	tr.Warnings = append(tr.Warnings, r.warnings...)
	tr.ReadsInput = it.input.Reads() > 0
	if it.input.UsedRandom() {
		tr.Warnings = append(tr.Warnings, userinput.Warning)
	}
	switch {
	case reason != "":
		tr.Warnings = append([]string{reason}, tr.Warnings...)
		return tr
	case !r.reached:
		tr.Warnings = append([]string{"blok ini tidak pernah dijalankan (fungsinya tidak dipanggil dari main, atau kondisinya tidak pernah tercapai)"}, tr.Warnings...)
		return tr
	}
	tr.Mode = trace.ModeSimulated
	tr.Steps = r.steps
	tr.Entry, tr.Files = it.traceFiles()
	tr.Truncated = r.truncated
	tr.Graph = r.graph
	tr.Grid = r.grid
	tr.Tree = r.tree
	if r.tree == nil {
		for i := range tr.Steps {
			tr.Steps[i].Node = nil
		}
	}
	if r.grid != nil || r.tree != nil {
		coll = ""
	}
	if r.graph == nil {
		for i := range tr.Steps {
			tr.Steps[i].Graph = nil
		}
	}
	if coll != "" {
		for _, s := range r.steps {
			if s.Items != nil {
				tr.Collection = &trace.Collection{Kind: collKind, Items: s.Items, Keys: s.Keys}
				break
			}
		}
	}
	return tr
}

// run menjalankan main (atau fungsi tempat blok berada) dan mengembalikan alasan
// bila simulasi harus beralih ke mode struktur.
func (it *interp) run(node ast.Node) (structural string) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if ft, ok := r.(fatal); ok {
			if !it.rec.reached {
				structural = "program berhenti sebelum sampai ke blok ini: " + ft.msg
			} else {
				it.rec.warn("program berhenti: " + ft.msg)
				it.closeWithError(ft.msg)
			}
			return
		}
		if gp, ok := r.(goPanic); ok {
			// panic yang tidak ditangkap recover() sampai ke puncak program
			if !it.rec.reached {
				structural = "program akan panic sebelum sampai ke blok ini: " + gp.msg
			} else {
				it.rec.warn("program berhenti karena " + gp.msg)
				it.closeWithError(gp.msg)
			}
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
				structural = "program akan panic sebelum sampai ke blok ini: " + a.msg
			}
		}
	}()
	// Program (init global + main) berjalan sebagai goroutine main di penjadwal; panic atau
	// abort dari goroutine mana pun diteruskan ke sini.
	if r := it.runProgram(func() {
		it.initGlobals()
		if main, ok := it.funcs["main"]; ok && main.Body != nil {
			it.callFunc(&funcVal{name: "main", decl: main}, nil, main)
		}
		it.classFinish()
	}); r != nil {
		panic(r)
	}
	if !it.rec.reached {
		// Blok tidak tercapai dari main (atau tidak ada main): jalankan fungsi tempatnya
		// berada dengan parameter yang tidak diketahui.
		if fd := enclosingFunc(it.p.File, node); fd != nil && fd.Name.Name != "main" {
			if r := it.runProgram(func() { it.callFunc(&funcVal{name: fd.Name.Name, decl: fd}, nil, fd) }); r != nil {
				panic(r)
			}
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

// blockNames mengembalikan nama (identifier) yang dipakai di dalam blok, terurut dan unik.
func blockNames(node ast.Node) []string {
	seen := map[string]bool{}
	var out []string
	ast.Inspect(node, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name != "_" && !seen[id.Name] {
			seen[id.Name] = true
			out = append(out, id.Name)
		}
		return true
	})
	sort.Strings(out)
	return out
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
		case *types.Map:
			kinds[id.Name] = "map"
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
		case *ast.CallExpr: // delete(m, k), slices.Contains(s, x), sort.SearchInts(a, x)
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && len(x.Args) > 0 {
				if pkg, ok := sel.X.(*ast.Ident); ok {
					if _, ok := searchKinds[pkg.Name+"."+sel.Sel.Name]; ok {
						consider(x.Args[0], 2)
					}
				}
			}
			if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "delete" && len(x.Args) > 0 {
				consider(x.Args[0], 3)
			}
		case *ast.AssignStmt: // koleksi yang diubah lebih menarik daripada yang hanya dibaca
			for _, l := range x.Lhs {
				if ix, ok := l.(*ast.IndexExpr); ok {
					consider(ix.X, 3)
				}
			}
		case *ast.IncDecStmt:
			if ix, ok := x.X.(*ast.IndexExpr); ok {
				consider(ix.X, 3)
			}
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

// graphUnlessGrid: grid[r][c] bukan graf daftar ketetanggaan.
func graphUnlessGrid(node ast.Node, grid *graphs.GridSpec) *graphs.Spec {
	if grid != nil {
		return nil
	}
	return findGraph(node)
}
