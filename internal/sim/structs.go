package sim

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"
)

// ---------- struct dan field ----------

func (it *interp) structLit(n *ast.CompositeLit, u *types.Struct, e *env) value {
	st := &structVal{}
	for i := 0; i < u.NumFields(); i++ {
		st.names = append(st.names, u.Field(i).Name())
		st.fields = append(st.fields, zeroOf(u.Field(i).Type()))
	}
	for i, el := range n.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			id, _ := kv.Key.(*ast.Ident)
			if id == nil {
				continue
			}
			if k := st.index(id.Name); k >= 0 {
				st.fields[k] = coerce(copyVal(it.eval(kv.Value, e)), u.Field(k).Type())
			}
			continue
		}
		if i < len(st.fields) {
			st.fields[i] = coerce(copyVal(it.eval(el, e)), u.Field(i).Type())
		}
	}
	return value{k: kStruct, st: st}
}

// deref mengikuti pointer; pointer nil membuat program panic seperti Go.
func (it *interp) deref(v value, at ast.Node) value {
	if v.k != kPtr {
		return v
	}
	if v.ptr == nil {
		it.panicf(at, "runtime error: invalid memory address or nil pointer dereference")
	}
	return *v.ptr
}

// fieldPath mengikuti jalur indeks field (termasuk field dari struct tertanam) dan
// mengembalikan alamat field terakhir, supaya bisa dibaca maupun diubah.
func (it *interp) fieldPath(base *value, path []int, at ast.Node) *value {
	cur := base
	for _, idx := range path {
		if cur.k == kPtr {
			if cur.ptr == nil {
				it.panicf(at, "runtime error: invalid memory address or nil pointer dereference")
			}
			cur = cur.ptr
		}
		if cur.k != kStruct || cur.st == nil || idx >= len(cur.st.fields) {
			it.structural(fmt.Sprintf("field %q belum bisa dibaca simulator (baris %d)", it.text(at), it.line(at)))
		}
		cur = &cur.st.fields[idx]
	}
	return cur
}

func (it *interp) selector(n *ast.SelectorExpr, e *env) value {
	sel := it.p.Info.Selections[n]
	if sel == nil {
		return unknown(fmt.Sprintf("%s belum disimulasikan", it.text(n)))
	}
	switch sel.Kind() {
	case types.FieldVal:
		base := it.eval(n.X, e)
		if base.k == kUnknown {
			return base
		}
		return *it.fieldPath(&base, sel.Index(), n)
	case types.MethodVal:
		// method value (x.M tanpa dipanggil): receiver diikat sekarang
		recv, decl := it.receiver(n, e)
		if decl == nil {
			return unknown(fmt.Sprintf("method %s belum disimulasikan", it.text(n)))
		}
		return value{k: kFunc, fn: &funcVal{name: n.Sel.Name, decl: decl, recv: &recv}}
	}
	return unknown(fmt.Sprintf("%s belum disimulasikan", it.text(n)))
}

// ---------- pointer ----------

func (it *interp) newPtr(cell *value) value {
	it.pids++
	return value{k: kPtr, ptr: cell, i: int64(it.pids)}
}

// addressOf menjalankan &x: alamat variabel, elemen, field, atau nilai baru di heap.
func (it *interp) addressOf(x ast.Expr, e *env) value {
	switch t := unparen(x).(type) {
	case *ast.CompositeLit:
		v := it.eval(t, e)
		return it.newPtr(&v)
	case *ast.SelectorExpr:
		if sel := it.p.Info.Selections[t]; sel != nil && sel.Kind() == types.FieldVal {
			return it.newPtr(it.fieldPath(it.ref(t.X, e), sel.Index(), t))
		}
	}
	return it.newPtr(it.ref(x, e))
}

// ---------- method ----------

// recvTypeName mengembalikan nama tipe receiver sebuah method: "Person" untuk (p *Person).
func recvTypeName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	t := fd.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	if ix, ok := t.(*ast.IndexExpr); ok { // tipe generik T[K]
		t = ix.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func ptrRecv(fd *ast.FuncDecl) bool {
	_, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
	return ok
}

// dynName mengembalikan nama tipe dinamis sebuah nilai (menembus pointer).
func dynName(v value) string {
	if v.k == kPtr && v.ptr != nil {
		return v.ptr.tname
	}
	return v.tname
}

// receiver menyiapkan receiver untuk x.M(): alamat x bila method-nya ber-receiver
// pointer, salinan nilai bila ber-receiver nilai. Interface dipanggil lewat tipe dinamisnya.
func (it *interp) receiver(n *ast.SelectorExpr, e *env) (value, *ast.FuncDecl) {
	x := it.eval(n.X, e)
	name := dynName(x)
	if name == "" {
		if sel := it.p.Info.Selections[n]; sel != nil {
			name = namedOf(derefType(sel.Recv()))
		}
	}
	decl := it.methods[name][n.Sel.Name]
	if decl == nil {
		return x, nil
	}
	if ptrRecv(decl) {
		if x.k == kPtr {
			it.noteRecv(x, x, n)
			return x, decl
		}
		p := it.addressOf(n.X, e)
		it.noteRecv(p, p, n)
		return p, decl
	}
	c := copyVal(it.deref(x, n))
	it.noteRecv(x, c, n) // value receiver: method bekerja pada salinan
	return c, decl
}

func derefType(t types.Type) types.Type {
	if p, ok := t.(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

func (it *interp) callMethod(sel *ast.SelectorExpr, call *ast.CallExpr, e *env) []value {
	// error bawaan (errors.New / fmt.Errorf) hanya punya Error()
	if sel.Sel.Name == "Error" && len(call.Args) == 0 {
		if x := it.eval(sel.X, e); x.k == kError {
			return []value{strV(x.s)}
		}
	}
	recv, decl := it.receiver(sel, e)
	if decl == nil {
		it.warn(fmt.Sprintf("method %s belum disimulasikan", it.text(sel)))
		it.args(call, e)
		return []value{unknown(it.text(sel) + " belum disimulasikan")}
	}
	args := it.args(call, e)
	name := recvTypeName(decl) + "." + decl.Name.Name
	return it.callFunc(&funcVal{name: name, decl: decl, recv: &recv}, args, call)
}

// stringer memanggil String() atau Error() milik tipe user saat nilainya dicetak fmt.
func (it *interp) stringer(v value, at ast.Node) value {
	name := dynName(v)
	if name == "" {
		return v
	}
	for _, m := range []string{"Error", "String"} {
		decl := it.methods[name][m]
		if decl == nil || decl.Type.Params.NumFields() != 0 {
			continue
		}
		recv := v
		if ptrRecv(decl) && v.k != kPtr {
			cell := v
			recv = it.newPtr(&cell)
		} else if !ptrRecv(decl) {
			recv = copyVal(it.deref(v, at))
		}
		res := it.callFunc(&funcVal{name: name + "." + m, decl: decl, recv: &recv}, nil, at)
		if len(res) == 1 && res[0].k == kString {
			return res[0]
		}
	}
	return v
}

// ---------- error ----------

func (it *interp) newErr(msg string, wrap *value) value {
	it.eids++
	return value{k: kError, s: msg, wrap: wrap, eid: it.eids}
}

// errorsIs menelusuri rantai error yang dibungkus (%w) seperti errors.Is.
func errorsIs(err, target value) bool {
	for cur := &err; cur != nil; cur = cur.wrap {
		if sameErr(*cur, target) {
			return true
		}
		if cur.k != kError {
			return false
		}
	}
	return false
}

func sameErr(a, b value) bool {
	switch {
	case a.k == kError && b.k == kError:
		return a.eid == b.eid
	case a.k == kPtr && b.k == kPtr:
		return a.ptr == b.ptr
	}
	return a.k == kNil && b.k == kNil
}

// equal membandingkan dua nilai dengan aturan == Go untuk struct, pointer, dan error.
func equal(a, b value) (bool, bool) {
	switch {
	case a.k == kStruct && b.k == kStruct:
		if a.st == nil || b.st == nil || len(a.st.fields) != len(b.st.fields) {
			return false, true
		}
		for i := range a.st.fields {
			eq, ok := equal(a.st.fields[i], b.st.fields[i])
			if !ok {
				return false, false
			}
			if !eq {
				return false, true
			}
		}
		return true, true
	case a.k == kPtr && b.k == kPtr:
		return a.ptr == b.ptr, true
	case a.k == kError && b.k == kError:
		return a.eid == b.eid, true
	case a.k == kInt && b.k == kInt:
		return a.i == b.i, true
	case a.k == kFloat && b.k == kFloat:
		return a.f == b.f, true
	case a.k == kString && b.k == kString:
		return a.s == b.s, true
	case a.k == kBool && b.k == kBool:
		return a.b == b.b, true
	}
	return false, false
}

// errorfMessage membuat pesan fmt.Errorf dan mengambil error yang dibungkus %w.
func errorfMessage(format string, args []value) (string, *value) {
	var wrapped *value
	verbs := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			continue
		}
		if format[i+1] == '%' {
			i++
			continue
		}
		if format[i+1] == 'w' && verbs < len(args) {
			w := args[verbs]
			wrapped = &w
		}
		verbs++
	}
	natives := make([]any, len(args))
	for i, a := range args {
		natives[i] = native(a)
	}
	return fmt.Sprintf(strings.ReplaceAll(format, "%w", "%v"), natives...), wrapped
}
