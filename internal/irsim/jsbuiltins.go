package irsim

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf16"

	"gopherviz/internal/ir"
)

type builtinFn = func(it *interp, this Value, args []Value, at ir.Node) Value

func bf(name string, f builtinFn) Value {
	return Value{k: kFunc, fn: &Func{name: name, builtin: func(it *interp, this Value, args []Value, at nodeRef) Value {
		return f(it, this, args, at)
	}}}
}

func arg(args []Value, i int) Value {
	if i < len(args) {
		return args[i]
	}
	return undef()
}

func (it *interp) out(s string) { it.rec.out.WriteString(s) }

// moduleV membuat objek berisi fungsi/konstanta (Math, JSON, console, ...).
func (it *interp) moduleV(members map[string]Value) Value {
	d := it.newDict("module")
	keys := make([]string, 0, len(members))
	for k := range members {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		d.set(strV(k), members[k])
	}
	return Value{k: kDict, d: d}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func (it *interp) setupJS(s *jsSem) {
	g := s.globals
	n := func(v Value) float64 { return it.jsToNumber(v) }

	// --- kelas error ---
	errBase := &Class{name: "Error", native: "Error", methods: map[string]*Fn{}, getters: map[string]*Fn{}, setters: map[string]*Fn{}, statics: it.newDict("statics"), kind: "class"}
	errInit := func(it *interp, this Value, args []Value, at nodeRef) {
		if len(args) > 0 && args[0].k != kUndef && this.k == kObj {
			this.o.fields.set(strV("message"), strV(it.sem.str(it, args[0])))
		}
	}
	mkErr := func(c *Class) {
		c.init = errInit
		c.ctor = func(it *interp, args []Value, at nodeRef) Value {
			it.ids++
			o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
			v := Value{k: kObj, o: o}
			errInit(it, v, args, at)
			return v
		}
		g[c.name] = Value{k: kClass, c: c}
	}
	mkErr(errBase)
	for _, name := range []string{"TypeError", "RangeError", "ReferenceError", "SyntaxError", "EvalError", "URIError"} {
		c := &Class{name: name, native: "Error", super: errBase, methods: map[string]*Fn{}, getters: map[string]*Fn{}, setters: map[string]*Fn{}, statics: it.newDict("statics"), kind: "class"}
		mkErr(c)
	}

	// --- console ---
	logTo := func(name string) Value {
		return bf(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			it.out(it.consoleLog(args) + "\n")
			return undef()
		})
	}
	g["console"] = it.moduleV(map[string]Value{
		"log": logTo("log"), "info": logTo("info"), "debug": logTo("debug"), "error": logTo("error"), "warn": logTo("warn"),
		"table": logTo("table"),
	})

	// --- Math ---
	m1 := func(name string, f func(float64) float64) Value {
		return bf(name, func(it *interp, this Value, args []Value, at ir.Node) Value { return num(f(n(arg(args, 0)))) })
	}
	g["Math"] = it.moduleV(map[string]Value{
		"PI": num(math.Pi), "E": num(math.E), "SQRT2": num(math.Sqrt2), "LN2": num(math.Ln2), "LN10": num(math.Ln10),
		"abs": m1("abs", math.Abs), "floor": m1("floor", math.Floor), "ceil": m1("ceil", math.Ceil),
		"sqrt": m1("sqrt", math.Sqrt), "cbrt": m1("cbrt", math.Cbrt), "trunc": m1("trunc", math.Trunc),
		"sin": m1("sin", math.Sin), "cos": m1("cos", math.Cos), "tan": m1("tan", math.Tan), "atan": m1("atan", math.Atan),
		"log": m1("log", math.Log), "log2": m1("log2", math.Log2), "log10": m1("log10", math.Log10), "exp": m1("exp", math.Exp),
		"sign": m1("sign", func(x float64) float64 {
			switch {
			case x > 0:
				return 1
			case x < 0:
				return -1
			}
			return x
		}),
		"round": m1("round", func(x float64) float64 {
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return x
			}
			return math.Floor(x + 0.5)
		}),
		"pow":   bf("pow", func(it *interp, this Value, args []Value, at ir.Node) Value { return num(math.Pow(n(arg(args, 0)), n(arg(args, 1)))) }),
		"atan2": bf("atan2", func(it *interp, this Value, args []Value, at ir.Node) Value { return num(math.Atan2(n(arg(args, 0)), n(arg(args, 1)))) }),
		"hypot": bf("hypot", func(it *interp, this Value, args []Value, at ir.Node) Value {
			s := 0.0
			for _, a := range args {
				x := n(a)
				s += x * x
			}
			return num(math.Sqrt(s))
		}),
		"max": bf("max", func(it *interp, this Value, args []Value, at ir.Node) Value {
			r := math.Inf(-1)
			for _, a := range args {
				x := n(a)
				if math.IsNaN(x) {
					return num(math.NaN())
				}
				r = math.Max(r, x)
			}
			return num(r)
		}),
		"min": bf("min", func(it *interp, this Value, args []Value, at ir.Node) Value {
			r := math.Inf(1)
			for _, a := range args {
				x := n(a)
				if math.IsNaN(x) {
					return num(math.NaN())
				}
				r = math.Min(r, x)
			}
			return num(r)
		}),
		"random": bf("random", func(it *interp, this Value, args []Value, at ir.Node) Value {
			it.warn("Math.random() disimulasikan dengan angka acak tetap (seed); hasilnya tidak sama dengan saat program dijalankan")
			return num(it.rng.Float64())
		}),
	})

	// --- JSON ---
	g["JSON"] = it.moduleV(map[string]Value{
		"stringify": bf("stringify", func(it *interp, this Value, args []Value, at ir.Node) Value {
			ind := ""
			if sp := arg(args, 2); isNum(sp) {
				ind = strings.Repeat(" ", int(n(sp)))
			} else if sp.k == kStr {
				ind = sp.s
			}
			s := it.jsonStringify(arg(args, 0), ind)
			if s == "undefined" {
				return undef()
			}
			return strV(s)
		}),
		"parse": bf("parse", func(it *interp, this Value, args []Value, at ir.Node) Value {
			return it.jsonParse(it.sem.str(it, arg(args, 0)), at)
		}),
	})

	// --- fungsi global ---
	g["parseInt"] = bf("parseInt", func(it *interp, this Value, args []Value, at ir.Node) Value {
		radix := n(arg(args, 1))
		if math.IsNaN(radix) || math.IsInf(radix, 0) {
			radix = 0
		}
		return num(jsParseInt(it.sem.str(it, arg(args, 0)), int(radix)))
	})
	g["parseFloat"] = bf("parseFloat", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return num(jsParseFloat(it.sem.str(it, arg(args, 0))))
	})
	g["isNaN"] = bf("isNaN", func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(math.IsNaN(n(arg(args, 0)))) })
	g["isFinite"] = bf("isFinite", func(it *interp, this Value, args []Value, at ir.Node) Value {
		x := n(arg(args, 0))
		return boolV(!math.IsNaN(x) && !math.IsInf(x, 0))
	})
	g["NaN"] = num(math.NaN())
	g["Infinity"] = num(math.Inf(1))
	g["undefined"] = undef()
	for _, name := range []string{"prompt", "readline", "require"} {
		nm := name
		g[nm] = bf(nm, func(it *interp, this Value, args []Value, at ir.Node) Value {
			if nm == "require" {
				return unknown("modul")
			}
			return unknown("membaca input dari user (" + nm + ") belum didukung")
		})
	}

	// --- Number / String / Boolean ---
	numClass := it.nativeFuncClass("Number", func(it *interp, args []Value) Value {
		if len(args) == 0 {
			return num(0)
		}
		return num(n(args[0]))
	})
	numClass.statics.set(strV("isInteger"), bf("isInteger", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a := arg(args, 0)
		return boolV(a.k == kFloat && isIntegral(a.f))
	}))
	numClass.statics.set(strV("isNaN"), bf("isNaN", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a := arg(args, 0)
		return boolV(a.k == kFloat && math.IsNaN(a.f))
	}))
	numClass.statics.set(strV("parseInt"), g["parseInt"])
	numClass.statics.set(strV("parseFloat"), g["parseFloat"])
	numClass.statics.set(strV("MAX_SAFE_INTEGER"), num(9007199254740991))
	numClass.statics.set(strV("MIN_SAFE_INTEGER"), num(-9007199254740991))
	numClass.statics.set(strV("EPSILON"), num(math.Pow(2, -52)))
	numClass.statics.set(strV("MAX_VALUE"), num(math.MaxFloat64))
	numClass.statics.set(strV("POSITIVE_INFINITY"), num(math.Inf(1)))
	numClass.statics.set(strV("NEGATIVE_INFINITY"), num(math.Inf(-1)))
	g["Number"] = Value{k: kClass, c: numClass}
	strClass := it.nativeFuncClass("String", func(it *interp, args []Value) Value {
		if len(args) == 0 {
			return strV("")
		}
		return strV(it.sem.str(it, args[0]))
	})
	strClass.statics.set(strV("fromCharCode"), bf("fromCharCode", func(it *interp, this Value, args []Value, at ir.Node) Value {
		var units []uint16
		for _, a := range args {
			units = append(units, uint16(toInt32(n(a))))
		}
		return strV(string(utf16.Decode(units)))
	}))
	g["String"] = Value{k: kClass, c: strClass}
	g["Boolean"] = Value{k: kClass, c: it.nativeFuncClass("Boolean", func(it *interp, args []Value) Value {
		return boolV(len(args) > 0 && it.sem.truthy(args[0]))
	})}
	g["BigInt"] = bf("BigInt", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := arg(args, 0)
		b, ok := newBigFromString(it.sem.str(it, v))
		if !ok {
			it.throw(at, "SyntaxError", "Cannot convert "+it.sem.str(it, v)+" to a BigInt")
		}
		return Value{k: kBig, big: b}
	})

	// --- Array ---
	arrClass := it.nativeFuncClass("Array", func(it *interp, args []Value) Value {
		if len(args) == 1 && isNum(args[0]) {
			size := int(n(args[0]))
			elems := make([]Value, size)
			for i := range elems {
				elems[i] = Value{k: kUndef, t: "hole"}
			}
			return it.newList("array", elems)
		}
		return it.newList("array", append([]Value(nil), args...))
	})
	arrClass.statics.set(strV("isArray"), bf("isArray", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return boolV(arg(args, 0).k == kList)
	}))
	arrClass.statics.set(strV("of"), bf("of", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.newList("array", append([]Value(nil), args...))
	}))
	arrClass.statics.set(strV("from"), bf("from", func(it *interp, this Value, args []Value, at ir.Node) Value {
		src := arg(args, 0)
		var elems []Value
		if src.k == kDict && src.d.kind == "object" {
			if l, ok := src.d.get(strV("length")); ok {
				for i := 0; i < int(n(l)); i++ {
					v, ok := src.d.get(strV(strconv.Itoa(i)))
					if !ok {
						v = undef()
					}
					elems = append(elems, v)
				}
			}
		} else if pairs, ok := it.sem.iterate(it, src, nil); ok {
			for _, p := range pairs {
				elems = append(elems, p.v)
			}
		}
		if f := arg(args, 1); f.k == kFunc {
			for i := range elems {
				elems[i] = it.call(f, undef(), []Value{elems[i], num(float64(i))}, at)
			}
		}
		return it.newList("array", elems)
	}))
	g["Array"] = Value{k: kClass, c: arrClass}

	// --- Object ---
	objClass := it.nativeFuncClass("Object", func(it *interp, args []Value) Value { return it.dictV("object") })
	ownEntries := func(v Value) ([]Value, []Value) {
		switch v.k {
		case kDict:
			keys := jsKeyOrder(v.d)
			vals := make([]Value, len(keys))
			for i, k := range keys {
				vals[i], _ = v.d.get(k)
			}
			return keys, vals
		case kObj:
			var keys, vals []Value
			for i, k := range v.o.fields.keys {
				if !strings.HasPrefix(k.s, "#") {
					keys = append(keys, k)
					vals = append(vals, v.o.fields.vals[i])
				}
			}
			return keys, vals
		case kList:
			var keys []Value
			for i := range v.l.elems {
				keys = append(keys, strV(strconv.Itoa(i)))
			}
			return keys, v.l.elems
		case kStr:
			var keys, vals []Value
			for i, r := range []rune(v.s) {
				keys = append(keys, strV(strconv.Itoa(i)))
				vals = append(vals, strV(string(r)))
			}
			return keys, vals
		}
		return nil, nil
	}
	objClass.statics.set(strV("keys"), bf("keys", func(it *interp, this Value, args []Value, at ir.Node) Value {
		k, _ := ownEntries(arg(args, 0))
		return it.newList("array", k)
	}))
	objClass.statics.set(strV("values"), bf("values", func(it *interp, this Value, args []Value, at ir.Node) Value {
		_, v := ownEntries(arg(args, 0))
		return it.newList("array", append([]Value(nil), v...))
	}))
	objClass.statics.set(strV("entries"), bf("entries", func(it *interp, this Value, args []Value, at ir.Node) Value {
		k, v := ownEntries(arg(args, 0))
		var out []Value
		for i := range k {
			out = append(out, it.newList("array", []Value{k[i], v[i]}))
		}
		return it.newList("array", out)
	}))
	objClass.statics.set(strV("assign"), bf("assign", func(it *interp, this Value, args []Value, at ir.Node) Value {
		dst := arg(args, 0)
		for _, src := range args[1:] {
			k, v := ownEntries(src)
			for i := range k {
				switch dst.k {
				case kDict:
					dst.d.set(k[i], v[i])
				case kObj:
					dst.o.fields.set(k[i], v[i])
				}
			}
		}
		return dst
	}))
	objClass.statics.set(strV("fromEntries"), bf("fromEntries", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.newDict("object")
		if pairs, ok := it.sem.iterate(it, arg(args, 0), nil); ok {
			for _, p := range pairs {
				if p.v.k == kList && len(p.v.l.elems) >= 2 {
					d.set(strV(it.sem.str(it, p.v.l.elems[0])), p.v.l.elems[1])
				}
			}
		}
		return Value{k: kDict, d: d}
	}))
	ident := bf("freeze", func(it *interp, this Value, args []Value, at ir.Node) Value { return arg(args, 0) })
	objClass.statics.set(strV("freeze"), ident)
	g["Object"] = Value{k: kClass, c: objClass}

	// --- Map / Set ---
	mapClass := &Class{name: "Map", native: "Map", statics: it.newDict("statics"), kind: "native"}
	mapClass.ctor = func(it *interp, args []Value, at nodeRef) Value {
		d := it.newDict("Map")
		if len(args) > 0 {
			if pairs, ok := it.sem.iterate(it, args[0], nil); ok {
				for _, p := range pairs {
					if p.v.k == kList && len(p.v.l.elems) >= 2 {
						d.set(p.v.l.elems[0], p.v.l.elems[1])
					}
				}
			}
		}
		return Value{k: kDict, d: d}
	}
	g["Map"] = Value{k: kClass, c: mapClass}
	setClass := &Class{name: "Set", native: "Set", statics: it.newDict("statics"), kind: "native"}
	setClass.ctor = func(it *interp, args []Value, at nodeRef) Value {
		d := it.newDict("set")
		if len(args) > 0 {
			if pairs, ok := it.sem.iterate(it, args[0], nil); ok {
				for _, p := range pairs {
					d.set(p.v, p.v)
				}
			}
		}
		return Value{k: kSet, d: d}
	}
	g["Set"] = Value{k: kClass, c: setClass}

	// --- Promise & timer ---
	promClass := &Class{name: "Promise", native: "Promise", statics: it.newDict("statics"), kind: "native"}
	promClass.ctor = func(it *interp, args []Value, at nodeRef) Value {
		p := it.newPromise()
		resolve := bf("resolve", func(it *interp, this Value, a []Value, at ir.Node) Value {
			it.settle(p, arg(a, 0), false)
			return undef()
		})
		reject := bf("reject", func(it *interp, this Value, a []Value, at ir.Node) Value {
			it.settle(p, arg(a, 0), true)
			return undef()
		})
		if f := arg(args, 0); f.k == kFunc {
			it.call(f, undef(), []Value{resolve, reject}, at)
		}
		return p
	}
	promClass.statics.set(strV("resolve"), bf("resolve", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.resolvedPromise(arg(args, 0))
	}))
	g["Promise"] = Value{k: kClass, c: promClass}
	seq := 0
	g["setTimeout"] = bf("setTimeout", func(it *interp, this Value, args []Value, at ir.Node) Value {
		f := arg(args, 0)
		delay := n(arg(args, 1))
		if math.IsNaN(delay) {
			delay = 0
		}
		extra := append([]Value(nil), args[min(2, len(args)):]...)
		seq++
		it.timers = append(it.timers, timerTask{at: delay, seq: seq, fn: func() { it.call(f, undef(), extra, at) }})
		return num(float64(seq))
	})
	g["clearTimeout"] = bf("clearTimeout", func(it *interp, this Value, args []Value, at ir.Node) Value { return undef() })
	g["globalThis"] = it.dictV("object")
}

// nativeFuncClass: kelas bawaan yang juga bisa dipanggil sebagai fungsi (Number("5"), Array(3)).
func (it *interp) nativeFuncClass(name string, f func(it *interp, args []Value) Value) *Class {
	c := &Class{name: name, native: name, statics: it.newDict("statics"), methods: map[string]*Fn{}, kind: "native"}
	c.ctor = func(it *interp, args []Value, at nodeRef) Value { return f(it, args) }
	return c
}

func jsParseInt(s string, radix int) float64 {
	s = strings.TrimSpace(s)
	sign := 1.0
	if strings.HasPrefix(s, "-") {
		sign, s = -1, s[1:]
	} else {
		s = strings.TrimPrefix(s, "+")
	}
	if radix == 0 {
		radix = 10
		if strings.HasPrefix(strings.ToLower(s), "0x") {
			radix, s = 16, s[2:]
		}
	} else if radix == 16 && strings.HasPrefix(strings.ToLower(s), "0x") {
		s = s[2:]
	}
	if radix < 2 || radix > 36 {
		return math.NaN()
	}
	end := 0
	for end < len(s) {
		d := digitVal(s[end])
		if d < 0 || d >= radix {
			break
		}
		end++
	}
	if end == 0 {
		return math.NaN()
	}
	f := 0.0
	for _, c := range s[:end] {
		f = f*float64(radix) + float64(digitVal(byte(c)))
	}
	return sign * f
}

func digitVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return -1
}

func jsParseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "Infinity") || strings.HasPrefix(s, "+Infinity") {
		return math.Inf(1)
	}
	if strings.HasPrefix(s, "-Infinity") {
		return math.Inf(-1)
	}
	end := 0
	seenDot, seenE, seenDigit := false, false, false
	for end < len(s) {
		c := s[end]
		switch {
		case c >= '0' && c <= '9':
			seenDigit = true
		case (c == '+' || c == '-') && (end == 0 || s[end-1] == 'e' || s[end-1] == 'E'):
		case c == '.' && !seenDot && !seenE:
			seenDot = true
		case (c == 'e' || c == 'E') && seenDigit && !seenE:
			seenE = true
		default:
			goto done
		}
		end++
	}
done:
	for end > 0 {
		if f, err := strconv.ParseFloat(s[:end], 64); err == nil {
			return f
		}
		end--
	}
	return math.NaN()
}

// ---------- Promise ----------

func (it *interp) newPromise() Value {
	d := it.newDict("promise")
	d.set(strV("state"), strV("pending"))
	d.set(strV("value"), undef())
	return Value{k: kDict, d: d}
}

func (it *interp) resolvedPromise(v Value) Value {
	if v.k == kDict && v.d.kind == "promise" {
		return v
	}
	p := it.newPromise()
	it.settle(p, v, false)
	return p
}

func (it *interp) settle(p Value, v Value, rejected bool) {
	if st, _ := p.d.get(strV("state")); st.s != "pending" {
		return
	}
	state := "fulfilled"
	if rejected {
		state = "rejected"
	}
	p.d.set(strV("state"), strV(state))
	p.d.set(strV("value"), v)
	if cbs, ok := p.d.get(strV("cbs")); ok && cbs.k == kList {
		for _, cb := range cbs.l.elems {
			fn := cb.fn
			it.tasks = append(it.tasks, func() { fn.builtin(it, undef(), nil, nil) })
		}
	}
}

// then mendaftarkan callback yang dijalankan sebagai microtask setelah promise selesai.
func (it *interp) then(p Value, onOk, onErr Value, at ir.Node) Value {
	next := it.newPromise()
	run := func() {
		st, _ := p.d.get(strV("state"))
		v, _ := p.d.get(strV("value"))
		h := onOk
		if st.s == "rejected" {
			h = onErr
		}
		if h.k != kFunc {
			it.settle(next, v, st.s == "rejected")
			return
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					if t, ok := r.(thrown); ok {
						it.settle(next, t.v, true)
						return
					}
					panic(r)
				}
			}()
			r := it.call(h, undef(), []Value{v}, at)
			if r.k == kDict && r.d.kind == "promise" {
				it.then(r, bf("ok", func(it *interp, this Value, a []Value, at ir.Node) Value {
					it.settle(next, arg(a, 0), false)
					return undef()
				}), bf("err", func(it *interp, this Value, a []Value, at ir.Node) Value {
					it.settle(next, arg(a, 0), true)
					return undef()
				}), at)
				return
			}
			it.settle(next, r, false)
		}()
	}
	if st, _ := p.d.get(strV("state")); st.s != "pending" {
		it.tasks = append(it.tasks, run)
	} else {
		cbs, ok := p.d.get(strV("cbs"))
		if !ok {
			cbs = it.newList("array", nil)
			p.d.set(strV("cbs"), cbs)
		}
		cbs.l.elems = append(cbs.l.elems, Value{k: kFunc, fn: &Func{builtin: func(it *interp, this Value, args []Value, at nodeRef) Value {
			run()
			return undef()
		}}})
	}
	return next
}

// awaitValue: await dijalankan sinkron. Promise yang sudah selesai langsung memberi nilainya;
// microtask yang tertunda dijalankan dulu supaya promise sempat selesai.
func (it *interp) awaitValue(v Value, at ir.Node) Value {
	if v.k != kDict || v.d.kind != "promise" {
		return v
	}
	for i := 0; i < 1000; i++ {
		st, _ := v.d.get(strV("state"))
		val, _ := v.d.get(strV("value"))
		switch st.s {
		case "fulfilled":
			return val
		case "rejected":
			panic(thrown{v: val, msg: it.exceptionText(val), at: at})
		}
		if len(it.tasks) == 0 && len(it.timers) == 0 {
			break
		}
		it.runOneTask()
	}
	return unknown("promise belum selesai")
}

func (it *interp) runOneTask() {
	if len(it.tasks) > 0 {
		t := it.tasks[0]
		it.tasks = it.tasks[1:]
		t()
		return
	}
	if len(it.timers) > 0 {
		best := 0
		for j, t := range it.timers {
			if t.at < it.timers[best].at || t.at == it.timers[best].at && t.seq < it.timers[best].seq {
				best = j
			}
		}
		t := it.timers[best]
		it.timers = append(it.timers[:best:best], it.timers[best+1:]...)
		t.fn()
	}
}

// ---------- member bawaan ----------

func (s *jsSem) global(it *interp, name string) (Value, bool) {
	v, ok := s.globals[name]
	return v, ok
}

func bound(this Value, name string, f builtinFn) Value {
	v := bf(name, f)
	t := this
	v.fn.this = &t
	return v
}

func (s *jsSem) member(it *interp, x Value, name string, at ir.Node) (Value, bool) {
	switch x.k {
	case kStr:
		return it.jsStringMember(x, name, at)
	case kList:
		return it.jsArrayMember(x, name, at)
	case kFloat, kInt:
		return it.jsNumberMember(x, name)
	case kSet:
		return it.jsSetMember(x, name)
	case kDict:
		switch x.d.kind {
		case "Map":
			return it.jsMapMember(x, name)
		case "promise":
			switch name {
			case "then":
				return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
					return it.then(this, arg(args, 0), arg(args, 1), at)
				}), true
			case "catch":
				return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
					return it.then(this, undef(), arg(args, 0), at)
				}), true
			case "finally":
				return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
					f := arg(args, 0)
					wrap := bf("finally", func(it *interp, _ Value, a []Value, at ir.Node) Value {
						it.call(f, undef(), nil, at)
						return arg(a, 0)
					})
					return it.then(this, wrap, wrap, at)
				}), true
			}
		case "regex":
			switch name {
			case "test":
				return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
					re, _ := it.goRegex(this, at)
					return boolV(re.MatchString(it.sem.str(it, arg(args, 0))))
				}), true
			case "source", "flags", "global":
				v, _ := x.d.get(strV(name))
				return v, true
			}
		}
		switch name {
		case "hasOwnProperty":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
				return boolV(this.d.has(strV(it.sem.str(it, arg(args, 0)))))
			}), true
		case "toString":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value { return strV("[object Object]") }), true
		}
	case kObj:
		switch name {
		case "name":
			if instanceOf(x.o.class, "Error") {
				return strV(x.o.class.name), true
			}
		case "message":
			if instanceOf(x.o.class, "Error") {
				return strV(""), true
			}
		case "stack":
			return strV(it.exceptionText(x)), true
		case "constructor":
			return Value{k: kClass, c: x.o.class}, true
		case "hasOwnProperty":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
				return boolV(this.o.fields.has(strV(it.sem.str(it, arg(args, 0)))))
			}), true
		case "toString":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(it.sem.str(it, this)) }), true
		}
	case kFunc:
		switch name {
		case "name":
			return strV(x.fn.name), true
		case "call":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
				f := *this.fn
				t := arg(args, 0)
				f.this = &t
				return it.call(Value{k: kFunc, fn: &f}, t, args[min(1, len(args)):], at)
			}), true
		case "apply":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
				f := *this.fn
				t := arg(args, 0)
				f.this = &t
				var a []Value
				if l := arg(args, 1); l.k == kList {
					a = l.l.elems
				}
				return it.call(Value{k: kFunc, fn: &f}, t, a, at)
			}), true
		case "bind":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
				f := *this.fn
				t := arg(args, 0)
				f.this = &t
				pre := append([]Value(nil), args[min(1, len(args)):]...)
				if len(pre) == 0 {
					return Value{k: kFunc, fn: &f}
				}
				return bf(f.name, func(it *interp, _ Value, a []Value, at ir.Node) Value {
					return it.call(Value{k: kFunc, fn: &f}, t, append(append([]Value(nil), pre...), a...), at)
				})
			}), true
		case "length":
			if x.fn.decl != nil {
				return num(float64(requiredParams(x.fn.decl))), true
			}
		}
	case kClass:
		if name == "name" {
			return strV(x.c.name), true
		}
	}
	return Value{}, false
}

func (it *interp) jsNumberMember(x Value, name string) (Value, bool) {
	f := toF(x)
	switch name {
	case "toFixed":
		return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			d := int(it.jsToNumber(arg(args, 0)))
			if d < 0 || d > 100 {
				it.throw(at, "RangeError", "toFixed() digits argument must be between 0 and 100")
			}
			return strV(jsToFixed(f, d))
		}), true
	case "toString":
		return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			if r := arg(args, 0); r.k != kUndef {
				return strV(jsRadix(f, int(it.jsToNumber(r))))
			}
			return strV(jsNumber(f))
		}), true
	case "toPrecision":
		return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			p := int(it.jsToNumber(arg(args, 0)))
			if p <= 0 {
				return strV(jsNumber(f))
			}
			e0 := strconv.FormatFloat(f, 'e', p-1, 64)
			mant, exps, _ := strings.Cut(e0, "e")
			e, _ := strconv.Atoi(exps)
			if e < -6 || e >= p {
				sign := "+"
				if e < 0 {
					sign, e = "-", -e
				}
				return strV(mant + "e" + sign + strconv.Itoa(e))
			}
			s := strconv.FormatFloat(f, 'f', max(0, p-1-e), 64)
			return strV(s)
		}), true
	case "toLocaleString":
		return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			return strV(groupThousands(jsNumber(f), ","))
		}), true
	}
	return Value{}, false
}

func groupThousands(s, sep string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(c)
	}
	out := b.String()
	if hasFrac {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

// jsStringMember: method string JavaScript (indeks memakai rune).
func (it *interp) jsStringMember(x Value, name string, at ir.Node) (Value, bool) {
	s := x.s
	rs := []rune(s)
	n := func(v Value) float64 { return it.jsToNumber(v) }
	idx := func(v Value, def int) int {
		if v.k == kUndef {
			return def
		}
		f := n(v)
		if math.IsNaN(f) {
			return 0
		}
		return int(math.Trunc(f))
	}
	clampRel := func(i, l int) int {
		if i < 0 {
			i += l
			if i < 0 {
				i = 0
			}
		}
		if i > l {
			i = l
		}
		return i
	}
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	switch name {
	case "length":
		return num(float64(len(utf16.Encode(rs)))), true
	case "toUpperCase", "toLocaleUpperCase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.ToUpper(s)) })
	case "toLowerCase", "toLocaleLowerCase":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.ToLower(s)) })
	case "trim":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.TrimSpace(s)) })
	case "trimStart":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.TrimLeft(s, " \t\n\r")) })
	case "trimEnd":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(strings.TrimRight(s, " \t\n\r")) })
	case "charAt", "at":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			i := idx(arg(args, 0), 0)
			if name == "at" && i < 0 {
				i += len(rs)
			}
			if i < 0 || i >= len(rs) {
				if name == "at" {
					return undef()
				}
				return strV("")
			}
			return strV(string(rs[i]))
		})
	case "charCodeAt", "codePointAt":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			i := idx(arg(args, 0), 0)
			if i < 0 || i >= len(rs) {
				return num(math.NaN())
			}
			return num(float64(rs[i]))
		})
	case "indexOf", "lastIndexOf", "includes", "startsWith", "endsWith":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			sub := it.sem.str(it, arg(args, 0))
			switch name {
			case "includes":
				return boolV(strings.Contains(s, sub))
			case "startsWith":
				from := clampRel(idx(arg(args, 1), 0), len(rs))
				return boolV(strings.HasPrefix(string(rs[from:]), sub))
			case "endsWith":
				end := clampRel(idx(arg(args, 1), len(rs)), len(rs))
				return boolV(strings.HasSuffix(string(rs[:end]), sub))
			case "lastIndexOf":
				i := strings.LastIndex(s, sub)
				if i < 0 {
					return num(-1)
				}
				return num(float64(len([]rune(s[:i]))))
			}
			from := clampRel(idx(arg(args, 1), 0), len(rs))
			i := strings.Index(string(rs[from:]), sub)
			if i < 0 {
				return num(-1)
			}
			return num(float64(from + len([]rune(string(rs[from:])[:i]))))
		})
	case "slice", "substring", "substr":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l := len(rs)
			switch name {
			case "slice":
				a, b := clampRel(idx(arg(args, 0), 0), l), clampRel(idx(arg(args, 1), l), l)
				if a >= b {
					return strV("")
				}
				return strV(string(rs[a:b]))
			case "substring":
				a, b := idx(arg(args, 0), 0), idx(arg(args, 1), l)
				a, b = max(0, min(a, l)), max(0, min(b, l))
				if a > b {
					a, b = b, a
				}
				return strV(string(rs[a:b]))
			}
			a := clampRel(idx(arg(args, 0), 0), l)
			cnt := idx(arg(args, 1), l-a)
			b := min(l, a+max(0, cnt))
			return strV(string(rs[a:b]))
		})
	case "split":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			sep := arg(args, 0)
			var parts []string
			switch {
			case sep.k == kUndef:
				parts = []string{s}
			case sep.k == kDict && sep.d.kind == "regex":
				re, _ := it.goRegex(sep, at)
				parts = re.Split(s, -1)
			default:
				sp := it.sem.str(it, sep)
				if sp == "" {
					for _, r := range rs {
						parts = append(parts, string(r))
					}
				} else {
					parts = strings.Split(s, sp)
				}
			}
			if lim := arg(args, 1); lim.k != kUndef && int(n(lim)) < len(parts) {
				parts = parts[:max(0, int(n(lim)))]
			}
			out := make([]Value, len(parts))
			for i, p := range parts {
				out[i] = strV(p)
			}
			return it.newList("array", out)
		})
	case "replace", "replaceAll":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			pat, rep := arg(args, 0), arg(args, 1)
			repl := func(match string, groups []string) string {
				if rep.k == kFunc {
					a := []Value{strV(match)}
					for _, g := range groups {
						a = append(a, strV(g))
					}
					return it.sem.str(it, it.call(rep, undef(), a, at))
				}
				r := it.sem.str(it, rep)
				r = strings.ReplaceAll(r, "$&", match)
				for i := len(groups); i >= 1; i-- {
					r = strings.ReplaceAll(r, "$"+strconv.Itoa(i), groups[i-1])
				}
				return r
			}
			if pat.k == kDict && pat.d.kind == "regex" {
				re, global := it.goRegex(pat, at)
				count := 0
				return strV(re.ReplaceAllStringFunc(s, func(m string) string {
					count++
					if !global && name == "replace" && count > 1 {
						return m
					}
					sub := re.FindStringSubmatch(m)
					var groups []string
					if len(sub) > 1 {
						groups = sub[1:]
					}
					return repl(m, groups)
				}))
			}
			p := it.sem.str(it, pat)
			if name == "replaceAll" {
				var b strings.Builder
				rest := s
				for {
					i := strings.Index(rest, p)
					if i < 0 || p == "" {
						b.WriteString(rest)
						break
					}
					b.WriteString(rest[:i])
					b.WriteString(repl(p, nil))
					rest = rest[i+len(p):]
				}
				return strV(b.String())
			}
			i := strings.Index(s, p)
			if i < 0 {
				return strV(s)
			}
			return strV(s[:i] + repl(p, nil) + s[i+len(p):])
		})
	case "repeat":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			c := int(n(arg(args, 0)))
			if c < 0 {
				it.throw(at, "RangeError", "Invalid count value: "+strconv.Itoa(c))
			}
			return strV(strings.Repeat(s, c))
		})
	case "padStart", "padEnd":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			target := int(n(arg(args, 0)))
			fill := " "
			if f := arg(args, 1); f.k != kUndef {
				fill = it.sem.str(it, f)
			}
			if len(rs) >= target || fill == "" {
				return strV(s)
			}
			need := target - len(rs)
			pad := strings.Repeat(fill, need/len([]rune(fill))+1)
			pad = string([]rune(pad)[:need])
			if name == "padStart" {
				return strV(pad + s)
			}
			return strV(s + pad)
		})
	case "concat":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			out := s
			for _, a := range args {
				out += it.sem.str(it, a)
			}
			return strV(out)
		})
	case "localeCompare":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			o := it.sem.str(it, arg(args, 0))
			switch c := strings.Compare(strings.ToLower(s), strings.ToLower(o)); {
			case c != 0:
				return num(float64(c))
			}
			return num(float64(strings.Compare(o, s)))
		})
	case "match":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			p := arg(args, 0)
			if p.k != kDict || p.d.kind != "regex" {
				p = it.jsRegex("/" + regexpQuote(it.sem.str(it, p)) + "/")
			}
			re, global := it.goRegex(p, at)
			if global {
				all := re.FindAllString(s, -1)
				if all == nil {
					return null()
				}
				out := make([]Value, len(all))
				for i, a := range all {
					out[i] = strV(a)
				}
				return it.newList("array", out)
			}
			sub := re.FindStringSubmatch(s)
			if sub == nil {
				return null()
			}
			out := make([]Value, len(sub))
			for i, a := range sub {
				out[i] = strV(a)
			}
			return it.newList("array", out)
		})
	case "toString", "valueOf":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(s) })
	}
	return Value{}, false
}

func regexpQuote(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\.+*?()|[]{}^$/`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// jsArrayMember: method array JavaScript.
func (it *interp) jsArrayMember(x Value, name string, at ir.Node) (Value, bool) {
	l := x.l
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	n := func(v Value) float64 { return it.jsToNumber(v) }
	rel := func(v Value, def int) int {
		if v.k == kUndef {
			return def
		}
		i := int(math.Trunc(n(v)))
		if i < 0 {
			i += len(l.elems)
			if i < 0 {
				i = 0
			}
		}
		if i > len(l.elems) {
			i = len(l.elems)
		}
		return i
	}
	cb := func(f Value, e Value, i int) Value {
		return it.call(f, undef(), []Value{e, num(float64(i)), x}, at)
	}
	needFn := func(f Value) {
		if f.k != kFunc {
			it.throw(at, "TypeError", it.sem.show(it, f)+" is not a function")
		}
	}
	switch name {
	case "length":
		return num(float64(len(l.elems))), true
	case "push":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l.elems = append(l.elems, args...)
			return num(float64(len(l.elems)))
		})
	case "pop":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if len(l.elems) == 0 {
				return undef()
			}
			v := l.elems[len(l.elems)-1]
			l.elems = l.elems[:len(l.elems)-1]
			return v
		})
	case "shift":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if len(l.elems) == 0 {
				return undef()
			}
			v := l.elems[0]
			l.elems = append([]Value(nil), l.elems[1:]...)
			return v
		})
	case "unshift":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			l.elems = append(append([]Value(nil), args...), l.elems...)
			return num(float64(len(l.elems)))
		})
	case "slice":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			a, b := rel(arg(args, 0), 0), rel(arg(args, 1), len(l.elems))
			if a >= b {
				return it.newList("array", nil)
			}
			return it.newList("array", append([]Value(nil), l.elems[a:b]...))
		})
	case "splice":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			start := rel(arg(args, 0), 0)
			cnt := len(l.elems) - start
			if len(args) > 1 {
				cnt = max(0, min(int(n(args[1])), len(l.elems)-start))
			}
			removed := append([]Value(nil), l.elems[start:start+cnt]...)
			var ins []Value
			if len(args) > 2 {
				ins = args[2:]
			}
			rest := append(append(append([]Value(nil), l.elems[:start]...), ins...), l.elems[start+cnt:]...)
			l.elems = rest
			return it.newList("array", removed)
		})
	case "concat":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			out := append([]Value(nil), l.elems...)
			for _, a := range args {
				if a.k == kList {
					out = append(out, a.l.elems...)
				} else {
					out = append(out, a)
				}
			}
			return it.newList("array", out)
		})
	case "join":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			sep := ","
			if s := arg(args, 0); s.k != kUndef {
				sep = it.sem.str(it, s)
			}
			parts := make([]string, len(l.elems))
			for i, e := range l.elems {
				if e.k != kNull && e.k != kUndef {
					parts[i] = it.sem.str(it, e)
				}
			}
			return strV(strings.Join(parts, sep))
		})
	case "toString":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return strV(it.sem.str(it, x)) })
	case "reverse":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for i, j := 0, len(l.elems)-1; i < j; i, j = i+1, j-1 {
				l.elems[i], l.elems[j] = l.elems[j], l.elems[i]
			}
			return x
		})
	case "sort", "toSorted":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			f := arg(args, 0)
			less := func(a, b Value) bool {
				if a.k == kUndef {
					return false
				}
				if b.k == kUndef {
					return true
				}
				if f.k == kFunc {
					return it.jsToNumber(it.call(f, undef(), []Value{a, b}, at)) < 0
				}
				return compareUTF16(it.sem.str(it, a), it.sem.str(it, b)) < 0
			}
			target := l.elems
			if name == "toSorted" {
				target = append([]Value(nil), l.elems...)
			}
			mergeSort(target, less)
			if name == "toSorted" {
				return it.newList("array", target)
			}
			return x
		})
	case "indexOf", "lastIndexOf", "includes":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			want := arg(args, 0)
			if name == "lastIndexOf" {
				for i := len(l.elems) - 1; i >= 0; i-- {
					if jsStrictEq(l.elems[i], want) {
						return num(float64(i))
					}
				}
				return num(-1)
			}
			for i := rel(arg(args, 1), 0); i < len(l.elems); i++ {
				e := l.elems[i]
				if jsStrictEq(e, want) || name == "includes" && e.k == kFloat && want.k == kFloat && math.IsNaN(e.f) && math.IsNaN(want.f) {
					if name == "includes" {
						return boolV(true)
					}
					return num(float64(i))
				}
			}
			if name == "includes" {
				return boolV(false)
			}
			return num(-1)
		})
	case "forEach":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			f := arg(args, 0)
			needFn(f)
			for i := 0; i < len(l.elems); i++ {
				cb(f, l.elems[i], i)
			}
			return undef()
		})
	case "map":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			f := arg(args, 0)
			needFn(f)
			out := make([]Value, len(l.elems))
			for i := 0; i < len(l.elems) && i < len(out); i++ {
				out[i] = cb(f, l.elems[i], i)
			}
			return it.newList("array", out)
		})
	case "filter":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			f := arg(args, 0)
			needFn(f)
			var out []Value
			for i := 0; i < len(l.elems); i++ {
				if it.sem.truthy(cb(f, l.elems[i], i)) {
					out = append(out, l.elems[i])
				}
			}
			return it.newList("array", out)
		})
	case "find", "findIndex", "findLast", "findLastIndex", "some", "every":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			f := arg(args, 0)
			needFn(f)
			order := make([]int, len(l.elems))
			for i := range order {
				order[i] = i
				if strings.HasPrefix(name, "findLast") {
					order[i] = len(l.elems) - 1 - i
				}
			}
			for _, i := range order {
				if i >= len(l.elems) {
					continue
				}
				ok := it.sem.truthy(cb(f, l.elems[i], i))
				switch name {
				case "find", "findLast":
					if ok {
						return l.elems[i]
					}
				case "findIndex", "findLastIndex":
					if ok {
						return num(float64(i))
					}
				case "some":
					if ok {
						return boolV(true)
					}
				case "every":
					if !ok {
						return boolV(false)
					}
				}
			}
			switch name {
			case "findIndex", "findLastIndex":
				return num(-1)
			case "some":
				return boolV(false)
			case "every":
				return boolV(true)
			}
			return undef()
		})
	case "reduce", "reduceRight":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			f := arg(args, 0)
			needFn(f)
			idxs := make([]int, len(l.elems))
			for i := range idxs {
				idxs[i] = i
				if name == "reduceRight" {
					idxs[i] = len(l.elems) - 1 - i
				}
			}
			var acc Value
			start := 0
			if len(args) > 1 {
				acc = args[1]
			} else {
				if len(l.elems) == 0 {
					it.throw(at, "TypeError", "Reduce of empty array with no initial value")
				}
				acc = l.elems[idxs[0]]
				start = 1
			}
			for _, i := range idxs[start:] {
				acc = it.call(f, undef(), []Value{acc, l.elems[i], num(float64(i)), x}, at)
			}
			return acc
		})
	case "fill":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			a, b := rel(arg(args, 1), 0), rel(arg(args, 2), len(l.elems))
			for i := a; i < b; i++ {
				l.elems[i] = arg(args, 0)
			}
			return x
		})
	case "flat":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			depth := 1
			if d := arg(args, 0); d.k != kUndef {
				depth = int(n(d))
			}
			var flat func(es []Value, d int) []Value
			flat = func(es []Value, d int) []Value {
				var out []Value
				for _, e := range es {
					if e.k == kList && d > 0 {
						out = append(out, flat(e.l.elems, d-1)...)
					} else {
						out = append(out, e)
					}
				}
				return out
			}
			return it.newList("array", flat(l.elems, depth))
		})
	case "flatMap":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			f := arg(args, 0)
			var out []Value
			for i := 0; i < len(l.elems); i++ {
				r := cb(f, l.elems[i], i)
				if r.k == kList {
					out = append(out, r.l.elems...)
				} else {
					out = append(out, r)
				}
			}
			return it.newList("array", out)
		})
	case "at":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			i := int(n(arg(args, 0)))
			if i < 0 {
				i += len(l.elems)
			}
			if i < 0 || i >= len(l.elems) {
				return undef()
			}
			return l.elems[i]
		})
	case "keys", "values", "entries":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			out := make([]Value, len(l.elems))
			for i, e := range l.elems {
				switch name {
				case "keys":
					out[i] = num(float64(i))
				case "values":
					out[i] = e
				default:
					out[i] = it.newList("array", []Value{num(float64(i)), e})
				}
			}
			return it.newList("array", out)
		})
	}
	return Value{}, false
}

// mergeSort mengurutkan secara stabil (seperti Array.prototype.sort di V8).
func mergeSort(a []Value, less func(x, y Value) bool) {
	if len(a) < 2 {
		return
	}
	mid := len(a) / 2
	left := append([]Value(nil), a[:mid]...)
	right := append([]Value(nil), a[mid:]...)
	mergeSort(left, less)
	mergeSort(right, less)
	i, j, k := 0, 0, 0
	for i < len(left) && j < len(right) {
		if less(right[j], left[i]) {
			a[k] = right[j]
			j++
		} else {
			a[k] = left[i]
			i++
		}
		k++
	}
	for ; i < len(left); i, k = i+1, k+1 {
		a[k] = left[i]
	}
	for ; j < len(right); j, k = j+1, k+1 {
		a[k] = right[j]
	}
}

func (it *interp) jsMapMember(x Value, name string) (Value, bool) {
	d := x.d
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	switch name {
	case "size":
		return num(float64(d.len())), true
	case "set":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.set(arg(args, 0), arg(args, 1))
			return x
		})
	case "get":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			if v, ok := d.get(arg(args, 0)); ok {
				return v
			}
			return undef()
		})
	case "has":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.has(arg(args, 0))) })
	case "delete":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.del(arg(args, 0))) })
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.keys, d.vals, d.idx = nil, nil, map[string]int{}
			return undef()
		})
	case "forEach":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for i := 0; i < len(d.keys); i++ {
				it.call(arg(args, 0), undef(), []Value{d.vals[i], d.keys[i], x}, at)
			}
			return undef()
		})
	case "keys", "values", "entries":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			out := make([]Value, len(d.keys))
			for i, k := range d.keys {
				switch name {
				case "keys":
					out[i] = k
				case "values":
					out[i] = d.vals[i]
				default:
					out[i] = it.newList("array", []Value{k, d.vals[i]})
				}
			}
			return it.newList("array", out)
		})
	}
	return Value{}, false
}

func (it *interp) jsSetMember(x Value, name string) (Value, bool) {
	d := x.d
	m := func(f builtinFn) (Value, bool) { return bound(x, name, f), true }
	switch name {
	case "size":
		return num(float64(d.len())), true
	case "add":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.set(arg(args, 0), arg(args, 0))
			return x
		})
	case "has":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.has(arg(args, 0))) })
	case "delete":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(d.del(arg(args, 0))) })
	case "clear":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			d.keys, d.vals, d.idx = nil, nil, map[string]int{}
			return undef()
		})
	case "forEach":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			for i := 0; i < len(d.keys); i++ {
				it.call(arg(args, 0), undef(), []Value{d.keys[i], d.keys[i], x}, at)
			}
			return undef()
		})
	case "values", "keys":
		return m(func(it *interp, this Value, args []Value, at ir.Node) Value {
			return it.newList("array", append([]Value(nil), d.keys...))
		})
	}
	return Value{}, false
}

func newBigFromString(s string) (*big.Int, bool) {
	return new(big.Int).SetString(strings.TrimSpace(s), 10)
}

var _ = fmt.Sprint
