package sim

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"strings"
	"unicode/utf8"
)

type kind uint8

const (
	kUnknown kind = iota // nilai yang tidak bisa diketahui tanpa menjalankan program sungguhan
	kNil
	kBool
	kInt
	kFloat
	kString
	kSlice
	kArray
	kMap
	kFunc
	kChan
	kTuple
	kStruct
	kPtr
	kError
)

// value adalah nilai Go di dalam simulator. Slice, map, channel, dan pointer adalah
// referensi (berbagi data seperti di Go); array dan struct adalah nilai (disalin).
type value struct {
	k     kind
	b     bool
	i     int64
	f     float64
	s     string    // string, atau pesan untuk kError
	sl    *sliceVal // nil = slice nil
	arr   []value
	m     *mapVal // nil = map nil
	fn    *funcVal
	ch    *chanVal
	tup   []value
	st    *structVal
	ptr   *value // kPtr: sel yang ditunjuk; nil = pointer nil
	wrap  *value // kError: error yang dibungkus lewat %w
	eid   int    // kError: identitas, untuk == dan errors.Is
	tname string // nama tipe bernama (mis. "Person"), untuk method dan String()
	why   string // alasan untuk kUnknown
}

type structVal struct {
	names  []string
	fields []value
}

func (s *structVal) index(name string) int {
	for i, n := range s.names {
		if n == name {
			return i
		}
	}
	return -1
}

type sliceVal struct {
	data          *[]value
	off, len, cap int
}

type mapVal struct {
	keys []value
	vals map[string]value
}

type funcVal struct {
	name    string
	decl    *ast.FuncDecl
	lit     *ast.FuncLit
	env     *env
	recv    *value // receiver untuk method
	builtin func(args []value) []value
}

type chanVal struct {
	id       int
	name     string // nama variabel pertama yang memegang channel ini (untuk tampilan)
	buf      []value
	cap      int
	closed   bool
	sendq    []*waiter // goroutine yang menunggu untuk mengirim
	recvq    []*waiter // goroutine yang menunggu untuk menerima
	inflight []*waiter // nilai sudah diserahkan ke penerima yang belum sempat jalan
	timer    int       // time.After: 1 = menunggu bunyi, 2 = sudah berbunyi
}

func unknown(why string) value { return value{k: kUnknown, why: why} }
func intV(i int64) value       { return value{k: kInt, i: i} }
func floatV(f float64) value   { return value{k: kFloat, f: f} }
func strV(s string) value      { return value{k: kString, s: s} }
func boolV(b bool) value       { return value{k: kBool, b: b} }

func sliceOf(elems []value) value {
	data := elems
	return value{k: kSlice, sl: &sliceVal{data: &data, len: len(elems), cap: len(elems)}}
}

func (s *sliceVal) at(i int) *value { return &(*s.data)[s.off+i] }

func (s *sliceVal) elems() []value {
	if s == nil {
		return nil
	}
	return (*s.data)[s.off : s.off+s.len]
}

func newMap() *mapVal { return &mapVal{vals: map[string]value{}} }

func mapKey(v value) string { return fmt.Sprintf("%d:%v", v.k, native(v)) }

func (m *mapVal) get(k value) (value, bool) {
	if m == nil {
		return value{}, false
	}
	v, ok := m.vals[mapKey(k)]
	return v, ok
}

func (m *mapVal) set(k, v value) {
	key := mapKey(k)
	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[key] = v
}

func (m *mapVal) del(k value) {
	key := mapKey(k)
	if _, ok := m.vals[key]; !ok {
		return
	}
	delete(m.vals, key)
	for i, kk := range m.keys {
		if mapKey(kk) == key {
			m.keys = append(m.keys[:i:i], m.keys[i+1:]...)
			break
		}
	}
}

// copyVal meniru semantik assignment Go: array dan struct disalin, yang lain berbagi.
func copyVal(v value) value {
	switch v.k {
	case kArray:
		arr := make([]value, len(v.arr))
		for i, e := range v.arr {
			arr[i] = copyVal(e)
		}
		v.arr = arr
	case kStruct:
		if v.st != nil {
			fields := make([]value, len(v.st.fields))
			for i, e := range v.st.fields {
				fields[i] = copyVal(e)
			}
			v.st = &structVal{names: v.st.names, fields: fields}
		}
	}
	return v
}

// namedOf mengembalikan nama tipe bernama buatan user (bukan error/interface), atau "".
func namedOf(t types.Type) string {
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return ""
	}
	if _, isIface := n.Underlying().(*types.Interface); isIface {
		return ""
	}
	return n.Obj().Name()
}

// ---------- tipe ----------

func zeroOf(t types.Type) value {
	if t == nil {
		return unknown("tipe tidak diketahui")
	}
	v := zeroUnder(t)
	if n := namedOf(t); n != "" && v.k != kUnknown {
		v.tname = n
	}
	return v
}

func zeroUnder(t types.Type) value {
	switch u := t.Underlying().(type) {
	case *types.Struct:
		st := &structVal{}
		for i := 0; i < u.NumFields(); i++ {
			st.names = append(st.names, u.Field(i).Name())
			st.fields = append(st.fields, zeroOf(u.Field(i).Type()))
		}
		return value{k: kStruct, st: st}
	case *types.Pointer:
		return value{k: kPtr}
	case *types.Interface:
		return value{k: kNil}
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return boolV(false)
		case u.Info()&types.IsInteger != 0:
			return intV(0)
		case u.Info()&types.IsFloat != 0:
			return floatV(0)
		case u.Info()&types.IsString != 0:
			return strV("")
		}
	case *types.Slice:
		return value{k: kSlice}
	case *types.Array:
		arr := make([]value, u.Len())
		for i := range arr {
			arr[i] = zeroOf(u.Elem())
		}
		return value{k: kArray, arr: arr}
	case *types.Map:
		return value{k: kMap}
	case *types.Signature:
		return value{k: kFunc}
	case *types.Chan:
		return value{k: kChan}
	}
	return unknown("nilai bertipe " + t.String() + " belum disimulasikan")
}

// coerce menyesuaikan v dengan tipe statis t (mis. konstanta 3 di konteks float64)
// dan menandai nilai dengan nama tipe bernamanya (untuk method dan String()).
func coerce(v value, t types.Type) value {
	if t == nil {
		return v
	}
	v = coerceUnder(v, t)
	if n := namedOf(t); n != "" && v.tname == "" && v.k != kUnknown && v.k != kNil {
		v.tname = n
	}
	return v
}

func coerceUnder(v value, t types.Type) value {
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		if v.k == kNil {
			return value{k: kPtr}
		}
	case *types.Basic:
		info := u.Info()
		switch {
		case info&types.IsFloat != 0 && v.k == kInt:
			return floatV(float64(v.i))
		case info&types.IsInteger != 0 && v.k == kFloat:
			return intV(int64(v.f))
		case info&types.IsInteger != 0 && v.k == kInt:
			return intV(wrapInt(v.i, u.Kind()))
		}
	case *types.Slice:
		if v.k == kNil {
			return value{k: kSlice}
		}
	case *types.Map:
		if v.k == kNil {
			return value{k: kMap}
		}
	}
	return v
}

// wrapInt meniru overflow bilangan bulat berukuran kecil (byte, int8, dst).
func wrapInt(i int64, k types.BasicKind) int64 {
	switch k {
	case types.Int8:
		return int64(int8(i))
	case types.Int16:
		return int64(int16(i))
	case types.Int32: // juga rune
		return int64(int32(i))
	case types.Uint8: // juga byte
		return int64(uint8(i))
	case types.Uint16:
		return int64(uint16(i))
	case types.Uint32:
		return int64(uint32(i))
	}
	return i
}

func constValue(tv types.TypeAndValue) value {
	c := tv.Value
	var v value
	switch c.Kind() {
	case constant.Bool:
		v = boolV(constant.BoolVal(c))
	case constant.String:
		v = strV(constant.StringVal(c))
	case constant.Int:
		if i, ok := constant.Int64Val(c); ok {
			v = intV(i)
		} else {
			f, _ := constant.Float64Val(c)
			v = floatV(f)
		}
	case constant.Float:
		f, _ := constant.Float64Val(c)
		v = floatV(f)
	default:
		return unknown("konstanta kompleks belum disimulasikan")
	}
	return coerce(v, tv.Type)
}

// ---------- format ----------

type unknownMark struct{}

func (unknownMark) String() string { return "?" }

// structPrint mencetak struct seperti fmt: {Ani 20}, atau {Name:Ani Age:20} untuk %+v.
type structPrint struct {
	names []string
	vals  []any
}

func (s structPrint) Format(f fmt.State, verb rune) {
	var b strings.Builder
	b.WriteByte('{')
	for i, v := range s.vals {
		if i > 0 {
			b.WriteByte(' ')
		}
		if f.Flag('+') {
			b.WriteString(s.names[i] + ":")
			fmt.Fprintf(&b, "%+v", v)
		} else {
			fmt.Fprint(&b, v)
		}
	}
	b.WriteByte('}')
	_, _ = f.Write([]byte(b.String()))
}

// ptrPrint mencetak pointer: &{...} untuk struct, alamat palsu yang deterministik untuk yang lain.
type ptrPrint struct {
	target   any
	isStruct bool
	pid      int
}

func (p ptrPrint) Format(f fmt.State, verb rune) {
	if p.isStruct {
		if f.Flag('+') {
			_, _ = fmt.Fprintf(f, "&%+v", p.target)
		} else {
			_, _ = fmt.Fprintf(f, "&%v", p.target)
		}
		return
	}
	_, _ = fmt.Fprintf(f, "0xc%09x", 0x10000+p.pid*0x18)
}

type errPrint string

func (e errPrint) Error() string { return string(e) }

// native mengubah v menjadi nilai Go biasa supaya fmt mencetaknya persis seperti program asli.
func native(v value) any { return nativeD(v, 0) }

func nativeD(v value, depth int) any {
	if depth > 8 {
		return "…" // struktur melingkar (mis. linked list yang menunjuk dirinya sendiri)
	}
	switch v.k {
	case kStruct:
		if v.st == nil {
			return structPrint{}
		}
		vals := make([]any, len(v.st.fields))
		for i, fv := range v.st.fields {
			vals[i] = nativeD(fv, depth+1)
		}
		return structPrint{names: v.st.names, vals: vals}
	case kPtr:
		if v.ptr == nil {
			return nil
		}
		return ptrPrint{target: nativeD(*v.ptr, depth+1), isStruct: v.ptr.k == kStruct, pid: int(v.i)}
	case kError:
		return errPrint(v.s)
	}
	switch v.k {
	case kBool:
		return v.b
	case kInt:
		return int(v.i)
	case kFloat:
		return v.f
	case kString:
		return v.s
	case kSlice:
		elems := v.sl.elems()
		out := make([]any, len(elems))
		for i, e := range elems {
			out[i] = nativeD(e, depth+1)
		}
		return out
	case kArray:
		out := make([]any, len(v.arr))
		for i, e := range v.arr {
			out[i] = nativeD(e, depth+1)
		}
		return out
	case kMap:
		out := map[any]any{}
		if v.m != nil {
			for _, k := range v.m.keys {
				val, _ := v.m.get(k)
				out[nativeD(k, depth+1)] = nativeD(val, depth+1)
			}
		}
		return out
	case kNil:
		return nil
	case kFunc:
		return "func"
	case kChan:
		return "chan"
	}
	return unknownMark{}
}

func format(v value) string { return fmt.Sprint(native(v)) }

// show memformat nilai untuk catatan dan daftar variabel: string diberi tanda kutip, dipotong bila panjang.
func show(v value) string {
	s := format(v)
	switch {
	case v.k == kString:
		s = fmt.Sprintf("%q", v.s)
	case v.k == kPtr && v.ptr != nil && v.ptr.k != kStruct:
		s = "→ " + show(*v.ptr) // pointer ke nilai biasa: tampilkan yang ditunjuk, bukan alamat
	case v.k == kError:
		s = "error(" + fmt.Sprintf("%q", v.s) + ")"
	}
	const max = 40
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max-1]) + "…"
	}
	return s
}

func describe(v value) string {
	switch v.k {
	case kSlice:
		return fmt.Sprintf("%d elemen", v.sl.elemsLen())
	case kArray:
		return fmt.Sprintf("%d elemen", len(v.arr))
	case kString:
		return fmt.Sprintf("%d karakter", utf8.RuneCountInString(v.s))
	case kMap:
		n := 0
		if v.m != nil {
			n = len(v.m.keys)
		}
		return fmt.Sprintf("%d pasangan", n)
	case kInt:
		return fmt.Sprintf("angka %d", v.i)
	case kChan:
		return "channel"
	case kFunc:
		return "iterator"
	}
	return strings.TrimSpace(format(v))
}

func (s *sliceVal) elemsLen() int {
	if s == nil {
		return 0
	}
	return s.len
}
