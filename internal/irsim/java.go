package irsim

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"gopherviz/internal/ir"
)

// javaSem: semantik Java. Tipe primitif dengan lebar tetap (int 32, long 64, char 16-bit
// tak bertanda), promosi numerik biner Java, bagi nol bilangan bulat = ArithmeticException.
// String immutable (kStr); objek dan array dipegang lewat referensi.
type javaSem struct {
	globals   map[string]Value
	comparing builtinFn       // Comparator.comparing (dipakai thenComparing)
	pqCmp     map[*List]Value // comparator tiap PriorityQueue
}

func (s *javaSem) setup(it *interp) {
	s.globals = map[string]Value{}
	s.pqCmp = map[*List]Value{}
	it.setupJava(s)
}

func javaIntType(t string) (intType, bool) {
	switch strings.TrimSpace(t) {
	case "byte":
		return intType{8, false, 1}, true
	case "short":
		return intType{16, false, 2}, true
	case "char":
		return intType{16, true, 2}, true
	case "int", "Integer":
		return intType{32, false, 3}, true
	case "long", "Long":
		return intType{64, false, 5}, true
	}
	return intType{}, false
}

func jtype(v Value) string {
	switch v.k {
	case kInt:
		if v.t == "" {
			return "int"
		}
		return v.t
	case kFloat:
		if v.t == "float" {
			return "float"
		}
		return "double"
	}
	return ""
}

func jint(n int64) Value  { return typedInt(int64(int32(n)), "int") }
func jlong(n int64) Value { return typedInt(n, "long") }
func jchar(r int64) Value { return typedInt(int64(uint16(r)), "char") }

func (s *javaSem) lit(it *interp, l *ir.Lit) Value {
	switch l.Kind {
	case ir.LitInt:
		raw := strings.ToLower(strings.ReplaceAll(l.Raw, "_", ""))
		v := strings.TrimRight(strings.ReplaceAll(l.Value, "_", ""), "lL")
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
			return unknown("angka")
		}
		if strings.HasSuffix(raw, "l") {
			return jlong(int64(n))
		}
		return jint(int64(n))
	case ir.LitFloat:
		raw := strings.ToLower(l.Raw)
		f, _ := strconv.ParseFloat(strings.TrimRight(strings.ReplaceAll(l.Value, "_", ""), "fFdD"), 64)
		if strings.HasSuffix(raw, "f") {
			return Value{k: kFloat, f: float64(float32(f)), t: "float"}
		}
		return floatV(f)
	case ir.LitString:
		return strV(l.Value)
	case ir.LitChar:
		rs := utf16.Encode([]rune(l.Value))
		if len(rs) == 0 {
			return jchar(0)
		}
		return jchar(int64(rs[0]))
	case ir.LitBool:
		return boolV(l.Value == "true")
	case ir.LitNull:
		return null()
	}
	return unknown("literal")
}

func (s *javaSem) truthy(v Value) bool { return v.k == kBool && v.b }

// ---------- string ----------

func (s *javaSem) str(it *interp, v Value) string {
	switch v.k {
	case kStr:
		return v.s
	case kInt:
		if v.t == "char" {
			return string(utf16.Decode([]uint16{uint16(v.i)}))
		}
		return strconv.FormatInt(v.i, 10)
	case kFloat:
		if v.t == "float" {
			return javaDouble(v.f, 32)
		}
		return javaDouble(v.f, 64)
	case kBool:
		if v.b {
			return "true"
		}
		return "false"
	case kNull:
		return "null"
	case kList:
		return it.javaListString(v)
	case kDict:
		var parts []string
		for i, k := range javaIterOrder(v.d) {
			_ = i
			val, _ := v.d.get(k)
			parts = append(parts, s.str(it, k)+"="+s.str(it, val))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case kSet:
		var parts []string
		for _, k := range javaIterOrder(v.d) {
			parts = append(parts, s.str(it, k))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case kObj:
		return it.javaObjString(v)
	case kFunc:
		off := 0
		if v.fn.decl != nil { // fungsi bawaan tidak punya deklarasi
			off = v.fn.decl.S.Start.Off
		}
		return "Main$$Lambda@" + strconv.FormatInt(int64(0x1b6d3586+off), 16)
	case kClass:
		return "class " + v.c.name
	case kUnknown:
		return "?"
	}
	return ""
}

func (it *interp) javaListString(v Value) string {
	switch v.l.kind {
	case "array":
		return "[" + javaArrayType(v.l.elem) + "@" + strconv.FormatInt(int64(0x1b6d3586+v.l.id*0x10), 16)
	}
	parts := make([]string, len(v.l.elems))
	for i, e := range v.l.elems {
		if e.k == kList && e.l == v.l {
			parts[i] = "(this Collection)"
			continue
		}
		parts[i] = it.sem.str(it, e)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func javaArrayType(elem string) string {
	switch elem {
	case "int":
		return "I"
	case "double":
		return "D"
	case "char":
		return "C"
	case "boolean":
		return "Z"
	case "long":
		return "J"
	case "String":
		return "Ljava.lang.String;"
	}
	return "L" + elem + ";"
}

func (it *interp) javaObjString(v Value) string {
	c := v.o.class
	if m := findMethod(c, "toString"); m != nil && len(m.Params) == 0 {
		r := it.callFunc(&Func{name: c.name + ".toString", decl: m, env: c.env, class: it.classOf[m]}, v, argList{}, m)
		return it.sem.str(it, r)
	}
	switch {
	case c.kind == "enum":
		n, _ := v.o.fields.get(strV("name"))
		return n.s
	case c.kind == "record":
		var parts []string
		for _, f := range it.allFields(c) {
			fv, _ := v.o.fields.get(strV(f.name))
			parts = append(parts, f.name+"="+it.sem.str(it, fv))
		}
		return c.name + "[" + strings.Join(parts, ", ") + "]"
	case instanceOf(c, "Throwable"):
		m, _ := v.o.fields.get(strV("message"))
		name := c.name
		if c.native != "" {
			name = javaPkg(c.name) + c.name
		}
		if m.k == kNull || m.k == kUnknown && m.why == "" {
			return name
		}
		return name + ": " + it.sem.str(it, m)
	case c.native == "StringBuilder":
		sb, _ := v.o.fields.get(strV("s"))
		return sb.s
	}
	return fmt.Sprintf("%s@%x", c.name, 0x1b6d3586+v.o.id*0x2f)
}

func javaPkg(name string) string {
	switch name {
	case "InputMismatchException", "NoSuchElementException", "ConcurrentModificationException":
		return "java.util."
	}
	return "java.lang."
}

func (s *javaSem) show(it *interp, v Value) string {
	switch v.k {
	case kStr:
		return strconv.Quote(v.s)
	case kInt:
		if v.t == "char" {
			return "'" + s.str(it, v) + "'"
		}
	case kList:
		parts := make([]string, 0, len(v.l.elems))
		for i, e := range v.l.elems {
			if i >= 20 {
				parts = append(parts, "…")
				break
			}
			parts = append(parts, s.show(it, e))
		}
		if v.l.kind == "array" {
			return "{" + strings.Join(parts, ", ") + "}"
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case kObj:
		if v.o.class.kind != "enum" && v.o.class.native == "" && findMethod(v.o.class, "toString") == nil {
			var parts []string
			for i, k := range v.o.fields.keys {
				parts = append(parts, k.s+"="+s.show(it, v.o.fields.vals[i]))
			}
			return v.o.class.name + "{" + strings.Join(parts, ", ") + "}"
		}
	}
	return s.str(it, v)
}

// ---------- operator ----------

// numPromote: tipe hasil promosi numerik biner Java.
func numPromote(a, b Value) string {
	ta, tb := jtype(a), jtype(b)
	switch {
	case ta == "double" || tb == "double":
		return "double"
	case ta == "float" || tb == "float":
		return "float"
	case ta == "long" || tb == "long":
		return "long"
	}
	return "int"
}

func (s *javaSem) binary(it *interp, op string, a, b Value, at ir.Node) Value {
	if op == "+" && (a.k == kStr || b.k == kStr) {
		return strV(s.str(it, a) + s.str(it, b))
	}
	switch op {
	case "==", "!=":
		eq := it.javaRefEq(a, b)
		if op == "!=" {
			eq = !eq
		}
		return boolV(eq)
	case "&&", "||":
		return boolV(op == "&&" && s.truthy(a) && s.truthy(b) || op == "||" && (s.truthy(a) || s.truthy(b)))
	}
	if a.k == kBool && b.k == kBool {
		switch op {
		case "&":
			return boolV(a.b && b.b)
		case "|":
			return boolV(a.b || b.b)
		case "^":
			return boolV(a.b != b.b)
		}
	}
	if !numVal(a) || !numVal(b) {
		if a.k == kNull || b.k == kNull {
			it.throw(at, "NullPointerException", "Cannot unbox null value")
		}
		if a.k == kUnknown {
			return a
		}
		if b.k == kUnknown {
			return b
		}
		it.structural(fmt.Sprintf("operator %s untuk nilai ini belum disimulasikan (baris %d)", op, it.line(at)))
	}
	rt := numPromote(a, b)
	if rt == "double" || rt == "float" {
		x, y := toF(a), toF(b)
		res := func(f float64) Value {
			if rt == "float" {
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
		case "%":
			return res(math.Mod(x, y))
		case "<":
			return boolV(x < y)
		case ">":
			return boolV(x > y)
		case "<=":
			return boolV(x <= y)
		case ">=":
			return boolV(x >= y)
		}
		it.structural("operator " + op + " untuk bilangan desimal belum disimulasikan")
	}
	x, y := intOf(a), intOf(b)
	ret := func(n int64) Value {
		if rt == "long" {
			return jlong(n)
		}
		return jint(n)
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
			it.throw(at, "ArithmeticException", "/ by zero")
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
		if rt == "long" || jtype(a) == "long" {
			return jlong(x << uint(y&63))
		}
		return jint(int64(int32(x) << uint(y&31)))
	case ">>":
		if jtype(a) == "long" {
			return jlong(x >> uint(y&63))
		}
		return jint(int64(int32(x) >> uint(y&31)))
	case ">>>":
		if jtype(a) == "long" {
			return jlong(int64(uint64(x) >> uint(y&63)))
		}
		return jint(int64(uint32(int32(x)) >> uint(y&31)))
	case "<":
		return boolV(x < y)
	case ">":
		return boolV(x > y)
	case "<=":
		return boolV(x <= y)
	case ">=":
		return boolV(x >= y)
	}
	it.structural("operator " + op + " belum disimulasikan")
	return Value{}
}

// javaRefEq: == di Java (primitif: nilai; objek: referensi; string literal: diperlakukan sama).
func (it *interp) javaRefEq(a, b Value) bool {
	if numVal(a) && numVal(b) {
		return toF(a) == toF(b)
	}
	if a.k != b.k {
		return false
	}
	switch a.k {
	case kStr:
		return a.s == b.s
	case kNull:
		return true
	case kObj:
		return a.o == b.o
	}
	return jsStrictEq(a, b)
}

// javaEquals: a.equals(b).
func (it *interp) javaEquals(a, b Value) bool {
	switch {
	case a.k == kObj:
		if m := findMethod(a.o.class, "equals"); m != nil {
			return it.sem.truthy(it.callFunc(&Func{decl: m, env: a.o.class.env, class: it.classOf[m]}, a, argList{pos: []Value{b}}, m))
		}
		if a.o.class.kind == "record" && b.k == kObj && b.o.class == a.o.class {
			for i, k := range a.o.fields.keys {
				bv, _ := b.o.fields.get(k)
				if !it.javaEquals(a.o.fields.vals[i], bv) {
					return false
				}
			}
			return true
		}
		return b.k == kObj && a.o == b.o
	case numVal(a) && numVal(b):
		return jtype(a) == jtype(b) && toF(a) == toF(b) || a.k == kBool && b.k == kBool && a.b == b.b
	case a.k == kList && b.k == kList && a.l.kind != "array":
		if len(a.l.elems) != len(b.l.elems) {
			return false
		}
		for i := range a.l.elems {
			if !it.javaEquals(a.l.elems[i], b.l.elems[i]) {
				return false
			}
		}
		return true
	}
	return it.javaRefEq(a, b)
}

func (s *javaSem) unary(it *interp, op string, v Value, at ir.Node) Value {
	switch op {
	case "!":
		return boolV(!s.truthy(v))
	case "-":
		switch v.k {
		case kFloat:
			return Value{k: kFloat, f: -v.f, t: v.t}
		case kInt:
			if v.t == "long" {
				return jlong(-v.i)
			}
			return jint(-v.i)
		}
	case "+":
		if v.k == kInt && v.t != "long" {
			return jint(v.i)
		}
		return v
	case "~":
		if v.t == "long" {
			return jlong(^v.i)
		}
		return jint(^v.i)
	}
	it.structural("operator " + op + " belum disimulasikan")
	return Value{}
}

func (s *javaSem) caseEqual(it *interp, a, b Value) bool {
	if a.k == kStr && b.k == kStr {
		return a.s == b.s
	}
	if a.k == kObj && b.k == kObj {
		return a.o == b.o
	}
	return numVal(a) && numVal(b) && toF(a) == toF(b)
}

func (s *javaSem) zero(typ string) Value {
	t := strings.TrimSpace(typ)
	switch t {
	case "int", "short", "byte":
		return typedInt(0, t)
	case "long":
		return jlong(0)
	case "char":
		return jchar(0)
	case "double":
		return floatV(0)
	case "float":
		return Value{k: kFloat, t: "float"}
	case "boolean":
		return boolV(false)
	}
	return null()
}

// convert: assignment ke variabel bertipe (pelebaran otomatis; penyempitan lewat cast).
func (s *javaSem) convert(it *interp, v Value, typ string) Value {
	t := strings.TrimSpace(typ)
	if i := strings.IndexByte(t, '<'); i >= 0 {
		t = t[:i]
	}
	switch t {
	case "double", "Double":
		if numVal(v) && v.k != kBool {
			return floatV(toF(v))
		}
	case "float", "Float":
		if numVal(v) && v.k != kBool {
			return Value{k: kFloat, f: float64(float32(toF(v))), t: "float"}
		}
	case "int", "Integer", "long", "Long", "short", "Short", "byte", "Byte", "char", "Character":
		jt := map[string]string{"Integer": "int", "Long": "long", "Short": "short", "Byte": "byte", "Character": "char"}[t]
		if jt == "" {
			jt = t
		}
		ti, _ := javaIntType(jt)
		switch v.k {
		case kInt:
			return typedInt(wrap(v.i, ti), jt)
		case kFloat:
			f := v.f
			switch {
			case math.IsNaN(f):
				return typedInt(0, jt)
			case jt == "long" && f >= math.MaxInt64:
				return jlong(math.MaxInt64)
			case jt == "long" && f <= math.MinInt64:
				return jlong(math.MinInt64)
			case jt != "long" && f >= math.MaxInt32:
				return typedInt(wrap(math.MaxInt32, ti), jt)
			case jt != "long" && f <= math.MinInt32:
				return typedInt(wrap(math.MinInt32, ti), jt)
			}
			return typedInt(wrap(int64(f), ti), jt)
		}
	case "String":
		return v
	}
	return v
}

func (s *javaSem) newError(it *interp, class, msg string) Value {
	c := it.findClass(class, nil)
	if c == nil {
		c = it.findClass("RuntimeException", nil)
	}
	return it.construct(c, []Value{strV(msg)}, nil)
}

func (s *javaSem) iterate(it *interp, x Value, fe *ir.ForEach) ([]kv, bool) {
	switch x.k {
	case kList:
		out := make([]kv, len(x.l.elems))
		for i, e := range x.l.elems {
			out[i] = kv{k: jint(int64(i)), v: e}
		}
		return out, true
	case kSet:
		keys := javaIterOrder(x.d)
		out := make([]kv, len(keys))
		for i, k := range keys {
			out[i] = kv{k: jint(int64(i)), v: k}
		}
		return out, true
	case kNull:
		it.throw(nil, "NullPointerException", "Cannot iterate over null")
	}
	return nil, false
}

// ---------- urutan HashMap / HashSet ----------

func javaHash(v Value) (int32, bool) {
	switch v.k {
	case kStr:
		var h int32
		for _, u := range utf16.Encode([]rune(v.s)) {
			h = 31*h + int32(u)
		}
		return h, true
	case kInt:
		if v.t == "long" {
			return int32(v.i ^ int64(uint64(v.i)>>32)), true
		}
		return int32(v.i), true
	case kBool:
		if v.b {
			return 1231, true
		}
		return 1237, true
	case kFloat:
		bits := math.Float64bits(v.f)
		return int32(bits ^ bits>>32), true
	}
	return 0, false
}

// javaIterOrder meniru urutan iterasi HashMap/HashSet Java (bucket = hash & (kapasitas-1),
// di dalam bucket urutan sisip). TreeMap/LinkedHashMap memakai urutannya sendiri.
func javaIterOrder(d *Dict) []Value {
	if d.kind != "HashMap" && d.kind != "HashSet" {
		return d.keys
	}
	capacity := 16
	for float64(len(d.keys)) > 0.75*float64(capacity) {
		capacity *= 2
	}
	type ent struct {
		k      Value
		bucket int
		order  int
	}
	ents := make([]ent, len(d.keys))
	for i, k := range d.keys {
		h, ok := javaHash(k)
		if !ok {
			return d.keys
		}
		spread := h ^ int32(uint32(h)>>16)
		ents[i] = ent{k, int(spread) & (capacity - 1), i}
	}
	sort.SliceStable(ents, func(a, b int) bool { return ents[a].bucket < ents[b].bucket })
	out := make([]Value, len(ents))
	for i, e := range ents {
		out[i] = e.k
	}
	return out
}

// ---------- format ----------

// javaFormat meniru String.format / printf Java.
func (it *interp) javaFormat(f string, args []Value, at ir.Node) string {
	var b strings.Builder
	ai := 0
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			b.WriteByte(f[i])
			continue
		}
		j := i + 1
		for j < len(f) && strings.IndexByte("-+ 0,#(", f[j]) >= 0 {
			j++
		}
		flags := f[i+1 : j]
		ws := j
		for j < len(f) && f[j] >= '0' && f[j] <= '9' {
			j++
		}
		width, _ := strconv.Atoi(f[ws:j])
		prec := -1
		if j < len(f) && f[j] == '.' {
			j++
			ps := j
			for j < len(f) && f[j] >= '0' && f[j] <= '9' {
				j++
			}
			prec, _ = strconv.Atoi(f[ps:j])
		}
		if j >= len(f) {
			break
		}
		conv := f[j]
		i = j
		if conv == 'n' {
			b.WriteString("\n")
			continue
		}
		if conv == '%' {
			b.WriteByte('%')
			continue
		}
		var v Value
		if ai < len(args) {
			v = args[ai]
			ai++
		} else {
			it.throw(at, "MissingFormatArgumentException", "Format specifier '%"+string(conv)+"'")
		}
		var s string
		switch conv {
		case 'd':
			if !numVal(v) || v.k == kFloat {
				it.throw(at, "IllegalFormatConversionException", fmt.Sprintf("d != %s", javaBoxName(v)))
			}
			n := intOf(v)
			s = strconv.FormatInt(abs64(n), 10)
			if strings.Contains(flags, ",") {
				s = groupThousands(s, ",")
			}
			s = javaSign(s, n < 0, flags)
		case 'f', 'e', 'E', 'g':
			if !numVal(v) || v.k != kFloat {
				it.throw(at, "IllegalFormatConversionException", fmt.Sprintf("%c != %s", conv, javaBoxName(v)))
			}
			p := prec
			if p < 0 {
				p = 6
			}
			x := toF(v)
			switch conv {
			case 'f':
				s = strings.TrimPrefix(jsToFixed(math.Abs(x), p), "-")
				if strings.Contains(flags, ",") {
					ip, fp, has := strings.Cut(s, ".")
					s = groupThousands(ip, ",")
					if has {
						s += "." + fp
					}
				}
			case 'e', 'E':
				s = strconv.FormatFloat(math.Abs(x), 'e', p, 64)
				if conv == 'E' {
					s = strings.ToUpper(s)
				}
			default:
				s = strconv.FormatFloat(math.Abs(x), 'g', p, 64)
			}
			s = javaSign(s, x < 0, flags)
		case 's', 'S':
			s = it.sem.str(it, v)
			if prec >= 0 && prec < len(s) {
				s = s[:prec]
			}
			if conv == 'S' {
				s = strings.ToUpper(s)
			}
		case 'c':
			s = it.sem.str(it, jchar(intOf(v)))
		case 'b', 'B':
			s = strconv.FormatBool(v.k != kNull && !(v.k == kBool && !v.b))
		case 'x', 'X':
			n := intOf(v)
			if v.t == "long" {
				s = strconv.FormatUint(uint64(n), 16)
			} else {
				s = strconv.FormatUint(uint64(uint32(n)), 16)
			}
			if conv == 'X' {
				s = strings.ToUpper(s)
			}
		case 'o':
			s = strconv.FormatUint(uint64(uint32(intOf(v))), 8)
		default:
			it.throw(at, "UnknownFormatConversionException", "Conversion = '"+string(conv)+"'")
		}
		if len([]rune(s)) < width {
			pad := width - len([]rune(s))
			switch {
			case strings.Contains(flags, "-"):
				s += strings.Repeat(" ", pad)
			case strings.Contains(flags, "0") && strings.ContainsRune("dfxXeEo", rune(conv)):
				sign := ""
				if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") || strings.HasPrefix(s, " ") {
					sign, s = s[:1], s[1:]
				}
				s = sign + strings.Repeat("0", pad) + s
			default:
				s = strings.Repeat(" ", pad) + s
			}
		}
		b.WriteString(s)
	}
	return b.String()
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func javaSign(s string, neg bool, flags string) string {
	switch {
	case neg:
		return "-" + s
	case strings.Contains(flags, "+"):
		return "+" + s
	case strings.Contains(flags, " "):
		return " " + s
	}
	return s
}

func javaBoxName(v Value) string {
	switch v.k {
	case kInt:
		return map[string]string{"long": "java.lang.Long", "char": "java.lang.Character"}[v.t] + map[bool]string{true: "java.lang.Integer"}[v.t != "long" && v.t != "char"]
	case kFloat:
		return "java.lang.Double"
	case kStr:
		return "java.lang.String"
	case kBool:
		return "java.lang.Boolean"
	}
	return "java.lang.Object"
}

func javaRound(f float64) int64 { return int64(math.Floor(f + 0.5)) }
