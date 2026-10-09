package irsim

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopherviz/internal/ir"
)

func (s *cSem) global(it *interp, name string) (Value, bool) {
	v, ok := s.globals[strings.TrimPrefix(name, "std::")]
	return v, ok
}

// progExit dipanggil exit(): program berhenti normal.
type progExit struct{ code int64 }

func (it *interp) nativeClass(name string) *Class {
	c := &Class{name: name, native: name, methods: map[string]*Fn{}, getters: map[string]*Fn{}, setters: map[string]*Fn{},
		statics: it.newDict("statics"), kind: "native"}
	it.classes[name] = c
	return c
}

func (it *interp) newStream(name string) Value {
	c := it.classes["ostream"]
	it.ids++
	o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
	o.fields.set(strV("stream"), strV(name))
	o.fields.set(strV("precision"), typedInt(6, "int"))
	o.fields.set(strV("width"), typedInt(0, "int"))
	o.fields.set(strV("floatfield"), strV(""))
	o.fields.set(strV("adjust"), strV("right"))
	o.fields.set(strV("boolalpha"), boolV(false))
	o.fields.set(strV("fill"), typedInt(' ', "char"))
	return Value{k: kObj, o: o}
}

func (it *interp) manip(name string, arg Value) Value {
	c := it.classes["__manip"]
	it.ids++
	o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
	o.fields.set(strV("name"), strV(name))
	o.fields.set(strV("arg"), arg)
	return Value{k: kObj, o: o}
}

// templateArgs: "map<string, int>" → ["string", "int"].
func templateArgs(t string) []string {
	i := strings.IndexByte(t, '<')
	j := strings.LastIndexByte(t, '>')
	if i < 0 || j < i {
		return nil
	}
	var out []string
	depth, start := 0, i+1
	for k := i + 1; k < j; k++ {
		switch t[k] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(t[start:k]))
				start = k + 1
			}
		}
	}
	return append(out, strings.TrimSpace(t[start:j]))
}

// rangeOf: (awal, akhir) iterator/pointer → list dan indeks.
func (it *interp) rangeOf(a, b Value, at ir.Node) (*List, int, int) {
	toPtr := func(v Value) *Ptr {
		switch v.k {
		case kPtr:
			return v.p
		case kList:
			return &Ptr{list: v.l}
		}
		it.structural("rentang iterator ini belum disimulasikan")
		return nil
	}
	pa, pb := toPtr(a), toPtr(b)
	if pa.list == nil || pa.list != pb.list {
		it.structural("rentang iterator harus dari wadah yang sama")
	}
	lo, hi := pa.idx, pb.idx
	if lo < 0 || hi > len(pa.list.elems) || lo > hi {
		it.cCrash(at, "rentang iterator di luar batas")
	}
	return pa.list, lo, hi
}

func (it *interp) cLess(a, b Value, cmp Value, at ir.Node) bool {
	if cmp.k == kFunc {
		return it.sem.truthy(it.call(cmp, undef(), []Value{a, b}, at))
	}
	if cmp.k == kStr && cmp.s == "greater" {
		return it.sem.truthy(it.sem.binary(it, ">", a, b, at))
	}
	if a.k == kList && b.k == kList { // pair / vector: leksikografis
		for i := 0; i < len(a.l.elems) && i < len(b.l.elems); i++ {
			if it.cLess(a.l.elems[i], b.l.elems[i], Value{}, at) {
				return true
			}
			if it.cLess(b.l.elems[i], a.l.elems[i], Value{}, at) {
				return false
			}
		}
		return len(a.l.elems) < len(b.l.elems)
	}
	return it.sem.truthy(it.sem.binary(it, "<", a, b, at))
}

func (it *interp) setupC(s *cSem) {
	g := s.globals
	fn := func(name string, f builtinFn) { g[name] = bf(name, f) }
	num := func(v Value) float64 { return toF(v) }
	ci := func(n int64) Value { return typedInt(n, "int") }

	// --- konstanta ---
	g["INT_MAX"], g["INT_MIN"] = ci(math.MaxInt32), ci(math.MinInt32)
	g["UINT_MAX"] = typedInt(math.MaxUint32, "unsigned int")
	g["LONG_MAX"], g["LONG_MIN"] = typedInt(math.MaxInt32, "long"), typedInt(math.MinInt32, "long")
	g["LLONG_MAX"], g["LLONG_MIN"] = typedInt(math.MaxInt64, "long long"), typedInt(math.MinInt64, "long long")
	g["CHAR_MAX"], g["CHAR_MIN"] = ci(127), ci(-128)
	g["SHRT_MAX"] = ci(32767)
	g["RAND_MAX"] = ci(32767)
	g["EXIT_SUCCESS"], g["EXIT_FAILURE"] = ci(0), ci(1)
	g["M_PI"], g["M_E"] = floatV(math.Pi), floatV(math.E)
	g["NULL"] = null()
	g["EOF"] = ci(-1)
	g["FLT_MAX"], g["DBL_MAX"] = floatV(math.MaxFloat32), floatV(math.MaxFloat64)
	g["true"], g["false"] = it.cBool(true), it.cBool(false)

	// --- output C ---
	fn("printf", func(it *interp, this Value, args []Value, at ir.Node) Value {
		out := it.cPrintf(cString(arg(args, 0)), args[min(1, len(args)):], at)
		it.out(out)
		return ci(int64(len(out)))
	})
	fn("puts", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.out(cString(arg(args, 0)) + "\n")
		return ci(1)
	})
	fn("putchar", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.out(string([]byte{byte(intOf(arg(args, 0)))}))
		return arg(args, 0)
	})
	writeBuf := func(dst Value, s string, at ir.Node) {
		var l *List
		off := 0
		switch dst.k {
		case kList:
			l = dst.l
		case kPtr:
			l, off = dst.p.list, dst.p.idx
		}
		if l == nil {
			it.structural("tujuan string bukan array char")
		}
		if off+len(s)+1 > len(l.elems) {
			it.cCrash(at, "buffer overflow: string lebih panjang dari array tujuan")
		}
		for i := 0; i < len(s); i++ {
			l.elems[off+i] = typedInt(int64(int8(s[i])), "char")
		}
		l.elems[off+len(s)] = typedInt(0, "char")
	}
	fn("sprintf", func(it *interp, this Value, args []Value, at ir.Node) Value {
		out := it.cPrintf(cString(arg(args, 1)), args[min(2, len(args)):], at)
		writeBuf(arg(args, 0), out, at)
		return ci(int64(len(out)))
	})
	fn("snprintf", func(it *interp, this Value, args []Value, at ir.Node) Value {
		out := it.cPrintf(cString(arg(args, 2)), args[min(3, len(args)):], at)
		n := int(intOf(arg(args, 1)))
		w := out
		if n > 0 && len(w) >= n {
			w = w[:n-1]
		}
		writeBuf(arg(args, 0), w, at)
		return ci(int64(len(out)))
	})
	// Input user: diisi angka acak (package userinput), ditampilkan seperti diketik di terminal.
	fn("scanf", func(it *interp, this Value, args []Value, at ir.Node) Value {
		n := 0
		for i, conv := range scanfConvs(it.sem.str(it, arg(args, 0))) {
			if i+1 >= len(args) {
				break
			}
			var s string
			ok := true
			if conv == 'c' {
				var c byte
				c, ok = it.stdinChar()
				s = string([]byte{c})
			} else {
				s, ok = it.readToken()
			}
			if !ok {
				if n == 0 {
					return ci(-1) // EOF
				}
				break
			}
			switch dst := args[i+1]; {
			case conv == 's':
				writeBuf(dst, s, at)
			case dst.k == kPtr:
				it.storeInput(dst.p, s)
			default:
				it.structural("argumen scanf harus alamat variabel, misalnya &x")
			}
			n++
		}
		return ci(int64(n))
	})
	fn("getchar", func(it *interp, this Value, args []Value, at ir.Node) Value {
		c, ok := it.stdinChar()
		if !ok {
			return ci(-1) // EOF
		}
		return ci(int64(c))
	})
	fn("gets", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s, ok := it.readLine()
		if !ok {
			return null()
		}
		writeBuf(arg(args, 0), s, at)
		return arg(args, 0)
	})
	fn("fgets", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s, ok := it.readLine()
		if !ok {
			return null()
		}
		s += "\n"
		if max := int(intOf(arg(args, 1))) - 1; max >= 0 && len(s) > max {
			s = s[:max]
		}
		writeBuf(arg(args, 0), s, at)
		return arg(args, 0)
	})
	for _, name := range []string{"getline", "fscanf", "sscanf_s"} {
		nm := name
		fn(nm, func(it *interp, this Value, args []Value, at ir.Node) Value {
			it.structural("membaca input dari user (" + nm + ") belum didukung simulator")
			return Value{}
		})
	}
	fn("exit", func(it *interp, this Value, args []Value, at ir.Node) Value {
		panic(progExit{code: intOf(arg(args, 0))})
	})
	fn("system", func(it *interp, this Value, args []Value, at ir.Node) Value { return ci(0) })

	// --- string C ---
	fn("strlen", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return typedInt(int64(len(cString(arg(args, 0)))), "size_t")
	})
	fn("strcpy", func(it *interp, this Value, args []Value, at ir.Node) Value {
		writeBuf(arg(args, 0), cString(arg(args, 1)), at)
		return arg(args, 0)
	})
	fn("strncpy", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s := cString(arg(args, 1))
		n := int(intOf(arg(args, 2)))
		if len(s) > n {
			s = s[:n]
		}
		writeBuf(arg(args, 0), s, at)
		return arg(args, 0)
	})
	fn("strcat", func(it *interp, this Value, args []Value, at ir.Node) Value {
		writeBuf(arg(args, 0), cString(arg(args, 0))+cString(arg(args, 1)), at)
		return arg(args, 0)
	})
	fn("strcmp", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return ci(int64(strings.Compare(cString(arg(args, 0)), cString(arg(args, 1)))))
	})
	fn("strncmp", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := cString(arg(args, 0)), cString(arg(args, 1))
		n := int(intOf(arg(args, 2)))
		if len(a) > n {
			a = a[:n]
		}
		if len(b) > n {
			b = b[:n]
		}
		return ci(int64(strings.Compare(a, b)))
	})
	ctype := func(name string, f func(rune) bool) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			if f(rune(byte(intOf(arg(args, 0))))) {
				return ci(1)
			}
			return ci(0)
		})
	}
	ctype("isdigit", unicode.IsDigit)
	ctype("isalpha", unicode.IsLetter)
	ctype("isalnum", func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) })
	ctype("isspace", unicode.IsSpace)
	ctype("isupper", unicode.IsUpper)
	ctype("islower", unicode.IsLower)
	ctype("ispunct", unicode.IsPunct)
	fn("toupper", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return ci(int64(unicode.ToUpper(rune(byte(intOf(arg(args, 0)))))))
	})
	fn("tolower", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return ci(int64(unicode.ToLower(rune(byte(intOf(arg(args, 0)))))))
	})
	fn("atoi", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v, _ := phpNumeric(strings.TrimSpace(cString(arg(args, 0))))
		return ci(int64(int32(toF(v))))
	})
	g["atol"] = g["atoi"]
	fn("atof", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v, _ := phpNumeric(cString(arg(args, 0)))
		return floatV(toF(v))
	})

	// --- math ---
	m1 := func(name string, f func(float64) float64) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value { return floatV(f(num(arg(args, 0)))) })
	}
	m1("sqrt", math.Sqrt)
	m1("cbrt", math.Cbrt)
	m1("floor", math.Floor)
	m1("ceil", math.Ceil)
	m1("round", func(x float64) float64 { return math.Round(x) })
	m1("trunc", math.Trunc)
	m1("fabs", math.Abs)
	m1("log", math.Log)
	m1("log10", math.Log10)
	m1("log2", math.Log2)
	m1("exp", math.Exp)
	m1("sin", math.Sin)
	m1("cos", math.Cos)
	m1("tan", math.Tan)
	m1("atan", math.Atan)
	fn("pow", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return floatV(math.Pow(num(arg(args, 0)), num(arg(args, 1))))
	})
	fn("fmod", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return floatV(math.Mod(num(arg(args, 0)), num(arg(args, 1))))
	})
	fn("hypot", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return floatV(math.Hypot(num(arg(args, 0)), num(arg(args, 1))))
	})
	fn("atan2", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return floatV(math.Atan2(num(arg(args, 0)), num(arg(args, 1))))
	})
	fn("abs", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := arg(args, 0)
		if v.k == kFloat {
			return floatV(math.Abs(v.f))
		}
		n := intOf(v)
		if n < 0 {
			n = -n
		}
		return typedInt(n, promote(cTypeOf(v), "int"))
	})
	g["labs"], g["llabs"] = g["abs"], g["abs"]
	fn("rand", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.warn("rand() disimulasikan dengan seed tetap; hasilnya berbeda dengan saat program dijalankan")
		return ci(it.rng.Int63n(32768))
	})
	fn("srand", func(it *interp, this Value, args []Value, at ir.Node) Value { return undef() })
	fn("time", func(it *interp, this Value, args []Value, at ir.Node) Value { return typedInt(1700000000, "long long") })

	// --- memori ---
	fn("malloc", func(it *interp, this Value, args []Value, at ir.Node) Value {
		n := intOf(arg(args, 0))
		if n > 10_000_000 {
			it.structural("alokasi memori terlalu besar")
		}
		it.ids++
		return Value{k: kPtr, p: &Ptr{list: &List{kind: "carray", elem: "bytes:" + strconv.FormatInt(n, 10), id: it.ids}, name: "heap"}}
	})
	fn("calloc", func(it *interp, this Value, args []Value, at ir.Node) Value {
		n := intOf(arg(args, 0)) * intOf(arg(args, 1))
		it.ids++
		return Value{k: kPtr, p: &Ptr{list: &List{kind: "carray", elem: "bytes:" + strconv.FormatInt(n, 10), id: it.ids}, name: "heap"}}
	})
	fn("realloc", func(it *interp, this Value, args []Value, at ir.Node) Value {
		p := arg(args, 0)
		n := intOf(arg(args, 1))
		if p.k != kPtr || p.p.list == nil {
			return g["malloc"].fn.builtin(it, this, args[1:], at)
		}
		l := p.p.list
		size := int64(4)
		if sz := it.sizeOf(&ir.TypeRef{Name: l.elem}, it.globals); sz.k == kInt && sz.i > 0 {
			size = sz.i
		}
		want := int(n / size)
		for len(l.elems) < want {
			l.elems = append(l.elems, it.zeroOfType(l.elem, nil))
		}
		l.elems = l.elems[:want]
		return p
	})
	fn("free", func(it *interp, this Value, args []Value, at ir.Node) Value { return undef() })
	fn("memset", func(it *interp, this Value, args []Value, at ir.Node) Value {
		p := arg(args, 0)
		if p.k == kList {
			for i := range p.l.elems {
				p.l.elems[i] = it.sem.convert(it, typedInt(intOf(arg(args, 1)), "int"), p.l.elem)
			}
		}
		return p
	})

	if !s.cpp {
		return
	}

	// ---------- C++ ----------
	it.nativeClass("ostream")
	it.nativeClass("istream")
	it.nativeClass("__manip")
	g["cout"] = it.newStream("cout")
	g["cerr"] = it.newStream("cerr")
	g["clog"] = it.newStream("cerr")
	g["cin"] = Value{k: kObj, o: &Object{class: it.classes["istream"], fields: it.newDict("fields")}}
	for _, m := range []string{"endl", "fixed", "scientific", "defaultfloat", "left", "right", "internal", "boolalpha", "noboolalpha", "showpoint", "flush"} {
		g[m] = it.manip(m, Value{})
	}
	for _, m := range []string{"setw", "setprecision", "setfill"} {
		name := m
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value { return it.manip(name, arg(args, 0)) })
	}
	g["npos"] = typedInt(-1, "size_t")
	g["ios"] = it.moduleV(map[string]Value{"fixed": g["fixed"], "scientific": g["scientific"], "left": g["left"], "right": g["right"], "boolalpha": g["boolalpha"]})
	g["ios_base"] = g["ios"]

	// exception
	excInit := func(it *interp, this Value, args []Value, at nodeRef) {
		if this.k == kObj {
			msg := ""
			if len(args) > 0 {
				msg = it.cppStrOperand(args[0])
			}
			this.o.fields.set(strV("message"), strV(msg))
		}
	}
	mkExc := func(name string, super *Class) *Class {
		c := it.nativeClass(name)
		c.super = super
		c.kind = "class"
		c.init = excInit
		c.ctor = func(it *interp, args []Value, at nodeRef) Value {
			it.ids++
			o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
			v := Value{k: kObj, o: o}
			excInit(it, v, args, at)
			if len(args) == 0 && name == "exception" {
				o.fields.set(strV("message"), strV("std::exception"))
			}
			return v
		}
		g[name] = Value{k: kClass, c: c}
		return c
	}
	exc := mkExc("exception", nil)
	logic := mkExc("logic_error", exc)
	mkExc("invalid_argument", logic)
	mkExc("out_of_range", logic)
	mkExc("length_error", logic)
	mkExc("domain_error", logic)
	rt := mkExc("runtime_error", exc)
	mkExc("overflow_error", rt)
	mkExc("underflow_error", rt)
	mkExc("range_error", rt)
	mkExc("bad_alloc", exc)

	// kontainer
	container := func(name string, mk func(it *interp, typ string, args []Value, at ir.Node) Value) {
		c := it.nativeClass(name)
		c.ctorT = mk
		c.ctor = func(it *interp, args []Value, at nodeRef) Value { return mk(it, name, args, at) }
		g[name] = Value{k: kClass, c: c}
	}
	listFrom := func(kind, typ string) func(it *interp, t string, args []Value, at ir.Node) Value {
		return func(it *interp, t string, args []Value, at ir.Node) Value {
			elem := ""
			if ta := templateArgs(t); len(ta) > 0 {
				elem = ta[0]
			}
			var elems []Value
			switch {
			case len(args) == 0:
			case len(args) == 1 && args[0].k == kList && (args[0].l.kind == "init" || args[0].l.kind == kind || args[0].l.kind == "carray"):
				for _, e := range args[0].l.elems {
					if elem != "" {
						e = it.sem.convert(it, e, elem)
					}
					elems = append(elems, e)
				}
			case len(args) == 2 && (args[0].k == kPtr || args[0].k == kList) && (args[1].k == kPtr || args[1].k == kList):
				l, lo, hi := it.rangeOf(args[0], args[1], at)
				elems = append(elems, l.elems[lo:hi]...)
			case isNum(args[0]):
				n := int(intOf(args[0]))
				if n < 0 || n > 10_000_000 {
					it.structural("ukuran wadah tidak valid")
				}
				fill := it.zeroOfType(elem, at)
				if len(args) > 1 {
					fill = args[1]
				}
				for i := 0; i < n; i++ {
					elems = append(elems, copyFor(it, fill))
				}
			}
			v := it.newList(kind, elems)
			v.l.elem = elem
			if strings.Contains(t, "greater") {
				v.l.kind = "pq-min"
			}
			return v
		}
	}
	container("vector", listFrom("vector", "vector"))
	container("deque", listFrom("vector", "deque"))
	container("list", listFrom("vector", "list"))
	container("stack", listFrom("stack", "stack"))
	container("queue", listFrom("queue", "queue"))
	container("priority_queue", listFrom("pq", "priority_queue"))
	container("array", listFrom("vector", "array"))
	container("string", func(it *interp, t string, args []Value, at ir.Node) Value {
		switch len(args) {
		case 0:
			return strV("")
		case 1:
			return strV(it.cppStrOperand(args[0]))
		}
		if isNum(args[0]) {
			return strV(strings.Repeat(it.cppStrOperand(args[1]), int(intOf(args[0]))))
		}
		return strV(it.cppStrOperand(args[0]))
	})
	it.classes["string"].statics.set(strV("npos"), typedInt(-1, "size_t"))
	mapLike := func(kind string, set bool) func(it *interp, t string, args []Value, at ir.Node) Value {
		return func(it *interp, t string, args []Value, at ir.Node) Value {
			d := it.newDict(kind)
			if len(args) == 1 && args[0].k == kList {
				for _, e := range args[0].l.elems {
					if set {
						d.set(e, e)
					} else if e.k == kList && len(e.l.elems) == 2 {
						d.set(e.l.elems[0], e.l.elems[1])
					}
				}
			}
			if kind == "umap" || kind == "uset" {
				it.warn("urutan unordered_map/unordered_set di simulator mengikuti urutan sisip; di C++ asli urutannya tidak ditentukan")
			}
			if set {
				return Value{k: kSet, d: d, t: t}
			}
			return Value{k: kDict, d: d, t: t}
		}
	}
	container("map", mapLike("map", false))
	container("unordered_map", mapLike("umap", false))
	container("set", mapLike("set-sorted", true))
	container("unordered_set", mapLike("uset", true))
	container("pair", func(it *interp, t string, args []Value, at ir.Node) Value {
		// pair<int,int> p = {5, 6}: daftar inisialisasi = first, second
		if len(args) == 1 && args[0].k == kList && args[0].l.kind != "pair" && len(args[0].l.elems) == 2 {
			return it.cppPair(args[0].l.elems[0], args[0].l.elems[1])
		}
		return it.cppPair(arg(args, 0), arg(args, 1))
	})

	// fungsi bebas
	fn("make_pair", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.cppPair(arg(args, 0), arg(args, 1))
	})
	fn("to_string", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := arg(args, 0)
		if v.k == kFloat {
			return strV(strconv.FormatFloat(v.f, 'f', 6, 64))
		}
		return strV(strconv.FormatInt(intOf(v), 10))
	})
	stoi := func(name string, float bool) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			s := strings.TrimSpace(it.cppStrOperand(arg(args, 0)))
			v, kind := phpNumeric(s)
			if kind == 0 {
				it.throw(at, "invalid_argument", name)
			}
			if float {
				return floatV(toF(v))
			}
			return ci(int64(int32(toF(v))))
		})
	}
	stoi("stoi", false)
	stoi("stol", false)
	stoi("stoll", false)
	stoi("stod", true)
	stoi("stof", true)
	fn("max", func(it *interp, this Value, args []Value, at ir.Node) Value {
		items := args
		if len(args) == 1 && args[0].k == kList {
			items = args[0].l.elems
		}
		best := items[0]
		for _, x := range items[1:] {
			if it.cLess(best, x, Value{}, at) {
				best = x
			}
		}
		return best
	})
	fn("min", func(it *interp, this Value, args []Value, at ir.Node) Value {
		items := args
		if len(args) == 1 && args[0].k == kList {
			items = args[0].l.elems
		}
		best := items[0]
		for _, x := range items[1:] {
			if it.cLess(x, best, Value{}, at) {
				best = x
			}
		}
		return best
	})
	fn("sort", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
		cmp := arg(args, 2)
		part := l.elems[lo:hi]
		sort.SliceStable(part, func(i, j int) bool { return it.cLess(part[i], part[j], cmp, at) })
		return undef()
	})
	g["stable_sort"] = g["sort"]
	fn("reverse", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
		for i, j := lo, hi-1; i < j; i, j = i+1, j-1 {
			l.elems[i], l.elems[j] = l.elems[j], l.elems[i]
		}
		return undef()
	})
	fn("find", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
		for i := lo; i < hi; i++ {
			if it.sem.truthy(it.sem.binary(it, "==", l.elems[i], arg(args, 2), at)) {
				return Value{k: kPtr, p: &Ptr{list: l, idx: i}}
			}
		}
		return Value{k: kPtr, p: &Ptr{list: l, idx: hi}}
	})
	fn("count", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
		n := 0
		for i := lo; i < hi; i++ {
			if it.sem.truthy(it.sem.binary(it, "==", l.elems[i], arg(args, 2), at)) {
				n++
			}
		}
		return typedInt(int64(n), "long long")
	})
	fn("accumulate", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
		acc := arg(args, 2)
		for i := lo; i < hi; i++ {
			if f := arg(args, 3); f.k == kFunc {
				acc = it.call(f, undef(), []Value{acc, l.elems[i]}, at)
			} else {
				acc = it.sem.convert(it, it.sem.binary(it, "+", acc, l.elems[i], at), cTypeOf(acc))
			}
		}
		return acc
	})
	minmaxEl := func(name string, wantMax bool) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
			best := lo
			for i := lo + 1; i < hi; i++ {
				if wantMax && it.cLess(l.elems[best], l.elems[i], arg(args, 2), at) || !wantMax && it.cLess(l.elems[i], l.elems[best], arg(args, 2), at) {
					best = i
				}
			}
			return Value{k: kPtr, p: &Ptr{list: l, idx: best}}
		})
	}
	minmaxEl("max_element", true)
	minmaxEl("min_element", false)
	fn("fill", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
		for i := lo; i < hi; i++ {
			l.elems[i] = arg(args, 2)
		}
		return undef()
	})
	fn("iota", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
		v := arg(args, 2)
		for i := lo; i < hi; i++ {
			l.elems[i] = v
			v = it.sem.binary(it, "+", v, ci(1), at)
		}
		return undef()
	})
	// lower_bound/upper_bound/binary_search: pencarian biner persis seperti libstdc++ (lihat probe.LowerBound)
	bound := func(args []Value, at ir.Node, upper bool) (*List, int, int) {
		l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
		first, count := lo, hi-lo
		for count > 0 {
			step := count / 2
			i := first + step
			if it.cLess(l.elems[i], arg(args, 2), arg(args, 3), at) || upper && !it.cLess(arg(args, 2), l.elems[i], arg(args, 3), at) {
				first, count = i+1, count-step-1
			} else {
				count = step
			}
		}
		return l, first, hi
	}
	fn("lower_bound", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, i, _ := bound(args, at, false)
		return Value{k: kPtr, p: &Ptr{list: l, idx: i}}
	})
	fn("upper_bound", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, i, _ := bound(args, at, true)
		return Value{k: kPtr, p: &Ptr{list: l, idx: i}}
	})
	fn("binary_search", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, i, hi := bound(args, at, false)
		return boolV(i < hi && !it.cLess(arg(args, 2), l.elems[i], arg(args, 3), at))
	})
	fn("find_if", func(it *interp, this Value, args []Value, at ir.Node) Value {
		l, lo, hi := it.rangeOf(arg(args, 0), arg(args, 1), at)
		for i := lo; i < hi; i++ {
			if it.sem.truthy(it.call(arg(args, 2), undef(), []Value{l.elems[i]}, at)) {
				return Value{k: kPtr, p: &Ptr{list: l, idx: i}}
			}
		}
		return Value{k: kPtr, p: &Ptr{list: l, idx: hi}}
	})
	fn("distance", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := arg(args, 0), arg(args, 1)
		if a.k == kPtr && b.k == kPtr {
			return typedInt(int64(b.p.idx-a.p.idx), "long long")
		}
		return typedInt(0, "long long")
	})
	fn("greater", func(it *interp, this Value, args []Value, at ir.Node) Value { return strV("greater") })
	g["greater"] = Value{k: kClass, c: &Class{name: "greater", native: "greater", statics: it.newDict("statics"),
		ctor: func(it *interp, args []Value, at nodeRef) Value { return strV("greater") }}}
	lim := it.nativeClass("numeric_limits")
	lim.statics.set(strV("max"), bf("max", func(it *interp, this Value, args []Value, at ir.Node) Value { return ci(math.MaxInt32) }))
	lim.statics.set(strV("min"), bf("min", func(it *interp, this Value, args []Value, at ir.Node) Value { return ci(math.MinInt32) }))
	g["numeric_limits"] = Value{k: kClass, c: lim}
}

// ---------- member bawaan C++ ----------

func (s *cSem) member(it *interp, x Value, name string, at ir.Node) (Value, bool) {
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	if x.k == kObj && x.o.class.native == "istream" {
		failed := func(this Value) bool { f, _ := this.o.fields.get(strV("fail")); return f.k == kBool && f.b }
		switch name {
		case "ignore", "clear", "sync":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				switch name {
				case "clear":
					this.o.fields.set(strV("fail"), boolV(false))
				case "ignore": // buang Enter yang tertinggal setelah cin >> x
					if it.pendingNL {
						it.pendingNL = false
					} else if it.stdinBuf != "" {
						it.stdinBuf = ""
					}
				}
				return undef()
			})
		case "get":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				c, ok := it.stdinChar()
				if !ok {
					return typedInt(-1, "int")
				}
				return typedInt(int64(c), "char")
			})
		case "fail", "eof":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(failed(this)) })
		case "good":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(!failed(this)) })
		}
	}
	size := func(n int) Value { return typedInt(int64(n), "size_t") }
	switch x.k {
	case kStr:
		str := x.s
		switch name {
		case "size", "length":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return size(len(str)) })
		case "empty":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(str == "") })
		case "substr":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				pos := int(intOf(arg(args, 0)))
				if pos > len(str) {
					it.throw(at, "out_of_range", "basic_string::substr: __pos (which is "+strconv.Itoa(pos)+") > this->size() (which is "+strconv.Itoa(len(str))+")")
				}
				end := len(str)
				if len(args) > 1 {
					n := intOf(args[1])
					if n >= 0 && pos+int(n) < end {
						end = pos + int(n)
					}
				}
				return strV(str[pos:end])
			})
		case "find", "rfind":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				sub := it.cppStrOperand(arg(args, 0))
				from := 0
				if len(args) > 1 {
					from = int(intOf(args[1]))
				}
				var i int
				if name == "find" {
					if from > len(str) {
						return typedInt(-1, "size_t")
					}
					i = strings.Index(str[from:], sub)
					if i >= 0 {
						i += from
					}
				} else {
					i = strings.LastIndex(str, sub)
				}
				return typedInt(int64(i), "size_t")
			})
		case "at":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				i := int(intOf(arg(args, 0)))
				if i < 0 || i >= len(str) {
					it.throw(at, "out_of_range", fmt.Sprintf("basic_string::at: __n (which is %d) >= this->size() (which is %d)", i, len(str)))
				}
				return typedInt(int64(int8(str[i])), "char")
			})
		case "front", "back":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				if str == "" {
					return typedInt(0, "char")
				}
				if name == "front" {
					return typedInt(int64(str[0]), "char")
				}
				return typedInt(int64(str[len(str)-1]), "char")
			})
		case "c_str", "data", "str":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(str) })
		case "compare":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				return typedInt(int64(strings.Compare(str, it.cppStrOperand(arg(args, 0)))), "int")
			})
		case "begin":
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				it.structural("iterator string belum disimulasikan")
				return Value{}
			})
		}
	case kList:
		return it.cppListMember(x, name, at)
	case kDict:
		return it.cppMapMember(x, name, at)
	case kSet:
		return it.cppSetMember(x, name, at)
	case kObj:
		if instanceOf(x.o.class, "exception") && name == "what" {
			return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
				v, _ := this.o.fields.get(strV("message"))
				return v
			})
		}
		if x.o.class.native == "ostream" {
			switch name {
			case "precision":
				return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
					old := it.streamState(this, "precision")
					if len(args) > 0 {
						this.o.fields.set(strV("precision"), args[0])
					}
					return old
				})
			case "setf", "unsetf", "flush", "put", "write":
				return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
					if name == "unsetf" {
						this.o.fields.set(strV("floatfield"), strV(""))
					}
					return this
				})
			}
		}
	}
	return Value{}, false
}

// cppStringMutate menjalankan method string yang mengubah isi; hasil baru ditulis
// kembali ke variabelnya oleh evalCall.
func (it *interp) cppStringMutate(s, name string, args []Value, at ir.Node) (string, Value, bool) {
	switch name {
	case "push_back":
		return s + it.cppStrOperand(arg(args, 0)), undef(), true
	case "pop_back":
		if s == "" {
			return s, undef(), true
		}
		return s[:len(s)-1], undef(), true
	case "append":
		ns := s + it.cppStrOperand(arg(args, 0))
		return ns, strV(ns), true
	case "insert":
		pos := min(int(intOf(arg(args, 0))), len(s))
		ns := s[:pos] + it.cppStrOperand(arg(args, 1)) + s[pos:]
		return ns, strV(ns), true
	case "erase":
		pos := 0
		if len(args) > 0 {
			pos = min(int(intOf(args[0])), len(s))
		}
		end := len(s)
		if len(args) > 1 {
			end = min(len(s), pos+int(intOf(args[1])))
		}
		ns := s[:pos] + s[end:]
		return ns, strV(ns), true
	case "replace":
		pos := min(int(intOf(arg(args, 0))), len(s))
		end := min(len(s), pos+int(intOf(arg(args, 1))))
		ns := s[:pos] + it.cppStrOperand(arg(args, 2)) + s[end:]
		return ns, strV(ns), true
	case "clear":
		return "", undef(), true
	case "resize":
		n := int(intOf(arg(args, 0)))
		if n < len(s) {
			return s[:n], undef(), true
		}
		fill := "\x00"
		if len(args) > 1 {
			fill = it.cppStrOperand(args[1])
		}
		return s + strings.Repeat(fill, n-len(s)), undef(), true
	}
	return s, Value{}, false
}

func (it *interp) cppListMember(x Value, name string, at ir.Node) (Value, bool) {
	l := x.l
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	size := func(n int) Value { return typedInt(int64(n), "size_t") }
	if l.kind == "pair" {
		switch name {
		case "first":
			return l.elems[0], true
		case "second":
			return l.elems[1], true
		}
	}
	pqTop := func() int {
		best := 0
		for i := range l.elems {
			if l.kind == "pq-min" && it.cLess(l.elems[i], l.elems[best], Value{}, at) ||
				l.kind != "pq-min" && it.cLess(l.elems[best], l.elems[i], Value{}, at) {
				best = i
			}
		}
		return best
	}
	empty := func(what string) {
		if len(l.elems) == 0 {
			it.cCrash(at, what+" pada wadah kosong (perilaku tidak terdefinisi)")
		}
	}
	switch name {
	case "size":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return size(len(l.elems)) })
	case "empty":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(len(l.elems) == 0) })
	case "push_back", "emplace_back", "push", "emplace":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			v := arg(args, 0)
			if name == "emplace_back" && len(args) > 1 {
				v = it.constructType(l.elem, args, at)
			} else if l.elem != "" {
				v = it.sem.convert(it, v, l.elem)
			}
			l.elems = append(l.elems, copyFor(it, v))
			return undef()
		})
	case "push_front":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l.elems = append([]Value{copyFor(it, arg(args, 0))}, l.elems...)
			return undef()
		})
	case "pop_back":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			empty("pop_back")
			l.elems = l.elems[:len(l.elems)-1]
			return undef()
		})
	case "pop", "pop_front":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			empty(name)
			switch {
			case l.kind == "stack":
				l.elems = l.elems[:len(l.elems)-1]
			case l.kind == "pq" || l.kind == "pq-min":
				i := pqTop()
				l.elems = append(l.elems[:i:i], l.elems[i+1:]...)
			default:
				l.elems = append([]Value(nil), l.elems[1:]...)
			}
			return undef()
		})
	case "top":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			empty("top")
			if l.kind == "pq" || l.kind == "pq-min" {
				return l.elems[pqTop()]
			}
			return l.elems[len(l.elems)-1]
		})
	case "front":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			empty("front")
			return l.elems[0]
		})
	case "back":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			empty("back")
			return l.elems[len(l.elems)-1]
		})
	case "at":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			i := int(intOf(arg(args, 0)))
			if i < 0 || i >= len(l.elems) {
				it.throw(at, "out_of_range", fmt.Sprintf("vector::_M_range_check: __n (which is %d) >= this->size() (which is %d)", i, len(l.elems)))
			}
			return l.elems[i]
		})
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l.elems = nil
			return undef()
		})
	case "begin", "cbegin":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return Value{k: kPtr, p: &Ptr{list: l, idx: 0}}
		})
	case "end", "cend":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return Value{k: kPtr, p: &Ptr{list: l, idx: len(l.elems)}}
		})
	case "insert":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			pos := arg(args, 0)
			if pos.k != kPtr || pos.p.list != l {
				it.structural("insert dengan posisi ini belum disimulasikan")
			}
			i := pos.p.idx
			l.elems = append(l.elems[:i], append([]Value{copyFor(it, arg(args, 1))}, l.elems[i:]...)...)
			return Value{k: kPtr, p: &Ptr{list: l, idx: i}}
		})
	case "erase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			a := arg(args, 0)
			if a.k != kPtr || a.p.list != l {
				it.structural("erase dengan posisi ini belum disimulasikan")
			}
			lo, hi := a.p.idx, a.p.idx+1
			if b := arg(args, 1); b.k == kPtr {
				hi = b.p.idx
			}
			l.elems = append(l.elems[:lo:lo], l.elems[hi:]...)
			return Value{k: kPtr, p: &Ptr{list: l, idx: lo}}
		})
	case "resize":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			n := int(intOf(arg(args, 0)))
			fill := it.zeroOfType(l.elem, at)
			if len(args) > 1 {
				fill = args[1]
			}
			for len(l.elems) < n {
				l.elems = append(l.elems, copyFor(it, fill))
			}
			l.elems = l.elems[:n]
			return undef()
		})
	case "reserve", "shrink_to_fit":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return undef() })
	case "capacity":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return size(len(l.elems)) })
	case "assign":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			n := int(intOf(arg(args, 0)))
			l.elems = nil
			for i := 0; i < n; i++ {
				l.elems = append(l.elems, copyFor(it, arg(args, 1)))
			}
			return undef()
		})
	}
	return Value{}, false
}

func (it *interp) cppMapMember(x Value, name string, at ir.Node) (Value, bool) {
	d := x.d
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	iter := func(i int) Value { return Value{k: kPtr, p: &Ptr{dict: d, idx: i, iter: true}} }
	switch name {
	case "size":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return typedInt(int64(d.len()), "size_t")
		})
	case "empty":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.len() == 0) })
	case "count", "contains":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if name == "contains" {
				return boolV(d.has(arg(args, 0)))
			}
			if d.has(arg(args, 0)) {
				return typedInt(1, "size_t")
			}
			return typedInt(0, "size_t")
		})
	case "find":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if i, ok := d.idx[keyID(arg(args, 0))]; ok {
				return iter(i)
			}
			return iter(d.len())
		})
	case "begin":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return iter(0) })
	case "end":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return iter(d.len()) })
	case "insert", "emplace":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			k, v := arg(args, 0), arg(args, 1)
			if len(args) == 1 && k.k == kList && len(k.l.elems) == 2 {
				k, v = k.l.elems[0], k.l.elems[1]
			}
			if !d.has(k) {
				d.set(k, v)
				return boolV(true)
			}
			return boolV(false)
		})
	case "erase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			k := arg(args, 0)
			if k.k == kPtr && k.p.iter {
				if k.p.idx < d.len() {
					d.del(d.keys[k.p.idx])
				}
				return iter(k.p.idx)
			}
			if d.del(k) {
				return typedInt(1, "size_t")
			}
			return typedInt(0, "size_t")
		})
	case "at":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if v, ok := d.get(arg(args, 0)); ok {
				return v
			}
			it.throw(at, "out_of_range", "map::at")
			return Value{}
		})
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.keys, d.vals, d.idx = nil, nil, map[string]int{}
			return undef()
		})
	}
	return Value{}, false
}

func (it *interp) cppSetMember(x Value, name string, at ir.Node) (Value, bool) {
	d := x.d
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	iter := func(i int) Value { return Value{k: kPtr, p: &Ptr{dict: d, idx: i, iter: true}} }
	switch name {
	case "size":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return typedInt(int64(d.len()), "size_t")
		})
	case "empty":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.len() == 0) })
	case "insert", "emplace":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.set(arg(args, 0), arg(args, 0))
			return undef()
		})
	case "count", "contains":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if name == "contains" {
				return boolV(d.has(arg(args, 0)))
			}
			if d.has(arg(args, 0)) {
				return typedInt(1, "size_t")
			}
			return typedInt(0, "size_t")
		})
	case "erase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if d.del(arg(args, 0)) {
				return typedInt(1, "size_t")
			}
			return typedInt(0, "size_t")
		})
	case "find":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if i, ok := d.idx[keyID(arg(args, 0))]; ok {
				return iter(i)
			}
			return iter(d.len())
		})
	case "begin":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return iter(0) })
	case "end":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return iter(d.len()) })
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.keys, d.vals, d.idx = nil, nil, map[string]int{}
			return undef()
		})
	}
	return Value{}, false
}

// scanfConvs mengembalikan huruf konversi scanf yang mengisi argumen (%d, %s, %lf → 'f', ...).
func scanfConvs(format string) []byte {
	var out []byte
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i < len(format) && format[i] == '%' {
			continue
		}
		skip := i < len(format) && format[i] == '*'
		for i < len(format) && strings.IndexByte("*0123456789hlLqjzt", format[i]) >= 0 {
			i++
		}
		if i >= len(format) {
			break
		}
		c := format[i]
		if c == '[' {
			for i < len(format) && format[i] != ']' {
				i++
			}
			c = 's'
		}
		if !skip {
			out = append(out, c)
		}
	}
	return out
}
