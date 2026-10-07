package irsim

import (
	"fmt"
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// closure membuat nilai fungsi dari deklarasi f di lingkungan e.
func (it *interp) closure(f *ir.Func, e *Env) *Func {
	fn := &Func{name: f.Name, decl: f, env: e, class: e.owner()}
	if f.Arrow && (it.lang == "javascript" || it.lang == "typescript") {
		fn.this = e.thisVal() // arrow function memakai this dari luar
		if fn.this == nil {
			u := undef()
			fn.this = &u
		}
	}
	if f.Name == "" {
		fn.name = "(anonim)"
		if it.lang == "python" {
			fn.name = "<lambda>"
		}
	}
	if it.lang == "php" && len(f.Uses) > 0 {
		fn.captured = map[string]Value{}
		for _, u := range f.Uses {
			if c := e.local(u); c != nil {
				fn.captured[u] = copyFor(it, c.get())
			} else {
				fn.captured[u] = null()
			}
		}
	}
	if it.lang == "python" { // default dievaluasi sekali, di lingkungan tempat def ditulis
		for i, p := range f.Params {
			if p.Default != nil {
				if fn.defaults == nil {
					fn.defaults = map[int]Value{}
				}
				fn.defaults[i] = it.eval(p.Default, e)
			}
		}
	}
	return fn
}

type argList struct {
	pos   []Value
	kw    map[string]Value
	kwOrd []string
	exprs []ir.Expr // ekspresi asli (untuk parameter referensi)
}

func (it *interp) evalArgs(args []ir.Expr, e *Env) argList {
	var out argList
	for _, a := range args {
		switch x := a.(type) {
		case *ir.Spread:
			v := it.eval(x.X, e)
			if v.k == kUnknown {
				out.pos = append(out.pos, v)
				continue
			}
			out.pos = append(out.pos, it.sequence(v, x)...)
		case *ir.KwArg:
			v := it.eval(x.Value, e)
			if out.kw == nil {
				out.kw = map[string]Value{}
			}
			if x.Star2 {
				if v.k == kDict {
					for i, k := range v.d.keys {
						name := it.sem.str(it, k)
						out.kw[name] = v.d.vals[i]
						out.kwOrd = append(out.kwOrd, name)
					}
				}
				continue
			}
			out.kw[x.Name] = v
			out.kwOrd = append(out.kwOrd, x.Name)
		default:
			out.pos = append(out.pos, it.eval(a, e))
			out.exprs = append(out.exprs, a)
		}
	}
	return out
}

// ---------- panggilan ----------

func (it *interp) evalCall(c *ir.Call, e *Env) Value {
	if it.lang == "php" {
		if id, ok := c.Fun.(*ir.Ident); ok {
			switch strings.ToLower(id.Name) {
			case "isset":
				for _, a := range c.Args {
					if v, ok := it.phpPeek(a, e); !ok || v.k == kNull || v.k == kUndef {
						return boolV(false)
					}
				}
				return boolV(true)
			case "empty":
				if len(c.Args) == 1 {
					v, ok := it.phpPeek(c.Args[0], e)
					return boolV(!ok || !it.sem.truthy(v))
				}
			}
		}
	}
	var fv Value
	this := undef()
	collCall := ""
	switch f := c.Fun.(type) {
	case *ir.This:
		if f.Name == "super" || f.Name == "parent" { // super(...) di konstruktor
			return it.superCtor(c, e)
		}
		if f.Name == "this" && it.lang == "java" { // this(...) memanggil konstruktor lain
			t := e.thisVal()
			if t != nil && t.k == kObj {
				args := it.evalArgs(c.Args, e)
				if ctor := it.findCtor(t.o.class, len(args.pos)); ctor != nil {
					it.callFunc(&Func{name: t.o.class.name, decl: ctor, env: t.o.class.env, class: t.o.class}, *t, args, c)
				}
			}
			return undef()
		}
		fv = it.eval(f, e)
	case *ir.Member:
		if id, ok := f.X.(*ir.Ident); ok && id.Name == "std" && it.lang == "cpp" { // std::sort(...)
			return it.evalCall(&ir.Call{S: c.S, Fun: &ir.Ident{S: f.S, Name: f.Name}, Args: c.Args}, e)
		}
		if it.lang == "cpp" && f.Op == "::" { // Base::metode() di dalam method turunan
			if id, ok := f.X.(*ir.Ident); ok {
				if cls := it.classes[id.Name]; cls != nil {
					if t := e.thisVal(); t != nil {
						self := *t
						if self.k == kPtr {
							self = self.p.load()
						}
						if self.k == kObj && instanceOf(self.o.class, cls.name) {
							if m := findMethod(cls, f.Name); m != nil && !m.Static {
								return it.callFunc(&Func{name: cls.name + "::" + f.Name, decl: m, env: cls.env, class: it.classOf[m]}, self, it.evalArgs(c.Args, e), c)
							}
						}
					}
				}
			}
		}
		if it.lang == "cpp" && f.Op != "::" {
			if obj := it.eval(f.X, e); obj.k == kStr { // method string yang mengubah isi
				args := it.evalArgs(c.Args, e)
				if ns, ret, ok := it.cppStringMutate(obj.s, f.Name, args.pos, c); ok {
					it.bindTarget(f.X, e, strV(ns), false)
					return ret
				}
			}
		}
		if call, ok := f.X.(*ir.Call); ok && it.lang == "python" {
			if id, ok := call.Fun.(*ir.Ident); ok && id.Name == "super" && len(call.Args) == 0 {
				// super().metode(...)
				owner := e.owner()
				t := e.thisVal()
				if owner != nil && owner.super != nil && t != nil {
					args := it.evalArgs(c.Args, e)
					if m := findMethod(owner.super, f.Name); m != nil {
						return it.callFunc(&Func{name: f.Name, decl: m, env: owner.super.env, class: it.classOf[m]}, *t, args, c)
					}
					if owner.super.native != "" {
						return it.nativeInit(owner.super, *t, args.pos, c)
					}
					return none(it)
				}
			}
		}
		if th, ok := f.X.(*ir.This); ok && (th.Name == "super" || th.Name == "parent") {
			owner := e.owner()
			t := e.thisVal()
			if owner != nil && owner.super != nil {
				args := it.evalArgs(c.Args, e)
				if m := findMethod(owner.super, f.Name); m != nil {
					self := undef()
					if t != nil {
						self = *t
					}
					return it.callFunc(&Func{name: f.Name, decl: m, env: owner.super.env, class: it.classOf[m]}, self, args, c)
				}
				if f.Name == "__construct" && owner.super.native != "" && t != nil {
					return it.nativeInit(owner.super, *t, args.pos, c)
				}
				if t != nil {
					if v, ok := it.sem.member(it, *t, f.Name, f); ok {
						return it.call(v, *t, args.pos, c)
					}
				}
			}
		}
		obj := it.eval(f.X, e)
		if obj.k == kUnknown {
			it.evalArgs(c.Args, e)
			return obj
		}
		if (f.Op == "?." || c.Opt) && (obj.k == kNull || obj.k == kUndef) {
			return undef()
		}
		if f.Op == "->" && obj.k == kPtr {
			obj = obj.p.load()
		}
		this = obj
		fv = it.methodOrProp(obj, f.Name, f)
		if id, ok := f.X.(*ir.Ident); ok && id.Name == it.rec.coll && it.recording() {
			collCall = f.Name
		}
	case *ir.Ident:
		if (it.lang == "cpp" || it.lang == "c") && len(c.Args) == 1 && it.varCell(e, f.Name) == nil {
			if _, isInt := cIntType(f.Name); isInt || isFloatType(f.Name) {
				return it.sem.convert(it, it.eval(c.Args[0], e), f.Name)
			}
		}
		if f.Name == "swap" && it.lang == "cpp" && len(c.Args) == 2 && len(it.funcs["swap"]) == 0 {
			pa, pb := it.addrOf(c.Args[0], e), it.addrOf(c.Args[1], e)
			va, vb := pa.load(), pb.load()
			pa.store(vb)
			pb.store(va)
			it.recordSwapCall(c.Args, e)
			it.markColl(c.Args[0], e)
			return undef()
		}
		fv = it.eval(f, e)
		if fv.k == kFunc && fv.fn.this != nil {
			this = *fv.fn.this
		}
	default:
		fv = it.eval(c.Fun, e)
	}
	if c.Opt && (fv.k == kNull || fv.k == kUndef) {
		return undef()
	}
	args := it.evalArgsFor(fv, c.Args, e)
	before := 0
	if collCall != "" {
		if v := it.collValue(e); v.k == kList {
			before = len(v.l.elems)
		}
	}
	r := it.callValueArgs(fv, this, args, c)
	if collCall != "" {
		it.collMutation(collCall, e, before)
	}
	return r
}

// evalArgsFor: parameter referensi (C++ int&, PHP &$x) menerima alamat, bukan nilai.
func (it *interp) evalArgsFor(fv Value, args []ir.Expr, e *Env) argList {
	if fv.k == kFunc && fv.fn.decl != nil && (it.lang == "cpp" || it.lang == "php") {
		params := fv.fn.decl.Params
		hasRef := false
		for _, p := range params {
			if p.Ref {
				hasRef = true
			}
		}
		if hasRef {
			var out argList
			for i, a := range args {
				if i < len(params) && params[i].Ref {
					out.pos = append(out.pos, Value{k: kPtr, p: it.addrOf(a, e), t: "&ref"})
				} else {
					out.pos = append(out.pos, it.eval(a, e))
				}
				out.exprs = append(out.exprs, a)
			}
			return out
		}
	}
	return it.evalArgs(args, e)
}

func (it *interp) collValue(e *Env) Value {
	if c := it.findVar(e, it.rec.coll); c != nil {
		return c.get()
	}
	return Value{}
}

// collMutation mencatat operasi koleksi lewat method: push/append/add → append; lainnya → isi berubah.
func (it *interp) collMutation(name string, e *Env, before int) {
	v := it.collValue(e)
	switch name {
	case "push", "append", "push_back", "add", "emplace_back", "unshift", "insert", "extend",
		"pop", "shift", "remove", "erase", "splice", "sort", "reverse", "clear", "pop_back", "fill", "set", "put", "delete", "discard":
	default:
		return
	}
	if v.k == kList && (name == "push" || name == "append" || name == "push_back" || name == "add" || name == "emplace_back") &&
		len(v.l.elems) == before+1 {
		last := len(v.l.elems) - 1
		it.rec.op(trace.Op{Op: trace.OpAppend, A: last, Value: it.format(v.l.elems[last])})
		return
	}
	it.rec.dirty = true
}

func (it *interp) callValueArgs(fv, this Value, args argList, at ir.Node) Value {
	switch fv.k {
	case kFunc:
		return it.callFunc(fv.fn, this, args, at)
	case kClass:
		if it.lang == "javascript" || it.lang == "typescript" {
			if fv.c.native == "" || fv.c.kind == "class" {
				it.throw(at, "TypeError", fmt.Sprintf("Class constructor %s cannot be invoked without 'new'", fv.c.name))
			}
		}
		return it.constructArgs(fv.c, args, at)
	case kUnknown:
		return fv
	}
	name := it.text(at)
	if c, ok := at.(*ir.Call); ok {
		name = it.text(c.Fun)
	}
	switch it.lang {
	case "javascript", "typescript":
		it.throw(at, "TypeError", name+" is not a function")
	case "python":
		it.throw(at, "TypeError", fmt.Sprintf("'%s' object is not callable", it.pyTypeName(fv)))
	case "php":
		it.throw(at, "Error", "Call to undefined function "+name+"()")
	}
	it.structural(fmt.Sprintf("%s di baris %d bukan fungsi yang dikenal", name, it.line(at)))
	return Value{}
}

// call memanggil nilai fungsi dengan argumen posisi (dipakai builtin seperti map/sort).
func (it *interp) call(fv, this Value, args []Value, at ir.Node) Value {
	return it.callValueArgs(fv, this, argList{pos: args}, at)
}

func (it *interp) callFunc(fn *Func, this Value, args argList, at ir.Node) Value {
	if fn.builtin != nil {
		prev := it.kwargs
		it.kwargs = args.kw
		defer func() { it.kwargs = prev }()
		self := this
		if fn.this != nil {
			self = *fn.this
		}
		return fn.builtin(it, self, args.pos, at)
	}
	if len(fn.overloads) > 1 {
		fn = it.pickByArgs(fn, args.pos)
	}
	decl := fn.decl
	if decl.Generator {
		it.structural("generator (function* / yield) belum disimulasikan")
	}
	it.depth++
	defer func() { it.depth-- }()
	if it.depth > maxDepth {
		switch it.lang {
		case "python":
			it.throw(at, "RecursionError", "maximum recursion depth exceeded")
		case "javascript", "typescript":
			it.throw(at, "RangeError", "Maximum call stack size exceeded")
		}
		it.structural("rekursi terlalu dalam untuk disimulasikan")
	}
	if fn.this != nil {
		this = *fn.this
	}
	r := it.rec
	callerRecording := it.recording()
	var fr *frame
	leave := func() {}
	if decl.Body != nil {
		fr, leave = it.enterBlock(decl, scene.BlockFunc)
	}
	defer leave()
	startsFocus := fr != nil && fr.focus && r.active == 1
	_, viaIter := at.(*ir.ForEach)
	recordCall := (callerRecording || startsFocus) && !viaIter
	if recordCall {
		prevDepth := r.depth
		if startsFocus {
			r.depth = 0
		} else {
			r.depth = prevDepth + 1
		}
		defer func() { r.depth = prevDepth }()
	}

	scope := newEnv(fn.env, true)
	if it.lang == "php" {
		scope.arrow = decl.Arrow
		for k, v := range fn.captured {
			scope.define(k, v)
		}
	}
	owner := fn.class
	if owner == nil {
		owner = it.classOf[decl]
	}
	scope.class = owner
	method := owner != nil && !fn.static && !decl.Static && !hasDecor(decl, "staticmethod")
	if method || it.lang == "javascript" || it.lang == "typescript" {
		t := this
		scope.this = &t
	}
	pos := args.pos
	if it.lang == "python" && owner != nil && !hasDecor(decl, "staticmethod") && (method || hasDecor(decl, "classmethod")) {
		// self / cls adalah parameter pertama
		first := this
		if hasDecor(decl, "classmethod") {
			first = Value{k: kClass, c: owner}
			if this.k == kObj {
				first = Value{k: kClass, c: this.o.class}
			} else if this.k == kClass {
				first = this
			}
		}
		pos = append([]Value{first}, pos...)
	}
	params := it.bindParams(fn, scope, argList{pos: pos, kw: args.kw, kwOrd: args.kwOrd}, at)
	if len(decl.Inits) > 0 && scope.this != nil && scope.this.k == kObj {
		it.ctorInits(decl, scope)
	}
	argText := strings.Join(params, ", ")
	name := fn.name
	if name == "" {
		name = "(anonim)"
	}
	if recordCall && it.recording() {
		it.add(trace.Step{Phase: trace.PhaseCall, Line: it.line(at), Func: name, Args: argText,
			Note: fmt.Sprintf("panggil %s(%s)", name, argText)}, scope)
	}
	quietBody := it.rec.on() && !it.insideFocus(decl)
	if quietBody {
		it.quiet++
	}
	var ret Value
	func() {
		defer func() {
			if quietBody {
				it.quiet--
			}
		}()
		switch {
		case decl.Body != nil:
			if it.lang == "javascript" || it.lang == "typescript" {
				it.hoist(decl.Body.List, scope)
			}
			f := it.execList(decl.Body.List, scope)
			if f.k == fReturn {
				ret = f.ret
			} else {
				ret = it.defaultReturn()
			}
		case decl.ExprBody != nil:
			ret = it.eval(decl.ExprBody, scope)
		default:
			ret = it.defaultReturn()
		}
	}()
	if decl.Ret != "" && it.isTyped() && decl.Ret != "void" && decl.Ret != "auto" {
		ret = it.sem.convert(it, ret, decl.Ret)
	}
	if decl.Async {
		ret = it.resolvedPromise(ret)
	}
	if recordCall && it.recording() {
		rs := it.sem.show(it, ret)
		note := fmt.Sprintf("%s(%s) selesai", name, argText)
		if ret.k != kUndef && !(ret.k == kNull && it.lang != "javascript") && decl.Ret != "void" {
			note = fmt.Sprintf("%s(%s) mengembalikan %s", name, argText, rs)
		} else {
			rs = ""
		}
		line := decl.S.End.Line
		if decl.Body != nil {
			line = decl.Body.S.End.Line
		}
		it.add(trace.Step{Phase: trace.PhaseReturn, Line: line, Func: name, Return: rs, Note: note}, scope)
	}
	return ret
}

func hasDecor(f *ir.Func, name string) bool {
	for _, d := range f.Decor {
		if d == name {
			return true
		}
	}
	return false
}

func (it *interp) defaultReturn() Value {
	switch it.lang {
	case "javascript", "typescript":
		return undef()
	}
	return null()
}

func none(it *interp) Value { return null() }

// bindParams mengisi parameter; mengembalikan teks "nama = nilai" untuk catatan.
func (it *interp) bindParams(fn *Func, scope *Env, args argList, at ir.Node) []string {
	decl := fn.decl
	var texts []string
	used := map[string]bool{}
	i := 0
	for pi, p := range decl.Params {
		var v Value
		switch {
		case p.Rest:
			var rest []Value
			if i < len(args.pos) {
				rest = append(rest, args.pos[i:]...)
			}
			i = len(args.pos)
			kind := "array"
			if it.lang == "python" {
				kind = "tuple"
			}
			v = it.newList(kind, rest)
			if it.lang == "php" {
				v = it.listToPHP(rest)
			}
		case p.KwRest:
			d := it.newDict("dict")
			for _, k := range args.kwOrd {
				if !used[k] {
					d.set(strV(k), args.kw[k])
				}
			}
			v = Value{k: kDict, d: d}
		case args.kw != nil && hasKw(args.kw, p.Name):
			v = args.kw[p.Name]
			used[p.Name] = true
		case i < len(args.pos):
			v = args.pos[i]
			i++
		case p.Default != nil:
			if d, ok := fn.defaults[pi]; ok {
				v = d
			} else {
				v = it.eval(p.Default, scope)
			}
		default:
			switch it.lang {
			case "javascript", "typescript":
				v = undef()
			case "python":
				if fn.decl.Name != "" || pi > 0 {
					missing := []string{}
					for _, q := range decl.Params[pi:] {
						if q.Default == nil && !q.Rest && !q.KwRest && !hasKw(args.kw, q.Name) {
							missing = append(missing, "'"+q.Name+"'")
						}
					}
					it.throw(at, "TypeError", fmt.Sprintf("%s() missing %d required positional argument%s: %s",
						fn.name, len(missing), plural(len(missing)), strings.Join(missing, " and ")))
				}
				v = unknown(p.Name + " adalah parameter fungsi " + fn.name)
			case "php":
				it.throw(at, "ArgumentCountError", fmt.Sprintf("Too few arguments to function %s(), %d passed and at least %d expected", fn.name, len(args.pos), requiredParams(decl)))
			default:
				v = unknown(fmt.Sprintf("%s adalah parameter fungsi %s", p.Name, fn.name))
			}
		}
		if v.k == kPtr && v.t == "&ref" { // parameter referensi
			c := scope.define(p.Name, Value{})
			c.alias = v.p
			c.typ = strings.TrimSuffix(strings.TrimSpace(p.Type), "&")
			texts = append(texts, p.Name+" = "+it.sem.show(it, c.get()))
			continue
		}
		if p.Type != "" && it.isTyped() && !p.Rest {
			v = it.sem.convert(it, v, p.Type)
		}
		v = copyFor(it, v)
		if p.Pattern != nil {
			it.bindTarget(p.Pattern, scope, v, true)
			texts = append(texts, it.text(p.Pattern)+" = "+it.sem.show(it, v))
			continue
		}
		c := scope.define(p.Name, v)
		if p.Type != "" && it.isTyped() && !p.Rest {
			c.typ = p.Type
		}
		if p.Promote {
			if t := scope.this; t != nil && t.k == kObj {
				t.o.fields.set(strV(strings.TrimPrefix(p.Name, "$")), v)
			}
		}
		if p.Name != "self" && p.Name != "cls" || it.lang != "python" || pi > 0 {
			texts = append(texts, p.Name+" = "+it.sem.show(it, v))
		}
	}
	if it.lang == "python" && i < len(args.pos) && !hasRest(decl) {
		it.throw(at, "TypeError", fmt.Sprintf("%s() takes %d positional argument%s but %d were given", fn.name, len(decl.Params), plural(len(decl.Params)), len(args.pos)))
	}
	if it.lang == "javascript" || it.lang == "typescript" {
		scope.define("arguments", it.newList("array", args.pos))
	}
	return texts
}

func hasKw(kw map[string]Value, name string) bool {
	_, ok := kw[name]
	return ok
}

func hasRest(f *ir.Func) bool {
	for _, p := range f.Params {
		if p.Rest {
			return true
		}
	}
	return false
}

func requiredParams(f *ir.Func) int {
	n := 0
	for _, p := range f.Params {
		if p.Default == nil && !p.Rest {
			n++
		}
	}
	return n
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// hoist: deklarasi fungsi dan var JavaScript tersedia sejak awal fungsi.
func (it *interp) hoist(list []ir.Stmt, scope *Env) {
	for _, s := range list {
		ir.Walk(s, func(n ir.Node) bool {
			switch x := n.(type) {
			case *ir.Func:
				return false
			case *ir.Class:
				return false
			case *ir.VarDecl:
				if x.Kind == "var" {
					for _, sp := range x.Specs {
						if sp.Name != "" && scope.vars[sp.Name] == nil {
							scope.define(sp.Name, undef())
						}
					}
				}
			}
			return true
		})
		if f, ok := s.(*ir.Func); ok && f.Name != "" {
			scope.define(f.Name, Value{k: kFunc, fn: it.closure(f, scope)})
		}
	}
}

// ---------- properti ----------

func (it *interp) evalMember(n *ir.Member, e *Env) Value {
	if id, ok := n.X.(*ir.Ident); ok && id.Name == "std" && it.lang == "cpp" { // std::cout
		return it.lookupName(&ir.Ident{S: n.S, Name: n.Name}, e)
	}
	if th, ok := n.X.(*ir.This); ok && (th.Name == "super" || th.Name == "parent") {
		owner := e.owner()
		if owner != nil && owner.super != nil {
			if m := findMethod(owner.super, n.Name); m != nil {
				return Value{k: kFunc, fn: &Func{name: n.Name, decl: m, env: owner.super.env, this: e.thisVal(), class: it.classOf[m]}}
			}
			if v, ok := owner.super.statics.get(strV(n.Name)); ok {
				return v
			}
		}
	}
	obj := it.eval(n.X, e)
	switch {
	case obj.k == kUnknown:
		return obj
	case (n.Op == "?." || n.Op == "?->") && (obj.k == kNull || obj.k == kUndef):
		return undef()
	case n.Op == "->" && obj.k == kPtr:
		obj = obj.p.load()
	}
	if it.recording() && it.rec.coll != "" && it.rec.collKind == "map" {
		if id, ok := n.X.(*ir.Ident); ok && id.Name == it.rec.coll && obj.k == kDict && obj.d.kind == "object" {
			it.rec.op(trace.Op{Op: trace.OpRead, A: -1, Key: n.Name})
		}
	}
	return it.getProp(obj, n.Name, n)
}

// getProp mengambil properti/method name dari obj (method dikembalikan terikat ke obj).
func (it *interp) getProp(obj Value, name string, at ir.Node) Value {
	name = strings.TrimPrefix(name, "$")
	switch obj.k {
	case kUnknown:
		return obj
	case kObj:
		o := obj.o
		if v, ok := o.fields.get(strV(name)); ok {
			return v
		}
		for c := o.class; c != nil; c = c.super {
			if g := c.getters[name]; g != nil {
				return it.callFunc(&Func{name: name, decl: g, env: c.env, class: c}, obj, argList{}, at)
			}
			if m := c.methods[name]; m != nil {
				t := obj
				return Value{k: kFunc, fn: &Func{name: c.name + "." + name, decl: m, env: c.env, this: &t, class: c}}
			}
			if c.statics != nil {
				if v, ok := c.statics.get(strV(name)); ok {
					return v
				}
			}
		}
		if v, ok := it.sem.member(it, obj, name, at); ok {
			return v
		}
		return it.missingProp(obj, name, at)
	case kClass:
		for c := obj.c; c != nil; c = c.super {
			if c.statics != nil {
				if v, ok := c.statics.get(strV(name)); ok {
					return v
				}
			}
			if m := c.methods[name]; m != nil { // method lewat kelas: tanpa self otomatis
				return Value{k: kFunc, fn: &Func{name: c.name + "." + name, decl: m, env: c.env, class: c, static: true}}
			}
		}
		if v, ok := it.sem.member(it, obj, name, at); ok {
			return v
		}
		return it.missingProp(obj, name, at)
	case kDict:
		if obj.d.kind == "object" || obj.d.kind == "module" {
			if v, ok := obj.d.get(strV(name)); ok {
				if v.k == kFunc && v.fn.decl != nil && !v.fn.decl.Arrow {
					t := obj
					f := *v.fn
					f.this = &t
					return Value{k: kFunc, fn: &f}
				}
				return v
			}
		}
	case kNull, kUndef:
		switch it.lang {
		case "javascript", "typescript":
			it.throw(at, "TypeError", fmt.Sprintf("Cannot read properties of %s (reading '%s')", it.sem.str(it, obj), name))
		case "java":
			it.throw(at, "NullPointerException", "Cannot invoke \""+name+"\" because value is null")
		case "python":
			it.throw(at, "AttributeError", fmt.Sprintf("'NoneType' object has no attribute '%s'", name))
		case "php":
			it.throw(at, "Error", fmt.Sprintf("Call to a member function %s() on null", name))
		}
		it.structural("akses lewat pointer/nilai null di baris " + fmt.Sprint(it.line(at)))
	}
	if v, ok := it.sem.member(it, obj, name, at); ok {
		return v
	}
	return it.missingProp(obj, name, at)
}

func (it *interp) missingProp(obj Value, name string, at ir.Node) Value {
	switch it.lang {
	case "javascript", "typescript":
		return undef()
	case "python":
		it.throw(at, "AttributeError", fmt.Sprintf("'%s' object has no attribute '%s'", it.pyTypeName(obj), name))
	case "php":
		it.warn("Warning: Undefined property: $" + name)
		return null()
	}
	return unknown(fmt.Sprintf("%s belum disimulasikan", name))
}

func (it *interp) setMember(n *ir.Member, e *Env, v Value) {
	obj := it.eval(n.X, e)
	if n.Op == "->" && obj.k == kPtr {
		obj = obj.p.load()
	}
	name := strings.TrimPrefix(n.Name, "$")
	switch obj.k {
	case kObj:
		for c := obj.o.class; c != nil; c = c.super {
			if s := c.setters[name]; s != nil {
				it.callFunc(&Func{name: name, decl: s, env: c.env, class: c}, obj, argList{pos: []Value{v}}, n)
				return
			}
			if c.getters[name] != nil && it.lang == "python" {
				it.throw(n, "AttributeError", "property '"+name+"' has no setter")
			}
		}
		if !obj.o.fields.has(strV(name)) {
			for c := obj.o.class; c != nil; c = c.super {
				if c.statics != nil && c.statics.has(strV(name)) && it.lang != "python" && it.lang != "javascript" && it.lang != "typescript" {
					c.statics.set(strV(name), v)
					return
				}
			}
		}
		obj.o.fields.set(strV(name), it.convertField(obj.o.class, name, v))
	case kClass:
		for c := obj.c; c != nil; c = c.super {
			if c.statics.has(strV(name)) {
				c.statics.set(strV(name), v)
				return
			}
		}
		obj.c.statics.set(strV(name), v)
	case kDict:
		obj.d.set(strV(name), v)
		if id, ok := n.X.(*ir.Ident); ok && id.Name == it.rec.coll && it.recording() {
			it.rec.op(trace.Op{Op: trace.OpSet, A: -1, Key: name, Value: it.format(v)})
		}
	case kUnknown:
	case kNull, kUndef:
		switch it.lang {
		case "javascript", "typescript":
			it.throw(n, "TypeError", fmt.Sprintf("Cannot set properties of %s (setting '%s')", it.sem.str(it, obj), name))
		case "java":
			it.throw(n, "NullPointerException", "Cannot assign field \""+name+"\" because value is null")
		}
		it.structural("menulis field lewat null di baris " + fmt.Sprint(it.line(n)))
	case kList:
		if obj.l.kind == "pair" && (name == "first" || name == "second") {
			obj.l.elems[map[string]int{"first": 0, "second": 1}[name]] = v
			return
		}
		it.structural("menulis field " + name + " belum disimulasikan")
	default:
		if it.lang == "python" {
			it.throw(n, "AttributeError", fmt.Sprintf("'%s' object has no attribute '%s'", it.pyTypeName(obj), name))
		}
	}
}

// ---------- indeks ----------

func (it *interp) evalIndex(n *ir.Index, e *Env) Value {
	base := it.eval(n.X, e)
	if base.k == kUnknown {
		return base
	}
	if n.Opt && (base.k == kNull || base.k == kUndef) {
		return undef()
	}
	if n.I == nil {
		it.structural("indeks kosong [] hanya untuk menambah elemen")
	}
	idx := it.eval(n.I, e)
	if idx.k == kUnknown {
		return idx
	}
	if (base.k == kDict) && it.recording() {
		if id, ok := n.X.(*ir.Ident); ok && id.Name == it.rec.coll {
			it.rec.op(trace.Op{Op: trace.OpRead, A: -1, Key: it.format(it.dictKey(idx))})
		}
	}
	return it.index(base, idx, n)
}

func (it *interp) index(base, idx Value, at ir.Node) Value {
	switch base.k {
	case kList:
		if it.lang == "python" && idx.k == kList { // indeks tuple pada list tidak didukung
			it.throw(at, "TypeError", "list indices must be integers or slices, not tuple")
		}
		i, ok := it.intIndex(idx)
		if !ok {
			if it.lang == "javascript" || it.lang == "typescript" {
				if v, ok := it.sem.member(it, base, it.sem.str(it, idx), at); ok {
					return v
				}
				return undef()
			}
			it.throw(at, "TypeError", "indeks harus bilangan bulat")
		}
		n := len(base.l.elems)
		if it.lang == "python" && i < 0 {
			i += n
		}
		if i < 0 || i >= n {
			return it.outOfRange(base, i, n, at)
		}
		return base.l.elems[i]
	case kDict:
		key := it.dictKey(idx)
		if v, ok := base.d.get(key); ok {
			return v
		}
		switch base.d.kind {
		case "defaultdict": // nilai bawaan dibuat lalu disimpan
			z := it.call(*base.d.factory, undef(), nil, at)
			base.d.set(key, z)
			return z
		case "Counter":
			return pyInt(0)
		}
		switch it.lang {
		case "javascript", "typescript":
			if v, ok := it.sem.member(it, base, it.sem.str(it, idx), at); ok {
				return v
			}
			return undef()
		case "python":
			it.throw(at, "KeyError", it.sem.show(it, idx))
		case "php":
			it.warn("Warning: Undefined array key " + it.sem.show(it, key))
			return null()
		case "cpp":
			z := it.mapDefault(base.d)
			base.d.set(key, z) // map[k] membuat entri baru
			return z
		}
		return null()
	case kStr:
		i, ok := it.intIndex(idx)
		if !ok {
			return undef()
		}
		switch it.lang {
		case "c", "cpp":
			if i == len(base.s) {
				return typedInt(0, "char")
			}
			if i < 0 || i > len(base.s) {
				return it.outOfRange(base, i, len(base.s), at)
			}
			return typedInt(int64(base.s[i]), "char")
		case "php":
			if i < 0 {
				i += len(base.s)
			}
			if i < 0 || i >= len(base.s) {
				it.warn(fmt.Sprintf("Warning: Uninitialized string offset %d", i))
				return strV("")
			}
			return strV(base.s[i : i+1])
		}
		rs := []rune(base.s)
		if it.lang == "python" && i < 0 {
			i += len(rs)
		}
		if i < 0 || i >= len(rs) {
			if it.lang == "python" {
				it.throw(at, "IndexError", "string index out of range")
			}
			return undef()
		}
		return strV(string(rs[i]))
	case kPtr:
		i, _ := it.intIndex(idx)
		if base.p.list != nil {
			j := base.p.idx + i
			if j < 0 || j >= len(base.p.list.elems) {
				return it.outOfRange(Value{k: kList, l: base.p.list}, j, len(base.p.list.elems), at)
			}
			return base.p.list.elems[j]
		}
		if i == 0 {
			return base.p.load()
		}
	case kClass: // Enum["Nama"] / Enum[0] (TypeScript)
		return it.getProp(base, it.sem.str(it, idx), at)
	case kObj, kSet:
		if v, ok := it.sem.member(it, base, "[]", at); ok {
			return it.call(v, base, []Value{idx}, at)
		}
	case kNull, kUndef:
		switch it.lang {
		case "javascript", "typescript":
			it.throw(at, "TypeError", fmt.Sprintf("Cannot read properties of %s (reading '%s')", it.sem.str(it, base), it.sem.str(it, idx)))
		case "python":
			it.throw(at, "TypeError", "'NoneType' object is not subscriptable")
		case "php":
			return null()
		case "java":
			it.throw(at, "NullPointerException", "Cannot load from array because it is null")
		}
	}
	if it.lang == "python" {
		it.throw(at, "TypeError", fmt.Sprintf("'%s' object is not subscriptable", it.pyTypeName(base)))
	}
	return unknown("indeks pada nilai ini belum disimulasikan")
}

func (it *interp) intIndex(v Value) (int, bool) {
	switch v.k {
	case kInt:
		return int(v.i), true
	case kFloat:
		if isIntegral(v.f) {
			return int(v.f), true
		}
	case kBool:
		if v.b {
			return 1, true
		}
		return 0, true
	case kStr:
		if it.lang == "javascript" || it.lang == "typescript" || it.lang == "php" {
			var n int
			if _, err := fmt.Sscanf(v.s, "%d", &n); err == nil && fmt.Sprint(n) == v.s {
				return n, true
			}
		}
	}
	return 0, false
}

func (it *interp) outOfRange(base Value, i, n int, at ir.Node) Value {
	switch it.lang {
	case "javascript", "typescript":
		return undef()
	case "python":
		what := "list"
		if base.k == kList && base.l.kind == "tuple" {
			what = "tuple"
		}
		it.throw(at, "IndexError", what+" index out of range")
	case "java":
		it.throw(at, "ArrayIndexOutOfBoundsException", fmt.Sprintf("Index %d out of bounds for length %d", i, n))
	case "php":
		it.warn(fmt.Sprintf("Warning: Undefined array key %d", i))
		return null()
	}
	it.structural(fmt.Sprintf("akses indeks %d di luar batas (panjang %d) di baris %d: perilaku tidak terdefinisi di %s", i, n, it.line(at), map[string]string{"c": "C", "cpp": "C++"}[it.lang]))
	return Value{}
}

func (it *interp) mapDefault(d *Dict) Value {
	if len(d.vals) > 0 {
		return it.sem.zero(d.vals[0].t + kindZero(d.vals[0]))
	}
	return typedInt(0, "int")
}

func kindZero(v Value) string {
	switch v.k {
	case kStr:
		return "string"
	case kFloat:
		return "double"
	case kList:
		return "vector"
	}
	return ""
}

func (it *interp) setIndex(n *ir.Index, e *Env, v Value) {
	base := it.eval(n.X, e)
	if base.k == kUnknown {
		return
	}
	if it.lang == "php" && (base.k == kNull || base.k == kUndef) { // auto-vivifikasi $a[] / $a['k']
		base = it.dictV("array")
		it.bindTarget(n.X, e, base, false)
	}
	if n.I == nil { // PHP $a[] = v
		switch base.k {
		case kDict:
			k := intV(base.d.next)
			base.d.set(k, v)
			if id, ok := n.X.(*ir.Ident); ok && id.Name == it.rec.coll && it.recording() {
				it.rec.op(trace.Op{Op: trace.OpAppend, A: base.d.len() - 1, Value: it.format(v)})
			}
		case kList:
			base.l.elems = append(base.l.elems, v)
		}
		return
	}
	idx := it.eval(n.I, e)
	switch base.k {
	case kList:
		i, ok := it.intIndex(idx)
		if !ok {
			if it.lang == "javascript" || it.lang == "typescript" {
				return
			}
			it.throw(n, "TypeError", "indeks harus bilangan bulat")
		}
		size := len(base.l.elems)
		if it.lang == "python" && i < 0 {
			i += size
		}
		if (it.lang == "javascript" || it.lang == "typescript") && i >= size {
			for len(base.l.elems) < i {
				base.l.elems = append(base.l.elems, undef())
			}
			base.l.elems = append(base.l.elems, v)
			return
		}
		if base.l.kind == "tuple" {
			it.throw(n, "TypeError", "'tuple' object does not support item assignment")
		}
		if i < 0 || i >= size {
			if it.lang == "python" {
				it.throw(n, "IndexError", "list assignment index out of range")
			}
			it.outOfRange(base, i, size, n)
			return
		}
		if base.l.elem != "" {
			v = it.sem.convert(it, v, base.l.elem)
		}
		base.l.elems[i] = v
	case kDict:
		base.d.set(it.dictKey(idx), v)
	case kStr:
		i, _ := it.intIndex(idx)
		if it.lang == "c" || it.lang == "cpp" || it.lang == "php" {
			b := []byte(base.s)
			if i >= 0 && i < len(b) {
				ch := byte(toF(v))
				if v.k == kStr && len(v.s) > 0 {
					ch = v.s[0]
				}
				b[i] = ch
				it.bindTarget(n.X, e, strV(string(b)), false)
				return
			}
		}
		if it.lang == "python" {
			it.throw(n, "TypeError", "'str' object does not support item assignment")
		}
	case kPtr:
		i, _ := it.intIndex(idx)
		if base.p.list != nil {
			j := base.p.idx + i
			if j < 0 || j >= len(base.p.list.elems) {
				it.outOfRange(Value{k: kList, l: base.p.list}, j, len(base.p.list.elems), n)
				return
			}
			base.p.list.elems[j] = it.convertPtr(&Ptr{list: base.p.list, idx: j}, v)
		}
	case kNull, kUndef:
		switch it.lang {
		case "javascript", "typescript":
			it.throw(n, "TypeError", fmt.Sprintf("Cannot set properties of %s", it.sem.str(it, base)))
		case "python":
			it.throw(n, "TypeError", "'NoneType' object does not support item assignment")
		case "java":
			it.throw(n, "NullPointerException", "Cannot store to array because it is null")
		}
	default:
		if it.lang == "python" {
			it.throw(n, "TypeError", fmt.Sprintf("'%s' object does not support item assignment", it.pyTypeName(base)))
		}
	}
}

// evalSlice: a[lo:hi:step] Python.
func (it *interp) evalSlice(n *ir.Slice, e *Env) Value {
	base := it.eval(n.X, e)
	var lo, hi, step *int
	get := func(x ir.Expr) *int {
		if x == nil {
			return nil
		}
		v := it.eval(x, e)
		i, ok := it.intIndex(v)
		if !ok {
			if v.k == kNull {
				return nil
			}
			it.throw(n, "TypeError", "slice indices must be integers or None")
		}
		return &i
	}
	lo, hi, step = get(n.Lo), get(n.Hi), get(n.Step)
	var length int
	switch base.k {
	case kList:
		length = len(base.l.elems)
	case kStr:
		length = len([]rune(base.s))
	case kUnknown:
		return base
	default:
		it.throw(n, "TypeError", "object is not subscriptable")
	}
	idxs := pySliceIndices(length, lo, hi, step, func() { it.throw(n, "ValueError", "slice step cannot be zero") })
	if base.k == kStr {
		rs := []rune(base.s)
		var b strings.Builder
		for _, i := range idxs {
			b.WriteRune(rs[i])
		}
		return strV(b.String())
	}
	out := make([]Value, 0, len(idxs))
	for _, i := range idxs {
		out = append(out, base.l.elems[i])
	}
	return it.newList(base.l.kind, out)
}

func pySliceIndices(n int, lo, hi, step *int, zero func()) []int {
	st := 1
	if step != nil {
		st = *step
	}
	if st == 0 {
		zero()
		return nil
	}
	norm := func(p *int, def int, lowLimit, highLimit int) int {
		if p == nil {
			return def
		}
		v := *p
		if v < 0 {
			v += n
			if v < lowLimit {
				v = lowLimit
			}
		} else if v > highLimit {
			v = highLimit
		}
		return v
	}
	var out []int
	if st > 0 {
		a, b := norm(lo, 0, 0, n), norm(hi, n, 0, n)
		for i := a; i < b; i += st {
			out = append(out, i)
		}
	} else {
		a, b := norm(lo, n-1, -1, n-1), norm(hi, -1, -1, n-1)
		for i := a; i > b; i += st {
			out = append(out, i)
		}
	}
	return out
}

// ---------- kelas ----------

func (it *interp) defineClass(n *ir.Class, e *Env) *Class {
	c := &Class{name: n.Name, methods: map[string]*Fn{}, getters: map[string]*Fn{}, setters: map[string]*Fn{},
		statics: it.newDict("statics"), env: e, kind: n.Kind}
	if c.name == "" {
		c.name = "(anonim)"
	}
	for _, ext := range n.Extends {
		if sc := it.findClass(ext, e); sc != nil && (sc.kind != "interface" || it.lang != "java") {
			if sc.kind == "interface" && it.lang != "python" {
				continue
			}
			c.super = sc
			break
		}
	}
	scope := newEnv(e, false)
	scope.class = c
	if n.Name != "" {
		it.classes[n.Name] = c // tersedia untuk method yang merujuk kelasnya sendiri
	}
	ordinal := 0
	for _, m := range n.Members {
		switch x := m.(type) {
		case *ir.Func:
			it.classOf[x] = c
			switch {
			case x.Kind == "get" || hasDecor(x, "property"):
				c.getters[x.Name] = x
			case x.Kind == "set" || hasDecorSuffix(x, ".setter"):
				c.setters[x.Name] = x
			default:
				if prev := c.methods[x.Name]; prev != nil && (it.lang == "cpp" || it.lang == "java") {
					c.addOverload(x.Name, prev)
					c.addOverload(x.Name, x)
				}
				c.methods[x.Name] = x
				if it.lang == "cpp" || it.lang == "java" {
					c.addOverload(x.Name, x)
				}
			}
		case *ir.VarDecl:
			static := strings.Contains(x.Kind, "static") || x.Kind == "enum" || x.Kind == "const" || it.lang == "python"
			for _, s := range x.Specs {
				name := strings.TrimPrefix(s.Name, "$")
				typ := s.Type
				if typ == "" && x.Kind != "field" && x.Kind != "static field" && x.Kind != "enum" && x.Kind != "const" {
					typ = strings.TrimPrefix(strings.TrimPrefix(x.Kind, "static "), "final ")
				}
				typ = strings.TrimSpace(strings.TrimPrefix(typ, "final "))
				if x.Kind == "enum" {
					ev := it.enumValue(c, s, ordinal, scope)
					c.statics.set(strV(name), ev)
					if n.Kind == "enum" && (it.lang == "c" || it.lang == "cpp") {
						e.define(name, ev) // enum C/C++ biasa: konstantanya ada di cakupan luar
					}
					if it.lang == "typescript" && ev.k == kFloat { // reverse mapping: Color[0] → "Red"
						c.statics.set(strV(jsNumber(ev.f)), strV(name))
					}
					ordinal++
					if isNum(ev) {
						ordinal = int(toF(ev)) + 1
					}
					continue
				}
				if static {
					var v Value
					if s.Init != nil {
						v = it.eval(s.Init, scope)
					} else {
						v = it.zeroOfType(typ, s)
					}
					if typ != "" && it.isTyped() {
						v = it.sem.convert(it, v, typ)
					}
					c.statics.set(strV(name), v)
					continue
				}
				spec := s
				dims := s.Dims
				c.fields = append(c.fields, fieldInit{name: name, typ: typ, array: len(dims) > 0, init: func(it *interp, this Value) Value {
					fs := newEnv(scope, true)
					fs.this = &this
					fs.class = c
					switch {
					case len(dims) > 0:
						return it.makeArray(typ, dims, spec.Init, fs)
					case spec.Init != nil:
						if al, ok := spec.Init.(*ir.ArrayLit); ok && al.Kind == "init" {
							return it.initList(typ, al, fs)
						}
						v := it.eval(spec.Init, fs)
						if typ != "" && it.isTyped() {
							v = it.sem.convert(it, v, typ)
						}
						return v
					case spec.Ctor != nil:
						return it.constructType(typ, it.evalArgs(spec.Ctor, fs).pos, spec)
					}
					if it.lang == "javascript" || it.lang == "typescript" {
						return undef()
					}
					if it.lang == "php" && typ == "" {
						return null()
					}
					return it.zeroOfType(typ, spec)
				}})
			}
		case *ir.Class:
			inner := it.defineClass(x, scope)
			c.statics.set(strV(x.Name), Value{k: kClass, c: inner})
		case *ir.Block: // blok static Java
			it.execList(x.List, scope)
		default:
			if it.lang == "python" { // badan kelas Python: assignment menjadi atribut kelas
				it.pyClassStmt(c, m, scope)
			}
		}
	}
	return c
}

func hasDecorSuffix(f *ir.Func, suffix string) bool {
	for _, d := range f.Decor {
		if strings.HasSuffix(d, suffix) {
			return true
		}
	}
	return false
}

func (c *Class) addOverload(name string, f *Fn) {
	if c.overloads == nil {
		c.overloads = map[string][]*Fn{}
	}
	for _, x := range c.overloads[name] {
		if x == f {
			return
		}
	}
	c.overloads[name] = append(c.overloads[name], f)
}

// pyClassStmt menjalankan statement di badan kelas Python (atribut kelas).
func (it *interp) pyClassStmt(c *Class, s ir.Stmt, scope *Env) {
	cs := newEnv(scope, true)
	it.quietExec(s, cs)
	for _, name := range cs.order {
		c.statics.set(strV(name), cs.vars[name].get())
	}
}

func (it *interp) findClass(name string, e *Env) *Class {
	name = strings.TrimPrefix(strings.TrimSpace(name), "\\")
	if i := strings.IndexByte(name, '<'); i >= 0 {
		name = name[:i]
	}
	if c := it.classes[name]; c != nil {
		return c
	}
	if e != nil {
		if cell := e.lookup(name); cell != nil && cell.get().k == kClass {
			return cell.get().c
		}
	}
	if v, ok := it.sem.global(it, name); ok && v.k == kClass {
		return v.c
	}
	return nil
}

// enumValue: konstanta enum (TS/C/C++ = angka, Java/PHP = objek dengan name/ordinal).
func (it *interp) enumValue(c *Class, s *ir.VarSpec, ordinal int, scope *Env) Value {
	switch it.lang {
	case "java", "php":
		it.ids++
		o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
		o.fields.set(strV("name"), strV(s.Name))
		if it.lang == "java" {
			o.fields.set(strV("ordinal"), typedInt(int64(ordinal), "int"))
		}
		if s.Init != nil && it.lang == "php" {
			o.fields.set(strV("value"), it.eval(s.Init, scope))
		}
		if s.Ctor != nil {
			if ctor := it.findCtor(c, len(s.Ctor)); ctor != nil {
				it.callFunc(&Func{name: c.name, decl: ctor, env: c.env, class: c}, Value{k: kObj, o: o}, it.evalArgs(s.Ctor, scope), s.Ctor[0])
			}
		}
		return Value{k: kObj, o: o}
	}
	v := intV(int64(ordinal))
	if s.Init != nil {
		v = it.eval(s.Init, scope)
	}
	if it.lang == "c" || it.lang == "cpp" {
		v = typedInt(int64(toF(v)), "int")
	}
	return v
}

// findCtor mencari konstruktor kelas c (atau induknya) yang cocok dengan jumlah argumen.
func (it *interp) findCtor(c *Class, nargs int) *Fn {
	for ; c != nil; c = c.super {
		switch it.lang {
		case "javascript", "typescript":
			if m := c.methods["constructor"]; m != nil {
				return m
			}
		case "python":
			if m := c.methods["__init__"]; m != nil {
				return m
			}
		case "php":
			if m := c.methods["__construct"]; m != nil {
				return m
			}
		default:
			if ovs := c.overloads[c.name]; len(ovs) > 0 {
				return pickOverload(ovs, nargs)
			}
			if m := c.methods[c.name]; m != nil {
				return m
			}
			return nil // C++/Java: konstruktor tidak diwarisi
		}
	}
	return nil
}

func pickOverload(ovs []*Fn, nargs int) *Fn {
	for _, f := range ovs {
		req, max := 0, len(f.Params)
		for _, p := range f.Params {
			if p.Default == nil && !p.Rest {
				req++
			}
			if p.Rest {
				max = 1 << 20
			}
		}
		if nargs >= req && nargs <= max {
			return f
		}
	}
	return ovs[0]
}

func (it *interp) construct(c *Class, args []Value, at ir.Node) Value {
	return it.constructArgs(c, argList{pos: args}, at)
}

func (it *interp) constructArgs(c *Class, args argList, at ir.Node) Value {
	if c.native != "" && c.ctor != nil {
		return c.ctor(it, args.pos, at)
	}
	if c.kind == "interface" || c.kind == "abstract" {
		it.throw(at, "Error", "Cannot instantiate "+c.kind+" "+c.name)
	}
	it.ids++
	o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
	obj := Value{k: kObj, o: o}
	it.initFields(c, obj)
	ctor := it.findCtor(c, len(args.pos))
	switch {
	case ctor != nil:
		owner := it.classOf[ctor]
		f := &Func{name: c.name, decl: ctor, env: owner.env, class: owner}
		if ovs := owner.overloads[owner.name]; len(ovs) > 1 {
			f.overloads = ovs
		}
		it.callFunc(f, obj, args, at)
	case it.nativeBase(c) != nil:
		it.nativeInit(it.nativeBase(c), obj, args.pos, at)
	case (it.lang == "c" || it.lang == "cpp") && len(args.pos) > 0: // aggregate: Point{1, 2}
		for i, f := range it.allFields(c) {
			if i < len(args.pos) {
				it.setAggField(obj, f, args.pos[i])
			}
		}
	case it.lang == "python" && len(args.pos) > 0:
		it.throw(at, "TypeError", c.name+"() takes no arguments")
	}
	return obj
}

func (it *interp) nativeBase(c *Class) *Class {
	for ; c != nil; c = c.super {
		if c.native != "" {
			return c
		}
	}
	return nil
}

// nativeInit menjalankan konstruktor kelas bawaan (mis. Error) untuk objek turunan.
func (it *interp) nativeInit(base *Class, obj Value, args []Value, at ir.Node) Value {
	if base.init != nil {
		base.init(it, obj, args, at)
	}
	return undef()
}

func (it *interp) allFields(c *Class) []fieldInit {
	var chain []*Class
	for ; c != nil; c = c.super {
		chain = append([]*Class{c}, chain...)
	}
	var out []fieldInit
	for _, k := range chain {
		out = append(out, k.fields...)
	}
	return out
}

func (it *interp) initFields(c *Class, obj Value) {
	for _, f := range it.allFields(c) {
		obj.o.fields.set(strV(f.name), f.init(it, obj))
	}
}

// superCtor menjalankan super(...) di konstruktor JS/Java/PHP.
func (it *interp) superCtor(c *ir.Call, e *Env) Value {
	owner := e.owner()
	t := e.thisVal()
	args := it.evalArgs(c.Args, e)
	if owner == nil || owner.super == nil || t == nil {
		return undef()
	}
	sup := owner.super
	if sup.native != "" {
		return it.nativeInit(sup, *t, args.pos, c)
	}
	if ctor := it.findCtor(sup, len(args.pos)); ctor != nil {
		so := it.classOf[ctor]
		it.callFunc(&Func{name: sup.name, decl: ctor, env: so.env, class: so}, *t, args, c)
	} else if nb := it.nativeBase(sup); nb != nil {
		it.nativeInit(nb, *t, args.pos, c)
	}
	return undef()
}

// ---------- new ----------

func (it *interp) evalNew(n *ir.New, e *Env) Value {
	if n.Anon != nil {
		c := it.defineClass(n.Anon, e)
		if len(n.Anon.Extends) > 0 && c.super == nil && n.Type != "" {
			c.super = it.findClass(n.Type, e)
		}
		return it.constructArgs(c, it.evalArgs(n.Args, e), n)
	}
	if it.isTyped() {
		base := n.Type
		if i := strings.IndexAny(base, "<["); i >= 0 {
			base = base[:i]
		}
		base = strings.TrimSpace(base)
		if len(n.Dims) > 0 || n.Init != nil {
			elem := strings.TrimSpace(n.Type)
			for strings.HasSuffix(elem, "[]") {
				elem = strings.TrimSuffix(elem, "[]")
			}
			var arr Value
			if n.Init != nil {
				arr = it.initList(elem+"[]", n.Init, e)
			} else {
				arr = it.makeArray(elem, n.Dims, nil, e)
			}
			if it.lang == "cpp" || it.lang == "c" {
				return Value{k: kPtr, p: &Ptr{list: arr.l, idx: 0, name: "heap"}}
			}
			return arr
		}
		args := it.evalArgs(n.Args, e)
		v := it.constructType(n.Type, args.pos, n)
		if it.lang == "cpp" {
			return Value{k: kPtr, p: &Ptr{cell: &Cell{v: v}, name: "heap"}}
		}
		return v
	}
	cls := it.eval(n.Class, e)
	args := it.evalArgs(n.Args, e)
	switch cls.k {
	case kClass:
		return it.constructArgs(cls.c, args, n)
	case kUnknown:
		return cls
	case kStr: // PHP: new $namaKelas
		if c := it.findClass(cls.s, e); c != nil {
			return it.constructArgs(c, args, n)
		}
	}
	it.throw(n, "TypeError", it.text(n.Class)+" is not a constructor")
	return Value{}
}

// constructType membuat nilai bertipe typ dengan argumen konstruktor (C++/Java).
func (it *interp) constructType(typ string, args []Value, at ir.Node) Value {
	base := strings.TrimSpace(typ)
	base = strings.TrimPrefix(base, "std::")
	name := base
	if i := strings.IndexByte(name, '<'); i >= 0 {
		name = name[:i]
	}
	if c := it.findClass(name, it.globals); c != nil {
		if c.native != "" && c.ctorT != nil {
			return c.ctorT(it, base, args, at)
		}
		return it.construct(c, args, at)
	}
	if v, ok := it.sem.global(it, name); ok && v.k == kClass {
		if v.c.ctorT != nil {
			return v.c.ctorT(it, base, args, at)
		}
		return it.construct(v.c, args, at)
	}
	if len(args) == 1 {
		return it.sem.convert(it, args[0], base) // int x(5)
	}
	if len(args) == 0 {
		return it.sem.zero(base)
	}
	return unknown("konstruktor " + typ + " belum disimulasikan")
}

// zeroOfType: nilai awal variabel tanpa inisialisasi.
func (it *interp) zeroOfType(typ string, at ir.Node) Value {
	t := strings.TrimSpace(typ)
	if strings.HasSuffix(t, "*") || strings.HasSuffix(t, "[]") {
		return null()
	}
	name := strings.TrimPrefix(strings.TrimPrefix(t, "struct "), "std::")
	if i := strings.IndexByte(name, '<'); i >= 0 {
		name = name[:i]
	}
	if c := it.findClass(name, it.globals); c != nil {
		if it.lang == "java" {
			return null()
		}
		if c.native != "" {
			if c.ctorT != nil {
				return c.ctorT(it, strings.TrimPrefix(t, "std::"), nil, at)
			}
			return null()
		}
		return it.construct(c, nil, at)
	}
	return it.sem.zero(t)
}

// makeArray membuat array C/C++/Java berdimensi dims, diisi init (bila ada).
func (it *interp) makeArray(elem string, dims []ir.Expr, init ir.Expr, e *Env) Value {
	elem = strings.TrimSpace(strings.TrimSuffix(elem, "[]"))
	var initVals Value
	if init != nil {
		if al, ok := init.(*ir.ArrayLit); ok {
			initVals = it.initList(elem+strings.Repeat("[]", len(dims)), al, e)
		} else {
			initVals = it.eval(init, e)
		}
	}
	var build func(d int, src Value) Value
	build = func(d int, src Value) Value {
		size := -1
		if d < len(dims) && dims[d] != nil {
			n := it.eval(dims[d], e)
			if n.k == kUnknown {
				it.structural("ukuran array tidak diketahui: " + n.why)
			}
			size = int(toF(n))
			if size < 0 {
				if it.lang == "java" {
					it.throw(dims[d], "NegativeArraySizeException", fmt.Sprint(size))
				}
				it.structural("ukuran array negatif")
			}
		}
		var given []Value
		switch src.k {
		case kList:
			given = src.l.elems
		case kStr: // char s[] = "hai"
			for i := 0; i < len(src.s); i++ {
				given = append(given, typedInt(int64(src.s[i]), "char"))
			}
			given = append(given, typedInt(0, "char"))
		}
		if size < 0 {
			size = len(given)
		}
		if size > 100_000 {
			it.structural("array terlalu besar untuk disimulasikan")
		}
		elems := make([]Value, size)
		for i := range elems {
			switch {
			case d+1 < len(dims):
				var sub Value
				if i < len(given) {
					sub = given[i]
				}
				elems[i] = build(d+1, sub)
			case i < len(given):
				elems[i] = it.sem.convert(it, given[i], elem)
			default:
				elems[i] = it.zeroOfType(elem, init)
			}
		}
		kind := "carray"
		if it.lang == "java" {
			kind = "array"
		}
		it.ids++
		et := elem
		if d+1 < len(dims) {
			et = elem + strings.Repeat("[]", len(dims)-d-1)
		}
		return Value{k: kList, l: &List{elems: elems, kind: kind, elem: et, id: it.ids}}
	}
	return build(0, initVals)
}

// initList mengubah {a, b, c} sesuai tipe tujuan: array, struct, vector, map.
func (it *interp) initList(typ string, al *ir.ArrayLit, e *Env) Value {
	t := strings.TrimSpace(typ)
	if strings.HasSuffix(t, "[]") {
		elem := strings.TrimSuffix(t, "[]")
		var elems []Value
		for _, x := range al.Elems {
			if sub, ok := x.(*ir.ArrayLit); ok {
				elems = append(elems, it.initList(elem, sub, e))
				continue
			}
			v := it.eval(x, e)
			if !strings.HasSuffix(elem, "[]") {
				v = it.sem.convert(it, v, elem)
			}
			elems = append(elems, v)
		}
		kind := "carray"
		if it.lang == "java" {
			kind = "array"
		}
		it.ids++
		return Value{k: kList, l: &List{elems: elems, kind: kind, elem: elem, id: it.ids}}
	}
	name := strings.TrimPrefix(strings.TrimPrefix(t, "struct "), "std::")
	base := name
	if i := strings.IndexByte(base, '<'); i >= 0 {
		base = base[:i]
	}
	if c := it.findClass(base, e); c != nil && c.native == "" {
		var args []Value
		fields := it.allFields(c)
		for i, x := range al.Elems {
			if sub, ok := x.(*ir.ArrayLit); ok && i < len(fields) {
				args = append(args, it.initList(fields[i].typ, sub, e))
				continue
			}
			args = append(args, it.eval(x, e))
		}
		if it.findCtor(c, len(args)) != nil {
			return it.construct(c, args, al)
		}
		it.ids++
		o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
		obj := Value{k: kObj, o: o}
		it.initFields(c, obj)
		for i, f := range fields {
			if i < len(args) {
				it.setAggField(obj, f, args[i])
			}
		}
		return obj
	}
	var elems []Value
	for _, x := range al.Elems {
		if sub, ok := x.(*ir.ArrayLit); ok {
			elems = append(elems, it.arrayLit(sub, e))
			continue
		}
		elems = append(elems, it.eval(x, e))
	}
	if c := it.findClass(base, e); c != nil && c.ctorT != nil { // vector<int> v = {1, 2}
		return c.ctorT(it, name, []Value{it.newList("init", elems)}, al)
	}
	if len(elems) == 1 { // int x = {5}
		return it.sem.convert(it, elems[0], t)
	}
	return it.newList("init", elems)
}

func (it *interp) designatedInit(typ string, ol *ir.ObjectLit, e *Env) Value {
	name := strings.TrimPrefix(strings.TrimSpace(typ), "struct ")
	c := it.findClass(name, e)
	if c == nil {
		return unknown("struct " + name + " tidak dikenal")
	}
	obj := it.construct(c, nil, ol)
	for _, p := range ol.Props {
		if id, ok := p.Key.(*ir.Ident); ok {
			obj.o.fields.set(strV(id.Name), it.convertField(c, id.Name, it.eval(p.Value, e)))
		}
	}
	return obj
}

// copyFor menyalin nilai untuk bahasa dengan semantik nilai (struct C/C++, vector, array PHP).
func copyFor(it *interp, v Value) Value {
	switch it.lang {
	case "c", "cpp":
		switch v.k {
		case kObj:
			if v.o.class.native == "" {
				it.ids++
				o := &Object{class: v.o.class, fields: it.newDict("fields"), id: it.ids}
				for i, k := range v.o.fields.keys {
					o.fields.set(k, copyFor(it, v.o.fields.vals[i]))
				}
				return Value{k: kObj, o: o, t: v.t}
			}
		case kList:
			if v.l.kind == "vector" || v.l.kind == "init" || v.l.kind == "list" || v.l.kind == "deque" || v.l.kind == "array-std" {
				it.ids++
				elems := make([]Value, len(v.l.elems))
				for i, x := range v.l.elems {
					elems[i] = copyFor(it, x)
				}
				return Value{k: kList, l: &List{elems: elems, kind: v.l.kind, elem: v.l.elem, id: it.ids}, t: v.t}
			}
		case kDict, kSet:
			nd := it.cloneDict(v.d)
			return Value{k: v.k, d: nd, t: v.t}
		}
	case "php":
		if v.k == kDict && v.d.kind == "array" {
			nd := it.newDict("array")
			for i, k := range v.d.keys {
				nd.set(k, copyFor(it, v.d.vals[i]))
			}
			nd.next = v.d.next
			return Value{k: kDict, d: nd}
		}
	}
	return v
}

func (it *interp) listToPHP(vals []Value) Value {
	d := it.newDict("array")
	for _, v := range vals {
		d.set(intV(d.next), v)
	}
	return Value{k: kDict, d: d}
}

// phpPeek membaca variabel/indeks/properti tanpa peringatan (untuk isset/empty).
func (it *interp) phpPeek(x ir.Expr, e *Env) (Value, bool) {
	switch t := x.(type) {
	case *ir.Ident:
		if c := it.varCell(e, t.Name); c != nil {
			return c.get(), true
		}
		return Value{}, false
	case *ir.Index:
		base, ok := it.phpPeek(t.X, e)
		if !ok || t.I == nil {
			return Value{}, false
		}
		k := it.dictKey(it.eval(t.I, e))
		switch base.k {
		case kDict:
			return base.d.get(k)
		case kStr:
			i, ok := it.intIndex(k)
			if ok && i >= 0 && i < len(base.s) {
				return strV(base.s[i : i+1]), true
			}
		}
		return Value{}, false
	case *ir.Member:
		base, ok := it.phpPeek(t.X, e)
		if !ok {
			return Value{}, false
		}
		name := strings.TrimPrefix(t.Name, "$")
		switch base.k {
		case kObj:
			return base.o.fields.get(strV(name))
		case kClass:
			return base.c.statics.get(strV(name))
		}
		return Value{}, false
	case *ir.This:
		return it.thisValue(t, e), true
	}
	return it.eval(x, e), true
}

// methodOrProp: di PHP/Java/C++ method dan properti punya ruang nama sendiri, jadi
// obj.nama() mencari method lebih dulu.
func (it *interp) methodOrProp(obj Value, name string, at ir.Node) Value {
	if obj.k == kObj && (it.lang == "php" || it.lang == "java" || it.lang == "cpp" || it.lang == "c") {
		for c := obj.o.class; c != nil; c = c.super {
			if m := c.methods[name]; m != nil {
				t := obj
				return Value{k: kFunc, fn: &Func{name: c.name + "." + name, decl: m, env: c.env, this: &t, class: c, overloads: c.overloads[name]}}
			}
		}
	}
	if obj.k == kClass && (it.lang == "php" || it.lang == "java" || it.lang == "cpp") {
		for c := obj.c; c != nil; c = c.super {
			if m := c.methods[name]; m != nil {
				return Value{k: kFunc, fn: &Func{name: c.name + "." + name, decl: m, env: c.env, class: c, static: true, overloads: c.overloads[name]}}
			}
		}
	}
	return it.getProp(obj, name, at)
}

// pickByArgs memilih overload (C/C++/Java) yang paling cocok dengan jumlah dan tipe argumen.
func (it *interp) pickByArgs(fn *Func, args []Value) *Func {
	best, bestScore := fn.overloads[0], -1
	for _, f := range fn.overloads {
		req, max := 0, len(f.Params)
		for _, p := range f.Params {
			if p.Default == nil && !p.Rest {
				req++
			}
			if p.Rest {
				max = 1 << 20
			}
		}
		if len(args) < req || len(args) > max {
			continue
		}
		score := 1
		for i, p := range f.Params {
			if i >= len(args) {
				break
			}
			t := normType(strings.TrimSuffix(strings.TrimSpace(p.Type), "&"))
			a := args[i]
			switch {
			case isFloatType(t) && a.k == kFloat, (t == "string" || t == "String" || strings.Contains(t, "char*")) && a.k == kStr:
				score += 3
			case a.k == kInt && normType(a.t) == "char" && t == "char":
				score += 3
			case a.k == kInt || a.k == kBool:
				if _, ok := cIntType(t); ok {
					score += 2
				} else if isFloatType(t) {
					score++
				}
			case a.k == kObj && strings.Contains(t, a.o.class.name):
				score += 3
			case a.k == kList && (strings.Contains(t, "[]") || strings.Contains(t, "vector") || strings.Contains(t, "List")):
				score += 2
			}
		}
		if score > bestScore {
			best, bestScore = f, score
		}
	}
	nf := *fn
	nf.decl = best
	nf.overloads = nil
	if c := it.classOf[best]; c != nil {
		nf.class = c
	}
	return &nf
}

// ctorInits menjalankan ": base(args), field(nilai)" konstruktor C++.
func (it *interp) ctorInits(decl *ir.Func, scope *Env) {
	this := *scope.this
	o := this.o
	for _, x := range decl.Inits {
		call, ok := x.(*ir.Call)
		if !ok {
			continue
		}
		id, ok := call.Fun.(*ir.Ident)
		if !ok {
			continue
		}
		args := it.evalArgs(call.Args, scope)
		if o.fields.has(strV(id.Name)) {
			typ := ""
			for _, f := range it.allFields(o.class) {
				if f.name == id.Name {
					typ = f.typ
				}
			}
			var v Value
			if len(args.pos) == 1 {
				v = it.sem.convert(it, copyFor(it, args.pos[0]), typ)
			} else {
				v = it.constructType(typ, args.pos, call)
			}
			o.fields.set(strV(id.Name), v)
			continue
		}
		for c := o.class.super; c != nil; c = c.super {
			if c.name != id.Name {
				continue
			}
			if c.native != "" {
				it.nativeInit(c, this, args.pos, call)
			} else if ctor := it.findCtor(c, len(args.pos)); ctor != nil {
				f := &Func{name: c.name, decl: ctor, env: c.env, class: c}
				if ovs := c.overloads[c.name]; len(ovs) > 1 {
					f.overloads = ovs
				}
				it.callFunc(f, this, args, call)
			}
			break
		}
	}
}

// recordSwapCall mencatat swap(a[i], a[j]) sebagai operasi tukar pada koleksi.
func (it *interp) recordSwapCall(args []ir.Expr, e *Env) {
	if it.rec.coll == "" || !it.recording() {
		return
	}
	i0, i1 := it.collIndex(args[0], e), it.collIndex(args[1], e)
	if i0 >= 0 && i1 >= 0 && i0 != i1 {
		it.rec.ops = it.rec.ops[:0]
		a, b := min(i0, i1), max(i0, i1)
		it.rec.op(trace.Op{Op: trace.OpSwap, A: a, B: &b})
	}
}

// setAggField mengisi field dari inisialisasi agregat {...}; field array diisi elemen demi
// elemen (string literal menjadi karakter + NUL).
func (it *interp) setAggField(obj Value, f fieldInit, v Value) {
	if !f.array {
		obj.o.fields.set(strV(f.name), it.sem.convert(it, v, f.typ))
		return
	}
	arr, _ := obj.o.fields.get(strV(f.name))
	if arr.k != kList {
		arr = f.init(it, obj)
	}
	switch v.k {
	case kStr:
		for i := 0; i < len(arr.l.elems); i++ {
			ch := int64(0)
			if i < len(v.s) {
				ch = int64(int8(v.s[i]))
			}
			arr.l.elems[i] = typedInt(ch, "char")
		}
	case kList:
		for i := 0; i < len(arr.l.elems) && i < len(v.l.elems); i++ {
			arr.l.elems[i] = it.sem.convert(it, v.l.elems[i], f.typ)
		}
	}
	obj.o.fields.set(strV(f.name), arr)
}
