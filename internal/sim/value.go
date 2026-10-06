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
)

// value adalah nilai Go di dalam simulator. Slice, map, dan channel adalah
// referensi (berbagi data seperti di Go); array adalah nilai (disalin).
type value struct {
	k   kind
	b   bool
	i   int64
	f   float64
	s   string
	sl  *sliceVal // nil = slice nil
	arr []value
	m   *mapVal // nil = map nil
	fn  *funcVal
	ch  *chanVal
	tup []value
	why string // alasan untuk kUnknown
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
	builtin func(args []value) []value
}

type chanVal struct {
	buf    []value
	cap    int
	closed bool
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

// copyVal meniru semantik assignment Go: array disalin, yang lain berbagi.
func copyVal(v value) value {
	if v.k == kArray {
		arr := make([]value, len(v.arr))
		for i, e := range v.arr {
			arr[i] = copyVal(e)
		}
		v.arr = arr
	}
	return v
}

// ---------- tipe ----------

func zeroOf(t types.Type) value {
	if t == nil {
		return unknown("tipe tidak diketahui")
	}
	switch u := t.Underlying().(type) {
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

// coerce menyesuaikan v dengan tipe statis t (mis. konstanta 3 di konteks float64).
func coerce(v value, t types.Type) value {
	if t == nil {
		return v
	}
	switch u := t.Underlying().(type) {
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

// native mengubah v menjadi nilai Go biasa supaya fmt mencetaknya persis seperti program asli.
func native(v value) any {
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
			out[i] = native(e)
		}
		return out
	case kArray:
		out := make([]any, len(v.arr))
		for i, e := range v.arr {
			out[i] = native(e)
		}
		return out
	case kMap:
		out := map[any]any{}
		if v.m != nil {
			for _, k := range v.m.keys {
				val, _ := v.m.get(k)
				out[native(k)] = native(val)
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
	if v.k == kString {
		s = fmt.Sprintf("%q", v.s)
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
