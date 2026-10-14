package irsim

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/lang"
	"gopherviz/internal/project"
	"gopherviz/internal/race"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
	"gopherviz/internal/userinput"
)

// Options membatasi simulasi (sama dengan sim.Options).
type Options struct {
	MaxSteps int
	MaxIter  int
	Budget   int
	Seed     int64

	Input     []string // nilai input user dari panel; sisanya angka acak
	InputSeed int64    // benih angka acak input (0 = bawaan)

	Files project.Files // akses baca file lain (modul yang di-import); nil = hanya file ini

	// Call: fungsi fokus dipanggil langsung dengan list acak (lomba algoritma, Fase 12c).
	Call *race.Spec

	// outHook (khusus test): menerima seluruh output program saat Trace selesai menjalankannya.
	outHook func(string)
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

// Supported melaporkan apakah bahasa ini sudah bisa disimulasikan.
func Supported(language string) bool {
	_, ok := semFor(language)
	return ok
}

// Trace mensimulasikan program dan merekam blok blockID. Tidak pernah panik.
func Trace(res *lang.Result, blockID string, opt Options) (tr trace.Trace) {
	opt = opt.withDefaults()
	tr = trace.Trace{
		Version: trace.Version, Language: res.Scene.Language, File: res.Scene.File,
		Mode: trace.ModeStructural, Steps: []trace.Step{}, Warnings: []string{},
	}
	defer func() {
		if r := recover(); r != nil {
			tr.Mode, tr.Steps = trace.ModeStructural, []trace.Step{}
			tr.Warnings = append([]string{fmt.Sprintf("internal simulator error: %v", r)}, tr.Warnings...)
		}
	}()
	blk, ok := res.Scene.Block(blockID)
	if !ok {
		tr.Warnings = append(tr.Warnings, "block "+blockID+" not found")
		return tr
	}
	tr.Block = trace.BlockInfo{ID: blk.ID, Kind: blk.Kind, Line: blk.Line, EndLine: blk.EndLine, Name: blk.Name}
	for _, l := range res.Scene.Loops {
		if l.ID == blockID {
			tr.Loop = trace.LoopInfo{ID: l.ID, Kind: l.Kind, Line: l.Line, EndLine: l.EndLine, Label: l.Label, Vars: l.Vars, Children: l.Children}
		}
	}
	sem, ok := semFor(res.Scene.Language)
	if !ok {
		tr.Warnings = append(tr.Warnings, "value simulation for "+lang.Names[res.Scene.Language]+" is not available yet, so only the flow is shown")
		return tr
	}
	node := res.Nodes[blockID]
	if node == nil || blk.State == scene.StateIncomplete || len(res.Diagnostics) > 0 {
		tr.Warnings = append(tr.Warnings, "the code is incomplete or still has errors; finish it so the values can be computed")
		return tr
	}
	tr.Block.Names = blockNames(node)
	coll := findCollection(node)
	gspec := findGraph(node)
	grid := findGrid(node)
	if grid != nil {
		gspec = nil // grid[r][c] bukan graf daftar ketetanggaan
	}
	tree := findTree(node)
	var obj objects
	switch x := node.(type) {
	case *ir.Class: // trace class: object-objectnya, bukan koleksi/graf di dalam method
		coll, gspec, grid, tree = "", nil, nil, nil
		obj.cls = x
		tr.Class = classInfo(res, x)
	case *ir.Func:
		if c := classOfMethod(res, x); c != nil {
			obj.method = true
			tr.Class = classInfo(res, c)
		}
	}
	mk := func() *interp {
		it := &interp{
			lang: res.Scene.Language, sem: sem, res: res, src: res.Src,
			globals: newEnv(nil, true), budget: opt.Budget, rng: rand.New(rand.NewSource(opt.Seed)),
			rec:   &recorder{focus: node, maxSteps: opt.MaxSteps, maxIter: opt.MaxIter, coll: coll, graphSpec: gspec, gridSpec: grid, treeSpec: tree, obj: obj},
			funcs: map[string][]*Fn{}, classes: map[string]*Class{}, macros: map[string]Value{}, classOf: map[*Fn]*Class{},
			pyGlobals: map[*Env]map[string]bool{}, pyNonloc: map[*Env]map[string]bool{}, phpGlobal: map[*Env]map[string]bool{},
			statics: map[*ir.VarSpec]*Cell{}, input: userinput.New(opt.Input, opt.InputSeed),
		}
		it.globals.global = true
		it.newModules(opt.Files)
		it.budget0 = opt.Budget
		it.asyncUsed = usesAsync(res.Scene.Language, res.Src)
		if _, isFn := node.(*ir.Func); isFn && opt.Call != nil {
			it.raceSpec = opt.Call
		}
		return it
	}
	it := mk()
	if opt.outHook != nil {
		it.rec.all = &strings.Builder{}
	}
	reason := it.run(node, false)
	if opt.outHook != nil {
		opt.outHook(it.rec.all.String())
	}
	tr.Work = it.work
	if it.raceSpec != nil && it.raceSpec.Count {
		if reason != "" { // error atau terlalu panjang: jumlah operasinya tidak berarti
			tr.Warnings = append(tr.Warnings, reason)
		}
		return tr
	}
	if !it.rec.reached && opt.Files != nil && it.raceSpec == nil {
		// blok fokus tidak tercapai dari file ini sendiri (mis. modul yang hanya berisi fungsi):
		// jalankan program dari file yang meng-import-nya
		for _, p := range importers(res, opt.Files) {
			it2 := mk()
			entry := it2.loadModule(p)
			if entry == nil {
				continue
			}
			it2.entry = entry
			if reason2 := it2.run(node, false); it2.rec.reached {
				it, reason = it2, reason2
				tr.Entry = entry.name
				break
			}
		}
	}
	if !it.rec.reached && reason == "" && it.raceSpec == nil {
		it = mk()
		reason = it.run(node, true) // fungsi yang tidak dipanggil: parameter tidak diketahui
	}
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
		tr.Warnings = append([]string{"this block never runs (its function is not called, or its condition is never reached)"}, tr.Warnings...)
		return tr
	}
	tr.Mode = trace.ModeSimulated
	tr.Steps = r.steps
	tr.Files = it.traceFiles(tr.Entry)
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
		tr.Collection = nil
		coll = ""
	}
	if r.graph == nil {
		for i := range tr.Steps {
			tr.Steps[i].Graph = nil
		}
	}
	if coll != "" && r.collKind != "" {
		for _, s := range r.steps {
			if s.Items != nil {
				tr.Collection = &trace.Collection{Kind: r.collKind, Items: s.Items, Keys: s.Keys}
				break
			}
		}
	}
	return tr
}

// run menjalankan program dari titik masuknya dan mengembalikan alasan bila simulasi
// harus beralih ke mode struktur.
func (it *interp) run(node ir.Node, unknownArgs bool) (structural string) {
	defer it.killCoros() // setelah panic program ditangani: coroutine async yang tertunda dihentikan
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		switch x := r.(type) {
		case progExit:
			return
		case thrown:
			msg := x.msg
			if msg == "" {
				msg = it.exceptionText(x.v)
			}
			if !it.rec.reached {
				structural = "the program stops with an error before reaching this block: " + msg
				return
			}
			it.rec.warn("the program stops because of " + msg)
			it.finalStep(x, msg)
		case abort:
			switch x.kind {
			case abortStructural:
				structural = "values cannot be computed, so only the flow is shown: " + x.msg
			case abortBudget:
				if len(it.rec.steps) > 0 {
					it.rec.truncated = true
					it.rec.warn(x.msg)
				} else {
					structural = x.msg
				}
			}
		default:
			structural = fmt.Sprintf("internal simulator error: %v", r)
		}
	}()
	it.sem.setup(it)
	if fn, ok := node.(*ir.Func); ok && it.raceSpec != nil {
		it.runRace(fn)
		return ""
	}
	it.runProgram()
	it.classFinish()
	if !it.rec.reached && unknownArgs {
		if fn := enclosingFunc(it.res, node); fn != nil {
			it.callUnknown(fn)
		}
	}
	return ""
}

// finalStep merekam exception yang tidak tertangkap sebagai langkah terakhir.
func (it *interp) finalStep(x thrown, msg string) {
	defer func() { recover() }() // batas langkah sudah tercapai: abaikan
	line := 0
	if x.at != nil {
		line = it.line(x.at)
	}
	note := it.uncaughtNote(msg)
	if x.fatal {
		note = msg + " → program crash"
	}
	if strings.HasPrefix(msg, userinput.Ended) {
		note = "user input ran out → the simulation stops here"
		if n := len(it.rec.steps); line == 0 && n > 0 {
			line = it.rec.steps[n-1].Line
		}
	}
	if x.depth > 0 { // tumpukan panggilan sudah terurai: catat di lantai tempat error terjadi
		it.rec.depth = x.depth - 1
	}
	it.rec.add(trace.Step{Phase: trace.PhaseExit, Line: line, Reason: trace.ReasonPanic, Note: note, Err: it.errInfo(x, msg)}, nil, func() ([]string, []string) { return nil, nil })
}

func (it *interp) uncaughtNote(msg string) string {
	switch it.lang {
	case "python":
		return "Traceback: " + msg + " → the program stops"
	case "javascript", "typescript":
		return "Uncaught " + msg + " → the program stops"
	case "php":
		return "Fatal error: Uncaught " + msg + " → the program stops"
	case "java":
		return "Exception in thread \"main\" " + msg + " → the program stops"
	case "cpp":
		return "terminate called after throwing " + msg + " → the program stops"
	}
	return msg + " → the program stops"
}

// runProgram menjalankan titik masuk sesuai bahasa.
func (it *interp) runProgram() {
	entry := it.entry
	if entry == nil {
		entry = it.focusMod
	}
	defer it.setCur(entry)()
	body := entry.res.File.Body
	switch it.lang {
	case "javascript", "typescript":
		it.hoist(body, it.globals)
		it.execTop(body)
		it.runTasks()
	case "python":
		it.execTop(body)
	case "php":
		it.hoistPHP(body)
		it.execTop(body)
	case "c", "cpp":
		it.declareC(body, it.globals)
		if fs := it.funcs["main"]; len(fs) > 0 {
			ret := it.callFunc(&Func{name: "main", decl: fs[0], env: it.globals}, undef(), argList{}, fs[0])
			_ = ret
		}
	case "java":
		it.javaSiblings() // class lain di folder yang sama (satu package); class file ini menang bila namanya sama
		for _, s := range body {
			if c, ok := s.(*ir.Class); ok {
				it.defineClass(c, it.globals)
			}
		}
		for _, s := range body {
			c, ok := s.(*ir.Class)
			if !ok {
				continue
			}
			if it.classes[c.Name] == nil {
				continue
			}
			if m := it.classes[c.Name].methods["main"]; m != nil && m.Static {
				args := it.newList("array", nil)
				args.l.elem = "String"
				it.callFunc(&Func{name: "main", decl: m, env: it.globals, class: it.classes[c.Name], static: true}, undef(), argList{pos: []Value{args}}, m)
				return
			}
		}
		it.structural("there is no public static void main(String[] args) method")
	}
}

func (it *interp) execTop(body []ir.Stmt) {
	for _, s := range body {
		if f := it.exec(s, it.globals); f.k == fReturn {
			return
		}
	}
}

// hoistPHP: fungsi dan kelas tingkat atas PHP bisa dipakai sebelum ditulis.
func (it *interp) hoistPHP(body []ir.Stmt) {
	for _, s := range body {
		switch x := s.(type) {
		case *ir.Func:
			if x.Name != "" {
				it.globals.define(strings.ToLower(x.Name), Value{k: kFunc, fn: it.closure(x, it.globals)})
				it.funcs[strings.ToLower(x.Name)] = []*Fn{x}
			}
		case *ir.Class:
			c := it.defineClass(x, it.globals)
			it.classes[x.Name] = c
		}
	}
}

// declareC mendaftarkan fungsi, struct/kelas, makro, dan variabel global C/C++.
func (it *interp) declareC(body []ir.Stmt, e *Env) {
	for _, s := range body {
		switch x := s.(type) {
		case *ir.Func:
			if x.Body == nil {
				continue
			}
			if x.Class != "" { // int Point::area() { } di luar kelas
				if c := it.classes[x.Class]; c != nil {
					c.methods[x.Name] = x
					c.addOverload(x.Name, x)
					it.classOf[x] = c
					continue
				}
			}
			it.funcs[x.Name] = append(it.funcs[x.Name], x)
		case *ir.Class:
			c := it.defineClass(x, e)
			if x.Name != "" {
				it.classes[x.Name] = c
			}
		case *ir.Block: // namespace { ... }
			it.declareC(x.List, e)
		case *ir.Simple:
			it.simple(x, e)
		case *ir.VarDecl:
			if cls, name, ok := it.outOfClassStatic(x); ok {
				d := *x
				d.Specs = []*ir.VarSpec{{S: x.Specs[0].S, Name: name, Type: x.Specs[0].Type, Init: x.Specs[0].Init}}
				tmp := newEnv(e, false)
				it.varDecl(&d, tmp)
				cls.statics.set(strV(name), tmp.vars[name].get())
				continue
			}
			it.varDecl(x, e)
		}
	}
}

// outOfClassStatic mengenali "int Kelas::nama = nilai;" (definisi anggota statis C++).
func (it *interp) outOfClassStatic(d *ir.VarDecl) (*Class, string, bool) {
	if it.lang != "cpp" || len(d.Specs) != 1 {
		return nil, "", false
	}
	text := it.text(d.Specs[0])
	before, _, _ := strings.Cut(text, "=")
	before = strings.TrimSpace(before)
	if i := strings.LastIndex(before, "::"); i > 0 {
		fields := strings.Fields(before[:i])
		if len(fields) > 0 {
			if c := it.classes[fields[len(fields)-1]]; c != nil {
				return c, d.Specs[0].Name, true
			}
		}
	}
	return nil, "", false
}

func (it *interp) runTasks() {
	for i := 0; i < 10_000; i++ {
		if len(it.tasks) > 0 {
			t := it.tasks[0]
			it.tasks = it.tasks[1:]
			it.runTask(t.fn, t.label, t.rec, "microtask")
			continue
		}
		if len(it.timers) > 0 {
			best := 0
			for j, t := range it.timers {
				if t.at < it.timers[best].at || t.at == it.timers[best].at && t.seq < it.timers[best].seq {
					best = j
				}
			}
			t := it.timers[best]
			it.timers = append(it.timers[:best:best], it.timers[best+1:]...)
			it.runTask(t.fn, t.label, t.rec, "timer")
			continue
		}
		return
	}
}

// callUnknown menjalankan fungsi yang memuat blok fokus dengan parameter tidak diketahui.
func (it *interp) callUnknown(fn *ir.Func) {
	f := &Func{name: fn.Name, decl: fn, env: it.globals, class: it.classOf[fn]}
	this := undef()
	if c := it.classOf[fn]; c != nil && !fn.Static {
		it.ids++
		o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
		this = Value{k: kObj, o: o}
		func() {
			defer func() { recover() }()
			it.initFields(c, this)
		}()
	}
	var args []Value
	for _, p := range fn.Params {
		if p.Name == "self" && it.lang == "python" {
			continue
		}
		args = append(args, unknown(p.Name+" is a parameter of function "+fn.Name+", which is not called with a value"))
	}
	it.callFunc(f, this, argList{pos: args}, fn)
}

func enclosingFunc(res *lang.Result, node ir.Node) *ir.Func {
	ns := node.Span()
	var best *ir.Func
	for _, n := range res.Nodes {
		f, ok := n.(*ir.Func)
		if !ok || f.Body == nil {
			continue
		}
		fs := f.Span()
		if fs.Start.Off <= ns.Start.Off && ns.End.Off <= fs.End.Off {
			if best == nil || fs.End.Off-fs.Start.Off < best.S.End.Off-best.S.Start.Off {
				best = f
			}
		}
	}
	return best
}

// blockNames mengembalikan nama (identifier) yang dipakai di dalam blok, terurut dan unik.
func blockNames(node ir.Node) []string {
	seen := map[string]bool{}
	var out []string
	ir.Walk(node, func(n ir.Node) bool {
		if id, ok := n.(*ir.Ident); ok && id.Name != "_" && !seen[id.Name] {
			seen[id.Name] = true
			out = append(out, id.Name)
		}
		return true
	})
	sort.Strings(out)
	return out
}

// findCollection memilih variabel koleksi yang paling sering diakses di blok fokus.
func findCollection(node ir.Node) string {
	score := map[string]int{}
	order := []string{}
	add := func(x ir.Expr, w int) {
		id, ok := x.(*ir.Ident)
		if !ok {
			return
		}
		if _, seen := score[id.Name]; !seen {
			order = append(order, id.Name)
		}
		score[id.Name] += w
	}
	ir.Walk(node, func(n ir.Node) bool {
		switch x := n.(type) {
		case *ir.Index:
			add(x.X, 1)
		case *ir.ForEach:
			add(x.X, 2)
			if c, ok := x.X.(*ir.Call); ok { // enumerate(arr), arr.entries()
				if id, ok := c.Fun.(*ir.Ident); ok && id.Name == "enumerate" && len(c.Args) > 0 {
					add(c.Args[0], 2)
				} else if m, ok := c.Fun.(*ir.Member); ok && m.Name == "entries" {
					add(m.X, 2)
				}
			}
		case *ir.Assign:
			if ix, ok := x.L.(*ir.Index); ok {
				add(ix.X, 3)
			}
			if al, ok := x.L.(*ir.ArrayLit); ok {
				for _, el := range al.Elems {
					if ix, ok := el.(*ir.Index); ok {
						add(ix.X, 3)
					}
				}
			}
		case *ir.Binary:
			if x.Op == "in" || x.Op == "not in" {
				add(x.Y, 2)
			}
		case *ir.Call:
			if sp, ok := searchCall(x); ok {
				if name := collName(sp.coll); name != "" {
					add(&ir.Ident{Name: name}, 2)
				}
			}
			if m, ok := x.Fun.(*ir.Member); ok {
				switch m.Name {
				case "push", "append", "push_back", "add", "emplace_back", "insert", "pop", "remove", "put", "set":
					add(m.X, 3)
				}
			}
		}
		return true
	})
	best, bestScore := "", 0
	for _, name := range order {
		if score[name] > bestScore {
			best, bestScore = name, score[name]
		}
	}
	return best
}

// detectCollKind dipanggil sebelum langkah direkam: jenis koleksi diketahui dari nilainya.
func (it *interp) detectCollKind(e *Env) {
	if it.rec.coll == "" || it.rec.collKind != "" || e == nil {
		return
	}
	c := it.findVar(e, it.rec.coll)
	if c == nil {
		return
	}
	switch v := c.get(); v.k {
	case kList:
		if v.l.kind == "carray" || v.l.kind == "array" && it.lang == "java" {
			it.rec.collKind = "array"
		} else {
			it.rec.collKind = "slice"
		}
	case kDict:
		if v.d.kind == "array" && phpIsList(v.d) {
			it.rec.collKind = "slice"
		} else {
			it.rec.collKind = "map"
		}
	default:
		it.rec.coll = "" // bukan koleksi
	}
}

func phpIsList(d *Dict) bool {
	for i, k := range d.keys {
		if k.k != kInt || k.i != int64(i) {
			return false
		}
	}
	return true
}

func semFor(language string) (semantics, bool) {
	switch language {
	case "javascript", "typescript":
		return &jsSem{}, true
	case "python":
		return &pySem{}, true
	case "php":
		return &phpSem{}, true
	case "cpp":
		return &cSem{cpp: true}, true
	case "c":
		return &cSem{}, true
	case "java":
		return &javaSem{}, true
	}
	return nil, false
}

// Run mensimulasikan seluruh program tanpa merekam langkah dan mengembalikan stdout-nya
// (dipakai test pembanding dengan interpreter asli). err berisi alasan bila simulasi
// tidak selesai (mode struktur, exception tak tertangkap, batas komputasi).
func Run(res *lang.Result, opt Options) (stdout string, err string) {
	opt = opt.withDefaults()
	sem, ok := semFor(res.Scene.Language)
	if !ok {
		return "", "language not supported yet"
	}
	if len(res.Diagnostics) > 0 {
		return "", "the code still has errors: " + res.Diagnostics[0].Message
	}
	it := &interp{
		lang: res.Scene.Language, sem: sem, res: res, src: res.Src,
		globals: newEnv(nil, true), budget: opt.Budget, rng: rand.New(rand.NewSource(opt.Seed)),
		rec:   &recorder{maxSteps: opt.MaxSteps, maxIter: opt.MaxIter},
		funcs: map[string][]*Fn{}, classes: map[string]*Class{}, macros: map[string]Value{}, classOf: map[*Fn]*Class{},
		pyGlobals: map[*Env]map[string]bool{}, pyNonloc: map[*Env]map[string]bool{}, phpGlobal: map[*Env]map[string]bool{},
		statics: map[*ir.VarSpec]*Cell{}, input: userinput.New(opt.Input, opt.InputSeed),
	}
	it.globals.global = true
	it.newModules(opt.Files)
	func() {
		defer it.killCoros()
		defer func() {
			if r := recover(); r != nil {
				switch x := r.(type) {
				case progExit:
				case thrown:
					err = "exception: " + x.msg
				case abort:
					err = x.msg
				default:
					err = fmt.Sprintf("internal error: %v", r)
				}
			}
		}()
		it.sem.setup(it)
		it.runProgram()
	}()
	return it.rec.out.String(), err
}
