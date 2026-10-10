package irsim

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"gopherviz/internal/ir"
)

// jsSem: semantik JavaScript (dan TypeScript: anotasi tipe sudah dibuang oleh parser).
// Semua angka JavaScript adalah float64 (kFloat).
type jsSem struct {
	globals map[string]Value
}

func (s *jsSem) setup(it *interp) {
	s.globals = map[string]Value{}
	it.setupJS(s)
}

func num(f float64) Value { return floatV(f) }

func (s *jsSem) lit(it *interp, l *ir.Lit) Value {
	switch l.Kind {
	case ir.LitInt, ir.LitFloat:
		return num(parseJSNumber(l.Value))
	case ir.LitBigInt:
		b, _ := new(big.Int).SetString(l.Value, 0)
		if b == nil {
			b = new(big.Int)
		}
		return Value{k: kBig, big: b}
	case ir.LitString, ir.LitChar:
		return strV(l.Value)
	case ir.LitBool:
		return boolV(l.Value == "true")
	case ir.LitNull:
		return null()
	case ir.LitUndefined:
		return undef()
	case ir.LitRegex:
		return it.jsRegex(l.Value)
	}
	return unknown("literal")
}

func parseJSNumber(s string) float64 {
	s = strings.ReplaceAll(s, "_", "")
	low := strings.ToLower(s)
	switch {
	case strings.HasPrefix(low, "0x"), strings.HasPrefix(low, "0b"), strings.HasPrefix(low, "0o"):
		n, err := strconv.ParseInt(low[2:], map[byte]int{'x': 16, 'b': 2, 'o': 8}[low[1]], 64)
		if err == nil {
			return float64(n)
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

// ---------- konversi ----------

func (s *jsSem) truthy(v Value) bool {
	switch v.k {
	case kBool:
		return v.b
	case kFloat:
		return v.f != 0 && !math.IsNaN(v.f)
	case kInt:
		return v.i != 0
	case kBig:
		return v.big.Sign() != 0
	case kStr:
		return v.s != ""
	case kNull, kUndef:
		return false
	}
	return true
}

// toNumber mengikuti ToNumber JavaScript.
func (it *interp) jsToNumber(v Value) float64 {
	switch v.k {
	case kFloat:
		return v.f
	case kInt:
		return float64(v.i)
	case kBool:
		if v.b {
			return 1
		}
		return 0
	case kNull:
		return 0
	case kUndef:
		return math.NaN()
	case kStr:
		t := strings.TrimSpace(v.s)
		if t == "" {
			return 0
		}
		if t == "Infinity" || t == "+Infinity" {
			return math.Inf(1)
		}
		if t == "-Infinity" {
			return math.Inf(-1)
		}
		low := strings.ToLower(t)
		if strings.HasPrefix(low, "0x") || strings.HasPrefix(low, "0b") || strings.HasPrefix(low, "0o") {
			return parseJSNumber(t)
		}
		if strings.ContainsAny(low, "abcdfghijklmnopqrstuvwxyz_") && !strings.Contains(low, "e") || strings.Count(low, "e") > 1 {
			return math.NaN()
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	case kList, kDict, kObj:
		return it.jsToNumber(strV(it.sem.str(it, v)))
	}
	return math.NaN()
}

func (s *jsSem) str(it *interp, v Value) string {
	switch v.k {
	case kStr:
		return v.s
	case kFloat:
		return jsNumber(v.f)
	case kInt:
		return strconv.FormatInt(v.i, 10)
	case kBig:
		return v.big.String()
	case kBool:
		if v.b {
			return "true"
		}
		return "false"
	case kNull:
		return "null"
	case kUndef:
		return "undefined"
	case kList:
		parts := make([]string, len(v.l.elems))
		for i, e := range v.l.elems {
			if e.k != kNull && e.k != kUndef {
				parts[i] = s.str(it, e)
			}
		}
		return strings.Join(parts, ",")
	case kObj:
		if m := findMethod(v.o.class, "toString"); m != nil {
			return s.str(it, it.callFunc(&Func{name: "toString", decl: m, env: v.o.class.env, class: it.classOf[m]}, v, argList{}, m))
		}
		if instanceOf(v.o.class, "Error") {
			return it.exceptionText(v)
		}
		return "[object Object]"
	case kDict:
		if v.d.kind == "regex" {
			src, _ := v.d.get(strV("source"))
			flags, _ := v.d.get(strV("flags"))
			return "/" + src.s + "/" + flags.s
		}
		if f, ok := v.d.get(strV("toString")); ok && f.k == kFunc {
			return s.str(it, it.call(f, v, nil, nil))
		}
		return "[object Object]"
	case kSet:
		return "[object " + map[string]string{"set": "Set", "Map": "Map"}[v.d.kind] + "]"
	case kFunc:
		return "function " + v.fn.name + "() { [native code] }"
	case kClass:
		return "class " + v.c.name + " { }"
	}
	return "?"
}

func (s *jsSem) show(it *interp, v Value) string {
	if v.k == kStr {
		return strconv.Quote(v.s)
	}
	return it.jsInspect(v, 0, false, map[int]bool{}, 60)
}

// ---------- operator ----------

func (s *jsSem) binary(it *interp, op string, a, b Value, at ir.Node) Value {
	switch op {
	case "===":
		return boolV(jsStrictEq(a, b))
	case "!==":
		return boolV(!jsStrictEq(a, b))
	case "==":
		return boolV(it.jsLooseEq(a, b))
	case "!=":
		return boolV(!it.jsLooseEq(a, b))
	case "in":
		key := s.str(it, a)
		switch b.k {
		case kDict:
			return boolV(b.d.has(strV(key)))
		case kList:
			i, err := strconv.Atoi(key)
			return boolV(err == nil && i >= 0 && i < len(b.l.elems) || key == "length")
		case kObj:
			if b.o.fields.has(strV(key)) || findMethod(b.o.class, key) != nil {
				return boolV(true)
			}
			return boolV(false)
		}
		it.throw(at, "TypeError", "Cannot use 'in' operator to search for '"+key+"' in "+s.str(it, b))
	}
	if a.k == kBig || b.k == kBig {
		return it.jsBigBinary(op, a, b, at)
	}
	switch op {
	case "+":
		pa, pb := it.jsToPrimitive(a), it.jsToPrimitive(b)
		if pa.k == kStr || pb.k == kStr {
			return strV(s.str(it, pa) + s.str(it, pb))
		}
		return num(it.jsToNumber(pa) + it.jsToNumber(pb))
	case "<", ">", "<=", ">=":
		pa, pb := it.jsToPrimitive(a), it.jsToPrimitive(b)
		if pa.k == kStr && pb.k == kStr {
			c := compareUTF16(pa.s, pb.s)
			return boolV(map[string]bool{"<": c < 0, ">": c > 0, "<=": c <= 0, ">=": c >= 0}[op])
		}
		x, y := it.jsToNumber(pa), it.jsToNumber(pb)
		if math.IsNaN(x) || math.IsNaN(y) {
			return boolV(false)
		}
		return boolV(map[string]bool{"<": x < y, ">": x > y, "<=": x <= y, ">=": x >= y}[op])
	}
	x, y := it.jsToNumber(a), it.jsToNumber(b)
	switch op {
	case "-":
		return num(x - y)
	case "*":
		return num(x * y)
	case "/":
		return num(x / y)
	case "%":
		if y == 0 || math.IsInf(x, 0) || math.IsNaN(x) || math.IsNaN(y) {
			return num(math.NaN())
		}
		if math.IsInf(y, 0) {
			return num(x)
		}
		return num(math.Mod(x, y))
	case "**":
		return num(math.Pow(x, y))
	case "&":
		return num(float64(toInt32(x) & toInt32(y)))
	case "|":
		return num(float64(toInt32(x) | toInt32(y)))
	case "^":
		return num(float64(toInt32(x) ^ toInt32(y)))
	case "<<":
		return num(float64(toInt32(x) << (uint32(toInt32(y)) & 31)))
	case ">>":
		return num(float64(toInt32(x) >> (uint32(toInt32(y)) & 31)))
	case ">>>":
		return num(float64(uint32(toInt32(x)) >> (uint32(toInt32(y)) & 31)))
	}
	it.structural("operator " + op + " belum disimulasikan")
	return Value{}
}

func compareUTF16(a, b string) int {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			if ua[i] < ub[i] {
				return -1
			}
			return 1
		}
	}
	return len(ua) - len(ub)
}

func toInt32(f float64) int32 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return int32(uint32(int64(math.Trunc(math.Mod(f, 4294967296)))))
}

func (it *interp) jsBigBinary(op string, a, b Value, at ir.Node) Value {
	if a.k != kBig || b.k != kBig {
		if op == "+" && (a.k == kStr || b.k == kStr) {
			return strV(it.sem.str(it, a) + it.sem.str(it, b))
		}
		it.throw(at, "TypeError", "Cannot mix BigInt and other types, use explicit conversions")
	}
	r := new(big.Int)
	switch op {
	case "+":
		r.Add(a.big, b.big)
	case "-":
		r.Sub(a.big, b.big)
	case "*":
		r.Mul(a.big, b.big)
	case "/":
		if b.big.Sign() == 0 {
			it.throw(at, "RangeError", "Division by zero")
		}
		r.Quo(a.big, b.big)
	case "%":
		if b.big.Sign() == 0 {
			it.throw(at, "RangeError", "Division by zero")
		}
		r.Rem(a.big, b.big)
	case "**":
		r.Exp(a.big, b.big, nil)
	case "<", ">", "<=", ">=":
		c := a.big.Cmp(b.big)
		return boolV(map[string]bool{"<": c < 0, ">": c > 0, "<=": c <= 0, ">=": c >= 0}[op])
	default:
		it.structural("operator BigInt " + op + " belum disimulasikan")
	}
	return Value{k: kBig, big: r}
}

func (it *interp) jsToPrimitive(v Value) Value {
	switch v.k {
	case kList, kDict, kObj, kSet, kFunc, kClass:
		if v.k == kObj {
			if m := findMethod(v.o.class, "valueOf"); m != nil {
				return it.callFunc(&Func{name: "valueOf", decl: m, env: v.o.class.env}, v, argList{}, m)
			}
		}
		return strV(it.sem.str(it, v))
	}
	return v
}

func jsStrictEq(a, b Value) bool {
	if (a.k == kFloat || a.k == kInt) && (b.k == kFloat || b.k == kInt) {
		return toF(a) == toF(b)
	}
	if a.k != b.k {
		return false
	}
	switch a.k {
	case kStr:
		return a.s == b.s
	case kBool:
		return a.b == b.b
	case kNull, kUndef:
		return true
	case kBig:
		return a.big.Cmp(b.big) == 0
	case kList:
		return a.l == b.l
	case kDict, kSet:
		return a.d == b.d
	case kObj:
		return a.o == b.o
	case kFunc:
		return a.fn == b.fn || a.fn.decl != nil && a.fn.decl == b.fn.decl && a.fn.this == b.fn.this
	case kClass:
		return a.c == b.c
	}
	return false
}

func (it *interp) jsLooseEq(a, b Value) bool {
	nullish := func(v Value) bool { return v.k == kNull || v.k == kUndef }
	switch {
	case nullish(a) || nullish(b):
		return nullish(a) && nullish(b)
	case a.k == b.k || isNum(a) && isNum(b):
		return jsStrictEq(a, b)
	case a.k == kBool:
		return it.jsLooseEq(num(it.jsToNumber(a)), b)
	case b.k == kBool:
		return it.jsLooseEq(a, num(it.jsToNumber(b)))
	case isNum(a) && b.k == kStr || a.k == kStr && isNum(b):
		return it.jsToNumber(a) == it.jsToNumber(b)
	case (a.k == kList || a.k == kDict || a.k == kObj) && (b.k == kStr || isNum(b)):
		return it.jsLooseEq(it.jsToPrimitive(a), b)
	case (b.k == kList || b.k == kDict || b.k == kObj) && (a.k == kStr || isNum(a)):
		return it.jsLooseEq(a, it.jsToPrimitive(b))
	}
	return false
}

func (s *jsSem) unary(it *interp, op string, v Value, at ir.Node) Value {
	switch op {
	case "!":
		return boolV(!s.truthy(v))
	case "-":
		if v.k == kBig {
			return Value{k: kBig, big: new(big.Int).Neg(v.big)}
		}
		return num(-it.jsToNumber(v))
	case "+":
		return num(it.jsToNumber(v))
	case "~":
		return num(float64(^toInt32(it.jsToNumber(v))))
	case "void":
		return undef()
	case "typeof":
		switch v.k {
		case kFloat, kInt:
			return strV("number")
		case kBig:
			return strV("bigint")
		case kStr:
			return strV("string")
		case kBool:
			return strV("boolean")
		case kUndef:
			return strV("undefined")
		case kFunc, kClass:
			return strV("function")
		}
		return strV("object")
	}
	it.structural("operator " + op + " belum disimulasikan")
	return Value{}
}

func (s *jsSem) caseEqual(it *interp, a, b Value) bool { return jsStrictEq(a, b) }

func (s *jsSem) zero(typ string) Value { return undef() }

func (s *jsSem) convert(it *interp, v Value, typ string) Value { return v }

func (s *jsSem) newError(it *interp, class, msg string) Value {
	c := it.findClass(class, nil)
	if c == nil {
		c = it.findClass("Error", nil)
	}
	return it.construct(c, []Value{strV(msg)}, nil)
}

// ---------- iterasi ----------

func (s *jsSem) iterate(it *interp, x Value, fe *ir.ForEach) ([]kv, bool) {
	keysOnly := fe != nil && fe.Kind == "in"
	switch x.k {
	case kList:
		out := make([]kv, len(x.l.elems))
		for i, e := range x.l.elems {
			out[i] = kv{k: strV(strconv.Itoa(i)), v: e}
			if !keysOnly {
				out[i].k = num(float64(i))
			}
		}
		return out, true
	case kStr:
		var out []kv
		i := 0
		for _, r := range x.s {
			out = append(out, kv{k: strV(strconv.Itoa(i)), v: strV(string(r))})
			i++
		}
		return out, true
	case kDict:
		if x.d.kind == "Map" {
			out := make([]kv, len(x.d.keys))
			for i, k := range x.d.keys {
				out[i] = kv{k: k, v: it.newList("array", []Value{k, x.d.vals[i]})}
			}
			return out, !keysOnly
		}
		if keysOnly {
			out := make([]kv, len(x.d.keys))
			for i, k := range jsKeyOrder(x.d) {
				out[i] = kv{k: k, v: k}
			}
			return out, true
		}
		return nil, false
	case kSet:
		out := make([]kv, len(x.d.keys))
		for i, k := range x.d.keys {
			out[i] = kv{k: k, v: k}
		}
		return out, !keysOnly
	case kObj:
		if keysOnly {
			var out []kv
			for _, k := range x.o.fields.keys {
				if !strings.HasPrefix(k.s, "#") {
					out = append(out, kv{k: k, v: k})
				}
			}
			return out, true
		}
	case kNull, kUndef:
		if keysOnly {
			return nil, true
		}
	}
	return nil, false
}

// jsKeyOrder: key berupa indeks bilangan bulat diurutkan lebih dulu (aturan objek JS).
func jsKeyOrder(d *Dict) []Value {
	var ints, rest []Value
	for _, k := range d.keys {
		if n, err := strconv.ParseUint(k.s, 10, 32); err == nil && strconv.FormatUint(n, 10) == k.s {
			ints = append(ints, k)
		} else {
			rest = append(rest, k)
		}
	}
	sort.SliceStable(ints, func(i, j int) bool {
		a, _ := strconv.ParseUint(ints[i].s, 10, 32)
		b, _ := strconv.ParseUint(ints[j].s, 10, 32)
		return a < b
	})
	return append(ints, rest...)
}

// ---------- inspect (console.log) ----------

var jsIdentRe = regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z_$0-9]*$`)

// jsQuote mengikuti strEscape di util.inspect Node.
func jsQuote(s string) string {
	q := byte('\'')
	if strings.Contains(s, "'") {
		switch {
		case !strings.Contains(s, `"`):
			q = '"'
		case !strings.Contains(s, "`") && !strings.Contains(s, "${"):
			q = '`'
		}
	}
	var b strings.Builder
	b.WriteByte(q)
	for _, r := range s {
		switch {
		case r == rune(q) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\v':
			b.WriteString(`\v`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(q)
	return b.String()
}

type inspectCtx struct {
	it         *interp
	seen       map[int]bool
	depth      int // kedalaman maksimum (2)
	currentDep int
	indent     int
	panel      bool // tampilan panel: ringkas, satu baris
}

// jsInspect memformat nilai seperti util.inspect (console.log) Node.js.
func (it *interp) jsInspect(v Value, _ int, top bool, seen map[int]bool, _ int) string {
	ctx := &inspectCtx{it: it, seen: seen, depth: 2, panel: !top}
	return ctx.format(v, 0, top)
}

func (c *inspectCtx) format(v Value, recurse int, top bool) string {
	it := c.it
	switch v.k {
	case kStr:
		if top {
			return v.s
		}
		return jsQuote(v.s)
	case kFloat:
		if v.f == 0 && math.Signbit(v.f) {
			return "-0"
		}
		return jsNumber(v.f)
	case kInt:
		return strconv.FormatInt(v.i, 10)
	case kBig:
		return v.big.String() + "n"
	case kBool, kNull, kUndef:
		return it.sem.str(it, v)
	case kFunc:
		if v.fn.name == "" || v.fn.name == "(anonim)" {
			return "[Function (anonymous)]"
		}
		return "[Function: " + v.fn.name + "]"
	case kClass:
		s := "[class " + v.c.name
		if v.c.super != nil {
			s += " extends " + v.c.super.name
		}
		return s + "]"
	case kUnknown:
		return "?"
	}
	id := objectID(v)
	if c.seen[id] {
		return "[Circular *1]"
	}
	if recurse > c.depth {
		switch {
		case v.k == kList:
			return "[Array]"
		case v.k == kObj:
			return "[" + v.o.class.name + "]"
		}
		return "[Object]"
	}
	c.seen[id] = true
	defer delete(c.seen, id)
	c.currentDep = recurse
	var output []string
	braces := [2]string{"{", "}"}
	base := ""
	isArray := false
	switch v.k {
	case kList:
		isArray = true
		braces = [2]string{"[", "]"}
		holes := 0
		flush := func() {
			if holes > 0 {
				s := "s"
				if holes == 1 {
					s = ""
				}
				output = append(output, fmt.Sprintf("<%d empty item%s>", holes, s))
				holes = 0
			}
		}
		for i, e := range v.l.elems {
			if i >= 100 {
				flush()
				output = append(output, fmt.Sprintf("... %d more items", len(v.l.elems)-100))
				break
			}
			if e.k == kUndef && e.t == "hole" {
				holes++
				continue
			}
			flush()
			output = append(output, c.child(e, recurse))
		}
		flush()
	case kDict:
		switch v.d.kind {
		case "Map":
			base = fmt.Sprintf("Map(%d)", v.d.len())
			for i, k := range v.d.keys {
				output = append(output, c.child(k, recurse)+" => "+c.child(v.d.vals[i], recurse))
			}
		case "regex":
			return it.sem.str(it, v)
		case "promise": // seperti Node: Promise { <pending> } / Promise { 5 } / Promise { <rejected> err }
			state, _ := v.d.get(strV("state"))
			st, _ := v.d.get(strV("value"))
			switch state.s {
			case "pending":
				return "Promise { <pending> }"
			case "rejected":
				return "Promise { <rejected> " + c.child(st, recurse) + " }"
			}
			return "Promise { " + c.child(st, recurse) + " }"
		default:
			for _, k := range jsKeyOrder(v.d) {
				val, _ := v.d.get(k)
				output = append(output, jsKey(k.s)+": "+c.child(val, recurse))
			}
		}
	case kSet:
		base = fmt.Sprintf("Set(%d)", v.d.len())
		for _, k := range v.d.keys {
			output = append(output, c.child(k, recurse))
		}
	case kObj:
		if instanceOf(v.o.class, "Error") {
			return it.exceptionText(v)
		}
		base = v.o.class.name
		for i, k := range v.o.fields.keys {
			if strings.HasPrefix(k.s, "#") {
				continue
			}
			output = append(output, jsKey(k.s)+": "+c.child(v.o.fields.vals[i], recurse))
		}
	case kPtr:
		return c.format(v.p.load(), recurse, top)
	}
	if len(output) == 0 {
		if base != "" {
			return base + " " + braces[0] + braces[1]
		}
		return braces[0] + braces[1]
	}
	return c.reduce(output, base, braces, isArray, recurse, v)
}

func (c *inspectCtx) child(v Value, recurse int) string {
	c.indent += 2
	s := c.format(v, recurse+1, false)
	c.indent -= 2
	return s
}

func objectID(v Value) int {
	switch v.k {
	case kList:
		return v.l.id
	case kDict, kSet:
		return v.d.id + 1<<20
	case kObj:
		return v.o.id + 1<<21
	}
	return -1
}

func jsKey(k string) string {
	if jsIdentRe.MatchString(k) {
		return k
	}
	return jsQuote(k)
}

const breakLength = 80

// reduce mengikuti reduceToSingleString (compact = 3) dan groupArrayElements Node.js.
func (c *inspectCtx) reduce(output []string, base string, braces [2]string, isArray bool, recurse int, v Value) string {
	entries := len(output)
	if c.panel {
		s := strings.Join(output, ", ")
		if base != "" {
			base += " "
		}
		if isArray {
			return base + braces[0] + s + braces[1]
		}
		return base + braces[0] + s + braces[1]
	}
	if isArray && entries > 6 {
		output = groupArrayElements(c, output, v)
	}
	if c.currentDep-recurse < 3 && entries == len(output) {
		start := len(output) + c.indent + len(braces[0]) + len(base) + 10
		if isBelowBreakLength(output, start, base) {
			joined := strings.Join(output, ", ")
			if !strings.Contains(joined, "\n") {
				b := ""
				if base != "" {
					b = base + " "
				}
				return b + braces[0] + " " + joined + " " + braces[1]
			}
		}
	}
	ind := "\n" + strings.Repeat(" ", c.indent)
	b := ""
	if base != "" {
		b = base + " "
	}
	return b + braces[0] + ind + "  " + strings.Join(output, ","+ind+"  ") + ind + braces[1]
}

func isBelowBreakLength(output []string, start int, base string) bool {
	total := len(output) + start
	if total+len(output) > breakLength {
		return false
	}
	for _, o := range output {
		total += strLen(o)
		if total > breakLength {
			return false
		}
	}
	return base == "" || !strings.Contains(base, "\n")
}

func strLen(s string) int { return len(utf16.Encode([]rune(s))) }

func groupArrayElements(c *inspectCtx, output []string, v Value) []string {
	totalLength, maxLength := 0, 0
	outputLength := len(output)
	if v.k == kList && len(v.l.elems) > 100 {
		outputLength-- // entri "... more items"
	}
	const sep = 2
	dataLen := make([]int, outputLength)
	for i := 0; i < outputLength; i++ {
		l := strLen(output[i])
		dataLen[i] = l
		totalLength += l + sep
		if maxLength < l {
			maxLength = l
		}
	}
	actualMax := maxLength + sep
	if actualMax*3+c.indent < breakLength && (float64(totalLength)/float64(actualMax) > 5 || maxLength <= 6) {
		averageBias := math.Sqrt(float64(actualMax) - float64(totalLength)/float64(len(output)))
		biasedMax := math.Max(float64(actualMax)-3-averageBias, 1)
		columns := int(math.Min(math.Min(
			math.Round(math.Sqrt(2.5*biasedMax*float64(outputLength))/biasedMax),
			math.Floor(float64(breakLength-c.indent)/float64(actualMax))),
			math.Min(3*4, 15)))
		if columns <= 1 {
			return output
		}
		var maxLineLength []int
		for i := 0; i < columns; i++ {
			lineLength := 0
			for j := i; j < len(output); j += columns {
				if j < outputLength && dataLen[j] > lineLength {
					lineLength = dataLen[j]
				}
			}
			maxLineLength = append(maxLineLength, lineLength+sep)
		}
		padStart := true
		if v.k == kList {
			for i := 0; i < len(v.l.elems) && i < outputLength; i++ {
				if e := v.l.elems[i]; e.k != kFloat && e.k != kInt && e.k != kBig {
					padStart = false
					break
				}
			}
		}
		var tmp []string
		for i := 0; i < outputLength; i += columns {
			max := min(i+columns, outputLength)
			var b strings.Builder
			j := i
			for ; j < max-1; j++ {
				cell := output[j] + ", "
				pad := maxLineLength[j-i]
				if padStart {
					b.WriteString(strings.Repeat(" ", max0(pad-strLen(cell))) + cell)
				} else {
					b.WriteString(cell + strings.Repeat(" ", max0(pad-strLen(cell))))
				}
			}
			if padStart {
				pad := maxLineLength[j-i] - sep
				b.WriteString(strings.Repeat(" ", max0(pad-strLen(output[j]))) + output[j])
			} else {
				b.WriteString(output[j])
			}
			tmp = append(tmp, b.String())
		}
		if outputLength < len(output) {
			tmp = append(tmp, output[outputLength])
		}
		return tmp
	}
	return output
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// consoleLog memformat argumen console.log (termasuk %s %d %i %f %o %O %j %c).
func (it *interp) consoleLog(args []Value) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	rest := args
	if args[0].k == kStr && strings.Contains(args[0].s, "%") {
		f := args[0].s
		rest = args[1:]
		var b strings.Builder
		for i := 0; i < len(f); i++ {
			if f[i] != '%' || i+1 >= len(f) {
				b.WriteByte(f[i])
				continue
			}
			verb := f[i+1]
			if verb == '%' {
				b.WriteByte('%')
				i++
				continue
			}
			if !strings.ContainsRune("sdifjoOc", rune(verb)) || len(rest) == 0 {
				b.WriteByte(f[i])
				continue
			}
			a := rest[0]
			rest = rest[1:]
			i++
			switch verb {
			case 's':
				if a.k == kStr {
					b.WriteString(a.s)
				} else {
					b.WriteString(it.jsInspect(a, 0, false, map[int]bool{}, 0))
				}
			case 'd', 'i':
				n := it.jsToNumber(a)
				if verb == 'i' {
					n = math.Trunc(n)
				}
				b.WriteString(jsNumber(n))
			case 'f':
				b.WriteString(jsNumber(it.jsToNumber(a)))
			case 'j':
				b.WriteString(it.jsonStringify(a, ""))
			case 'o', 'O':
				b.WriteString(it.jsInspect(a, 0, false, map[int]bool{}, 0))
			case 'c':
			}
		}
		parts = append(parts, b.String())
	}
	for _, a := range rest {
		ctx := &inspectCtx{it: it, seen: map[int]bool{}, depth: 2}
		parts = append(parts, ctx.format(a, 0, true))
	}
	return strings.Join(parts, " ")
}

// ---------- JSON ----------

func (it *interp) jsonStringify(v Value, indent string) string {
	var write func(v Value, lvl int) (string, bool)
	write = func(v Value, lvl int) (string, bool) {
		nl, ind, ind2 := "", "", ""
		if indent != "" {
			nl = "\n"
			ind = strings.Repeat(indent, lvl+1)
			ind2 = strings.Repeat(indent, lvl)
		}
		colon := ":"
		if indent != "" {
			colon = ": "
		}
		switch v.k {
		case kStr:
			b, _ := json.Marshal(v.s)
			return string(b), true
		case kFloat:
			if math.IsNaN(v.f) || math.IsInf(v.f, 0) {
				return "null", true
			}
			return jsNumber(v.f), true
		case kInt:
			return strconv.FormatInt(v.i, 10), true
		case kBool:
			return it.sem.str(it, v), true
		case kNull:
			return "null", true
		case kList:
			if len(v.l.elems) == 0 {
				return "[]", true
			}
			var parts []string
			for _, e := range v.l.elems {
				s, ok := write(e, lvl+1)
				if !ok {
					s = "null"
				}
				parts = append(parts, ind+s)
			}
			return "[" + nl + strings.Join(parts, ","+nl) + nl + ind2 + "]", true
		case kDict, kObj:
			d := v.d
			if v.k == kObj {
				if m := findMethod(v.o.class, "toJSON"); m != nil {
					return write(it.callFunc(&Func{decl: m, env: v.o.class.env}, v, argList{}, m), lvl)
				}
				d = v.o.fields
			}
			if v.k == kDict && (d.kind == "Map" || d.kind == "regex") {
				return "{}", true
			}
			var parts []string
			for _, k := range jsKeyOrder(d) {
				val, _ := d.get(k)
				s, ok := write(val, lvl+1)
				if !ok {
					continue
				}
				kb, _ := json.Marshal(k.s)
				parts = append(parts, ind+string(kb)+colon+s)
			}
			if len(parts) == 0 {
				return "{}", true
			}
			return "{" + nl + strings.Join(parts, ","+nl) + nl + ind2 + "}", true
		case kSet:
			return "{}", true
		}
		return "", false
	}
	s, ok := write(v, 0)
	if !ok {
		return "undefined"
	}
	return s
}

func (it *interp) jsonParse(s string, at ir.Node) Value {
	var raw any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		it.throw(at, "SyntaxError", "Unexpected token in JSON: "+err.Error())
	}
	var conv func(x any) Value
	conv = func(x any) Value {
		switch t := x.(type) {
		case nil:
			return null()
		case bool:
			return boolV(t)
		case json.Number:
			f, _ := t.Float64()
			return num(f)
		case string:
			return strV(t)
		case []any:
			var out []Value
			for _, e := range t {
				out = append(out, conv(e))
			}
			return it.newList("array", out)
		case map[string]any:
			d := it.newDict("object")
			// urutan key mengikuti sumber
			keys := jsonKeyOrder(s, t)
			for _, k := range keys {
				d.set(strV(k), conv(t[k]))
			}
			return Value{k: kDict, d: d}
		}
		return undef()
	}
	return conv(raw)
}

func jsonKeyOrder(src string, m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.Index(src, `"`+keys[i]+`"`) < strings.Index(src, `"`+keys[j]+`"`)
	})
	return keys
}

// ---------- angka ----------

// jsToFixed mengikuti Number.prototype.toFixed (pembulatan setengah ke atas pada nilai desimal persis).
func jsToFixed(f float64, digits int) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.Abs(f) >= 1e21 {
		return jsNumber(f)
	}
	exact := new(big.Float).SetPrec(2000).SetFloat64(f).Text('f', 1100)
	neg := strings.HasPrefix(exact, "-")
	exact = strings.TrimPrefix(exact, "-")
	intPart, frac, _ := strings.Cut(exact, ".")
	frac += strings.Repeat("0", digits+1)
	keep := intPart + frac[:digits]
	roundUp := frac[digits] >= '5'
	n := new(big.Int)
	n.SetString(keep, 10)
	if roundUp {
		n.Add(n, big.NewInt(1))
	}
	s := n.String()
	for len(s) <= digits {
		s = "0" + s
	}
	out := s
	if digits > 0 {
		out = s[:len(s)-digits] + "." + s[len(s)-digits:]
	}
	if neg && strings.Trim(out, "0.") != "" {
		out = "-" + out
	}
	return out
}

// jsRadix mengubah angka ke basis lain (Number.prototype.toString(radix)).
func jsRadix(f float64, radix int) string {
	if isIntegral(f) && math.Abs(f) < 1e18 {
		return strconv.FormatInt(int64(f), radix)
	}
	return jsNumber(f)
}

// ---------- regex ----------

func (it *interp) jsRegex(lit string) Value {
	i := strings.LastIndex(lit, "/")
	src, flags := strings.TrimPrefix(lit, "/"), ""
	if i > 0 {
		src, flags = lit[1:i], lit[i+1:]
	}
	d := it.newDict("regex")
	d.set(strV("source"), strV(src))
	d.set(strV("flags"), strV(flags))
	d.set(strV("global"), boolV(strings.Contains(flags, "g")))
	return Value{k: kDict, d: d}
}

func (it *interp) goRegex(v Value, at ir.Node) (*regexp.Regexp, bool) {
	src, _ := v.d.get(strV("source"))
	flags, _ := v.d.get(strV("flags"))
	pat := src.s
	prefix := ""
	if strings.Contains(flags.s, "i") {
		prefix += "i"
	}
	if strings.Contains(flags.s, "m") {
		prefix += "m"
	}
	if strings.Contains(flags.s, "s") {
		prefix += "s"
	}
	if prefix != "" {
		pat = "(?" + prefix + ")" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		it.structural("regex " + src.s + " belum didukung simulator")
	}
	return re, strings.Contains(flags.s, "g")
}
