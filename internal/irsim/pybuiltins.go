package irsim

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopherviz/internal/ir"
)

func (s *pySem) global(it *interp, name string) (Value, bool) {
	v, ok := s.globals[name]
	return v, ok
}

// pyClass membuat kelas bawaan Python (int, str, list, Exception, ...).
func (it *interp) pyClass(name string, super *Class, ctor func(it *interp, args []Value, at nodeRef) Value) *Class {
	c := &Class{name: name, native: name, super: super, methods: map[string]*Fn{}, getters: map[string]*Fn{},
		setters: map[string]*Fn{}, statics: it.newDict("statics"), kind: "class", ctor: ctor}
	return c
}

func (it *interp) pyKw(name string) (Value, bool) {
	if it.kwargs == nil {
		return Value{}, false
	}
	v, ok := it.kwargs[name]
	return v, ok
}

// pyIter mengubah nilai iterable menjadi daftar elemen (TypeError bila tidak bisa).
func (it *interp) pyIter(v Value, at ir.Node) []Value {
	pairs, ok := it.sem.iterate(it, v, nil)
	if !ok {
		if v.k == kUnknown {
			it.structural("unknown collection contents: " + v.why)
		}
		it.throw(at, "TypeError", fmt.Sprintf("'%s' object is not iterable", it.pyTypeName(v)))
	}
	out := make([]Value, len(pairs))
	for i, p := range pairs {
		out[i] = p.v
	}
	return out
}

func (it *interp) pyLen(v Value, at ir.Node) int {
	switch v.k {
	case kStr:
		return len([]rune(v.s))
	case kList:
		return len(v.l.elems)
	case kDict, kSet:
		return v.d.len()
	case kObj:
		if m := findMethod(v.o.class, "__len__"); m != nil {
			return int(toF(it.callFunc(&Func{decl: m, env: v.o.class.env}, v, argList{}, at)))
		}
	case kUnknown:
		it.structural("unknown length: " + v.why)
	}
	it.throw(at, "TypeError", fmt.Sprintf("object of type '%s' has no len()", it.pyTypeName(v)))
	return 0
}

// pySort mengurutkan secara stabil dengan key/reverse ala Python.
func (it *interp) pySort(elems []Value, key Value, reverse bool, at ir.Node) {
	keys := make([]Value, len(elems))
	for i, e := range elems {
		keys[i] = e
		if key.k == kFunc || key.k == kClass {
			keys[i] = it.call(key, undef(), []Value{e}, at)
		}
	}
	idx := make([]int, len(elems))
	for i := range idx {
		idx[i] = i
	}
	var ferr bool
	less := func(a, b int) bool {
		c, ok := it.pyCompare(keys[a], keys[b])
		if !ok {
			ferr = true
			it.throw(at, "TypeError", fmt.Sprintf("'<' not supported between instances of '%s' and '%s'", it.pyTypeName(keys[b]), it.pyTypeName(keys[a])))
		}
		if reverse {
			return c > 0
		}
		return c < 0
	}
	sort.SliceStable(idx, func(i, j int) bool { return less(idx[i], idx[j]) })
	_ = ferr
	sorted := make([]Value, len(elems))
	for i, k := range idx {
		sorted[i] = elems[k]
	}
	copy(elems, sorted)
}

func (it *interp) pyRange(args []Value, at ir.Node) Value {
	var start, stop, step int64 = 0, 0, 1
	for _, a := range args {
		if !pyIntLike(a) {
			if a.k == kUnknown {
				it.structural("unknown range bounds: " + a.why)
			}
			it.throw(at, "TypeError", fmt.Sprintf("'%s' object cannot be interpreted as an integer", it.pyTypeName(a)))
		}
	}
	switch len(args) {
	case 1:
		stop = int64(toF(args[0]))
	case 2:
		start, stop = int64(toF(args[0])), int64(toF(args[1]))
	case 3:
		start, stop, step = int64(toF(args[0])), int64(toF(args[1])), int64(toF(args[2]))
	default:
		it.throw(at, "TypeError", fmt.Sprintf("range expected at least 1 argument, got %d", len(args)))
	}
	if step == 0 {
		it.throw(at, "ValueError", "range() arg 3 must not be zero")
	}
	var elems []Value
	for i := start; step > 0 && i < stop || step < 0 && i > stop; i += step {
		if len(elems) > 1_000_000 {
			it.structural("range too large to simulate")
		}
		elems = append(elems, pyInt(i))
	}
	v := it.newList("range", elems)
	if step == 1 {
		v.t = fmt.Sprintf("range(%d, %d)", start, stop)
	} else {
		v.t = fmt.Sprintf("range(%d, %d, %d)", start, stop, step)
	}
	return v
}

func (it *interp) pyIterV(name string, elems []Value) Value {
	v := it.newList("iter", elems)
	v.t = name
	return v
}

// pyFsum: penjumlahan float terkompensasi (Neumaier) seperti sum() Python 3.12.
func pyFsum(xs []float64) float64 {
	s, c := 0.0, 0.0
	for _, x := range xs {
		t := s + x
		if math.Abs(s) >= math.Abs(x) {
			c += (s - t) + x
		} else {
			c += (x - t) + s
		}
		s = t
	}
	return s + c
}

func pyParseInt(it *interp, s string, base int, at ir.Node) Value {
	t := strings.ReplaceAll(strings.TrimSpace(s), "_", "")
	neg := false
	if strings.HasPrefix(t, "-") || strings.HasPrefix(t, "+") {
		neg = t[0] == '-'
		t = t[1:]
	}
	low := strings.ToLower(t)
	if base == 0 || base == 16 && strings.HasPrefix(low, "0x") || base == 8 && strings.HasPrefix(low, "0o") || base == 2 && strings.HasPrefix(low, "0b") {
		switch {
		case strings.HasPrefix(low, "0x"):
			base, t = 16, t[2:]
		case strings.HasPrefix(low, "0o"):
			base, t = 8, t[2:]
		case strings.HasPrefix(low, "0b"):
			base, t = 2, t[2:]
		default:
			if base == 0 {
				base = 10
			}
		}
	}
	b, ok := new(big.Int).SetString(t, base)
	if !ok || t == "" {
		it.throw(at, "ValueError", fmt.Sprintf("invalid literal for int() with base %d: %s", base, pyQuote(s)))
	}
	if neg {
		b.Neg(b)
	}
	return bigV(b)
}

func (it *interp) setupPython(s *pySem) {
	g := s.globals
	fn := func(name string, f builtinFn) { g[name] = bf(name, f) }

	// --- exception ---
	excInit := func(it *interp, this Value, args []Value, at nodeRef) {
		if this.k == kObj {
			this.o.fields.set(strV("args"), it.newList("tuple", append([]Value(nil), args...)))
		}
	}
	mkExc := func(name string, super *Class) *Class {
		var c *Class
		c = it.pyClass(name, super, func(it *interp, args []Value, at nodeRef) Value {
			it.ids++
			o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
			v := Value{k: kObj, o: o}
			excInit(it, v, args, at)
			return v
		})
		c.init = excInit
		g[name] = Value{k: kClass, c: c}
		return c
	}
	base := mkExc("BaseException", nil)
	exc := mkExc("Exception", base)
	arith := mkExc("ArithmeticError", exc)
	mkExc("ZeroDivisionError", arith)
	mkExc("OverflowError", arith)
	lookup := mkExc("LookupError", exc)
	mkExc("IndexError", lookup)
	mkExc("KeyError", lookup)
	mkExc("ValueError", exc)
	mkExc("TypeError", exc)
	name := mkExc("NameError", exc)
	mkExc("UnboundLocalError", name)
	mkExc("AttributeError", exc)
	rt := mkExc("RuntimeError", exc)
	mkExc("RecursionError", rt)
	mkExc("NotImplementedError", rt)
	mkExc("AssertionError", exc)
	mkExc("StopIteration", exc)
	mkExc("SyntaxError", exc)
	mkExc("FileNotFoundError", mkExc("OSError", exc))
	mkExc("KeyboardInterrupt", base)
	mkExc("ModuleNotFoundError", mkExc("ImportError", exc))

	// --- tipe dasar ---
	conv := func(name string, f func(it *interp, args []Value, at nodeRef) Value) {
		c := it.pyClass(name, nil, f)
		g[name] = Value{k: kClass, c: c}
	}
	conv("int", func(it *interp, args []Value, at nodeRef) Value {
		if len(args) == 0 {
			return pyInt(0)
		}
		a := args[0]
		switch a.k {
		case kInt, kBig:
			return a
		case kBool:
			return pyInt(map[bool]int64{true: 1}[a.b])
		case kFloat:
			if math.IsInf(a.f, 0) || math.IsNaN(a.f) {
				it.throw(at, "OverflowError", "cannot convert float infinity to integer")
			}
			b, _ := new(big.Float).SetFloat64(math.Trunc(a.f)).Int(nil)
			return bigV(b)
		case kStr:
			base := 10
			if len(args) > 1 {
				base = int(toF(args[1]))
			}
			return pyParseInt(it, a.s, base, at)
		case kUnknown:
			return a
		}
		it.throw(at, "TypeError", fmt.Sprintf("int() argument must be a string, a bytes-like object or a real number, not '%s'", it.pyTypeName(a)))
		return Value{}
	})
	conv("float", func(it *interp, args []Value, at nodeRef) Value {
		if len(args) == 0 {
			return floatV(0)
		}
		a := args[0]
		switch a.k {
		case kStr:
			t := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(a.s, "_", "")))
			switch t {
			case "inf", "+inf", "infinity":
				return floatV(math.Inf(1))
			case "-inf", "-infinity":
				return floatV(math.Inf(-1))
			case "nan":
				return floatV(math.NaN())
			}
			f, err := strconv.ParseFloat(t, 64)
			if err != nil {
				it.throw(at, "ValueError", "could not convert string to float: "+pyQuote(a.s))
			}
			return floatV(f)
		case kUnknown:
			return a
		}
		return floatV(toF(a))
	})
	conv("str", func(it *interp, args []Value, at nodeRef) Value {
		if len(args) == 0 {
			return strV("")
		}
		return strV(it.sem.str(it, args[0]))
	})
	conv("bool", func(it *interp, args []Value, at nodeRef) Value {
		return boolV(len(args) > 0 && it.sem.truthy(args[0]))
	})
	conv("list", func(it *interp, args []Value, at nodeRef) Value {
		if len(args) == 0 {
			return it.newList("list", nil)
		}
		return it.newList("list", append([]Value(nil), it.pyIter(args[0], at)...))
	})
	conv("tuple", func(it *interp, args []Value, at nodeRef) Value {
		if len(args) == 0 {
			return it.newList("tuple", nil)
		}
		return it.newList("tuple", append([]Value(nil), it.pyIter(args[0], at)...))
	})
	conv("set", func(it *interp, args []Value, at nodeRef) Value {
		d := it.newDict("set")
		if len(args) > 0 {
			for _, v := range it.pyIter(args[0], at) {
				d.set(v, v)
			}
		}
		return Value{k: kSet, d: d}
	})
	conv("frozenset", g["set"].c.ctor)
	conv("dict", func(it *interp, args []Value, at nodeRef) Value {
		d := it.newDict("dict")
		if len(args) > 0 {
			if args[0].k == kDict {
				for i, k := range args[0].d.keys {
					d.set(k, args[0].d.vals[i])
				}
			} else {
				for _, p := range it.pyIter(args[0], at) {
					if p.k == kList && len(p.l.elems) == 2 {
						d.set(p.l.elems[0], p.l.elems[1])
					}
				}
			}
		}
		if it.kwargs != nil {
			names := make([]string, 0, len(it.kwargs))
			for k := range it.kwargs {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				d.set(strV(k), it.kwargs[k])
			}
		}
		return Value{k: kDict, d: d}
	})
	conv("object", func(it *interp, args []Value, at nodeRef) Value { return it.dictV("dict") })

	// --- fungsi bawaan ---
	fn("print", func(it *interp, this Value, args []Value, at ir.Node) Value {
		sep, end := " ", "\n"
		if v, ok := it.pyKw("sep"); ok && v.k != kNull {
			sep = it.sem.str(it, v)
		}
		if v, ok := it.pyKw("end"); ok && v.k != kNull {
			end = it.sem.str(it, v)
		}
		parts := make([]string, len(args))
		for i, a := range args {
			if a.k == kUnknown {
				parts[i] = "?"
				continue
			}
			parts[i] = it.sem.str(it, a)
		}
		it.out(strings.Join(parts, sep) + end)
		return null()
	})
	fn("input", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if len(args) > 0 {
			it.out(it.sem.str(it, args[0]))
		}
		s, ok := it.userText(true)
		if !ok {
			it.throw(at, "EOFError", "EOF when reading a line")
		}
		return strV(s)
	})
	fn("len", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return pyInt(int64(it.pyLen(arg(args, 0), at)))
	})
	fn("range", func(it *interp, this Value, args []Value, at ir.Node) Value { return it.pyRange(args, at) })
	fn("abs", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a := arg(args, 0)
		switch a.k {
		case kFloat:
			return floatV(math.Abs(a.f))
		case kInt, kBig, kBool:
			return bigV(new(big.Int).Abs(toBig(a)))
		}
		if r, ok := it.pyDunderUnary("__abs__", a, at); ok {
			return r
		}
		it.throw(at, "TypeError", "bad operand type for abs(): '"+it.pyTypeName(a)+"'")
		return Value{}
	})
	minmax := func(name string, want int) builtinFn {
		return func(it *interp, this Value, args []Value, at ir.Node) Value {
			items := args
			if len(args) == 1 {
				items = it.pyIter(args[0], at)
			}
			key, _ := it.pyKw("key")
			if len(items) == 0 {
				if d, ok := it.pyKw("default"); ok {
					return d
				}
				it.throw(at, "ValueError", name+"() iterable argument is empty")
			}
			best := items[0]
			bestKey := best
			if key.k == kFunc || key.k == kClass {
				bestKey = it.call(key, undef(), []Value{best}, at)
			}
			for _, x := range items[1:] {
				k := x
				if key.k == kFunc || key.k == kClass {
					k = it.call(key, undef(), []Value{x}, at)
				}
				c, ok := it.pyCompare(k, bestKey)
				if !ok {
					it.throw(at, "TypeError", fmt.Sprintf("'%s' not supported between instances of '%s' and '%s'", map[int]string{1: ">", -1: "<"}[want], it.pyTypeName(k), it.pyTypeName(bestKey)))
				}
				if c == want {
					best, bestKey = x, k
				}
			}
			return best
		}
	}
	fn("max", minmax("max", 1))
	fn("min", minmax("min", -1))
	fn("sum", func(it *interp, this Value, args []Value, at ir.Node) Value {
		items := it.pyIter(arg(args, 0), at)
		acc := pyInt(0)
		if len(args) > 1 {
			acc = args[1]
		} else if v, ok := it.pyKw("start"); ok {
			acc = v
		}
		allNum := true
		var fs []float64
		for _, x := range items {
			if !pyNumeric(x) {
				allNum = false
			}
		}
		hasFloat := acc.k == kFloat
		for _, x := range items {
			if x.k == kFloat {
				hasFloat = true
			}
		}
		if allNum && pyNumeric(acc) && hasFloat {
			fs = append(fs, toF(acc))
			for _, x := range items {
				fs = append(fs, toF(x))
			}
			return floatV(pyFsum(fs))
		}
		for _, x := range items {
			acc = it.sem.binary(it, "+", acc, x, at)
		}
		return acc
	})
	sortedFn := func(it *interp, this Value, args []Value, at ir.Node) Value {
		items := append([]Value(nil), it.pyIter(arg(args, 0), at)...)
		key, _ := it.pyKw("key")
		rev, hasRev := it.pyKw("reverse")
		it.pySort(items, key, hasRev && it.sem.truthy(rev), at)
		return it.newList("list", items)
	}
	fn("sorted", sortedFn)
	fn("reversed", func(it *interp, this Value, args []Value, at ir.Node) Value {
		items := it.pyIter(arg(args, 0), at)
		out := make([]Value, len(items))
		for i, x := range items {
			out[len(items)-1-i] = x
		}
		return it.pyIterV("list_reverseiterator", out)
	})
	fn("enumerate", func(it *interp, this Value, args []Value, at ir.Node) Value {
		start := int64(0)
		if len(args) > 1 {
			start = int64(toF(args[1]))
		} else if v, ok := it.pyKw("start"); ok {
			start = int64(toF(v))
		}
		items := it.pyIter(arg(args, 0), at)
		out := make([]Value, len(items))
		for i, x := range items {
			out[i] = it.newList("tuple", []Value{pyInt(start + int64(i)), x})
		}
		return it.pyIterV("enumerate", out)
	})
	fn("zip", func(it *interp, this Value, args []Value, at ir.Node) Value {
		var lists [][]Value
		n := -1
		for _, a := range args {
			l := it.pyIter(a, at)
			lists = append(lists, l)
			if n < 0 || len(l) < n {
				n = len(l)
			}
		}
		var out []Value
		for i := 0; i < n; i++ {
			t := make([]Value, len(lists))
			for j := range lists {
				t[j] = lists[j][i]
			}
			out = append(out, it.newList("tuple", t))
		}
		return it.pyIterV("zip", out)
	})
	fn("map", func(it *interp, this Value, args []Value, at ir.Node) Value {
		f := arg(args, 0)
		var lists [][]Value
		n := -1
		for _, a := range args[1:] {
			l := it.pyIter(a, at)
			lists = append(lists, l)
			if n < 0 || len(l) < n {
				n = len(l)
			}
		}
		var out []Value
		for i := 0; i < n; i++ {
			a := make([]Value, len(lists))
			for j := range lists {
				a[j] = lists[j][i]
			}
			out = append(out, it.call(f, undef(), a, at))
		}
		return it.pyIterV("map", out)
	})
	fn("filter", func(it *interp, this Value, args []Value, at ir.Node) Value {
		f := arg(args, 0)
		var out []Value
		for _, x := range it.pyIter(arg(args, 1), at) {
			keep := it.sem.truthy(x)
			if f.k != kNull {
				keep = it.sem.truthy(it.call(f, undef(), []Value{x}, at))
			}
			if keep {
				out = append(out, x)
			}
		}
		return it.pyIterV("filter", out)
	})
	fn("any", func(it *interp, this Value, args []Value, at ir.Node) Value {
		for _, x := range it.pyIter(arg(args, 0), at) {
			if it.sem.truthy(x) {
				return boolV(true)
			}
		}
		return boolV(false)
	})
	fn("all", func(it *interp, this Value, args []Value, at ir.Node) Value {
		for _, x := range it.pyIter(arg(args, 0), at) {
			if !it.sem.truthy(x) {
				return boolV(false)
			}
		}
		return boolV(true)
	})
	fn("round", func(it *interp, this Value, args []Value, at ir.Node) Value {
		x := arg(args, 0)
		if len(args) < 2 || args[1].k == kNull {
			if pyIntLike(x) {
				return x
			}
			return bigV(bigFromFloat(math.RoundToEven(toF(x))))
		}
		nd := int(toF(args[1]))
		if pyIntLike(x) {
			if nd >= 0 {
				return x
			}
			p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-nd)), nil)
			q := new(big.Rat).SetFrac(toBig(x), p)
			f, _ := q.Float64()
			r := bigFromFloat(math.RoundToEven(f))
			return bigV(r.Mul(r, p))
		}
		if nd < 0 {
			p := math.Pow(10, float64(-nd))
			return floatV(math.RoundToEven(toF(x)/p) * p)
		}
		f, _ := strconv.ParseFloat(strconv.FormatFloat(toF(x), 'f', nd, 64), 64)
		return floatV(f)
	})
	fn("divmod", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := arg(args, 0), arg(args, 1)
		return it.newList("tuple", []Value{it.pyArith("//", a, b, at), it.pyArith("%", a, b, at)})
	})
	fn("pow", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if len(args) == 3 {
			return bigV(new(big.Int).Exp(toBig(args[0]), toBig(args[1]), toBig(args[2])))
		}
		return it.sem.binary(it, "**", arg(args, 0), arg(args, 1), at)
	})
	fn("chr", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return strV(string(rune(toF(arg(args, 0)))))
	})
	fn("ord", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s := arg(args, 0)
		rs := []rune(s.s)
		if s.k != kStr || len(rs) != 1 {
			it.throw(at, "TypeError", fmt.Sprintf("ord() expected a character, but string of length %d found", len(rs)))
		}
		return pyInt(int64(rs[0]))
	})
	radix := func(name, prefix string, base int) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			b := toBig(arg(args, 0))
			sign := ""
			if b.Sign() < 0 {
				sign = "-"
			}
			return strV(sign + prefix + new(big.Int).Abs(b).Text(base))
		})
	}
	radix("hex", "0x", 16)
	radix("oct", "0o", 8)
	radix("bin", "0b", 2)
	fn("isinstance", func(it *interp, this Value, args []Value, at ir.Node) Value {
		x, t := arg(args, 0), arg(args, 1)
		classes := []Value{t}
		if t.k == kList {
			classes = t.l.elems
		}
		for _, c := range classes {
			if c.k != kClass {
				continue
			}
			if x.k == kObj && instanceOf(x.o.class, c.c.name) || builtinClassMatch(c.c, x) ||
				c.c.name == "int" && x.k == kBool || c.c.name == "object" {
				return boolV(true)
			}
		}
		return boolV(false)
	})
	fn("type", func(it *interp, this Value, args []Value, at ir.Node) Value {
		x := arg(args, 0)
		if x.k == kObj {
			return Value{k: kClass, c: x.o.class}
		}
		if c, ok := g[it.pyTypeName(x)]; ok {
			return c
		}
		return Value{k: kClass, c: it.pyClass(it.pyTypeName(x), nil, nil)}
	})
	fn("repr", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return strV(it.pyRepr(arg(args, 0), map[int]bool{}))
	})
	fn("format", func(it *interp, this Value, args []Value, at ir.Node) Value {
		spec := ""
		if len(args) > 1 {
			spec = it.sem.str(it, args[1])
		}
		return strV(it.pyFormatValue(arg(args, 0), spec, at))
	})
	fn("callable", func(it *interp, this Value, args []Value, at ir.Node) Value {
		k := arg(args, 0).k
		return boolV(k == kFunc || k == kClass)
	})
	fn("hasattr", func(it *interp, this Value, args []Value, at ir.Node) Value {
		x, n := arg(args, 0), it.sem.str(it, arg(args, 1))
		if x.k == kObj {
			return boolV(x.o.fields.has(strV(n)) || findMethod(x.o.class, n) != nil)
		}
		_, ok := it.sem.member(it, x, n, at)
		return boolV(ok)
	})
	fn("getattr", func(it *interp, this Value, args []Value, at ir.Node) Value {
		x, n := arg(args, 0), it.sem.str(it, arg(args, 1))
		if len(args) > 2 && x.k == kObj && !x.o.fields.has(strV(n)) && findMethod(x.o.class, n) == nil {
			return args[2]
		}
		return it.getProp(x, n, at)
	})
	fn("setattr", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if x := arg(args, 0); x.k == kObj {
			x.o.fields.set(strV(it.sem.str(it, arg(args, 1))), arg(args, 2))
		}
		return null()
	})
	fn("id", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return pyInt(int64(140000000000 + objectID(arg(args, 0))*16))
	})
	fn("hash", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a := arg(args, 0)
		if pyIntLike(a) {
			return a
		}
		it.warn("Python randomizes string hash() on every run; the value here is only an example")
		return pyInt(int64(len(keyID(a))))
	})
	fn("open", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.structural("reading/writing files (open) is not supported by the simulator yet")
		return Value{}
	})
	fn("iter", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.pyIterV("iterator", append([]Value(nil), it.pyIter(arg(args, 0), at)...))
	})
	fn("next", func(it *interp, this Value, args []Value, at ir.Node) Value {
		x := arg(args, 0)
		if x.k == kList && x.l.kind == "iter" {
			if len(x.l.elems) == 0 {
				if len(args) > 1 {
					return args[1]
				}
				it.throw(at, "StopIteration", "")
			}
			v := x.l.elems[0]
			x.l.elems = x.l.elems[1:]
			return v
		}
		it.throw(at, "TypeError", fmt.Sprintf("'%s' object is not an iterator", it.pyTypeName(x)))
		return Value{}
	})
	g["NotImplemented"] = unknown("NotImplemented")
	g["__name__"] = strV("__main__")

	// --- modul ---
	s.modules["math"] = it.pyMathModule()
	s.modules["random"] = it.pyRandomModule()
	s.modules["string"] = it.moduleV(map[string]Value{
		"ascii_lowercase": strV("abcdefghijklmnopqrstuvwxyz"), "ascii_uppercase": strV("ABCDEFGHIJKLMNOPQRSTUVWXYZ"),
		"ascii_letters": strV("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"), "digits": strV("0123456789"),
		"punctuation": strV("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"),
	})
	s.modules["collections"] = it.pyCollectionsModule()
	s.modules["functools"] = it.moduleV(map[string]Value{
		"reduce": bf("reduce", func(it *interp, this Value, args []Value, at ir.Node) Value {
			f := arg(args, 0)
			items := it.pyIter(arg(args, 1), at)
			var acc Value
			if len(args) > 2 {
				acc = args[2]
			} else {
				if len(items) == 0 {
					it.throw(at, "TypeError", "reduce() of empty iterable with no initial value")
				}
				acc, items = items[0], items[1:]
			}
			for _, x := range items {
				acc = it.call(f, undef(), []Value{acc, x}, at)
			}
			return acc
		}),
	})
	s.modules["bisect"] = it.pyBisectModule()
	s.modules["sys"] = it.moduleV(map[string]Value{"maxsize": pyInt(math.MaxInt64), "argv": it.newList("list", []Value{strV("main.py")})})
	for name, m := range s.modules {
		m.t = name
		s.modules[name] = m
	}
}

// pyBisectModule: bisect_left/bisect_right/bisect dan insort_* seperti modul bisect Python.
func (it *interp) pyBisectModule() Value {
	search := func(right bool) func(it *interp, this Value, args []Value, at ir.Node) Value {
		return func(it *interp, this Value, args []Value, at ir.Node) Value {
			a := arg(args, 0)
			if a.k != kList {
				it.throw(at, "TypeError", "bisect needs a list")
			}
			x := arg(args, 1)
			lo, hi := 0, len(a.l.elems)
			if len(args) > 2 {
				lo = int(intOf(args[2]))
			}
			if len(args) > 3 {
				hi = int(intOf(args[3]))
			}
			for lo < hi {
				mid := (lo + hi) / 2
				r := it.relation(a.l.elems[mid], x)
				if r == "<" || right && r == "=" {
					lo = mid + 1
				} else {
					hi = mid
				}
			}
			return pyInt(int64(lo))
		}
	}
	insort := func(right bool) func(it *interp, this Value, args []Value, at ir.Node) Value {
		find := search(right)
		return func(it *interp, this Value, args []Value, at ir.Node) Value {
			i := int(intOf(find(it, this, args, at)))
			l := arg(args, 0).l
			l.elems = append(l.elems[:i], append([]Value{arg(args, 1)}, l.elems[i:]...)...)
			it.rec.dirty = true
			return null()
		}
	}
	return it.moduleV(map[string]Value{
		"bisect_left": bf("bisect_left", search(false)), "bisect_right": bf("bisect_right", search(true)),
		"bisect": bf("bisect", search(true)), "insort_left": bf("insort_left", insort(false)),
		"insort_right": bf("insort_right", insort(true)), "insort": bf("insort", insort(true)),
	})
}

func bigFromFloat(f float64) *big.Int {
	b, _ := new(big.Float).SetFloat64(f).Int(nil)
	if b == nil {
		return new(big.Int)
	}
	return b
}

func (it *interp) pyMathModule() Value {
	n := func(v Value) float64 { return toF(v) }
	f1 := func(name string, f func(float64) float64) Value {
		return bf(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			x := arg(args, 0)
			if !pyNumeric(x) {
				if x.k == kUnknown {
					return x
				}
				it.throw(at, "TypeError", "must be real number, not "+it.pyTypeName(x))
			}
			r := f(n(x))
			if math.IsNaN(r) && !math.IsNaN(n(x)) {
				it.throw(at, "ValueError", "math domain error")
			}
			return floatV(r)
		})
	}
	toInt := func(name string, f func(float64) float64) Value {
		return bf(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			x := arg(args, 0)
			if pyIntLike(x) {
				return x
			}
			return bigV(bigFromFloat(f(n(x))))
		})
	}
	return it.moduleV(map[string]Value{
		"pi": floatV(math.Pi), "e": floatV(math.E), "tau": floatV(2 * math.Pi), "inf": floatV(math.Inf(1)), "nan": floatV(math.NaN()),
		"sqrt": f1("sqrt", math.Sqrt), "exp": f1("exp", math.Exp), "fabs": f1("fabs", math.Abs),
		"sin": f1("sin", math.Sin), "cos": f1("cos", math.Cos), "tan": f1("tan", math.Tan),
		"asin": f1("asin", math.Asin), "acos": f1("acos", math.Acos), "atan": f1("atan", math.Atan),
		"log2": f1("log2", math.Log2), "log10": f1("log10", math.Log10),
		"degrees": f1("degrees", func(x float64) float64 { return x * 180 / math.Pi }),
		"radians": f1("radians", func(x float64) float64 { return x * math.Pi / 180 }),
		"floor":   toInt("floor", math.Floor), "ceil": toInt("ceil", math.Ceil), "trunc": toInt("trunc", math.Trunc),
		"log": bf("log", func(it *interp, this Value, args []Value, at ir.Node) Value {
			x := n(arg(args, 0))
			if x <= 0 {
				it.throw(at, "ValueError", "math domain error")
			}
			if len(args) > 1 {
				return floatV(math.Log(x) / math.Log(n(args[1])))
			}
			return floatV(math.Log(x))
		}),
		"pow": bf("pow", func(it *interp, this Value, args []Value, at ir.Node) Value {
			return floatV(math.Pow(n(arg(args, 0)), n(arg(args, 1))))
		}),
		"atan2": bf("atan2", func(it *interp, this Value, args []Value, at ir.Node) Value {
			return floatV(math.Atan2(n(arg(args, 0)), n(arg(args, 1))))
		}),
		"hypot": bf("hypot", func(it *interp, this Value, args []Value, at ir.Node) Value {
			s := 0.0
			for _, a := range args {
				s += n(a) * n(a)
			}
			return floatV(math.Sqrt(s))
		}),
		"gcd": bf("gcd", func(it *interp, this Value, args []Value, at ir.Node) Value {
			r := new(big.Int)
			for _, a := range args {
				r.GCD(nil, nil, r, new(big.Int).Abs(toBig(a)))
			}
			return bigV(r)
		}),
		"lcm": bf("lcm", func(it *interp, this Value, args []Value, at ir.Node) Value {
			r := big.NewInt(1)
			for _, a := range args {
				b := new(big.Int).Abs(toBig(a))
				if b.Sign() == 0 {
					return pyInt(0)
				}
				g := new(big.Int).GCD(nil, nil, r, b)
				r.Mul(r, b).Div(r, g)
			}
			return bigV(r)
		}),
		"factorial": bf("factorial", func(it *interp, this Value, args []Value, at ir.Node) Value {
			x := arg(args, 0)
			if !pyIntLike(x) {
				it.throw(at, "TypeError", "'"+it.pyTypeName(x)+"' object cannot be interpreted as an integer")
			}
			k := toBig(x)
			if k.Sign() < 0 {
				it.throw(at, "ValueError", "factorial() not defined for negative values")
			}
			if k.Int64() > 5000 {
				it.structural("factorial too large to simulate")
			}
			return bigV(new(big.Int).MulRange(1, k.Int64()))
		}),
		"comb": bf("comb", func(it *interp, this Value, args []Value, at ir.Node) Value {
			return bigV(new(big.Int).Binomial(toBig(arg(args, 0)).Int64(), toBig(arg(args, 1)).Int64()))
		}),
		"perm": bf("perm", func(it *interp, this Value, args []Value, at ir.Node) Value {
			nn, k := toBig(arg(args, 0)).Int64(), toBig(arg(args, 1)).Int64()
			if k > nn {
				return pyInt(0)
			}
			return bigV(new(big.Int).MulRange(nn-k+1, nn))
		}),
		"isqrt": bf("isqrt", func(it *interp, this Value, args []Value, at ir.Node) Value {
			return bigV(new(big.Int).Sqrt(toBig(arg(args, 0))))
		}),
		"isclose": bf("isclose", func(it *interp, this Value, args []Value, at ir.Node) Value {
			a, b := n(arg(args, 0)), n(arg(args, 1))
			return boolV(math.Abs(a-b) <= math.Max(1e-9*math.Max(math.Abs(a), math.Abs(b)), 0))
		}),
		"isnan": bf("isnan", func(it *interp, this Value, args []Value, at ir.Node) Value {
			return boolV(math.IsNaN(n(arg(args, 0))))
		}),
		"isinf": bf("isinf", func(it *interp, this Value, args []Value, at ir.Node) Value {
			return boolV(math.IsInf(n(arg(args, 0)), 0))
		}),
		"prod": bf("prod", func(it *interp, this Value, args []Value, at ir.Node) Value {
			acc := pyInt(1)
			for _, x := range it.pyIter(arg(args, 0), at) {
				acc = it.sem.binary(it, "*", acc, x, at)
			}
			return acc
		}),
		"fsum": bf("fsum", func(it *interp, this Value, args []Value, at ir.Node) Value {
			var fs []float64
			for _, x := range it.pyIter(arg(args, 0), at) {
				fs = append(fs, n(x))
			}
			return floatV(pyFsum(fs))
		}),
	})
}

func (it *interp) pyRandomModule() Value {
	warn := func() {
		it.warn("the random module is simulated with a fixed seed; results differ from a real run")
	}
	return it.moduleV(map[string]Value{
		"random": bf("random", func(it *interp, this Value, args []Value, at ir.Node) Value {
			warn()
			return floatV(it.rng.Float64())
		}),
		"randint": bf("randint", func(it *interp, this Value, args []Value, at ir.Node) Value {
			warn()
			a, b := int64(toF(arg(args, 0))), int64(toF(arg(args, 1)))
			if b < a {
				it.throw(at, "ValueError", "empty range in randrange")
			}
			return pyInt(a + it.rng.Int63n(b-a+1))
		}),
		"randrange": bf("randrange", func(it *interp, this Value, args []Value, at ir.Node) Value {
			warn()
			r := it.pyRange(args, at)
			if len(r.l.elems) == 0 {
				it.throw(at, "ValueError", "empty range for randrange()")
			}
			return r.l.elems[it.rng.Intn(len(r.l.elems))]
		}),
		"choice": bf("choice", func(it *interp, this Value, args []Value, at ir.Node) Value {
			warn()
			items := it.pyIter(arg(args, 0), at)
			if len(items) == 0 {
				it.throw(at, "IndexError", "Cannot choose from an empty sequence")
			}
			return items[it.rng.Intn(len(items))]
		}),
		"shuffle": bf("shuffle", func(it *interp, this Value, args []Value, at ir.Node) Value {
			warn()
			if l := arg(args, 0); l.k == kList {
				it.rng.Shuffle(len(l.l.elems), func(i, j int) { l.l.elems[i], l.l.elems[j] = l.l.elems[j], l.l.elems[i] })
			}
			return null()
		}),
		"uniform": bf("uniform", func(it *interp, this Value, args []Value, at ir.Node) Value {
			warn()
			a, b := toF(arg(args, 0)), toF(arg(args, 1))
			return floatV(a + (b-a)*it.rng.Float64())
		}),
		"seed": bf("seed", func(it *interp, this Value, args []Value, at ir.Node) Value { return null() }),
	})
}

func (it *interp) pyCollectionsModule() Value {
	counter := it.pyClass("Counter", nil, func(it *interp, args []Value, at nodeRef) Value {
		d := it.newDict("Counter")
		if len(args) > 0 {
			if args[0].k == kDict {
				for i, k := range args[0].d.keys {
					d.set(k, args[0].d.vals[i])
				}
			} else {
				for _, x := range it.pyIter(args[0], at) {
					cur, _ := d.get(x)
					if cur.k != kInt {
						cur = pyInt(0)
					}
					d.set(x, pyInt(cur.i+1))
				}
			}
		}
		return Value{k: kDict, d: d}
	})
	dd := it.pyClass("defaultdict", nil, func(it *interp, args []Value, at nodeRef) Value {
		d := it.newDict("defaultdict")
		f := arg(args, 0)
		d.factory = &f
		return Value{k: kDict, d: d}
	})
	deque := it.pyClass("deque", nil, func(it *interp, args []Value, at nodeRef) Value {
		var elems []Value
		if len(args) > 0 {
			elems = append(elems, it.pyIter(args[0], at)...)
		}
		return it.newList("deque", elems)
	})
	return it.moduleV(map[string]Value{
		"Counter": {k: kClass, c: counter}, "defaultdict": {k: kClass, c: dd}, "deque": {k: kClass, c: deque},
	})
}

// pyImport menjalankan import / from ... import ... untuk modul yang didukung.
func (it *interp) pyImport(n *ir.Simple, e *Env) {
	s := it.sem.(*pySem)
	text := strings.Join(strings.Fields(it.text(n)), " ")
	bind := func(name string, v Value) { it.assignName(&ir.Ident{S: n.S, Name: name}, e, v) }
	module := func(name string) (Value, bool) {
		if m, ok := s.modules[name]; ok {
			return m, true
		}
		return it.pyUserModule(name) // modul buatan user: name.py di folder yang sama
	}
	if strings.HasPrefix(text, "from ") {
		rest := strings.TrimPrefix(text, "from ")
		modName, names, _ := strings.Cut(rest, " import ")
		m, ok := module(strings.TrimSpace(modName))
		names = strings.Trim(strings.TrimSpace(names), "()")
		for _, part := range strings.Split(names, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			orig, alias, hasAlias := strings.Cut(part, " as ")
			if !hasAlias {
				alias = orig
			}
			orig, alias = strings.TrimSpace(orig), strings.TrimSpace(alias)
			if !ok {
				bind(alias, unknown("module "+modName+" is not supported by the simulator yet"))
				continue
			}
			if orig == "*" {
				for i, k := range m.d.keys {
					bind(k.s, m.d.vals[i])
				}
				continue
			}
			v, found := m.d.get(strV(orig))
			if !found {
				it.throw(n, "ImportError", fmt.Sprintf("cannot import name '%s' from '%s'", orig, modName))
			}
			bind(alias, v)
		}
		return
	}
	for _, part := range strings.Split(strings.TrimPrefix(text, "import "), ",") {
		orig, alias, hasAlias := strings.Cut(strings.TrimSpace(part), " as ")
		orig = strings.TrimSpace(orig)
		if !hasAlias {
			alias = strings.Split(orig, ".")[0]
		}
		if m, ok := module(orig); ok {
			bind(strings.TrimSpace(alias), m)
		} else {
			bind(strings.TrimSpace(alias), unknown("module"))
			it.warn("module " + orig + " is not supported by the simulator yet; its values cannot be computed")
		}
	}
}

// ---------- member bawaan ----------

func (s *pySem) member(it *interp, x Value, name string, at ir.Node) (Value, bool) {
	switch x.k {
	case kStr:
		return it.pyStrMember(x, name, at)
	case kList:
		return it.pyListMember(x, name, at)
	case kDict:
		return it.pyDictMember(x, name, at)
	case kSet:
		return it.pySetMember(x, name, at)
	case kInt, kBig, kBool:
		switch name {
		case "bit_length":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
				return pyInt(int64(new(big.Int).Abs(toBig(this)).BitLen()))
			}), true
		case "real", "numerator":
			return x, true
		}
	case kFloat:
		switch name {
		case "is_integer":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(isIntegral(this.f)) }), true
		case "real":
			return x, true
		}
	case kObj:
		if name == "__class__" {
			return Value{k: kClass, c: x.o.class}, true
		}
		if name == "__dict__" {
			return Value{k: kDict, d: x.o.fields}, true
		}
	case kClass:
		if x.c.native != "" && x.c.ctor != nil && !instanceOf(x.c, "BaseException") {
			// str.upper, list.append, ...: method tak terikat, objeknya argumen pertama
			if name != "__name__" {
				return bf(x.c.name+"."+name, func(it *interp, this Value, args []Value, at ir.Node) Value {
					if len(args) == 0 {
						it.throw(at, "TypeError", fmt.Sprintf("unbound method %s.%s() needs an argument", x.c.name, name))
					}
					return it.call(it.getProp(args[0], name, at), args[0], args[1:], at)
				}), true
			}
		}
		switch name {
		case "__name__":
			return strV(x.c.name), true
		case "__class__":
			return Value{k: kClass, c: it.findClass("type", nil)}, true
		}
	case kFunc:
		if name == "__name__" {
			return strV(x.fn.name), true
		}
	}
	return Value{}, false
}

func (it *interp) pyStrMember(x Value, name string, at ir.Node) (Value, bool) {
	s := x.s
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	str := func(v Value) string {
		if v.k != kStr {
			it.throw(at, "TypeError", fmt.Sprintf("must be str, not %s", it.pyTypeName(v)))
		}
		return v.s
	}
	switch name {
	case "upper":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.ToUpper(s)) })
	case "lower", "casefold":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.ToLower(s)) })
	case "strip", "lstrip", "rstrip":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			cut := " \t\n\r\x0b\x0c"
			if a := arg(args, 0); a.k == kStr {
				cut = a.s
			}
			switch name {
			case "lstrip":
				return strV(strings.TrimLeft(s, cut))
			case "rstrip":
				return strV(strings.TrimRight(s, cut))
			}
			return strV(strings.Trim(s, cut))
		})
	case "split", "rsplit":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			sep := arg(args, 0)
			if v, ok := it.pyKw("sep"); ok {
				sep = v
			}
			max := -1
			if len(args) > 1 {
				max = int(toF(args[1]))
			} else if v, ok := it.pyKw("maxsplit"); ok {
				max = int(toF(v))
			}
			var parts []string
			if sep.k == kNull || sep.k == kUndef {
				if max < 0 {
					parts = strings.Fields(s)
				} else {
					parts = strings.Fields(s)
					if len(parts) > max+1 {
						// sisa tetap disatukan dari teks aslinya
						rest := strings.TrimLeft(s, " \t\n")
						var out []string
						for i := 0; i < max; i++ {
							f := strings.Fields(rest)
							out = append(out, f[0])
							rest = strings.TrimLeft(rest[strings.Index(rest, f[0])+len(f[0]):], " \t\n")
						}
						parts = append(out, rest)
					}
				}
			} else {
				sp := str(sep)
				if sp == "" {
					it.throw(at, "ValueError", "empty separator")
				}
				if name == "rsplit" && max >= 0 {
					all := strings.Split(s, sp)
					if len(all) > max+1 {
						head := strings.Join(all[:len(all)-max], sp)
						parts = append([]string{head}, all[len(all)-max:]...)
					} else {
						parts = all
					}
				} else {
					parts = strings.SplitN(s, sp, max+map[bool]int{true: 1, false: 0}[max >= 0])
					if max < 0 {
						parts = strings.Split(s, sp)
					}
				}
			}
			out := make([]Value, len(parts))
			for i, p := range parts {
				out[i] = strV(p)
			}
			return it.newList("list", out)
		})
	case "splitlines":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n"), "\n")
			if s == "" {
				lines = nil
			}
			out := make([]Value, len(lines))
			for i, l := range lines {
				out[i] = strV(l)
			}
			return it.newList("list", out)
		})
	case "join":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			items := it.pyIter(arg(args, 0), at)
			parts := make([]string, len(items))
			for i, v := range items {
				if v.k != kStr {
					it.throw(at, "TypeError", fmt.Sprintf("sequence item %d: expected str instance, %s found", i, it.pyTypeName(v)))
				}
				parts[i] = v.s
			}
			return strV(strings.Join(parts, s))
		})
	case "replace":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			n := -1
			if len(args) > 2 {
				n = int(toF(args[2]))
			}
			return strV(strings.Replace(s, str(arg(args, 0)), str(arg(args, 1)), n))
		})
	case "find", "rfind", "index", "rindex":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			sub := str(arg(args, 0))
			var i int
			if strings.HasPrefix(name, "r") {
				i = strings.LastIndex(s, sub)
			} else {
				i = strings.Index(s, sub)
			}
			if i >= 0 {
				i = len([]rune(s[:i]))
			}
			if i < 0 && strings.Contains(name, "index") {
				it.throw(at, "ValueError", "substring not found")
			}
			return pyInt(int64(i))
		})
	case "count":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			sub := str(arg(args, 0))
			if sub == "" {
				return pyInt(int64(len([]rune(s)) + 1))
			}
			return pyInt(int64(strings.Count(s, sub)))
		})
	case "startswith", "endswith":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			cands := []Value{arg(args, 0)}
			if c := arg(args, 0); c.k == kList {
				cands = c.l.elems
			}
			for _, c := range cands {
				if name == "startswith" && strings.HasPrefix(s, str(c)) || name == "endswith" && strings.HasSuffix(s, str(c)) {
					return boolV(true)
				}
			}
			return boolV(false)
		})
	case "isdigit", "isnumeric", "isdecimal", "isalpha", "isalnum", "isspace", "isupper", "islower", "istitle":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if s == "" {
				return boolV(false)
			}
			hasCased := false
			for _, r := range s {
				switch name {
				case "isdigit", "isnumeric", "isdecimal":
					if !unicode.IsDigit(r) {
						return boolV(false)
					}
				case "isalpha":
					if !unicode.IsLetter(r) {
						return boolV(false)
					}
				case "isalnum":
					if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
						return boolV(false)
					}
				case "isspace":
					if !unicode.IsSpace(r) {
						return boolV(false)
					}
				case "isupper":
					if unicode.IsLower(r) {
						return boolV(false)
					}
					hasCased = hasCased || unicode.IsUpper(r)
				case "islower":
					if unicode.IsUpper(r) {
						return boolV(false)
					}
					hasCased = hasCased || unicode.IsLower(r)
				}
			}
			if name == "isupper" || name == "islower" {
				return boolV(hasCased)
			}
			if name == "istitle" {
				return boolV(s == pyTitle(s))
			}
			return boolV(true)
		})
	case "title":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(pyTitle(s)) })
	case "capitalize":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			rs := []rune(strings.ToLower(s))
			if len(rs) > 0 {
				rs[0] = unicode.ToUpper(rs[0])
			}
			return strV(string(rs))
		})
	case "swapcase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return strV(strings.Map(func(r rune) rune {
				if unicode.IsUpper(r) {
					return unicode.ToLower(r)
				}
				return unicode.ToUpper(r)
			}, s))
		})
	case "center", "ljust", "rjust":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			w := int(toF(arg(args, 0)))
			fill := " "
			if f := arg(args, 1); f.k == kStr {
				fill = f.s
			}
			n := len([]rune(s))
			if w <= n {
				return strV(s)
			}
			pad := w - n
			switch name {
			case "ljust":
				return strV(s + strings.Repeat(fill, pad))
			case "rjust":
				return strV(strings.Repeat(fill, pad) + s)
			}
			left := pad / 2
			if pad%2 == 1 && n%2 == 1 {
				left++
			}
			return strV(strings.Repeat(fill, left) + s + strings.Repeat(fill, pad-left))
		})
	case "zfill":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			w := int(toF(arg(args, 0)))
			sign := ""
			body := s
			if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
				sign, body = s[:1], s[1:]
			}
			if len(s) >= w {
				return strV(s)
			}
			return strV(sign + strings.Repeat("0", w-len(s)) + body)
		})
	case "format":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(it.pyStrFormat(s, args, at)) })
	case "partition", "rpartition":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			sep := str(arg(args, 0))
			var i int
			if name == "partition" {
				i = strings.Index(s, sep)
			} else {
				i = strings.LastIndex(s, sep)
			}
			if i < 0 {
				if name == "partition" {
					return it.newList("tuple", []Value{strV(s), strV(""), strV("")})
				}
				return it.newList("tuple", []Value{strV(""), strV(""), strV(s)})
			}
			return it.newList("tuple", []Value{strV(s[:i]), strV(sep), strV(s[i+len(sep):])})
		})
	case "encode":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(s) })
	}
	return Value{}, false
}

func pyTitle(s string) string {
	rs := []rune(s)
	prevCased := false
	for i, r := range rs {
		if unicode.IsLetter(r) {
			if prevCased {
				rs[i] = unicode.ToLower(r)
			} else {
				rs[i] = unicode.ToUpper(r)
			}
			prevCased = true
		} else {
			prevCased = false
		}
	}
	return string(rs)
}

// pyStrFormat: "{} {0} {nama} {:.2f}".format(...)
func (it *interp) pyStrFormat(f string, args []Value, at ir.Node) string {
	var b strings.Builder
	auto := 0
	for i := 0; i < len(f); i++ {
		c := f[i]
		if c == '{' && i+1 < len(f) && f[i+1] == '{' {
			b.WriteByte('{')
			i++
			continue
		}
		if c == '}' && i+1 < len(f) && f[i+1] == '}' {
			b.WriteByte('}')
			i++
			continue
		}
		if c != '{' {
			b.WriteByte(c)
			continue
		}
		end := strings.IndexByte(f[i:], '}')
		if end < 0 {
			it.throw(at, "ValueError", "Single '{' encountered in format string")
		}
		field := f[i+1 : i+end]
		i += end
		name, spec := field, ""
		if j := strings.IndexAny(field, "!:"); j >= 0 {
			name, spec = field[:j], field[j:]
		}
		var v Value
		switch {
		case name == "":
			v = arg(args, auto)
			auto++
		case name[0] >= '0' && name[0] <= '9':
			idx, _ := strconv.Atoi(strings.SplitN(name, ".", 2)[0])
			v = arg(args, idx)
		default:
			key := strings.SplitN(name, ".", 2)[0]
			if kv, ok := it.pyKw(key); ok {
				v = kv
			} else {
				it.throw(at, "KeyError", pyQuote(key))
			}
		}
		if dot := strings.Index(name, "."); dot >= 0 {
			v = it.getProp(v, name[dot+1:], at)
		}
		b.WriteString(it.pyFormatSpec(v, spec, at))
	}
	return b.String()
}

func (it *interp) pyListMember(x Value, name string, at ir.Node) (Value, bool) {
	l := x.l
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	tuple := l.kind == "tuple"
	switch name {
	case "count":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			n := 0
			for _, e := range l.elems {
				if it.pyEq(e, arg(args, 0)) {
					n++
				}
			}
			return pyInt(int64(n))
		})
	case "index":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for i, e := range l.elems {
				if it.pyEq(e, arg(args, 0)) {
					return pyInt(int64(i))
				}
			}
			what := "list"
			if tuple {
				it.throw(at, "ValueError", "tuple.index(x): x not in tuple")
			}
			it.throw(at, "ValueError", it.pyRepr(arg(args, 0), map[int]bool{})+" is not in "+what)
			return Value{}
		})
	}
	if tuple {
		return Value{}, false
	}
	switch name {
	case "append":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l.elems = append(l.elems, arg(args, 0))
			return null()
		})
	case "appendleft":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l.elems = append([]Value{arg(args, 0)}, l.elems...)
			return null()
		})
	case "extend":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l.elems = append(l.elems, it.pyIter(arg(args, 0), at)...)
			return null()
		})
	case "insert":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			i := int(toF(arg(args, 0)))
			if i < 0 {
				i += len(l.elems)
				if i < 0 {
					i = 0
				}
			}
			if i > len(l.elems) {
				i = len(l.elems)
			}
			l.elems = append(l.elems[:i], append([]Value{arg(args, 1)}, l.elems[i:]...)...)
			return null()
		})
	case "pop", "popleft":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if len(l.elems) == 0 {
				if l.kind == "deque" {
					it.throw(at, "IndexError", "pop from an empty deque")
				}
				it.throw(at, "IndexError", "pop from empty list")
			}
			i := len(l.elems) - 1
			if name == "popleft" {
				i = 0
			}
			if len(args) > 0 {
				i = int(toF(args[0]))
				if i < 0 {
					i += len(l.elems)
				}
				if i < 0 || i >= len(l.elems) {
					it.throw(at, "IndexError", "pop index out of range")
				}
			}
			v := l.elems[i]
			l.elems = append(l.elems[:i:i], l.elems[i+1:]...)
			return v
		})
	case "remove":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for i, e := range l.elems {
				if it.pyEq(e, arg(args, 0)) {
					l.elems = append(l.elems[:i:i], l.elems[i+1:]...)
					return null()
				}
			}
			it.throw(at, "ValueError", "list.remove(x): x not in list")
			return Value{}
		})
	case "sort":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			key, _ := it.pyKw("key")
			rev, hasRev := it.pyKw("reverse")
			it.pySort(l.elems, key, hasRev && it.sem.truthy(rev), at)
			return null()
		})
	case "reverse":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for i, j := 0, len(l.elems)-1; i < j; i, j = i+1, j-1 {
				l.elems[i], l.elems[j] = l.elems[j], l.elems[i]
			}
			return null()
		})
	case "copy":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return it.newList(l.kind, append([]Value(nil), l.elems...))
		})
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l.elems = nil
			return null()
		})
	}
	return Value{}, false
}

func (it *interp) pyDictMember(x Value, name string, at ir.Node) (Value, bool) {
	d := x.d
	if d.kind == "module" {
		return Value{}, false
	}
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	switch name {
	case "get":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if v, ok := d.get(arg(args, 0)); ok {
				return v
			}
			if len(args) > 1 {
				return args[1]
			}
			return null()
		})
	case "keys", "values", "items":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			out := make([]Value, len(d.keys))
			for i, k := range d.keys {
				switch name {
				case "keys":
					out[i] = k
				case "values":
					out[i] = d.vals[i]
				default:
					out[i] = it.newList("tuple", []Value{k, d.vals[i]})
				}
			}
			return it.newList("dict_"+name, out)
		})
	case "pop":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			k := arg(args, 0)
			if v, ok := d.get(k); ok {
				d.del(k)
				return v
			}
			if len(args) > 1 {
				return args[1]
			}
			it.throw(at, "KeyError", it.pyRepr(k, map[int]bool{}))
			return Value{}
		})
	case "popitem":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if d.len() == 0 {
				it.throw(at, "KeyError", "'popitem(): dictionary is empty'")
			}
			i := d.len() - 1
			k, v := d.keys[i], d.vals[i]
			d.del(k)
			return it.newList("tuple", []Value{k, v})
		})
	case "update":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if src := arg(args, 0); src.k == kDict {
				for i, k := range src.d.keys {
					d.set(k, src.d.vals[i])
				}
			} else if src.k == kList {
				for _, p := range src.l.elems {
					if p.k == kList && len(p.l.elems) == 2 {
						d.set(p.l.elems[0], p.l.elems[1])
					}
				}
			}
			for k, v := range it.kwargs {
				d.set(strV(k), v)
			}
			return null()
		})
	case "setdefault":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if v, ok := d.get(arg(args, 0)); ok {
				return v
			}
			v := null()
			if len(args) > 1 {
				v = args[1]
			}
			d.set(arg(args, 0), v)
			return v
		})
	case "copy":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return Value{k: kDict, d: it.cloneDict(d)}
		})
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.keys, d.vals, d.idx = nil, nil, map[string]int{}
			return null()
		})
	case "most_common":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			idx := make([]int, len(d.keys))
			for i := range idx {
				idx[i] = i
			}
			sort.SliceStable(idx, func(a, b int) bool { return toF(d.vals[idx[a]]) > toF(d.vals[idx[b]]) })
			n := len(idx)
			if len(args) > 0 && args[0].k != kNull {
				n = min(n, int(toF(args[0])))
			}
			var out []Value
			for _, i := range idx[:n] {
				out = append(out, it.newList("tuple", []Value{d.keys[i], d.vals[i]}))
			}
			return it.newList("list", out)
		})
	}
	return Value{}, false
}

func (it *interp) pySetMember(x Value, name string, at ir.Node) (Value, bool) {
	d := x.d
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	other := func(v Value) Value {
		if v.k == kSet {
			return v
		}
		nd := it.newDict("set")
		for _, e := range it.pyIter(v, at) {
			nd.set(e, e)
		}
		return Value{k: kSet, d: nd}
	}
	switch name {
	case "add":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.set(arg(args, 0), arg(args, 0))
			return null()
		})
	case "remove":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if !d.del(arg(args, 0)) {
				it.throw(at, "KeyError", it.pyRepr(arg(args, 0), map[int]bool{}))
			}
			return null()
		})
	case "discard":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.del(arg(args, 0))
			return null()
		})
	case "pop":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if d.len() == 0 {
				it.throw(at, "KeyError", "'pop from an empty set'")
			}
			k := pySetOrder(d)[0]
			d.del(k)
			return k
		})
	case "union", "intersection", "difference", "symmetric_difference":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			op := map[string]string{"union": "|", "intersection": "&", "difference": "-", "symmetric_difference": "^"}[name]
			r := x
			for _, a := range args {
				r = it.pySetOp(op, r, other(a))
			}
			if len(args) == 0 {
				r = Value{k: kSet, d: it.cloneDict(d)}
			}
			return r
		})
	case "issubset", "issuperset", "isdisjoint":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			o := other(arg(args, 0))
			a, b := x, o
			if name == "issuperset" {
				a, b = o, x
			}
			if name == "isdisjoint" {
				for _, k := range d.keys {
					if o.d.has(k) {
						return boolV(false)
					}
				}
				return boolV(true)
			}
			for _, k := range a.d.keys {
				if !b.d.has(k) {
					return boolV(false)
				}
			}
			return boolV(true)
		})
	case "update":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for _, a := range args {
				for _, e := range it.pyIter(a, at) {
					d.set(e, e)
				}
			}
			return null()
		})
	case "copy":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return Value{k: kSet, d: it.cloneDict(d)}
		})
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.keys, d.vals, d.idx = nil, nil, map[string]int{}
			return null()
		})
	}
	return Value{}, false
}
