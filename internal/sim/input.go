package sim

import (
	"fmt"
	"go/ast"
	"go/types"
	"strconv"

	"gopherviz/internal/userinput"
)

// userText memberi baris pengganti input user (angka acak, lihat package userinput) dan
// menampilkannya di output seperti yang diketik di terminal. ok=false bila input sudah habis.
func (it *interp) userText() (string, bool) {
	if it.input == nil {
		it.input = userinput.New(nil, 0)
	}
	s, ok := it.input.Next()
	if ok {
		it.output(s + "\n")
	}
	if !ok && it.input.Stalled() {
		it.inputEnded()
	}
	return s, ok
}

// storeInput menulis input ke variabel yang ditunjuk (fmt.Scan(&x)) sesuai tipenya.
func storeInput(cell *value, s string) {
	switch cell.k {
	case kInt:
		cell.i, _ = strconv.ParseInt(s, 10, 64)
	case kFloat:
		cell.f, _ = strconv.ParseFloat(s, 64)
	case kString:
		cell.s = s
	}
}

// scanCall menangani fmt.Scan, fmt.Scanln, dan fmt.Scanf: setiap pointer diisi satu angka acak.
func (it *interp) scanCall(name string, args []value) []value {
	ptrs := args
	if name == "Scanf" && len(ptrs) > 0 {
		ptrs = ptrs[1:]
	}
	n := 0
	for _, a := range ptrs {
		if a.k != kPtr || a.ptr == nil {
			continue
		}
		s, ok := it.userText()
		if !ok {
			return []value{intV(int64(n)), it.newErr("EOF", nil)}
		}
		storeInput(a.ptr, s)
		n++
	}
	return []value{intV(int64(n)), {k: kNil}}
}

// newReader membuat bufio.Reader / bufio.Scanner tiruan: field pertama menyimpan baris terakhir.
func newReader() value {
	return value{k: kPtr, ptr: &value{k: kStruct, st: &structVal{names: []string{"n"}, fields: []value{strV("")}}}}
}

// bufioCall menjalankan method bufio.Reader dan bufio.Scanner yang membaca dari keyboard.
func (it *interp) bufioCall(sel *ast.SelectorExpr, call *ast.CallExpr, e *env, typ, name string) []value {
	x := it.eval(sel.X, e)
	it.args(call, e)
	var f *value
	if x.k == kPtr && x.ptr != nil && x.ptr.k == kStruct && x.ptr.st != nil && len(x.ptr.st.fields) > 0 {
		f = &x.ptr.st.fields[0]
	}
	switch typ + "." + name {
	case "Reader.ReadString":
		s, ok := it.userText()
		if !ok {
			return []value{strV(""), it.newErr("EOF", nil)}
		}
		return []value{strV(s + "\n"), {k: kNil}}
	case "Scanner.Scan":
		if f != nil {
			s, ok := it.userText()
			f.s = s
			return []value{boolV(ok)}
		}
	case "Scanner.Text":
		if f != nil {
			return []value{strV(f.s)}
		}
	}
	it.warn(fmt.Sprintf("bufio.%s.%s is not simulated yet", typ, name))
	return []value{unknown(it.text(call) + " is not simulated yet")}
}

// stubType mengembalikan nama tipe dari package pkg (mis. "WaitGroup" untuk sync) bila sel
// adalah method tipe itu.
func stubType(sel *types.Selection, pkg string) string {
	if sel == nil || sel.Kind() != types.MethodVal {
		return ""
	}
	if p := sel.Obj().Pkg(); p == nil || p.Path() != pkg {
		return ""
	}
	return namedOf(derefType(sel.Recv()))
}
