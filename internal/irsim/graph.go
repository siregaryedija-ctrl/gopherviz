package irsim

import (
	"slices"
	"strconv"

	"gopherviz/internal/graphs"
	"gopherviz/internal/ir"
	"gopherviz/internal/trace"
)

// Penelusuran graf (DFS/BFS): blok fokus yang memakai graph[node] bersama antrian/tumpukan,
// rekursi, atau penanda visited direkam sebagai Trace.Graph + Step.Graph untuk panggung graf.

// findGraph menganalisis sintaks blok fokus; nil bila tidak terlihat seperti penelusuran graf.
func findGraph(node ir.Node) *graphs.Spec {
	sp := &graphs.Spec{Cur: map[string]string{}}
	count := map[string]int{}
	note := func(g, cur ir.Expr) {
		gi, ok1 := g.(*ir.Ident)
		ci, ok2 := cur.(*ir.Ident)
		if !ok1 || !ok2 || gi.Name == ci.Name {
			return
		}
		if count[gi.Name] == 0 {
			sp.Graphs = append(sp.Graphs, gi.Name)
		}
		count[gi.Name]++
		if sp.Cur[gi.Name] == "" {
			sp.Cur[gi.Name] = ci.Name
		}
	}
	self := ""
	if f, ok := node.(*ir.Func); ok {
		self = f.Name
	}
	recursive := false
	ir.Walk(node, func(n ir.Node) bool {
		switch x := n.(type) {
		case *ir.Index:
			note(x.X, x.I)
		case *ir.Ident:
			if graphs.IsVisitedName(x.Name) && !slices.Contains(sp.Visited, x.Name) {
				sp.Visited = append(sp.Visited, x.Name)
			}
		case *ir.Call:
			switch f := x.Fun.(type) {
			case *ir.Ident:
				if self != "" && f.Name == self {
					recursive = true
				}
				if len(x.Args) == 1 { // PHP array_shift($antrian) / array_pop($tumpukan)
					if q, ok := x.Args[0].(*ir.Ident); ok {
						switch f.Name {
						case "array_shift":
							sp.Frontier, sp.Kind = q.Name, "bfs"
						case "array_pop":
							if sp.Frontier == "" {
								sp.Frontier, sp.Kind = q.Name, "dfs"
							}
						}
					}
				}
			case *ir.Member:
				if (f.Name == "get" || f.Name == "getOrDefault" || f.Name == "at") && len(x.Args) >= 1 {
					note(f.X, x.Args[0])
				}
				q, ok := f.X.(*ir.Ident)
				if !ok {
					break
				}
				switch {
				case f.Name == "pop" && len(x.Args) == 1:
					if l, ok := x.Args[0].(*ir.Lit); ok && l.Value == "0" { // Python list.pop(0)
						sp.Frontier, sp.Kind = q.Name, "bfs"
					}
				case f.Name == "pop" && len(x.Args) == 0:
					if sp.Frontier == "" {
						sp.Frontier, sp.Kind = q.Name, "?" // C++ queue.pop() vs stack.pop(): dari jenis nilainya
					}
				case f.Name == "remove" && len(x.Args) > 0:
				case slices.Contains(graphs.QueueOps, f.Name):
					sp.Frontier, sp.Kind = q.Name, "bfs"
				case slices.Contains(graphs.StackOps, f.Name) && sp.Frontier == "":
					sp.Frontier, sp.Kind = q.Name, "dfs"
				}
			}
		}
		return true
	})
	if len(sp.Graphs) == 0 || sp.Frontier == "" && !recursive && len(sp.Visited) == 0 {
		return nil
	}
	slices.SortStableFunc(sp.Graphs, func(a, b string) int { return count[b] - count[a] })
	if sp.Frontier == "" && recursive {
		sp.Kind = "dfs"
	}
	return sp
}

// graphStep membaca keadaan penelusuran dari variabel program pada langkah ini.
func (it *interp) graphStep(e *Env) *trace.GraphState {
	sp := it.rec.graphSpec
	if sp == nil || e == nil {
		return nil
	}
	if it.rec.graph == nil {
		for _, name := range sp.Graphs {
			if c := it.findVar(e, name); c != nil {
				if adj, ok := it.adjFrom(c.get()); ok {
					if g := adj.Graph(name); g != nil {
						it.rec.graph = g
						it.rec.graphCur = sp.Cur[name]
						break
					}
				}
			}
		}
		if it.rec.graph == nil {
			return nil
		}
	}
	g := it.rec.graph
	st := &trace.GraphState{}
	if c := it.findVar(e, it.rec.graphCur); c != nil {
		if id := it.format(c.get()); graphs.Contains(g, id) {
			st.Cur = id
		}
	}
	if sp.Frontier != "" {
		if c := it.findVar(e, sp.Frontier); c != nil {
			v := c.get()
			if v.k == kDict && v.d.kind == "array" && phpIsList(v.d) { // array PHP sebagai antrian
				v = Value{k: kList, l: &List{elems: v.d.vals, kind: "array"}}
			}
			if v.k == kList {
				g.Frontier = sp.Frontier
				if g.Kind == "" {
					g.Kind = sp.Kind
					if g.Kind == "?" {
						g.Kind = map[bool]string{true: "bfs", false: "dfs"}[v.l.kind == "queue" || v.l.kind == "priority_queue"]
					}
				}
				for _, x := range v.l.elems {
					if x.k == kList && len(x.l.elems) > 0 { // (simpul, jarak)
						x = x.l.elems[0]
					}
					st.Frontier = append(st.Frontier, it.format(x))
				}
				st.Frontier = graphs.Filter(g, st.Frontier)
			}
		}
	} else if g.Kind == "" {
		g.Kind = sp.Kind
	}
	for _, name := range sp.Visited {
		if g.Visited != "" && name != g.Visited {
			continue
		}
		c := it.findVar(e, name)
		if c == nil {
			continue
		}
		if vis, ok := it.visitedNodes(c.get(), g); ok {
			g.Visited = name
			st.Visited = vis
			break
		}
	}
	return st
}

// visitedNodes membaca penanda kunjungan: set simpul, dict simpul → bool, list bool per indeks,
// atau list urutan kunjungan.
func (it *interp) visitedNodes(v Value, g *trace.Graph) ([]string, bool) {
	var out []string
	switch v.k {
	case kSet:
		for _, k := range v.d.keys {
			out = append(out, it.format(k))
		}
	case kDict:
		if v.d.kind == "array" && phpIsList(v.d) {
			return it.visitedNodes(Value{k: kList, l: &List{elems: v.d.vals}}, g)
		}
		for i, k := range v.d.keys {
			if it.sem.truthy(v.d.vals[i]) || v.d.vals[i].k == kInt {
				out = append(out, it.format(k))
			}
		}
	case kList:
		flags := len(v.l.elems) == len(g.Nodes)
		for _, x := range v.l.elems {
			if x.k != kBool && !(x.k == kInt && (x.i == 0 || x.i == 1)) {
				flags = false
			}
		}
		for i, x := range v.l.elems {
			switch {
			case flags && it.sem.truthy(x):
				out = append(out, g.Nodes[i])
			case !flags:
				out = append(out, it.format(x))
			}
		}
	default:
		return nil, false
	}
	return graphs.Filter(g, out), true
}

// adjFrom membaca graf dari nilai program: dict/map simpul → daftar tetangga, list of list
// (indeks = simpul), atau matriks ketetanggaan 0/1.
func (it *interp) adjFrom(v Value) (*graphs.Adj, bool) {
	adj := &graphs.Adj{}
	neighbors := func(from string, x Value) bool {
		var elems []Value
		switch x.k {
		case kList:
			elems = x.l.elems
		case kSet:
			elems = x.d.keys
		case kDict:
			if x.d.kind != "array" || !phpIsList(x.d) {
				return false
			}
			elems = x.d.vals
		default:
			return false
		}
		for _, n := range elems {
			if n.k == kList && len(n.l.elems) > 0 { // (tetangga, bobot)
				n = n.l.elems[0]
			}
			if n.k != kInt && n.k != kStr {
				return false
			}
			adj.Link(from, it.format(n))
		}
		return true
	}
	switch v.k {
	case kDict:
		if len(v.d.keys) == 0 || len(v.d.keys) > graphs.MaxNodes {
			return nil, false
		}
		for i, k := range v.d.keys {
			adj.Add(it.format(k))
			if !neighbors(it.format(k), v.d.vals[i]) {
				return nil, false
			}
		}
		if v.d.kind == "array" && phpIsList(v.d) && !indexNodes(adj, len(v.d.keys)) {
			return nil, false
		}
	case kList:
		n := len(v.l.elems)
		if n < 2 || n > graphs.MaxNodes {
			return nil, false
		}
		if m, ok := it.matrixAdj(v.l.elems); ok {
			return m, true
		}
		for i, row := range v.l.elems {
			adj.Add(itoa(i))
			if !neighbors(itoa(i), row) {
				return nil, false
			}
		}
		if !indexNodes(adj, n) {
			return nil, false
		}
	default:
		return nil, false
	}
	return adj, true
}

// indexNodes: pada graf berbentuk list of list, setiap simpul harus indeks baris 0..rows-1.
func indexNodes(adj *graphs.Adj, rows int) bool {
	for _, x := range adj.Nodes {
		if i, err := strconv.Atoi(x); err != nil || i < 0 || i >= rows {
			return false
		}
	}
	return true
}

// matrixAdj: matriks n×n berisi 0/1 dengan diagonal 0.
func (it *interp) matrixAdj(rows []Value) (*graphs.Adj, bool) {
	n := len(rows)
	adj := &graphs.Adj{}
	for i := 0; i < n; i++ {
		adj.Add(itoa(i))
	}
	for i, r := range rows {
		if r.k != kList || len(r.l.elems) != n {
			return nil, false
		}
		for j, x := range r.l.elems {
			if x.k != kInt || x.i != 0 && x.i != 1 || i == j && x.i != 0 {
				return nil, false
			}
			if x.i == 1 {
				adj.Link(itoa(i), itoa(j))
			}
		}
	}
	return adj, true
}

func itoa(i int) string { return strconv.Itoa(i) }
