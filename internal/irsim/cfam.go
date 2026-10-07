package irsim

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"gopherviz/internal/ir"
)

// cSem: semantik C dan C++ (cpp = true). Bilangan bulat memakai lebar dan tanda tipe
// statisnya (int 32-bit, long 32-bit seperti di Windows/MinGW, long long 64-bit), dengan
// promosi aritmetika biasa C. Pembagian bilangan bulat dengan nol menghentikan program.
type cSem struct {
	cpp     bool
	globals map[string]Value
}

func (s *cSem) setup(it *interp) {
	s.globals = map[string]Value{}
	it.setupC(s)
}

// ---------- tipe bilangan ----------

type intType struct {
	bits     uint
	unsigned bool
	rank     int
}

// normType merapikan nama tipe: "const unsigned int" → "unsigned int", "std::string" → "string".
func normType(t string) string {
	t = strings.TrimSpace(t)
	t = strings.ReplaceAll(t, "std::", "")
	var out []string
	for _, w := range strings.Fields(t) {
		switch w {
		case "const", "static", "volatile", "register", "inline", "constexpr", "extern", "struct", "enum", "mutable":
			continue
		}
		out = append(out, w)
	}
	s := strings.Join(out, " ")
	s = strings.ReplaceAll(s, " *", "*")
	s = strings.ReplaceAll(s, " &", "&")
	return s
}

func cIntType(t string) (intType, bool) {
	switch normType(t) {
	case "char", "signed char":
		return intType{8, false, 1}, true
	case "unsigned char", "uint8_t":
		return intType{8, true, 1}, true
	case "int8_t":
		return intType{8, false, 1}, true
	case "short", "short int", "signed short", "int16_t":
		return intType{16, false, 2}, true
	case "unsigned short", "unsigned short int", "uint16_t":
		return intType{16, true, 2}, true
	case "int", "signed", "signed int", "int32_t", "wchar_t":
		return intType{32, false, 3}, true
	case "unsigned", "unsigned int", "uint32_t":
		return intType{32, true, 3}, true
	case "long", "long int", "signed long":
		return intType{32, false, 4}, true
	case "unsigned long", "unsigned long int":
		return intType{32, true, 4}, true
	case "long long", "long long int", "signed long long", "int64_t", "ptrdiff_t", "ssize_t", "intptr_t":
		return intType{64, false, 5}, true
	case "unsigned long long", "unsigned long long int", "uint64_t", "size_t", "uintptr_t":
		return intType{64, true, 5}, true
	case "bool", "_Bool":
		return intType{1, true, 0}, true
	}
	return intType{}, false
}

func isFloatType(t string) bool {
	switch normType(t) {
	case "float", "double", "long double":
		return true
	}
	return false
}

// wrap memotong n ke lebar bit tipe t (seperti overflow di C).
func wrap(n int64, it intType) int64 {
	switch {
	case it.bits == 1:
		if n != 0 {
			return 1
		}
		return 0
	case it.bits >= 64:
		return n
	case it.unsigned:
		return int64(uint64(n) & (1<<it.bits - 1))
	}
	shift := 64 - it.bits
	return n << shift >> shift
}

func cTypeOf(v Value) string {
	switch v.k {
	case kInt:
		if v.t != "" {
			return normType(v.t)
		}
		return "int"
	case kFloat:
		if v.t == "float" {
			return "float"
		}
		return "double"
	case kBool:
		return "bool"
	}
	return ""
}

// promote: tipe hasil operasi aritmetika dua bilangan bulat.
func promote(a, b string) string {
	ta, okA := cIntType(a)
	tb, okB := cIntType(b)
	if !okA {
		ta = intType{32, false, 3}
	}
	if !okB {
		tb = intType{32, false, 3}
	}
	if ta.rank < 3 {
		ta, a = intType{32, false, 3}, "int"
	}
	if tb.rank < 3 {
		tb, b = intType{32, false, 3}, "int"
	}
	switch {
	case ta.rank > tb.rank:
		return a
	case tb.rank > ta.rank:
		return b
	case ta.unsigned:
		return a
	}
	return b
}

func (s *cSem) lit(it *interp, l *ir.Lit) Value {
	switch l.Kind {
	case ir.LitInt:
		raw := strings.ToLower(strings.ReplaceAll(l.Raw, "'", ""))
		v := strings.TrimRight(strings.ReplaceAll(l.Value, "'", ""), "uUlL")
		base := 10
		switch {
		case strings.HasPrefix(strings.ToLower(v), "0x"):
			base, v = 16, v[2:]
		case strings.HasPrefix(strings.ToLower(v), "0b"):
			base, v = 2, v[2:]
		case len(v) > 1 && v[0] == '0':
			base, v = 8, v[1:]
		}
		n, err := strconv.ParseUint(v, base, 64)
		if err != nil {
			return unknown("angka terlalu besar")
		}
		t := "int"
		switch {
		case strings.Contains(raw, "ll") && strings.Contains(raw, "u"):
			t = "unsigned long long"
		case strings.Contains(raw, "ll"):
			t = "long long"
		case strings.HasSuffix(raw, "ul") || strings.HasSuffix(raw, "lu"):
			t = "unsigned long"
		case strings.HasSuffix(raw, "l"):
			t = "long"
		case strings.HasSuffix(raw, "u"):
			t = "unsigned int"
		case n > math.MaxInt32:
			t = "long long"
		}
		return typedInt(int64(n), t)
	case ir.LitFloat:
		f, _ := strconv.ParseFloat(strings.TrimRight(l.Value, "fFlL"), 64)
		if strings.HasSuffix(strings.ToLower(l.Raw), "f") {
			return Value{k: kFloat, f: float64(float32(f)), t: "float"}
		}
		return floatV(f)
	case ir.LitString:
		return strV(l.Value)
	case ir.LitChar:
		if len(l.Value) == 0 {
			return typedInt(0, "char")
		}
		return typedInt(int64(int8(l.Value[0])), "char")
	case ir.LitBool:
		if !s.cpp {
			return typedInt(map[bool]int64{true: 1}[l.Value == "true"], "int")
		}
		return boolV(l.Value == "true")
	case ir.LitNull:
		return null()
	}
	return unknown("literal")
}

func (s *cSem) truthy(v Value) bool {
	switch v.k {
	case kBool:
		return v.b
	case kInt:
		return v.i != 0
	case kFloat:
		return v.f != 0
	case kNull:
		return false
	case kPtr:
		return true
	case kStr:
		return true // pointer string tidak NULL
	}
	return true
}

func (it *interp) cBool(b bool) Value {
	if it.lang == "c" {
		if b {
			return typedInt(1, "int")
		}
		return typedInt(0, "int")
	}
	return boolV(b)
}

// ---------- operator ----------

func numVal(v Value) bool { return v.k == kInt || v.k == kFloat || v.k == kBool }

func (s *cSem) binary(it *interp, op string, a, b Value, at ir.Node) Value {
	// stream: cout << x
	if op == "<<" && a.k == kObj && a.o.class.native == "ostream" {
		if b.k == kObj && b.o.class.native == "" {
			if fs := it.funcs["operator<<"]; len(fs) > 0 {
				return it.callFunc(&Func{name: "operator<<", decl: fs[0], env: it.globals, overloads: fs}, undef(), argList{pos: []Value{a, b}}, at)
			}
		}
		it.streamWrite(a, b, at)
		return a
	}
	if op == ">>" && a.k == kObj && a.o.class.native == "istream" {
		it.structural("membaca input dari user (cin >>) belum didukung simulator")
	}
	// operator buatan user (C++): a + b → a.operator+(b)
	if a.k == kObj && s.cpp {
		if m := findMethod(a.o.class, "operator"+op); m != nil {
			return it.callFunc(&Func{name: "operator" + op, decl: m, env: a.o.class.env, class: it.classOf[m]}, a, argList{pos: []Value{b}}, at)
		}
		if fs := it.funcs["operator"+op]; len(fs) > 0 {
			return it.callFunc(&Func{name: "operator" + op, decl: fs[0], env: it.globals}, undef(), argList{pos: []Value{a, b}}, at)
		}
	}
	if fs := it.funcs["operator"+op]; len(fs) > 0 && s.cpp && (a.k == kObj || b.k == kObj) {
		return it.callFunc(&Func{name: "operator" + op, decl: fs[0], env: it.globals}, undef(), argList{pos: []Value{a, b}}, at)
	}
	// string C++
	if s.cpp && (a.k == kStr || b.k == kStr) && a.k != kPtr && b.k != kPtr {
		as, bs := it.cppStrOperand(a), it.cppStrOperand(b)
		switch op {
		case "+":
			return strV(as + bs)
		case "==":
			return boolV(as == bs)
		case "!=":
			return boolV(as != bs)
		case "<":
			return boolV(as < bs)
		case ">":
			return boolV(as > bs)
		case "<=":
			return boolV(as <= bs)
		case ">=":
			return boolV(as >= bs)
		}
	}
	// pointer
	if a.k == kPtr || b.k == kPtr || a.k == kNull || b.k == kNull {
		return it.cPtrOp(op, a, b, at)
	}
	if a.k == kList && (op == "+" || op == "-") && isNum(b) { // array meluruh jadi pointer
		return it.cPtrOp(op, Value{k: kPtr, p: &Ptr{list: a.l}}, b, at)
	}
	if a.k == kList && b.k == kList && (op == "==" || op == "!=") {
		eq := it.cListEq(a, b)
		if op == "!=" {
			eq = !eq
		}
		return it.cBool(eq)
	}
	if a.k == kObj && b.k == kObj && (op == "==" || op == "!=") {
		return it.cBool((a.o == b.o) == (op == "=="))
	}
	if !numVal(a) || !numVal(b) {
		if a.k == kUnknown {
			return a
		}
		if b.k == kUnknown {
			return b
		}
		it.structural(fmt.Sprintf("operator %s untuk nilai ini belum disimulasikan (baris %d)", op, it.line(at)))
	}
	ta, tb := cTypeOf(a), cTypeOf(b)
	if a.k == kFloat || b.k == kFloat {
		x, y := toF(a), toF(b)
		isFloat32 := (ta == "float" || a.k != kFloat) && (tb == "float" || b.k != kFloat) && (ta == "float" || tb == "float")
		res := func(f float64) Value {
			if isFloat32 {
				return Value{k: kFloat, f: float64(float32(f)), t: "float"}
			}
			return floatV(f)
		}
		switch op {
		case "+":
			return res(x + y)
		case "-":
			return res(x - y)
		case "*":
			return res(x * y)
		case "/":
			return res(x / y)
		case "<":
			return it.cBool(x < y)
		case ">":
			return it.cBool(x > y)
		case "<=":
			return it.cBool(x <= y)
		case ">=":
			return it.cBool(x >= y)
		case "==":
			return it.cBool(x == y)
		case "!=":
			return it.cBool(x != y)
		case "%":
			it.structural("operator % tidak berlaku untuk bilangan desimal (gunakan fmod)")
		}
		it.structural("operator " + op + " untuk bilangan desimal belum disimulasikan")
	}
	rt := promote(ta, tb)
	t, _ := cIntType(rt)
	x, y := wrap(intOf(a), t), wrap(intOf(b), t)
	ux, uy := uint64(x), uint64(y)
	if t.bits < 64 && t.unsigned {
		ux, uy = uint64(x)&(1<<t.bits-1), uint64(y)&(1<<t.bits-1)
	}
	ret := func(n int64) Value { return typedInt(wrap(n, t), rt) }
	cmp := func(lt, eq bool) Value {
		switch op {
		case "<":
			return it.cBool(lt)
		case ">":
			return it.cBool(!lt && !eq)
		case "<=":
			return it.cBool(lt || eq)
		case ">=":
			return it.cBool(!lt)
		case "==":
			return it.cBool(eq)
		}
		return it.cBool(!eq)
	}
	switch op {
	case "+":
		return ret(x + y)
	case "-":
		return ret(x - y)
	case "*":
		return ret(x * y)
	case "/", "%":
		if y == 0 {
			it.cCrash(at, "pembagian bilangan bulat dengan nol (program crash: SIGFPE)")
		}
		if t.unsigned {
			if op == "/" {
				return ret(int64(ux / uy))
			}
			return ret(int64(ux % uy))
		}
		if op == "/" {
			return ret(x / y)
		}
		return ret(x % y)
	case "&":
		return ret(x & y)
	case "|":
		return ret(x | y)
	case "^":
		return ret(x ^ y)
	case "<<":
		return typedInt(wrap(x<<uint(intOf(b)), t), rt)
	case ">>":
		if t.unsigned {
			return ret(int64(ux >> uint(intOf(b))))
		}
		return ret(x >> uint(intOf(b)))
	case "<", ">", "<=", ">=", "==", "!=":
		if t.unsigned {
			return cmp(ux < uy, ux == uy)
		}
		return cmp(x < y, x == y)
	}
	it.structural("operator " + op + " belum disimulasikan")
	return Value{}
}

func intOf(v Value) int64 {
	switch v.k {
	case kInt:
		return v.i
	case kBool:
		if v.b {
			return 1
		}
	case kFloat:
		return int64(v.f)
	}
	return 0
}

// cCrash menghentikan program seperti crash di C/C++ (tidak bisa ditangkap catch).
func (it *interp) cCrash(at ir.Node, msg string) {
	c := &Class{name: "__crash__", native: "crash", statics: it.newDict("statics")}
	it.ids++
	o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
	o.fields.set(strV("message"), strV(msg))
	panic(thrown{v: Value{k: kObj, o: o}, msg: msg, at: at})
}

func (it *interp) cListEq(a, b Value) bool {
	if len(a.l.elems) != len(b.l.elems) {
		return false
	}
	for i := range a.l.elems {
		r := it.sem.binary(it, "==", a.l.elems[i], b.l.elems[i], nil)
		if !it.sem.truthy(r) {
			return false
		}
	}
	return true
}

func (it *interp) cppStrOperand(v Value) string {
	switch v.k {
	case kStr:
		return v.s
	case kInt:
		if normType(v.t) == "char" {
			return string([]byte{byte(v.i)})
		}
	case kList:
		return cString(v)
	}
	return it.sem.str(it, v)
}

// cPtrOp: aritmetika dan perbandingan pointer.
func (it *interp) cPtrOp(op string, a, b Value, at ir.Node) Value {
	switch op {
	case "+", "-":
		if a.k == kPtr && isNum(b) {
			np := *a.p
			if op == "+" {
				np.idx += int(intOf(b))
			} else {
				np.idx -= int(intOf(b))
			}
			return Value{k: kPtr, p: &np, t: a.t}
		}
		if a.k == kPtr && b.k == kPtr && op == "-" {
			return typedInt(int64(a.p.idx-b.p.idx), "long long")
		}
		if b.k == kPtr && isNum(a) && op == "+" {
			return it.cPtrOp("+", b, a, at)
		}
		if a.k == kStr && isNum(b) { // "teks" + 1
			n := int(intOf(b))
			if op == "+" && n <= len(a.s) {
				return strV(a.s[n:])
			}
		}
	case "==", "!=":
		eq := ptrEq(a, b)
		if op == "!=" {
			eq = !eq
		}
		return it.cBool(eq)
	case "<", ">", "<=", ">=":
		if a.k == kPtr && b.k == kPtr {
			x, y := a.p.idx, b.p.idx
			return it.cBool(map[string]bool{"<": x < y, ">": x > y, "<=": x <= y, ">=": x >= y}[op])
		}
	}
	it.structural(fmt.Sprintf("operasi pointer %s di baris %d belum disimulasikan", op, it.line(at)))
	return Value{}
}

func ptrEq(a, b Value) bool {
	switch {
	case a.k == kNull || b.k == kNull:
		return a.k == b.k
	case a.k == kPtr && b.k == kPtr:
		pa, pb := a.p, b.p
		if pa.list != nil || pb.list != nil {
			return pa.list == pb.list && pa.idx == pb.idx
		}
		if pa.dict != nil || pb.dict != nil {
			return pa.dict == pb.dict && pa.idx == pb.idx
		}
		if pa.obj != nil {
			return pa.obj == pb.obj && pa.field == pb.field
		}
		if pa.cell != nil && pb.cell != nil {
			va, vb := pa.cell.get(), pb.cell.get()
			if va.k == kObj && vb.k == kObj {
				return va.o == vb.o
			}
		}
		return pa.cell == pb.cell
	}
	return false
}

func (s *cSem) unary(it *interp, op string, v Value, at ir.Node) Value {
	switch op {
	case "!":
		return it.cBool(!s.truthy(v))
	case "-":
		if v.k == kFloat {
			return Value{k: kFloat, f: -v.f, t: v.t}
		}
		t := promote(cTypeOf(v), "int")
		it2, _ := cIntType(t)
		return typedInt(wrap(-intOf(v), it2), t)
	case "+":
		if v.k == kFloat {
			return v
		}
		return typedInt(intOf(v), promote(cTypeOf(v), "int"))
	case "~":
		t := promote(cTypeOf(v), "int")
		it2, _ := cIntType(t)
		return typedInt(wrap(^intOf(v), it2), t)
	}
	it.structural("operator " + op + " belum disimulasikan")
	return Value{}
}

func (s *cSem) caseEqual(it *interp, a, b Value) bool { return intOf(a) == intOf(b) }

func (s *cSem) zero(typ string) Value {
	t := normType(typ)
	if it, ok := cIntType(t); ok {
		if t == "bool" && s.cpp {
			return boolV(false)
		}
		_ = it
		return typedInt(0, t)
	}
	if isFloatType(t) {
		if t == "float" {
			return Value{k: kFloat, t: "float"}
		}
		return floatV(0)
	}
	if t == "string" {
		return strV("")
	}
	if strings.HasSuffix(t, "*") {
		return null()
	}
	return typedInt(0, "int")
}

// convert menyimpan nilai ke variabel bertipe typ (konversi implisit C/C++).
func (s *cSem) convert(it *interp, v Value, typ string) Value {
	t := normType(typ)
	if t == "" || t == "auto" || strings.HasSuffix(t, "&") && v.k == kPtr {
		return v
	}
	t = strings.TrimSuffix(t, "&")
	if v.k == kUnknown {
		return v
	}
	if ti, ok := cIntType(t); ok {
		switch v.k {
		case kInt, kBool:
			if t == "bool" {
				if s.cpp {
					return boolV(intOf(v) != 0)
				}
				return typedInt(map[bool]int64{true: 1}[intOf(v) != 0], "_Bool")
			}
			return typedInt(wrap(intOf(v), ti), t)
		case kFloat:
			if t == "bool" {
				if s.cpp {
					return boolV(v.f != 0)
				}
				return typedInt(map[bool]int64{true: 1}[v.f != 0], "_Bool")
			}
			if math.IsNaN(v.f) || math.IsInf(v.f, 0) {
				return typedInt(wrap(math.MinInt64, ti), t)
			}
			return typedInt(wrap(int64(v.f), ti), t)
		case kPtr, kNull:
			if t == "bool" {
				return it.cBool(v.k == kPtr)
			}
		case kStr:
			if t == "char" && len(v.s) == 1 {
				return typedInt(int64(int8(v.s[0])), "char")
			}
		}
		return v
	}
	if isFloatType(t) {
		if numVal(v) {
			f := toF(v)
			if t == "float" {
				return Value{k: kFloat, f: float64(float32(f)), t: "float"}
			}
			return floatV(f)
		}
		return v
	}
	if v.k == kList && v.l.kind == "init" { // {a, b} menjadi struct/kelas tujuan
		name := t
		if i := strings.IndexByte(name, '<'); i >= 0 {
			name = name[:i]
		}
		if c := it.findClass(name, it.globals); c != nil && c.native == "" {
			if it.findCtor(c, len(v.l.elems)) != nil {
				return it.construct(c, v.l.elems, nil)
			}
			obj := it.construct(c, nil, nil)
			for i, f := range it.allFields(c) {
				if i < len(v.l.elems) {
					it.setAggField(obj, f, v.l.elems[i])
				}
			}
			return obj
		}
		if c := it.findClass(name, it.globals); c != nil && c.ctorT != nil { // vector<int> v = {..} di parameter/return
			return c.ctorT(it, t, []Value{v}, nil)
		}
		if t == "pair" || strings.HasPrefix(t, "pair<") {
			return it.cppPair(arg(v.l.elems, 0), arg(v.l.elems, 1))
		}
	}
	if t == "string" {
		switch v.k {
		case kList:
			return strV(cString(v))
		case kInt:
			if normType(v.t) == "char" {
				return strV(string([]byte{byte(v.i)}))
			}
		}
		return v
	}
	if strings.HasSuffix(t, "*") {
		switch v.k {
		case kList: // array meluruh jadi pointer ke elemen pertama
			if t == "char*" || t == "char *" {
				return v
			}
			return Value{k: kPtr, p: &Ptr{list: v.l}}
		case kPtr:
			if v.p.list != nil && strings.HasPrefix(v.p.list.elem, "bytes:") { // hasil malloc
				it.cTypeHeap(v.p.list, strings.TrimSuffix(t, "*"))
			}
		}
		return v
	}
	return v
}

// cTypeHeap memberi tipe pada memori malloc: N byte → N/sizeof(T) elemen.
func (it *interp) cTypeHeap(l *List, elem string) {
	n, _ := strconv.Atoi(strings.TrimPrefix(l.elem, "bytes:"))
	size := int64(4)
	if sz := it.sizeOf(&ir.TypeRef{Name: elem}, it.globals); sz.k == kInt && sz.i > 0 {
		size = sz.i
	}
	cnt := n / int(size)
	l.elems = make([]Value, cnt)
	for i := range l.elems {
		l.elems[i] = it.zeroOfType(elem, nil)
	}
	l.elem = elem
}

func (s *cSem) newError(it *interp, class, msg string) Value {
	c := it.findClass(class, nil)
	if c == nil {
		c = it.findClass("runtime_error", nil)
	}
	return it.construct(c, []Value{strV(msg)}, nil)
}

func (s *cSem) iterate(it *interp, x Value, fe *ir.ForEach) ([]kv, bool) {
	switch x.k {
	case kList:
		out := make([]kv, len(x.l.elems))
		for i, e := range x.l.elems {
			out[i] = kv{k: typedInt(int64(i), "size_t"), v: e}
		}
		return out, true
	case kStr:
		out := make([]kv, len(x.s))
		for i := 0; i < len(x.s); i++ {
			out[i] = kv{k: typedInt(int64(i), "size_t"), v: typedInt(int64(int8(x.s[i])), "char")}
		}
		return out, true
	case kDict:
		out := make([]kv, len(x.d.keys))
		for i, k := range x.d.keys {
			out[i] = kv{k: k, v: it.cppPair(k, x.d.vals[i])}
		}
		return out, true
	case kSet:
		out := make([]kv, len(x.d.keys))
		for i, k := range x.d.keys {
			out[i] = kv{k: k, v: k}
		}
		return out, true
	}
	return nil, false
}

func (it *interp) cppPair(a, b Value) Value { return it.newList("pair", []Value{a, b}) }

// ---------- tampilan ----------

func cString(v Value) string {
	switch v.k {
	case kStr:
		return v.s
	case kList:
		var b []byte
		for _, e := range v.l.elems {
			if e.k != kInt || e.i == 0 {
				break
			}
			b = append(b, byte(e.i))
		}
		return string(b)
	case kPtr:
		if v.p.list != nil {
			var b []byte
			for _, e := range v.p.list.elems[min(v.p.idx, len(v.p.list.elems)):] {
				if e.k != kInt || e.i == 0 {
					break
				}
				b = append(b, byte(e.i))
			}
			return string(b)
		}
	}
	return ""
}

func (s *cSem) str(it *interp, v Value) string {
	switch v.k {
	case kStr:
		return v.s
	case kInt:
		if normType(v.t) == "char" || normType(v.t) == "unsigned char" {
			return string([]byte{byte(v.i)})
		}
		if ti, ok := cIntType(v.t); ok && ti.unsigned && ti.bits == 64 {
			return strconv.FormatUint(uint64(v.i), 10)
		}
		return strconv.FormatInt(v.i, 10)
	case kFloat:
		return cDouble(v.f)
	case kBool:
		if v.b {
			return "1"
		}
		return "0"
	case kNull:
		return "0"
	case kList:
		if v.l.kind == "carray" && v.l.elem == "char" {
			return cString(v)
		}
	case kObj:
		if instanceOf(v.o.class, "exception") {
			m, _ := v.o.fields.get(strV("message"))
			return m.s
		}
	}
	return s.show(it, v)
}

func (s *cSem) show(it *interp, v Value) string {
	switch v.k {
	case kStr:
		return strconv.Quote(v.s)
	case kInt:
		if normType(v.t) == "char" {
			if v.i >= 32 && v.i < 127 {
				return "'" + string(rune(v.i)) + "'"
			}
			return strconv.FormatInt(v.i, 10)
		}
		return s.str(it, v)
	case kFloat:
		return cDouble(v.f)
	case kBool:
		if v.b {
			return "true"
		}
		return "false"
	case kNull:
		if s.cpp {
			return "nullptr"
		}
		return "NULL"
	case kList:
		if v.l.kind == "carray" && v.l.elem == "char" {
			return strconv.Quote(cString(v))
		}
		var parts []string
		for i, e := range v.l.elems {
			if i >= 20 {
				parts = append(parts, "…")
				break
			}
			parts = append(parts, s.show(it, e))
		}
		if v.l.kind == "pair" {
			return "(" + strings.Join(parts, ", ") + ")"
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case kDict:
		var parts []string
		for i, k := range v.d.keys {
			parts = append(parts, "{"+s.show(it, k)+", "+s.show(it, v.d.vals[i])+"}")
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case kSet:
		var parts []string
		for _, k := range v.d.keys {
			parts = append(parts, s.show(it, k))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case kObj:
		if v.o.class.native == "ostream" {
			return "cout"
		}
		var parts []string
		for i, k := range v.o.fields.keys {
			parts = append(parts, k.s+": "+s.show(it, v.o.fields.vals[i]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case kPtr:
		if v.p.name != "" && v.p.list == nil {
			return "→ " + v.p.name
		}
		if v.p.list != nil {
			return fmt.Sprintf("→ elemen ke-%d", v.p.idx)
		}
		return "→ ?"
	case kFunc:
		return v.fn.name
	case kUnknown:
		return "?"
	}
	return "?"
}

// ---------- stream (cout) ----------

func (it *interp) streamState(st Value, name string) Value {
	v, _ := st.o.fields.get(strV(name))
	return v
}

// streamWrite menulis satu nilai ke cout dengan memperhatikan manipulator aktif.
func (it *interp) streamWrite(st, v Value, at ir.Node) {
	if v.k == kObj && v.o.class.native == "__manip" {
		name, _ := v.o.fields.get(strV("name"))
		arg, _ := v.o.fields.get(strV("arg"))
		switch name.s {
		case "endl":
			it.streamOut(st, "\n")
		case "setw":
			st.o.fields.set(strV("width"), arg)
		case "setprecision":
			st.o.fields.set(strV("precision"), arg)
		case "setfill":
			st.o.fields.set(strV("fill"), arg)
		case "fixed", "scientific":
			st.o.fields.set(strV("floatfield"), strV(name.s))
		case "defaultfloat":
			st.o.fields.set(strV("floatfield"), strV(""))
		case "left", "right", "internal":
			st.o.fields.set(strV("adjust"), strV(name.s))
		case "boolalpha":
			st.o.fields.set(strV("boolalpha"), boolV(true))
		case "noboolalpha":
			st.o.fields.set(strV("boolalpha"), boolV(false))
		case "showpoint":
			st.o.fields.set(strV("showpoint"), boolV(true))
		case "flush":
		}
		return
	}
	var s string
	switch v.k {
	case kFloat:
		prec := int(intOf(it.streamState(st, "precision")))
		switch it.streamState(st, "floatfield").s {
		case "fixed":
			s = strconv.FormatFloat(v.f, 'f', prec, 64)
		case "scientific":
			s = strconv.FormatFloat(v.f, 'e', prec, 64)
		default:
			if prec == 0 {
				prec = 1
			}
			if math.IsInf(v.f, 0) || math.IsNaN(v.f) {
				s = cDouble(v.f)
			} else {
				s = strconv.FormatFloat(v.f, 'g', prec, 64)
			}
		}
		if v.t == "float" && it.streamState(st, "floatfield").s == "" {
			s = strconv.FormatFloat(float64(float32(v.f)), 'g', max(1, prec), 32)
		}
	case kBool:
		if it.streamState(st, "boolalpha").b {
			s = map[bool]string{true: "true", false: "false"}[v.b]
		} else {
			s = map[bool]string{true: "1", false: "0"}[v.b]
		}
	case kPtr:
		if v.p.list != nil && v.p.list.elem == "char" {
			s = cString(v)
		} else {
			s = fmt.Sprintf("0x%x", 0x61fe00+uintptr(objectID(Value{k: kList, l: v.p.list}))*16+uintptr(v.p.idx)*4)
		}
	case kNull:
		s = "0"
	case kUnknown:
		s = "?"
	default:
		s = it.sem.str(it, v)
	}
	if w := int(intOf(it.streamState(st, "width"))); w > len(s) {
		fill := " "
		if f := it.streamState(st, "fill"); f.k == kInt {
			fill = string([]byte{byte(f.i)})
		}
		pad := strings.Repeat(fill, w-len(s))
		if it.streamState(st, "adjust").s == "left" {
			s += pad
		} else {
			s = pad + s
		}
	}
	st.o.fields.set(strV("width"), typedInt(0, "int"))
	it.streamOut(st, s)
}

func (it *interp) streamOut(st Value, s string) {
	if name, _ := st.o.fields.get(strV("stream")); name.s == "cerr" {
		it.warn("output ke cerr tidak ditampilkan di output program")
		return
	}
	it.out(s)
}

// cPrintf memformat printf C.
func (it *interp) cPrintf(f string, args []Value, at ir.Node) string {
	var b strings.Builder
	ai := 0
	next := func() Value {
		if ai >= len(args) {
			return typedInt(0, "int")
		}
		v := args[ai]
		ai++
		return v
	}
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			b.WriteByte(f[i])
			continue
		}
		j := i + 1
		for j < len(f) && strings.IndexByte("-+ #0", f[j]) >= 0 {
			j++
		}
		flags := f[i+1 : j]
		width := ""
		if j < len(f) && f[j] == '*' {
			width = strconv.FormatInt(intOf(next()), 10)
			j++
		}
		for j < len(f) && f[j] >= '0' && f[j] <= '9' {
			width += string(f[j])
			j++
		}
		prec := ""
		if j < len(f) && f[j] == '.' {
			prec = "."
			j++
			if j < len(f) && f[j] == '*' {
				prec += strconv.FormatInt(intOf(next()), 10)
				j++
			}
			for j < len(f) && f[j] >= '0' && f[j] <= '9' {
				prec += string(f[j])
				j++
			}
		}
		length := ""
		for j < len(f) && strings.IndexByte("hlLqjzt", f[j]) >= 0 {
			length += string(f[j])
			j++
		}
		if j >= len(f) {
			b.WriteString(f[i:])
			break
		}
		verb := f[j]
		i = j
		spec := "%" + flags + width + prec
		switch verb {
		case '%':
			b.WriteByte('%')
		case 'd', 'i':
			v := next()
			n := intOf(v)
			switch length {
			case "hh":
				n = int64(int8(n))
			case "h":
				n = int64(int16(n))
			case "", "l":
				n = int64(int32(n))
			}
			b.WriteString(fmt.Sprintf(spec+"d", n))
		case 'u':
			n := uint64(intOf(next()))
			if length == "" || length == "l" {
				n &= 0xffffffff
			}
			b.WriteString(fmt.Sprintf(spec+"d", n))
		case 'x', 'X', 'o':
			n := uint64(intOf(next()))
			if length == "" || length == "l" {
				n &= 0xffffffff
			}
			b.WriteString(fmt.Sprintf(spec+string(verb), n))
		case 'f', 'F', 'e', 'E', 'g', 'G':
			v := next()
			x := toF(v)
			p := prec
			if p == "" {
				p = ".6"
			}
			s := fmt.Sprintf("%"+flags+width+p+string(verb), x)
			b.WriteString(s)
		case 'c':
			v := next()
			b.WriteString(fmt.Sprintf("%"+flags+width+"c", rune(byte(intOf(v)))))
		case 's':
			v := next()
			s := cString(v)
			if v.k == kStr {
				s = v.s
			}
			b.WriteString(fmt.Sprintf(spec+"s", s))
		case 'p':
			next()
			b.WriteString("000000000061FE14")
		default:
			b.WriteString(f[i-len(spec)+1 : i+1])
		}
	}
	return b.String()
}
