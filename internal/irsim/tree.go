package irsim

import (
	"fmt"
	"slices"

	"gopherviz/internal/graphs"
	"gopherviz/internal/ir"
	"gopherviz/internal/trace"
)

// Penelusuran pohon (Fase 11b): fungsi rekursif yang memanggil dirinya dengan node.left /
// node.right (Left/Right, kiri/kanan, ->left) atau setiap anak di node.children. Pohon dibaca dari
// nilai parameter pada panggilan pertama: object class, dict, pointer struct C/C++.

// findTree menganalisis sintaks blok fokus; nil bila bukan penelusuran pohon.
func findTree(node ir.Node) *graphs.TreeSpec {
	f, ok := node.(*ir.Func)
	if !ok || f.Name == "" {
		return nil
	}
	params := map[string]bool{}
	for _, p := range f.Params {
		params[p.Name] = true
	}
	fields := map[string][]string{} // parameter → field anak
	kids := map[string]string{}     // parameter → field daftar anak
	loops := map[string][2]string{} // variabel loop → (parameter, field) untuk for c in node.children
	self := func(fun ir.Expr) bool {
		switch x := fun.(type) {
		case *ir.Ident:
			return x.Name == f.Name
		case *ir.Member: // self.inorder(node.left) / this.inorder(...)
			return x.Name == f.Name
		}
		return false
	}
	ir.Walk(node, func(n ir.Node) bool {
		switch x := n.(type) {
		case *ir.ForEach:
			if m, ok := x.X.(*ir.Member); ok {
				if p, ok := m.X.(*ir.Ident); ok && params[p.Name] && graphs.IsChildrenName(m.Name) {
					if v, ok := x.Value.(*ir.Ident); ok {
						loops[v.Name] = [2]string{p.Name, m.Name}
					}
				}
			}
		case *ir.Call:
			if !self(x.Fun) {
				break
			}
			for _, a := range x.Args {
				switch y := a.(type) {
				case *ir.Member:
					if p, ok := y.X.(*ir.Ident); ok && params[p.Name] && !slices.Contains(fields[p.Name], y.Name) {
						fields[p.Name] = append(fields[p.Name], y.Name)
					}
				case *ir.Index: // pohon dict: n["left"]
					p, ok1 := y.X.(*ir.Ident)
					l, ok2 := y.I.(*ir.Lit)
					if ok1 && ok2 && params[p.Name] && l.Kind == ir.LitString && !slices.Contains(fields[p.Name], l.Value) {
						fields[p.Name] = append(fields[p.Name], l.Value)
					}
				case *ir.Ident:
					if l, ok := loops[y.Name]; ok {
						kids[l[0]] = l[1]
					}
				}
			}
		}
		return true
	})
	var best *graphs.TreeSpec
	for _, p := range f.Params {
		fs := fields[p.Name]
		sp := &graphs.TreeSpec{Param: p.Name, Fields: fs, Kids: kids[p.Name]}
		// pohon: minimal dua field anak (linked list node.next bukan pohon) atau daftar anak
		if len(fs) < 2 && sp.Kids == "" {
			continue
		}
		if best == nil || len(fs) > len(best.Fields) {
			best = sp
		}
	}
	return best
}

// treeNode: identitas dan nilai simpul (pointer diikuti); ok = object/dict.
func treeNode(v Value) (key any, val Value, ok bool) {
	for i := 0; i < 4 && v.k == kPtr && v.p != nil; i++ {
		switch {
		case v.p.cell != nil:
			v = v.p.cell.get()
		case v.p.list != nil && v.p.idx >= 0 && v.p.idx < len(v.p.list.elems): // malloc(sizeof(struct Node))
			v = v.p.list.elems[v.p.idx]
		default:
			return nil, v, false
		}
	}
	switch v.k {
	case kObj:
		return v.o, v, v.o != nil
	case kDict:
		return v.d, v, v.d != nil
	}
	return nil, v, false
}

func nullish(v Value) bool {
	switch v.k {
	case kNull, kUndef:
		return true
	case kPtr:
		return v.p == nil || v.p.cell == nil && v.p.list == nil && v.p.obj == nil
	case kInt:
		return v.i == 0 // C: NULL
	}
	return false
}

func nodeField(v Value, name string) (Value, bool) {
	switch v.k {
	case kObj:
		return v.o.fields.get(strV(name))
	case kDict:
		return v.d.get(strV(name))
	}
	return Value{}, false
}

// readTree membaca pohon dari simpul akar; nil bila terlalu besar atau bukan pohon.
func (it *interp) readTree(root Value, sp *graphs.TreeSpec) (*trace.Tree, map[any]string) {
	ids := map[any]string{}
	tr := &trace.Tree{Var: sp.Param}
	var visit func(v Value) string
	tooBig := false
	visit = func(v Value) string {
		key, val, ok := treeNode(v)
		if !ok || tooBig {
			return ""
		}
		if id, seen := ids[key]; seen {
			return id // siklus/berbagi simpul: jangan diulang
		}
		if len(ids) >= graphs.MaxTreeNodes {
			tooBig = true
			return ""
		}
		id := fmt.Sprintf("n%d", len(ids)+1)
		ids[key] = id
		idx := len(tr.Nodes)
		tr.Nodes = append(tr.Nodes, trace.TreeNode{ID: id, Label: it.treeLabel(val, sp)})
		var left, right string
		for i, f := range sp.Fields {
			c, ok := nodeField(val, f)
			if !ok || nullish(c) {
				continue
			}
			cid := visit(c)
			switch {
			case len(sp.Fields) >= 2 && graphs.IsLeftName(sp.Fields[1]) && i == 1, i == 0 && !(len(sp.Fields) >= 2 && graphs.IsLeftName(sp.Fields[1])):
				left = cid
			default:
				right = cid
			}
		}
		tr.Nodes[idx].Left, tr.Nodes[idx].Right = left, right
		if sp.Kids != "" {
			if c, ok := nodeField(val, sp.Kids); ok {
				var elems []Value
				switch c.k {
				case kList:
					elems = c.l.elems
				case kDict:
					elems = c.d.vals
				}
				for _, x := range elems {
					if cid := visit(x); cid != "" {
						tr.Nodes[idx].Kids = append(tr.Nodes[idx].Kids, cid)
					}
				}
			}
		}
		return id
	}
	tr.Root = visit(root)
	if tooBig {
		it.warn(fmt.Sprintf("trees with more than %d nodes are not drawn", graphs.MaxTreeNodes))
		return nil, nil
	}
	if tr.Root == "" {
		return nil, nil
	}
	return tr, ids
}

// treeLabel: val/value/data/key/... atau field sederhana pertama.
func (it *interp) treeLabel(v Value, sp *graphs.TreeSpec) string {
	for _, f := range graphs.LabelFields() {
		if x, ok := nodeField(v, f); ok {
			return clipText(it.display(x, false), 6)
		}
	}
	var d *Dict
	if v.k == kObj {
		d = v.o.fields
	} else if v.k == kDict {
		d = v.d
	}
	if d != nil {
		for i, k := range d.keys {
			name := it.format(k)
			if slices.Contains(sp.Fields, name) || name == sp.Kids {
				continue
			}
			switch d.vals[i].k {
			case kInt, kFloat, kStr, kBool:
				return clipText(it.display(d.vals[i], false), 6)
			}
		}
	}
	return "?"
}

// treeStep membaca simpul yang dipegang parameter pada langkah ini.
func (it *interp) treeStep(e *Env) *trace.TreeState {
	r := it.rec
	sp := r.treeSpec
	if sp == nil || e == nil {
		return nil
	}
	c := e.local(sp.Param)
	if c == nil {
		return nil
	}
	v := c.get()
	if nullish(v) {
		if r.tree == nil {
			return nil
		}
		return &trace.TreeState{Null: true}
	}
	key, _, ok := treeNode(v)
	if !ok {
		return nil
	}
	if r.tree == nil {
		r.treeRoot = v
		r.tree, r.treeIDs = it.readTree(v, sp)
		if r.tree == nil {
			r.treeSpec = nil // terlalu besar / bukan pohon: lupakan
			return nil
		}
	}
	id, ok := r.treeIDs[key]
	if !ok { // pohon berubah (mis. sisip simpul baru): baca ulang dari akar
		if t, ids := it.readTree(r.treeRoot, sp); t != nil {
			r.tree, r.treeIDs = t, ids
			id = ids[key]
		}
	}
	if id == "" {
		return nil
	}
	return &trace.TreeState{Cur: id}
}
