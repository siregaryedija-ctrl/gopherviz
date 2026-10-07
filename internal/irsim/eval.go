package irsim

import (
	"fmt"
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/trace"
)

func (it *interp) eval(x ir.Expr, e *Env) Value {
	it.tick()
	switch n := x.(type) {
	case nil:
		return undef()
	case *ir.Ident:
		return it.lookupName(n, e)
	case *ir.Lit:
		return it.sem.lit(it, n)
	case *ir.Template:
		return it.template(n, e)
	case *ir.Unary:
		return it.evalUnary(n, e)
	case *ir.Binary:
		return it.evalBinary(n, e)
	case *ir.Assign:
		return it.evalAssign(n, e)
	case *ir.Cond:
		if it.cond(n.C, e, "?:") {
			return it.eval(n.T, e)
		}
		return it.eval(n.F, e)
	case *ir.Call:
		return it.evalCall(n, e)
	case *ir.New:
		return it.evalNew(n, e)
	case *ir.Member:
		return it.evalMember(n, e)
	case *ir.Index:
		return it.evalIndex(n, e)
	case *ir.Slice:
		return it.evalSlice(n, e)
	case *ir.ArrayLit:
		return it.arrayLit(n, e)
	case *ir.ObjectLit:
		return it.objectLit(n, e)
	case *ir.Func:
		return Value{k: kFunc, fn: it.closure(n, e)}
	case *ir.Class:
		c := it.defineClass(n, e)
		return Value{k: kClass, c: c}
	case *ir.Cast:
		v := it.eval(n.X, e)
		return it.cast(v, n.Type, n)
	case *ir.Comp:
		return it.comprehension(n, e)
	case *ir.This:
		return it.thisValue(n, e)
	case *ir.Seq:
		var v Value
		for _, y := range n.List {
			v = it.eval(y, e)
		}
		return v
	case *ir.Spread:
		it.structural(fmt.Sprintf("spread di baris %d belum disimulasikan di posisi ini", it.line(n)))
	case *ir.KwArg:
		return it.eval(n.Value, e)
	case *ir.TypeRef:
		return unknown("tipe " + n.Name)
	case *ir.BadExpr:
		it.structural(fmt.Sprintf("ekspresi di baris %d belum lengkap", it.line(n)))
	}
	it.structural(fmt.Sprintf("ekspresi di baris %d belum disimulasikan", it.line(x)))
	return Value{}
}

// ---------- nama ----------

// varCell mencari sel variabel menurut aturan cakupan bahasa.
func (it *interp) varCell(e *Env, name string) *Cell {
	switch it.lang {
	case "php":
		return e.local(name)
	case "python":
		fe := e.fnEnv()
		if it.pyGlobals[fe][name] {
			return it.globals.vars[name]
		}
	}
	return e.lookup(name)
}

func (it *interp) lookupName(id *ir.Ident, e *Env) Value {
	name := id.Name
	if c := it.varCell(e, name); c != nil {
		v := c.get()
		if v.k == kUnknown && v.why == "belum diisi" {
			it.throw(id, "UnboundLocalError", fmt.Sprintf("cannot access local variable '%s' where it is not associated with a value", name))
		}
		return v
	}
	if v, ok := it.implicitField(e, name); ok {
		return v
	}
	if it.lang == "c" || it.lang == "cpp" {
		if fs := it.funcs[name]; len(fs) > 0 {
			return Value{k: kFunc, fn: &Func{name: name, decl: fs[0], env: it.globals, overloads: fs}}
		}
	}
	if it.lang == "php" && !strings.HasPrefix(name, "$") {
		if fs := it.funcs[strings.ToLower(name)]; len(fs) > 0 { // nama fungsi PHP tidak peka huruf besar
			return Value{k: kFunc, fn: &Func{name: fs[0].Name, decl: fs[0], env: it.globals}}
		}
	}
	if c := it.classes[name]; c != nil {
		return Value{k: kClass, c: c}
	}
	if v, ok := it.macros[name]; ok {
		return v
	}
	if v, ok := it.sem.global(it, name); ok {
		return v
	}
	switch it.lang {
	case "javascript", "typescript":
		it.throw(id, "ReferenceError", name+" is not defined")
	case "python":
		it.throw(id, "NameError", fmt.Sprintf("name '%s' is not defined", name))
	case "php":
		if strings.HasPrefix(name, "$") {
			it.warn("Warning: Undefined variable " + name)
			return null()
		}
		if strings.ToUpper(name) == name { // konstanta tak terdefinisi
			it.throw(id, "Error", fmt.Sprintf("Undefined constant \"%s\"", name))
		}
		return strV(name)
	}
	it.structural(fmt.Sprintf("%q di baris %d belum dideklarasikan (atau berasal dari library yang belum didukung)", name, it.line(id)))
	return Value{}
}

// implicitField: di method C++/Java, nama field/method dari objek this atau field statis kelas
// boleh ditulis tanpa this.
func (it *interp) implicitField(e *Env, name string) (Value, bool) {
	if it.lang != "cpp" && it.lang != "java" {
		return Value{}, false
	}
	if this := e.thisVal(); this != nil && this.k == kObj {
		if v, ok := this.o.fields.get(strV(name)); ok {
			return v, true
		}
		if m := findMethod(this.o.class, name); m != nil {
			return Value{k: kFunc, fn: &Func{name: name, decl: m, env: this.o.class.env, this: this, class: it.classOf[m]}}, true
		}
	}
	for c := e.owner(); c != nil; c = c.super {
		if c.statics != nil {
			if v, ok := c.statics.get(strV(name)); ok {
				return v, true
			}
		}
		if m := findMethod(c, name); m != nil {
			return Value{k: kFunc, fn: &Func{name: name, decl: m, env: c.env, class: c, static: true}}, true
		}
	}
	return Value{}, false
}

func findMethod(c *Class, name string) *Fn {
	for k := c; k != nil; k = k.super {
		if m := k.methods[name]; m != nil && (m.Body != nil || m.ExprBody != nil || k.kind != "interface") {
			return m
		}
	}
	for k := c; k != nil; k = k.super { // Java: method default dari interface
		for _, i := range k.ifaces {
			if m := findMethod(i, name); m != nil && (m.Body != nil || m.ExprBody != nil) {
				return m
			}
		}
	}
	return nil
}

// assignName menulis variabel menurut aturan bahasa.
func (it *interp) assignName(id *ir.Ident, e *Env, v Value) {
	name := id.Name
	switch it.lang {
	case "javascript", "typescript":
		if c := e.lookup(name); c != nil {
			if c.konst {
				it.throw(id, "TypeError", "Assignment to constant variable.")
			}
			c.set(v)
			return
		}
		it.globals.define(name, v)
	case "python":
		fe := e.fnEnv()
		switch {
		case it.pyGlobals[fe][name]:
			it.globals.define(name, v)
		case it.pyNonloc[fe][name]:
			for s := fe.parent; s != nil; s = s.parent {
				if c, ok := s.vars[name]; ok {
					c.set(v)
					return
				}
			}
			it.throw(id, "SyntaxError", "no binding for nonlocal '"+name+"' found")
		default:
			if c, ok := fe.vars[name]; ok {
				c.set(v)
				return
			}
			fe.define(name, v)
		}
	case "php":
		if c := e.local(name); c != nil {
			c.set(v)
			return
		}
		e.fnEnv().define(name, v)
	default:
		if c := e.lookup(name); c != nil {
			if c.konst {
				it.structural(fmt.Sprintf("%s adalah konstanta dan tidak bisa diubah (baris %d)", name, it.line(id)))
			}
			if c.typ != "" {
				v = it.sem.convert(it, v, c.typ)
			}
			c.set(v)
			return
		}
		if this := e.thisVal(); this != nil && this.k == kObj && this.o.fields.has(strV(name)) {
			this.o.fields.set(strV(name), it.convertField(this.o.class, name, v))
			return
		}
		for c := e.owner(); c != nil; c = c.super {
			if c.statics != nil && c.statics.has(strV(name)) {
				c.statics.set(strV(name), v)
				return
			}
		}
		it.structural(fmt.Sprintf("%q di baris %d belum dideklarasikan", name, it.line(id)))
	}
}

func (it *interp) convertField(c *Class, name string, v Value) Value {
	for ; c != nil; c = c.super {
		for _, f := range c.fields {
			if f.name == name && f.typ != "" {
				return it.sem.convert(it, v, f.typ)
			}
		}
	}
	return v
}

// ---------- deklarasi ----------

func (it *interp) varDecl(n *ir.VarDecl, e *Env) {
	switch it.lang {
	case "javascript", "typescript":
		scope := e
		if n.Kind == "var" {
			scope = e.fnEnv()
		}
		for _, s := range n.Specs {
			if s.Init == nil && n.Kind == "var" && scope.vars[s.Name] != nil {
				continue // var x; tidak menghapus nilai yang sudah ada
			}
			v := undef()
			if s.Init != nil {
				v = it.eval(s.Init, e)
			}
			if s.Pattern != nil {
				it.bindTarget(s.Pattern, scope, v, true)
				continue
			}
			c := scope.define(s.Name, v)
			c.konst = n.Kind == "const"
		}
	case "python":
		for _, s := range n.Specs {
			if s.Init == nil {
				continue // x: int (anotasi tanpa nilai)
			}
			v := it.eval(s.Init, e)
			if s.Pattern != nil {
				it.bindTarget(s.Pattern, e, v, false)
			} else {
				it.assignName(&ir.Ident{S: s.S, Name: s.Name}, e, v)
			}
		}
	case "php":
		for _, s := range n.Specs {
			v := null()
			if s.Init != nil {
				v = it.eval(s.Init, e)
			}
			switch n.Kind {
			case "const":
				it.globals.define(s.Name, v)
			case "static":
				it.phpStatic(s, e, v)
			default:
				e.fnEnv().define(s.Name, v)
			}
		}
	default:
		for _, s := range n.Specs {
			if strings.HasPrefix(n.Kind, "static ") && !e.global && e.fnEnv() != it.globals {
				// variabel lokal static: diinisialisasi sekali, nilainya bertahan antar panggilan
				c, ok := it.statics[s]
				if !ok {
					tmp := newEnv(it.globals, false)
					it.typedDecl(n, s, tmp)
					c = tmp.vars[s.Name]
					it.statics[s] = c
				}
				e.vars[s.Name] = c
				e.order = append(e.order, s.Name)
				continue
			}
			it.typedDecl(n, s, e)
		}
	}
}

// phpStatic: static $x = 0; nilai bertahan antar panggilan fungsi.
func (it *interp) phpStatic(s *ir.VarSpec, e *Env, v Value) {
	c, ok := it.statics[s]
	if !ok {
		c = &Cell{v: v}
		it.statics[s] = c
	}
	fe := e.fnEnv()
	fe.vars[s.Name] = c
	fe.order = append(fe.order, s.Name)
}

// typedDecl: deklarasi C/C++/Java "int x = 5", "int a[3]", "Point p(1, 2)", "int& r = x".
func (it *interp) typedDecl(n *ir.VarDecl, s *ir.VarSpec, e *Env) {
	typ := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s.Type, "static "), "final "))
	konst := strings.HasPrefix(typ, "const ") || strings.Contains(n.Kind, "final") || strings.HasPrefix(n.Kind, "const")
	typ = strings.TrimSpace(strings.TrimPrefix(typ, "const "))
	if s.Ref && s.Init != nil { // referensi C++: alias ke variabel lain
		p := it.addrOf(s.Init, e)
		c := e.define(s.Name, Value{})
		c.alias = p
		return
	}
	if s.Pattern != nil { // auto [a, b] = pair
		it.bindTarget(s.Pattern, e, it.eval(s.Init, e), true)
		return
	}
	var v Value
	switch {
	case len(s.Dims) > 0:
		v = it.makeArray(typ, s.Dims, s.Init, e)
	case s.Ctor != nil:
		args := it.evalArgs(s.Ctor, e)
		v = it.constructType(typ, args.pos, s)
	case s.Init != nil:
		if al, ok := s.Init.(*ir.ArrayLit); ok && al.Kind == "init" {
			v = it.initList(typ, al, e)
		} else if ol, ok := s.Init.(*ir.ObjectLit); ok && ol.Kind == "struct" {
			v = it.designatedInit(typ, ol, e)
		} else {
			v = it.eval(s.Init, e)
			v = copyFor(it, v)
		}
	default:
		v = it.zeroOfType(typ, s)
	}
	if typ != "auto" && typ != "var" && !strings.HasSuffix(typ, "auto") && len(s.Dims) == 0 {
		v = it.sem.convert(it, v, typ)
	}
	c := e.define(s.Name, v)
	if typ != "auto" && typ != "var" && len(s.Dims) == 0 {
		c.typ = typ
	}
	c.konst = konst
}

// ---------- target assignment ----------

// bindTarget menyimpan v ke target (nama, field, indeks, atau pola destructuring).
// declare = true: nama dibuat di e (variabel loop, catch, deklarasi pola).
func (it *interp) bindTarget(t ir.Expr, e *Env, v Value, declare bool) {
	switch x := t.(type) {
	case *ir.Ident:
		if declare {
			e.define(x.Name, v)
		} else {
			it.assignName(x, e, v)
		}
	case *ir.Member:
		it.setMember(x, e, v)
	case *ir.Index:
		it.setIndex(x, e, v)
	case *ir.ArrayLit:
		elems := it.sequence(v, x)
		for i, el := range x.Elems {
			if sp, ok := el.(*ir.Spread); ok { // [a, ...rest] / a, *rest
				restN := len(elems) - (len(x.Elems) - 1)
				var rest []Value
				if restN > 0 && i <= len(elems) {
					rest = append(rest, elems[i:min(len(elems), i+restN)]...)
				}
				kind := "array"
				if it.lang == "python" {
					kind = "list"
				}
				it.bindTarget(sp.X, e, it.newList(kind, rest), declare)
				for j := i + 1; j < len(x.Elems); j++ {
					k := len(elems) - (len(x.Elems) - j)
					var ev Value = undef()
					if k >= 0 && k < len(elems) {
						ev = elems[k]
					}
					it.bindTarget(x.Elems[j], e, ev, declare)
				}
				return
			}
			var ev Value = undef()
			if i < len(elems) {
				ev = elems[i]
			} else if it.lang == "python" {
				it.throw(x, "ValueError", fmt.Sprintf("not enough values to unpack (expected %d, got %d)", len(x.Elems), len(elems)))
			}
			if l, ok := el.(*ir.Lit); ok && l.Kind == ir.LitUndefined {
				continue // lubang [, b]
			}
			it.bindTarget(el, e, ev, declare)
		}
		if it.lang == "python" && len(elems) > len(x.Elems) {
			it.throw(x, "ValueError", fmt.Sprintf("too many values to unpack (expected %d)", len(x.Elems)))
		}
	case *ir.ObjectLit:
		used := map[string]bool{}
		for _, p := range x.Props {
			if p.Spread {
				rest := it.dictV("object")
				if v.k == kDict {
					for i, k := range v.d.keys {
						if !used[keyID(k)] {
							rest.d.set(k, v.d.vals[i])
						}
					}
				}
				it.bindTarget(p.Value, e, rest, declare)
				continue
			}
			key := it.propKey(p, e)
			used[keyID(key)] = true
			var pv Value
			if it.lang == "php" && v.k == kDict { // ['x' => $x] = $arr
				pv, _ = v.d.get(it.dictKey(key))
				if pv.k == kUnknown && pv.why == "" {
					pv = null()
				}
			} else {
				pv = it.getProp(v, it.sem.str(it, key), x)
			}
			it.bindTarget(p.Value, e, pv, declare)
		}
	case *ir.Assign: // nilai bawaan pola: {a = 1} / [a = 1]
		if v.k == kUndef {
			v = it.eval(x.R, e)
		}
		it.bindTarget(x.L, e, v, declare)
	case *ir.Unary:
		if x.Op == "*" { // C: *p = v
			p := it.eval(x.X, e)
			if p.k != kPtr {
				it.structural(fmt.Sprintf("dereferensi di baris %d bukan pointer", it.line(x)))
			}
			p.p.store(it.convertPtr(p.p, v))
			if p.p.list != nil && it.rec.coll != "" {
				it.rec.dirty = true // elemen koleksi berubah lewat pointer (mis. tukar(&a[i], &a[j]))
			}
			return
		}
		it.structural(fmt.Sprintf("target assignment di baris %d belum disimulasikan", it.line(t)))
	default:
		it.structural(fmt.Sprintf("target assignment di baris %d belum disimulasikan", it.line(t)))
	}
}

func (it *interp) convertPtr(p *Ptr, v Value) Value {
	if p.cell != nil && p.cell.typ != "" {
		return it.sem.convert(it, v, p.cell.typ)
	}
	if p.list != nil && p.list.elem != "" {
		return it.sem.convert(it, v, p.list.elem)
	}
	return v
}

// sequence mengubah nilai menjadi daftar elemen untuk destructuring.
func (it *interp) sequence(v Value, at ir.Node) []Value {
	switch v.k {
	case kList:
		return v.l.elems
	case kStr:
		var out []Value
		for _, r := range v.s {
			out = append(out, strV(string(r)))
		}
		return out
	case kDict:
		if it.lang == "php" {
			return v.d.vals
		}
		return v.d.keys
	}
	if pairs, ok := it.sem.iterate(it, v, nil); ok {
		var out []Value
		for _, p := range pairs {
			out = append(out, p.v)
		}
		return out
	}
	it.throw(at, "TypeError", it.sem.show(it, v)+" is not iterable")
	return nil
}

func (it *interp) evalAssign(n *ir.Assign, e *Env) Value {
	switch n.Op {
	case "=", ":=":
		v := it.eval(n.R, e)
		v = copyFor(it, v)
		it.recordSet(n.L, e, v)
		it.bindTarget(n.L, e, v, false)
		it.markColl(n.L, e)
		return v
	case "??=", "||=", "&&=":
		cur := it.eval(n.L, e)
		do := false
		switch n.Op {
		case "??=":
			do = cur.k == kNull || cur.k == kUndef
		case "||=":
			do = !it.sem.truthy(cur)
		case "&&=":
			do = it.sem.truthy(cur)
		}
		if !do {
			return cur
		}
		v := it.eval(n.R, e)
		it.bindTarget(n.L, e, v, false)
		it.markColl(n.L, e)
		return v
	}
	op := strings.TrimSuffix(n.Op, "=")
	cur := it.eval(n.L, e)
	r := it.eval(n.R, e)
	if it.lang == "python" && op == "+" && cur.k == kList && r.k == kList {
		cur.l.elems = append(cur.l.elems, r.l.elems...) // += pada list mengubah list yang sama
		it.markColl(n.L, e)
		return cur
	}
	v := it.sem.binary(it, op, cur, r, n)
	it.recordSet(n.L, e, v)
	it.bindTarget(n.L, e, v, false)
	it.markColl(n.L, e)
	return v
}

// recordSet mencatat penulisan ke elemen koleksi mode data.
func (it *interp) recordSet(t ir.Expr, e *Env, v Value) {
	if it.rec.coll == "" || !it.recording() {
		return
	}
	if key, ok := it.mapKey(t, e); ok {
		it.rec.op(trace.Op{Op: trace.OpSet, A: -1, Key: key, Value: it.format(v)})
		return
	}
	if idx := it.collIndex(t, e); idx >= 0 {
		it.rec.op(trace.Op{Op: trace.OpSet, A: idx, Value: it.format(v)})
		return
	}
	// tukar dengan destructuring: [a[i], a[j]] = [a[j], a[i]] / a[i], a[j] = a[j], a[i]
	if al, ok := t.(*ir.ArrayLit); ok && len(al.Elems) == 2 {
		i0, i1 := it.collIndex(al.Elems[0], e), it.collIndex(al.Elems[1], e)
		if i0 >= 0 && i1 >= 0 && i0 != i1 {
			a, b := min(i0, i1), max(i0, i1)
			it.rec.op(trace.Op{Op: trace.OpSwap, A: a, B: &b})
		}
	}
}

func (it *interp) markColl(t ir.Expr, e *Env) {
	if it.rec.coll == "" || !it.recording() {
		return
	}
	switch x := t.(type) {
	case *ir.Ident:
		if x.Name == it.rec.coll {
			it.rec.dirty = true
		}
	case *ir.Index:
		if id, ok := x.X.(*ir.Ident); ok && id.Name == it.rec.coll {
			it.rec.dirty = true
		}
	case *ir.ArrayLit:
		for _, el := range x.Elems {
			it.markColl(el, e)
		}
	}
}

// ---------- unary ----------

func (it *interp) evalUnary(n *ir.Unary, e *Env) Value {
	switch n.Op {
	case "++", "--":
		cur := it.eval(n.X, e)
		if cur.k == kUnknown {
			return cur
		}
		op := "+"
		if n.Op == "--" {
			op = "-"
		}
		one := intV(1)
		if it.lang == "javascript" || it.lang == "typescript" {
			one = floatV(1)
		}
		nv := it.sem.binary(it, op, cur, one, n)
		if cur.k == kPtr { // p++ pada pointer
			nv = cur
			np := *cur.p
			if op == "+" {
				np.idx++
			} else {
				np.idx--
			}
			nv.p = &np
		}
		it.recordSet(n.X, e, nv)
		it.bindTarget(n.X, e, nv, false)
		it.markColl(n.X, e)
		if n.Postfix {
			return cur
		}
		return nv
	case "typeof":
		if id, ok := n.X.(*ir.Ident); ok {
			if it.varCell(e, id.Name) == nil {
				if _, ok := it.sem.global(it, id.Name); !ok {
					return strV("undefined")
				}
			}
		}
		return it.sem.unary(it, "typeof", it.eval(n.X, e), n)
	case "delete":
		it.deleteTarget(n.X, e)
		return boolV(true)
	case "await":
		v := it.eval(n.X, e)
		it.warn("await disimulasikan secara langsung (sinkron); urutan output program async bisa berbeda dengan aslinya")
		return it.awaitValue(v, n)
	case "yield", "yield*", "yield from":
		it.structural("generator (yield) belum disimulasikan")
	case "&":
		p := it.addrOf(n.X, e)
		return Value{k: kPtr, p: p}
	case "*":
		v := it.eval(n.X, e)
		switch v.k {
		case kPtr:
			return v.p.load()
		case kList: // *arr = elemen pertama
			if len(v.l.elems) > 0 {
				return v.l.elems[0]
			}
		case kStr:
			if len(v.s) > 0 {
				return typedInt(int64(v.s[0]), "char")
			}
			return typedInt(0, "char")
		case kNull:
			it.throw(n, "SegmentationFault", "dereferensi pointer NULL")
		}
		return unknown("dereferensi nilai ini belum disimulasikan")
	case "sizeof":
		return it.sizeOf(n.X, e)
	case "clone":
		v := it.eval(n.X, e)
		if v.k == kObj {
			o := &Object{class: v.o.class, fields: it.cloneDict(v.o.fields)}
			it.ids++
			o.id = it.ids
			return Value{k: kObj, o: o}
		}
		return v
	case "throw":
		v := it.eval(n.X, e)
		panic(thrown{v: v, msg: it.exceptionText(v), at: n})
	case "new":
		return it.eval(n.X, e)
	case "$":
		it.structural("variabel variabel ($$x) belum disimulasikan")
	}
	return it.sem.unary(it, n.Op, it.eval(n.X, e), n)
}

func (it *interp) cloneDict(d *Dict) *Dict {
	nd := it.newDict(d.kind)
	for i, k := range d.keys {
		nd.set(k, d.vals[i])
	}
	nd.next = d.next
	return nd
}

// addrOf mengembalikan pointer ke variabel/elemen (C/C++ &x, referensi).
func (it *interp) addrOf(x ir.Expr, e *Env) *Ptr {
	switch t := x.(type) {
	case *ir.Ident:
		if c := it.varCell(e, t.Name); c != nil {
			if c.alias != nil {
				return c.alias
			}
			return &Ptr{cell: c, name: t.Name}
		}
	case *ir.Index:
		base := it.eval(t.X, e)
		idx := it.eval(t.I, e)
		if base.k == kStr && isNum(idx) {
			if id, ok := t.X.(*ir.Ident); ok {
				if c := it.varCell(e, id.Name); c != nil {
					return &Ptr{cell: c, idx: int(toF(idx)), char: true, name: it.text(t)}
				}
			}
		}
		if base.k == kList && isNum(idx) {
			return &Ptr{list: base.l, idx: int(toF(idx)), name: it.text(t)}
		}
		if base.k == kPtr && base.p.list != nil && isNum(idx) {
			return &Ptr{list: base.p.list, idx: base.p.idx + int(toF(idx)), name: it.text(t)}
		}
	case *ir.Member:
		obj := it.eval(t.X, e)
		if t.Op == "->" && obj.k == kPtr {
			obj = obj.p.load()
		}
		if obj.k == kObj {
			return &Ptr{obj: obj.o, field: t.Name, name: it.text(t)}
		}
	case *ir.Unary:
		if t.Op == "*" {
			v := it.eval(t.X, e)
			if v.k == kPtr {
				return v.p
			}
		}
	}
	// nilai sementara: alamat ke sel baru
	return &Ptr{cell: &Cell{v: it.eval(x, e)}, name: it.text(x)}
}

func (it *interp) deleteTarget(x ir.Expr, e *Env) {
	switch t := x.(type) {
	case *ir.Index:
		base := it.eval(t.X, e)
		key := it.eval(t.I, e)
		switch base.k {
		case kDict:
			if !base.d.del(it.dictKey(key)) && it.lang == "python" {
				it.throw(t, "KeyError", it.sem.show(it, key))
			}
			if id, ok := t.X.(*ir.Ident); ok && id.Name == it.rec.coll && it.recording() {
				it.rec.op(trace.Op{Op: trace.OpDelete, A: -1, Key: it.format(key)})
			}
		case kList:
			i := int(toF(key))
			if i < 0 {
				i += len(base.l.elems)
			}
			if i < 0 || i >= len(base.l.elems) {
				it.throw(t, "IndexError", "list assignment index out of range")
			}
			if it.lang == "javascript" || it.lang == "typescript" {
				base.l.elems[i] = undef()
			} else {
				base.l.elems = append(base.l.elems[:i:i], base.l.elems[i+1:]...)
			}
			it.markColl(t, e)
		}
	case *ir.Member:
		obj := it.eval(t.X, e)
		if obj.k == kDict {
			obj.d.del(strV(t.Name))
		} else if obj.k == kObj {
			obj.o.fields.del(strV(t.Name))
		}
	case *ir.Ident:
		if c := e.fnEnv(); c != nil {
			delete(c.vars, t.Name)
			for i, n := range c.order {
				if n == t.Name {
					c.order = append(c.order[:i:i], c.order[i+1:]...)
					break
				}
			}
		}
	case *ir.Call: // unset($a, $b) dibungkus
		for _, a := range t.Args {
			it.deleteTarget(a, e)
		}
	case *ir.ArrayLit:
		for _, a := range t.Elems {
			it.deleteTarget(a, e)
		}
	}
}

// ---------- biner ----------

func (it *interp) evalBinary(n *ir.Binary, e *Env) Value {
	switch n.Op {
	case "&&", "and":
		a := it.eval(n.X, e)
		if a.k == kUnknown {
			return a
		}
		if !it.sem.truthy(a) {
			return it.logicResult(a, false)
		}
		b := it.eval(n.Y, e)
		return it.logicResult(b, it.sem.truthy(b))
	case "||", "or":
		a := it.eval(n.X, e)
		if a.k == kUnknown {
			return a
		}
		if it.sem.truthy(a) {
			return it.logicResult(a, true)
		}
		b := it.eval(n.Y, e)
		return it.logicResult(b, it.sem.truthy(b))
	case "??":
		a := it.eval(n.X, e)
		if a.k != kNull && a.k != kUndef {
			return a
		}
		return it.eval(n.Y, e)
	case "instanceof":
		a := it.eval(n.X, e)
		name := ""
		switch y := n.Y.(type) {
		case *ir.TypeRef:
			name = y.Name
		case *ir.Ident:
			name = y.Name
		}
		if a.k == kObj {
			return boolV(instanceOf(a.o.class, strings.TrimPrefix(name, "\\")))
		}
		return boolV(builtinInstance(a, name))
	case "as", "satisfies":
		return it.eval(n.X, e)
	}
	a, b := it.eval(n.X, e), it.eval(n.Y, e)
	if a.k == kUnknown {
		return a
	}
	if b.k == kUnknown {
		return b
	}
	v := it.sem.binary(it, n.Op, a, b, n)
	if isCompareOp(n.Op) && it.recording() && it.rec.coll != "" {
		it.compareOp(n, e, a, b)
	}
	return v
}

func builtinInstance(v Value, name string) bool {
	switch name {
	case "Array":
		return v.k == kList
	case "Object":
		return v.k == kDict || v.k == kList || v.k == kObj
	case "Function":
		return v.k == kFunc
	}
	return false
}

// logicResult: JS dan Python mengembalikan operandnya; bahasa lain mengembalikan boolean.
func (it *interp) logicResult(v Value, truth bool) Value {
	switch it.lang {
	case "javascript", "typescript", "python":
		return v
	case "c":
		if truth {
			return typedInt(1, "int")
		}
		return typedInt(0, "int")
	}
	return boolV(truth)
}

func (it *interp) compareOp(n *ir.Binary, e *Env, a, b Value) {
	ia, ib := it.collIndex(n.X, e), it.collIndex(n.Y, e)
	if ia < 0 && ib < 0 {
		return
	}
	rel := it.relation(a, b)
	if ia < 0 {
		ia, ib = ib, -1
		rel = map[string]string{">": "<", "<": ">", "=": "="}[rel]
	}
	op := trace.Op{Op: trace.OpCompare, A: ia, Result: rel}
	if ib >= 0 {
		op.B = &ib
	} else {
		other := b
		if it.collIndex(n.X, e) < 0 {
			other = a
		}
		op.Value = it.format(other)
	}
	it.rec.op(op)
}

func (it *interp) relation(a, b Value) string {
	lt := func(x, y Value) bool {
		if x.k == kStr && y.k == kStr {
			return x.s < y.s
		}
		return toF(x) < toF(y)
	}
	switch {
	case lt(a, b):
		return "<"
	case lt(b, a):
		return ">"
	}
	return "="
}

// ---------- template ----------

func (it *interp) template(n *ir.Template, e *Env) Value {
	if n.Tag != nil {
		it.structural("tagged template belum disimulasikan")
	}
	var b strings.Builder
	for i, p := range n.Parts {
		b.WriteString(p)
		if i < len(n.Exprs) {
			v := it.eval(n.Exprs[i], e)
			if v.k == kUnknown {
				return v
			}
			spec := ""
			if i < len(n.Specs) {
				spec = n.Specs[i]
			}
			if spec != "" {
				b.WriteString(it.pyFormat(v, spec, n))
			} else {
				b.WriteString(it.sem.str(it, v))
			}
		}
	}
	return strV(b.String())
}

// ---------- literal koleksi ----------

func (it *interp) arrayLit(n *ir.ArrayLit, e *Env) Value {
	var elems []Value
	for _, x := range n.Elems {
		if sp, ok := x.(*ir.Spread); ok {
			v := it.eval(sp.X, e)
			elems = append(elems, it.sequence(v, sp)...)
			continue
		}
		v := it.eval(x, e)
		if v.k == kUnknown {
			return v
		}
		elems = append(elems, copyFor(it, v))
	}
	switch {
	case it.lang == "php":
		d := it.newDict("array")
		for _, v := range elems {
			d.set(intV(d.next), v)
		}
		return Value{k: kDict, d: d}
	case n.Kind == "set":
		d := it.newDict("set")
		for _, v := range elems {
			d.set(v, v)
		}
		return Value{k: kSet, d: d}
	case n.Kind == "tuple":
		return it.newList("tuple", elems)
	case n.Kind == "list":
		return it.newList("list", elems)
	case n.Kind == "init" && (it.lang == "java"):
		return it.newList("array", elems)
	case n.Kind == "init":
		return it.newList("init", elems)
	}
	return it.newList("array", elems)
}

func (it *interp) propKey(p *ir.Prop, e *Env) Value {
	switch k := p.Key.(type) {
	case *ir.Ident:
		if !p.Computed {
			return strV(k.Name)
		}
	case *ir.Lit:
		if it.lang == "javascript" || it.lang == "typescript" {
			return strV(it.sem.str(it, it.sem.lit(it, k)))
		}
	}
	return it.eval(p.Key, e)
}

func (it *interp) objectLit(n *ir.ObjectLit, e *Env) Value {
	switch n.Kind {
	case "dict":
		d := it.newDict("dict")
		for _, p := range n.Props {
			if p.Spread {
				v := it.eval(p.Value, e)
				if v.k == kDict {
					for i, k := range v.d.keys {
						d.set(k, v.d.vals[i])
					}
				}
				continue
			}
			k := it.eval(p.Key, e)
			v := it.eval(p.Value, e)
			if k.k == kUnknown || v.k == kUnknown {
				return unknown("isi dict tidak diketahui")
			}
			d.set(k, v)
		}
		return Value{k: kDict, d: d}
	case "array": // PHP berkey
		d := it.newDict("array")
		for _, p := range n.Props {
			v := copyFor(it, it.eval(p.Value, e))
			if p.Key == nil {
				d.set(intV(d.next), v)
				continue
			}
			d.set(it.dictKey(it.eval(p.Key, e)), v)
		}
		return Value{k: kDict, d: d}
	case "struct":
		return unknown("designated initializer")
	}
	d := it.newDict("object")
	for _, p := range n.Props {
		if p.Spread {
			v := it.eval(p.Value, e)
			switch v.k {
			case kDict:
				for i, k := range v.d.keys {
					d.set(k, v.d.vals[i])
				}
			case kList:
				for i, x := range v.l.elems {
					d.set(strV(fmt.Sprint(i)), x)
				}
			case kObj:
				for i, k := range v.o.fields.keys {
					d.set(k, v.o.fields.vals[i])
				}
			}
			continue
		}
		key := it.propKey(p, e)
		v := it.eval(p.Value, e)
		if f, ok := p.Value.(*ir.Func); ok && f.Kind == "get" {
			v = it.call(v, Value{k: kDict, d: d}, nil, n) // getter objek literal: dihitung sekali
		}
		d.set(strV(it.sem.str(it, key)), v)
	}
	return Value{k: kDict, d: d}
}

// dictKey menormalkan key map: PHP mengubah "5" menjadi 5, JS objek memakai string.
func (it *interp) dictKey(k Value) Value {
	switch it.lang {
	case "php":
		switch k.k {
		case kStr:
			if n, ok := phpIntKey(k.s); ok {
				return intV(n)
			}
		case kBool:
			if k.b {
				return intV(1)
			}
			return intV(0)
		case kFloat:
			return intV(int64(k.f))
		case kNull:
			return strV("")
		}
	case "javascript", "typescript":
		if k.k != kStr {
			return strV(it.sem.str(it, k))
		}
	}
	return k
}

// ---------- this ----------

func (it *interp) thisValue(n *ir.This, e *Env) Value {
	switch n.Name {
	case "this", "self", "static":
		if n.Name == "self" || n.Name == "static" {
			if c := e.owner(); c != nil {
				return Value{k: kClass, c: c}
			}
		}
		if t := e.thisVal(); t != nil {
			if it.lang == "cpp" && t.k == kObj {
				return Value{k: kPtr, p: &Ptr{cell: &Cell{v: *t}, name: "this"}}
			}
			return *t
		}
		return undef()
	case "super", "parent":
		if c := e.owner(); c != nil && c.super != nil {
			return Value{k: kClass, c: c.super, t: "super"}
		}
	}
	return undef()
}

// ---------- cast & sizeof ----------

func (it *interp) cast(v Value, typ string, at ir.Node) Value {
	if v.k == kUnknown {
		return v
	}
	return it.sem.convert(it, v, strings.TrimSpace(typ))
}

func (it *interp) sizeOf(x ir.Expr, e *Env) Value {
	sz := func(t string) int64 {
		t = strings.TrimSpace(t)
		switch {
		case strings.HasSuffix(t, "*"):
			return 8
		case t == "char" || t == "bool" || t == "unsigned char" || t == "signed char":
			return 1
		case t == "short":
			return 2
		case t == "int" || t == "float" || t == "unsigned" || t == "unsigned int" || t == "long" && it.lang != "" || t == "unsigned long":
			return 4
		case t == "double" || t == "long long" || t == "size_t" || t == "long double":
			return 8
		}
		if c := it.classes[strings.TrimPrefix(t, "struct ")]; c != nil {
			var n int64
			for _, f := range c.fields {
				if strings.Contains(f.typ, "double") || strings.Contains(f.typ, "*") || strings.Contains(f.typ, "long long") {
					n = (n+7)/8*8 + 8
				} else if f.typ == "char" {
					n++
				} else {
					n = (n+3)/4*4 + 4
				}
			}
			return n
		}
		return 4
	}
	if tr, ok := x.(*ir.TypeRef); ok {
		return typedInt(sz(tr.Name), "size_t")
	}
	if id, ok := x.(*ir.Ident); ok {
		if c := it.varCell(e, id.Name); c != nil {
			v := c.get()
			if v.k == kList {
				return typedInt(int64(len(v.l.elems))*sz(v.l.elem), "size_t")
			}
			if c.typ != "" {
				return typedInt(sz(c.typ), "size_t")
			}
		}
	}
	v := it.eval(x, e)
	if v.k == kList {
		return typedInt(int64(len(v.l.elems))*sz(v.l.elem), "size_t")
	}
	return typedInt(sz(v.t), "size_t")
}

// ---------- comprehension (Python) ----------

func (it *interp) comprehension(n *ir.Comp, e *Env) Value {
	scope := newEnv(e, false)
	var out []Value
	d := it.newDict("dict")
	var loop func(i int)
	loop = func(i int) {
		if i == len(n.Fors) {
			switch n.Kind {
			case "dict":
				d.set(it.eval(n.Key, scope), it.eval(n.Value, scope))
			default:
				out = append(out, it.eval(n.Elt, scope))
			}
			return
		}
		f := n.Fors[i]
		src := it.eval(f.Iter, scope)
		if src.k == kUnknown {
			it.structural("isi comprehension tidak diketahui: " + src.why)
		}
		pairs, ok := it.sem.iterate(it, src, nil)
		if !ok {
			it.throw(n, "TypeError", "object is not iterable")
		}
		for _, p := range pairs {
			it.tick()
			it.bindTarget(f.Target, scope, p.v, true)
			keep := true
			for _, c := range f.Ifs {
				if !it.cond(c, scope, "if") {
					keep = false
					break
				}
			}
			if keep {
				loop(i + 1)
			}
		}
	}
	it.quiet++
	loop(0)
	it.quiet--
	switch n.Kind {
	case "dict":
		return Value{k: kDict, d: d}
	case "set":
		s := it.newDict("set")
		for _, v := range out {
			s.set(v, v)
		}
		return Value{k: kSet, d: s}
	}
	return it.newList("list", out)
}
