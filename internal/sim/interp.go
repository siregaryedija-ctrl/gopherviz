package sim

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"math/rand"
	"strings"
	"unicode/utf8"

	"gopherviz/internal/analyzer"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// abort menghentikan simulasi lewat panic yang ditangkap di Trace.
type abort struct {
	kind abortKind
	msg  string
}

type abortKind int

const (
	abortDone       abortKind = iota // rekaman cukup (selesai atau dipotong)
	abortStructural                  // nilai tidak diketahui memengaruhi alur
	abortPanic                       // program akan panic
	abortBudget                      // terlalu banyak langkah
)

type flowKind uint8

const (
	fNone flowKind = iota
	fBreak
	fContinue
	fReturn
)

type flow struct {
	k     flowKind
	label string
	ret   []value
}

type env struct {
	vars   map[string]*value
	order  []string
	parent *env
	fn     bool // batas fungsi
}

func newEnv(parent *env, fn bool) *env {
	return &env{vars: map[string]*value{}, parent: parent, fn: fn}
}

func (e *env) lookup(name string) *value {
	for s := e; s != nil; s = s.parent {
		if v, ok := s.vars[name]; ok {
			return v
		}
	}
	return nil
}

func (e *env) define(name string, v value) *value {
	if name == "_" {
		return &value{}
	}
	if _, ok := e.vars[name]; !ok {
		e.order = append(e.order, name)
	}
	cell := &v
	e.vars[name] = cell
	return cell
}

type interp struct {
	p       *analyzer.Program
	globals *env
	funcs   map[string]*ast.FuncDecl
	budget  int
	depth   int
	rec     *recorder
	rng     *rand.Rand
	quiet   int
}

const maxDepth = 200

func (it *interp) tick() {
	it.budget--
	if it.budget < 0 {
		panic(abort{abortBudget, "program terlalu panjang untuk disimulasikan"})
	}
}

func (it *interp) line(n ast.Node) int    { return it.p.Line(n.Pos()) }
func (it *interp) text(n ast.Node) string { return it.p.Text(n) }

func (it *interp) structural(msg string) { panic(abort{abortStructural, msg}) }

// panicf mensimulasikan runtime panic Go: dicatat sebagai langkah terakhir bila sedang merekam.
func (it *interp) panicf(at ast.Node, format string, args ...any) {
	msg := "panic: " + fmt.Sprintf(format, args...)
	if it.rec.on() && it.quiet == 0 {
		it.add(trace.Step{Phase: trace.PhaseExit, Line: it.line(at), Reason: trace.ReasonPanic, Note: msg}, nil)
	}
	panic(abort{abortPanic, msg})
}

func (it *interp) warn(msg string) { it.rec.warn(msg) }

// ---------- perekaman ----------

// recording: loop fokus sedang berjalan dan kode yang dieksekusi memang tertulis di dalamnya
// (fungsi lain yang dipanggil dari dalam loop dijalankan dalam mode quiet).
func (it *interp) recording() bool { return it.rec.on() && it.quiet == 0 }

func (it *interp) insideFocus(n ast.Node) bool {
	f := it.rec.focus
	return f != nil && f.Pos() <= n.Pos() && n.End() <= f.End()
}

func (it *interp) add(s trace.Step, e *env) {
	var vars map[string]string
	if e != nil {
		vars = it.snapshot(e)
	}
	it.rec.add(s, vars, func() []string { return it.collItems(e) })
}

func (it *interp) step(n ast.Node, e *env, note string) {
	if !it.recording() {
		return
	}
	if note == "" {
		note = it.text(n)
	}
	it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(n), Note: note}, e)
}

// snapshot mengambil variabel di fungsi yang sedang berjalan (cakupan terdalam menang).
func (it *interp) snapshot(e *env) map[string]string {
	out := map[string]string{}
	for s := e; s != nil; s = s.parent {
		for i := len(s.order) - 1; i >= 0; i-- {
			name := s.order[i]
			if _, ok := out[name]; ok || len(out) >= 12 {
				continue
			}
			v := *s.vars[name]
			if v.k == kFunc {
				continue
			}
			out[name] = show(v)
		}
		if s.fn {
			break
		}
	}
	return out
}

func (it *interp) collItems(e *env) []string {
	if it.rec.coll == "" || e == nil {
		return nil
	}
	cell := e.lookup(it.rec.coll)
	if cell == nil {
		return nil
	}
	var elems []value
	switch cell.k {
	case kSlice:
		elems = cell.sl.elems()
	case kArray:
		elems = cell.arr
	default:
		return nil
	}
	items := make([]string, len(elems))
	for i, v := range elems {
		items[i] = format(v)
	}
	return items
}

// collIndex mengembalikan indeks bila x berbentuk koleksi[i], atau -1.
func (it *interp) collIndex(x ast.Expr, e *env) int {
	ix, ok := unparen(x).(*ast.IndexExpr)
	if !ok || it.rec.coll == "" {
		return -1
	}
	id, ok := ix.X.(*ast.Ident)
	if !ok || id.Name != it.rec.coll {
		return -1
	}
	v := it.evalQuiet(ix.Index, e)
	if v.k != kInt {
		return -1
	}
	return int(v.i)
}

func unparen(x ast.Expr) ast.Expr {
	for {
		p, ok := x.(*ast.ParenExpr)
		if !ok {
			return x
		}
		x = p.X
	}
}

func (it *interp) evalQuiet(x ast.Expr, e *env) value {
	it.quiet++
	defer func() { it.quiet-- }()
	return it.eval(x, e)
}

// subst menulis ulang ekspresi dengan nilai variabel, mis. "i < n" menjadi "0 < 3".
func (it *interp) subst(x ast.Expr, e *env) string {
	switch n := x.(type) {
	case *ast.BinaryExpr:
		if !isCompare(n.Op) && n.Op != token.LAND && n.Op != token.LOR {
			return show(it.evalQuiet(n, e)) // aritmetika langsung dihitung: "n-1" menjadi "5"
		}
		return it.subst(n.X, e) + " " + n.Op.String() + " " + it.subst(n.Y, e)
	case *ast.ParenExpr:
		return "(" + it.subst(n.X, e) + ")"
	case *ast.UnaryExpr:
		return n.Op.String() + it.subst(n.X, e)
	case *ast.Ident:
		if cell := e.lookup(n.Name); cell != nil && cell.k != kFunc {
			return show(*cell)
		}
	case *ast.IndexExpr:
		return show(it.evalQuiet(n, e))
	case *ast.CallExpr:
		if id, ok := n.Fun.(*ast.Ident); ok && (id.Name == "len" || id.Name == "cap") && it.isBuiltin(id) {
			return show(it.evalQuiet(n, e))
		}
	}
	return it.text(x)
}

func (it *interp) condNote(x ast.Expr, e *env, result bool, prefix string) string {
	src, sub := it.text(x), it.subst(x, e)
	if sub == src {
		return fmt.Sprintf("%s%s → %v", prefix, src, result)
	}
	return fmt.Sprintf("%s%s → %s → %v", prefix, src, sub, result)
}

// effectNote menjelaskan hasil assignment, mis. "sum += v → sum = 12".
func (it *interp) effectNote(n ast.Stmt, lhs []ast.Expr, e *env) string {
	var parts []string
	for _, l := range lhs {
		switch t := unparen(l).(type) {
		case *ast.Ident:
			if t.Name == "_" {
				continue
			}
			if cell := e.lookup(t.Name); cell != nil {
				parts = append(parts, t.Name+" = "+show(*cell))
			}
		case *ast.IndexExpr:
			parts = append(parts, it.text(t)+" = "+show(it.evalQuiet(t, e)))
		}
	}
	if len(parts) == 0 {
		return it.text(n)
	}
	return it.text(n) + " → " + strings.Join(parts, ", ")
}

// ---------- statement ----------

func (it *interp) execList(list []ast.Stmt, e *env) flow {
	for _, s := range list {
		if f := it.exec(s, e); f.k != fNone {
			return f
		}
	}
	return flow{}
}

func (it *interp) exec(s ast.Stmt, e *env) flow {
	it.tick()
	switch n := s.(type) {
	case *ast.BlockStmt:
		return it.execList(n.List, newEnv(e, false))
	case *ast.ExprStmt:
		it.eval(n.X, e)
		note := ""
		if out := it.rec.out.String(); out != "" && it.recording() {
			note = it.text(n) + " → cetak " + fmt.Sprintf("%q", strings.TrimSuffix(out, "\n"))
		}
		it.step(n, e, note)
	case *ast.AssignStmt:
		it.assign(n, e)
		it.step(n, e, it.effectNote(n, n.Lhs, e))
	case *ast.IncDecStmt:
		op := token.ADD
		if n.Tok == token.DEC {
			op = token.SUB
		}
		t := it.target(n.X, e, false)
		t.set(it.binary(op, t.get(), intV(1), n))
		it.markColl(n.X)
		it.step(n, e, it.effectNote(n, []ast.Expr{n.X}, e))
	case *ast.DeclStmt:
		it.decl(n.Decl, e)
		it.step(n, e, "")
	case *ast.IfStmt:
		return it.execIf(n, e)
	case *ast.ForStmt:
		return it.execFor(n, "", e)
	case *ast.RangeStmt:
		return it.execRange(n, "", e)
	case *ast.LabeledStmt:
		switch l := n.Stmt.(type) {
		case *ast.ForStmt:
			return it.execFor(l, n.Label.Name, e)
		case *ast.RangeStmt:
			return it.execRange(l, n.Label.Name, e)
		}
		return it.exec(n.Stmt, e)
	case *ast.BranchStmt:
		label := ""
		if n.Label != nil {
			label = n.Label.Name
		}
		switch n.Tok {
		case token.BREAK:
			if it.recording() {
				it.add(trace.Step{Phase: trace.PhaseBreak, Line: it.line(n), Note: strings.TrimSpace("break "+label) + " → keluar dari loop"}, e)
			}
			return flow{k: fBreak, label: label}
		case token.CONTINUE:
			if it.recording() {
				it.add(trace.Step{Phase: trace.PhaseContinue, Line: it.line(n), Note: strings.TrimSpace("continue "+label) + " → langsung ke putaran berikutnya"}, e)
			}
			return flow{k: fContinue, label: label}
		default:
			it.structural(n.Tok.String() + " belum disimulasikan")
		}
	case *ast.ReturnStmt:
		var vals []value
		if len(n.Results) == 1 {
			vals = it.evalMulti(n.Results[0], e, -1)
		} else {
			for _, r := range n.Results {
				vals = append(vals, it.eval(r, e))
			}
		}
		it.step(n, e, it.text(n)+" → keluar dari fungsi")
		return flow{k: fReturn, ret: vals}
	case *ast.SwitchStmt:
		return it.execSwitch(n, e)
	case *ast.SendStmt:
		ch := it.eval(n.Chan, e)
		v := it.eval(n.Value, e)
		if ch.k != kChan || ch.ch == nil {
			it.structural("kirim ke channel ini belum disimulasikan")
		}
		if ch.ch.closed {
			it.panicf(n, "send on closed channel")
		}
		if len(ch.ch.buf) >= ch.ch.cap {
			it.structural("channel penuh: program butuh goroutine lain (belum disimulasikan)")
		}
		ch.ch.buf = append(ch.ch.buf, v)
		it.step(n, e, "")
	case *ast.EmptyStmt:
	case *ast.GoStmt:
		it.warn(fmt.Sprintf("goroutine (baris %d) tidak disimulasikan", it.line(n)))
	case *ast.DeferStmt:
		it.warn(fmt.Sprintf("defer (baris %d) tidak disimulasikan", it.line(n)))
	default:
		it.structural(fmt.Sprintf("statement di baris %d belum disimulasikan", it.line(n)))
	}
	return flow{}
}

// execIf menjalankan satu rantai if / else if / else. Setiap kondisi yang dicek direkam;
// bila if ini adalah blok fokus, cabang yang dipilih dan akhir blok juga direkam.
func (it *interp) execIf(n *ast.IfStmt, e *env) flow {
	fr, leave := it.enterBlock(n, scene.BlockIf)
	defer leave()
	focus := fr != nil && fr.focus
	branch := 0
	cur, parent := n, e
	for {
		scope := newEnv(parent, false)
		if cur.Init != nil {
			it.exec(cur.Init, scope)
		}
		c := it.cond(cur.Cond, scope, "if")
		if it.recording() {
			r := c
			prefix := "if "
			if cur != n {
				prefix = "else if "
			}
			it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(cur), Result: &r, Note: it.condNote(cur.Cond, scope, c, prefix)}, scope)
		}
		if c {
			it.branchStep(focus, n, branch, scope, fmt.Sprintf("masuk cabang ke-%d (%s)", branch+1, strings.TrimSpace(prefixOf(cur == n)+it.text(cur.Cond))))
			return it.blockExit(focus, n, it.exec(cur.Body, scope), scope)
		}
		branch++
		switch el := cur.Else.(type) {
		case *ast.IfStmt:
			cur, parent = el, scope
			continue
		case *ast.BlockStmt:
			it.branchStep(focus, n, branch, scope, "semua kondisi false → masuk cabang else")
			return it.blockExit(focus, n, it.exec(el, scope), scope)
		}
		it.branchStep(focus, n, branch, scope, "semua kondisi false dan tidak ada else → lewati")
		return it.blockExit(focus, n, flow{}, scope)
	}
}

func prefixOf(first bool) string {
	if first {
		return "if "
	}
	return "else if "
}

// branchStep merekam cabang yang dipilih, hanya bila if/switch ini blok fokus.
func (it *interp) branchStep(focus bool, n ast.Node, idx int, e *env, note string) {
	if !focus || !it.recording() {
		return
	}
	b := idx
	it.add(trace.Step{Phase: trace.PhaseBranch, Line: it.line(n), Branch: &b, Note: note}, e)
}

// blockExit merekam akhir if/switch fokus beserta alasannya, lalu meneruskan flow.
func (it *interp) blockExit(focus bool, n ast.Node, f flow, e *env) flow {
	if !focus || !it.recording() {
		return f
	}
	reason, note := trace.ReasonDone, "selesai, lanjut ke baris berikutnya"
	switch f.k {
	case fBreak:
		reason, note = trace.ReasonBreak, "keluar karena break"
	case fContinue:
		reason, note = trace.ReasonContinue, "keluar karena continue: loop di luarnya lanjut ke putaran berikutnya"
	case fReturn:
		reason, note = trace.ReasonReturn, "keluar karena return"
	}
	it.add(trace.Step{Phase: trace.PhaseExit, Line: it.p.Line(n.End() - 1), Reason: reason, Note: note}, e)
	return f
}

// cond mengevaluasi kondisi; nilai yang tidak diketahui membuat simulasi beralih ke mode struktur.
func (it *interp) cond(x ast.Expr, e *env, what string) bool {
	v := it.eval(x, e)
	if v.k != kBool {
		why := v.why
		if why == "" {
			why = "nilainya tidak diketahui"
		}
		it.structural(fmt.Sprintf("kondisi %s %q di baris %d tidak bisa dihitung: %s", what, it.text(x), it.line(x), why))
	}
	return v.b
}

func (it *interp) execSwitch(n *ast.SwitchStmt, e *env) flow {
	fr, leave := it.enterBlock(n, scene.BlockSwitch)
	defer leave()
	focus := fr != nil && fr.focus
	scope := newEnv(e, false)
	if n.Init != nil {
		it.exec(n.Init, scope)
	}
	tag := boolV(true)
	if n.Tag != nil {
		tag = it.eval(n.Tag, scope)
		if focus && it.recording() {
			it.add(trace.Step{Phase: trace.PhaseInit, Line: it.line(n), Note: fmt.Sprintf("switch %s → %s", it.text(n.Tag), show(tag))}, scope)
		}
	}
	def := -1
	for i, s := range n.Body.List {
		cc := s.(*ast.CaseClause)
		if cc.List == nil {
			def = i
			continue
		}
		for _, x := range cc.List {
			v := it.eval(x, scope)
			eq := it.binary(token.EQL, tag, v, x)
			if eq.k != kBool {
				it.structural(fmt.Sprintf("case di baris %d tidak bisa dihitung", it.line(x)))
			}
			if focus && it.recording() {
				r := eq.b
				note := fmt.Sprintf("case %s → %s == %s → %v", it.text(x), show(tag), show(v), eq.b)
				if n.Tag == nil {
					note = it.condNote(x, scope, eq.b, "case ")
				}
				it.add(trace.Step{Phase: trace.PhaseCond, Line: it.line(cc), Result: &r, Note: note}, scope)
			}
			if eq.b {
				return it.caseBody(focus, n, cc, i, scope)
			}
		}
	}
	if def >= 0 {
		return it.caseBody(focus, n, n.Body.List[def].(*ast.CaseClause), def, scope)
	}
	it.branchStep(focus, n, len(n.Body.List), scope, "tidak ada case yang cocok → lewati")
	return it.blockExit(focus, n, flow{}, scope)
}

func (it *interp) caseBody(focus bool, n *ast.SwitchStmt, cc *ast.CaseClause, idx int, e *env) flow {
	if focus {
		it.branchStep(focus, n, idx, e, "masuk "+map[bool]string{true: "default", false: "case " + it.textList(cc.List)}[cc.List == nil])
	} else {
		it.step(cc, e, "case "+it.textList(cc.List))
	}
	f := it.execList(cc.Body, newEnv(e, false))
	if f.k == fBreak && f.label == "" {
		f = flow{} // break di dalam switch hanya keluar dari switch
	}
	return it.blockExit(focus, n, f, e)
}

func (it *interp) textList(xs []ast.Expr) string {
	if len(xs) == 0 {
		return "default"
	}
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = it.text(x)
	}
	return strings.Join(parts, ", ")
}

func (it *interp) decl(d ast.Decl, e *env) {
	gd, ok := d.(*ast.GenDecl)
	if !ok || (gd.Tok != token.VAR && gd.Tok != token.CONST) {
		return
	}
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		var vals []value
		if len(vs.Values) == 1 && len(vs.Names) > 1 {
			vals = it.evalMulti(vs.Values[0], e, len(vs.Names))
		} else {
			for _, x := range vs.Values {
				vals = append(vals, it.eval(x, e))
			}
		}
		for i, name := range vs.Names {
			var v value
			if i < len(vals) {
				v = copyVal(vals[i])
			} else if obj := it.p.Info.Defs[name]; obj != nil {
				v = zeroOf(obj.Type())
			} else {
				v = unknown("tipe " + name.Name + " tidak diketahui")
			}
			if obj := it.p.Info.Defs[name]; obj != nil {
				v = coerce(v, obj.Type())
			}
			e.define(name.Name, v)
		}
	}
}

// ---------- loop ----------

// enterBlock menyiapkan perekaman untuk blok n (loop, if, switch, fungsi); kembalian
// dipanggil saat blok selesai. Blok fokus menyalakan perekaman; rekursi ke blok fokus
// tidak menambah Run, tetapi tetap tercatat lewat Depth.
func (it *interp) enterBlock(n ast.Node, kind scene.BlockKind) (*frame, func()) {
	r := it.rec
	isFocus := n == r.focus
	if isFocus {
		if r.active == 0 {
			r.runs++
			r.reached = true
			r.dirty = true
		}
		r.active++
	}
	var f *frame
	if it.recording() {
		f = &frame{id: it.p.BlockIDs[n], kind: kind, focus: isFocus}
		r.frames = append(r.frames, f)
	}
	return f, func() {
		if f != nil {
			r.frames = r.frames[:len(r.frames)-1]
		}
		if isFocus {
			r.active--
		}
	}
}

func (it *interp) nextIter(f *frame) {
	if f == nil {
		return
	}
	f.iter++
	if f.focus && f.iter >= it.rec.maxIter {
		it.rec.truncated = true
		it.warn(fmt.Sprintf("trace dibatasi %d putaran pertama", it.rec.maxIter))
		panic(abort{kind: abortDone})
	}
}

func (it *interp) exitStep(n ast.Node, e *env, reason trace.Reason, note string) {
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseExit, Line: it.line(n), Reason: reason, Note: note}, e)
	}
}

// loopFlow menerapkan break/continue/return dari body ke loop berlabel label.
// done=true berarti loop berhenti; out adalah flow yang diteruskan ke luar.
func (it *interp) loopFlow(f flow, label string, n ast.Node, e *env) (done bool, out flow) {
	switch f.k {
	case fBreak:
		if f.label == "" || f.label == label {
			it.exitStep(n, e, trace.ReasonBreak, "keluar dari loop karena break")
			return true, flow{}
		}
		it.exitStep(n, e, trace.ReasonBreak, "keluar dari loop karena break "+f.label)
		return true, f
	case fContinue:
		if f.label == "" || f.label == label {
			return false, flow{}
		}
		it.exitStep(n, e, trace.ReasonBreak, "keluar dari loop karena continue "+f.label)
		return true, f
	case fReturn:
		it.exitStep(n, e, trace.ReasonReturn, "keluar dari loop karena return")
		return true, f
	}
	return false, flow{}
}

func (it *interp) execFor(n *ast.ForStmt, label string, e *env) flow {
	fr, leave := it.enterBlock(n, scene.BlockLoop)
	defer leave()
	scope := newEnv(e, false)
	if n.Init != nil {
		it.exec2(n.Init, scope)
		if it.recording() {
			it.add(trace.Step{Phase: trace.PhaseInit, Line: it.line(n), Note: it.effectNote(n.Init, initLHS(n.Init), scope)}, scope)
		}
	}
	for {
		if n.Cond != nil {
			c := it.cond(n.Cond, scope, "loop")
			if it.recording() {
				r := c
				it.add(trace.Step{Phase: trace.PhaseCond, Line: it.line(n), Result: &r, Note: it.condNote(n.Cond, scope, c, "")}, scope)
			}
			if !c {
				it.exitStep(n, scope, trace.ReasonCondFalse, "kondisi false → keluar dari loop")
				return flow{}
			}
		}
		f := it.exec(n.Body, scope)
		if done, out := it.loopFlow(f, label, n, scope); done {
			return out
		}
		if n.Post != nil {
			it.exec2(n.Post, scope)
			if it.recording() {
				it.add(trace.Step{Phase: trace.PhasePost, Line: it.line(n), Note: it.effectNote(n.Post, initLHS(n.Post), scope)}, scope)
			}
		}
		it.nextIter(fr)
	}
}

// exec2 menjalankan statement tanpa merekamnya sebagai langkah body (untuk post loop).
func (it *interp) exec2(s ast.Stmt, e *env) {
	it.quiet++
	defer func() { it.quiet-- }()
	it.exec(s, e)
}

func initLHS(s ast.Stmt) []ast.Expr {
	switch n := s.(type) {
	case *ast.AssignStmt:
		return n.Lhs
	case *ast.IncDecStmt:
		return []ast.Expr{n.X}
	}
	return nil
}

func (it *interp) execRange(n *ast.RangeStmt, label string, e *env) flow {
	x := it.eval(n.X, e) // dievaluasi sekali, sebelum loop dimulai (seperti di Go)
	fr, leave := it.enterBlock(n, scene.BlockLoop)
	defer leave()
	quietAtStart := it.quiet
	framesAtStart := len(it.rec.frames)
	if x.k == kUnknown {
		it.structural(fmt.Sprintf("range %q di baris %d tidak bisa dihitung: %s", it.text(n.X), it.line(n), x.why))
	}
	scope := newEnv(e, false)
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseInit, Line: it.line(n), Note: fmt.Sprintf("range %s (%s)", it.text(n.X), describe(x))}, scope)
	}
	readsColl := false
	if id, ok := unparen(n.X).(*ast.Ident); ok && id.Name == it.rec.coll {
		readsColl = true
	}

	// iterasi memanggil body untuk satu pasangan key/value; mengembalikan false bila loop harus berhenti.
	var result flow
	stopped := false
	iterate := func(k, v value, idx int) bool {
		it.tick()
		iterScope := newEnv(scope, false) // Go 1.22: variabel loop baru setiap putaran
		it.bindRange(n, iterScope, k, v)
		if it.recording() {
			if readsColl {
				it.rec.op(trace.Op{Op: trace.OpRead, A: idx})
			}
			r := true
			it.add(trace.Step{Phase: trace.PhaseCond, Line: it.line(n), Result: &r, Note: it.rangeNote(n, iterScope, idx, x.k == kString)}, iterScope)
		}
		f := it.exec(n.Body, iterScope)
		if done, out := it.loopFlow(f, label, n, iterScope); done {
			result, stopped = out, true
			return false
		}
		it.nextIter(fr)
		return true
	}

	switch x.k {
	case kInt:
		for i := int64(0); i < x.i; i++ {
			if !iterate(intV(i), value{}, int(i)) {
				return result
			}
		}
	case kSlice, kArray:
		var elems func(i int) value
		count := 0
		if x.k == kSlice {
			count = x.sl.elemsLen()
			elems = func(i int) value { return *x.sl.at(i) } // dibaca langsung: perubahan saat loop terlihat, seperti di Go
		} else {
			arr := copyVal(x).arr // range atas array memakai salinan
			count = len(arr)
			elems = func(i int) value { return arr[i] }
		}
		for i := 0; i < count; i++ {
			if !iterate(intV(int64(i)), elems(i), i) {
				return result
			}
		}
	case kString:
		i := 0
		for i < len(x.s) {
			r, size := utf8.DecodeRuneInString(x.s[i:])
			if !iterate(intV(int64(i)), intV(int64(r)), i) {
				return result
			}
			i += size
		}
	case kMap:
		if x.m != nil {
			keys := append([]value(nil), x.m.keys...)
			it.rng.Shuffle(len(keys), func(a, b int) { keys[a], keys[b] = keys[b], keys[a] })
			it.warn("urutan range atas map diacak: di Go urutannya tidak dijamin dan bisa berbeda setiap kali dijalankan")
			for i, k := range keys {
				v, ok := x.m.get(k)
				if !ok {
					continue // sudah dihapus selama loop
				}
				if !iterate(k, v, i) {
					return result
				}
			}
		}
	case kChan:
		if x.ch == nil {
			it.structural("range atas channel nil akan menunggu selamanya")
		}
		for i := 0; ; i++ {
			if len(x.ch.buf) == 0 {
				if x.ch.closed {
					break
				}
				it.structural("channel kosong dan belum ditutup: program butuh goroutine lain (belum disimulasikan)")
			}
			v := x.ch.buf[0]
			x.ch.buf = x.ch.buf[1:]
			if !iterate(v, value{}, i) {
				return result
			}
		}
	case kFunc:
		i := 0
		yield := &funcVal{name: "yield", builtin: func(args []value) []value {
			if stopped {
				it.panicf(n, "range function continued iteration after function for loop body returned false")
			}
			var k, v value
			if len(args) > 0 {
				k = args[0]
			}
			if len(args) > 1 {
				v = args[1]
			}
			// Fungsi iterator dijalankan quiet, tapi body loop yang dipanggil lewat yield harus terekam
			// lagi, dengan tumpukan blok seperti saat range dimulai (bukan di dalam fungsi iterator).
			prev, prevFrames := it.quiet, it.rec.frames
			it.quiet = quietAtStart
			if framesAtStart <= len(prevFrames) {
				it.rec.frames = prevFrames[:framesAtStart:framesAtStart]
			}
			ok := iterate(k, v, i)
			it.quiet, it.rec.frames = prev, prevFrames
			i++
			return []value{boolV(ok)}
		}}
		it.callFunc(x.fn, []value{{k: kFunc, fn: yield}}, n)
		if stopped {
			return result
		}
	default:
		it.structural(fmt.Sprintf("range atas nilai ini belum disimulasikan (baris %d)", it.line(n)))
	}
	if it.recording() {
		r := false
		it.add(trace.Step{Phase: trace.PhaseCond, Line: it.line(n), Result: &r, Note: "tidak ada elemen lagi"}, scope)
	}
	it.exitStep(n, scope, trace.ReasonRangeDone, "range selesai → keluar dari loop")
	return flow{}
}

func (it *interp) bindRange(n *ast.RangeStmt, e *env, k, v value) {
	assign := func(x ast.Expr, val value) {
		if x == nil {
			return
		}
		if n.Tok == token.DEFINE {
			if id, ok := x.(*ast.Ident); ok {
				if obj := it.p.Info.Defs[id]; obj != nil {
					val = coerce(val, obj.Type())
				}
				e.define(id.Name, val)
			}
			return
		}
		it.target(x, e, false).set(copyVal(val))
	}
	assign(n.Key, k)
	assign(n.Value, v)
}

func (it *interp) rangeNote(n *ast.RangeStmt, e *env, idx int, overString bool) string {
	var parts []string
	for _, x := range []ast.Expr{n.Key, n.Value} {
		if id, ok := x.(*ast.Ident); ok && id.Name != "_" {
			if cell := e.lookup(id.Name); cell != nil {
				part := id.Name + " = " + show(*cell)
				if overString && x == n.Value && cell.k == kInt {
					part += fmt.Sprintf(" (%q)", rune(cell.i)) // rune juga ditampilkan sebagai huruf
				}
				parts = append(parts, part)
			}
		}
	}
	note := fmt.Sprintf("ambil elemen ke-%d", idx+1)
	if len(parts) > 0 {
		note += ": " + strings.Join(parts, ", ")
	}
	return note
}

// ---------- assignment ----------

type target struct {
	get func() value
	set func(value)
}

func (it *interp) assign(n *ast.AssignStmt, e *env) {
	if n.Tok != token.ASSIGN && n.Tok != token.DEFINE {
		op := assignOp[n.Tok]
		t := it.target(n.Lhs[0], e, false)
		v := it.binary(op, t.get(), it.eval(n.Rhs[0], e), n)
		if tv, ok := it.p.Info.Types[n.Lhs[0]]; ok {
			v = coerce(v, tv.Type)
		}
		t.set(v)
		it.markColl(n.Lhs[0])
		return
	}

	var rhs []value
	if len(n.Rhs) == 1 && len(n.Lhs) > 1 {
		rhs = it.evalMulti(n.Rhs[0], e, len(n.Lhs))
	} else {
		for _, r := range n.Rhs {
			rhs = append(rhs, it.eval(r, e))
		}
	}
	for len(rhs) < len(n.Lhs) {
		rhs = append(rhs, unknown("jumlah nilai tidak cocok"))
	}
	it.detectOps(n, e, rhs)
	targets := make([]target, len(n.Lhs))
	for i, l := range n.Lhs {
		targets[i] = it.target(l, e, n.Tok == token.DEFINE)
	}
	for i, t := range targets {
		v := copyVal(rhs[i])
		if id, ok := n.Lhs[i].(*ast.Ident); ok {
			if obj := it.p.Info.Defs[id]; obj != nil {
				v = coerce(v, obj.Type())
			} else if obj := it.p.Info.Uses[id]; obj != nil {
				v = coerce(v, obj.Type())
			}
		}
		t.set(v)
	}
	for _, l := range n.Lhs {
		it.markColl(l)
	}
}

var assignOp = map[token.Token]token.Token{
	token.ADD_ASSIGN: token.ADD, token.SUB_ASSIGN: token.SUB, token.MUL_ASSIGN: token.MUL,
	token.QUO_ASSIGN: token.QUO, token.REM_ASSIGN: token.REM, token.AND_ASSIGN: token.AND,
	token.OR_ASSIGN: token.OR, token.XOR_ASSIGN: token.XOR, token.SHL_ASSIGN: token.SHL,
	token.SHR_ASSIGN: token.SHR, token.AND_NOT_ASSIGN: token.AND_NOT,
}

// markColl menandai isi koleksi berubah bila x adalah koleksi atau elemennya.
func (it *interp) markColl(x ast.Expr) {
	if it.rec.coll == "" || !it.recording() {
		return
	}
	switch t := unparen(x).(type) {
	case *ast.Ident:
		if t.Name == it.rec.coll {
			it.rec.dirty = true
		}
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok && id.Name == it.rec.coll {
			it.rec.dirty = true
		}
	}
}

// detectOps mengenali operasi koleksi pada assignment: tukar, ubah elemen, append.
func (it *interp) detectOps(n *ast.AssignStmt, e *env, rhs []value) {
	if it.rec.coll == "" || !it.recording() {
		return
	}
	if len(n.Lhs) == 2 && len(n.Rhs) == 2 {
		l0, l1 := it.collIndex(n.Lhs[0], e), it.collIndex(n.Lhs[1], e)
		r0, r1 := it.collIndex(n.Rhs[0], e), it.collIndex(n.Rhs[1], e)
		if l0 >= 0 && l1 >= 0 && r0 == l1 && r1 == l0 && l0 != l1 {
			a, b := min(l0, l1), max(l0, l1)
			it.rec.op(trace.Op{Op: trace.OpSwap, A: a, B: &b})
			return
		}
	}
	for i, l := range n.Lhs {
		if idx := it.collIndex(l, e); idx >= 0 {
			it.rec.op(trace.Op{Op: trace.OpSet, A: idx, Value: format(rhs[i])})
			continue
		}
		id, ok := l.(*ast.Ident)
		if !ok || id.Name != it.rec.coll || i >= len(n.Rhs) {
			continue
		}
		if call, ok := n.Rhs[i].(*ast.CallExpr); ok {
			if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "append" && rhs[i].k == kSlice && rhs[i].sl.elemsLen() > 0 {
				elems := rhs[i].sl.elems()
				it.rec.op(trace.Op{Op: trace.OpAppend, A: len(elems) - 1, Value: format(elems[len(elems)-1])})
			}
		}
	}
}

func (it *interp) target(x ast.Expr, e *env, define bool) target {
	switch t := unparen(x).(type) {
	case *ast.Ident:
		if t.Name == "_" {
			return target{get: func() value { return value{} }, set: func(value) {}}
		}
		var cell *value
		if define {
			if c, ok := e.vars[t.Name]; ok {
				cell = c // := dengan variabel yang sudah ada di cakupan yang sama
			} else {
				cell = e.define(t.Name, value{})
			}
		} else if cell = e.lookup(t.Name); cell == nil {
			cell = it.globals.define(t.Name, unknown(t.Name+" tidak diketahui"))
		}
		return target{get: func() value { return *cell }, set: func(v value) { *cell = v }}
	case *ast.IndexExpr:
		base := it.eval(t.X, e)
		if base.k == kMap {
			key := it.eval(t.Index, e)
			if base.m == nil {
				it.panicf(t, "assignment to entry in nil map")
			}
			zero := zeroOf(it.p.Info.Types[t].Type)
			return target{
				get: func() value {
					if v, ok := base.m.get(key); ok {
						return v
					}
					return zero
				},
				set: func(v value) { base.m.set(key, v) },
			}
		}
		ref := it.ref(t, e)
		return target{get: func() value { return *ref }, set: func(v value) { *ref = v }}
	}
	it.structural(fmt.Sprintf("assignment ke %q belum disimulasikan (baris %d)", it.text(x), it.line(x)))
	return target{}
}

// ref mengembalikan alamat elemen slice/array supaya bisa diubah di tempat.
func (it *interp) ref(x ast.Expr, e *env) *value {
	switch t := unparen(x).(type) {
	case *ast.Ident:
		if cell := e.lookup(t.Name); cell != nil {
			return cell
		}
	case *ast.IndexExpr:
		var base *value
		if _, isIdent := unparen(t.X).(*ast.Ident); isIdent {
			base = it.ref(t.X, e)
		} else {
			v := it.eval(t.X, e)
			if v.k == kArray {
				base = it.ref(t.X, e)
			} else {
				base = &v
			}
		}
		idx := it.index(t, it.eval(t.Index, e), it.lenOf(*base))
		switch base.k {
		case kSlice:
			return base.sl.at(idx)
		case kArray:
			return &base.arr[idx]
		}
	}
	it.structural(fmt.Sprintf("%q belum bisa diubah oleh simulator (baris %d)", it.text(x), it.line(x)))
	return nil
}

func (it *interp) lenOf(v value) int {
	switch v.k {
	case kSlice:
		return v.sl.elemsLen()
	case kArray:
		return len(v.arr)
	case kString:
		return len(v.s)
	}
	return -1
}

func (it *interp) index(at ast.Node, i value, length int) int {
	if i.k != kInt {
		it.structural(fmt.Sprintf("indeks di baris %d tidak bisa dihitung", it.line(at)))
	}
	if length < 0 {
		it.structural(fmt.Sprintf("indeks pada %q belum disimulasikan", it.text(at)))
	}
	if i.i < 0 || i.i >= int64(length) {
		it.panicf(at, "runtime error: index out of range [%d] with length %d", i.i, length)
	}
	return int(i.i)
}

// ---------- program ----------

func (it *interp) initGlobals() {
	for _, d := range it.p.File.Decls {
		switch n := d.(type) {
		case *ast.FuncDecl:
			if n.Recv == nil && n.Name != nil {
				it.funcs[n.Name.Name] = n
			}
		case *ast.GenDecl:
			if n.Tok == token.VAR {
				it.decl(n, it.globals)
			}
		}
	}
}

func (it *interp) isBuiltin(id *ast.Ident) bool {
	_, ok := it.p.Info.Uses[id].(*types.Builtin)
	return ok
}
