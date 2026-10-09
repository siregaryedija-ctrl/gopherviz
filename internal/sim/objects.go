package sim

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"gopherviz/internal/analyzer"
	"gopherviz/internal/trace"
)

// Trace class Go (Fase 10a): blok fokus = type X struct. Method-methodnya (receiver X / *X)
// direkam seperti trace fungsi, statement di luar yang mengubah struct X ikut direkam, dan setiap
// langkah membawa potret struct X (Step.Objs) beserta struct receiver-nya (Step.Self).
// Identitas object = struct di memori: x := y menyalin struct, jadi menjadi object (rumah) baru,
// sedangkan p := &x menunjuk rumah yang sama. Method dengan value receiver bekerja pada salinan.

const maxObjs = 16

type objects struct {
	typ       string          // nama tipe struct fokus ("" = bukan trace class)
	className string          // nama struct object yang digambar (trace class atau method)
	methods   []*ast.FuncDecl // method tipe fokus (trace class)
	method    bool            // fokus = method sebuah struct
	list      []*structVal
	id        map[*structVal]string
	name      map[*structVal]string
	alias     map[*structVal]*structVal // salinan value receiver → struct asal
	recv      []value                   // receiver method yang sedang berjalan (tumpukan)
	lastSig   string
	overflow  bool
	final     bool // langkah penutup: variabel lokal main tidak terjangkau, nama lama dipakai
}

func (r *recorder) objMode() bool { return r.obj.typ != "" || r.obj.method }

// focusMethod: n adalah method tipe fokus.
func (it *interp) focusMethod(n ast.Node) bool {
	fd, ok := n.(*ast.FuncDecl)
	return ok && it.rec.obj.typ != "" && fd.Recv != nil && recvTypeName(fd) == it.rec.obj.typ
}

// inFocusMethod: n berada di dalam salah satu method tipe fokus.
func (it *interp) inFocusMethod(n ast.Node) bool {
	for _, m := range it.rec.obj.methods {
		if m.Pos() <= n.Pos() && n.End() <= m.End() {
			return true
		}
	}
	return false
}

func (it *interp) objID(st *structVal) string {
	ob := &it.rec.obj
	if id, ok := ob.id[st]; ok {
		return id
	}
	if len(ob.list) >= maxObjs {
		if !ob.overflow {
			ob.overflow = true
			it.warn(fmt.Sprintf("hanya %d object pertama yang digambar", maxObjs))
		}
		return ""
	}
	if ob.id == nil {
		ob.id, ob.name = map[*structVal]string{}, map[*structVal]string{}
	}
	id := fmt.Sprintf("obj%d", len(ob.list)+1)
	ob.id[st] = id
	ob.list = append(ob.list, st)
	it.rec.reached = true
	return id
}

// structOf: struct tipe fokus di v (langsung atau lewat pointer), atau nil.
func (it *interp) structOf(v value) *structVal {
	if v.k == kPtr && v.ptr != nil {
		v = *v.ptr
	}
	if v.k != kStruct || v.st == nil {
		return nil
	}
	if it.rec.obj.typ != "" && v.tname != it.rec.obj.typ {
		return nil
	}
	return v.st
}

// noteRecv dipanggil saat method dipanggil: value receiver = salinan dari struct asal.
func (it *interp) noteRecv(orig, recv value, at ast.Node) {
	ob := &it.rec.obj
	if !it.rec.objMode() {
		return
	}
	o, c := it.structOf(orig), it.structOf(recv)
	if o != nil && c != nil && o != c {
		if ob.alias == nil {
			ob.alias = map[*structVal]*structVal{}
		}
		ob.alias[c] = o
	}
	// x.Setor(5): object ini bernama "x" (variabel pemanggil tidak terjangkau dari dalam method)
	if sel, ok := at.(*ast.SelectorExpr); ok && o != nil {
		if id, ok := sel.X.(*ast.Ident); ok {
			if ob.name == nil {
				ob.id, ob.name = map[*structVal]string{}, map[*structVal]string{}
			}
			ob.name[o] = id.Name
		}
	}
}

// objNames mencari variabel yang memegang struct tipe fokus (langsung, pointer, atau elemen
// slice/array/map satu tingkat) dan mendaftarkannya.
func (it *interp) objNames(e *env, register bool) map[*structVal][]string {
	out := map[*structVal][]string{}
	see := func(v value, name string) {
		st := it.structOf(v)
		if st == nil || it.rec.obj.alias[st] != nil {
			return
		}
		if _, ok := it.rec.obj.id[st]; !ok {
			if !register || it.rec.obj.typ == "" {
				return
			}
			if it.objID(st) == "" {
				return
			}
		}
		out[st] = append(out[st], name)
	}
	for s := e; s != nil; s = s.parent {
		for _, name := range s.order {
			v := *s.vars[name]
			switch v.k {
			case kStruct, kPtr:
				see(v, name)
			case kSlice:
				for i, x := range v.sl.elems() {
					if i >= 50 {
						break
					}
					see(x, fmt.Sprintf("%s[%d]", name, i))
				}
			case kArray:
				for i, x := range v.arr {
					if i >= 50 {
						break
					}
					see(x, fmt.Sprintf("%s[%d]", name, i))
				}
			case kMap:
				if v.m != nil {
					for i, k := range v.m.keys {
						if i >= 50 {
							break
						}
						x, _ := v.m.get(k)
						see(x, fmt.Sprintf("%s[%s]", name, format(k)))
					}
				}
			}
		}
	}
	return out
}

// objSnapshot memotret object yang digambar; selfCopy = receiver method saat ini adalah salinan.
func (it *interp) objSnapshot(e *env) (objs []trace.ObjState, self string, selfCopy bool) {
	ob := &it.rec.obj
	var cur *structVal
	if n := len(ob.recv); n > 0 {
		cur = it.structOf(ob.recv[n-1])
	}
	var copyOf *structVal
	if cur != nil && ob.alias[cur] != nil {
		copyOf, selfCopy = ob.alias[cur], true
	}
	if ob.method {
		if cur == nil {
			return nil, "", false
		}
		if copyOf != nil {
			it.objID(copyOf)
		} else {
			it.objID(cur)
		}
	}
	if e != nil {
		// di luar method nama dihitung ulang (variabel bisa pindah ke object lain); di dalam method
		// variabel pemanggil sering tidak terjangkau, jadi nama lama dipertahankan
		outside := !it.recording() && !ob.final
		prev := map[*structVal]string{}
		if outside {
			for _, st := range ob.list {
				prev[st], ob.name[st] = ob.name[st], ""
			}
		}
		for st, names := range it.objNames(e, ob.typ != "") {
			if outside || ob.name[st] == "" {
				ob.name[st] = pickName(names, prev[st])
			}
		}
		// object yang hanya terjangkau lewat field object lain: "head.Next", "head.Next.Next"
		for changed := true; changed; {
			changed = false
			for _, st := range ob.list {
				base := ob.name[st]
				if base == "" || len(base) > 24 {
					continue
				}
				for i, f := range st.fields {
					if x := it.structOf(f); x != nil && ob.id[x] != "" && ob.name[x] == "" {
						ob.name[x] = base + "." + st.names[i]
						changed = true
					}
				}
			}
		}
	}
	target := cur
	if copyOf != nil {
		target = copyOf
	}
	if target != nil {
		self = ob.id[target]
	}
	for _, st := range ob.list {
		if ob.method && st != target {
			continue
		}
		src := st
		if st == copyOf {
			src = cur // tampilkan isi salinan yang sedang diubah method
		}
		o := trace.ObjState{ID: ob.id[st], Class: ob.className, Name: ob.name[st], Fields: []trace.MemField{}}
		for i, n := range src.names {
			f := trace.MemField{Name: n}
			if x := it.structOf(src.fields[i]); x != nil {
				if id, ok := ob.id[x]; ok {
					f.Points = id
				}
			}
			txt := show(src.fields[i])
			if src.fields[i].k == kString {
				txt = src.fields[i].s // tanpa tanda kutip, sama dengan bahasa lain
			}
			f.Value = clip(txt, 24)
			o.Fields = append(o.Fields, f)
		}
		objs = append(objs, o)
	}
	return objs, self, selfCopy
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

func clip(s string, n int) string {
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

// classWatch merekam statement di luar method yang mengubah struct tipe fokus.
func (it *interp) classWatch(n ast.Node, e *env) {
	r := it.rec
	if r.obj.typ == "" || it.recording() || it.quiet > 0 {
		return
	}
	objs, _, _ := it.objSnapshot(e)
	if len(objs) == 0 {
		return
	}
	if objSig(objs) == r.obj.lastSig {
		return
	}
	r.reached = true
	it.add(trace.Step{Phase: trace.PhaseBody, Line: it.line(n), Note: it.text(n)}, e)
}

// classFinish menutup trace class dengan langkah "program selesai".
func (it *interp) classFinish() {
	r := it.rec
	if r.obj.typ == "" || !r.reached || r.truncated {
		return
	}
	defer func() { recover() }()
	r.obj.final = true
	it.add(trace.Step{Phase: trace.PhaseExit, Reason: trace.ReasonDone, Note: "program selesai"}, it.globals)
}

// classInfo: cetak biru struct name: field, method (urut sumber), struct tertanam (induk),
// dan struct lain yang menanamkan struct ini (anak).
func classInfo(files []*ast.File, name string) *trace.ClassInfo {
	info := &trace.ClassInfo{Name: name}
	inspect := func(fn func(ast.Node) bool) {
		for _, f := range files {
			ast.Inspect(f, fn)
		}
	}
	inspect(func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			return true
		}
		for _, f := range st.Fields.List {
			embedded := ""
			if len(f.Names) == 0 {
				t := f.Type
				if s, ok := t.(*ast.StarExpr); ok {
					t = s.X
				}
				if id, ok := t.(*ast.Ident); ok {
					embedded = id.Name
				}
			}
			if ts.Name.Name == name {
				if embedded != "" {
					if info.Super == "" {
						info.Super = embedded
					}
					info.Fields = append(info.Fields, embedded)
				}
				for _, n := range f.Names {
					info.Fields = append(info.Fields, n.Name)
				}
			} else if embedded == name {
				info.Subs = append(info.Subs, ts.Name.Name)
			}
		}
		return true
	})
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil && recvTypeName(fd) == name {
				info.Methods = append(info.Methods, fd.Name.Name)
			}
		}
	}
	return info
}

// focusMethods: semua method tipe name di file.
func focusMethods(files []*ast.File, name string) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil && fd.Body != nil && recvTypeName(fd) == name {
				out = append(out, fd)
			}
		}
	}
	return out
}

// ---------- program multi-file (Fase 10b) ----------

// allDecls: deklarasi tingkat atas semua file di package (file fokus dulu).
func allDecls(p *analyzer.Program) []ast.Decl {
	var out []ast.Decl
	for _, f := range p.Files() {
		out = append(out, f.Decls...)
	}
	return out
}

// enterFile memindahkan "file yang sedang berjalan" ke file tempat pos berada; kembaliannya
// memulihkan file sebelumnya.
func (it *interp) enterFile(pos token.Pos) func() {
	name := it.p.FileOf(pos)
	if name == it.curFile {
		return func() {}
	}
	prev := it.curFile
	it.curFile = name
	return func() { it.curFile = prev }
}

// traceFiles: file program yang ikut berjalan (file fokus dulu) dan file tempat main berada
// bila bukan file fokus. files nil bila hanya satu file.
func (it *interp) traceFiles() (entry string, files []string) {
	if len(it.p.Siblings) == 0 {
		return "", nil
	}
	if m := it.funcs["main"]; m != nil {
		entry = it.p.FileOf(m.Pos())
	}
	files = []string{filepath.Base(it.p.Result.Scene.File)}
	seen := map[string]bool{"": true}
	for _, s := range it.rec.steps {
		if !seen[s.File] {
			seen[s.File] = true
			files = append(files, s.File)
		}
	}
	if entry != "" && !seen[entry] {
		files = append(files, entry)
	}
	if len(files) < 2 {
		files = nil
	}
	return entry, files
}
