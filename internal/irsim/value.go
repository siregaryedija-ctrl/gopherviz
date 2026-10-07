// Package irsim mensimulasikan program bahasa selain Go (JavaScript, TypeScript, Python,
// PHP, C++, C, Java) dari IR (package ir) dengan aman: kode user tidak pernah dijalankan.
// Hasilnya Trace yang sama bentuknya dengan simulator Go (package sim), jadi semua renderer
// di panel bisa langsung memakainya. Perbedaan semantik antar bahasa (pembagian bilangan
// bulat, truthiness, cara mencetak, salinan vs referensi) ada di file per bahasa.
package irsim

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

type kind uint8

const (
	kUnknown kind = iota // nilai yang tidak bisa dihitung (input user, library yang belum didukung)
	kUndef               // undefined (JavaScript)
	kNull                // null / None / nullptr / NULL
	kBool
	kInt   // bilangan bulat 64-bit; t = tipe statis C/Java ("int", "char", "long", ...)
	kBig   // bilangan bulat besar Python
	kFloat // t = "float" untuk float 32-bit C/Java
	kStr
	kList  // array/list/tuple/vector/ArrayList
	kDict  // objek JS, dict Python, array PHP, map C++/Java
	kSet   // set Python / Set JS (Dict tanpa nilai)
	kObj   // instance kelas
	kFunc  // fungsi / method / lambda
	kClass // kelas (bisa dipanggil sebagai konstruktor)
	kPtr   // pointer C/C++
)

// Value adalah nilai dinamis. Koleksi dan objek dipegang lewat pointer: bahasa dengan
// semantik nilai (C struct, vector C++, array PHP) menyalinnya secara eksplisit (copyVal).
type Value struct {
	k   kind
	b   bool
	i   int64
	f   float64
	s   string
	big *big.Int
	l   *List
	d   *Dict
	o   *Object
	fn  *Func
	c   *Class
	p   *Ptr
	t   string // tipe statis (C/C++/Java) atau penanda khusus ("tuple" tidak di sini, lihat List.kind)
	why string // alasan untuk kUnknown
}

// List adalah urutan nilai.
type List struct {
	elems []Value
	kind  string // "array" (JS/Java/C), "list", "tuple" (Python), "vector" (C++), "ArrayList" (Java)
	elem  string // tipe elemen statis (C/C++/Java), untuk nilai nol
	id    int
}

// Dict adalah map berurutan (urutan sisip, kecuali kind "map"/"TreeMap" yang terurut key).
type Dict struct {
	keys []Value
	vals []Value
	idx  map[string]int // keyID → posisi
	kind string         // "object" (JS), "dict" (Python), "array" (PHP), "map" (C++), "HashMap", "Map" (JS), "set"
	next int64          // PHP: indeks otomatis berikutnya
	id   int
}

// Object adalah instance kelas.
type Object struct {
	class  *Class
	fields *Dict
	id     int
}

// Ptr menunjuk sel variabel atau elemen array (C/C++).
type Ptr struct {
	cell  *Cell
	list  *List
	idx   int
	obj   *Object // pointer ke field objek/struct (&p.x)
	field string
	name  string // nama variabel yang ditunjuk (untuk tampilan)
}

// Func adalah fungsi user (decl) atau bawaan (builtin).
type Func struct {
	name    string
	decl    *Fn
	env     *Env
	this    *Value // this yang terikat (method terikat / arrow function)
	class   *Class // kelas pemilik (untuk super)
	builtin func(it *interp, this Value, args []Value, at nodeRef) Value
	static  bool
}

// Class adalah kelas user atau bawaan.
type Class struct {
	name    string
	super   *Class
	methods map[string]*Fn
	getters map[string]*Fn
	setters map[string]*Fn
	statics *Dict
	fields  []fieldInit // inisialisasi field instance (urut deklarasi)
	env     *Env
	native  string // kelas bawaan: "Error", "Map", "Set", "ArrayList", ...
	kind    string // "class", "struct", "enum", "interface"

	overloads map[string][]*Fn // C++/Java: method/konstruktor dengan nama sama
	ctor      func(it *interp, args []Value, at nodeRef) Value             // bawaan: new X(args)
	ctorT     func(it *interp, typ string, args []Value, at nodeRef) Value // bawaan bertipe: vector<int>(5, 0)
	init      func(it *interp, this Value, args []Value, at nodeRef)       // bawaan sebagai kelas induk (Error)
}

type fieldInit struct {
	name string
	typ  string
	init func(it *interp, this Value) Value
}

// ---------- konstruktor ----------

func undef() Value            { return Value{k: kUndef} }
func null() Value             { return Value{k: kNull} }
func boolV(b bool) Value      { return Value{k: kBool, b: b} }
func intV(i int64) Value      { return Value{k: kInt, i: i} }
func typedInt(i int64, t string) Value { return Value{k: kInt, i: i, t: t} }
func floatV(f float64) Value  { return Value{k: kFloat, f: f} }
func strV(s string) Value     { return Value{k: kStr, s: s} }
func unknown(why string) Value { return Value{k: kUnknown, why: why} }

func (it *interp) newList(kind string, elems []Value) Value {
	it.ids++
	return Value{k: kList, l: &List{elems: elems, kind: kind, id: it.ids}}
}

func (it *interp) newDict(kind string) *Dict {
	it.ids++
	return &Dict{idx: map[string]int{}, kind: kind, id: it.ids}
}

func (it *interp) dictV(kind string) Value { return Value{k: kDict, d: it.newDict(kind)} }

// ---------- Dict ----------

// keyID membuat kunci internal yang membedakan tipe ("1" vs 1), kecuali bila bahasanya
// menyamakannya (PHP: "1" dan 1 adalah key yang sama; JS: key objek selalu string).
func keyID(v Value) string {
	switch v.k {
	case kInt:
		return "i" + strconv.FormatInt(v.i, 10)
	case kBig:
		return "i" + v.big.String()
	case kFloat:
		if v.f == math.Trunc(v.f) && math.Abs(v.f) < 1e18 {
			return "i" + strconv.FormatInt(int64(v.f), 10) // 1.0 dan 1 adalah key yang sama (Python)
		}
		return "f" + strconv.FormatFloat(v.f, 'g', -1, 64)
	case kStr:
		return "s" + v.s
	case kBool:
		if v.b {
			return "i1" // Python: True == 1
		}
		return "i0"
	case kNull:
		return "n"
	case kUndef:
		return "u"
	case kList:
		var b strings.Builder
		b.WriteString("t(")
		for _, e := range v.l.elems {
			b.WriteString(keyID(e))
			b.WriteByte(',')
		}
		b.WriteByte(')')
		return b.String()
	case kObj:
		return "o" + strconv.Itoa(v.o.id)
	}
	return "?" + strconv.Itoa(int(v.k))
}

func (d *Dict) get(k Value) (Value, bool) {
	if i, ok := d.idx[keyID(k)]; ok {
		return d.vals[i], true
	}
	return Value{}, false
}

func (d *Dict) has(k Value) bool {
	_, ok := d.idx[keyID(k)]
	return ok
}

func (d *Dict) set(k, v Value) {
	id := keyID(k)
	if i, ok := d.idx[id]; ok {
		d.vals[i] = v
		return
	}
	if d.kind == "map" || d.kind == "TreeMap" || d.kind == "set-sorted" {
		// map C++ / TreeMap Java: terurut menurut key
		pos := len(d.keys)
		for i, x := range d.keys {
			if less(k, x) {
				pos = i
				break
			}
		}
		d.keys = append(d.keys, Value{})
		d.vals = append(d.vals, Value{})
		copy(d.keys[pos+1:], d.keys[pos:])
		copy(d.vals[pos+1:], d.vals[pos:])
		d.keys[pos], d.vals[pos] = k, v
		d.reindex()
	} else {
		d.idx[id] = len(d.keys)
		d.keys = append(d.keys, k)
		d.vals = append(d.vals, v)
	}
	if k.k == kInt && k.i >= d.next {
		d.next = k.i + 1
	}
}

func (d *Dict) del(k Value) bool {
	i, ok := d.idx[keyID(k)]
	if !ok {
		return false
	}
	d.keys = append(d.keys[:i:i], d.keys[i+1:]...)
	d.vals = append(d.vals[:i:i], d.vals[i+1:]...)
	d.reindex()
	return true
}

func (d *Dict) reindex() {
	d.idx = make(map[string]int, len(d.keys))
	for i, k := range d.keys {
		d.idx[keyID(k)] = i
	}
}

func (d *Dict) len() int { return len(d.keys) }

// less membandingkan dua key untuk map terurut (angka, lalu string).
func less(a, b Value) bool {
	if isNum(a) && isNum(b) {
		return toF(a) < toF(b)
	}
	return a.s < b.s
}

// ---------- angka ----------

func isNum(v Value) bool { return v.k == kInt || v.k == kFloat || v.k == kBig }

func toF(v Value) float64 {
	switch v.k {
	case kInt:
		return float64(v.i)
	case kFloat:
		return v.f
	case kBig:
		f, _ := new(big.Float).SetInt(v.big).Float64()
		return f
	case kBool:
		if v.b {
			return 1
		}
	}
	return 0
}

func isIntegral(f float64) bool { return f == math.Trunc(f) && !math.IsInf(f, 0) }

// digits mengembalikan digit terpendek f (tanpa tanda) dan eksponen desimal n:
// f = 0.d1d2d3... × 10^n.
func digits(f float64, bits int) (string, int) {
	s := strconv.FormatFloat(math.Abs(f), 'e', -1, bits)
	mant, exp, _ := strings.Cut(s, "e")
	e, _ := strconv.Atoi(exp)
	return strings.ReplaceAll(mant, ".", ""), e + 1
}

// jsNumber mengikuti Number.prototype.toString di JavaScript.
func jsNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	sign := ""
	if f < 0 {
		sign = "-"
	}
	d, n := digits(f, 64)
	k := len(d)
	switch {
	case k <= n && n <= 21:
		return sign + d + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return sign + d[:n] + "." + d[n:]
	case -6 < n && n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + d
	}
	e := n - 1
	es := "+" + strconv.Itoa(e)
	if e < 0 {
		es = strconv.Itoa(e)
	}
	if k == 1 {
		return sign + d + "e" + es
	}
	return sign + d[:1] + "." + d[1:] + "e" + es
}

// pyFloat mengikuti repr(float) di Python.
func pyFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case f == 0:
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	sign := ""
	if f < 0 {
		sign = "-"
	}
	d, n := digits(f, 64)
	k := len(d)
	if n > 16 || n < -3 {
		e := n - 1
		es := strconv.Itoa(e)
		if e >= 0 {
			es = "+" + es
		}
		if len(es) == 2 {
			es = es[:1] + "0" + es[1:]
		}
		if k == 1 {
			return sign + d + "e" + es
		}
		return sign + d[:1] + "." + d[1:] + "e" + es
	}
	switch {
	case n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + d
	case k <= n:
		return sign + d + strings.Repeat("0", n-k) + ".0"
	}
	return sign + d[:n] + "." + d[n:]
}

// phpFloat mengikuti echo float di PHP (precision = 14).
func phpFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NAN"
	case math.IsInf(f, 1):
		return "INF"
	case math.IsInf(f, -1):
		return "-INF"
	case f == 0:
		if math.Signbit(f) {
			return "-0"
		}
		return "0"
	}
	s := strconv.FormatFloat(f, 'e', 13, 64) // 14 digit signifikan
	mant, exp, _ := strings.Cut(s, "e")
	e, _ := strconv.Atoi(exp)
	sign := ""
	if strings.HasPrefix(mant, "-") {
		sign, mant = "-", mant[1:]
	}
	d := strings.TrimRight(strings.ReplaceAll(mant, ".", ""), "0")
	if d == "" {
		d = "0"
	}
	n := e + 1
	if e < -4 || e >= 14 {
		m := d[:1]
		if len(d) > 1 {
			m += "." + d[1:]
		} else {
			m += ".0"
		}
		es := strconv.Itoa(e)
		if e >= 0 {
			es = "+" + es
		}
		return sign + m + "E" + es
	}
	switch {
	case n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + d
	case len(d) <= n:
		return sign + d + strings.Repeat("0", n-len(d))
	}
	return sign + d[:n] + "." + d[n:]
}

// phpFloatRepr mengikuti var_dump / json_encode (serialize_precision = -1).
func phpFloatRepr(f float64) string {
	if isIntegral(f) && math.Abs(f) < 1e15 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	s := pyFloat(f)
	s = strings.Replace(s, "e+", "E+", 1)
	s = strings.Replace(s, "e-", "E-", 1)
	return strings.Replace(strings.Replace(s, "E+0", "E+", 1), "E-0", "E-", 1)
}

// javaDouble mengikuti Double.toString (bits = 32 untuk Float.toString).
func javaDouble(f float64, bits int) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	sign := ""
	if f < 0 {
		sign = "-"
	}
	d, n := digits(f, bits)
	a := math.Abs(f)
	if a >= 1e-3 && a < 1e7 {
		switch {
		case n <= 0:
			return sign + "0." + strings.Repeat("0", -n) + d
		case len(d) <= n:
			return sign + d + strings.Repeat("0", n-len(d)) + ".0"
		}
		return sign + d[:n] + "." + d[n:]
	}
	m := d[:1] + "."
	if len(d) > 1 {
		m += d[1:]
	} else {
		m += "0"
	}
	return sign + m + "E" + strconv.Itoa(n-1)
}

// cDouble mengikuti printf("%g") / cout << double (presisi 6).
func cDouble(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	return strconv.FormatFloat(f, 'g', 6, 64)
}
