package irsim

import (
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/race"
)

// Lomba algoritma (Fase 12c): fungsi fokus dipanggil langsung dengan list acak (race.Spec).
// Definisi program (fungsi, class, import, variabel global) dijalankan dulu tanpa direkam,
// kode lain di tingkat atas (pemanggilan, print, if __name__ == "__main__") dilewati.

// runRace menjalankan definisi lalu memanggil fn. it.work = jumlah evaluasi selama pemanggilan.
func (it *interp) runRace(fn *ir.Func) {
	it.rec.hold++
	it.runPrelude()
	it.rec.hold--
	if it.raceSpec.Count {
		it.rec.hold++
		defer func() { it.rec.hold-- }()
	}
	f := &Func{name: fn.Name, decl: fn, env: it.globals, class: it.classOf[fn], static: fn.Static}
	this := undef()
	if c := it.classOf[fn]; c != nil && !fn.Static {
		it.ids++
		o := &Object{class: c, fields: it.newDict("fields"), id: it.ids}
		this = Value{k: kObj, o: o}
		func() {
			defer func() { recover() }()
			it.initFields(c, this)
		}()
	}
	args := it.raceArgs(fn)
	it.budget = it.budget0
	defer func() { it.work = it.budget0 - it.budget }()
	it.callFunc(f, this, argList{pos: args}, fn)
}

// runPrelude menyiapkan definisi program tanpa menjalankan program utamanya.
func (it *interp) runPrelude() {
	entry := it.entry
	if entry == nil {
		entry = it.focusMod
	}
	defer it.setCur(entry)()
	body := entry.res.File.Body
	switch it.lang {
	case "javascript", "typescript":
		it.hoist(body, it.globals)
		it.execDefs(body)
	case "python":
		it.execDefs(body)
	case "php":
		it.hoistPHP(body)
		it.execDefs(body)
	case "c", "cpp":
		it.declareC(body, it.globals)
	case "java":
		it.javaSiblings()
		for _, s := range body {
			if c, ok := s.(*ir.Class); ok {
				it.defineClass(c, it.globals)
			}
		}
	}
}

// execDefs menjalankan statement tingkat atas yang hanya mendefinisikan sesuatu.
func (it *interp) execDefs(body []ir.Stmt) {
	for _, s := range body {
		switch x := s.(type) {
		case *ir.Func, *ir.Class, *ir.VarDecl, *ir.Simple:
			it.exec(s, it.globals)
		case *ir.ExprStmt:
			if _, ok := x.X.(*ir.Assign); ok {
				it.exec(s, it.globals)
			}
		}
	}
}

// raceArgs mengisi parameter fn: list acak dan batas/panjangnya.
func (it *interp) raceArgs(fn *ir.Func) []Value {
	n := it.raceSpec.N
	nums := race.List(n, it.raceSpec.Seed)
	var args []Value
	haveList := false
	for _, p := range fn.Params {
		if p.Name == "self" && it.lang == "python" {
			continue
		}
		role := race.RoleOf(p.Name, p.Type, haveList)
		if role == race.RoleList {
			haveList = true
			args = append(args, it.raceList(nums, p.Type))
			continue
		}
		args = append(args, it.raceInt(race.Int(role, n)))
	}
	return args
}

func (it *interp) raceInt(v int) Value {
	switch it.lang {
	case "c", "cpp", "java":
		return typedInt(int64(v), "int")
	}
	return intV(int64(v))
}

func (it *interp) raceList(nums []int, typ string) Value {
	elems := make([]Value, len(nums))
	for i, x := range nums {
		elems[i] = it.raceInt(x)
	}
	switch it.lang {
	case "python":
		return it.newList("list", elems)
	case "php":
		d := it.newDict("array")
		for _, v := range elems {
			d.set(intV(d.next), v)
		}
		return Value{k: kDict, d: d}
	case "java":
		kind := "array"
		if strings.Contains(typ, "List") {
			kind = "ArrayList"
		}
		l := it.newList(kind, elems)
		l.l.elem = "int"
		if kind == "ArrayList" {
			l.l.elem = "Integer"
		}
		return l
	case "c", "cpp":
		kind := "array"
		if strings.Contains(typ, "vector") {
			kind = "vector"
		}
		l := it.newList(kind, elems)
		l.l.elem = "int"
		return l
	}
	return it.newList("array", elems)
}
