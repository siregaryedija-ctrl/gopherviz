package irsim

import (
	"fmt"
	"math/rand"
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/lang"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
	"gopherviz/internal/userinput"
)

// semantics berisi perilaku yang berbeda per bahasa.
type semantics interface {
	binary(it *interp, op string, a, b Value, at ir.Node) Value
	unary(it *interp, op string, v Value, at ir.Node) Value
	truthy(v Value) bool
	caseEqual(it *interp, a, b Value) bool // pencocokan switch/case
	show(it *interp, v Value) string       // tampilan di panel (variabel, catatan)
	str(it *interp, v Value) string        // konversi ke string saat dicetak / digabung
	lit(it *interp, l *ir.Lit) Value
	global(it *interp, name string) (Value, bool)
	member(it *interp, x Value, name string, at ir.Node) (Value, bool)
	iterate(it *interp, x Value, fe *ir.ForEach) ([]kv, bool) // isi for-each (nil, false = tidak bisa diiterasi)
	newError(it *interp, class, msg string) Value
	zero(typ string) Value
	convert(it *interp, v Value, typ string) Value // nilai yang disimpan ke variabel bertipe
	setup(it *interp)                              // menyiapkan kelas/objek bawaan sebelum program jalan
}

type kv struct{ k, v Value }

type interp struct {
	lang      string
	sem       semantics
	res       *lang.Result
	src       string
	globals   *Env
	rec       *recorder
	rng       *rand.Rand
	budget    int
	depth     int
	quiet     int
	ids       int
	kwargs    map[string]Value // argumen bernama Python untuk builtin yang sedang dipanggil
	funcs     map[string][]*Fn // fungsi tingkat atas C/C++ (overload per jumlah argumen)
	classes   map[string]*Class
	macros    map[string]Value // #define C/C++
	classOf   map[*Fn]*Class   // method → kelasnya
	pyGlobals map[*Env]map[string]bool
	pyNonloc  map[*Env]map[string]bool
	phpGlobal map[*Env]map[string]bool
	statics   map[*ir.VarSpec]*Cell // PHP: static $x di fungsi
	handling  *Value                // exception yang sedang ditangani (raise tanpa argumen)
	tasks     []func()              // JS: microtask (Promise.then) dan timer, dijalankan setelah skrip utama
	timers    []timerTask
	input     *userinput.Source // angka pengganti input user (dibuat saat pertama dipakai)
	stdinBuf  string            // sisa baris input untuk getchar
	pendingNL bool              // Enter yang tertinggal setelah membaca angka (cin >>, nextInt)
}

type timerTask struct {
	at  float64
	seq int
	fn  func()
}

const maxDepth = 200

func (it *interp) tick() {
	it.budget--
	if it.budget <= 0 {
		panic(abort{abortBudget, "terlalu banyak langkah komputasi: kemungkinan loop tak berujung atau rekursi terlalu dalam"})
	}
}

func (it *interp) line(n ir.Node) int { return n.Span().Start.Line }

func (it *interp) text(n ir.Node) string {
	if n == nil {
		return ""
	}
	s := n.Span()
	if s.Start.Off < 0 || s.End.Off > len(it.src) || s.Start.Off >= s.End.Off {
		return ""
	}
	return strings.Join(strings.Fields(it.src[s.Start.Off:s.End.Off]), " ")
}

func (it *interp) structural(msg string) { panic(abort{abortStructural, msg}) }

func (it *interp) warn(msg string) { it.rec.warn(msg) }

func (it *interp) recording() bool { return it.rec.on() && it.quiet == 0 }

func (it *interp) insideFocus(n ir.Node) bool {
	f := it.rec.focus
	if f == nil || n == nil {
		return false
	}
	fs, ns := f.Span(), n.Span()
	return fs.Start.Off <= ns.Start.Off && ns.End.Off <= fs.End.Off
}

// throw memulai exception program dengan kelas error bawaan bahasa ini.
func (it *interp) throw(at ir.Node, class, msg string) {
	v := it.sem.newError(it, class, msg)
	panic(thrown{v: v, msg: class + ": " + msg, at: at})
}

// ---------- perekaman ----------

func (it *interp) add(s trace.Step, e *Env) {
	var vars map[string]string
	if e != nil {
		vars = it.snapshot(e)
		it.detectCollKind(e)
	}
	it.rec.add(s, vars, func() ([]string, []string) { return it.collItems(e) })
}

func (it *interp) step(n ir.Node, e *Env, note string) {
	if !it.recording() {
		return
	}
	if note == "" {
		note = strings.TrimSuffix(it.text(n), ";")
	}
	it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(n), Note: note}, e)
}

// snapshot mengambil variabel fungsi yang sedang berjalan (cakupan terdalam menang).
func (it *interp) snapshot(e *Env) map[string]string {
	out := map[string]string{}
	for s := e; s != nil; s = s.parent {
		for i := len(s.order) - 1; i >= 0; i-- {
			name := s.order[i]
			if _, ok := out[name]; ok || len(out) >= 12 {
				continue
			}
			v := s.vars[name].get()
			if v.k == kFunc || v.k == kClass || v.k == kUnknown && v.why == "modul" {
				continue
			}
			out[name] = it.sem.show(it, v)
		}
		if s.fn {
			break
		}
	}
	return out
}

// collItems mengembalikan isi koleksi mode data saat ini.
func (it *interp) collItems(e *Env) (items, keys []string) {
	if it.rec.coll == "" || e == nil {
		return nil, nil
	}
	cell := it.findVar(e, it.rec.coll)
	if cell == nil {
		return nil, nil
	}
	v := cell.get()
	switch v.k {
	case kList:
		items = make([]string, len(v.l.elems))
		for i, x := range v.l.elems {
			items[i] = it.format(x)
		}
		return items, nil
	case kDict:
		items, keys = []string{}, []string{}
		for i, k := range v.d.keys {
			keys = append(keys, it.format(k))
			items = append(items, it.format(v.d.vals[i]))
		}
		if it.rec.collKind == "slice" { // array PHP berindeks 0..n-1
			keys = nil
		}
		return items, keys
	}
	return nil, nil
}

// format: nilai sebagai teks pendek tanpa kutip (untuk elemen koleksi).
func (it *interp) format(v Value) string {
	if v.k == kStr {
		return v.s
	}
	return it.sem.show(it, v)
}

// findVar mencari sel variabel tanpa efek samping (untuk tampilan).
func (it *interp) findVar(e *Env, name string) *Cell {
	if it.lang == "php" {
		return e.local(name)
	}
	return e.lookup(name)
}

// collIndex mengembalikan indeks bila x berbentuk koleksi[i], atau -1.
func (it *interp) collIndex(x ir.Expr, e *Env) int {
	ix, ok := x.(*ir.Index)
	if !ok || it.rec.coll == "" || it.rec.collKind == "map" || ix.I == nil {
		return -1
	}
	if id, ok := ix.X.(*ir.Ident); !ok || id.Name != it.rec.coll {
		return -1
	}
	v := it.evalQuiet(ix.I, e)
	if v.k == kFloat && isIntegral(v.f) {
		return int(v.f)
	}
	if v.k != kInt {
		return -1
	}
	if v.i < 0 { // Python a[-1]
		if c := it.findVar(e, it.rec.coll); c != nil && c.get().k == kList {
			return len(c.get().l.elems) + int(v.i)
		}
	}
	return int(v.i)
}

func (it *interp) mapKey(x ir.Expr, e *Env) (string, bool) {
	ix, ok := x.(*ir.Index)
	if !ok || it.rec.coll == "" || it.rec.collKind != "map" || ix.I == nil {
		return "", false
	}
	if id, ok := ix.X.(*ir.Ident); !ok || id.Name != it.rec.coll {
		return "", false
	}
	return it.format(it.evalQuiet(ix.I, e)), true
}

func (it *interp) evalQuiet(x ir.Expr, e *Env) Value {
	it.quiet++
	defer func() { it.quiet-- }()
	return it.eval(x, e)
}

// subst menulis ulang ekspresi dengan nilainya, mis. "i < n" menjadi "0 < 3".
func (it *interp) subst(x ir.Expr, e *Env) string {
	switch n := x.(type) {
	case *ir.Binary:
		if !isCompareOp(n.Op) && !isLogicOp(n.Op) {
			return it.sem.show(it, it.evalQuiet(n, e))
		}
		return it.subst(n.X, e) + " " + n.Op + " " + it.subst(n.Y, e)
	case *ir.Unary:
		if n.Op == "!" || n.Op == "not" || n.Op == "-" {
			sep := ""
			if n.Op == "not" {
				sep = " "
			}
			return n.Op + sep + it.subst(n.X, e)
		}
	case *ir.Ident:
		if c := it.findVar(e, n.Name); c != nil {
			if v := c.get(); v.k != kFunc && v.k != kClass {
				return it.sem.show(it, v)
			}
		}
		if v, ok := it.implicitField(e, n.Name); ok {
			return it.sem.show(it, v)
		}
	case *ir.Index, *ir.Member:
		v := it.evalQuiet(n, e)
		if v.k != kFunc && v.k != kUnknown {
			return it.sem.show(it, v)
		}
	case *ir.Call:
		v := it.evalQuiet(n, e)
		if v.k != kUnknown && isSimpleCall(n) {
			return it.sem.show(it, v)
		}
	}
	return it.text(x)
}

// isSimpleCall: panggilan yang aman dihitung ulang untuk catatan (len, size, length, ...).
func isSimpleCall(c *ir.Call) bool {
	name := ""
	switch f := c.Fun.(type) {
	case *ir.Ident:
		name = f.Name
	case *ir.Member:
		name = f.Name
	}
	switch name {
	case "len", "count", "strlen", "size", "length", "sizeof", "abs", "min", "max", "isEmpty", "empty":
		return true
	}
	return false
}

func isCompareOp(op string) bool {
	switch op {
	case "<", ">", "<=", ">=", "==", "!=", "===", "!==", "<>", "in", "not in", "is", "is not", "instanceof":
		return true
	}
	return false
}

func isLogicOp(op string) bool {
	switch op {
	case "&&", "||", "and", "or", "??":
		return true
	}
	return false
}

func (it *interp) condNote(x ir.Expr, e *Env, result bool, prefix string) string {
	src, sub := it.text(x), it.subst(x, e)
	r := it.boolText(result)
	if sub == src {
		return fmt.Sprintf("%s%s → %s", prefix, src, r)
	}
	return fmt.Sprintf("%s%s → %s → %s", prefix, src, sub, r)
}

func (it *interp) boolText(b bool) string {
	switch it.lang {
	case "python":
		if b {
			return "True"
		}
		return "False"
	}
	if b {
		return "true"
	}
	return "false"
}

// effectNote menjelaskan hasil assignment, mis. "sum += v → sum = 12".
func (it *interp) effectNote(n ir.Node, targets []ir.Expr, e *Env) string {
	var parts []string
	for _, t := range targets {
		switch x := t.(type) {
		case *ir.Ident:
			if x.Name == "_" {
				continue
			}
			if c := it.findVar(e, x.Name); c != nil {
				parts = append(parts, x.Name+" = "+it.sem.show(it, c.get()))
			} else if v, ok := it.implicitField(e, x.Name); ok {
				parts = append(parts, x.Name+" = "+it.sem.show(it, v))
			}
		case *ir.Index, *ir.Member:
			v := it.evalQuiet(x, e)
			if v.k != kUnknown {
				parts = append(parts, it.text(x)+" = "+it.sem.show(it, v))
			}
		}
	}
	text := strings.TrimSuffix(strings.TrimSpace(it.text(n)), ";")
	if len(parts) == 0 {
		return text
	}
	return text + " → " + strings.Join(parts, ", ")
}

// assignTargets mengembalikan target yang diubah oleh sebuah ekspresi/statement.
func assignTargets(x ir.Node) []ir.Expr {
	switch n := x.(type) {
	case *ir.ExprStmt:
		return assignTargets(n.X)
	case *ir.VarDecl:
		var out []ir.Expr
		for _, s := range n.Specs {
			if s.Name != "" {
				out = append(out, &ir.Ident{S: s.S, Name: s.Name})
			} else if s.Pattern != nil {
				out = append(out, patternIdents(s.Pattern)...)
			}
		}
		return out
	case *ir.Assign:
		if n.Op == "=" || n.Op == ":=" {
			if inner := assignTargets(n.R); len(inner) > 0 {
				return append(patternIdents(n.L), inner...)
			}
		}
		return patternIdents(n.L)
	case *ir.Unary:
		if n.Op == "++" || n.Op == "--" {
			return []ir.Expr{n.X}
		}
	case *ir.Seq:
		var out []ir.Expr
		for _, y := range n.List {
			out = append(out, assignTargets(y)...)
		}
		return out
	}
	return nil
}

func patternIdents(x ir.Expr) []ir.Expr {
	switch n := x.(type) {
	case *ir.ArrayLit:
		var out []ir.Expr
		for _, el := range n.Elems {
			out = append(out, patternIdents(el)...)
		}
		return out
	case *ir.ObjectLit:
		var out []ir.Expr
		for _, p := range n.Props {
			out = append(out, patternIdents(p.Value)...)
		}
		return out
	case *ir.Spread:
		return patternIdents(n.X)
	case *ir.Assign:
		return patternIdents(n.L)
	case *ir.Ident, *ir.Index, *ir.Member:
		return []ir.Expr{x}
	}
	return nil
}

// ---------- blok & fokus ----------

func (it *interp) enterBlock(n ir.Node, kind scene.BlockKind) (*frame, func()) {
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
		f = &frame{id: it.res.IDs[n], kind: kind, focus: isFocus}
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

func (it *interp) exitStep(n ir.Node, e *Env, reason trace.Reason, note string) {
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseExit, Line: it.line(n), Reason: reason, Note: note}, e)
	}
}

// ---------- statement ----------

func (it *interp) execList(list []ir.Stmt, e *Env) flow {
	for _, s := range list {
		if f := it.exec(s, e); f.k != fNone {
			return f
		}
	}
	return flow{}
}

// blockScope: Python dan PHP tidak punya cakupan blok.
func (it *interp) blockScope(e *Env) *Env {
	if it.lang == "python" || it.lang == "php" {
		return e
	}
	return newEnv(e, false)
}

func (it *interp) execBody(s ir.Stmt, e *Env) flow {
	if b, ok := s.(*ir.Block); ok {
		return it.execList(b.List, it.blockScope(e))
	}
	if s == nil {
		return flow{}
	}
	return it.exec(s, e)
}

func (it *interp) exec(s ir.Stmt, e *Env) flow {
	it.tick()
	switch n := s.(type) {
	case *ir.Block:
		if !n.Closed && it.lang != "python" {
			it.structural(fmt.Sprintf("blok di baris %d belum ditutup", it.line(n)))
		}
		return it.execList(n.List, it.blockScope(e))
	case *ir.ExprStmt:
		it.execExpr(n, e)
	case *ir.VarDecl:
		it.varDecl(n, e)
		if it.recording() {
			it.step(n, e, it.effectNote(n, assignTargets(n), e))
		}
	case *ir.If:
		return it.execIf(n, e)
	case *ir.For:
		return it.execFor(n, "", e)
	case *ir.ForEach:
		return it.execForEach(n, "", e)
	case *ir.While:
		return it.execWhile(n, "", e)
	case *ir.Labeled:
		switch l := n.Stmt.(type) {
		case *ir.For:
			return it.execFor(l, n.Label, e)
		case *ir.ForEach:
			return it.execForEach(l, n.Label, e)
		case *ir.While:
			return it.execWhile(l, n.Label, e)
		}
		f := it.exec(n.Stmt, e)
		if f.k == fBreak && f.label == n.Label {
			return flow{}
		}
		return f
	case *ir.Switch:
		return it.execSwitch(n, e)
	case *ir.Branch:
		switch n.Tok {
		case "break":
			if it.recording() {
				it.add(trace.Step{Phase: trace.PhaseBreak, Line: it.line(n), Note: strings.TrimSpace("break "+n.Label) + " → keluar dari loop"}, e)
			}
			return flow{k: fBreak, label: n.Label, count: n.Count}
		case "continue":
			if it.recording() {
				it.add(trace.Step{Phase: trace.PhaseContinue, Line: it.line(n), Note: strings.TrimSpace("continue "+n.Label) + " → langsung ke putaran berikutnya"}, e)
			}
			return flow{k: fContinue, label: n.Label, count: n.Count}
		default:
			it.structural("goto belum disimulasikan")
		}
	case *ir.Return:
		v := undef()
		if it.lang != "javascript" && it.lang != "typescript" {
			v = null()
		}
		if n.X != nil {
			v = it.eval(n.X, e)
		}
		it.step(n, e, strings.TrimSuffix(it.text(n), ";")+" → keluar dari fungsi")
		return flow{k: fReturn, ret: v}
	case *ir.Throw:
		var v Value
		if n.X != nil {
			v = it.eval(n.X, e)
		} else if cur := it.currentException(e); cur != nil {
			v = *cur // raise tanpa argumen: lempar ulang
		} else {
			it.throw(n, "RuntimeError", "No active exception to reraise")
		}
		if v.k == kClass { // raise ValueError
			v = it.construct(v.c, nil, n)
		}
		msg := it.exceptionText(v)
		it.step(n, e, it.text(n)+" → lempar "+msg)
		panic(thrown{v: v, msg: msg, at: n})
	case *ir.Try:
		return it.execTry(n, e)
	case *ir.Func:
		if n.Name != "" && !it.hoisted(n) {
			fn := it.closure(n, e)
			it.declareFunc(e, n.Name, fn)
		}
		it.step(n, e, "fungsi "+n.Name+" didefinisikan")
	case *ir.Class:
		if c := it.defineClass(n, e); c != nil && n.Name != "" {
			e.define(n.Name, Value{k: kClass, c: c})
		}
	case *ir.Simple:
		return it.simple(n, e)
	case *ir.Bad:
		it.structural(fmt.Sprintf("baris %d belum bisa dibaca: %s", it.line(n), n.Msg))
	default:
		it.structural(fmt.Sprintf("statement di baris %d belum disimulasikan", it.line(s)))
	}
	return flow{}
}

// hoisted: deklarasi fungsi JS/C/C++/PHP sudah didefinisikan sebelum program berjalan.
func (it *interp) hoisted(f *ir.Func) bool {
	switch it.lang {
	case "javascript", "typescript", "php", "c", "cpp":
		return true
	}
	return false
}

func (it *interp) declareFunc(e *Env, name string, fn *Func) {
	e.define(name, Value{k: kFunc, fn: fn})
}

func (it *interp) execExpr(n *ir.ExprStmt, e *Env) {
	x := it.eval(n.X, e)
	if !it.recording() {
		return
	}
	note := ""
	if out := it.rec.out.String(); out != "" {
		note = strings.TrimSuffix(it.text(n), ";") + " → cetak " + fmt.Sprintf("%q", strings.TrimSuffix(out, "\n"))
	} else if targets := assignTargets(n.X); len(targets) > 0 {
		note = it.effectNote(n, targets, e)
	}
	_ = x
	it.step(n, e, note)
}

func (it *interp) execIf(n *ir.If, e *Env) flow {
	fr, leave := it.enterBlock(n, scene.BlockIf)
	defer leave()
	focus := fr != nil && fr.focus
	branch := 0
	for cur := n; ; {
		c := it.cond(cur.Cond, e, "if")
		prefix := it.ifPrefix(cur == n)
		if it.recording() {
			r := c
			it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(cur), Result: &r, Note: it.condNote(cur.Cond, e, c, prefix)}, e)
		}
		if c {
			it.branchStep(focus, n, branch, e, fmt.Sprintf("masuk cabang ke-%d (%s)", branch+1, strings.TrimSpace(prefix+it.text(cur.Cond))))
			return it.blockExit(focus, n, it.execBody(cur.Then, e), e)
		}
		branch++
		switch el := cur.Else.(type) {
		case *ir.If:
			cur = el
			continue
		case nil:
			it.branchStep(focus, n, branch, e, "semua kondisi false dan tidak ada else → lewati")
			return it.blockExit(focus, n, flow{}, e)
		default:
			it.branchStep(focus, n, branch, e, "semua kondisi false → masuk cabang else")
			return it.blockExit(focus, n, it.execBody(el, e), e)
		}
	}
}

func (it *interp) ifPrefix(first bool) string {
	if first {
		return "if "
	}
	switch it.lang {
	case "python":
		return "elif "
	case "php":
		return "elseif "
	}
	return "else if "
}

func (it *interp) branchStep(focus bool, n ir.Node, idx int, e *Env, note string) {
	if !focus || !it.recording() {
		return
	}
	b := idx
	it.add(trace.Step{Phase: trace.PhaseBranch, Line: it.line(n), Branch: &b, Note: note}, e)
}

func (it *interp) blockExit(focus bool, n ir.Node, f flow, e *Env) flow {
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
	it.add(trace.Step{Phase: trace.PhaseExit, Line: n.Span().End.Line, Reason: reason, Note: note}, e)
	return f
}

// cond mengevaluasi kondisi sesuai aturan truthiness bahasa ini.
func (it *interp) cond(x ir.Expr, e *Env, what string) bool {
	if x == nil {
		return true
	}
	if _, bad := x.(*ir.BadExpr); bad {
		it.structural(fmt.Sprintf("kondisi %s di baris %d belum lengkap", what, it.line(x)))
	}
	v := it.eval(x, e)
	if v.k == kUnknown {
		why := v.why
		if why == "" {
			why = "nilainya tidak diketahui"
		}
		it.structural(fmt.Sprintf("kondisi %s %q di baris %d tidak bisa dihitung: %s", what, it.text(x), it.line(x), why))
	}
	return it.sem.truthy(v)
}

func (it *interp) loopFlow(f flow, label string, n ir.Node, e *Env) (done bool, out flow) {
	switch f.k {
	case fBreak:
		if f.count > 1 { // PHP: break 2
			it.exitStep(n, e, trace.ReasonBreak, fmt.Sprintf("keluar dari loop karena break %d", f.count))
			f.count--
			return true, f
		}
		if f.label == "" || f.label == label {
			it.exitStep(n, e, trace.ReasonBreak, "keluar dari loop karena break")
			return true, flow{}
		}
		it.exitStep(n, e, trace.ReasonBreak, "keluar dari loop karena break "+f.label)
		return true, f
	case fContinue:
		if f.count > 1 {
			it.exitStep(n, e, trace.ReasonBreak, fmt.Sprintf("keluar dari loop karena continue %d", f.count))
			f.count--
			return true, f
		}
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

func (it *interp) execFor(n *ir.For, label string, e *Env) flow {
	fr, leave := it.enterBlock(n, scene.BlockLoop)
	defer leave()
	scope := newEnv(e, false)
	if it.lang == "python" || it.lang == "php" {
		scope = e
	}
	if n.Init != nil {
		it.quietExec(n.Init, scope)
		if it.recording() {
			it.add(trace.Step{Phase: trace.PhaseInit, Line: it.line(n), Note: it.effectNote(n.Init, assignTargets(n.Init), scope)}, scope)
		}
	}
	for {
		iterScope := scope
		if n.Cond != nil {
			c := it.cond(n.Cond, iterScope, "loop")
			if it.recording() {
				r := c
				it.add(trace.Step{Phase: trace.PhaseCond, Line: it.line(n), Result: &r, Note: it.condNote(n.Cond, iterScope, c, "")}, iterScope)
			}
			if !c {
				it.exitStep(n, iterScope, trace.ReasonCondFalse, "kondisi false → keluar dari loop")
				return flow{}
			}
		}
		f := it.execBody(n.Body, iterScope)
		if done, out := it.loopFlow(f, label, n, iterScope); done {
			return out
		}
		if n.Post != nil {
			it.quiet++
			it.eval(n.Post, iterScope)
			it.quiet--
			if it.recording() {
				it.add(trace.Step{Phase: trace.PhasePost, Line: it.line(n), Note: it.effectNote(n.Post, assignTargets(n.Post), iterScope)}, iterScope)
			}
		}
		it.nextIter(fr)
	}
}

func (it *interp) quietExec(s ir.Stmt, e *Env) {
	it.quiet++
	defer func() { it.quiet-- }()
	it.exec(s, e)
}

func (it *interp) execWhile(n *ir.While, label string, e *Env) flow {
	fr, leave := it.enterBlock(n, scene.BlockLoop)
	defer leave()
	if n.Do {
		for {
			f := it.execBody(n.Body, e)
			if done, out := it.loopFlow(f, label, n, e); done {
				return out
			}
			c := it.cond(n.Cond, e, "loop")
			if it.recording() {
				r := c
				it.add(trace.Step{Phase: trace.PhaseCond, Line: n.Span().End.Line, Result: &r, Note: it.condNote(n.Cond, e, c, "while ")}, e)
			}
			if !c {
				it.exitStep(n, e, trace.ReasonCondFalse, "kondisi false → keluar dari loop")
				return flow{}
			}
			it.nextIter(fr)
		}
	}
	for {
		c := it.cond(n.Cond, e, "loop")
		if it.recording() {
			r := c
			it.add(trace.Step{Phase: trace.PhaseCond, Line: it.line(n), Result: &r, Note: it.condNote(n.Cond, e, c, "")}, e)
		}
		if !c {
			it.exitStep(n, e, trace.ReasonCondFalse, "kondisi false → keluar dari loop")
			if n.Else != nil { // Python while-else: jalan bila loop selesai tanpa break
				return it.execBody(n.Else, e)
			}
			return flow{}
		}
		f := it.execBody(n.Body, e)
		if done, out := it.loopFlow(f, label, n, e); done {
			return out
		}
		it.nextIter(fr)
	}
}

func (it *interp) execForEach(n *ir.ForEach, label string, e *Env) flow {
	x := it.eval(n.X, e)
	if it.lang == "php" && x.k == kDict && !strings.Contains(n.Decl, "&") {
		x = copyFor(it, x) // foreach PHP berjalan atas salinan array
	}
	fr, leave := it.enterBlock(n, scene.BlockLoop)
	defer leave()
	if x.k == kUnknown {
		it.structural(fmt.Sprintf("koleksi %q di baris %d tidak bisa dihitung: %s", it.text(n.X), it.line(n), x.why))
	}
	scope := e
	if it.lang != "python" && it.lang != "php" {
		scope = newEnv(e, false)
	}
	pairs, ok := it.sem.iterate(it, x, n)
	if !ok {
		it.throw(n, "TypeError", it.sem.show(it, x)+" tidak bisa diiterasi")
	}
	if it.recording() {
		it.add(trace.Step{Phase: trace.PhaseInit, Line: it.line(n), Note: fmt.Sprintf("%s %s (%s)", it.eachWord(n), it.text(n.X), it.describe(x, len(pairs)))}, scope)
	}
	readsColl := false
	if id, ok := n.X.(*ir.Ident); ok && id.Name == it.rec.coll {
		readsColl = true
	}
	for i, p := range pairs {
		it.tick()
		iterScope := scope
		if it.lang != "python" && it.lang != "php" {
			iterScope = newEnv(scope, false) // let/const: variabel baru setiap putaran
		}
		it.bindEach(n, iterScope, p, x, i)
		if it.recording() {
			if readsColl && it.rec.collKind == "map" {
				it.rec.op(trace.Op{Op: trace.OpRead, A: -1, Key: it.format(p.k)})
			} else if readsColl {
				it.rec.op(trace.Op{Op: trace.OpRead, A: i})
			}
			r := true
			it.add(trace.Step{Phase: trace.PhaseCond, Line: it.line(n), Result: &r, Note: it.eachNote(n, iterScope, i)}, iterScope)
		}
		f := it.execBody(n.Body, iterScope)
		if done, out := it.loopFlow(f, label, n, iterScope); done {
			return out
		}
		it.nextIter(fr)
		if x.k == kList && it.lang == "python" && i == len(pairs)-1 && len(x.l.elems) > len(pairs) {
			// Python: list yang bertambah selama loop ikut diiterasi
			for _, el := range x.l.elems[len(pairs):] {
				pairs = append(pairs, kv{k: intV(int64(len(pairs))), v: el})
			}
		}
	}
	if it.recording() {
		r := false
		it.add(trace.Step{Phase: trace.PhaseCond, Line: it.line(n), Result: &r, Note: "tidak ada elemen lagi"}, scope)
	}
	it.exitStep(n, scope, trace.ReasonRangeDone, "semua elemen sudah dikunjungi → keluar dari loop")
	if n.Else != nil {
		return it.execBody(n.Else, scope)
	}
	return flow{}
}

func (it *interp) eachWord(n *ir.ForEach) string {
	switch n.Kind {
	case "of":
		return "for...of"
	case "in":
		if it.lang == "python" {
			return "for ... in"
		}
		return "for...in"
	case "py":
		return "for ... in"
	case "foreach":
		return "foreach"
	}
	return "for-each"
}

func (it *interp) describe(x Value, n int) string {
	switch x.k {
	case kStr:
		return fmt.Sprintf("%d karakter", n)
	case kDict:
		if x.d.kind == "object" || x.d.kind == "dict" {
			return fmt.Sprintf("%d key", n)
		}
		return fmt.Sprintf("%d pasangan", n)
	}
	return fmt.Sprintf("%d elemen", n)
}

// bindEach mengisi variabel for-each untuk satu putaran.
func (it *interp) bindEach(n *ir.ForEach, e *Env, p kv, coll Value, i int) {
	if n.Key != nil {
		it.bindTarget(n.Key, e, p.k, true)
	}
	v := p.v
	if n.Kind == "in" && (it.lang == "javascript" || it.lang == "typescript") {
		v = p.k // for...in: key
	}
	if strings.Contains(n.Decl, "&") && (coll.k == kList || coll.k == kDict) { // C++ auto& / PHP &$v
		if id, ok := n.Value.(*ir.Ident); ok {
			var c *Cell
			if it.lang == "php" {
				c = &Cell{}
				fe := e.fnEnv()
				if fe.vars[id.Name] == nil {
					fe.order = append(fe.order, id.Name)
				}
				fe.vars[id.Name] = c
			} else {
				c = e.define(id.Name, Value{})
			}
			if coll.k == kDict {
				c.alias = &Ptr{dict: coll.d, key: p.k}
			} else {
				c.alias = &Ptr{list: coll.l, idx: i}
			}
			return
		}
	}
	if n.Decl != "" && it.isTyped() {
		v = it.sem.convert(it, v, strings.TrimSpace(strings.TrimRight(n.Decl, "&")))
	}
	it.bindTarget(n.Value, e, copyFor(it, v), true)
}

func (it *interp) isTyped() bool { return it.lang == "c" || it.lang == "cpp" || it.lang == "java" }

func (it *interp) eachNote(n *ir.ForEach, e *Env, idx int) string {
	var parts []string
	for _, t := range append(patternIdents(n.Key), patternIdents(n.Value)...) {
		if id, ok := t.(*ir.Ident); ok && id.Name != "_" {
			if c := it.findVar(e, id.Name); c != nil {
				parts = append(parts, id.Name+" = "+it.sem.show(it, c.get()))
			}
		}
	}
	note := fmt.Sprintf("ambil elemen ke-%d", idx+1)
	if len(parts) > 0 {
		note += ": " + strings.Join(parts, ", ")
	}
	return note
}

func (it *interp) execSwitch(n *ir.Switch, e *Env) flow {
	fr, leave := it.enterBlock(n, scene.BlockSwitch)
	defer leave()
	focus := fr != nil && fr.focus
	scope := it.blockScope(e)
	if n.Match && it.lang == "python" {
		return it.pyMatch(n, scope, focus)
	}
	tag := it.eval(n.Tag, scope)
	if tag.k == kUnknown {
		it.structural(fmt.Sprintf("nilai switch di baris %d tidak bisa dihitung: %s", it.line(n), tag.why))
	}
	if focus && it.recording() {
		it.add(trace.Step{Phase: trace.PhaseInit, Line: it.line(n), Note: fmt.Sprintf("switch %s → %s", it.text(n.Tag), it.sem.show(it, tag))}, scope)
	}
	start, def := -1, -1
	for i, c := range n.Cases {
		if c.Exprs == nil {
			def = i
			continue
		}
		for _, x := range c.Exprs {
			var v Value
			if id, ok := x.(*ir.Ident); ok && it.lang == "java" && tag.k == kObj && tag.o.class.kind == "enum" {
				v, _ = tag.o.class.statics.get(strV(id.Name)) // case MERAH: konstanta enum tanpa nama kelas
			} else {
				v = it.eval(x, scope)
			}
			eq := it.sem.caseEqual(it, tag, v)
			if focus && it.recording() {
				r := eq
				it.add(trace.Step{Phase: trace.PhaseCond, Line: it.line(c), Result: &r,
					Note: fmt.Sprintf("case %s → %s == %s → %s", it.text(x), it.sem.show(it, tag), it.sem.show(it, v), it.boolText(eq))}, scope)
			}
			if eq {
				start = i
				break
			}
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		start = def
	}
	if start < 0 {
		it.branchStep(focus, n, len(n.Cases), scope, "tidak ada case yang cocok → lewati")
		return it.blockExit(focus, n, flow{}, scope)
	}
	c := n.Cases[start]
	if focus {
		label := "default"
		if c.Exprs != nil {
			label = "case " + it.textList(c.Exprs)
		}
		it.branchStep(focus, n, start, scope, "masuk "+label)
	} else {
		it.step(c, scope, "case "+it.textList(c.Exprs))
	}
	// C/JS/Java/PHP: tanpa break, eksekusi jatuh ke case berikutnya (fallthrough)
	var f flow
	for i := start; i < len(n.Cases); i++ {
		f = it.execList(n.Cases[i].Body, scope)
		if f.k != fNone || n.Cases[i].Arrow || n.Match {
			break
		}
		if i+1 < len(n.Cases) && len(n.Cases[i].Body) > 0 && it.recording() {
			it.step(n.Cases[i+1], scope, "tanpa break → lanjut ke "+caseLabel(it, n.Cases[i+1]))
		}
	}
	if f.k == fBreak && f.label == "" {
		if f.count > 1 {
			f.count--
		} else {
			f = flow{} // break di dalam switch hanya keluar dari switch
		}
	}
	return it.blockExit(focus, n, f, scope)
}

func caseLabel(it *interp, c *ir.Case) string {
	if c.Exprs == nil {
		return "default"
	}
	return "case " + it.textList(c.Exprs)
}

func (it *interp) textList(xs []ir.Expr) string {
	if len(xs) == 0 {
		return "default"
	}
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = it.text(x)
	}
	return strings.Join(parts, ", ")
}

// execTry menjalankan try/catch/finally.
func (it *interp) execTry(n *ir.Try, e *Env) (out flow) {
	runFinally := func() {
		if n.Finally != nil {
			if it.recording() {
				it.step(n.Finally, e, "finally dijalankan")
			}
			if f := it.execList(n.Finally.List, it.blockScope(e)); f.k != fNone {
				out = f
			}
		}
	}
	var caught *thrown
	func() {
		defer func() {
			if r := recover(); r != nil {
				t, ok := r.(thrown)
				if !ok {
					panic(r)
				}
				caught = &t
			}
		}()
		out = it.execList(n.Body.List, it.blockScope(e))
	}()
	if caught == nil {
		if n.Else != nil {
			out = it.execList(n.Else.List, it.blockScope(e))
		}
		runFinally()
		return out
	}
	for _, c := range n.Catches {
		if !it.catchMatches(c, caught.v, e) {
			continue
		}
		scope := it.blockScope(e)
		if c.Name != "" {
			scope.define(c.Name, caught.v)
		}
		if it.recording() {
			it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(c), Note: "exception ditangkap: " + caught.msg}, scope)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					runFinally()
					panic(r)
				}
			}()
			prev := it.handling
			it.handling = &caught.v
			defer func() { it.handling = prev }()
			out = it.execList(c.Body.List, scope)
		}()
		runFinally()
		return out
	}
	runFinally()
	panic(*caught)
}

func (it *interp) currentException(e *Env) *Value { return it.handling }

// catchMatches: tanpa tipe = tangkap semua; selain itu cocokkan kelas exception.
func (it *interp) catchMatches(c *ir.Catch, v Value, e *Env) bool {
	if v.k == kObj && v.o.class.native == "crash" {
		return false // crash C/C++ (mis. bagi nol) tidak bisa ditangkap
	}
	if len(c.Types) == 0 {
		return true
	}
	for _, t := range c.Types {
		t = strings.TrimSpace(t)
		t = strings.TrimPrefix(strings.TrimPrefix(t, "std::"), "\\")
		t = strings.TrimSuffix(strings.TrimPrefix(t, "const "), "&")
		t = strings.TrimSpace(t)
		if t == "..." || t == "Exception" && it.lang == "python" || t == "BaseException" || t == "Throwable" {
			return true
		}
		if v.k == kObj && instanceOf(v.o.class, t) {
			return true
		}
		if v.k != kObj && (t == "Exception" || t == "exception") { // throw 42 di C++ / nilai biasa
			return it.lang != "cpp"
		}
		if v.k == kInt && (t == "int") || v.k == kStr && (t == "string" || t == "char*" || t == "char *" || t == "const char*") {
			return true
		}
	}
	return false
}

func instanceOf(c *Class, name string) bool {
	for ; c != nil; c = c.super {
		if c.name == name {
			return true
		}
		for _, i := range c.ifaces {
			if instanceOf(i, name) {
				return true
			}
		}
	}
	return false
}

func (it *interp) exceptionText(v Value) string {
	if v.k == kObj {
		msg := ""
		if m, ok := v.o.fields.get(strV("message")); ok {
			msg = it.sem.str(it, m)
		} else if m, ok := v.o.fields.get(strV("args")); ok && m.k == kList && len(m.l.elems) > 0 {
			msg = it.sem.str(it, m.l.elems[0])
		}
		if msg == "" {
			return v.o.class.name
		}
		return v.o.class.name + ": " + msg
	}
	return it.sem.show(it, v)
}

// simple menjalankan statement sederhana (import, pass, global, del, ...).
func (it *interp) simple(n *ir.Simple, e *Env) flow {
	switch n.Kind {
	case "import":
		if it.lang == "python" {
			it.pyImport(n, e)
		}
	case "empty", "pass", "type", "prototype":
		if n.Kind == "pass" {
			it.step(n, e, "pass (tidak melakukan apa-apa)")
		}
	case "directive":
		it.directive(n, e)
	case "global":
		fe := e.fnEnv()
		for _, name := range n.Names {
			if it.lang == "php" {
				c := it.globals.vars[name]
				if c == nil {
					c = it.globals.define(name, null())
				}
				fe.vars[name] = c
				fe.order = append(fe.order, name)
				continue
			}
			if it.pyGlobals[fe] == nil {
				it.pyGlobals[fe] = map[string]bool{}
			}
			it.pyGlobals[fe][name] = true
		}
	case "nonlocal":
		fe := e.fnEnv()
		if it.pyNonloc[fe] == nil {
			it.pyNonloc[fe] = map[string]bool{}
		}
		for _, name := range n.Names {
			it.pyNonloc[fe][name] = true
		}
	case "del":
		for _, x := range n.X {
			it.deleteTarget(x, e)
		}
		it.step(n, e, "")
	case "assert":
		if len(n.X) > 0 && !it.cond(n.X[0], e, "assert") {
			msg := ""
			if len(n.X) > 1 {
				msg = it.sem.str(it, it.eval(n.X[1], e))
			}
			it.throw(n, "AssertionError", msg)
		}
	case "with":
		if len(n.X) > 0 {
			v := it.eval(n.X[0], e)
			if len(n.X) > 1 {
				it.bindTarget(n.X[1], e, v, false)
			}
		}
	default:
		it.step(n, e, "")
	}
	return flow{}
}

// directive menangani #define NAMA nilai di C/C++ (nilai sederhana).
func (it *interp) directive(n *ir.Simple, e *Env) {
	if len(n.Names) == 0 {
		return
	}
	line := strings.TrimSpace(strings.TrimPrefix(n.Names[0], "#"))
	if !strings.HasPrefix(line, "define") {
		return
	}
	fields := strings.Fields(strings.TrimPrefix(line, "define"))
	if len(fields) < 2 || strings.Contains(fields[0], "(") {
		return
	}
	expr := strings.Join(fields[1:], " ")
	f := lang.Parse(it.lang, "int __m = "+expr+";")
	if len(f.Body) == 1 {
		if d, ok := f.Body[0].(*ir.VarDecl); ok && len(d.Specs) == 1 && d.Specs[0].Init != nil {
			saved := it.src
			it.src = "int __m = " + expr + ";"
			v := it.evalQuiet(d.Specs[0].Init, e)
			it.src = saved
			it.macros[fields[0]] = v
		}
	}
}
