package irsim

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"gopherviz/internal/ir"
)

func (s *javaSem) global(it *interp, name string) (Value, bool) {
	v, ok := s.globals[name]
	return v, ok
}

// jstr mengubah argumen menjadi teks Java (char → huruf).
func (it *interp) jstr(v Value) string { return it.sem.str(it, v) }

func (it *interp) javaObj(c *Class) Value {
	it.ids++
	return Value{k: kObj, o: &Object{class: c, fields: it.newDict("fields"), id: it.ids}}
}

func (it *interp) setupJava(s *javaSem) {
	g := s.globals
	fn := func(c *Class, name string, f builtinFn) { c.statics.set(strV(name), bf(name, f)) }
	cls := func(name string) *Class {
		c := it.nativeClass(name)
		g[name] = Value{k: kClass, c: c}
		return c
	}
	num := func(v Value) float64 { return toF(v) }

	// --- exception ---
	excInit := func(it *interp, this Value, args []Value, at nodeRef) {
		if this.k != kObj {
			return
		}
		msg := null()
		if len(args) > 0 {
			msg = args[0]
			if msg.k == kObj && instanceOf(msg.o.class, "Throwable") { // new RuntimeException(cause)
				this.o.fields.set(strV("cause"), msg)
				msg = strV(it.javaObjString(msg))
			}
		}
		this.o.fields.set(strV("message"), msg)
	}
	mkExc := func(name string, super *Class) *Class {
		c := cls(name)
		c.super, c.kind, c.init = super, "class", excInit
		c.ctor = func(it *interp, args []Value, at nodeRef) Value {
			v := it.javaObj(c)
			excInit(it, v, args, at)
			return v
		}
		return c
	}
	thr := mkExc("Throwable", nil)
	exc := mkExc("Exception", thr)
	mkExc("Error", thr)
	rt := mkExc("RuntimeException", exc)
	mkExc("ArithmeticException", rt)
	ioob := mkExc("IndexOutOfBoundsException", rt)
	mkExc("ArrayIndexOutOfBoundsException", ioob)
	mkExc("StringIndexOutOfBoundsException", ioob)
	mkExc("NullPointerException", rt)
	iae := mkExc("IllegalArgumentException", rt)
	mkExc("NumberFormatException", iae)
	mkExc("IllegalStateException", rt)
	mkExc("UnsupportedOperationException", rt)
	mkExc("ClassCastException", rt)
	mkExc("NegativeArraySizeException", rt)
	nse := mkExc("NoSuchElementException", rt)
	mkExc("InputMismatchException", nse)
	mkExc("ConcurrentModificationException", rt)
	mkExc("IOException", exc)
	mkExc("InterruptedException", exc)
	mkExc("CloneNotSupportedException", exc)
	for _, n := range []string{"MissingFormatArgumentException", "IllegalFormatConversionException", "UnknownFormatConversionException"} {
		mkExc(n, iae)
	}
	cls("Object").kind = "class"

	// --- System.out ---
	ps := it.nativeClass("PrintStream")
	out := it.javaObj(ps)
	errS := it.javaObj(ps)
	errS.o.fields.set(strV("err"), boolV(true))
	in := it.javaObj(it.nativeClass("InputStream"))
	sys := cls("System")
	sys.statics.set(strV("out"), out)
	sys.statics.set(strV("err"), errS)
	sys.statics.set(strV("in"), in)
	fn(sys, "currentTimeMillis", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return unknown("waktu sistem tidak bisa disimulasikan")
	})
	fn(sys, "nanoTime", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return unknown("waktu sistem tidak bisa disimulasikan")
	})
	fn(sys, "exit", func(it *interp, this Value, args []Value, at ir.Node) Value {
		panic(progExit{code: intOf(arg(args, 0))})
	})

	// --- Math ---
	m := cls("Math")
	m.statics.set(strV("PI"), floatV(math.Pi))
	m.statics.set(strV("E"), floatV(math.E))
	d1 := func(name string, f func(float64) float64) {
		fn(m, name, func(it *interp, this Value, args []Value, at ir.Node) Value { return floatV(f(num(arg(args, 0)))) })
	}
	d1("sqrt", math.Sqrt)
	d1("cbrt", math.Cbrt)
	d1("floor", math.Floor)
	d1("ceil", math.Ceil)
	d1("log", math.Log)
	d1("log10", math.Log10)
	d1("exp", math.Exp)
	d1("sin", math.Sin)
	d1("cos", math.Cos)
	d1("tan", math.Tan)
	d1("toRadians", func(x float64) float64 { return x / 180 * math.Pi })
	d1("toDegrees", func(x float64) float64 { return x * 180 / math.Pi })
	d1("signum", func(x float64) float64 {
		switch {
		case x > 0:
			return 1
		case x < 0:
			return -1
		}
		return x
	})
	d1("rint", math.RoundToEven)
	fn(m, "pow", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return floatV(math.Pow(num(arg(args, 0)), num(arg(args, 1))))
	})
	fn(m, "hypot", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return floatV(math.Hypot(num(arg(args, 0)), num(arg(args, 1))))
	})
	fn(m, "atan2", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return floatV(math.Atan2(num(arg(args, 0)), num(arg(args, 1))))
	})
	fn(m, "abs", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := arg(args, 0)
		if v.k == kFloat {
			return Value{k: kFloat, f: math.Abs(v.f), t: v.t}
		}
		if v.t == "long" {
			return jlong(abs64(v.i))
		}
		return jint(abs64(v.i))
	})
	mm := func(name string, wantMax bool) {
		fn(m, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			a, b := arg(args, 0), arg(args, 1)
			rt := numPromote(a, b)
			pick := b
			if wantMax == (toF(a) > toF(b)) || toF(a) == toF(b) {
				pick = a
			}
			return it.sem.convert(it, pick, rt)
		})
	}
	mm("max", true)
	mm("min", false)
	fn(m, "round", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := arg(args, 0)
		if v.t == "float" {
			return jint(javaRound(v.f))
		}
		return jlong(javaRound(num(v)))
	})
	fn(m, "floorDiv", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := intOf(arg(args, 0)), intOf(arg(args, 1))
		if b == 0 {
			it.throw(at, "ArithmeticException", "/ by zero")
		}
		q := a / b
		if (a%b != 0) && ((a < 0) != (b < 0)) {
			q--
		}
		return it.sem.convert(it, jlong(q), numPromote(arg(args, 0), arg(args, 1)))
	})
	fn(m, "floorMod", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := intOf(arg(args, 0)), intOf(arg(args, 1))
		if b == 0 {
			it.throw(at, "ArithmeticException", "/ by zero")
		}
		r := a % b
		if r != 0 && (r < 0) != (b < 0) {
			r += b
		}
		return it.sem.convert(it, jlong(r), numPromote(arg(args, 0), arg(args, 1)))
	})
	fn(m, "random", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.warn("Math.random() disimulasikan dengan seed tetap; hasilnya berbeda dengan saat program dijalankan")
		return floatV(it.rng.Float64())
	})

	// --- pembungkus ---
	parseInt := func(it *interp, s string, radix int, typ string, at ir.Node) Value {
		n, err := strconv.ParseInt(s, radix, 64)
		ti, _ := javaIntType(typ)
		if err != nil || wrap(n, ti) != n {
			it.throw(at, "NumberFormatException", fmt.Sprintf("For input string: \"%s\"", s)+map[bool]string{true: fmt.Sprintf(" under radix %d", radix)}[radix != 10])
		}
		return typedInt(n, typ)
	}
	integer := cls("Integer")
	integer.statics.set(strV("MAX_VALUE"), jint(math.MaxInt32))
	integer.statics.set(strV("MIN_VALUE"), jint(math.MinInt32))
	parse := func(it *interp, this Value, args []Value, at ir.Node) Value {
		radix := 10
		if len(args) > 1 {
			radix = int(intOf(args[1]))
		}
		return parseInt(it, it.jstr(arg(args, 0)), radix, "int", at)
	}
	fn(integer, "parseInt", parse)
	fn(integer, "valueOf", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if v := arg(args, 0); v.k == kStr {
			return parse(it, this, args, at)
		}
		return it.sem.convert(it, arg(args, 0), "int")
	})
	fn(integer, "toString", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if len(args) > 1 {
			return strV(strconv.FormatInt(intOf(args[0]), int(intOf(args[1]))))
		}
		return strV(it.jstr(arg(args, 0)))
	})
	fn(integer, "toBinaryString", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return strV(strconv.FormatUint(uint64(uint32(intOf(arg(args, 0)))), 2))
	})
	fn(integer, "toHexString", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return strV(strconv.FormatUint(uint64(uint32(intOf(arg(args, 0)))), 16))
	})
	fn(integer, "compare", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := intOf(arg(args, 0)), intOf(arg(args, 1))
		switch {
		case a < b:
			return jint(-1)
		case a > b:
			return jint(1)
		}
		return jint(0)
	})
	fn(integer, "sum", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return jint(intOf(arg(args, 0)) + intOf(arg(args, 1)))
	})
	fn(integer, "max", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return jint(max(intOf(arg(args, 0)), intOf(arg(args, 1))))
	})
	fn(integer, "min", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return jint(min(intOf(arg(args, 0)), intOf(arg(args, 1))))
	})
	long := cls("Long")
	long.statics.set(strV("MAX_VALUE"), jlong(math.MaxInt64))
	long.statics.set(strV("MIN_VALUE"), jlong(math.MinInt64))
	fn(long, "parseLong", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return parseInt(it, it.jstr(arg(args, 0)), 10, "long", at)
	})
	dbl := cls("Double")
	dbl.statics.set(strV("MAX_VALUE"), floatV(math.MaxFloat64))
	dbl.statics.set(strV("MIN_VALUE"), floatV(4.9e-324))
	dbl.statics.set(strV("POSITIVE_INFINITY"), floatV(math.Inf(1)))
	dbl.statics.set(strV("NaN"), floatV(math.NaN()))
	pd := func(it *interp, this Value, args []Value, at ir.Node) Value {
		s := strings.TrimSpace(it.jstr(arg(args, 0)))
		f, err := strconv.ParseFloat(strings.TrimRight(s, "dDfF"), 64)
		if err != nil {
			it.throw(at, "NumberFormatException", fmt.Sprintf("For input string: \"%s\"", s))
		}
		return floatV(f)
	}
	fn(dbl, "parseDouble", pd)
	fn(dbl, "valueOf", pd)
	fn(dbl, "compare", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := num(arg(args, 0)), num(arg(args, 1))
		switch {
		case a < b:
			return jint(-1)
		case a > b:
			return jint(1)
		}
		return jint(0)
	})
	fn(dbl, "isNaN", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return boolV(math.IsNaN(num(arg(args, 0))))
	})
	boolC := cls("Boolean")
	fn(boolC, "parseBoolean", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return boolV(strings.EqualFold(it.jstr(arg(args, 0)), "true"))
	})
	ch := cls("Character")
	cpred := func(name string, f func(rune) bool) {
		fn(ch, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			return boolV(f(rune(intOf(arg(args, 0)))))
		})
	}
	cpred("isDigit", unicode.IsDigit)
	cpred("isLetter", unicode.IsLetter)
	cpred("isLetterOrDigit", func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) })
	cpred("isUpperCase", unicode.IsUpper)
	cpred("isLowerCase", unicode.IsLower)
	cpred("isWhitespace", unicode.IsSpace)
	cpred("isAlphabetic", unicode.IsLetter)
	fn(ch, "toUpperCase", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return jchar(int64(unicode.ToUpper(rune(intOf(arg(args, 0))))))
	})
	fn(ch, "toLowerCase", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return jchar(int64(unicode.ToLower(rune(intOf(arg(args, 0))))))
	})
	fn(ch, "getNumericValue", func(it *interp, this Value, args []Value, at ir.Node) Value {
		r := rune(intOf(arg(args, 0)))
		if d := digitVal(byte(r)); r < 128 && d >= 0 {
			return jint(int64(d))
		}
		return jint(-1)
	})
	fn(ch, "toString", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return strV(it.jstr(jchar(intOf(arg(args, 0)))))
	})

	// --- String ---
	str := cls("String")
	fn(str, "valueOf", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := arg(args, 0)
		if v.k == kList && v.l.elem == "char" {
			var b strings.Builder
			for _, e := range v.l.elems {
				b.WriteString(it.jstr(e))
			}
			return strV(b.String())
		}
		return strV(it.jstr(v))
	})
	fn(str, "join", func(it *interp, this Value, args []Value, at ir.Node) Value {
		sep := it.jstr(arg(args, 0))
		items := args[min(1, len(args)):]
		if len(items) == 1 {
			if pairs, ok := it.sem.iterate(it, items[0], nil); ok {
				items = nil
				for _, p := range pairs {
					items = append(items, p.v)
				}
			}
		}
		parts := make([]string, len(items))
		for i, x := range items {
			parts[i] = it.jstr(x)
		}
		return strV(strings.Join(parts, sep))
	})
	fn(str, "format", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return strV(it.javaFormat(it.jstr(arg(args, 0)), args[min(1, len(args)):], at))
	})
	sb := cls("StringBuilder")
	sb.ctor = func(it *interp, args []Value, at nodeRef) Value {
		v := it.javaObj(sb)
		init := ""
		if a := arg(args, 0); a.k == kStr {
			init = a.s
		}
		v.o.fields.set(strV("s"), strV(init))
		return v
	}
	g["StringBuffer"] = g["StringBuilder"]

	// --- koleksi ---
	listCtor := func(kind string) *Class {
		c := cls(kind)
		c.ctor = func(it *interp, args []Value, at nodeRef) Value {
			var elems []Value
			if a := arg(args, 0); a.k == kList || a.k == kSet || a.k == kDict {
				if pairs, ok := it.sem.iterate(it, a, nil); ok {
					for _, p := range pairs {
						elems = append(elems, p.v)
					}
				}
			}
			return it.newList(kind, elems)
		}
		return c
	}
	for _, k := range []string{"ArrayList", "LinkedList", "Stack", "ArrayDeque", "PriorityQueue", "Vector"} {
		listCtor(k)
	}
	g["PriorityQueue"].c.ctor = func(it *interp, args []Value, at nodeRef) Value {
		v := it.newList("PriorityQueue", nil)
		if c := arg(args, 0); c.k == kFunc || c.k == kObj {
			it.sem.(*javaSem).pqCmp[v.l] = c
		} else if c.k == kList {
			v.l.elems = append(v.l.elems, c.l.elems...)
		}
		return v
	}
	mapCtor := func(kind string) {
		c := cls(kind)
		c.ctor = func(it *interp, args []Value, at nodeRef) Value {
			d := it.newDict(kind)
			if a := arg(args, 0); a.k == kDict {
				for _, k := range javaIterOrder(a.d) {
					v, _ := a.d.get(k)
					d.set(k, v)
				}
			}
			return Value{k: kDict, d: d}
		}
	}
	mapCtor("HashMap")
	mapCtor("TreeMap")
	mapCtor("LinkedHashMap")
	g["Hashtable"] = g["HashMap"]
	setCtor := func(kind string) {
		c := cls(kind)
		c.ctor = func(it *interp, args []Value, at nodeRef) Value {
			dk := kind
			if kind == "TreeSet" {
				dk = "set-sorted"
			}
			d := it.newDict(dk)
			if a := arg(args, 0); a.k == kList || a.k == kSet {
				if pairs, ok := it.sem.iterate(it, a, nil); ok {
					for _, p := range pairs {
						d.set(p.v, p.v)
					}
				}
			}
			return Value{k: kSet, d: d}
		}
	}
	setCtor("HashSet")
	setCtor("TreeSet")
	setCtor("LinkedHashSet")
	list := cls("List")
	fn(list, "of", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.newList("ImmutableList", append([]Value(nil), args...))
	})
	setI := cls("Set")
	fn(setI, "of", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.warn("urutan Set.of di Java tidak ditentukan; simulator memakai urutan tulis")
		d := it.newDict("LinkedHashSet")
		for _, a := range args {
			d.set(a, a)
		}
		return Value{k: kSet, d: d}
	})
	mapI := cls("Map")
	fn(mapI, "of", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.warn("urutan Map.of di Java tidak ditentukan; simulator memakai urutan tulis")
		d := it.newDict("LinkedHashMap")
		for i := 0; i+1 < len(args); i += 2 {
			d.set(args[i], args[i+1])
		}
		return Value{k: kDict, d: d}
	})
	fn(mapI, "entry", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.newList("entry", []Value{arg(args, 0), arg(args, 1)})
	})

	// --- Arrays & Collections ---
	arrays := cls("Arrays")
	fn(arrays, "toString", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a := arg(args, 0)
		if a.k == kNull {
			return strV("null")
		}
		parts := make([]string, len(a.l.elems))
		for i, e := range a.l.elems {
			parts[i] = it.jstr(e)
		}
		return strV("[" + strings.Join(parts, ", ") + "]")
	})
	var deep func(v Value) string
	deep = func(v Value) string {
		if v.k != kList {
			return it.jstr(v)
		}
		parts := make([]string, len(v.l.elems))
		for i, e := range v.l.elems {
			parts[i] = deep(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	fn(arrays, "deepToString", func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(deep(arg(args, 0))) })
	fn(arrays, "sort", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a := arg(args, 0)
		lo, hi := 0, len(a.l.elems)
		var cmp Value
		switch {
		case len(args) >= 3 && isNum(args[1]):
			lo, hi = int(intOf(args[1])), int(intOf(args[2]))
		case len(args) == 2:
			cmp = args[1]
		}
		it.javaSort(a.l.elems[lo:hi], cmp, at)
		return undef()
	})
	fn(arrays, "fill", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a := arg(args, 0)
		for i := range a.l.elems {
			a.l.elems[i] = it.sem.convert(it, arg(args, 1), a.l.elem)
		}
		return undef()
	})
	fn(arrays, "asList", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if len(args) == 1 && args[0].k == kList && args[0].l.kind == "array" {
			return it.newList("ArrayList", append([]Value(nil), args[0].l.elems...))
		}
		return it.newList("ArrayList", append([]Value(nil), args...))
	})
	fn(arrays, "copyOf", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a := arg(args, 0)
		n := int(intOf(arg(args, 1)))
		out := make([]Value, n)
		for i := range out {
			if i < len(a.l.elems) {
				out[i] = a.l.elems[i]
			} else {
				out[i] = it.sem.zero(a.l.elem)
			}
		}
		v := it.newList("array", out)
		v.l.elem = a.l.elem
		return v
	})
	fn(arrays, "copyOfRange", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a := arg(args, 0)
		lo, hi := int(intOf(arg(args, 1))), int(intOf(arg(args, 2)))
		v := it.newList("array", append([]Value(nil), a.l.elems[lo:min(hi, len(a.l.elems))]...))
		v.l.elem = a.l.elem
		return v
	})
	fn(arrays, "equals", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := arg(args, 0), arg(args, 1)
		if a.k != kList || b.k != kList || len(a.l.elems) != len(b.l.elems) {
			return boolV(false)
		}
		for i := range a.l.elems {
			if !it.javaEquals(a.l.elems[i], b.l.elems[i]) {
				return boolV(false)
			}
		}
		return boolV(true)
	})
	coll := cls("Collections")
	fn(coll, "sort", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.javaSort(arg(args, 0).l.elems, arg(args, 1), at)
		return undef()
	})
	fn(coll, "reverse", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l := arg(args, 0).l
		for i, j := 0, len(l.elems)-1; i < j; i, j = i+1, j-1 {
			l.elems[i], l.elems[j] = l.elems[j], l.elems[i]
		}
		return undef()
	})
	fn(coll, "swap", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l := arg(args, 0).l
		i, j := int(intOf(arg(args, 1))), int(intOf(arg(args, 2)))
		l.elems[i], l.elems[j] = l.elems[j], l.elems[i]
		return undef()
	})
	extreme := func(name string, wantMax bool) {
		fn(coll, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			items := it.javaItems(arg(args, 0))
			if len(items) == 0 {
				it.throw(at, "NoSuchElementException", "")
			}
			best := items[0]
			for _, x := range items[1:] {
				c := it.javaCompare(x, best, arg(args, 1), at)
				if wantMax && c > 0 || !wantMax && c < 0 {
					best = x
				}
			}
			return best
		})
	}
	extreme("max", true)
	extreme("min", false)
	fn(coll, "frequency", func(it *interp, this Value, args []Value, at ir.Node) Value {
		n := 0
		for _, x := range it.javaItems(arg(args, 0)) {
			if it.javaEquals(x, arg(args, 1)) {
				n++
			}
		}
		return jint(int64(n))
	})
	fn(coll, "shuffle", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.warn("Collections.shuffle disimulasikan dengan seed tetap")
		l := arg(args, 0).l
		it.rng.Shuffle(len(l.elems), func(i, j int) { l.elems[i], l.elems[j] = l.elems[j], l.elems[i] })
		return undef()
	})
	fn(coll, "unmodifiableList", func(it *interp, this Value, args []Value, at ir.Node) Value { return arg(args, 0) })
	fn(coll, "emptyList", func(it *interp, this Value, args []Value, at ir.Node) Value { return it.newList("ImmutableList", nil) })

	// --- lain-lain ---
	sc := cls("Scanner")
	sc.ctor = func(it *interp, args []Value, at nodeRef) Value { return it.javaObj(sc) }
	br := cls("BufferedReader")
	br.ctor = func(it *interp, args []Value, at nodeRef) Value { return it.javaObj(br) }
	isr := cls("InputStreamReader")
	isr.ctor = func(it *interp, args []Value, at nodeRef) Value { return it.javaObj(isr) }
	rnd := cls("Random")
	rnd.ctor = func(it *interp, args []Value, at nodeRef) Value { return it.javaObj(rnd) }
	objs := cls("Objects")
	fn(objs, "equals", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := arg(args, 0), arg(args, 1)
		if a.k == kNull || b.k == kNull {
			return boolV(a.k == b.k)
		}
		return boolV(it.javaEquals(a, b))
	})
	fn(objs, "hash", func(it *interp, this Value, args []Value, at ir.Node) Value {
		h := int32(1)
		for _, a := range args {
			x, _ := javaHash(a)
			h = 31*h + x
		}
		return jint(int64(h))
	})
	fn(objs, "requireNonNull", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if arg(args, 0).k == kNull {
			it.throw(at, "NullPointerException", "")
		}
		return arg(args, 0)
	})
	// --- Comparator ---
	cmpC := cls("Comparator")
	rev := it.javaObj(it.nativeClass("reverseOrder"))
	nat := bf("naturalOrder", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return jint(int64(it.javaCompare(arg(args, 0), arg(args, 1), Value{}, at)))
	})
	fn(cmpC, "reverseOrder", func(it *interp, this Value, args []Value, at ir.Node) Value { return rev })
	fn(coll, "reverseOrder", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if c := arg(args, 0); c.k == kFunc {
			return bf("reversed", func(it *interp, _ Value, a []Value, at ir.Node) Value {
				return jint(-intOf(it.call(c, undef(), a, at)))
			})
		}
		return rev
	})
	fn(cmpC, "naturalOrder", func(it *interp, this Value, args []Value, at ir.Node) Value { return nat })
	comparing := func(it *interp, this Value, args []Value, at ir.Node) Value {
		key := arg(args, 0)
		return bf("comparing", func(it *interp, _ Value, a []Value, at ir.Node) Value {
			x := it.call(key, undef(), []Value{arg(a, 0)}, at)
			y := it.call(key, undef(), []Value{arg(a, 1)}, at)
			return jint(int64(it.javaCompare(x, y, Value{}, at)))
		})
	}
	for _, n := range []string{"comparing", "comparingInt", "comparingDouble", "comparingLong"} {
		fn(cmpC, n, comparing)
	}
	s.comparing = comparing

	thread := cls("Thread")
	fn(thread, "sleep", func(it *interp, this Value, args []Value, at ir.Node) Value { return undef() })
}


func (it *interp) javaItems(v Value) []Value {
	switch v.k {
	case kList:
		return v.l.elems
	case kSet:
		return javaIterOrder(v.d)
	}
	return nil
}

// javaCompare: comparator (lambda/objek) atau urutan alami (Comparable).
func (it *interp) javaCompare(a, b, cmp Value, at ir.Node) int {
	if cmp.k == kFunc {
		return int(intOf(it.call(cmp, undef(), []Value{a, b}, at)))
	}
	if cmp.k == kObj {
		if m := findMethod(cmp.o.class, "compare"); m != nil {
			return int(intOf(it.callFunc(&Func{decl: m, env: cmp.o.class.env, class: it.classOf[m]}, cmp, argList{pos: []Value{a, b}}, at)))
		}
		if cmp.o.class.native == "reverseOrder" {
			return -it.javaCompare(a, b, Value{}, at)
		}
	}
	switch {
	case a.k == kStr && b.k == kStr:
		return javaStrCompare(a.s, b.s)
	case numVal(a) && numVal(b):
		x, y := toF(a), toF(b)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	case a.k == kObj:
		if m := findMethod(a.o.class, "compareTo"); m != nil {
			return int(intOf(it.callFunc(&Func{decl: m, env: a.o.class.env, class: it.classOf[m]}, a, argList{pos: []Value{b}}, at)))
		}
		if a.o.class.kind == "enum" && b.k == kObj {
			x, _ := a.o.fields.get(strV("ordinal"))
			y, _ := b.o.fields.get(strV("ordinal"))
			return int(x.i - y.i)
		}
	}
	it.throw(at, "ClassCastException", "class cannot be cast to class java.lang.Comparable")
	return 0
}

func javaStrCompare(a, b string) int {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return int(ua[i]) - int(ub[i])
		}
	}
	return len(ua) - len(ub)
}

func (it *interp) javaSort(elems []Value, cmp Value, at ir.Node) {
	sort.SliceStable(elems, func(i, j int) bool { return it.javaCompare(elems[i], elems[j], cmp, at) < 0 })
}

// javaSplit meniru String.split: regex, buang string kosong di akhir (limit 0).
func javaSplit(s, pattern string, limit int) []string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return []string{s}
	}
	n := -1
	if limit > 0 {
		n = limit
	}
	parts := re.Split(s, n)
	if len(parts) > 1 && parts[0] == "" && re.FindStringIndex(s) != nil && re.FindStringIndex(s)[1] > 0 && re.FindStringIndex(s)[0] == 0 {
		parts = parts[1:] // kecocokan lebar-nol di awal tidak menghasilkan elemen kosong (Java 8+)
	}
	if limit == 0 {
		for len(parts) > 0 && parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
		if len(parts) == 0 {
			return []string{""}
		}
	}
	return parts
}

// ---------- member bawaan ----------

func (s *javaSem) member(it *interp, x Value, name string, at ir.Node) (Value, bool) {
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	switch x.k {
	case kStr:
		return it.javaStrMember(x, name, at)
	case kList:
		if x.l.kind == "array" {
			if name == "length" {
				return jint(int64(len(x.l.elems))), true
			}
			if name == "clone" {
				return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
					v := it.newList("array", append([]Value(nil), this.l.elems...))
					v.l.elem = this.l.elem
					return v
				})
			}
		}
		if x.l.kind == "entry" {
			switch name {
			case "getKey":
				return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return this.l.elems[0] })
			case "getValue":
				return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return this.l.elems[1] })
			}
		}
		return it.javaListMember(x, name, at)
	case kDict:
		return it.javaMapMember(x, name, at)
	case kSet:
		return it.javaSetMember(x, name, at)
	case kInt, kFloat, kBool:
		switch name {
		case "equals":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				return boolV(it.javaEquals(this, arg(args, 0)))
			})
		case "compareTo":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				return jint(int64(it.javaCompare(this, arg(args, 0), Value{}, at)))
			})
		case "intValue":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return it.sem.convert(it, this, "int") })
		case "doubleValue":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return floatV(toF(this)) })
		case "toString":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(it.jstr(this)) })
		case "hashCode":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				h, _ := javaHash(this)
				return jint(int64(h))
			})
		}
	case kFunc: // antarmuka fungsional: compare/apply/accept/test/run/get
		switch name {
		case "compare", "apply", "accept", "test", "run", "get", "applyAsInt", "applyAsDouble", "call":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return it.call(this, undef(), args, at) })
		case "reversed":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				f := this
				return bf("reversed", func(it *interp, _ Value, a []Value, at ir.Node) Value {
					return jint(-intOf(it.call(f, undef(), a, at)))
				})
			})
		case "thenComparing", "thenComparingInt", "thenComparingDouble":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				first, next := this, arg(args, 0)
				if next.k == kFunc && next.fn.decl != nil && len(next.fn.decl.Params) == 1 || name != "thenComparing" {
					next = it.sem.(*javaSem).comparing(it, undef(), []Value{next}, at) // thenComparing(Orang::getNama)
				}
				return bf("thenComparing", func(it *interp, _ Value, a []Value, at ir.Node) Value {
					if r := intOf(it.call(first, undef(), a, at)); r != 0 {
						return jint(r)
					}
					return jint(int64(it.javaCompare(arg(a, 0), arg(a, 1), next, at)))
				})
			})
		}
	case kClass:
		c := x.c
		switch name {
		case "values":
			if c.kind == "enum" {
				return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
					var out []Value
					for _, v := range c.statics.vals {
						if v.k == kObj && v.o.class == c {
							out = append(out, v)
						}
					}
					l := it.newList("array", out)
					l.l.elem = c.name
					return l
				})
			}
		case "valueOf":
			if c.kind == "enum" {
				return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
					if v, ok := c.statics.get(strV(it.jstr(arg(args, 0)))); ok && v.k == kObj {
						return v
					}
					it.throw(at, "IllegalArgumentException", "No enum constant "+c.name+"."+it.jstr(arg(args, 0)))
					return Value{}
				})
			}
		case "getSimpleName", "getName":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(c.name) })
		}
	case kObj:
		return it.javaObjMember(x, name, at)
	}
	return Value{}, false
}

func (it *interp) javaObjMember(x Value, name string, at ir.Node) (Value, bool) {
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	c := x.o.class
	switch {
	case c.native == "PrintStream":
		toErr := x.o.fields.has(strV("err"))
		write := func(s string) {
			if toErr {
				it.warn("output ke System.err tidak ditampilkan di output program")
				return
			}
			it.out(s)
		}
		switch name {
		case "println":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				if len(args) == 0 {
					write("\n")
				} else {
					write(it.javaPrintStr(args[0]) + "\n")
				}
				return undef()
			})
		case "print":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				write(it.javaPrintStr(arg(args, 0)))
				return undef()
			})
		case "printf", "format":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				write(it.javaFormat(it.jstr(arg(args, 0)), args[min(1, len(args)):], at))
				return this
			})
		case "flush":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return undef() })
		}
	case c.native == "StringBuilder":
		get := func(this Value) string { v, _ := this.o.fields.get(strV("s")); return v.s }
		set := func(this Value, s string) { this.o.fields.set(strV("s"), strV(s)) }
		switch name {
		case "append":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				set(this, get(this)+it.jstr(arg(args, 0)))
				return this
			})
		case "toString":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(get(this)) })
		case "length":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				return jint(int64(len(utf16.Encode([]rune(get(this))))))
			})
		case "reverse":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				rs := []rune(get(this))
				for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
					rs[i], rs[j] = rs[j], rs[i]
				}
				set(this, string(rs))
				return this
			})
		case "insert":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				rs := []rune(get(this))
				i := min(int(intOf(arg(args, 0))), len(rs))
				set(this, string(rs[:i])+it.jstr(arg(args, 1))+string(rs[i:]))
				return this
			})
		case "charAt":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				rs := []rune(get(this))
				i := int(intOf(arg(args, 0)))
				if i < 0 || i >= len(rs) {
					it.throw(at, "StringIndexOutOfBoundsException", fmt.Sprintf("index %d,length %d", i, len(rs)))
				}
				return jchar(int64(rs[i]))
			})
		case "setCharAt":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				rs := []rune(get(this))
				i := int(intOf(arg(args, 0)))
				if i >= 0 && i < len(rs) {
					rs[i] = rune(intOf(arg(args, 1)))
				}
				set(this, string(rs))
				return undef()
			})
		case "deleteCharAt":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				rs := []rune(get(this))
				i := int(intOf(arg(args, 0)))
				if i < 0 || i >= len(rs) {
					it.throw(at, "StringIndexOutOfBoundsException", fmt.Sprintf("index %d,length %d", i, len(rs)))
				}
				set(this, string(rs[:i])+string(rs[i+1:]))
				return this
			})
		case "delete":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				rs := []rune(get(this))
				a, b := int(intOf(arg(args, 0))), min(int(intOf(arg(args, 1))), len(rs))
				set(this, string(rs[:a])+string(rs[b:]))
				return this
			})
		case "setLength":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				rs := []rune(get(this))
				n := int(intOf(arg(args, 0)))
				if n < len(rs) {
					set(this, string(rs[:n]))
				}
				return undef()
			})
		case "indexOf":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				return jint(int64(strings.Index(get(this), it.jstr(arg(args, 0)))))
			})
		}
	case c.native == "Scanner" || c.native == "BufferedReader":
		// input user: diisi angka acak (package userinput), ditampilkan seperti diketik di terminal
		read := func(at ir.Node, line bool) string {
			var s string
			ok := true
			if line {
				s, ok = it.readLine()
			} else {
				s, ok = it.readToken()
			}
			if !ok {
				it.throw(at, "NoSuchElementException", "")
			}
			return s
		}
		num := func(at ir.Node) int64 { n, _ := strconv.ParseInt(read(at, false), 10, 64); return n }
		switch name {
		case "nextInt", "nextShort", "nextByte":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return jint(num(at)) })
		case "nextLong":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return jlong(num(at)) })
		case "nextDouble", "nextFloat":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return floatV(float64(num(at))) })
		case "nextBoolean":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(num(at)%2 == 0) })
		case "next":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(read(at, false)) })
		case "nextLine":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(read(at, true)) })
		case "readLine":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				if s, ok := it.readLine(); ok {
					return strV(s)
				}
				return null()
			})
		case "hasNext", "hasNextInt", "hasNextLine", "hasNextDouble", "hasNextLong", "ready":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(it.inputLeft()) })
		case "close":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return undef() })
		}
	case c.native == "Random":
		it.warn("Random disimulasikan dengan seed tetap; hasilnya berbeda dengan saat program dijalankan")
		switch name {
		case "nextInt":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				if len(args) == 0 {
					return jint(int64(int32(it.rng.Uint32())))
				}
				lo, hi := int64(0), intOf(args[0])
				if len(args) > 1 {
					lo, hi = intOf(args[0]), intOf(args[1])
				}
				return jint(lo + it.rng.Int63n(hi-lo))
			})
		case "nextDouble":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return floatV(it.rng.Float64()) })
		case "nextBoolean":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(it.rng.Intn(2) == 1) })
		}
	case instanceOf(c, "Throwable"):
		switch name {
		case "getMessage", "getLocalizedMessage":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				v, _ := this.o.fields.get(strV("message"))
				return v
			})
		case "getCause":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				v, ok := this.o.fields.get(strV("cause"))
				if !ok {
					return null()
				}
				return v
			})
		case "printStackTrace":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				it.warn("printStackTrace menulis ke System.err (tidak ditampilkan)")
				return undef()
			})
		}
	case c.kind == "enum":
		switch name {
		case "name", "toString":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				v, _ := this.o.fields.get(strV("name"))
				return v
			})
		case "ordinal":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				v, _ := this.o.fields.get(strV("ordinal"))
				return v
			})
		case "compareTo":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				return jint(int64(it.javaCompare(this, arg(args, 0), Value{}, at)))
			})
		}
	}
	switch name {
	case "equals":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return boolV(it.javaEquals(this, arg(args, 0)))
		})
	case "toString":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(it.javaObjString(this)) })
	case "hashCode":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return jint(int64(0x1b6d3586 + this.o.id*0x2f))
		})
	case "getClass":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return Value{k: kClass, c: this.o.class} })
	}
	return Value{}, false
}

// javaPrintStr: println(char[]) mencetak isinya; nilai lain lewat String.valueOf.
func (it *interp) javaPrintStr(v Value) string {
	if v.k == kList && v.l.kind == "array" && v.l.elem == "char" {
		var b strings.Builder
		for _, e := range v.l.elems {
			b.WriteString(it.jstr(e))
		}
		return b.String()
	}
	return it.jstr(v)
}

func (it *interp) javaStrMember(x Value, name string, at ir.Node) (Value, bool) {
	s := x.s
	u := utf16.Encode([]rune(s))
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	sub := func(a, b int) string { return string(utf16.Decode(u[a:b])) }
	idx16 := func(byteIdx int) int { return len(utf16.Encode([]rune(s[:byteIdx]))) }
	argStr := func(v Value) string { return it.jstr(v) }
	switch name {
	case "length":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return jint(int64(len(u))) })
	case "charAt":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			i := int(intOf(arg(args, 0)))
			if i < 0 || i >= len(u) {
				it.throw(at, "StringIndexOutOfBoundsException", fmt.Sprintf("Index %d out of bounds for length %d", i, len(u)))
			}
			return jchar(int64(u[i]))
		})
	case "isEmpty":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(len(u) == 0) })
	case "isBlank":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(strings.TrimSpace(s) == "") })
	case "substring":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			a := int(intOf(arg(args, 0)))
			b := len(u)
			if len(args) > 1 {
				b = int(intOf(args[1]))
			}
			if a < 0 || b > len(u) || a > b {
				it.throw(at, "StringIndexOutOfBoundsException", fmt.Sprintf("begin %d, end %d, length %d", a, b, len(u)))
			}
			return strV(sub(a, b))
		})
	case "indexOf", "lastIndexOf":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			needle := argStr(arg(args, 0))
			var i int
			if name == "indexOf" {
				from := 0
				if len(args) > 1 {
					from = max(0, int(intOf(args[1])))
				}
				if from > len(u) {
					return jint(-1)
				}
				rest := sub(from, len(u))
				i = strings.Index(rest, needle)
				if i >= 0 {
					return jint(int64(from + len(utf16.Encode([]rune(rest[:i])))))
				}
				return jint(-1)
			}
			i = strings.LastIndex(s, needle)
			if i < 0 {
				return jint(-1)
			}
			return jint(int64(idx16(i)))
		})
	case "contains":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return boolV(strings.Contains(s, argStr(arg(args, 0))))
		})
	case "equals":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			o := arg(args, 0)
			return boolV(o.k == kStr && o.s == s)
		})
	case "equalsIgnoreCase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			o := arg(args, 0)
			return boolV(o.k == kStr && strings.EqualFold(o.s, s))
		})
	case "compareTo":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return jint(int64(javaStrCompare(s, argStr(arg(args, 0)))))
		})
	case "compareToIgnoreCase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return jint(int64(javaStrCompare(strings.ToLower(s), strings.ToLower(argStr(arg(args, 0))))))
		})
	case "toUpperCase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.ToUpper(s)) })
	case "toLowerCase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.ToLower(s)) })
	case "trim", "strip":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.TrimSpace(s)) })
	case "startsWith":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return boolV(strings.HasPrefix(s, argStr(arg(args, 0))))
		})
	case "endsWith":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return boolV(strings.HasSuffix(s, argStr(arg(args, 0))))
		})
	case "concat":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(s + argStr(arg(args, 0))) })
	case "repeat":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			n := int(intOf(arg(args, 0)))
			if n < 0 {
				it.throw(at, "IllegalArgumentException", "count is negative: "+strconv.Itoa(n))
			}
			return strV(strings.Repeat(s, n))
		})
	case "replace":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return strV(strings.ReplaceAll(s, argStr(arg(args, 0)), argStr(arg(args, 1))))
		})
	case "replaceAll", "replaceFirst":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			re, err := regexp.Compile(argStr(arg(args, 0)))
			if err != nil {
				it.structural("regex ini belum didukung simulator")
			}
			rep := strings.ReplaceAll(argStr(arg(args, 1)), "$", "$$")
			for i := 9; i >= 0; i-- {
				rep = strings.ReplaceAll(rep, "$$"+strconv.Itoa(i), "${"+strconv.Itoa(i)+"}")
			}
			if name == "replaceFirst" {
				done := false
				return strV(re.ReplaceAllStringFunc(s, func(mm string) string {
					if done {
						return mm
					}
					done = true
					return re.ReplaceAllString(mm, rep)
				}))
			}
			return strV(re.ReplaceAllString(s, rep))
		})
	case "matches":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			re, err := regexp.Compile("^(?:" + argStr(arg(args, 0)) + ")$")
			return boolV(err == nil && re.MatchString(s))
		})
	case "split":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			limit := 0
			if len(args) > 1 {
				limit = int(intOf(args[1]))
			}
			parts := javaSplit(s, argStr(arg(args, 0)), limit)
			out := make([]Value, len(parts))
			for i, p := range parts {
				out[i] = strV(p)
			}
			l := it.newList("array", out)
			l.l.elem = "String"
			return l
		})
	case "toCharArray":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			out := make([]Value, len(u))
			for i, c := range u {
				out[i] = jchar(int64(c))
			}
			l := it.newList("array", out)
			l.l.elem = "char"
			return l
		})
	case "hashCode":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			h, _ := javaHash(this)
			return jint(int64(h))
		})
	case "intern", "toString":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return this })
	case "chars":
		it.structural("stream (chars()) belum disimulasikan")
	}
	return Value{}, false
}

func (it *interp) javaListMember(x Value, name string, at ir.Node) (Value, bool) {
	l := x.l
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	immutable := func() {
		if l.kind == "ImmutableList" {
			it.throw(at, "UnsupportedOperationException", "")
		}
	}
	checkIdx := func(i, n int) {
		if i < 0 || i >= n {
			it.throw(at, "IndexOutOfBoundsException", fmt.Sprintf("Index %d out of bounds for length %d", i, n))
		}
	}
	empty := func() {
		if len(l.elems) == 0 {
			switch l.kind {
			case "Stack":
				it.throw(at, "EmptyStackException", "")
			default:
				it.throw(at, "NoSuchElementException", "")
			}
		}
	}
	pqHead := func() int {
		best := 0
		cmp := it.sem.(*javaSem).pqCmp[l]
		for i := range l.elems {
			if it.javaCompare(l.elems[i], l.elems[best], cmp, at) < 0 {
				best = i
			}
		}
		return best
	}
	switch name {
	case "size":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return jint(int64(len(l.elems))) })
	case "isEmpty", "empty":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(len(l.elems) == 0) })
	case "add", "addLast", "offer", "offerLast":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			immutable()
			if len(args) == 2 {
				i := int(intOf(args[0]))
				if i < 0 || i > len(l.elems) {
					it.throw(at, "IndexOutOfBoundsException", fmt.Sprintf("Index: %d, Size: %d", i, len(l.elems)))
				}
				l.elems = append(l.elems[:i], append([]Value{args[1]}, l.elems[i:]...)...)
				return undef()
			}
			l.elems = append(l.elems, arg(args, 0))
			return boolV(true)
		})
	case "addFirst", "offerFirst":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l.elems = append([]Value{arg(args, 0)}, l.elems...)
			return undef()
		})
	case "push":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if l.kind == "ArrayDeque" || l.kind == "LinkedList" {
				l.elems = append([]Value{arg(args, 0)}, l.elems...)
			} else {
				l.elems = append(l.elems, arg(args, 0))
			}
			return arg(args, 0)
		})
	case "pop":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			empty()
			if l.kind == "Stack" {
				v := l.elems[len(l.elems)-1]
				l.elems = l.elems[:len(l.elems)-1]
				return v
			}
			v := l.elems[0]
			l.elems = append([]Value(nil), l.elems[1:]...)
			return v
		})
	case "peek", "peekFirst", "element", "getFirst":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if len(l.elems) == 0 {
				if name == "peek" || name == "peekFirst" {
					if l.kind == "Stack" {
						it.throw(at, "EmptyStackException", "")
					}
					return null()
				}
				empty()
			}
			switch l.kind {
			case "Stack":
				return l.elems[len(l.elems)-1]
			case "PriorityQueue":
				return l.elems[pqHead()]
			}
			return l.elems[0]
		})
	case "poll", "pollFirst", "removeFirst":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if len(l.elems) == 0 {
				if name == "removeFirst" {
					empty()
				}
				return null()
			}
			i := 0
			if l.kind == "PriorityQueue" {
				i = pqHead()
			}
			v := l.elems[i]
			l.elems = append(l.elems[:i:i], l.elems[i+1:]...)
			return v
		})
	case "peekLast", "getLast":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if len(l.elems) == 0 {
				return null()
			}
			return l.elems[len(l.elems)-1]
		})
	case "pollLast", "removeLast":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if len(l.elems) == 0 {
				return null()
			}
			v := l.elems[len(l.elems)-1]
			l.elems = l.elems[:len(l.elems)-1]
			return v
		})
	case "get":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			i := int(intOf(arg(args, 0)))
			checkIdx(i, len(l.elems))
			return l.elems[i]
		})
	case "set":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			immutable()
			i := int(intOf(arg(args, 0)))
			checkIdx(i, len(l.elems))
			old := l.elems[i]
			l.elems[i] = arg(args, 1)
			return old
		})
	case "remove":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			immutable()
			if len(args) == 0 {
				empty()
				v := l.elems[0]
				l.elems = append([]Value(nil), l.elems[1:]...)
				return v
			}
			a := args[0]
			if a.k == kInt && a.t != "char" && l.kind != "HashSet" { // remove(int index)
				i := int(a.i)
				checkIdx(i, len(l.elems))
				v := l.elems[i]
				l.elems = append(l.elems[:i:i], l.elems[i+1:]...)
				return v
			}
			for i, e := range l.elems {
				if it.javaEquals(e, a) {
					l.elems = append(l.elems[:i:i], l.elems[i+1:]...)
					return boolV(true)
				}
			}
			return boolV(false)
		})
	case "contains":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for _, e := range l.elems {
				if it.javaEquals(e, arg(args, 0)) {
					return boolV(true)
				}
			}
			return boolV(false)
		})
	case "indexOf", "lastIndexOf":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			found := -1
			for i, e := range l.elems {
				if it.javaEquals(e, arg(args, 0)) {
					found = i
					if name == "indexOf" {
						break
					}
				}
			}
			return jint(int64(found))
		})
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			immutable()
			l.elems = nil
			return undef()
		})
	case "addAll":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			immutable()
			l.elems = append(l.elems, it.javaItems(arg(args, 0))...)
			return boolV(true)
		})
	case "sort":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			immutable()
			it.javaSort(l.elems, arg(args, 0), at)
			return undef()
		})
	case "forEach":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for i := 0; i < len(l.elems); i++ {
				it.call(arg(args, 0), undef(), []Value{l.elems[i]}, at)
			}
			return undef()
		})
	case "removeIf":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			var keep []Value
			removed := false
			for _, e := range l.elems {
				if it.sem.truthy(it.call(arg(args, 0), undef(), []Value{e}, at)) {
					removed = true
					continue
				}
				keep = append(keep, e)
			}
			l.elems = keep
			return boolV(removed)
		})
	case "toString":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(it.jstr(this)) })
	case "equals":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return boolV(it.javaEquals(this, arg(args, 0)))
		})
	case "subList":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			a, b := int(intOf(arg(args, 0))), int(intOf(arg(args, 1)))
			return it.newList("ArrayList", append([]Value(nil), l.elems[a:b]...))
		})
	case "toArray":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return it.newList("array", append([]Value(nil), l.elems...))
		})
	case "stream":
		it.structural("Stream API belum disimulasikan")
	}
	return Value{}, false
}

func (it *interp) javaMapMember(x Value, name string, at ir.Node) (Value, bool) {
	d := x.d
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	order := func() []Value { return javaIterOrder(d) }
	switch name {
	case "put":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			old, ok := d.get(arg(args, 0))
			d.set(arg(args, 0), arg(args, 1))
			if !ok {
				return null()
			}
			return old
		})
	case "get":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if v, ok := d.get(arg(args, 0)); ok {
				return v
			}
			return null()
		})
	case "getOrDefault":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if v, ok := d.get(arg(args, 0)); ok {
				return v
			}
			return arg(args, 1)
		})
	case "containsKey":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.has(arg(args, 0))) })
	case "containsValue":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for _, v := range d.vals {
				if it.javaEquals(v, arg(args, 0)) {
					return boolV(true)
				}
			}
			return boolV(false)
		})
	case "remove":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			old, ok := d.get(arg(args, 0))
			d.del(arg(args, 0))
			if !ok {
				return null()
			}
			return old
		})
	case "size":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return jint(int64(d.len())) })
	case "isEmpty":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.len() == 0) })
	case "putIfAbsent":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if v, ok := d.get(arg(args, 0)); ok && v.k != kNull {
				return v
			}
			d.set(arg(args, 0), arg(args, 1))
			return null()
		})
	case "merge":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			k, v := arg(args, 0), arg(args, 1)
			if old, ok := d.get(k); ok && old.k != kNull {
				v = it.call(arg(args, 2), undef(), []Value{old, v}, at)
			}
			d.set(k, v)
			return v
		})
	case "keySet":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			nd := it.newDict("LinkedHashSet")
			for _, k := range order() {
				nd.set(k, k)
			}
			return Value{k: kSet, d: nd}
		})
	case "values":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			var out []Value
			for _, k := range order() {
				v, _ := d.get(k)
				out = append(out, v)
			}
			return it.newList("ArrayList", out)
		})
	case "entrySet":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			var out []Value
			for _, k := range order() {
				v, _ := d.get(k)
				out = append(out, it.newList("entry", []Value{k, v}))
			}
			return it.newList("ArrayList", out)
		})
	case "forEach":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for _, k := range order() {
				v, _ := d.get(k)
				it.call(arg(args, 0), undef(), []Value{k, v}, at)
			}
			return undef()
		})
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.keys, d.vals, d.idx = nil, nil, map[string]int{}
			return undef()
		})
	case "toString":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(it.jstr(this)) })
	case "firstKey", "lastKey":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if d.len() == 0 {
				it.throw(at, "NoSuchElementException", "")
			}
			if name == "firstKey" {
				return d.keys[0]
			}
			return d.keys[d.len()-1]
		})
	}
	return Value{}, false
}

func (it *interp) javaSetMember(x Value, name string, at ir.Node) (Value, bool) {
	d := x.d
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	switch name {
	case "add":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if d.has(arg(args, 0)) {
				return boolV(false)
			}
			d.set(arg(args, 0), arg(args, 0))
			return boolV(true)
		})
	case "contains":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.has(arg(args, 0))) })
	case "remove":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.del(arg(args, 0))) })
	case "size":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return jint(int64(d.len())) })
	case "isEmpty":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.len() == 0) })
	case "addAll":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for _, v := range it.javaItems(arg(args, 0)) {
				d.set(v, v)
			}
			return boolV(true)
		})
	case "retainAll":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			other := it.javaItems(arg(args, 0))
			for _, k := range append([]Value(nil), d.keys...) {
				keep := false
				for _, o := range other {
					if it.javaEquals(k, o) {
						keep = true
					}
				}
				if !keep {
					d.del(k)
				}
			}
			return boolV(true)
		})
	case "forEach":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for _, k := range javaIterOrder(d) {
				it.call(arg(args, 0), undef(), []Value{k}, at)
			}
			return undef()
		})
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.keys, d.vals, d.idx = nil, nil, map[string]int{}
			return undef()
		})
	case "toString":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(it.jstr(this)) })
	case "first", "last":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if d.len() == 0 {
				it.throw(at, "NoSuchElementException", "")
			}
			if name == "first" {
				return d.keys[0]
			}
			return d.keys[d.len()-1]
		})
	}
	return Value{}, false
}
