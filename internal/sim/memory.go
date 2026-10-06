package sim

import (
	"fmt"
	"go/ast"
	"go/types"

	"gopherviz/internal/trace"
)

const (
	maxMemVars = 12
	maxMemObjs = 12
)

// usesMemory melaporkan apakah blok fokus memakai struct atau pointer; hanya trace
// seperti itu yang membawa potret memori (supaya trace lain tetap kecil).
func usesMemory(p interface {
	TypeOf(ast.Expr) types.Type
}, node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		x, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if t := p.TypeOf(x); t != nil {
			switch t.Underlying().(type) {
			case *types.Pointer, *types.Struct:
				found = true
			}
		}
		return true
	})
	return found
}

type infoTypes struct{ info *types.Info }

func (t infoTypes) TypeOf(x ast.Expr) types.Type {
	if tv, ok := t.info.Types[x]; ok && !tv.IsType() {
		return tv.Type
	}
	return nil
}

// memory memotret variabel fungsi yang sedang berjalan beserta objek heap yang
// ditunjuk pointer. Pointer menunjuk ke label: nama variabel, "var.field", atau "objN".
func (it *interp) memory(e *env) *trace.Memory {
	if e == nil {
		return nil
	}
	type namedCell struct {
		name string
		cell *value
	}
	var vars []namedCell
	seen := map[string]bool{}
	for s := e; s != nil; s = s.parent {
		for i := len(s.order) - 1; i >= 0; i-- {
			name := s.order[i]
			if seen[name] || name == "_" {
				continue
			}
			seen[name] = true
			if cell := s.vars[name]; cell.k != kFunc {
				vars = append(vars, namedCell{name, cell})
			}
		}
		if s.fn {
			break
		}
	}
	for i, j := 0, len(vars)-1; i < j; i, j = i+1, j-1 { // urutan deklarasi: yang pertama dibuat di atas
		vars[i], vars[j] = vars[j], vars[i]
	}
	if len(vars) > maxMemVars {
		vars = vars[len(vars)-maxMemVars:]
	}

	labels := map[*value]string{}
	for _, v := range vars {
		labels[v.cell] = v.name
		if v.cell.k == kStruct && v.cell.st != nil {
			for i := range v.cell.st.fields {
				labels[&v.cell.st.fields[i]] = v.name + "." + v.cell.st.names[i]
			}
		}
	}
	var queue []*value
	target := func(p *value) string {
		if p == nil {
			return "nil"
		}
		if l, ok := labels[p]; ok {
			return l
		}
		r := it.rec
		id, ok := r.objIDs[p]
		if !ok {
			r.nextObj++
			id = fmt.Sprintf("obj%d", r.nextObj)
			r.objIDs[p] = id
		}
		labels[p] = id
		queue = append(queue, p)
		return id
	}

	mem := &trace.Memory{Vars: []trace.MemVar{}}
	for _, v := range vars {
		mv := trace.MemVar{Name: v.name, Type: v.cell.tname}
		mv.Kind, mv.Value, mv.Fields, mv.Points = memDescribe(*v.cell, target)
		mem.Vars = append(mem.Vars, mv)
	}
	for len(queue) > 0 && len(mem.Heap) < maxMemObjs {
		p := queue[0]
		queue = queue[1:]
		ob := trace.MemObj{ID: labels[p], Type: p.tname}
		ob.Kind, ob.Value, ob.Fields, _ = memDescribe(*p, target)
		mem.Heap = append(mem.Heap, ob)
	}
	return mem
}

func memDescribe(v value, target func(*value) string) (kind, val string, fields []trace.MemField, points string) {
	switch v.k {
	case kStruct:
		kind = "struct"
		if v.st != nil {
			for i, f := range v.st.fields {
				mf := trace.MemField{Name: v.st.names[i], Value: show(f)}
				if f.k == kPtr {
					mf.Points = target(f.ptr)
					mf.Value = ""
				}
				fields = append(fields, mf)
			}
		}
	case kPtr:
		kind = "pointer"
		points = target(v.ptr)
	case kSlice, kArray:
		kind, val = "slice", show(v)
	case kMap:
		kind, val = "map", show(v)
	case kError:
		kind, val = "error", v.s
	default:
		kind, val = "value", show(v)
	}
	return kind, val, fields, points
}
