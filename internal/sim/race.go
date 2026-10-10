package sim

import (
	"go/ast"

	"gopherviz/internal/race"
)

// Lomba algoritma (Fase 12c): fungsi fokus dipanggil langsung dengan list acak (race.Spec).
// Variabel global disiapkan dulu tanpa direkam; main tidak dijalankan.

func (it *interp) runRace(fd *ast.FuncDecl) {
	it.rec.hold++
	it.initGlobals()
	it.rec.hold--
	if it.raceSpec.Count {
		it.rec.hold++
		defer func() { it.rec.hold-- }()
	}
	args := it.raceArgs(fd)
	it.budget = it.budget0
	defer func() { it.work = it.budget0 - it.budget }()
	it.callFunc(&funcVal{name: fd.Name.Name, decl: fd}, args, fd)
}

func (it *interp) raceArgs(fd *ast.FuncDecl) []value {
	n := it.raceSpec.N
	nums := race.List(n, it.raceSpec.Seed)
	var args []value
	haveList := false
	for _, f := range fd.Type.Params.List {
		typ := it.text(f.Type)
		names := f.Names
		if len(names) == 0 {
			names = []*ast.Ident{{Name: "_"}}
		}
		for _, nm := range names {
			role := race.RoleOf(nm.Name, typ, haveList)
			if role == race.RoleList {
				haveList = true
				elems := make([]value, len(nums))
				for i, x := range nums {
					elems[i] = intV(int64(x))
				}
				args = append(args, sliceOf(elems))
				continue
			}
			args = append(args, intV(int64(race.Int(role, n))))
		}
	}
	return args
}
