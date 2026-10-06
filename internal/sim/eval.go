package sim

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"math"
	"strconv"
	"strings"
	"unicode"

	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

func (it *interp) eval(x ast.Expr, e *env) value {
	it.tick()
	tv, hasType := it.p.Info.Types[x]
	if hasType && tv.Value != nil {
		return constValue(tv)
	}
	v := it.eval0(x, e)
	if hasType && tv.Type != nil && v.k != kTuple {
		v = coerce(v, tv.Type)
	}
	return v
}

// evalMulti mengevaluasi ekspresi yang bisa menghasilkan beberapa nilai:
// pemanggilan fungsi, v, ok := m[k], v, ok := <-ch. want = -1 berarti apa adanya.
func (it *interp) evalMulti(x ast.Expr, e *env, want int) []value {
	switch n := unparen(x).(type) {
	case *ast.CallExpr:
		if want != 1 {
			return it.call(n, e)
		}
	case *ast.IndexExpr:
		if want == 2 {
			base := it.eval(n.X, e)
			if base.k == kMap {
				v, ok := base.m.get(it.eval(n.Index, e))
				if !ok {
					v = zeroOf(it.p.Info.Types[n].Type)
				}
				return []value{v, boolV(ok)}
			}
		}
	case *ast.UnaryExpr:
		if want == 2 && n.Op == token.ARROW {
			v, ok := it.recv(n, e)
			return []value{v, boolV(ok)}
		}
	}
	return []value{it.eval(x, e)}
}

func (it *interp) eval0(x ast.Expr, e *env) value {
	switch n := x.(type) {
	case *ast.Ident:
		if n.Name == "nil" {
			return value{k: kNil}
		}
		if cell := e.lookup(n.Name); cell != nil {
			return *cell
		}
		if fd, ok := it.funcs[n.Name]; ok {
			return value{k: kFunc, fn: &funcVal{name: n.Name, decl: fd}}
		}
		return unknown(fmt.Sprintf("nilai %s tidak diketahui", n.Name))
	case *ast.BasicLit:
		return unknown("literal tidak terbaca")
	case *ast.ParenExpr:
		return it.eval(n.X, e)
	case *ast.FuncLit:
		return value{k: kFunc, fn: &funcVal{name: "func", lit: n, env: e}}
	case *ast.CompositeLit:
		return it.composite(n, e)
	case *ast.BinaryExpr:
		return it.evalBinary(n, e)
	case *ast.UnaryExpr:
		return it.unary(n, e)
	case *ast.IndexExpr:
		return it.evalIndex(n, e)
	case *ast.SliceExpr:
		return it.evalSlice(n, e)
	case *ast.CallExpr:
		res := it.call(n, e)
		switch len(res) {
		case 0:
			return value{}
		case 1:
			return res[0]
		}
		return value{k: kTuple, tup: res}
	case *ast.SelectorExpr:
		return unknown(fmt.Sprintf("%s belum disimulasikan", it.text(n)))
	}
	return unknown(fmt.Sprintf("ekspresi %q belum disimulasikan", it.text(x)))
}

func (it *interp) composite(n *ast.CompositeLit, e *env) value {
	t := it.p.Info.Types[n].Type
	if t == nil {
		return unknown("tipe literal tidak diketahui")
	}
	switch u := t.Underlying().(type) {
	case *types.Slice, *types.Array:
		var elem types.Type
		length := -1
		if s, ok := u.(*types.Slice); ok {
			elem = s.Elem()
		} else {
			a := u.(*types.Array)
			elem, length = a.Elem(), int(a.Len())
		}
		var elems []value
		idx := 0
		for _, el := range n.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				k := it.eval(kv.Key, e)
				if k.k != kInt {
					return unknown("indeks literal tidak diketahui")
				}
				idx = int(k.i)
				el = kv.Value
			}
			for len(elems) <= idx {
				elems = append(elems, zeroOf(elem))
			}
			elems[idx] = coerce(copyVal(it.eval(el, e)), elem)
			idx++
		}
		if length >= 0 {
			for len(elems) < length {
				elems = append(elems, zeroOf(elem))
			}
			return value{k: kArray, arr: elems}
		}
		return sliceOf(elems)
	case *types.Map:
		m := newMap()
		for _, el := range n.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			m.set(coerce(it.eval(kv.Key, e), u.Key()), coerce(copyVal(it.eval(kv.Value, e)), u.Elem()))
		}
		return value{k: kMap, m: m}
	}
	return unknown(fmt.Sprintf("nilai bertipe %s belum disimulasikan", t))
}

func (it *interp) evalBinary(n *ast.BinaryExpr, e *env) value {
	switch n.Op {
	case token.LAND, token.LOR:
		a := it.eval(n.X, e)
		if a.k != kBool {
			return unknown(a.why)
		}
		if (n.Op == token.LAND && !a.b) || (n.Op == token.LOR && a.b) {
			return a
		}
		return it.eval(n.Y, e)
	}
	a, b := it.eval(n.X, e), it.eval(n.Y, e)
	v := it.binary(n.Op, a, b, n)
	if isCompare(n.Op) && it.recording() && it.rec.coll != "" {
		it.compareOp(n, e, a, b)
	}
	return v
}

func isCompare(op token.Token) bool {
	switch op {
	case token.LSS, token.GTR, token.LEQ, token.GEQ, token.EQL, token.NEQ:
		return true
	}
	return false
}

// compareOp mencatat perbandingan yang melibatkan elemen koleksi.
func (it *interp) compareOp(n *ast.BinaryExpr, e *env, a, b value) {
	ia, ib := it.collIndex(n.X, e), it.collIndex(n.Y, e)
	if ia < 0 && ib < 0 {
		return
	}
	rel := relation(a, b)
	if ia < 0 { // elemen koleksi hanya di kanan: balik supaya A selalu elemen koleksi
		ia, ib = ib, -1
		rel = map[string]string{">": "<", "<": ">", "=": "="}[rel]
	}
	op := trace.Op{Op: trace.OpCompare, A: ia, Result: rel}
	if ib >= 0 {
		op.B = &ib
	} else {
		op.Value = format(b)
		if n.X != nil && it.collIndex(n.X, e) < 0 {
			op.Value = format(a)
		}
	}
	it.rec.op(op)
}

func relation(a, b value) string {
	lt := func(x, y value) bool {
		switch {
		case x.k == kInt && y.k == kInt:
			return x.i < y.i
		case x.k == kString && y.k == kString:
			return x.s < y.s
		}
		return toFloat(x) < toFloat(y)
	}
	switch {
	case lt(a, b):
		return "<"
	case lt(b, a):
		return ">"
	}
	return "="
}

func toFloat(v value) float64 {
	if v.k == kInt {
		return float64(v.i)
	}
	return v.f
}

func (it *interp) binary(op token.Token, a, b value, at ast.Node) value {
	if a.k == kUnknown {
		return a
	}
	if b.k == kUnknown {
		return b
	}
	if a.k == kInt && b.k == kFloat {
		a = floatV(float64(a.i))
	}
	if a.k == kFloat && b.k == kInt {
		b = floatV(float64(b.i))
	}
	switch {
	case a.k == kInt && b.k == kInt:
		x, y := a.i, b.i
		switch op {
		case token.ADD:
			return intV(x + y)
		case token.SUB:
			return intV(x - y)
		case token.MUL:
			return intV(x * y)
		case token.QUO, token.REM:
			if y == 0 {
				it.panicf(at, "runtime error: integer divide by zero")
			}
			if op == token.QUO {
				return intV(x / y)
			}
			return intV(x % y)
		case token.AND:
			return intV(x & y)
		case token.OR:
			return intV(x | y)
		case token.XOR:
			return intV(x ^ y)
		case token.AND_NOT:
			return intV(x &^ y)
		case token.SHL:
			return intV(x << uint64(y))
		case token.SHR:
			return intV(x >> uint64(y))
		case token.EQL:
			return boolV(x == y)
		case token.NEQ:
			return boolV(x != y)
		case token.LSS:
			return boolV(x < y)
		case token.LEQ:
			return boolV(x <= y)
		case token.GTR:
			return boolV(x > y)
		case token.GEQ:
			return boolV(x >= y)
		}
	case a.k == kFloat && b.k == kFloat:
		x, y := a.f, b.f
		switch op {
		case token.ADD:
			return floatV(x + y)
		case token.SUB:
			return floatV(x - y)
		case token.MUL:
			return floatV(x * y)
		case token.QUO:
			return floatV(x / y)
		case token.EQL:
			return boolV(x == y)
		case token.NEQ:
			return boolV(x != y)
		case token.LSS:
			return boolV(x < y)
		case token.LEQ:
			return boolV(x <= y)
		case token.GTR:
			return boolV(x > y)
		case token.GEQ:
			return boolV(x >= y)
		}
	case a.k == kString && b.k == kString:
		x, y := a.s, b.s
		switch op {
		case token.ADD:
			return strV(x + y)
		case token.EQL:
			return boolV(x == y)
		case token.NEQ:
			return boolV(x != y)
		case token.LSS:
			return boolV(x < y)
		case token.LEQ:
			return boolV(x <= y)
		case token.GTR:
			return boolV(x > y)
		case token.GEQ:
			return boolV(x >= y)
		}
	case a.k == kBool && b.k == kBool:
		switch op {
		case token.EQL:
			return boolV(a.b == b.b)
		case token.NEQ:
			return boolV(a.b != b.b)
		}
	case a.k == kNil || b.k == kNil:
		other := a
		if a.k == kNil {
			other = b
		}
		isNil := other.k == kNil || (other.k == kSlice && other.sl == nil) || (other.k == kMap && other.m == nil) ||
			(other.k == kFunc && other.fn == nil) || (other.k == kChan && other.ch == nil)
		switch op {
		case token.EQL:
			return boolV(isNil)
		case token.NEQ:
			return boolV(!isNil)
		}
	}
	return unknown(fmt.Sprintf("operasi %s di baris %d belum disimulasikan", op, it.line(at)))
}

func (it *interp) unary(n *ast.UnaryExpr, e *env) value {
	if n.Op == token.ARROW {
		v, _ := it.recv(n, e)
		return v
	}
	v := it.eval(n.X, e)
	switch {
	case v.k == kUnknown:
		return v
	case n.Op == token.SUB && v.k == kInt:
		return intV(-v.i)
	case n.Op == token.SUB && v.k == kFloat:
		return floatV(-v.f)
	case n.Op == token.ADD:
		return v
	case n.Op == token.NOT && v.k == kBool:
		return boolV(!v.b)
	case n.Op == token.XOR && v.k == kInt:
		return intV(^v.i)
	}
	return unknown(fmt.Sprintf("operator %s belum disimulasikan", n.Op))
}

func (it *interp) recv(n *ast.UnaryExpr, e *env) (value, bool) {
	ch := it.eval(n.X, e)
	if ch.k != kChan || ch.ch == nil {
		it.structural("menerima dari channel ini belum disimulasikan")
	}
	if len(ch.ch.buf) == 0 {
		if ch.ch.closed {
			return zeroOf(it.p.Info.Types[n].Type), false
		}
		it.structural("channel kosong: program butuh goroutine lain (belum disimulasikan)")
	}
	v := ch.ch.buf[0]
	ch.ch.buf = ch.ch.buf[1:]
	return v, true
}

func (it *interp) evalIndex(n *ast.IndexExpr, e *env) value {
	base := it.eval(n.X, e)
	idx := it.eval(n.Index, e)
	switch base.k {
	case kUnknown:
		return base
	case kMap:
		if v, ok := base.m.get(idx); ok {
			return v
		}
		return zeroOf(it.p.Info.Types[n].Type)
	case kSlice:
		return *base.sl.at(it.index(n, idx, base.sl.elemsLen()))
	case kArray:
		return base.arr[it.index(n, idx, len(base.arr))]
	case kString:
		return intV(int64(base.s[it.index(n, idx, len(base.s))]))
	}
	return unknown(fmt.Sprintf("indeks pada %q belum disimulasikan", it.text(n.X)))
}

func (it *interp) evalSlice(n *ast.SliceExpr, e *env) value {
	var base value
	if _, ok := unparen(n.X).(*ast.Ident); ok && it.eval(n.X, e).k == kArray {
		cell := it.ref(n.X, e) // arr[:] berbagi memori dengan array aslinya
		base = value{k: kSlice, sl: &sliceVal{data: &cell.arr, len: len(cell.arr), cap: len(cell.arr)}}
	} else {
		base = it.eval(n.X, e)
	}
	length, capacity := 0, 0
	switch base.k {
	case kSlice:
		length = base.sl.elemsLen()
		if base.sl != nil {
			capacity = base.sl.cap
		}
	case kString:
		length, capacity = len(base.s), len(base.s)
	case kUnknown:
		return base
	default:
		return unknown("slice atas nilai ini belum disimulasikan")
	}
	bound := func(x ast.Expr, def int) int {
		if x == nil {
			return def
		}
		v := it.eval(x, e)
		if v.k != kInt {
			it.structural(fmt.Sprintf("batas slice di baris %d tidak bisa dihitung", it.line(x)))
		}
		return int(v.i)
	}
	lo, hi := bound(n.Low, 0), bound(n.High, length)
	maxCap := capacity
	if n.Slice3 {
		maxCap = bound(n.Max, capacity)
	}
	if lo < 0 || hi < lo || hi > maxCap || maxCap > capacity {
		it.panicf(n, "runtime error: slice bounds out of range [%d:%d] with capacity %d", lo, hi, capacity)
	}
	if base.k == kString {
		return strV(base.s[lo:hi])
	}
	if base.sl == nil {
		return value{k: kSlice}
	}
	return value{k: kSlice, sl: &sliceVal{data: base.sl.data, off: base.sl.off + lo, len: hi - lo, cap: maxCap - lo}}
}

// ---------- pemanggilan ----------

func (it *interp) args(n *ast.CallExpr, e *env) []value {
	if len(n.Args) == 1 {
		if c, ok := unparen(n.Args[0]).(*ast.CallExpr); ok {
			if tv, ok := it.p.Info.Types[c]; ok {
				if _, multi := tv.Type.(*types.Tuple); multi {
					return it.call(c, e) // f(g()) dengan g mengembalikan banyak nilai
				}
			}
		}
	}
	out := make([]value, len(n.Args))
	for i, a := range n.Args {
		out[i] = copyVal(it.eval(a, e))
	}
	return out
}

func (it *interp) call(n *ast.CallExpr, e *env) []value {
	// konversi tipe: int(x), string(r), []byte(s), ...
	if tv, ok := it.p.Info.Types[n.Fun]; ok && tv.IsType() {
		if len(n.Args) != 1 {
			return []value{unknown("konversi tidak valid")}
		}
		return []value{convert(it.eval(n.Args[0], e), it.p.Info.Types[n.Args[0]].Type, tv.Type)}
	}
	switch f := unparen(n.Fun).(type) {
	case *ast.Ident:
		if it.isBuiltin(f) {
			return it.builtin(f.Name, n, e)
		}
	case *ast.SelectorExpr:
		if pkg, ok := f.X.(*ast.Ident); ok {
			if pn, ok := it.p.Info.Uses[pkg].(*types.PkgName); ok {
				return it.pkgCall(pn.Imported().Path(), f.Sel.Name, n, e)
			}
		}
		it.warn(fmt.Sprintf("method %s belum disimulasikan", it.text(n.Fun)))
		it.args(n, e)
		return []value{unknown(it.text(n.Fun) + " belum disimulasikan")}
	}
	fv := it.eval(n.Fun, e)
	if fv.k != kFunc || fv.fn == nil {
		if fv.k == kFunc {
			it.panicf(n, "runtime error: invalid memory address or nil pointer dereference")
		}
		return []value{unknown(fmt.Sprintf("fungsi %s tidak diketahui", it.text(n.Fun)))}
	}
	args := it.args(n, e)
	if n.Ellipsis.IsValid() && len(args) > 0 {
		last := args[len(args)-1]
		args = append(args[:len(args)-1:len(args)-1], last.sl.elems()...)
	}
	return it.callFunc(fv.fn, args, n)
}

func (it *interp) callFunc(fn *funcVal, args []value, at ast.Node) []value {
	if fn.builtin != nil {
		return fn.builtin(args)
	}
	it.depth++
	defer func() { it.depth-- }()
	if it.depth > maxDepth {
		it.structural("rekursi terlalu dalam untuk disimulasikan")
	}
	var ft *ast.FuncType
	var body *ast.BlockStmt
	parent := it.globals
	if fn.decl != nil {
		ft, body = fn.decl.Type, fn.decl.Body
	} else if fn.lit != nil {
		ft, body, parent = fn.lit.Type, fn.lit.Body, fn.env
	}
	if body == nil {
		return []value{unknown("fungsi tanpa isi")}
	}
	var node ast.Node
	if fn.decl != nil {
		node = fn.decl
	} else {
		node = fn.lit
	}

	// Langkah call/return dicatat bila pemanggilnya sedang direkam, atau bila fungsi ini
	// adalah fokus yang baru dimulai. Depth = "lantai" call stack relatif terhadap awal fokus.
	r := it.rec
	callerRecording := it.recording()
	fr, leave := it.enterBlock(node, scene.BlockFunc)
	defer leave()
	startsFocus := fr != nil && fr.focus && r.active == 1
	// Pemanggilan fungsi iterator oleh range adalah mekanisme loop, bukan panggilan yang ditulis user.
	_, viaRange := at.(*ast.RangeStmt)
	recordCall := (callerRecording || startsFocus) && !viaRange
	if recordCall {
		prevDepth := r.depth
		if startsFocus {
			r.depth = 0
		} else {
			r.depth = prevDepth + 1
		}
		defer func() { r.depth = prevDepth }()
	}

	scope := newEnv(parent, true)
	var params []string
	i := 0
	if ft.Params != nil {
		for fi, field := range ft.Params.List {
			_, variadic := field.Type.(*ast.Ellipsis)
			names := field.Names
			if len(names) == 0 {
				names = []*ast.Ident{ast.NewIdent("_")}
			}
			for _, name := range names {
				var v value
				switch {
				case variadic && fi == len(ft.Params.List)-1:
					var rest []value
					if i < len(args) {
						rest = args[i:]
					}
					v = sliceOf(append([]value(nil), rest...))
					i = len(args)
				case i < len(args):
					v = args[i]
					i++
				default:
					v = unknown(fmt.Sprintf("%s adalah parameter fungsi %s", name.Name, fn.name))
				}
				if obj := it.p.Info.Defs[name]; obj != nil {
					v = coerce(v, obj.Type())
				}
				scope.define(name.Name, v)
				if name.Name != "_" {
					params = append(params, name.Name+" = "+show(v))
				}
			}
		}
	}
	argText := strings.Join(params, ", ")
	if recordCall && it.recording() {
		it.add(trace.Step{Phase: trace.PhaseCall, Line: it.line(at), Func: fn.name, Args: argText,
			Note: fmt.Sprintf("panggil %s(%s)", fn.name, argText)}, scope)
	}
	var named []*ast.Ident
	if ft.Results != nil {
		for _, field := range ft.Results.List {
			for _, name := range field.Names {
				var v value
				if obj := it.p.Info.Defs[name]; obj != nil {
					v = zeroOf(obj.Type())
				}
				scope.define(name.Name, v)
				named = append(named, name)
			}
		}
	}
	// Fungsi yang ditulis di luar blok fokus tetap dijalankan, tetapi langkah di dalamnya
	// tidak direkam supaya trace hanya berisi kode blok itu sendiri. Output-nya tetap tercatat.
	quietBody := it.rec.on() && !it.insideFocus(body)
	if quietBody {
		it.quiet++
	}
	f := it.execList(body.List, scope)
	if quietBody {
		it.quiet--
	}
	out := f.ret
	if f.k != fReturn || len(f.ret) == 0 {
		out = make([]value, len(named))
		for i, name := range named {
			out[i] = *scope.lookup(name.Name)
		}
	}
	if recordCall && it.recording() {
		rets := make([]string, len(out))
		for i, v := range out {
			rets[i] = show(v)
		}
		ret := strings.Join(rets, ", ")
		note := fmt.Sprintf("%s(%s) selesai", fn.name, argText)
		if ret != "" {
			note = fmt.Sprintf("%s(%s) mengembalikan %s", fn.name, argText, ret)
		}
		it.add(trace.Step{Phase: trace.PhaseReturn, Line: it.p.Line(body.Rbrace), Func: fn.name, Return: ret, Note: note}, scope)
	}
	return out
}

func (it *interp) builtin(name string, n *ast.CallExpr, e *env) []value {
	one := func(v value) []value { return []value{v} }
	switch name {
	case "len", "cap":
		v := it.eval(n.Args[0], e)
		switch v.k {
		case kSlice:
			if name == "cap" && v.sl != nil {
				return one(intV(int64(v.sl.cap)))
			}
			return one(intV(int64(v.sl.elemsLen())))
		case kArray:
			return one(intV(int64(len(v.arr))))
		case kString:
			return one(intV(int64(len(v.s))))
		case kMap:
			if v.m == nil {
				return one(intV(0))
			}
			return one(intV(int64(len(v.m.keys))))
		case kChan:
			if v.ch == nil {
				return one(intV(0))
			}
			if name == "cap" {
				return one(intV(int64(v.ch.cap)))
			}
			return one(intV(int64(len(v.ch.buf))))
		}
		return one(unknown(fmt.Sprintf("panjang %s tidak diketahui", it.text(n.Args[0]))))
	case "append":
		s := it.eval(n.Args[0], e)
		if s.k == kUnknown {
			return one(s)
		}
		var extra []value
		if n.Ellipsis.IsValid() {
			rest := it.eval(n.Args[1], e)
			if rest.k == kString {
				for i := 0; i < len(rest.s); i++ {
					extra = append(extra, intV(int64(rest.s[i])))
				}
			} else {
				extra = append(extra, rest.sl.elems()...)
			}
		} else {
			for _, a := range n.Args[1:] {
				extra = append(extra, copyVal(it.eval(a, e)))
			}
		}
		if elemT := sliceElem(it.p.Info.Types[n].Type); elemT != nil {
			for i := range extra {
				extra[i] = coerce(extra[i], elemT)
			}
		}
		return one(appendVals(s, extra))
	case "make":
		t := it.p.Info.Types[n.Args[0]].Type
		size := 0
		if len(n.Args) > 1 {
			v := it.eval(n.Args[1], e)
			if v.k != kInt {
				return one(unknown("ukuran make tidak diketahui"))
			}
			size = int(v.i)
		}
		switch u := t.Underlying().(type) {
		case *types.Slice:
			c := size
			if len(n.Args) > 2 {
				if v := it.eval(n.Args[2], e); v.k == kInt {
					c = int(v.i)
				}
			}
			if size < 0 || c < size {
				it.panicf(n, "runtime error: makeslice: len out of range")
			}
			data := make([]value, c)
			for i := range data {
				data[i] = zeroOf(u.Elem())
			}
			return one(value{k: kSlice, sl: &sliceVal{data: &data, len: size, cap: c}})
		case *types.Map:
			return one(value{k: kMap, m: newMap()})
		case *types.Chan:
			return one(value{k: kChan, ch: &chanVal{cap: size}})
		}
		return one(unknown("make untuk tipe ini belum disimulasikan"))
	case "delete":
		m := it.eval(n.Args[0], e)
		if m.k == kMap && m.m != nil {
			m.m.del(it.eval(n.Args[1], e))
		}
		return nil
	case "copy":
		dst, src := it.eval(n.Args[0], e), it.eval(n.Args[1], e)
		if dst.k != kSlice || dst.sl == nil {
			return one(intV(0))
		}
		var from []value
		if src.k == kString {
			for i := 0; i < len(src.s); i++ {
				from = append(from, intV(int64(src.s[i])))
			}
		} else {
			from = append([]value(nil), src.sl.elems()...)
		}
		c := min(dst.sl.len, len(from))
		for i := 0; i < c; i++ {
			*dst.sl.at(i) = from[i]
		}
		return one(intV(int64(c)))
	case "min", "max":
		var best value
		for i, a := range n.Args {
			v := it.eval(a, e)
			if v.k == kUnknown {
				return one(v)
			}
			if i == 0 {
				best = v
				continue
			}
			rel := relation(v, best)
			if (name == "min" && rel == "<") || (name == "max" && rel == ">") {
				best = v
			}
		}
		return one(best)
	case "panic":
		v := it.eval(n.Args[0], e)
		it.panicf(n, "%s", format(v))
	case "print", "println":
		var parts []string
		for _, a := range n.Args {
			parts = append(parts, format(it.eval(a, e)))
		}
		sep, end := "", ""
		if name == "println" {
			sep, end = " ", "\n"
		}
		it.output(strings.Join(parts, sep) + end)
		return nil
	case "close":
		ch := it.eval(n.Args[0], e)
		if ch.k == kChan && ch.ch != nil {
			if ch.ch.closed {
				it.panicf(n, "close of closed channel")
			}
			ch.ch.closed = true
		}
		return nil
	}
	return one(unknown(fmt.Sprintf("fungsi bawaan %s belum disimulasikan", name)))
}

func sliceElem(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	if s, ok := t.Underlying().(*types.Slice); ok {
		return s.Elem()
	}
	return nil
}

// appendVals meniru append Go: menulis ke array lama bila kapasitas cukup, selain itu menyalin ke array baru.
func appendVals(s value, extra []value) value {
	if s.k != kSlice {
		return unknown("append pada nilai yang bukan slice")
	}
	n := s.sl.elemsLen()
	if s.sl != nil && n+len(extra) <= s.sl.cap {
		for i, v := range extra {
			*s.sl.at(n + i) = v
		}
		return value{k: kSlice, sl: &sliceVal{data: s.sl.data, off: s.sl.off, len: n + len(extra), cap: s.sl.cap}}
	}
	c := max(2*n, n+len(extra), 4)
	data := make([]value, c)
	copy(data, s.sl.elems())
	copy(data[n:], extra)
	return value{k: kSlice, sl: &sliceVal{data: &data, len: n + len(extra), cap: c}}
}

// isBytes melaporkan apakah t adalah slice byte (bukan slice rune).
func isBytes(t types.Type) bool {
	if t == nil {
		return false
	}
	if s, ok := t.Underlying().(*types.Slice); ok {
		if b, ok := s.Elem().Underlying().(*types.Basic); ok {
			return b.Kind() == types.Uint8
		}
	}
	return false
}

// convert menjalankan konversi tipe Go, mis. string(r), []byte(s), float64(n).
func convert(v value, from, to types.Type) value {
	if v.k == kUnknown {
		return v
	}
	switch u := to.Underlying().(type) {
	case *types.Basic:
		info := u.Info()
		switch {
		case info&types.IsString != 0:
			switch v.k {
			case kInt:
				return strV(string(rune(v.i)))
			case kSlice:
				var b strings.Builder
				bytes := isBytes(from)
				for _, el := range v.sl.elems() {
					if bytes {
						b.WriteByte(byte(el.i))
					} else {
						b.WriteRune(rune(el.i))
					}
				}
				return strV(b.String())
			}
			return v
		case info&types.IsInteger != 0:
			if v.k == kFloat {
				return intV(wrapInt(int64(v.f), u.Kind()))
			}
			if v.k == kInt {
				return intV(wrapInt(v.i, u.Kind()))
			}
		case info&types.IsFloat != 0:
			if v.k == kInt {
				return floatV(float64(v.i))
			}
		}
	case *types.Slice:
		if v.k == kString {
			var elems []value
			if isBytes(to) {
				for i := 0; i < len(v.s); i++ {
					elems = append(elems, intV(int64(v.s[i])))
				}
			} else {
				for _, r := range v.s {
					elems = append(elems, intV(int64(r)))
				}
			}
			return sliceOf(elems)
		}
	}
	return v
}

func (it *interp) output(s string) {
	if it.rec.on() {
		it.rec.out.WriteString(s)
	}
}

// pkgCall menangani fungsi dari package standar yang aman disimulasikan.
func (it *interp) pkgCall(pkg, name string, n *ast.CallExpr, e *env) []value {
	args := it.args(n, e)
	if pkg != "fmt" { // fmt tetap mencetak; nilai yang tidak diketahui tampil sebagai "?"
		for _, a := range args {
			if a.k == kUnknown {
				return []value{a}
			}
		}
	}
	natives := make([]any, len(args))
	for i, a := range args {
		natives[i] = native(a)
	}
	str := func(i int) string { return args[i].s }
	num := func(i int) float64 { return toFloat(args[i]) }
	switch pkg + "." + name {
	case "fmt.Println":
		it.output(fmt.Sprintln(natives...))
		return nil
	case "fmt.Print":
		it.output(fmt.Sprint(natives...))
		return nil
	case "fmt.Printf":
		if len(args) > 0 {
			it.output(fmt.Sprintf(str(0), natives[1:]...))
		}
		return nil
	case "fmt.Sprintf":
		return []value{strV(fmt.Sprintf(str(0), natives[1:]...))}
	case "fmt.Sprint":
		return []value{strV(fmt.Sprint(natives...))}
	case "fmt.Sprintln":
		return []value{strV(fmt.Sprintln(natives...))}
	case "strings.ToUpper":
		return []value{strV(strings.ToUpper(str(0)))}
	case "strings.ToLower":
		return []value{strV(strings.ToLower(str(0)))}
	case "strings.Repeat":
		if args[1].i < 0 || args[1].i > 10000 {
			return []value{unknown("strings.Repeat terlalu besar")}
		}
		return []value{strV(strings.Repeat(str(0), int(args[1].i)))}
	case "strings.Contains":
		return []value{boolV(strings.Contains(str(0), str(1)))}
	case "strings.HasPrefix":
		return []value{boolV(strings.HasPrefix(str(0), str(1)))}
	case "strings.HasSuffix":
		return []value{boolV(strings.HasSuffix(str(0), str(1)))}
	case "strings.Index":
		return []value{intV(int64(strings.Index(str(0), str(1))))}
	case "strings.TrimSpace":
		return []value{strV(strings.TrimSpace(str(0)))}
	case "strings.Split", "strings.Fields":
		var parts []string
		if name == "Split" {
			parts = strings.Split(str(0), str(1))
		} else {
			parts = strings.Fields(str(0))
		}
		vals := make([]value, len(parts))
		for i, p := range parts {
			vals[i] = strV(p)
		}
		return []value{sliceOf(vals)}
	case "strings.Join":
		var parts []string
		for _, v := range args[0].sl.elems() {
			parts = append(parts, v.s)
		}
		return []value{strV(strings.Join(parts, str(1)))}
	case "strconv.Itoa":
		return []value{strV(strconv.Itoa(int(args[0].i)))}
	case "strconv.Atoi":
		v, err := strconv.Atoi(str(0))
		if err != nil {
			return []value{intV(0), unknown("error dari strconv.Atoi")}
		}
		return []value{intV(int64(v)), {k: kNil}}
	case "math.Sqrt":
		return []value{floatV(math.Sqrt(num(0)))}
	case "math.Abs":
		return []value{floatV(math.Abs(num(0)))}
	case "math.Pow":
		return []value{floatV(math.Pow(num(0), num(1)))}
	case "math.Floor":
		return []value{floatV(math.Floor(num(0)))}
	case "math.Ceil":
		return []value{floatV(math.Ceil(num(0)))}
	case "math.Max":
		return []value{floatV(math.Max(num(0), num(1)))}
	case "math.Min":
		return []value{floatV(math.Min(num(0), num(1)))}
	case "math.Mod":
		return []value{floatV(math.Mod(num(0), num(1)))}
	case "unicode.IsUpper":
		return []value{boolV(unicode.IsUpper(rune(args[0].i)))}
	case "unicode.IsLower":
		return []value{boolV(unicode.IsLower(rune(args[0].i)))}
	case "unicode.IsDigit":
		return []value{boolV(unicode.IsDigit(rune(args[0].i)))}
	case "unicode.IsLetter":
		return []value{boolV(unicode.IsLetter(rune(args[0].i)))}
	case "unicode.IsSpace":
		return []value{boolV(unicode.IsSpace(rune(args[0].i)))}
	}
	it.warn(fmt.Sprintf("%s.%s tidak disimulasikan; hasilnya dianggap tidak diketahui", pkg, name))
	return []value{unknown(fmt.Sprintf("hasil %s.%s tidak diketahui", pkg, name))}
}
