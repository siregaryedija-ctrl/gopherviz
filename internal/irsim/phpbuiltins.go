package irsim

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopherviz/internal/ir"
)

func big1() *big.Int        { return new(big.Int) }
func big2(n int64) *big.Int { return big.NewInt(n) }

func (s *phpSem) global(it *interp, name string) (Value, bool) {
	if v, ok := s.consts[name]; ok {
		return v, true
	}
	v, ok := s.globals[strings.ToLower(strings.TrimPrefix(name, "\\"))]
	return v, ok
}

func (it *interp) phpArr(vals ...Value) Value {
	d := it.newDict("array")
	for _, v := range vals {
		d.set(intV(d.next), v)
	}
	return Value{k: kDict, d: d}
}

func (it *interp) needArray(v Value, fn string, pos int, at ir.Node) *Dict {
	if v.k != kDict {
		it.throw(at, "TypeError", fmt.Sprintf("%s(): Argument #%d ($array) must be of type array, %s given", fn, pos, phpTypeName(v)))
	}
	return v.d
}

func (it *interp) setupPHP(s *phpSem) {
	g := s.globals
	fn := func(name string, f builtinFn) { g[name] = bf(name, f) }
	c := s.consts
	c["PHP_EOL"] = strV("\n")
	c["PHP_INT_MAX"] = intV(math.MaxInt64)
	c["PHP_INT_MIN"] = intV(math.MinInt64)
	c["PHP_INT_SIZE"] = intV(8)
	c["PHP_FLOAT_EPSILON"] = floatV(math.Nextafter(1, 2) - 1)
	c["PHP_FLOAT_MAX"] = floatV(math.MaxFloat64)
	c["PHP_VERSION"] = strV("8.4.0")
	c["M_PI"] = floatV(math.Pi)
	c["M_E"] = floatV(math.E)
	c["M_SQRT2"] = floatV(math.Sqrt2)
	c["NAN"] = floatV(math.NaN())
	c["INF"] = floatV(math.Inf(1))
	c["STR_PAD_RIGHT"], c["STR_PAD_LEFT"], c["STR_PAD_BOTH"] = intV(1), intV(0), intV(2)
	c["SORT_REGULAR"], c["SORT_NUMERIC"], c["SORT_STRING"] = intV(0), intV(1), intV(2)
	c["COUNT_RECURSIVE"] = intV(1)
	c["JSON_PRETTY_PRINT"], c["JSON_UNESCAPED_SLASHES"], c["JSON_UNESCAPED_UNICODE"] = intV(128), intV(64), intV(256)
	c["JSON_THROW_ON_ERROR"] = intV(4194304)
	c["ARRAY_FILTER_USE_KEY"], c["ARRAY_FILTER_USE_BOTH"] = intV(2), intV(1)
	c["E_ALL"] = intV(32767)

	// --- exception ---
	excInit := func(it *interp, this Value, args []Value, at nodeRef) {
		if this.k != kObj {
			return
		}
		msg, code := strV(""), intV(0)
		if len(args) > 0 {
			msg = strV(it.sem.str(it, args[0]))
		}
		if len(args) > 1 {
			code = args[1]
		}
		this.o.fields.set(strV("message"), msg)
		this.o.fields.set(strV("code"), code)
	}
	mk := func(name string, super *Class) *Class {
		var cl *Class
		cl = &Class{name: name, native: name, super: super, methods: map[string]*Fn{}, getters: map[string]*Fn{},
			setters: map[string]*Fn{}, statics: it.newDict("statics"), kind: "class"}
		cl.init = excInit
		cl.ctor = func(it *interp, args []Value, at nodeRef) Value {
			it.ids++
			o := &Object{class: cl, fields: it.newDict("fields"), id: it.ids}
			v := Value{k: kObj, o: o}
			excInit(it, v, args, at)
			return v
		}
		it.classes[name] = cl
		return cl
	}
	throwable := mk("Throwable", nil)
	exc := mk("Exception", throwable)
	mk("ErrorException", exc)
	logic := mk("LogicException", exc)
	mk("InvalidArgumentException", logic)
	mk("DomainException", logic)
	mk("LengthException", logic)
	mk("OutOfRangeException", logic)
	runtime := mk("RuntimeException", exc)
	mk("OutOfBoundsException", runtime)
	mk("UnexpectedValueException", runtime)
	mk("OverflowException", runtime)
	mk("UnderflowException", runtime)
	mk("JsonException", exc)
	errC := mk("Error", throwable)
	mk("TypeError", errC)
	mk("ValueError", errC)
	mk("ArgumentCountError", mk("TypeError2", errC))
	arith := mk("ArithmeticError", errC)
	mk("DivisionByZeroError", arith)
	mk("UnhandledMatchError", errC)
	it.classes["ArgumentCountError"].super = it.classes["TypeError"]
	std := &Class{name: "stdClass", methods: map[string]*Fn{}, getters: map[string]*Fn{}, setters: map[string]*Fn{}, statics: it.newDict("statics"), kind: "class"}
	it.classes["stdClass"] = std

	// --- output ---
	fn("echo", func(it *interp, this Value, args []Value, at ir.Node) Value {
		for _, a := range args {
			it.out(it.sem.str(it, a))
		}
		return null()
	})
	fn("print", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.out(it.sem.str(it, arg(args, 0)))
		return intV(1)
	})
	fn("printf", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s := it.phpSprintf(it.sem.str(it, arg(args, 0)), args[min(1, len(args)):], at)
		it.out(s)
		return intV(int64(len(s)))
	})
	fn("sprintf", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return strV(it.phpSprintf(it.sem.str(it, arg(args, 0)), args[min(1, len(args)):], at))
	})
	fn("print_r", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s := it.phpPrintR(arg(args, 0), 0)
		if it.sem.truthy(arg(args, 1)) {
			return strV(s)
		}
		it.out(s)
		return boolV(true)
	})
	fn("var_dump", func(it *interp, this Value, args []Value, at ir.Node) Value {
		for _, a := range args {
			it.out(it.phpVarDump(a, 0))
		}
		return null()
	})
	fn("var_export", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s := it.phpVarExport(arg(args, 0), 0)
		if it.sem.truthy(arg(args, 1)) {
			return strV(s)
		}
		it.out(s)
		return null()
	})
	fn("json_encode", func(it *interp, this Value, args []Value, at ir.Node) Value {
		flags := int64(0)
		if len(args) > 1 {
			flags = int64(toF(args[1]))
		}
		return strV(it.phpJSON(arg(args, 0), flags, 0))
	})
	fn("json_decode", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.phpJSONDecode(it.sem.str(it, arg(args, 0)), it.sem.truthy(arg(args, 1)))
	})

	// --- string ---
	str1 := func(name string, f func(string) string) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			return strV(f(it.sem.str(it, arg(args, 0))))
		})
	}
	str1("strtoupper", strings.ToUpper)
	str1("strtolower", strings.ToLower)
	str1("mb_strtoupper", strings.ToUpper)
	str1("mb_strtolower", strings.ToLower)
	str1("ucwords", ucwords)
	str1("strrev", func(s string) string {
		b := []byte(s)
		for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
		return string(b)
	})
	str1("ucfirst", func(s string) string {
		if s == "" {
			return s
		}
		return strings.ToUpper(s[:1]) + s[1:]
	})
	str1("lcfirst", func(s string) string {
		if s == "" {
			return s
		}
		return strings.ToLower(s[:1]) + s[1:]
	})
	str1("nl2br", func(s string) string { return strings.ReplaceAll(s, "\n", "<br />\n") })
	str1("htmlspecialchars", func(s string) string {
		r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#039;")
		return r.Replace(s)
	})
	trimFn := func(name string) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			s := it.sem.str(it, arg(args, 0))
			cut := " \t\n\r\x00\x0B"
			if len(args) > 1 {
				cut = it.sem.str(it, args[1])
			}
			switch name {
			case "ltrim":
				return strV(strings.TrimLeft(s, cut))
			case "rtrim", "chop":
				return strV(strings.TrimRight(s, cut))
			}
			return strV(strings.Trim(s, cut))
		})
	}
	trimFn("trim")
	trimFn("ltrim")
	trimFn("rtrim")
	trimFn("chop")
	fn("strlen", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return intV(int64(len(it.sem.str(it, arg(args, 0)))))
	})
	fn("mb_strlen", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return intV(int64(utf8.RuneCountInString(it.sem.str(it, arg(args, 0)))))
	})
	fn("str_repeat", func(it *interp, this Value, args []Value, at ir.Node) Value {
		n := int(toF(arg(args, 1)))
		if n < 0 {
			it.throw(at, "ValueError", "str_repeat(): Argument #2 ($times) must be greater than or equal to 0")
		}
		return strV(strings.Repeat(it.sem.str(it, arg(args, 0)), n))
	})
	fn("str_pad", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s := it.sem.str(it, arg(args, 0))
		w := int(toF(arg(args, 1)))
		pad := " "
		if len(args) > 2 {
			pad = it.sem.str(it, args[2])
		}
		mode := int64(1)
		if len(args) > 3 {
			mode = int64(toF(args[3]))
		}
		need := w - len(s)
		if need <= 0 || pad == "" {
			return strV(s)
		}
		mk := func(n int) string { return strings.Repeat(pad, n/len(pad)+1)[:n] }
		switch mode {
		case 0:
			return strV(mk(need) + s)
		case 2:
			l := need / 2
			return strV(mk(l) + s + mk(need-l))
		}
		return strV(s + mk(need))
	})
	substr := func(s string, start, length int, hasLen bool) string {
		n := len(s)
		if start < 0 {
			start = max(0, n+start)
		}
		if start > n {
			return ""
		}
		end := n
		if hasLen {
			if length < 0 {
				end = n + length
			} else {
				end = min(n, start+length)
			}
		}
		if end <= start {
			return ""
		}
		return s[start:end]
	}
	fn("substr", func(it *interp, this Value, args []Value, at ir.Node) Value {
		hasLen := len(args) > 2 && args[2].k != kNull
		l := 0
		if hasLen {
			l = int(toF(args[2]))
		}
		return strV(substr(it.sem.str(it, arg(args, 0)), int(toF(arg(args, 1))), l, hasLen))
	})
	fn("mb_substr", g["substr"].fn.builtin)
	posFn := func(name string, last, ci bool) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			h, n := it.sem.str(it, arg(args, 0)), it.sem.str(it, arg(args, 1))
			if ci {
				h, n = strings.ToLower(h), strings.ToLower(n)
			}
			off := 0
			if len(args) > 2 {
				off = int(toF(args[2]))
				if off < 0 {
					off += len(h)
				}
			}
			if off > len(h) {
				it.throw(at, "ValueError", name+"(): Argument #3 ($offset) must be contained in argument #1 ($haystack)")
			}
			var i int
			if last {
				i = strings.LastIndex(h[off:], n)
			} else {
				i = strings.Index(h[off:], n)
			}
			if i < 0 {
				return boolV(false)
			}
			return intV(int64(i + off))
		})
	}
	posFn("strpos", false, false)
	posFn("stripos", false, true)
	posFn("strrpos", true, false)
	posFn("mb_strpos", false, false)
	fn("str_contains", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return boolV(strings.Contains(it.sem.str(it, arg(args, 0)), it.sem.str(it, arg(args, 1))))
	})
	fn("str_starts_with", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return boolV(strings.HasPrefix(it.sem.str(it, arg(args, 0)), it.sem.str(it, arg(args, 1))))
	})
	fn("str_ends_with", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return boolV(strings.HasSuffix(it.sem.str(it, arg(args, 0)), it.sem.str(it, arg(args, 1))))
	})
	fn("substr_count", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return intV(int64(strings.Count(it.sem.str(it, arg(args, 0)), it.sem.str(it, arg(args, 1)))))
	})
	fn("str_replace", func(it *interp, this Value, args []Value, at ir.Node) Value {
		search, repl, subj := arg(args, 0), arg(args, 1), arg(args, 2)
		var pairs [][2]string
		if search.k == kDict {
			for i, sv := range search.d.vals {
				r := ""
				if repl.k == kDict {
					if i < len(repl.d.vals) {
						r = it.sem.str(it, repl.d.vals[i])
					}
				} else {
					r = it.sem.str(it, repl)
				}
				pairs = append(pairs, [2]string{it.sem.str(it, sv), r})
			}
		} else {
			pairs = [][2]string{{it.sem.str(it, search), it.sem.str(it, repl)}}
		}
		apply := func(s string) string {
			for _, p := range pairs {
				if p[0] != "" {
					s = strings.ReplaceAll(s, p[0], p[1])
				}
			}
			return s
		}
		if subj.k == kDict {
			d := it.newDict("array")
			for i, k := range subj.d.keys {
				d.set(k, strV(apply(it.sem.str(it, subj.d.vals[i]))))
			}
			return Value{k: kDict, d: d}
		}
		return strV(apply(it.sem.str(it, subj)))
	})
	fn("explode", func(it *interp, this Value, args []Value, at ir.Node) Value {
		sep, s := it.sem.str(it, arg(args, 0)), it.sem.str(it, arg(args, 1))
		if sep == "" {
			it.throw(at, "ValueError", "explode(): Argument #1 ($separator) cannot be empty")
		}
		var parts []string
		if len(args) > 2 {
			lim := int(toF(args[2]))
			switch {
			case lim > 0:
				parts = strings.SplitN(s, sep, lim)
			case lim < 0:
				parts = strings.Split(s, sep)
				parts = parts[:max(0, len(parts)+lim)]
			default:
				parts = strings.SplitN(s, sep, 1)
			}
		} else {
			parts = strings.Split(s, sep)
		}
		out := make([]Value, len(parts))
		for i, p := range parts {
			out[i] = strV(p)
		}
		return it.phpArr(out...)
	})
	implode := func(it *interp, this Value, args []Value, at ir.Node) Value {
		sep, arr := arg(args, 0), arg(args, 1)
		if sep.k == kDict {
			sep, arr = arr, sep
		}
		if arr.k != kDict {
			it.throw(at, "TypeError", "implode(): Argument #2 ($array) must be of type ?array")
		}
		parts := make([]string, len(arr.d.vals))
		for i, v := range arr.d.vals {
			parts[i] = it.sem.str(it, v)
		}
		return strV(strings.Join(parts, it.sem.str(it, sep)))
	}
	fn("implode", implode)
	fn("join", implode)
	fn("str_split", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s := it.sem.str(it, arg(args, 0))
		n := 1
		if len(args) > 1 {
			n = int(toF(args[1]))
		}
		var out []Value
		for i := 0; i < len(s); i += n {
			out = append(out, strV(s[i:min(len(s), i+n)]))
		}
		if len(out) == 0 {
			out = []Value{strV("")}
		}
		return it.phpArr(out...)
	})
	fn("strcmp", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return intV(int64(strings.Compare(it.sem.str(it, arg(args, 0)), it.sem.str(it, arg(args, 1)))))
	})
	fn("strcasecmp", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return intV(int64(strings.Compare(strings.ToLower(it.sem.str(it, arg(args, 0))), strings.ToLower(it.sem.str(it, arg(args, 1))))))
	})
	fn("str_word_count", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return intV(int64(len(strings.Fields(it.sem.str(it, arg(args, 0))))))
	})
	fn("number_format", func(it *interp, this Value, args []Value, at ir.Node) Value {
		dec := 0
		if len(args) > 1 {
			dec = int(toF(args[1]))
		}
		dp, ts := ".", ","
		if len(args) > 2 {
			dp = it.sem.str(it, args[2])
		}
		if len(args) > 3 {
			ts = it.sem.str(it, args[3])
		}
		return strV(phpNumberFormat(toF(it.phpToNumber(arg(args, 0), "+", intV(0), at)), dec, dp, ts))
	})
	fn("chr", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return strV(string([]byte{byte(int64(toF(arg(args, 0))) & 255)}))
	})
	fn("ord", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s := it.sem.str(it, arg(args, 0))
		if s == "" {
			return intV(0)
		}
		return intV(int64(s[0]))
	})
	conv := func(name string, base int, toStr bool) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			if toStr {
				return strV(strconv.FormatInt(int64(toF(it.phpToNumber(arg(args, 0), "+", intV(0), at))), base))
			}
			n, _ := strconv.ParseInt(it.sem.str(it, arg(args, 0)), base, 64)
			return intV(n)
		})
	}
	conv("dechex", 16, true)
	conv("decbin", 2, true)
	conv("decoct", 8, true)
	conv("hexdec", 16, false)
	conv("bindec", 2, false)
	conv("octdec", 8, false)

	// --- tipe ---
	fn("intval", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if len(args) > 1 && arg(args, 0).k == kStr {
			n, _ := strconv.ParseInt(strings.TrimSpace(args[0].s), int(toF(args[1])), 64)
			return intV(n)
		}
		return it.sem.convert(it, arg(args, 0), "int")
	})
	fn("floatval", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.sem.convert(it, arg(args, 0), "float")
	})
	fn("doubleval", g["floatval"].fn.builtin)
	fn("strval", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return strV(it.sem.str(it, arg(args, 0)))
	})
	fn("boolval", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return boolV(it.sem.truthy(arg(args, 0)))
	})
	fn("gettype", func(it *interp, this Value, args []Value, at ir.Node) Value {
		switch v := arg(args, 0); v.k {
		case kInt:
			return strV("integer")
		case kFloat:
			return strV("double")
		case kStr:
			return strV("string")
		case kBool:
			return strV("boolean")
		case kDict:
			return strV("array")
		case kObj, kFunc:
			return strV("object")
		}
		return strV("NULL")
	})
	is := func(name string, f func(Value) bool) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value { return boolV(f(arg(args, 0))) })
	}
	is("is_int", func(v Value) bool { return v.k == kInt })
	is("is_integer", func(v Value) bool { return v.k == kInt })
	is("is_float", func(v Value) bool { return v.k == kFloat })
	is("is_string", func(v Value) bool { return v.k == kStr })
	is("is_bool", func(v Value) bool { return v.k == kBool })
	is("is_array", func(v Value) bool { return v.k == kDict })
	is("is_null", func(v Value) bool { return v.k == kNull || v.k == kUndef })
	is("is_object", func(v Value) bool { return v.k == kObj })
	is("is_callable", func(v Value) bool { return v.k == kFunc })
	is("is_numeric", func(v Value) bool {
		if isNum(v) {
			return true
		}
		if v.k == kStr {
			_, k := phpNumeric(v.s)
			return k == 2 && strings.TrimRight(v.s, " \t\n") == v.s
		}
		return false
	})
	fn("get_class", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if v := arg(args, 0); v.k == kObj {
			return strV(v.o.class.name)
		}
		return boolV(false)
	})
	fn("method_exists", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := arg(args, 0)
		name := it.sem.str(it, arg(args, 1))
		switch v.k {
		case kObj:
			return boolV(findMethod(v.o.class, name) != nil)
		case kStr:
			if c := it.findClass(v.s, nil); c != nil {
				return boolV(findMethod(c, name) != nil)
			}
		}
		return boolV(false)
	})
	fn("property_exists", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := arg(args, 0)
		return boolV(v.k == kObj && v.o.fields.has(strV(it.sem.str(it, arg(args, 1)))))
	})
	fn("function_exists", func(it *interp, this Value, args []Value, at ir.Node) Value {
		name := strings.ToLower(it.sem.str(it, arg(args, 0)))
		_, builtin := s.globals[name]
		return boolV(builtin || len(it.funcs[name]) > 0)
	})
	fn("define", func(it *interp, this Value, args []Value, at ir.Node) Value {
		s.consts[it.sem.str(it, arg(args, 0))] = arg(args, 1)
		return boolV(true)
	})
	fn("defined", func(it *interp, this Value, args []Value, at ir.Node) Value {
		_, ok := s.consts[it.sem.str(it, arg(args, 0))]
		return boolV(ok)
	})
	fn("constant", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return s.consts[it.sem.str(it, arg(args, 0))]
	})
	fn("call_user_func", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.call(it.phpCallable(arg(args, 0), at), undef(), args[min(1, len(args)):], at)
	})
	fn("call_user_func_array", func(it *interp, this Value, args []Value, at ir.Node) Value {
		var a []Value
		if l := arg(args, 1); l.k == kDict {
			a = l.d.vals
		}
		return it.call(it.phpCallable(arg(args, 0), at), undef(), a, at)
	})
	fn("readline", func(it *interp, this Value, args []Value, at ir.Node) Value {
		if len(args) > 0 {
			it.out(it.sem.str(it, args[0]))
		}
		if s, ok := it.userText(true); ok {
			return strV(s)
		}
		return boolV(false)
	})
	fn("fgets", func(it *interp, this Value, args []Value, at ir.Node) Value { // fgets(STDIN): baris + "\n"
		if s, ok := it.userText(true); ok {
			return strV(s + "\n")
		}
		return boolV(false)
	})
	for _, name := range []string{"fscanf", "date", "time", "microtime", "file_get_contents", "fopen"} {
		nm := name
		fn(nm, func(it *interp, this Value, args []Value, at ir.Node) Value {
			return unknown(nm + "() depends on input/time/files, so it cannot be simulated")
		})
	}
	c["STDIN"] = unknown("STDIN")

	// --- math ---
	n := func(it *interp, v Value, at ir.Node) Value { return it.phpToNumber(v, "+", intV(0), at) }
	fn("abs", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := n(it, arg(args, 0), at)
		if v.k == kInt {
			if v.i < 0 {
				return intV(-v.i)
			}
			return v
		}
		return floatV(math.Abs(v.f))
	})
	f1 := func(name string, f func(float64) float64) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			return floatV(f(toF(n(it, arg(args, 0), at))))
		})
	}
	f1("floor", math.Floor)
	f1("ceil", math.Ceil)
	f1("sqrt", math.Sqrt)
	f1("exp", math.Exp)
	f1("log10", math.Log10)
	f1("log2", math.Log2)
	f1("sin", math.Sin)
	f1("cos", math.Cos)
	f1("tan", math.Tan)
	f1("deg2rad", func(x float64) float64 { return x * math.Pi / 180 })
	f1("rad2deg", func(x float64) float64 { return x * 180 / math.Pi })
	fn("log", func(it *interp, this Value, args []Value, at ir.Node) Value {
		x := toF(n(it, arg(args, 0), at))
		if len(args) > 1 {
			return floatV(math.Log(x) / math.Log(toF(args[1])))
		}
		return floatV(math.Log(x))
	})
	fn("pi", func(it *interp, this Value, args []Value, at ir.Node) Value { return floatV(math.Pi) })
	fn("round", func(it *interp, this Value, args []Value, at ir.Node) Value {
		p := 0
		if len(args) > 1 {
			p = int(toF(args[1]))
		}
		return floatV(phpRound(toF(n(it, arg(args, 0), at)), p))
	})
	fn("pow", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.sem.binary(it, "**", arg(args, 0), arg(args, 1), at)
	})
	fn("intdiv", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := int64(toF(n(it, arg(args, 0), at))), int64(toF(n(it, arg(args, 1), at)))
		if b == 0 {
			it.throw(at, "DivisionByZeroError", "Division by zero")
		}
		return intV(a / b)
	})
	fn("fmod", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return floatV(math.Mod(toF(n(it, arg(args, 0), at)), toF(n(it, arg(args, 1), at))))
	})
	mm := func(name string, want int) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			items := args
			if len(args) == 1 && args[0].k == kDict {
				items = args[0].d.vals
			}
			if len(items) == 0 {
				it.throw(at, "ValueError", name+"(): Argument #1 ($value) must contain at least one element")
			}
			best := items[0]
			for _, x := range items[1:] {
				if it.phpLooseCmp(x, best) == want {
					best = x
				}
			}
			return best
		})
	}
	mm("max", 1)
	mm("min", -1)
	randFn := func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.warn("rand()/mt_rand() is simulated with a fixed seed; results differ from a real run")
		if len(args) >= 2 {
			a, b := int64(toF(args[0])), int64(toF(args[1]))
			if b < a {
				a, b = b, a
			}
			return intV(a + it.rng.Int63n(b-a+1))
		}
		return intV(it.rng.Int63n(2147483648))
	}
	fn("rand", randFn)
	fn("mt_rand", randFn)
	fn("random_int", randFn)

	// --- array ---
	fn("count", func(it *interp, this Value, args []Value, at ir.Node) Value {
		v := arg(args, 0)
		switch v.k {
		case kDict:
			if it.sem.truthy(arg(args, 1)) {
				var cnt func(d *Dict) int
				cnt = func(d *Dict) int {
					t := d.len()
					for _, x := range d.vals {
						if x.k == kDict {
							t += cnt(x.d)
						}
					}
					return t
				}
				return intV(int64(cnt(v.d)))
			}
			return intV(int64(v.d.len()))
		case kObj:
			if m := findMethod(v.o.class, "count"); m != nil {
				return it.callFunc(&Func{decl: m, env: v.o.class.env}, v, argList{}, at)
			}
		case kUnknown:
			return v
		}
		it.throw(at, "TypeError", fmt.Sprintf("count(): Argument #1 ($value) must be of type Countable|array, %s given", phpTypeName(v)))
		return Value{}
	})
	g["sizeof"] = g["count"]
	fn("array_push", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.needArray(arg(args, 0), "array_push", 1, at)
		for _, v := range args[1:] {
			d.set(intV(d.next), copyFor(it, v))
		}
		return intV(int64(d.len()))
	})
	fn("array_pop", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.needArray(arg(args, 0), "array_pop", 1, at)
		if d.len() == 0 {
			return null()
		}
		k, v := d.keys[d.len()-1], d.vals[d.len()-1]
		d.del(k)
		d.next = 0
		for _, kk := range d.keys {
			if kk.k == kInt && kk.i >= d.next {
				d.next = kk.i + 1
			}
		}
		return v
	})
	reindex := func(d *Dict) {
		var keys []Value
		next := int64(0)
		for _, k := range d.keys {
			if k.k == kInt {
				keys = append(keys, intV(next))
				next++
			} else {
				keys = append(keys, k)
			}
		}
		d.keys, d.next = keys, next
		d.reindex()
	}
	fn("array_shift", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.needArray(arg(args, 0), "array_shift", 1, at)
		if d.len() == 0 {
			return null()
		}
		v := d.vals[0]
		d.del(d.keys[0])
		reindex(d)
		return v
	})
	fn("array_unshift", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.needArray(arg(args, 0), "array_unshift", 1, at)
		keys := append(make([]Value, len(args)-1), d.keys...)
		vals := append(append([]Value(nil), args[1:]...), d.vals...)
		for i := range args[1:] {
			keys[i] = intV(0)
		}
		d.keys, d.vals = keys, vals
		reindex(d)
		return intV(int64(d.len()))
	})
	fn("array_keys", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.needArray(arg(args, 0), "array_keys", 1, at)
		var out []Value
		for i, k := range d.keys {
			if len(args) > 1 && it.phpLooseCmp(d.vals[i], args[1]) != 0 {
				continue
			}
			out = append(out, k)
		}
		return it.phpArr(out...)
	})
	fn("array_values", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return it.phpArr(it.needArray(arg(args, 0), "array_values", 1, at).vals...)
	})
	fn("array_merge", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.newDict("array")
		for i, a := range args {
			src := it.needArray(a, "array_merge", i+1, at)
			for j, k := range src.keys {
				if k.k == kInt {
					d.set(intV(d.next), src.vals[j])
				} else {
					d.set(k, src.vals[j])
				}
			}
		}
		return Value{k: kDict, d: d}
	})
	fn("array_combine", func(it *interp, this Value, args []Value, at ir.Node) Value {
		ks, vs := it.needArray(arg(args, 0), "array_combine", 1, at), it.needArray(arg(args, 1), "array_combine", 2, at)
		if ks.len() != vs.len() {
			it.throw(at, "ValueError", "array_combine(): Argument #1 ($keys) and argument #2 ($values) must have the same number of elements")
		}
		d := it.newDict("array")
		for i, k := range ks.vals {
			d.set(it.dictKey(k), vs.vals[i])
		}
		return Value{k: kDict, d: d}
	})
	fn("array_flip", func(it *interp, this Value, args []Value, at ir.Node) Value {
		src := it.needArray(arg(args, 0), "array_flip", 1, at)
		d := it.newDict("array")
		for i, k := range src.keys {
			d.set(it.dictKey(src.vals[i]), k)
		}
		return Value{k: kDict, d: d}
	})
	fn("array_slice", func(it *interp, this Value, args []Value, at ir.Node) Value {
		src := it.needArray(arg(args, 0), "array_slice", 1, at)
		off := int(toF(arg(args, 1)))
		total := src.len()
		if off < 0 {
			off = max(0, total+off)
		}
		end := total
		if len(args) > 2 && args[2].k != kNull {
			l := int(toF(args[2]))
			if l < 0 {
				end = total + l
			} else {
				end = min(total, off+l)
			}
		}
		preserve := it.sem.truthy(arg(args, 3))
		d := it.newDict("array")
		for i := off; i < end; i++ {
			k := src.keys[i]
			if k.k == kInt && !preserve {
				d.set(intV(d.next), src.vals[i])
			} else {
				d.set(k, src.vals[i])
			}
		}
		return Value{k: kDict, d: d}
	})
	fn("array_splice", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.needArray(arg(args, 0), "array_splice", 1, at)
		total := d.len()
		off := int(toF(arg(args, 1)))
		if off < 0 {
			off = max(0, total+off)
		}
		off = min(off, total)
		cnt := total - off
		if len(args) > 2 && args[2].k != kNull {
			cnt = int(toF(args[2]))
			if cnt < 0 {
				cnt = max(0, total+cnt-off)
			}
			cnt = min(cnt, total-off)
		}
		removed := it.phpArr(d.vals[off : off+cnt]...)
		var ins []Value
		if len(args) > 3 {
			if args[3].k == kDict {
				ins = args[3].d.vals
			} else {
				ins = []Value{args[3]}
			}
		}
		keys := append(append(append([]Value(nil), d.keys[:off]...), make([]Value, len(ins))...), d.keys[off+cnt:]...)
		vals := append(append(append([]Value(nil), d.vals[:off]...), ins...), d.vals[off+cnt:]...)
		for i := range ins {
			keys[off+i] = intV(0)
		}
		d.keys, d.vals = keys, vals
		reindex(d)
		return removed
	})
	fn("array_map", func(it *interp, this Value, args []Value, at ir.Node) Value {
		f := arg(args, 0)
		src := it.needArray(arg(args, 1), "array_map", 2, at)
		d := it.newDict("array")
		if len(args) > 2 {
			for i := range src.vals {
				var a []Value
				for _, other := range args[1:] {
					if other.k == kDict && i < other.d.len() {
						a = append(a, other.d.vals[i])
					} else {
						a = append(a, null())
					}
				}
				d.set(intV(d.next), it.call(it.phpCallable(f, at), undef(), a, at))
			}
			return Value{k: kDict, d: d}
		}
		for i, k := range src.keys {
			v := src.vals[i]
			if f.k != kNull {
				v = it.call(it.phpCallable(f, at), undef(), []Value{v}, at)
			}
			d.set(k, v)
		}
		return Value{k: kDict, d: d}
	})
	fn("array_filter", func(it *interp, this Value, args []Value, at ir.Node) Value {
		src := it.needArray(arg(args, 0), "array_filter", 1, at)
		f := arg(args, 1)
		mode := int64(0)
		if len(args) > 2 {
			mode = int64(toF(args[2]))
		}
		d := it.newDict("array")
		for i, k := range src.keys {
			v := src.vals[i]
			keep := it.sem.truthy(v)
			if f.k == kFunc || f.k == kStr {
				var a []Value
				switch mode {
				case 2:
					a = []Value{k}
				case 1:
					a = []Value{v, k}
				default:
					a = []Value{v}
				}
				keep = it.sem.truthy(it.call(it.phpCallable(f, at), undef(), a, at))
			}
			if keep {
				d.set(k, v)
			}
		}
		return Value{k: kDict, d: d}
	})
	fn("array_reduce", func(it *interp, this Value, args []Value, at ir.Node) Value {
		src := it.needArray(arg(args, 0), "array_reduce", 1, at)
		acc := arg(args, 2)
		if len(args) < 3 {
			acc = null()
		}
		for _, v := range src.vals {
			acc = it.call(it.phpCallable(arg(args, 1), at), undef(), []Value{acc, v}, at)
		}
		return acc
	})
	sumProd := func(name string, mul bool) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			acc := intV(0)
			op := "+"
			if mul {
				acc, op = intV(1), "*"
			}
			for _, v := range it.needArray(arg(args, 0), name, 1, at).vals {
				acc = it.sem.binary(it, op, acc, v, at)
			}
			return acc
		})
	}
	sumProd("array_sum", false)
	sumProd("array_product", true)
	fn("in_array", func(it *interp, this Value, args []Value, at ir.Node) Value {
		strict := it.sem.truthy(arg(args, 2))
		for _, v := range it.needArray(arg(args, 1), "in_array", 2, at).vals {
			if strict && it.phpIdentical(v, args[0]) || !strict && it.phpLooseCmp(v, args[0]) == 0 {
				return boolV(true)
			}
		}
		return boolV(false)
	})
	fn("array_search", func(it *interp, this Value, args []Value, at ir.Node) Value {
		strict := it.sem.truthy(arg(args, 2))
		src := it.needArray(arg(args, 1), "array_search", 2, at)
		for i, v := range src.vals {
			if strict && it.phpIdentical(v, args[0]) || !strict && it.phpLooseCmp(v, args[0]) == 0 {
				return src.keys[i]
			}
		}
		return boolV(false)
	})
	keyExists := func(it *interp, this Value, args []Value, at ir.Node) Value {
		return boolV(it.needArray(arg(args, 1), "array_key_exists", 2, at).has(it.dictKey(arg(args, 0))))
	}
	fn("array_key_exists", keyExists)
	fn("key_exists", keyExists)
	fn("array_key_first", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.needArray(arg(args, 0), "array_key_first", 1, at)
		if d.len() == 0 {
			return null()
		}
		return d.keys[0]
	})
	fn("array_key_last", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.needArray(arg(args, 0), "array_key_last", 1, at)
		if d.len() == 0 {
			return null()
		}
		return d.keys[d.len()-1]
	})
	fn("array_unique", func(it *interp, this Value, args []Value, at ir.Node) Value {
		src := it.needArray(arg(args, 0), "array_unique", 1, at)
		d := it.newDict("array")
		var seen []Value
	outer:
		for i, v := range src.vals {
			for _, s := range seen {
				if it.sem.str(it, s) == it.sem.str(it, v) {
					continue outer
				}
			}
			seen = append(seen, v)
			d.set(src.keys[i], v)
		}
		return Value{k: kDict, d: d}
	})
	fn("array_reverse", func(it *interp, this Value, args []Value, at ir.Node) Value {
		src := it.needArray(arg(args, 0), "array_reverse", 1, at)
		preserve := it.sem.truthy(arg(args, 1))
		d := it.newDict("array")
		for i := src.len() - 1; i >= 0; i-- {
			k := src.keys[i]
			if k.k == kInt && !preserve {
				d.set(intV(d.next), src.vals[i])
			} else {
				d.set(k, src.vals[i])
			}
		}
		return Value{k: kDict, d: d}
	})
	fn("array_fill", func(it *interp, this Value, args []Value, at ir.Node) Value {
		start, cnt := int64(toF(arg(args, 0))), int(toF(arg(args, 1)))
		d := it.newDict("array")
		for i := 0; i < cnt; i++ {
			d.set(intV(start+int64(i)), copyFor(it, arg(args, 2)))
		}
		return Value{k: kDict, d: d}
	})
	fn("array_fill_keys", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.newDict("array")
		for _, k := range it.needArray(arg(args, 0), "array_fill_keys", 1, at).vals {
			d.set(it.dictKey(k), copyFor(it, arg(args, 1)))
		}
		return Value{k: kDict, d: d}
	})
	fn("array_count_values", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.newDict("array")
		for _, v := range it.needArray(arg(args, 0), "array_count_values", 1, at).vals {
			k := it.dictKey(v)
			cur, _ := d.get(k)
			d.set(k, intV(cur.i+1))
		}
		return Value{k: kDict, d: d}
	})
	fn("array_column", func(it *interp, this Value, args []Value, at ir.Node) Value {
		d := it.newDict("array")
		col := arg(args, 1)
		for _, row := range it.needArray(arg(args, 0), "array_column", 1, at).vals {
			var v Value
			var ok bool
			switch row.k {
			case kDict:
				v, ok = row.d.get(it.dictKey(col))
			case kObj:
				v, ok = row.o.fields.get(strV(it.sem.str(it, col)))
			}
			if !ok {
				continue
			}
			if len(args) > 2 && row.k == kDict {
				if k, ok := row.d.get(it.dictKey(args[2])); ok {
					d.set(it.dictKey(k), v)
					continue
				}
			}
			d.set(intV(d.next), v)
		}
		return Value{k: kDict, d: d}
	})
	fn("array_is_list", func(it *interp, this Value, args []Value, at ir.Node) Value {
		return boolV(phpIsList(it.needArray(arg(args, 0), "array_is_list", 1, at)))
	})
	fn("range", func(it *interp, this Value, args []Value, at ir.Node) Value {
		a, b := arg(args, 0), arg(args, 1)
		step := 1.0
		if len(args) > 2 {
			step = math.Abs(toF(it.phpToNumber(args[2], "+", intV(0), at)))
		}
		if step == 0 {
			it.throw(at, "ValueError", "range(): Argument #3 ($step) cannot be 0")
		}
		if a.k == kStr && b.k == kStr && len(a.s) == 1 && len(b.s) == 1 {
			var out []Value
			x, y := int(a.s[0]), int(b.s[0])
			st := int(step)
			if x <= y {
				for c := x; c <= y; c += st {
					out = append(out, strV(string(rune(c))))
				}
			} else {
				for c := x; c >= y; c -= st {
					out = append(out, strV(string(rune(c))))
				}
			}
			return it.phpArr(out...)
		}
		x, y := it.phpToNumber(a, "+", intV(0), at), it.phpToNumber(b, "+", intV(0), at)
		isInt := x.k == kInt && y.k == kInt && step == math.Trunc(step)
		var out []Value
		fx, fy := toF(x), toF(y)
		if fx <= fy {
			for v := fx; v <= fy+1e-9 && len(out) < 1_000_000; v += step {
				out = append(out, numOrInt(v, isInt))
			}
		} else {
			for v := fx; v >= fy-1e-9 && len(out) < 1_000_000; v -= step {
				out = append(out, numOrInt(v, isInt))
			}
		}
		return it.phpArr(out...)
	})
	cmpVals := func(a, b Value) int {
		c := it.phpLooseCmp(a, b)
		if c == 2 {
			return 0
		}
		return c
	}
	sortFn := func(name string, keep, byKey, desc bool) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			d := it.needArray(arg(args, 0), name, 1, at)
			it.phpSort(d, func(ak, av, bk, bv Value) bool {
				x, y := av, bv
				if byKey {
					x, y = ak, bk
				}
				c := cmpVals(x, y)
				if desc {
					return c > 0
				}
				return c < 0
			}, keep)
			return boolV(true)
		})
	}
	sortFn("sort", false, false, false)
	sortFn("rsort", false, false, true)
	sortFn("asort", true, false, false)
	sortFn("arsort", true, false, true)
	sortFn("ksort", true, true, false)
	sortFn("krsort", true, true, true)
	userSort := func(name string, keep, byKey bool) {
		fn(name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			d := it.needArray(arg(args, 0), name, 1, at)
			f := it.phpCallable(arg(args, 1), at)
			it.phpSort(d, func(ak, av, bk, bv Value) bool {
				x, y := av, bv
				if byKey {
					x, y = ak, bk
				}
				return toF(it.phpToNumber(it.call(f, undef(), []Value{x, y}, at), "+", intV(0), at)) < 0
			}, keep)
			return boolV(true)
		})
	}
	userSort("usort", false, false)
	userSort("uasort", true, false)
	userSort("uksort", true, true)
	fn("shuffle", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.warn("shuffle() is simulated with a fixed seed")
		d := it.needArray(arg(args, 0), "shuffle", 1, at)
		it.rng.Shuffle(len(d.vals), func(i, j int) { d.vals[i], d.vals[j] = d.vals[j], d.vals[i] })
		for i := range d.keys {
			d.keys[i] = intV(int64(i))
		}
		d.next = int64(len(d.keys))
		d.reindex()
		return boolV(true)
	})
	fn("compact", func(it *interp, this Value, args []Value, at ir.Node) Value {
		it.structural("compact() is not simulated yet")
		return Value{}
	})
}

func numOrInt(f float64, isInt bool) Value {
	if isInt {
		return intV(int64(math.Round(f)))
	}
	return floatV(f)
}

// phpCallable menerima closure, nama fungsi ("strtoupper"), atau [obj, "method"].
func (it *interp) phpCallable(v Value, at ir.Node) Value {
	switch v.k {
	case kFunc:
		return v
	case kStr:
		name := strings.ToLower(v.s)
		if fs := it.funcs[name]; len(fs) > 0 {
			return Value{k: kFunc, fn: &Func{name: v.s, decl: fs[0], env: it.globals}}
		}
		if g, ok := it.sem.global(it, name); ok && g.k == kFunc {
			return g
		}
		if cls, meth, ok := strings.Cut(v.s, "::"); ok {
			return it.getProp(Value{k: kClass, c: it.findClass(cls, nil)}, meth, at)
		}
	case kDict:
		if v.d.len() == 2 {
			obj, m := v.d.vals[0], v.d.vals[1]
			if obj.k == kStr {
				obj = Value{k: kClass, c: it.findClass(obj.s, nil)}
			}
			return it.getProp(obj, it.sem.str(it, m), at)
		}
	}
	it.throw(at, "TypeError", "Argument must be a valid callback")
	return Value{}
}

func (s *phpSem) member(it *interp, x Value, name string, at ir.Node) (Value, bool) {
	if x.k == kObj && instanceOf(x.o.class, "Throwable") {
		get := func(field string) (Value, bool) {
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
				v, ok := this.o.fields.get(strV(field))
				if !ok {
					return null()
				}
				return v
			}), true
		}
		switch name {
		case "getMessage":
			return get("message")
		case "getCode":
			return get("code")
		case "getPrevious":
			return get("previous")
		case "getLine":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value { return intV(0) }), true
		case "getFile", "getTraceAsString":
			return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value { return strV("") }), true
		}
	}
	if x.k == kClass && name == "class" {
		return strV(x.c.name), true
	}
	if x.k == kObj && x.o.class.kind == "enum" {
		switch name {
		case "name", "value":
			v, ok := x.o.fields.get(strV(name))
			return v, ok
		}
	}
	if x.k == kClass && x.c.kind == "enum" && name == "cases" {
		return bound(x, name, func(it *interp, this Value, args []Value, at ir.Node) Value {
			var out []Value
			for _, v := range this.c.statics.vals {
				if v.k == kObj && v.o.class == this.c {
					out = append(out, v)
				}
			}
			return it.phpArr(out...)
		}), true
	}
	return Value{}, false
}
