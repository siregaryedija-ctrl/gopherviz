package irsim

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopherviz/internal/ir"
	"gopherviz/internal/trace"
)

// pySem: semantik Python 3.12. Bilangan bulat tak terbatas (int64, beralih ke big.Int bila
// meluap), pembagian "/" selalu float, perbandingan berantai sudah diurai parser.
type pySem struct {
	globals map[string]Value
	modules map[string]Value
}

func init() {
	pyFormatImpl = func(it *interp, v Value, spec string, at ir.Node) string { return it.pyFormatSpec(v, spec, at) }
	pyMatchImpl = func(it *interp, n *ir.Switch, e *Env, focus bool) flow { return it.execPyMatch(n, e, focus) }
}

func (s *pySem) setup(it *interp) {
	s.globals = map[string]Value{}
	s.modules = map[string]Value{}
	it.setupPython(s)
}

// ---------- angka ----------

func pyInt(i int64) Value { return Value{k: kInt, i: i} }

func bigV(b *big.Int) Value {
	if b.IsInt64() {
		return pyInt(b.Int64())
	}
	return Value{k: kBig, big: b}
}

func toBig(v Value) *big.Int {
	switch v.k {
	case kBig:
		return v.big
	case kBool:
		if v.b {
			return big.NewInt(1)
		}
		return big.NewInt(0)
	}
	return big.NewInt(v.i)
}

// pyIntLike: int, big int, atau bool (bool adalah turunan int di Python).
func pyIntLike(v Value) bool { return v.k == kInt || v.k == kBig || v.k == kBool }

func (s *pySem) lit(it *interp, l *ir.Lit) Value {
	switch l.Kind {
	case ir.LitInt:
		v := strings.ReplaceAll(strings.ToLower(l.Value), "_", "")
		base := 10
		switch {
		case strings.HasPrefix(v, "0x"):
			base, v = 16, v[2:]
		case strings.HasPrefix(v, "0o"):
			base, v = 8, v[2:]
		case strings.HasPrefix(v, "0b"):
			base, v = 2, v[2:]
		}
		b, ok := new(big.Int).SetString(v, base)
		if !ok {
			return unknown("number")
		}
		return bigV(b)
	case ir.LitFloat:
		if strings.HasSuffix(strings.ToLower(l.Value), "j") {
			return unknown("complex numbers are not supported yet")
		}
		f, _ := strconv.ParseFloat(strings.ReplaceAll(l.Value, "_", ""), 64)
		return floatV(f)
	case ir.LitString:
		return strV(l.Value)
	case ir.LitBool:
		return boolV(l.Value == "true")
	case ir.LitNull:
		return null()
	}
	return unknown("literal")
}

func (s *pySem) truthy(v Value) bool {
	switch v.k {
	case kBool:
		return v.b
	case kInt:
		return v.i != 0
	case kBig:
		return v.big.Sign() != 0
	case kFloat:
		return v.f != 0
	case kStr:
		return v.s != ""
	case kNull:
		return false
	case kList:
		return len(v.l.elems) > 0
	case kDict, kSet:
		return v.d.len() > 0
	}
	return true
}

// ---------- tampilan ----------

func pyQuote(s string) string {
	q := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = '"'
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
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case !unicode.IsPrint(r) && r > 0x7f:
			if r > 0xffff {
				fmt.Fprintf(&b, `\U%08x`, r)
			} else {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(q)
	return b.String()
}

func (s *pySem) show(it *interp, v Value) string { return it.pyRepr(v, map[int]bool{}) }

func (s *pySem) str(it *interp, v Value) string {
	switch v.k {
	case kStr:
		return v.s
	case kObj:
		if m := findMethod(v.o.class, "__str__"); m != nil {
			return it.sem.str(it, it.callFunc(&Func{name: "__str__", decl: m, env: v.o.class.env, class: it.classOf[m]}, v, argList{}, m))
		}
		if instanceOf(v.o.class, "BaseException") {
			return it.pyExcStr(v)
		}
	}
	return it.pyRepr(v, map[int]bool{})
}

func (it *interp) pyExcStr(v Value) string {
	args, _ := v.o.fields.get(strV("args"))
	if args.k != kList {
		return ""
	}
	switch len(args.l.elems) {
	case 0:
		return ""
	case 1:
		if instanceOf(v.o.class, "KeyError") {
			return it.pyRepr(args.l.elems[0], map[int]bool{})
		}
		return it.sem.str(it, args.l.elems[0])
	}
	return it.pyRepr(args, map[int]bool{})
}

func (it *interp) pyRepr(v Value, seen map[int]bool) string {
	switch v.k {
	case kInt:
		if v.t == "bool" {
			return map[bool]string{true: "True", false: "False"}[v.i != 0]
		}
		return strconv.FormatInt(v.i, 10)
	case kBig:
		return v.big.String()
	case kFloat:
		return pyFloat(v.f)
	case kStr:
		return pyQuote(v.s)
	case kBool:
		if v.b {
			return "True"
		}
		return "False"
	case kNull:
		return "None"
	case kUndef:
		return "None"
	case kUnknown:
		return "?"
	case kFunc:
		if v.fn.builtin != nil {
			return "<built-in function " + v.fn.name + ">"
		}
		if v.fn.this != nil && v.fn.this.k == kObj {
			return "<bound method " + v.fn.name + " of " + it.pyRepr(*v.fn.this, seen) + ">"
		}
		return "<function " + v.fn.name + ">"
	case kClass:
		if v.c.native != "" {
			return "<class '" + v.c.name + "'>"
		}
		return "<class '__main__." + v.c.name + "'>"
	}
	id := objectID(v)
	if seen[id] {
		switch v.k {
		case kList:
			return "[...]"
		}
		return "{...}"
	}
	seen[id] = true
	defer delete(seen, id)
	switch v.k {
	case kList:
		parts := make([]string, len(v.l.elems))
		for i, e := range v.l.elems {
			parts[i] = it.pyRepr(e, seen)
		}
		switch v.l.kind {
		case "deque":
			return "deque([" + strings.Join(parts, ", ") + "])"
		case "iter":
			return fmt.Sprintf("<%s object at 0x7f%010x>", v.t, 0x1000*v.l.id)
		case "tuple":
			if len(parts) == 1 {
				return "(" + parts[0] + ",)"
			}
			return "(" + strings.Join(parts, ", ") + ")"
		case "range":
			return v.t
		case "dict_keys", "dict_values", "dict_items":
			return v.l.kind + "([" + strings.Join(parts, ", ") + "])"
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case kDict:
		if v.d.kind == "module" {
			return "<module '" + v.t + "'>"
		}
		parts := make([]string, len(v.d.keys))
		for i, k := range v.d.keys {
			parts[i] = it.pyRepr(k, seen) + ": " + it.pyRepr(v.d.vals[i], seen)
		}
		body := "{" + strings.Join(parts, ", ") + "}"
		switch v.d.kind {
		case "Counter":
			return "Counter(" + body + ")"
		case "defaultdict":
			return "defaultdict(" + it.pyRepr(*v.d.factory, seen) + ", " + body + ")"
		}
		return body
	case kSet:
		if v.d.len() == 0 {
			return "set()"
		}
		var parts []string
		for _, k := range pySetOrder(v.d) {
			parts = append(parts, it.pyRepr(k, seen))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case kObj:
		if m := findMethod(v.o.class, "__repr__"); m != nil {
			return it.sem.str(it, it.callFunc(&Func{name: "__repr__", decl: m, env: v.o.class.env, class: it.classOf[m]}, v, argList{}, m))
		}
		if instanceOf(v.o.class, "BaseException") {
			args, _ := v.o.fields.get(strV("args"))
			inner := ""
			if args.k == kList {
				parts := make([]string, len(args.l.elems))
				for i, a := range args.l.elems {
					parts[i] = it.pyRepr(a, seen)
				}
				inner = strings.Join(parts, ", ")
			}
			return v.o.class.name + "(" + inner + ")"
		}
		return fmt.Sprintf("<__main__.%s object at 0x7f%010x>", v.o.class.name, 0x1000*v.o.id)
	}
	return "?"
}

// pySetOrder meniru urutan iterasi set CPython untuk bilangan bulat (hash = nilainya);
// selain itu memakai urutan sisip.
func pySetOrder(d *Dict) []Value {
	for _, k := range d.keys {
		if k.k != kInt && k.k != kBool {
			return d.keys
		}
	}
	const linearProbes = 9
	size := uint64(8)
	type slot struct {
		used bool
		key  Value
		hash uint64
	}
	table := make([]slot, size)
	fill := 0
	hashOf := func(v Value) uint64 {
		h := v.i
		if v.k == kBool && v.b {
			h = 1
		}
		if h == -1 {
			h = -2
		}
		return uint64(h)
	}
	insert := func(tbl []slot, key Value, h uint64) {
		mask := uint64(len(tbl) - 1)
		perturb := h
		i := h & mask
		for {
			if !tbl[i].used {
				tbl[i] = slot{true, key, h}
				return
			}
			if i+linearProbes <= mask {
				for j := uint64(1); j <= linearProbes; j++ {
					if !tbl[i+j].used {
						tbl[i+j] = slot{true, key, h}
						return
					}
				}
			}
			perturb >>= 5
			i = (i*5 + 1 + perturb) & mask
		}
	}
	for _, k := range d.keys {
		insert(table, k, hashOf(k))
		fill++
		mask := uint64(len(table) - 1)
		if uint64(fill)*5 >= mask*3 {
			minused := fill * 4
			if fill > 50000 {
				minused = fill * 2
			}
			ns := uint64(8)
			for ns <= uint64(minused) {
				ns <<= 1
			}
			nt := make([]slot, ns)
			for _, s := range table {
				if s.used {
					insert(nt, s.key, s.hash)
				}
			}
			table = nt
		}
	}
	out := make([]Value, 0, len(d.keys))
	for _, s := range table {
		if s.used {
			out = append(out, s.key)
		}
	}
	return out
}

// ---------- operator ----------

func (it *interp) pyTypeErr(at ir.Node, op string, a, b Value) {
	it.throw(at, "TypeError", fmt.Sprintf("unsupported operand type(s) for %s: '%s' and '%s'", op, it.pyTypeName(a), it.pyTypeName(b)))
}

func (s *pySem) binary(it *interp, op string, a, b Value, at ir.Node) Value {
	// operator overloading lewat method dunder
	if a.k == kObj || b.k == kObj {
		if v, ok := it.pyDunder(op, a, b, at); ok {
			return v
		}
	}
	switch op {
	case "==":
		return boolV(it.pyEq(a, b))
	case "!=":
		return boolV(!it.pyEq(a, b))
	case "is":
		return boolV(pyIs(a, b))
	case "is not":
		return boolV(!pyIs(a, b))
	case "in", "not in":
		r := it.pyContains(b, a, at)
		if op == "not in" {
			r = !r
		}
		return boolV(r)
	case "<", ">", "<=", ">=":
		c, ok := it.pyCompare(a, b)
		if !ok {
			it.throw(at, "TypeError", fmt.Sprintf("'%s' not supported between instances of '%s' and '%s'", op, it.pyTypeName(a), it.pyTypeName(b)))
		}
		return boolV(map[string]bool{"<": c < 0, ">": c > 0, "<=": c <= 0, ">=": c >= 0}[op])
	}
	// angka
	if pyNumeric(a) && pyNumeric(b) {
		return it.pyArith(op, a, b, at)
	}
	switch {
	case op == "+" && a.k == kStr && b.k == kStr:
		return strV(a.s + b.s)
	case op == "+" && a.k == kStr:
		it.throw(at, "TypeError", fmt.Sprintf("can only concatenate str (not \"%s\") to str", it.pyTypeName(b)))
	case op == "+" && a.k == kList && b.k == kList && a.l.kind == b.l.kind:
		return it.newList(a.l.kind, append(append([]Value(nil), a.l.elems...), b.l.elems...))
	case op == "*" && (a.k == kStr && pyIntLike(b) || pyIntLike(a) && b.k == kStr):
		str, n := a, b
		if a.k != kStr {
			str, n = b, a
		}
		c := int(toF(n))
		if c < 0 {
			c = 0
		}
		if len(str.s)*c > 10_000_000 {
			it.structural("string too large")
		}
		return strV(strings.Repeat(str.s, c))
	case op == "*" && (a.k == kList && pyIntLike(b) || pyIntLike(a) && b.k == kList):
		lst, n := a, b
		if a.k != kList {
			lst, n = b, a
		}
		c := int(toF(n))
		var out []Value
		for i := 0; i < c && len(out) < 1_000_000; i++ {
			out = append(out, lst.l.elems...)
		}
		return it.newList(lst.l.kind, out)
	case op == "%" && a.k == kStr:
		return strV(it.pyPercent(a.s, b, at))
	case a.k == kSet && b.k == kSet && strings.Contains("|&-^", op):
		return it.pySetOp(op, a, b)
	case op == "|" && a.k == kDict && b.k == kDict:
		d := it.cloneDict(a.d)
		for i, k := range b.d.keys {
			d.set(k, b.d.vals[i])
		}
		return Value{k: kDict, d: d}
	}
	it.pyTypeErr(at, op, a, b)
	return Value{}
}

func pyNumeric(v Value) bool { return v.k == kInt || v.k == kBig || v.k == kFloat || v.k == kBool }

func pyIs(a, b Value) bool {
	if a.k != b.k {
		return false
	}
	switch a.k {
	case kNull, kUndef:
		return true
	case kBool:
		return a.b == b.b
	case kInt:
		return a.i == b.i && a.i >= -5 && a.i <= 256
	case kStr:
		return a.s == b.s
	}
	return jsStrictEq(a, b)
}

func (it *interp) pyArith(op string, a, b Value, at ir.Node) Value {
	if a.k == kFloat || b.k == kFloat {
		x, y := toF(a), toF(b)
		switch op {
		case "+":
			return floatV(x + y)
		case "-":
			return floatV(x - y)
		case "*":
			return floatV(x * y)
		case "/":
			if y == 0 {
				it.throw(at, "ZeroDivisionError", "float division by zero")
			}
			return floatV(x / y)
		case "//":
			if y == 0 {
				it.throw(at, "ZeroDivisionError", "float floor division by zero")
			}
			return floatV(math.Floor(x / y))
		case "%":
			if y == 0 {
				it.throw(at, "ZeroDivisionError", "float modulo")
			}
			r := math.Mod(x, y)
			if r != 0 && (r < 0) != (y < 0) {
				r += y
			}
			return floatV(r)
		case "**":
			if x == 0 && y < 0 {
				it.throw(at, "ZeroDivisionError", "0.0 cannot be raised to a negative power")
			}
			return floatV(math.Pow(x, y))
		}
		it.pyTypeErr(at, op, a, b)
	}
	x, y := toBig(a), toBig(b)
	r := new(big.Int)
	switch op {
	case "+":
		r.Add(x, y)
	case "-":
		r.Sub(x, y)
	case "*":
		r.Mul(x, y)
	case "/":
		if y.Sign() == 0 {
			it.throw(at, "ZeroDivisionError", "division by zero")
		}
		fx, _ := new(big.Float).SetInt(x).Float64()
		fy, _ := new(big.Float).SetInt(y).Float64()
		if x.IsInt64() && y.IsInt64() && math.Abs(fx) < 1<<53 && math.Abs(fy) < 1<<53 {
			return floatV(fx / fy)
		}
		q, _ := new(big.Rat).SetFrac(x, y).Float64()
		return floatV(q)
	case "//", "%":
		if y.Sign() == 0 {
			if op == "%" {
				it.throw(at, "ZeroDivisionError", "integer modulo by zero")
			}
			it.throw(at, "ZeroDivisionError", "integer division or modulo by zero")
		}
		q, m := new(big.Int), new(big.Int)
		q.DivMod(x, y, m) // Euclid: m >= 0; Python membulatkan ke bawah (floor)
		if m.Sign() != 0 && y.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
			m.Add(m, y)
		}
		if op == "//" {
			return bigV(q)
		}
		return bigV(m)
	case "**":
		if y.Sign() < 0 {
			fx, _ := new(big.Float).SetInt(x).Float64()
			fy, _ := new(big.Float).SetInt(y).Float64()
			if fx == 0 {
				it.throw(at, "ZeroDivisionError", "0.0 cannot be raised to a negative power")
			}
			return floatV(math.Pow(fx, fy))
		}
		if y.BitLen() > 20 || x.BitLen()*int(y.Int64()) > 100_000 {
			it.structural("power result too large to simulate")
		}
		r.Exp(x, y, nil)
	case "&":
		r.And(x, y)
	case "|":
		r.Or(x, y)
	case "^":
		r.Xor(x, y)
	case "<<":
		if y.Sign() < 0 {
			it.throw(at, "ValueError", "negative shift count")
		}
		r.Lsh(x, uint(y.Int64()))
	case ">>":
		if y.Sign() < 0 {
			it.throw(at, "ValueError", "negative shift count")
		}
		r.Rsh(x, uint(y.Int64()))
	default:
		it.pyTypeErr(at, op, a, b)
	}
	return bigV(r)
}

func (s *pySem) unary(it *interp, op string, v Value, at ir.Node) Value {
	switch op {
	case "not", "!":
		return boolV(!s.truthy(v))
	case "-":
		switch v.k {
		case kFloat:
			return floatV(-v.f)
		case kInt, kBig, kBool:
			return bigV(new(big.Int).Neg(toBig(v)))
		}
		if r, ok := it.pyDunderUnary("__neg__", v, at); ok {
			return r
		}
		it.throw(at, "TypeError", "bad operand type for unary -: '"+it.pyTypeName(v)+"'")
	case "+":
		if pyNumeric(v) {
			if v.k == kBool {
				return pyInt(map[bool]int64{true: 1}[v.b])
			}
			return v
		}
		it.throw(at, "TypeError", "bad operand type for unary +: '"+it.pyTypeName(v)+"'")
	case "~":
		if pyIntLike(v) {
			return bigV(new(big.Int).Not(toBig(v)))
		}
		it.throw(at, "TypeError", "bad operand type for unary ~: '"+it.pyTypeName(v)+"'")
	}
	it.structural("operator " + op + " is not simulated yet")
	return Value{}
}

func (it *interp) pyEq(a, b Value) bool {
	if pyNumeric(a) && pyNumeric(b) {
		if a.k == kFloat || b.k == kFloat {
			return toF(a) == toF(b)
		}
		return toBig(a).Cmp(toBig(b)) == 0
	}
	if a.k == kObj {
		if m := findMethod(a.o.class, "__eq__"); m != nil {
			return it.sem.truthy(it.callFunc(&Func{decl: m, env: a.o.class.env, class: it.classOf[m]}, a, argList{pos: []Value{b}}, m))
		}
		return b.k == kObj && a.o == b.o
	}
	if a.k != b.k {
		return a.k == kNull && b.k == kUndef || a.k == kUndef && b.k == kNull
	}
	switch a.k {
	case kStr:
		return a.s == b.s
	case kNull, kUndef:
		return true
	case kList:
		if a.l.kind != b.l.kind && (a.l.kind == "tuple" || b.l.kind == "tuple") || len(a.l.elems) != len(b.l.elems) {
			return false
		}
		for i := range a.l.elems {
			if !it.pyEq(a.l.elems[i], b.l.elems[i]) {
				return false
			}
		}
		return true
	case kDict:
		if a.d.len() != b.d.len() {
			return false
		}
		for i, k := range a.d.keys {
			v, ok := b.d.get(k)
			if !ok || !it.pyEq(a.d.vals[i], v) {
				return false
			}
		}
		return true
	case kSet:
		if a.d.len() != b.d.len() {
			return false
		}
		for _, k := range a.d.keys {
			if !b.d.has(k) {
				return false
			}
		}
		return true
	}
	return jsStrictEq(a, b)
}

// pyCompare: -1/0/1; ok=false bila tipe tidak bisa dibandingkan.
func (it *interp) pyCompare(a, b Value) (int, bool) {
	switch {
	case pyNumeric(a) && pyNumeric(b):
		if a.k == kFloat || b.k == kFloat {
			x, y := toF(a), toF(b)
			switch {
			case x < y:
				return -1, true
			case x > y:
				return 1, true
			}
			return 0, true
		}
		return toBig(a).Cmp(toBig(b)), true
	case a.k == kStr && b.k == kStr:
		return strings.Compare(a.s, b.s), true
	case a.k == kList && b.k == kList && (a.l.kind == "tuple") == (b.l.kind == "tuple"):
		for i := 0; i < len(a.l.elems) && i < len(b.l.elems); i++ {
			if it.pyEq(a.l.elems[i], b.l.elems[i]) {
				continue
			}
			return it.pyCompare(a.l.elems[i], b.l.elems[i])
		}
		return len(a.l.elems) - len(b.l.elems), true
	case a.k == kSet && b.k == kSet:
		sub := true
		for _, k := range a.d.keys {
			if !b.d.has(k) {
				sub = false
			}
		}
		if sub && a.d.len() < b.d.len() {
			return -1, true
		}
		if sub {
			return 0, true
		}
		return 1, true
	case a.k == kObj:
		if m := findMethod(a.o.class, "__lt__"); m != nil {
			if it.sem.truthy(it.callFunc(&Func{decl: m, env: a.o.class.env, class: it.classOf[m]}, a, argList{pos: []Value{b}}, m)) {
				return -1, true
			}
			if b.k == kObj {
				if m2 := findMethod(b.o.class, "__lt__"); m2 != nil &&
					it.sem.truthy(it.callFunc(&Func{decl: m2, env: b.o.class.env, class: it.classOf[m2]}, b, argList{pos: []Value{a}}, m2)) {
					return 1, true
				}
			}
			return 0, true
		}
	}
	return 0, false
}

func (it *interp) pyContains(coll, x Value, at ir.Node) bool {
	switch coll.k {
	case kStr:
		if x.k != kStr {
			it.throw(at, "TypeError", "'in <string>' requires string as left operand, not "+it.pyTypeName(x))
		}
		return strings.Contains(coll.s, x.s)
	case kList:
		for _, e := range coll.l.elems {
			if it.pyEq(e, x) {
				return true
			}
		}
		return false
	case kDict, kSet:
		return coll.d.has(x)
	case kObj:
		if m := findMethod(coll.o.class, "__contains__"); m != nil {
			return it.sem.truthy(it.callFunc(&Func{decl: m, env: coll.o.class.env}, coll, argList{pos: []Value{x}}, m))
		}
	}
	it.throw(at, "TypeError", fmt.Sprintf("argument of type '%s' is not iterable", it.pyTypeName(coll)))
	return false
}

func (it *interp) pySetOp(op string, a, b Value) Value {
	d := it.newDict("set")
	switch op {
	case "|":
		for _, k := range a.d.keys {
			d.set(k, k)
		}
		for _, k := range b.d.keys {
			d.set(k, k)
		}
	case "&":
		for _, k := range a.d.keys {
			if b.d.has(k) {
				d.set(k, k)
			}
		}
	case "-":
		for _, k := range a.d.keys {
			if !b.d.has(k) {
				d.set(k, k)
			}
		}
	case "^":
		for _, k := range a.d.keys {
			if !b.d.has(k) {
				d.set(k, k)
			}
		}
		for _, k := range b.d.keys {
			if !a.d.has(k) {
				d.set(k, k)
			}
		}
	}
	return Value{k: kSet, d: d}
}

var pyDunders = map[string][2]string{
	"+": {"__add__", "__radd__"}, "-": {"__sub__", "__rsub__"}, "*": {"__mul__", "__rmul__"},
	"/": {"__truediv__", "__rtruediv__"}, "//": {"__floordiv__", "__rfloordiv__"}, "%": {"__mod__", "__rmod__"},
	"**": {"__pow__", "__rpow__"}, "<": {"__lt__", "__gt__"}, ">": {"__gt__", "__lt__"},
	"<=": {"__le__", "__ge__"}, ">=": {"__ge__", "__le__"},
}

func (it *interp) pyDunder(op string, a, b Value, at ir.Node) (Value, bool) {
	names, ok := pyDunders[op]
	if !ok {
		return Value{}, false
	}
	if a.k == kObj {
		if m := findMethod(a.o.class, names[0]); m != nil {
			return it.callFunc(&Func{name: names[0], decl: m, env: a.o.class.env, class: it.classOf[m]}, a, argList{pos: []Value{b}}, at), true
		}
	}
	if b.k == kObj {
		if m := findMethod(b.o.class, names[1]); m != nil {
			return it.callFunc(&Func{name: names[1], decl: m, env: b.o.class.env, class: it.classOf[m]}, b, argList{pos: []Value{a}}, at), true
		}
	}
	return Value{}, false
}

func (it *interp) pyDunderUnary(name string, v Value, at ir.Node) (Value, bool) {
	if v.k == kObj {
		if m := findMethod(v.o.class, name); m != nil {
			return it.callFunc(&Func{name: name, decl: m, env: v.o.class.env}, v, argList{}, at), true
		}
	}
	return Value{}, false
}

func (s *pySem) caseEqual(it *interp, a, b Value) bool { return it.pyEq(a, b) }

func (s *pySem) zero(typ string) Value { return null() }

func (s *pySem) convert(it *interp, v Value, typ string) Value { return v }

func (s *pySem) newError(it *interp, class, msg string) Value {
	c := it.findClass(class, nil)
	if c == nil {
		c = it.findClass("Exception", nil)
	}
	var args []Value
	if msg != "" || class == "KeyError" {
		args = []Value{strV(msg)}
	}
	if class == "KeyError" && strings.HasPrefix(msg, "'") {
		args = []Value{strV(strings.Trim(msg, "'"))}
	}
	return it.construct(c, args, nil)
}

// ---------- iterasi ----------

func (s *pySem) iterate(it *interp, x Value, fe *ir.ForEach) ([]kv, bool) {
	switch x.k {
	case kList:
		out := make([]kv, len(x.l.elems))
		for i, e := range x.l.elems {
			out[i] = kv{k: pyInt(int64(i)), v: e}
		}
		return out, true
	case kStr:
		var out []kv
		i := 0
		for _, r := range x.s {
			out = append(out, kv{k: pyInt(int64(i)), v: strV(string(r))})
			i++
		}
		return out, true
	case kDict:
		out := make([]kv, len(x.d.keys))
		for i, k := range x.d.keys {
			out[i] = kv{k: pyInt(int64(i)), v: k}
		}
		return out, true
	case kSet:
		keys := pySetOrder(x.d)
		out := make([]kv, len(keys))
		for i, k := range keys {
			out[i] = kv{k: pyInt(int64(i)), v: k}
		}
		return out, true
	case kObj:
		if m := findMethod(x.o.class, "__iter__"); m != nil {
			r := it.callFunc(&Func{decl: m, env: x.o.class.env}, x, argList{}, m)
			if r.k != kObj {
				return s.iterate(it, r, fe)
			}
		}
		it.structural("custom iterators (__iter__/__next__) are not simulated yet")
	}
	return nil, false
}

// ---------- format ----------

// pyFormatSpec menerapkan "!r:.2f" (f-string / format()).
func (it *interp) pyFormatSpec(v Value, spec string, at ir.Node) string {
	if strings.HasPrefix(spec, "!") {
		conv := spec[1:2]
		spec = strings.TrimPrefix(spec[2:], ":")
		switch conv {
		case "r", "a":
			v = strV(it.pyRepr(v, map[int]bool{}))
		case "s":
			v = strV(it.sem.str(it, v))
		}
	} else {
		spec = strings.TrimPrefix(spec, ":")
	}
	return it.pyFormatValue(v, spec, at)
}

// pyFormatValue: mini-bahasa format Python [[fill]align][sign][#][0][width][,_][.prec][type].
func (it *interp) pyFormatValue(v Value, spec string, at ir.Node) string {
	if v.k == kObj {
		if m := findMethod(v.o.class, "__format__"); m != nil {
			return it.sem.str(it, it.callFunc(&Func{decl: m, env: v.o.class.env}, v, argList{pos: []Value{strV(spec)}}, at))
		}
	}
	if spec == "" {
		return it.sem.str(it, v)
	}
	fill, align := " ", byte(0)
	rs := []rune(spec)
	i := 0
	if len(rs) >= 2 && strings.ContainsRune("<>^=", rs[1]) {
		fill, align = string(rs[0]), byte(rs[1])
		i = 2
	} else if len(rs) >= 1 && strings.ContainsRune("<>^=", rs[0]) {
		align = byte(rs[0])
		i = 1
	}
	rest := string(rs[i:])
	sign := byte('-')
	if rest != "" && strings.ContainsRune("+- ", rune(rest[0])) {
		sign = rest[0]
		rest = rest[1:]
	}
	alt := false
	if strings.HasPrefix(rest, "#") {
		alt = true
		rest = rest[1:]
	}
	if strings.HasPrefix(rest, "0") && align == 0 {
		fill, align = "0", '='
		rest = rest[1:]
	}
	width := 0
	for rest != "" && rest[0] >= '0' && rest[0] <= '9' {
		width = width*10 + int(rest[0]-'0')
		rest = rest[1:]
	}
	group := ""
	if rest != "" && (rest[0] == ',' || rest[0] == '_') {
		group = rest[:1]
		rest = rest[1:]
	}
	prec := -1
	if strings.HasPrefix(rest, ".") {
		rest = rest[1:]
		prec = 0
		for rest != "" && rest[0] >= '0' && rest[0] <= '9' {
			prec = prec*10 + int(rest[0]-'0')
			rest = rest[1:]
		}
	}
	typ := byte(0)
	if rest != "" {
		typ = rest[0]
	}
	var body string
	neg := false
	switch {
	case v.k == kStr:
		body = v.s
		if prec >= 0 && prec < utf8.RuneCountInString(body) {
			body = string([]rune(body)[:prec])
		}
		if align == 0 {
			align = '<'
		}
	case pyNumeric(v):
		if v.k == kBool {
			v = pyInt(map[bool]int64{true: 1}[v.b])
		}
		f := toF(v)
		neg = f < 0 || v.k == kBig && v.big.Sign() < 0
		isInt := v.k != kFloat
		if typ == 0 && !isInt {
			if prec >= 0 {
				typ = 'g'
			}
		}
		switch typ {
		case 'd', 'n':
			if !isInt {
				it.throw(at, "ValueError", "Unknown format code 'd' for object of type 'float'")
			}
			body = new(big.Int).Abs(toBig(v)).String()
		case 'x', 'X', 'o', 'b':
			base := map[byte]int{'x': 16, 'X': 16, 'o': 8, 'b': 2}[typ]
			body = new(big.Int).Abs(toBig(v)).Text(base)
			if typ == 'X' {
				body = strings.ToUpper(body)
			}
			if alt {
				body = "0" + map[byte]string{'x': "x", 'X': "X", 'o': "o", 'b': "b"}[typ] + body
			}
		case 'c':
			body = string(rune(toF(v)))
		case 'f', 'F', '%':
			p := prec
			if p < 0 {
				p = 6
			}
			x := math.Abs(f)
			if typ == '%' {
				x *= 100
			}
			body = strconv.FormatFloat(x, 'f', p, 64)
			if typ == '%' {
				body += "%"
			}
		case 'e', 'E':
			p := prec
			if p < 0 {
				p = 6
			}
			body = strconv.FormatFloat(math.Abs(f), 'e', p, 64)
			if typ == 'E' {
				body = strings.ToUpper(body)
			}
		case 'g', 'G':
			p := prec
			if p < 0 {
				p = 6
			}
			if p == 0 {
				p = 1
			}
			body = strconv.FormatFloat(math.Abs(f), 'g', p, 64)
			if alt && !strings.Contains(body, ".") {
				body += "."
			}
		default:
			if isInt {
				body = new(big.Int).Abs(toBig(v)).String()
			} else {
				body = pyFloat(math.Abs(f))
			}
		}
		if group != "" {
			intPart, frac, hasFrac := strings.Cut(body, ".")
			if typ == 'x' || typ == 'X' || typ == 'o' || typ == 'b' {
				hasFrac = false
			}
			grouped := groupThousands(intPart, group)
			body = grouped
			if hasFrac {
				body += "." + frac
			}
		}
		if align == 0 {
			align = '>'
		}
	default:
		body = it.sem.str(it, v)
		if align == 0 {
			align = '<'
		}
	}
	signStr := ""
	switch {
	case neg:
		signStr = "-"
	case sign == '+' && pyNumeric(v):
		signStr = "+"
	case sign == ' ' && pyNumeric(v):
		signStr = " "
	}
	n := utf8.RuneCountInString(body) + len(signStr)
	pad := ""
	if width > n {
		pad = strings.Repeat(fill, width-n)
	}
	switch align {
	case '<':
		return signStr + body + pad
	case '^':
		left := pad[:len(pad)/len(fill)/2*len(fill)]
		return left + signStr + body + pad[len(left):]
	case '=':
		return signStr + pad + body
	}
	return pad + signStr + body
}

// pyPercent: format gaya printf untuk "teks %d" % nilai.
func (it *interp) pyPercent(f string, args Value, at ir.Node) string {
	vals := []Value{args}
	if args.k == kList && args.l.kind == "tuple" {
		vals = args.l.elems
	}
	var b strings.Builder
	ai := 0
	next := func() Value {
		if ai >= len(vals) {
			it.throw(at, "TypeError", "not enough arguments for format string")
		}
		v := vals[ai]
		ai++
		return v
	}
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			b.WriteByte(f[i])
			continue
		}
		j := i + 1
		var key string
		if j < len(f) && f[j] == '(' {
			k := strings.IndexByte(f[j:], ')')
			if k > 0 {
				key = f[j+1 : j+k]
				j += k + 1
			}
		}
		start := j
		for j < len(f) && strings.IndexByte("-+ #0123456789.*", f[j]) >= 0 {
			j++
		}
		if j >= len(f) {
			b.WriteString(f[i:])
			break
		}
		flags := f[start:j]
		verb := f[j]
		i = j
		if verb == '%' {
			b.WriteByte('%')
			continue
		}
		var v Value
		if key != "" && args.k == kDict {
			v, _ = args.d.get(strV(key))
		} else {
			v = next()
		}
		switch verb {
		case 's':
			b.WriteString(fmt.Sprintf("%"+flags+"s", it.sem.str(it, v)))
		case 'r', 'a':
			b.WriteString(fmt.Sprintf("%"+flags+"s", it.pyRepr(v, map[int]bool{})))
		case 'd', 'i', 'u':
			if v.k == kFloat {
				v = pyInt(int64(v.f))
			}
			b.WriteString(cfmt("%"+flags+"d", toBig(v)))
		case 'f', 'F', 'e', 'E', 'g', 'G':
			if (verb == 'g' || verb == 'G') && !strings.Contains(flags, ".") {
				flags += ".6" // presisi bawaan C/Python, bukan "terpendek" seperti Go
			}
			b.WriteString(fmt.Sprintf("%"+flags+string(verb), toF(v)))
		case 'x', 'X', 'o':
			b.WriteString(cfmt("%"+flags+string(verb), toBig(v)))
		case 'c':
			if v.k == kStr {
				b.WriteString(v.s)
			} else {
				b.WriteRune(rune(toF(v)))
			}
		default:
			it.throw(at, "ValueError", fmt.Sprintf("unsupported format character '%c'", verb))
		}
	}
	if ai < len(vals) && !(args.k == kDict) && !(len(vals) == 1 && ai == 0) {
		it.throw(at, "TypeError", "not all arguments converted during string formatting")
	}
	return b.String()
}

func cfmt(format string, b *big.Int) string { return fmt.Sprintf(format, b) }

// ---------- match/case ----------

func (it *interp) execPyMatch(n *ir.Switch, e *Env, focus bool) flow {
	subj := it.eval(n.Tag, e)
	if subj.k == kUnknown {
		it.structural("unknown match value: " + subj.why)
	}
	if focus && it.recording() {
		it.add(stepInit(it.line(n), fmt.Sprintf("match %s → %s", it.text(n.Tag), it.sem.show(it, subj))), e)
	}
	for i, c := range n.Cases {
		ok := true
		if c.Exprs != nil {
			ok = it.pyPattern(c.Exprs[0], subj, e)
		}
		if ok && c.Guard != nil {
			ok = it.cond(c.Guard, e, "guard")
		}
		if focus && it.recording() {
			r := ok
			label := "case _"
			if c.Exprs != nil {
				label = "case " + it.textList(c.Exprs)
			}
			it.add(stepCond(it.line(c), label+" → "+it.boolText(ok), &r), e)
		}
		if ok {
			it.branchStep(focus, n, i, e, "take "+map[bool]string{true: "case _", false: "case " + it.textList(c.Exprs)}[c.Exprs == nil])
			f := it.execList(c.Body, e)
			return it.blockExit(focus, n, f, e)
		}
	}
	it.branchStep(focus, n, len(n.Cases), e, "no case matches → skip")
	return it.blockExit(focus, n, flow{}, e)
}

// pyPattern mencocokkan pola case dan mengikat variabel tangkapan.
func (it *interp) pyPattern(p ir.Expr, v Value, e *Env) bool {
	switch x := p.(type) {
	case *ir.Ident:
		if x.Name == "_" {
			return true
		}
		it.assignName(x, e, v) // tangkap
		return true
	case *ir.Lit, *ir.Unary, *ir.Template:
		return it.pyEq(it.eval(x, e), v)
	case *ir.Member: // pola nilai: Warna.MERAH
		return it.pyEq(it.eval(x, e), v)
	case *ir.Binary:
		if x.Op == "|" {
			return it.pyPattern(x.X, v, e) || it.pyPattern(x.Y, v, e)
		}
		return it.pyEq(it.eval(x, e), v)
	case *ir.Assign: // pola as nama
		if it.pyPattern(x.R, v, e) {
			it.bindTarget(x.L, e, v, false)
			return true
		}
		return false
	case *ir.ArrayLit:
		if v.k != kList {
			return false
		}
		elems := v.l.elems
		star := -1
		for i, el := range x.Elems {
			if _, ok := el.(*ir.Spread); ok {
				star = i
			}
		}
		if star < 0 && len(elems) != len(x.Elems) || star >= 0 && len(elems) < len(x.Elems)-1 {
			return false
		}
		for i, el := range x.Elems {
			if sp, ok := el.(*ir.Spread); ok {
				rest := elems[i : len(elems)-(len(x.Elems)-1-i)]
				it.pyPattern(sp.X, it.newList("list", append([]Value(nil), rest...)), e)
				continue
			}
			k := i
			if star >= 0 && i > star {
				k = len(elems) - (len(x.Elems) - i)
			}
			if !it.pyPattern(el, elems[k], e) {
				return false
			}
		}
		return true
	case *ir.ObjectLit:
		if v.k != kDict {
			return false
		}
		for _, pr := range x.Props {
			k := it.eval(pr.Key, e)
			val, ok := v.d.get(k)
			if !ok || !it.pyPattern(pr.Value, val, e) {
				return false
			}
		}
		return true
	case *ir.Call: // pola kelas: Point(x=0, y=py)
		cls := it.eval(x.Fun, e)
		if cls.k != kClass {
			return false
		}
		if !(v.k == kObj && instanceOf(v.o.class, cls.c.name) || builtinClassMatch(cls.c, v)) {
			return false
		}
		for i, a := range x.Args {
			switch arg := a.(type) {
			case *ir.KwArg:
				fv, ok := v.o.fields.get(strV(arg.Name))
				if !ok || !it.pyPattern(arg.Value, fv, e) {
					return false
				}
			default:
				if v.k != kObj { // int(x) / str(s)
					return it.pyPattern(arg, v, e)
				}
				ma, _ := cls.c.statics.get(strV("__match_args__"))
				if ma.k != kList || i >= len(ma.l.elems) {
					return false
				}
				fv, _ := v.o.fields.get(ma.l.elems[i])
				if !it.pyPattern(arg, fv, e) {
					return false
				}
			}
		}
		return true
	}
	return it.pyEq(it.eval(p, e), v)
}

func builtinClassMatch(c *Class, v Value) bool {
	switch c.name {
	case "int":
		return v.k == kInt || v.k == kBig
	case "float":
		return v.k == kFloat
	case "str":
		return v.k == kStr
	case "list":
		return v.k == kList && v.l.kind == "list"
	case "dict":
		return v.k == kDict
	case "bool":
		return v.k == kBool
	case "tuple":
		return v.k == kList && v.l.kind == "tuple"
	}
	return false
}

func stepInit(line int, note string) trace.Step {
	return trace.Step{Phase: trace.PhaseInit, Line: line, Note: note}
}

func stepCond(line int, note string, r *bool) trace.Step {
	return trace.Step{Phase: trace.PhaseCond, Line: line, Note: note, Result: r}
}
