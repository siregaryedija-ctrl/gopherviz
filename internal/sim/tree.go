package sim

import (
	"fmt"
	"go/ast"
	"slices"

	"gopherviz/internal/graphs"
	"gopherviz/internal/trace"
)

// Penelusuran pohon di Go (Fase 11b): func f(n *Node) yang memanggil dirinya dengan n.Left /
// n.Right atau setiap anak di n.Children. Lihat juga irsim/tree.go.

// findTree menganalisis sintaks blok fokus; nil bila bukan penelusuran pohon.
func findTree(node ast.Node) *graphs.TreeSpec {
	f, ok := node.(*ast.FuncDecl)
	if !ok || f.Type.Params == nil {
		return nil
	}
	var params []string
	isParam := map[string]bool{}
	for _, fl := range f.Type.Params.List {
		for _, n := range fl.Names {
			params = append(params, n.Name)
			isParam[n.Name] = true
		}
	}
	fields := map[string][]string{}
	kids := map[string]string{}
	loops := map[string][2]string{}
	self := func(fun ast.Expr) bool {
		switch x := unparen(fun).(type) {
		case *ast.Ident:
			return x.Name == f.Name.Name
		case *ast.SelectorExpr: // t.walk(n.Left)
			return x.Sel.Name == f.Name.Name
		}
		return false
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.RangeStmt:
			if s, ok := unparen(x.X).(*ast.SelectorExpr); ok {
				if p, ok := unparen(s.X).(*ast.Ident); ok && isParam[p.Name] && graphs.IsChildrenName(s.Sel.Name) {
					if v, ok := x.Value.(*ast.Ident); ok {
						loops[v.Name] = [2]string{p.Name, s.Sel.Name}
					}
				}
			}
		case *ast.CallExpr:
			if !self(x.Fun) {
				break
			}
			for _, a := range x.Args {
				switch y := unparen(a).(type) {
				case *ast.SelectorExpr:
					if p, ok := unparen(y.X).(*ast.Ident); ok && isParam[p.Name] && !slices.Contains(fields[p.Name], y.Sel.Name) {
						fields[p.Name] = append(fields[p.Name], y.Sel.Name)
					}
				case *ast.Ident:
					if l, ok := loops[y.Name]; ok {
						kids[l[0]] = l[1]
					}
				}
			}
		}
		return true
	})
	var best *graphs.TreeSpec
	for _, p := range params {
		sp := &graphs.TreeSpec{Param: p, Fields: fields[p], Kids: kids[p]}
		if len(sp.Fields) < 2 && sp.Kids == "" {
			continue
		}
		if best == nil || len(sp.Fields) > len(best.Fields) {
			best = sp
		}
	}
	return best
}

// treeNode: struct simpul di balik pointer (identitas = struct di memori).
func treeNode(v value) (*structVal, bool) {
	if v.k == kPtr {
		if v.ptr == nil {
			return nil, false
		}
		v = *v.ptr
	}
	if v.k != kStruct || v.st == nil {
		return nil, false
	}
	return v.st, true
}

func treeNil(v value) bool { return v.k == kNil || v.k == kPtr && v.ptr == nil }

func (st *structVal) field(name string) (value, bool) {
	if i := st.index(name); i >= 0 {
		return st.fields[i], true
	}
	return value{}, false
}

// readTree membaca pohon dari akar; nil bila terlalu besar.
func readTree(root value, sp *graphs.TreeSpec) (*trace.Tree, map[*structVal]string, bool) {
	ids := map[*structVal]string{}
	tr := &trace.Tree{Var: sp.Param}
	tooBig := false
	var visit func(v value) string
	visit = func(v value) string {
		st, ok := treeNode(v)
		if !ok || tooBig {
			return ""
		}
		if id, seen := ids[st]; seen {
			return id
		}
		if len(ids) >= graphs.MaxTreeNodes {
			tooBig = true
			return ""
		}
		id := fmt.Sprintf("n%d", len(ids)+1)
		ids[st] = id
		idx := len(tr.Nodes)
		tr.Nodes = append(tr.Nodes, trace.TreeNode{ID: id, Label: treeLabel(st, sp)})
		leftSecond := len(sp.Fields) >= 2 && graphs.IsLeftName(sp.Fields[1])
		for i, f := range sp.Fields {
			c, ok := st.field(f)
			if !ok || treeNil(c) {
				continue
			}
			cid := visit(c)
			if i == 0 && !leftSecond || i == 1 && leftSecond {
				tr.Nodes[idx].Left = cid
			} else {
				tr.Nodes[idx].Right = cid
			}
		}
		if sp.Kids != "" {
			if c, ok := st.field(sp.Kids); ok && c.k == kSlice && c.sl != nil {
				for _, x := range c.sl.elems() {
					if cid := visit(x); cid != "" {
						tr.Nodes[idx].Kids = append(tr.Nodes[idx].Kids, cid)
					}
				}
			}
		}
		return id
	}
	tr.Root = visit(root)
	if tooBig || tr.Root == "" {
		return nil, nil, tooBig
	}
	return tr, ids, false
}

func treeLabel(st *structVal, sp *graphs.TreeSpec) string {
	for _, f := range graphs.LabelFields() {
		if x, ok := st.field(f); ok {
			return clip(labelText(x), 6)
		}
	}
	for i, n := range st.names {
		if slices.Contains(sp.Fields, n) || n == sp.Kids {
			continue
		}
		switch st.fields[i].k {
		case kInt, kFloat, kString, kBool:
			return clip(labelText(st.fields[i]), 6)
		}
	}
	return "?"
}

func labelText(v value) string {
	if v.k == kString {
		return v.s
	}
	return show(v)
}

// treeStep membaca simpul yang dipegang parameter pada langkah ini.
func (it *interp) treeStep(e *env) *trace.TreeState {
	r := it.rec
	sp := r.treeSpec
	if sp == nil || e == nil {
		return nil
	}
	cell, ok := e.vars[sp.Param] // parameter fungsi yang sedang berjalan saja
	if !ok {
		for s := e; s != nil && !ok; s = s.parent {
			cell, ok = s.vars[sp.Param]
			if s.fn {
				break
			}
		}
		if !ok {
			return nil
		}
	}
	v := *cell
	if treeNil(v) {
		if r.tree == nil {
			return nil
		}
		return &trace.TreeState{Null: true}
	}
	st, ok := treeNode(v)
	if !ok {
		return nil
	}
	if r.tree == nil {
		t, ids, tooBig := readTree(v, sp)
		if tooBig {
			it.warn(fmt.Sprintf("trees with more than %d nodes are not drawn", graphs.MaxTreeNodes))
		}
		if t == nil {
			r.treeSpec = nil
			return nil
		}
		r.tree, r.treeIDs, r.treeRoot = t, ids, v
	}
	id, ok := r.treeIDs[st]
	if !ok {
		if t, ids, _ := readTree(r.treeRoot, sp); t != nil {
			r.tree, r.treeIDs = t, ids
			id = ids[st]
		}
	}
	if id == "" {
		return nil
	}
	return &trace.TreeState{Cur: id}
}
