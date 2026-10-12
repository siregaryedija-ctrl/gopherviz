package irsim

import (
	"fmt"
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/lang"
	"gopherviz/internal/trace"
)

// Trace class (Fase 10a): blok fokus = class. Yang direkam:
//   - setiap langkah di dalam method class itu (konstruktor juga), seperti trace fungsi;
//   - statement di luar class yang mengubah object-nya (object dibuat, dipegang variabel
//     baru, atau field-nya diubah langsung, mis. b.nilai = 99).
// Setiap langkah membawa potret object (Step.Objs) dan object yang menjadi this/self (Step.Self).
// Trace method (blok fokus = method) hanya memotret object self-nya.

const maxObjs = 16 // object yang digambar; sisanya hanya dihitung

// objects adalah daftar object yang digambar, urut pertama terlihat.
type objects struct {
	cls      *ir.Class // class fokus (nil = bukan trace class)
	method   bool      // fokus = method sebuah class
	list     []*Object
	id       map[*Object]string
	name     map[*Object]string
	lastSig  string
	overflow bool
	final    bool // langkah penutup: variabel lokal main tidak terjangkau, nama lama dipakai
}

func (r *recorder) objMode() bool { return r.obj.cls != nil || r.obj.method }

// isFocusClass: class c adalah class fokus atau turunannya.
func (r *recorder) isFocusClass(c *Class) bool {
	for ; c != nil; c = c.super {
		if c.decl != nil && c.decl == r.obj.cls {
			return true
		}
	}
	return false
}

// focusMethod: n adalah method (atau fungsi di dalam method) class fokus.
func (it *interp) focusMethod(n ir.Node) bool {
	if it.rec.obj.cls == nil {
		return false
	}
	_, ok := n.(*ir.Func)
	return ok && it.quiet == 0 && it.insideFocus(n)
}

// noteObj mendaftarkan object baru bila class-nya class fokus.
func (it *interp) noteObj(o *Object) {
	r := it.rec
	if r.obj.cls == nil || o == nil || !r.isFocusClass(o.class) {
		return
	}
	it.objID(o)
	r.reached = true
}

// objID memberi ID tetap; object ke-17 dst. tidak digambar ("" = tidak terdaftar).
func (it *interp) objID(o *Object) string {
	ob := &it.rec.obj
	if id, ok := ob.id[o]; ok {
		return id
	}
	if len(ob.list) >= maxObjs {
		if !ob.overflow {
			ob.overflow = true
			it.warn(fmt.Sprintf("only the first %d objects are drawn", maxObjs))
		}
		return ""
	}
	if ob.id == nil {
		ob.id, ob.name = map[*Object]string{}, map[*Object]string{}
	}
	id := fmt.Sprintf("obj%d", len(ob.list)+1)
	ob.id[o] = id
	ob.list = append(ob.list, o)
	return id
}

var selfNames = map[string]bool{"self": true, "this": true, "$this": true, "cls": true}

// objNames mencari variabel yang memegang object (langsung, lewat pointer, atau sebagai elemen
// list/dict satu tingkat) di cakupan e dan induk-induknya. Object class fokus yang belum
// terdaftar ikut didaftarkan (mis. struct C yang dibuat tanpa konstruktor).
func (it *interp) objNames(e *Env) map[*Object][]string {
	out := map[*Object][]string{}
	see := func(v Value, name string) {
		if v.k == kPtr && v.p != nil && v.p.cell != nil {
			v = v.p.cell.get()
		}
		if v.k != kObj || v.o == nil {
			return
		}
		if _, ok := it.rec.obj.id[v.o]; !ok {
			if it.rec.obj.cls == nil || !it.rec.isFocusClass(v.o.class) {
				return
			}
			it.noteObj(v.o)
		}
		out[v.o] = append(out[v.o], name)
	}
	for s := e; s != nil; s = s.parent {
		for _, name := range s.order {
			if selfNames[name] {
				continue
			}
			v := s.vars[name].get()
			switch v.k {
			case kObj, kPtr:
				see(v, name)
			case kList:
				for i, x := range v.l.elems {
					if i >= 50 {
						break
					}
					see(x, fmt.Sprintf("%s[%d]", name, i))
				}
			case kDict:
				for i, x := range v.d.vals {
					if i >= 50 {
						break
					}
					see(x, fmt.Sprintf("%s[%s]", name, it.display(v.d.keys[i], false)))
				}
			}
		}
	}
	return out
}

// nameFromCall: a.setor(5) / $a->setor(5) / p->geser() memberi nama "a" pada object this
// (variabel pemanggil sering tidak terjangkau dari dalam method, mis. lokal main di Java).
func (it *interp) nameFromCall(at ir.Node, this Value) {
	ob := &it.rec.obj
	if !it.rec.objMode() || this.k != kObj || this.o == nil {
		return
	}
	c, ok := at.(*ir.Call)
	if !ok {
		return
	}
	mb, ok := c.Fun.(*ir.Member)
	if !ok {
		return
	}
	id, ok := mb.X.(*ir.Ident)
	if !ok || selfNames[id.Name] {
		return
	}
	if ob.name == nil {
		ob.id, ob.name = map[*Object]string{}, map[*Object]string{}
	}
	ob.name[this.o] = id.Name
}

// objSnapshot memotret object yang digambar pada cakupan e.
func (it *interp) objSnapshot(e *Env) ([]trace.ObjState, string) {
	ob := &it.rec.obj
	self := ""
	var this *Object
	if e != nil {
		if t := e.thisVal(); t != nil && t.k == kObj {
			this = t.o
		}
	}
	if ob.method { // trace method: hanya object self
		if this == nil {
			return nil, ""
		}
		it.objID(this)
	}
	if e != nil {
		// nama dari variabel: di luar method selalu diperbarui; di dalam method hanya bila belum
		// punya nama (parameter lain yang kebetulan memegang object yang sama tidak menggantinya)
		outside := !it.recording() && !ob.final
		prev := map[*Object]string{}
		if outside {
			for _, o := range ob.list {
				prev[o], ob.name[o] = ob.name[o], ""
			}
		}
		for o, names := range it.objNames(e) {
			if outside || ob.name[o] == "" {
				ob.name[o] = pickName(names, prev[o])
			}
		}
		// object yang hanya terjangkau lewat field object lain: "head.next", "head.next.next"
		for changed := true; changed; {
			changed = false
			for _, o := range ob.list {
				base := ob.name[o]
				if base == "" || len(base) > 24 {
					continue
				}
				for i, k := range o.fields.keys {
					v := o.fields.vals[i]
					if v.k == kPtr && v.p != nil && v.p.cell != nil {
						v = v.p.cell.get()
					}
					if v.k == kObj && v.o != nil && ob.id[v.o] != "" && ob.name[v.o] == "" {
						ob.name[v.o] = base + "." + it.display(k, false)
						changed = true
					}
				}
			}
		}
	}
	if this != nil {
		if id, ok := ob.id[this]; ok {
			self = id
		}
	}
	out := make([]trace.ObjState, 0, len(ob.list))
	for _, o := range ob.list {
		if ob.method && o != this {
			continue
		}
		st := trace.ObjState{ID: ob.id[o], Class: o.class.name, Name: ob.name[o], Fields: []trace.MemField{}}
		for i, k := range o.fields.keys {
			v := o.fields.vals[i]
			if v.k == kFunc || v.k == kClass {
				continue
			}
			f := trace.MemField{Name: it.display(k, false)}
			if v.k == kObj && v.o != nil {
				if id, ok := ob.id[v.o]; ok {
					f.Points = id
				}
			}
			f.Value = clipText(it.display(v, false), 24)
			st.Fields = append(st.Fields, f)
		}
		out = append(out, st)
	}
	return out, self
}

// pickName: nama lama dipertahankan bila masih memegang object (label rumah tidak berganti-ganti).
func pickName(names []string, prev string) string {
	for _, n := range names {
		if n == prev {
			return n
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func objSig(objs []trace.ObjState) string {
	var b strings.Builder
	for _, o := range objs {
		b.WriteString(o.ID + "|" + o.Name)
		for _, f := range o.Fields {
			b.WriteString("|" + f.Name + "=" + f.Value + ">" + f.Points)
		}
		b.WriteByte(';')
	}
	return b.String()
}

// classWatch merekam statement di luar class fokus yang mengubah object-nya.
func (it *interp) classWatch(n ir.Node, e *Env) {
	r := it.rec
	if r.obj.cls == nil || it.recording() || it.quiet > 0 {
		return
	}
	it.quiet++
	objs, _ := it.objSnapshot(e)
	it.quiet--
	if len(objs) == 0 {
		return
	}
	sig := objSig(objs)
	if sig == r.obj.lastSig {
		return
	}
	r.reached = true
	it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(n), Note: strings.TrimSuffix(it.text(n), ";")}, e)
}

// classInfo membaca cetak biru class dari kode: method, field yang dideklarasikan atau diisi
// lewat self.x / this.x / $this->x, class induk, dan class anak di file yang sama.
func classInfo(res *lang.Result, c *ir.Class) *trace.ClassInfo {
	info := &trace.ClassInfo{Name: c.Name}
	if len(c.Extends) > 0 {
		info.Super = c.Extends[0]
	}
	seen := map[string]bool{}
	field := func(name string) {
		name = strings.TrimPrefix(name, "$")
		if name != "" && !seen[name] {
			seen[name] = true
			info.Fields = append(info.Fields, name)
		}
	}
	// field warisan dulu (class induk di file yang sama), lalu field class ini
	var chain []*ir.Class
	for k, depth := c, 0; k != nil && depth < 8; depth++ {
		chain = append([]*ir.Class{k}, chain...)
		if len(k.Extends) == 0 {
			break
		}
		k = findClassDecl(res, k.Extends[0])
	}
	for _, k := range chain {
		classFields(res, k, field)
	}
	for _, m := range c.Members {
		if x, ok := m.(*ir.Func); ok && x.Name != "" && !contains(info.Methods, x.Name) {
			info.Methods = append(info.Methods, x.Name)
		}
	}
	if res.File != nil {
		for _, s := range res.File.Body {
			ir.Walk(s, func(n ir.Node) bool {
				if k, ok := n.(*ir.Class); ok && k != c && len(k.Extends) > 0 && k.Extends[0] == c.Name {
					info.Subs = append(info.Subs, k.Name)
				}
				return true
			})
		}
	}
	return info
}

// findClassDecl mencari deklarasi class bernama name di file.
func findClassDecl(res *lang.Result, name string) *ir.Class {
	var found *ir.Class
	if res.File == nil {
		return nil
	}
	for _, s := range res.File.Body {
		ir.Walk(s, func(n ir.Node) bool {
			if k, ok := n.(*ir.Class); ok && found == nil && k.Name == name {
				found = k
			}
			return found == nil
		})
	}
	return found
}

// classFields memanggil field untuk setiap field object yang terlihat di kode class c.
func classFields(res *lang.Result, c *ir.Class, field func(string)) {
	for _, m := range c.Members {
		switch x := m.(type) {
		case *ir.VarDecl:
			if res.Scene.Language == "python" || x.Kind == "static" {
				continue // atribut class Python / field static bukan milik object
			}
			for _, s := range x.Specs {
				field(s.Name)
			}
		case *ir.Func:
			for _, p := range x.Params {
				if p.Promote {
					field(p.Name)
				}
			}
			ir.Walk(x, func(n ir.Node) bool {
				if a, ok := n.(*ir.Assign); ok {
					if mb, ok := a.L.(*ir.Member); ok {
						switch x := mb.X.(type) {
						case *ir.Ident:
							if selfNames[x.Name] {
								field(mb.Name)
							}
						case *ir.This:
							if x.Name == "this" {
								field(mb.Name)
							}
						}
					}
				}
				return true
			})
		}
	}
}

// classFinish menutup trace class dengan langkah "program finished" (membawa output terakhir).
func (it *interp) classFinish() {
	r := it.rec
	if r.obj.cls == nil || !r.reached || r.truncated {
		return
	}
	defer func() { recover() }() // batas langkah tercapai: abaikan
	r.obj.final = true
	it.add(trace.Step{Phase: trace.PhaseExit, Reason: trace.ReasonDone, Note: "program finished"}, it.globals)
}

// classOfMethod mencari class (IR) yang memuat method fn.
func classOfMethod(res *lang.Result, fn *ir.Func) *ir.Class {
	if res.File == nil || fn.Class == "" {
		return nil
	}
	var found *ir.Class
	for _, s := range res.File.Body {
		ir.Walk(s, func(n ir.Node) bool {
			if k, ok := n.(*ir.Class); ok && found == nil {
				for _, m := range k.Members {
					if m == ir.Stmt(fn) {
						found = k
					}
				}
			}
			return found == nil
		})
	}
	return found
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
