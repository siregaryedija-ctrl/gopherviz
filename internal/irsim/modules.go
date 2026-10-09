package irsim

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/lang"
	"gopherviz/internal/project"
)

// Program yang dipecah ke beberapa file (Fase 10b). Setiap file = satu modul: IR-nya sendiri,
// sumbernya sendiri (untuk teks catatan), dan untuk Python/JS cakupan global sendiri.
// File lain hanya DIBACA lewat Options.Files (buffer editor dulu, lalu disk), tidak dijalankan
// sebagai program: simulator menjalankan isinya sendiri seperti file utama.
//
//	Python      import x / from x import y        → x.py, x/__init__.py (relatif folder modul pemanggil)
//	JS/TS       import ... from './x', require('./x') → x, x.js, x.ts, ..., x/index.js
//	PHP         require / include (+ _once)        → file disebut, dijalankan di cakupan pemanggil
//	C/C++       #include "x.h"                     → x.h lalu pasangannya x.c / x.cpp
//	Java        class lain di folder yang sama      → semua *.java di folder (seperti satu package)
//
// Setiap langkah mencatat file asalnya (Step.File, "" = file fokus).

const maxModules = 24

type module struct {
	path string       // path absolut
	name string       // relatif terhadap folder file fokus ("utils.py"); "" = file fokus
	res  *lang.Result // IR + indeks blok
	env  *Env         // cakupan global modul (Python/JS); nil = it.globals
	val  Value        // Python: dict modul; JS: module.exports
	def  *Value       // JS: export default
	ran  bool
}

func pathKey(p string) string { return strings.ToLower(filepath.Clean(p)) }

// newModules menyiapkan modul file fokus.
func (it *interp) newModules(files project.Files) {
	it.files = files
	it.mods = map[string]*module{}
	it.declMod = map[ir.Node]*module{}
	it.focusMod = it.register(it.res.Scene.File, "", it.res)
	it.cur = it.focusMod
}

// register mencatat modul dan deklarasi di dalamnya (fungsi & class → modulnya).
func (it *interp) register(path, name string, res *lang.Result) *module {
	m := &module{path: path, name: name, res: res}
	it.mods[pathKey(path)] = m
	it.modOrder = append(it.modOrder, m)
	if res.File != nil {
		for _, s := range res.File.Body {
			ir.Walk(s, func(n ir.Node) bool {
				switch n.(type) {
				case *ir.Func, *ir.Class:
					it.declMod[n] = m
				}
				return true
			})
		}
	}
	return m
}

// loadModule membaca dan menganalisis file path (sekali per trace). nil bila tidak ada atau error.
func (it *interp) loadModule(path string) *module {
	if m, ok := it.mods[pathKey(path)]; ok {
		return m
	}
	if it.files == nil {
		return nil
	}
	if len(it.mods) >= maxModules {
		it.warn(fmt.Sprintf("hanya %d file pertama yang ikut disimulasikan", maxModules))
		return nil
	}
	text, ok := it.files.Read(path)
	if !ok {
		return nil
	}
	name := project.Rel(filepath.Dir(it.focusMod.path), path)
	res := lang.Analyze(it.lang, path, text)
	if len(res.Diagnostics) > 0 {
		it.warn(fmt.Sprintf("%s belum lengkap atau masih ada error (baris %d), jadi isinya tidak ikut disimulasikan", name, res.Diagnostics[0].Line))
		return nil
	}
	return it.register(path, name, res)
}

// setCur memindahkan "file yang sedang berjalan" ke m; kembaliannya memulihkan yang lama.
func (it *interp) setCur(m *module) func() {
	if m == nil || m == it.cur {
		return func() {}
	}
	prev, prevSrc := it.cur, it.src
	it.cur, it.src = m, m.res.Src
	return func() { it.cur, it.src = prev, prevSrc }
}

// modOf: modul tempat deklarasi fungsi/class n ditulis (nil = tidak diketahui).
func (it *interp) modOf(n ir.Node) *module {
	if it.declMod == nil || n == nil {
		return nil
	}
	return it.declMod[n]
}

func (it *interp) curDir() string { return filepath.Dir(it.cur.path) }

// root: cakupan global terdekat (modul Python/JS punya global sendiri).
func (e *Env) root() *Env {
	s := e
	for s.parent != nil {
		s = s.parent
	}
	return s
}

// traceFiles: file yang ikut berjalan (file fokus dulu, lalu urutan kemunculan di langkah,
// lalu file awal program). nil bila hanya satu file.
func (it *interp) traceFiles(entry string) []string {
	files := []string{filepath.Base(it.focusMod.path)}
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
		return nil
	}
	return files
}

// ---------- Python ----------

// pyUserModule: import x → x.py atau x/__init__.py di folder modul pemanggil.
func (it *interp) pyUserModule(name string) (Value, bool) {
	if it.files == nil || name == "" || strings.HasPrefix(name, ".") {
		return Value{}, false
	}
	rel := strings.ReplaceAll(name, ".", "/")
	for _, cand := range []string{rel + ".py", rel + "/__init__.py"} {
		if m := it.loadModule(filepath.Join(it.curDir(), filepath.FromSlash(cand))); m != nil {
			return it.pyRunModule(m, name), true
		}
	}
	return Value{}, false
}

// pyRunModule menjalankan isi modul sekali (dengan __name__ = nama modul, bukan "__main__")
// dan mengembalikan dict modul berisi fungsi, class, dan variabelnya.
func (it *interp) pyRunModule(m *module, name string) Value {
	if m.ran {
		return m.val // sudah (atau sedang) dijalankan: import melingkar memakai isi sejauh ini
	}
	m.ran = true
	m.env = newEnv(nil, true)
	m.env.global = true
	m.env.define("__name__", strV(name))
	m.val = Value{k: kDict, d: it.newDict("module"), t: name}
	defer it.setCur(m)()
	for _, s := range m.res.File.Body {
		if f := it.exec(s, m.env); f.k == fReturn {
			break
		}
	}
	it.syncModule(m)
	return m.val
}

// syncModule menyalin variabel global modul ke dict-nya (setelah modul atau fungsinya berjalan).
func (it *interp) syncModule(m *module) {
	if m == nil || m.env == nil || m.val.k != kDict || it.lang != "python" {
		return
	}
	for _, name := range m.env.order {
		if name == "__name__" {
			continue
		}
		m.val.d.set(strV(name), m.env.vars[name].get())
	}
}

// ---------- JavaScript / TypeScript ----------

var (
	jsImportFrom = regexp.MustCompile(`^import\s+(.+?)\s+from\s+['"]([^'"]+)['"]`)
	jsImportBare = regexp.MustCompile(`^import\s+['"]([^'"]+)['"]`)
	jsExportList = regexp.MustCompile(`(?m)^\s*export\s*\{([^}]*)\}\s*;?\s*$`)
	jsExportDef  = regexp.MustCompile(`(?m)^\s*export\s+default\s+(?:async\s+)?(?:function\s*\*?|class)\s+([A-Za-z_$][\w$]*)`)
)

func relativeSpec(spec string) bool {
	return strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")
}

// jsResolve mencari file untuk './x': x, x.js, x.ts, ..., x/index.js.
func (it *interp) jsResolve(spec string) *module {
	base := filepath.Join(it.curDir(), filepath.FromSlash(spec))
	cands := []string{base}
	for _, ext := range []string{".js", ".ts", ".mjs", ".cjs", ".jsx", ".tsx"} {
		cands = append(cands, base+ext)
	}
	for _, ext := range []string{".js", ".ts"} {
		cands = append(cands, filepath.Join(base, "index"+ext))
	}
	for _, c := range cands {
		if m := it.loadModule(c); m != nil {
			return m
		}
	}
	return nil
}

// jsRunModule menjalankan modul sekali dan mengembalikan module.exports.
func (it *interp) jsRunModule(m *module) Value {
	if m.ran {
		return it.jsExports(m)
	}
	m.ran = true
	m.env = newEnv(nil, true)
	m.env.global = true
	exports := it.dictV("object")
	modObj := it.dictV("object")
	modObj.d.set(strV("exports"), exports)
	m.env.define("module", modObj)
	m.env.define("exports", exports)
	defer it.setCur(m)()
	body := m.res.File.Body
	it.hoist(body, m.env)
	for _, s := range body {
		if x, ok := s.(*ir.ExprStmt); ok && strings.HasPrefix(it.text(s), "export default") {
			v := it.eval(x.X, m.env)
			m.def = &v
			continue
		}
		it.exec(s, m.env)
	}
	if mm := jsExportDef.FindStringSubmatch(m.res.Src); mm != nil && m.def == nil {
		if c := m.env.lookup(mm[1]); c != nil {
			v := c.get()
			m.def = &v
		}
	}
	return it.jsExports(m)
}

func (it *interp) jsExports(m *module) Value {
	if c := m.env.vars["module"]; c != nil && c.get().k == kDict {
		if v, ok := c.get().d.get(strV("exports")); ok {
			return v
		}
	}
	return undef()
}

// jsExportName: nama lokal untuk nama ekspor (export { lokal as ekspor }).
func jsExportName(src, name string) string {
	for _, mm := range jsExportList.FindAllStringSubmatch(src, -1) {
		for _, part := range strings.Split(mm[1], ",") {
			local, exp, ok := strings.Cut(strings.TrimSpace(part), " as ")
			if ok && strings.TrimSpace(exp) == name {
				return strings.TrimSpace(local)
			}
		}
	}
	return name
}

// jsMember mengambil nama yang diekspor modul: variabel/fungsi/class tingkat atas, lalu module.exports.
func (it *interp) jsMember(m *module, name string) (Value, bool) {
	if c := m.env.vars[jsExportName(m.res.Src, name)]; c != nil {
		return c.get(), true
	}
	if ex := it.jsExports(m); ex.k == kDict {
		if v, ok := ex.d.get(strV(name)); ok {
			return v, true
		}
	}
	return Value{}, false
}

// jsImport menjalankan import ... from './x' dan mengikat nama-namanya di e.
func (it *interp) jsImport(n *ir.Simple, e *Env) {
	text := strings.Join(strings.Fields(it.text(n)), " ")
	if strings.HasPrefix(text, "import type ") {
		return
	}
	clause, spec := "", ""
	if mm := jsImportFrom.FindStringSubmatch(text); mm != nil {
		clause, spec = mm[1], mm[2]
	} else if mm := jsImportBare.FindStringSubmatch(text); mm != nil {
		spec = mm[1]
	}
	if !relativeSpec(spec) {
		return
	}
	m := it.jsResolve(spec)
	if m == nil {
		it.warn("file " + spec + " tidak ditemukan, jadi isinya tidak bisa disimulasikan")
		return
	}
	it.jsRunModule(m)
	bind := func(name string, v Value) { e.define(name, v) }
	clause = strings.TrimSpace(clause)
	// default lebih dulu: import X, { a } from / import X, * as ns from
	if clause != "" && !strings.HasPrefix(clause, "{") && !strings.HasPrefix(clause, "*") {
		def, rest, _ := strings.Cut(clause, ",")
		switch {
		case m.def != nil:
			bind(strings.TrimSpace(def), *m.def)
		default:
			bind(strings.TrimSpace(def), it.jsExports(m))
		}
		clause = strings.TrimSpace(rest)
	}
	switch {
	case strings.HasPrefix(clause, "*"):
		ns := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(clause, "*")), "as"))
		obj := it.dictV("object")
		for _, name := range m.env.order {
			if name != "module" && name != "exports" {
				obj.d.set(strV(name), m.env.vars[name].get())
			}
		}
		if ex := it.jsExports(m); ex.k == kDict {
			for i, k := range ex.d.keys {
				obj.d.set(k, ex.d.vals[i])
			}
		}
		if m.def != nil {
			obj.d.set(strV("default"), *m.def)
		}
		bind(ns, obj)
	case strings.HasPrefix(clause, "{"):
		for _, part := range strings.Split(strings.Trim(clause, "{} "), ",") {
			orig, alias, has := strings.Cut(strings.TrimSpace(part), " as ")
			orig = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(orig), "type "))
			if orig == "" {
				continue
			}
			if !has {
				alias = orig
			}
			v, ok := it.jsMember(m, orig)
			if orig == "default" && m.def != nil {
				v, ok = *m.def, true
			}
			if !ok {
				it.throw(n, "SyntaxError", fmt.Sprintf("The requested module '%s' does not provide an export named '%s'", spec, orig))
			}
			bind(strings.TrimSpace(alias), v)
		}
	}
}

// jsRequire: require('./x') → module.exports modul itu (CommonJS).
func (it *interp) jsRequire(spec string, at ir.Node) (Value, bool) {
	if !relativeSpec(spec) || it.files == nil {
		return Value{}, false
	}
	m := it.jsResolve(spec)
	if m == nil {
		it.throw(at, "Error", "Cannot find module '"+spec+"'")
	}
	ex := it.jsRunModule(m)
	if m.def != nil && (ex.k != kDict || len(ex.d.keys) == 0) {
		return *m.def, true
	}
	return ex, true
}

// ---------- PHP ----------

var phpIncRe = regexp.MustCompile(`^(require|include)(_once)?\b`)
var quoted = regexp.MustCompile(`['"]([^'"]+)['"]`)

// phpInclude: require 'x.php' menjalankan isi x.php di cakupan pemanggil (sekali per file).
func (it *interp) phpInclude(n *ir.Simple, e *Env) bool {
	text := strings.TrimSpace(it.text(n))
	if !phpIncRe.MatchString(text) {
		return false
	}
	qs := quoted.FindAllStringSubmatch(text, -1)
	if len(qs) == 0 || it.files == nil {
		return true
	}
	rel := strings.TrimPrefix(qs[len(qs)-1][1], "/")
	m := it.loadModule(filepath.Join(it.curDir(), filepath.FromSlash(rel)))
	if m == nil {
		if !strings.HasPrefix(text, "include") {
			it.throw(n, "Error", "Failed opening required '"+rel+"'")
		}
		return true
	}
	if m.ran {
		return true
	}
	m.ran = true
	defer it.setCur(m)()
	it.hoistPHP(m.res.File.Body)
	for _, s := range m.res.File.Body {
		if f := it.exec(s, e); f.k == fReturn {
			break
		}
	}
	return true
}

// ---------- C / C++ ----------

var cIncRe = regexp.MustCompile(`^#?\s*include\s*"([^"]+)"`)

// cInclude: #include "x.h" mendaftarkan isi x.h dan pasangannya x.c / x.cpp (sekali per file).
func (it *interp) cInclude(line string, e *Env) bool {
	mm := cIncRe.FindStringSubmatch(strings.TrimSpace(line))
	if mm == nil {
		return false
	}
	if it.files == nil {
		return true
	}
	h := it.loadModule(filepath.Join(it.curDir(), filepath.FromSlash(mm[1])))
	if h == nil {
		return true
	}
	it.cDeclareModule(h, e)
	stem := strings.TrimSuffix(h.path, filepath.Ext(h.path))
	for _, ext := range []string{".c", ".cpp", ".cc", ".cxx"} {
		if pathKey(stem+ext) == pathKey(it.cur.path) {
			continue
		}
		if m := it.loadModule(stem + ext); m != nil {
			it.cDeclareModule(m, e)
			break
		}
	}
	return true
}

func (it *interp) cDeclareModule(m *module, e *Env) {
	if m.ran {
		return
	}
	m.ran = true
	defer it.setCur(m)()
	it.declareC(m.res.File.Body, e)
}

// ---------- Java ----------

// javaSiblings mendaftarkan class dari file .java lain di folder yang sama (satu package),
// hanya file yang class-nya disebut oleh file yang sudah dimuat (Nama.java untuk "Nama").
func (it *interp) javaSiblings() {
	if it.files == nil {
		return
	}
	texts := []string{it.cur.res.Src}
	cands := map[string]string{} // nama class → path
	for _, p := range it.files.List(it.curDir()) {
		if strings.EqualFold(filepath.Ext(p), ".java") && pathKey(p) != pathKey(it.cur.path) {
			cands[strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))] = p
		}
	}
	for len(texts) > 0 && len(cands) > 0 {
		src := texts[0]
		texts = texts[1:]
		for name, p := range cands {
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(src) {
				continue
			}
			delete(cands, name)
			m := it.loadModule(p)
			if m == nil || m.ran {
				continue
			}
			m.ran = true
			texts = append(texts, m.res.Src)
			restore := it.setCur(m)
			for _, s := range m.res.File.Body {
				if c, ok := s.(*ir.Class); ok && it.classes[c.Name] == nil {
					it.defineClass(c, it.globals)
				}
			}
			restore()
		}
	}
}

// ---------- file yang meng-import ----------

// importers mencari file lain di folder yang sama yang memakai file fokus (untuk menjalankan
// program dari sana bila blok fokus tidak tercapai dari file fokus sendiri). Urut: main.* dulu.
func importers(res *lang.Result, files project.Files) []string {
	if files == nil {
		return nil
	}
	path := res.Scene.File
	dir, base := filepath.Dir(path), filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	q := regexp.QuoteMeta(stem)
	var re *regexp.Regexp
	var exts []string
	needMain := ""
	switch res.Scene.Language {
	case "python":
		exts = []string{".py"}
		re = regexp.MustCompile(`(?m)^\s*(?:from\s+(?:\.\s*)?` + q + `\s+import\b|import\s+[^\n]*\b` + q + `\b)`)
	case "javascript", "typescript":
		exts = []string{".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx"}
		re = regexp.MustCompile(`['"]\.{1,2}/(?:[^'"]*/)?` + q + `(?:\.\w+)?['"]`)
	case "php":
		exts = []string{".php"}
		re = regexp.MustCompile(`(?:require|include)(?:_once)?[^;]*['"/]` + regexp.QuoteMeta(base) + `['"]`)
	case "c", "cpp":
		exts = []string{".c", ".cpp", ".cc", ".cxx"}
		re = regexp.MustCompile(`#\s*include\s*"(?:[^"]*/)?` + q + `\.(?:h|hpp|hh)"`)
		needMain = "main"
	case "java":
		exts = []string{".java"}
		re = regexp.MustCompile(`\b` + q + `\b`)
		needMain = "static void main"
	default:
		return nil
	}
	var out []string
	for _, p := range files.List(dir) {
		if pathKey(p) == pathKey(path) || !hasExt(p, exts) {
			continue
		}
		text, ok := files.Read(p)
		if !ok || !re.MatchString(text) || (needMain != "" && !strings.Contains(text, needMain)) {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		mi := strings.HasPrefix(strings.ToLower(filepath.Base(out[i])), "main")
		mj := strings.HasPrefix(strings.ToLower(filepath.Base(out[j])), "main")
		return mi && !mj
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func hasExt(p string, exts []string) bool {
	e := strings.ToLower(filepath.Ext(p))
	for _, x := range exts {
		if e == x {
			return true
		}
	}
	return false
}
