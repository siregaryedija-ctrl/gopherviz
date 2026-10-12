package irsim

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopherviz/internal/ir"
)

// phpSem: semantik PHP 8. Array adalah map berurutan yang disalin saat di-assign (copyFor),
// objek dipegang lewat handle. int 64-bit beralih ke float bila meluap.
type phpSem struct {
	globals map[string]Value
	consts  map[string]Value
}

func (s *phpSem) setup(it *interp) {
	s.globals = map[string]Value{}
	s.consts = map[string]Value{}
	it.setupPHP(s)
}

func (s *phpSem) lit(it *interp, l *ir.Lit) Value {
	switch l.Kind {
	case ir.LitInt:
		v := strings.ReplaceAll(strings.ToLower(l.Value), "_", "")
		base := 10
		switch {
		case strings.HasPrefix(v, "0x"):
			base, v = 16, v[2:]
		case strings.HasPrefix(v, "0b"):
			base, v = 2, v[2:]
		case strings.HasPrefix(v, "0o"):
			base, v = 8, v[2:]
		case len(v) > 1 && v[0] == '0':
			base, v = 8, v[1:]
		}
		n, err := strconv.ParseInt(v, base, 64)
		if err != nil {
			f, _ := strconv.ParseFloat(l.Value, 64)
			return floatV(f)
		}
		return intV(n)
	case ir.LitFloat:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(l.Value, "_", ""), 64)
		return floatV(f)
	case ir.LitString, ir.LitChar:
		return strV(l.Value)
	case ir.LitBool:
		return boolV(l.Value == "true")
	case ir.LitNull:
		return null()
	}
	return unknown("literal")
}

func (s *phpSem) truthy(v Value) bool {
	switch v.k {
	case kBool:
		return v.b
	case kInt:
		return v.i != 0
	case kFloat:
		return v.f != 0
	case kStr:
		return v.s != "" && v.s != "0"
	case kNull, kUndef:
		return false
	case kDict:
		return v.d.len() > 0
	}
	return true
}

// ---------- konversi ----------

func (s *phpSem) str(it *interp, v Value) string {
	switch v.k {
	case kStr:
		return v.s
	case kInt:
		return strconv.FormatInt(v.i, 10)
	case kFloat:
		return phpFloat(v.f)
	case kBool:
		if v.b {
			return "1"
		}
		return ""
	case kNull, kUndef:
		return ""
	case kDict:
		it.warn("Warning: Array to string conversion")
		return "Array"
	case kObj:
		if m := findMethod(v.o.class, "__toString"); m != nil {
			return s.str(it, it.callFunc(&Func{name: "__toString", decl: m, env: v.o.class.env, class: it.classOf[m]}, v, argList{}, m))
		}
		if instanceOf(v.o.class, "Throwable") {
			return it.exceptionText(v)
		}
		it.throw(nil, "Error", fmt.Sprintf("Object of class %s could not be converted to string", v.o.class.name))
	case kFunc:
		return "Closure"
	}
	return ""
}

func (s *phpSem) show(it *interp, v Value) string {
	switch v.k {
	case kStr:
		return strconv.Quote(v.s)
	case kBool:
		if v.b {
			return "true"
		}
		return "false"
	case kNull, kUndef:
		return "null"
	case kDict:
		var parts []string
		list := phpIsList(v.d)
		for i, k := range v.d.keys {
			if list {
				parts = append(parts, s.show(it, v.d.vals[i]))
			} else {
				parts = append(parts, s.show(it, k)+" => "+s.show(it, v.d.vals[i]))
			}
			if len(parts) > 20 {
				parts = append(parts, "…")
				break
			}
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case kObj:
		var parts []string
		for i, k := range v.o.fields.keys {
			parts = append(parts, k.s+": "+s.show(it, v.o.fields.vals[i]))
		}
		return v.o.class.name + " {" + strings.Join(parts, ", ") + "}"
	case kFunc:
		return "Closure"
	case kClass:
		return v.c.name
	case kUnknown:
		return "?"
	}
	return s.str(it, v)
}

// phpNumeric: string numerik penuh, awalan numerik ("5 apel"), atau bukan angka.
func phpNumeric(s string) (Value, int) {
	t := strings.TrimLeft(s, " \t\n\r\v\f")
	t2 := strings.TrimRight(t, " \t\n\r\v\f")
	if v, ok := phpParseNum(t2); ok {
		return v, 2 // numerik penuh
	}
	end := 0
	for end < len(t) && strings.IndexByte("+-0123456789.eE", t[end]) >= 0 {
		end++
	}
	for end > 0 {
		if v, ok := phpParseNum(t[:end]); ok {
			return v, 1 // awalan numerik
		}
		end--
	}
	return intV(0), 0
}

func phpParseNum(s string) (Value, bool) {
	if s == "" {
		return Value{}, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return intV(n), true
	}
	if strings.ContainsAny(s, "xXbBoO_") || strings.HasPrefix(strings.ToLower(s), "inf") || strings.Contains(strings.ToLower(s), "nan") {
		return Value{}, false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return floatV(f), true
	}
	return Value{}, false
}

// phpToNumber mengubah operand aritmetika menjadi int/float (dengan peringatan seperti PHP).
func (it *interp) phpToNumber(v Value, op string, other Value, at ir.Node) Value {
	switch v.k {
	case kInt, kFloat:
		return v
	case kBool:
		if v.b {
			return intV(1)
		}
		return intV(0)
	case kNull, kUndef:
		return intV(0)
	case kStr:
		n, kind := phpNumeric(v.s)
		switch kind {
		case 1:
			it.warn(`Warning: A non-numeric value encountered`)
		case 0:
			it.throw(at, "TypeError", fmt.Sprintf("Unsupported operand types: string %s %s", op, phpTypeName(other)))
		}
		return n
	case kDict:
		it.throw(at, "TypeError", fmt.Sprintf("Unsupported operand types: array %s %s", op, phpTypeName(other)))
	}
	it.throw(at, "TypeError", fmt.Sprintf("Unsupported operand types: %s %s %s", phpTypeName(v), op, phpTypeName(other)))
	return Value{}
}

func phpTypeName(v Value) string {
	switch v.k {
	case kInt:
		return "int"
	case kFloat:
		return "float"
	case kStr:
		return "string"
	case kBool:
		return "bool"
	case kNull, kUndef:
		return "null"
	case kDict:
		return "array"
	case kObj:
		return v.o.class.name
	}
	return "mixed"
}

// ---------- operator ----------

func (s *phpSem) binary(it *interp, op string, a, b Value, at ir.Node) Value {
	switch op {
	case ".":
		return strV(s.str(it, a) + s.str(it, b))
	case "===":
		return boolV(it.phpIdentical(a, b))
	case "!==":
		return boolV(!it.phpIdentical(a, b))
	case "==":
		return boolV(it.phpLooseCmp(a, b) == 0)
	case "!=", "<>":
		return boolV(it.phpLooseCmp(a, b) != 0)
	case "<", ">", "<=", ">=":
		c := it.phpLooseCmp(a, b)
		if c == 2 { // tidak bisa dibandingkan
			return boolV(false)
		}
		return boolV(map[string]bool{"<": c < 0, ">": c > 0, "<=": c <= 0, ">=": c >= 0}[op])
	case "<=>":
		return intV(int64(it.phpLooseCmp(a, b)))
	case "xor":
		return boolV(s.truthy(a) != s.truthy(b))
	case "+":
		if a.k == kDict && b.k == kDict { // gabungan array: key kiri menang
			d := it.cloneDict(a.d)
			for i, k := range b.d.keys {
				if !d.has(k) {
					d.set(k, b.d.vals[i])
				}
			}
			return Value{k: kDict, d: d}
		}
	}
	x, y := it.phpToNumber(a, op, b, at), it.phpToNumber(b, op, a, at)
	switch op {
	case "&", "|", "^", "<<", ">>":
		xi, yi := int64(toF(x)), int64(toF(y))
		switch op {
		case "&":
			return intV(xi & yi)
		case "|":
			return intV(xi | yi)
		case "^":
			return intV(xi ^ yi)
		case "<<":
			return intV(xi << uint(yi))
		}
		return intV(xi >> uint(yi))
	case "%":
		xi, yi := int64(toF(x)), int64(toF(y))
		if yi == 0 {
			it.throw(at, "DivisionByZeroError", "Modulo by zero")
		}
		return intV(xi % yi)
	case "/":
		if toF(y) == 0 {
			it.throw(at, "DivisionByZeroError", "Division by zero")
		}
		if x.k == kInt && y.k == kInt && x.i%y.i == 0 {
			return intV(x.i / y.i)
		}
		return floatV(toF(x) / toF(y))
	case "**":
		if x.k == kInt && y.k == kInt && y.i >= 0 {
			r := big1().Exp(big2(x.i), big2(y.i), nil)
			if r.IsInt64() {
				return intV(r.Int64())
			}
		}
		return floatV(math.Pow(toF(x), toF(y)))
	}
	if x.k == kInt && y.k == kInt {
		var r int64
		var ok bool
		switch op {
		case "+":
			r, ok = addOv(x.i, y.i)
		case "-":
			r, ok = subOv(x.i, y.i)
		case "*":
			r, ok = mulOv(x.i, y.i)
		default:
			it.structural("operator " + op + " is not simulated yet")
		}
		if ok {
			return intV(r)
		}
	}
	f, g := toF(x), toF(y)
	switch op {
	case "+":
		return floatV(f + g)
	case "-":
		return floatV(f - g)
	case "*":
		return floatV(f * g)
	}
	it.structural("operator " + op + " is not simulated yet")
	return Value{}
}

func addOv(a, b int64) (int64, bool) {
	c := a + b
	return c, (c > a) == (b > 0)
}

func subOv(a, b int64) (int64, bool) {
	c := a - b
	return c, (c < a) == (b > 0)
}

func mulOv(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	c := a * b
	return c, c/b == a && !(a == -1 && b == math.MinInt64) && !(b == -1 && a == math.MinInt64)
}

// phpIdentical: ===
func (it *interp) phpIdentical(a, b Value) bool {
	if a.k != b.k {
		return (a.k == kNull || a.k == kUndef) && (b.k == kNull || b.k == kUndef)
	}
	switch a.k {
	case kInt:
		return a.i == b.i
	case kFloat:
		return a.f == b.f
	case kStr:
		return a.s == b.s
	case kBool:
		return a.b == b.b
	case kDict:
		if a.d.len() != b.d.len() {
			return false
		}
		for i, k := range a.d.keys {
			if keyID(k) != keyID(b.d.keys[i]) || !it.phpIdentical(a.d.vals[i], b.d.vals[i]) {
				return false
			}
		}
		return true
	case kObj:
		return a.o == b.o
	}
	return jsStrictEq(a, b)
}

// phpLooseCmp membandingkan dengan aturan PHP 8: -1, 0, 1 (2 = tidak sebanding).
func (it *interp) phpLooseCmp(a, b Value) int {
	cmpF := func(x, y float64) int {
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
	switch {
	case (a.k == kNull || a.k == kUndef) && b.k == kStr:
		return strings.Compare("", b.s)
	case a.k == kStr && (b.k == kNull || b.k == kUndef):
		return strings.Compare(a.s, "")
	case a.k == kBool || b.k == kBool || a.k == kNull || b.k == kNull || a.k == kUndef || b.k == kUndef:
		x, y := it.sem.truthy(a), it.sem.truthy(b)
		switch {
		case x == y:
			return 0
		case !x:
			return -1
		}
		return 1
	case a.k == kStr && b.k == kStr:
		na, ka := phpNumeric(a.s)
		nb, kb := phpNumeric(b.s)
		if ka == 2 && kb == 2 {
			return cmpF(toF(na), toF(nb))
		}
		c := strings.Compare(a.s, b.s)
		switch {
		case c < 0:
			return -1
		case c > 0:
			return 1
		}
		return 0
	case isNum(a) && isNum(b):
		return cmpF(toF(a), toF(b))
	case isNum(a) && b.k == kStr:
		if n, k := phpNumeric(b.s); k == 2 {
			return cmpF(toF(a), toF(n))
		}
		return strings.Compare(it.sem.str(it, a), b.s)
	case a.k == kStr && isNum(b):
		return -it.phpLooseCmp(b, a)
	case a.k == kDict && b.k == kDict:
		if a.d.len() != b.d.len() {
			return cmpF(float64(a.d.len()), float64(b.d.len()))
		}
		for i, k := range a.d.keys {
			bv, ok := b.d.get(k)
			if !ok {
				return 2
			}
			if c := it.phpLooseCmp(a.d.vals[i], bv); c != 0 {
				return c
			}
		}
		return 0
	case a.k == kObj && b.k == kObj:
		if a.o == b.o {
			return 0
		}
		if a.o.class != b.o.class {
			return 2
		}
		for i, k := range a.o.fields.keys {
			bv, _ := b.o.fields.get(k)
			if c := it.phpLooseCmp(a.o.fields.vals[i], bv); c != 0 {
				return c
			}
		}
		return 0
	case a.k == kDict:
		return 1
	case b.k == kDict:
		return -1
	}
	return 2
}

func (s *phpSem) unary(it *interp, op string, v Value, at ir.Node) Value {
	switch op {
	case "!":
		return boolV(!s.truthy(v))
	case "-":
		n := it.phpToNumber(v, "*", intV(-1), at)
		if n.k == kInt && n.i != math.MinInt64 {
			return intV(-n.i)
		}
		return floatV(-toF(n))
	case "+":
		return it.phpToNumber(v, "*", intV(1), at)
	case "~":
		return intV(^int64(toF(it.phpToNumber(v, "~", intV(0), at))))
	}
	it.structural("operator " + op + " is not simulated yet")
	return Value{}
}

func (s *phpSem) caseEqual(it *interp, a, b Value) bool { return it.phpLooseCmp(a, b) == 0 }

func (s *phpSem) zero(typ string) Value { return null() }

func (s *phpSem) convert(it *interp, v Value, typ string) Value {
	switch strings.ToLower(strings.TrimPrefix(typ, "?")) {
	case "int", "integer":
		switch v.k {
		case kFloat:
			if math.IsNaN(v.f) || math.IsInf(v.f, 0) {
				return intV(0)
			}
			return intV(int64(v.f))
		case kStr:
			n, _ := phpNumeric(v.s)
			return intV(int64(toF(n)))
		case kBool:
			if v.b {
				return intV(1)
			}
			return intV(0)
		case kNull:
			return intV(0)
		case kDict:
			if v.d.len() > 0 {
				return intV(1)
			}
			return intV(0)
		}
	case "float", "double":
		switch v.k {
		case kStr:
			n, _ := phpNumeric(v.s)
			return floatV(toF(n))
		case kNull:
			return floatV(0)
		}
		if isNum(v) || v.k == kBool {
			return floatV(toF(v))
		}
	case "string":
		return strV(s.str(it, v))
	case "bool", "boolean":
		return boolV(s.truthy(v))
	case "array":
		if v.k == kDict {
			return v
		}
		if v.k == kNull {
			return it.dictV("array")
		}
		d := it.newDict("array")
		if v.k == kObj {
			for i, k := range v.o.fields.keys {
				d.set(k, v.o.fields.vals[i])
			}
		} else {
			d.set(intV(0), v)
		}
		return Value{k: kDict, d: d}
	}
	return v
}

func (s *phpSem) newError(it *interp, class, msg string) Value {
	c := it.findClass(class, nil)
	if c == nil {
		c = it.findClass("Error", nil)
	}
	return it.construct(c, []Value{strV(msg)}, nil)
}

func (s *phpSem) iterate(it *interp, x Value, fe *ir.ForEach) ([]kv, bool) {
	switch x.k {
	case kDict:
		out := make([]kv, len(x.d.keys))
		for i, k := range x.d.keys {
			out[i] = kv{k: k, v: x.d.vals[i]}
		}
		return out, true
	case kObj: // properti publik
		out := make([]kv, len(x.o.fields.keys))
		for i, k := range x.o.fields.keys {
			out[i] = kv{k: k, v: x.o.fields.vals[i]}
		}
		return out, true
	case kNull:
		it.warn("Warning: foreach() argument must be of type array|object, null given")
		return nil, true
	}
	return nil, false
}

// ---------- tampilan khusus ----------

// phpVarDump meniru var_dump.
func (it *interp) phpVarDump(v Value, ind int) string {
	pad := strings.Repeat(" ", ind)
	switch v.k {
	case kInt:
		return pad + "int(" + strconv.FormatInt(v.i, 10) + ")\n"
	case kFloat:
		return pad + "float(" + phpFloatRepr(v.f) + ")\n"
	case kStr:
		return pad + "string(" + strconv.Itoa(len(v.s)) + ") \"" + v.s + "\"\n"
	case kBool:
		return pad + "bool(" + map[bool]string{true: "true", false: "false"}[v.b] + ")\n"
	case kNull, kUndef:
		return pad + "NULL\n"
	case kDict:
		var b strings.Builder
		fmt.Fprintf(&b, "%sarray(%d) {\n", pad, v.d.len())
		for i, k := range v.d.keys {
			if k.k == kInt {
				fmt.Fprintf(&b, "%s  [%d]=>\n", pad, k.i)
			} else {
				fmt.Fprintf(&b, "%s  [\"%s\"]=>\n", pad, k.s)
			}
			b.WriteString(it.phpVarDump(v.d.vals[i], ind+2))
		}
		b.WriteString(pad + "}\n")
		return b.String()
	case kObj:
		var b strings.Builder
		fmt.Fprintf(&b, "%sobject(%s)#%d (%d) {\n", pad, v.o.class.name, v.o.id, v.o.fields.len())
		for i, k := range v.o.fields.keys {
			fmt.Fprintf(&b, "%s  [\"%s\"]=>\n", pad, k.s)
			b.WriteString(it.phpVarDump(v.o.fields.vals[i], ind+2))
		}
		b.WriteString(pad + "}\n")
		return b.String()
	}
	return pad + "NULL\n"
}

// phpPrintR meniru print_r.
func (it *interp) phpPrintR(v Value, ind int) string {
	switch v.k {
	case kDict, kObj:
		d := v.d
		head := "Array"
		if v.k == kObj {
			d = v.o.fields
			head = v.o.class.name + " Object"
		}
		pad := strings.Repeat(" ", ind)
		var b strings.Builder
		b.WriteString(head + "\n" + pad + "(\n")
		for i, k := range d.keys {
			key := it.sem.str(it, k)
			b.WriteString(pad + "    [" + key + "] => ")
			val := d.vals[i]
			if val.k == kDict || val.k == kObj {
				b.WriteString(it.phpPrintR(val, ind+8))
				b.WriteString("\n")
			} else {
				b.WriteString(it.sem.str(it, val) + "\n")
			}
		}
		b.WriteString(pad + ")\n")
		return b.String()
	}
	return it.sem.str(it, v)
}

// phpVarExport meniru var_export.
func (it *interp) phpVarExport(v Value, ind int) string {
	pad := strings.Repeat(" ", ind)
	switch v.k {
	case kStr:
		return "'" + strings.ReplaceAll(strings.ReplaceAll(v.s, `\`, `\\`), "'", `\'`) + "'"
	case kBool:
		return map[bool]string{true: "true", false: "false"}[v.b]
	case kNull, kUndef:
		return "NULL"
	case kFloat:
		s := phpFloatRepr(v.f)
		if !strings.ContainsAny(s, ".EN") {
			s += ".0"
		}
		return s
	case kDict:
		var b strings.Builder
		b.WriteString("array (\n")
		for i, k := range v.d.keys {
			b.WriteString(pad + "  " + it.phpVarExport(k, 0) + " => ")
			val := v.d.vals[i]
			if val.k == kDict {
				b.WriteString("\n" + pad + "  ")
			}
			b.WriteString(it.phpVarExport(val, ind+2) + ",\n")
		}
		b.WriteString(pad + ")")
		return b.String()
	}
	return it.sem.str(it, v)
}

func (it *interp) phpJSON(v Value, flags int64, ind int) string {
	pretty := flags&128 != 0
	unescSlash := flags&64 != 0
	unescUni := flags&256 != 0
	nl, pad, pad2, colon := "", "", "", ":"
	if pretty {
		nl, pad, pad2, colon = "\n", strings.Repeat("    ", ind+1), strings.Repeat("    ", ind), ": "
	}
	switch v.k {
	case kNull, kUndef:
		return "null"
	case kBool:
		return map[bool]string{true: "true", false: "false"}[v.b]
	case kInt:
		return strconv.FormatInt(v.i, 10)
	case kFloat:
		if isIntegral(v.f) && math.Abs(v.f) < 1e15 {
			return strconv.FormatFloat(v.f, 'f', -1, 64)
		}
		return strings.ToLower(phpFloatRepr(v.f))
	case kStr:
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range v.s {
			switch {
			case r == '"':
				b.WriteString(`\"`)
			case r == '\\':
				b.WriteString(`\\`)
			case r == '/' && !unescSlash:
				b.WriteString(`\/`)
			case r == '\n':
				b.WriteString(`\n`)
			case r == '\t':
				b.WriteString(`\t`)
			case r == '\r':
				b.WriteString(`\r`)
			case r < 0x20:
				fmt.Fprintf(&b, `\u%04x`, r)
			case r > 0x7f && !unescUni:
				if r > 0xffff {
					r1, r2 := utf16Pair(r)
					fmt.Fprintf(&b, `\u%04x\u%04x`, r1, r2)
				} else {
					fmt.Fprintf(&b, `\u%04x`, r)
				}
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		return b.String()
	case kDict, kObj:
		d := v.d
		isList := v.k == kDict && phpIsList(d)
		if v.k == kObj {
			d = v.o.fields
		}
		if d.len() == 0 {
			if isList {
				return "[]"
			}
			return "{}"
		}
		var parts []string
		for i, k := range d.keys {
			val := it.phpJSON(d.vals[i], flags, ind+1)
			if isList {
				parts = append(parts, pad+val)
			} else {
				parts = append(parts, pad+it.phpJSON(strV(it.sem.str(it, k)), flags, ind+1)+colon+val)
			}
		}
		if isList {
			return "[" + nl + strings.Join(parts, ","+nl) + nl + pad2 + "]"
		}
		return "{" + nl + strings.Join(parts, ","+nl) + nl + pad2 + "}"
	}
	return "null"
}

func utf16Pair(r rune) (rune, rune) {
	r -= 0x10000
	return 0xd800 + (r>>10)&0x3ff, 0xdc00 + r&0x3ff
}

func (it *interp) phpJSONDecode(s string, assoc bool) Value {
	var raw any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return null()
	}
	var conv func(x any) Value
	conv = func(x any) Value {
		switch t := x.(type) {
		case nil:
			return null()
		case bool:
			return boolV(t)
		case json.Number:
			if n, err := t.Int64(); err == nil {
				return intV(n)
			}
			f, _ := t.Float64()
			return floatV(f)
		case string:
			return strV(t)
		case []any:
			d := it.newDict("array")
			for _, e := range t {
				d.set(intV(d.next), conv(e))
			}
			return Value{k: kDict, d: d}
		case map[string]any:
			d := it.newDict("array")
			for _, k := range jsonKeyOrder(s, t) {
				d.set(it.dictKey(strV(k)), conv(t[k]))
			}
			if !assoc {
				it.ids++
				std := it.findClass("stdClass", nil)
				o := &Object{class: std, fields: d, id: it.ids}
				return Value{k: kObj, o: o}
			}
			return Value{k: kDict, d: d}
		}
		return null()
	}
	return conv(raw)
}

// phpRound: round() PHP — setengah menjauhi nol dengan pre-rounding 15 digit.
func phpRound(f float64, places int) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return f
	}
	pow := math.Pow(10, float64(abs(places)))
	var tmp float64
	if places >= 0 {
		tmp = f * pow
	} else {
		tmp = f / pow
	}
	pre, _ := strconv.ParseFloat(strconv.FormatFloat(tmp, 'g', 15, 64), 64)
	r := math.Round(pre) // setengah menjauhi nol
	if places >= 0 {
		r /= pow
	} else {
		r *= pow
	}
	return r
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// phpSprintf meniru sprintf PHP: %[argnum$][flags][width][.precision]specifier.
func (it *interp) phpSprintf(f string, args []Value, at ir.Node) string {
	var b strings.Builder
	ai := 0
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			b.WriteByte(f[i])
			continue
		}
		i++
		if i >= len(f) {
			break
		}
		if f[i] == '%' {
			b.WriteByte('%')
			continue
		}
		// argnum$
		argnum := -1
		if j := strings.IndexByte(f[i:], '$'); j > 0 {
			if n, err := strconv.Atoi(f[i : i+j]); err == nil {
				argnum = n - 1
				i += j + 1
			}
		}
		pad, left, plus := byte(' '), false, false
		for i < len(f) {
			switch f[i] {
			case '-':
				left = true
			case '+':
				plus = true
			case '0':
				pad = '0'
			case ' ':
				pad = ' '
			case '\'':
				i++
				pad = f[i]
			default:
				goto width
			}
			i++
		}
	width:
		w := 0
		for i < len(f) && f[i] >= '0' && f[i] <= '9' {
			w = w*10 + int(f[i]-'0')
			i++
		}
		prec := -1
		if i < len(f) && f[i] == '.' {
			i++
			prec = 0
			for i < len(f) && f[i] >= '0' && f[i] <= '9' {
				prec = prec*10 + int(f[i]-'0')
				i++
			}
		}
		if i >= len(f) {
			break
		}
		verb := f[i]
		var v Value
		if argnum >= 0 {
			v = arg(args, argnum)
		} else {
			if ai >= len(args) {
				it.throw(at, "ArgumentCountError", fmt.Sprintf("%d arguments are required, %d given", ai+2, len(args)+1))
			}
			v = args[ai]
			ai++
		}
		var s string
		num := func() Value { return it.phpToNumber(v, "+", intV(0), at) }
		switch verb {
		case 'd', 'i':
			n := int64(toF(num()))
			s = strconv.FormatInt(n, 10)
			if plus && n >= 0 {
				s = "+" + s
			}
		case 'u':
			s = strconv.FormatUint(uint64(int64(toF(num()))), 10)
		case 'f', 'F':
			p := prec
			if p < 0 {
				p = 6
			}
			x := toF(num())
			s = strconv.FormatFloat(x, 'f', p, 64)
			if plus && x >= 0 {
				s = "+" + s
			}
		case 'e', 'E':
			p := prec
			if p < 0 {
				p = 6
			}
			s = strconv.FormatFloat(toF(num()), 'e', p, 64)
			mant, exp, _ := strings.Cut(s, "e")
			e, _ := strconv.Atoi(exp)
			sign := "+"
			if e < 0 {
				sign, e = "-", -e
			}
			s = mant + "e" + sign + strconv.Itoa(e)
			if verb == 'E' {
				s = strings.ToUpper(s)
			}
		case 'g', 'G':
			p := prec
			if p < 0 {
				p = 6
			}
			s = strconv.FormatFloat(toF(num()), 'g', p, 64)
		case 's':
			s = it.sem.str(it, v)
			if prec >= 0 && prec < len(s) {
				s = s[:prec]
			}
		case 'x':
			s = strconv.FormatInt(int64(toF(num())), 16)
		case 'X':
			s = strings.ToUpper(strconv.FormatInt(int64(toF(num())), 16))
		case 'o':
			s = strconv.FormatInt(int64(toF(num())), 8)
		case 'b':
			s = strconv.FormatUint(uint64(int64(toF(num()))), 2)
		case 'c':
			s = string(rune(int(toF(num()))))
		default:
			it.throw(at, "ValueError", fmt.Sprintf("Unknown format specifier \"%c\"", verb))
		}
		if len(s) < w {
			fill := strings.Repeat(string(pad), w-len(s))
			switch {
			case left && pad == '0':
				s += strings.Repeat(" ", w-len(s))
			case left:
				s += fill
			case pad == '0' && (strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+")):
				s = s[:1] + fill + s[1:]
			default:
				s = fill + s
			}
		}
		b.WriteString(s)
	}
	return b.String()
}

func phpNumberFormat(f float64, dec int, dp, ts string) string {
	r := phpRound(f, dec)
	s := strconv.FormatFloat(math.Abs(r), 'f', dec, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	out := groupThousands(intPart, ts)
	if dec > 0 {
		out += dp + frac
	}
	if r < 0 && strings.Trim(out, "0.,") != "" {
		out = "-" + out
	}
	return out
}

func ucwords(s string) string {
	rs := []rune(s)
	up := true
	for i, r := range rs {
		if up && unicode.IsLetter(r) {
			rs[i] = unicode.ToUpper(r)
		}
		up = r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
	}
	return string(rs)
}

// phpSort mengurutkan array PHP; keepKeys untuk asort/ksort; byKey untuk ksort.
func (it *interp) phpSort(d *Dict, less func(ak, av, bk, bv Value) bool, keepKeys bool) {
	idx := make([]int, len(d.keys))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return less(d.keys[idx[a]], d.vals[idx[a]], d.keys[idx[b]], d.vals[idx[b]]) })
	keys, vals := make([]Value, len(idx)), make([]Value, len(idx))
	for i, j := range idx {
		keys[i], vals[i] = d.keys[j], d.vals[j]
	}
	if !keepKeys {
		for i := range keys {
			keys[i] = intV(int64(i))
		}
		d.next = int64(len(keys))
	}
	d.keys, d.vals = keys, vals
	d.reindex()
}
