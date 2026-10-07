package sim

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"

	"gopherviz/internal/graphs"
	"gopherviz/internal/trace"
)

// Penelusuran graf (DFS/BFS) di Go: graph[node] bersama antrian (q = q[1:]), tumpukan
// (st = st[:len(st)-1]), rekursi, atau penanda visited. Lihat juga irsim/graph.go.

// findGraph menganalisis sintaks blok fokus; nil bila tidak terlihat seperti penelusuran graf.
func findGraph(node ast.Node) *graphs.Spec {
	sp := &graphs.Spec{Cur: map[string]string{}}
	count := map[string]int{}
	self := ""
	if f, ok := node.(*ast.FuncDecl); ok {
		self = f.Name.Name
	}
	recursive := false
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IndexExpr:
			g, ok1 := unparen(x.X).(*ast.Ident)
			c, ok2 := unparen(x.Index).(*ast.Ident)
			if ok1 && ok2 && g.Name != c.Name {
				if count[g.Name] == 0 {
					sp.Graphs = append(sp.Graphs, g.Name)
				}
				count[g.Name]++
				if sp.Cur[g.Name] == "" {
					sp.Cur[g.Name] = c.Name
				}
			}
		case *ast.Ident:
			if graphs.IsVisitedName(x.Name) && !slices.Contains(sp.Visited, x.Name) {
				sp.Visited = append(sp.Visited, x.Name)
			}
		case *ast.CallExpr:
			if id, ok := x.Fun.(*ast.Ident); ok && self != "" && id.Name == self {
				recursive = true
			}
		case *ast.AssignStmt:
			// q = q[1:] (antrian) atau st = st[:len(st)-1] (tumpukan)
			if len(x.Lhs) != 1 || len(x.Rhs) != 1 || x.Tok != token.ASSIGN {
				break
			}
			l, ok := x.Lhs[0].(*ast.Ident)
			s, ok2 := x.Rhs[0].(*ast.SliceExpr)
			if !ok || !ok2 {
				break
			}
			if r, ok := s.X.(*ast.Ident); !ok || r.Name != l.Name {
				break
			}
			switch {
			case s.Low != nil && s.High == nil:
				sp.Frontier, sp.Kind = l.Name, "bfs"
			case s.Low == nil && s.High != nil && sp.Frontier == "":
				sp.Frontier, sp.Kind = l.Name, "dfs"
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
func (it *interp) graphStep(e *env) *trace.GraphState {
	r := it.rec
	sp := r.graphSpec
	if sp == nil || e == nil {
		return nil
	}
	if r.graph == nil {
		for _, name := range sp.Graphs {
			if v := e.lookup(name); v != nil {
				if adj, ok := adjFrom(*v); ok {
					if g := adj.Graph(name); g != nil {
						r.graph, r.graphCur = g, sp.Cur[name]
						g.Kind = sp.Kind
						break
					}
				}
			}
		}
		if r.graph == nil {
			return nil
		}
	}
	g := r.graph
	st := &trace.GraphState{}
	if v := e.lookup(r.graphCur); v != nil {
		if id := format(*v); graphs.Contains(g, id) {
			st.Cur = id
		}
	}
	if sp.Frontier != "" {
		if v := e.lookup(sp.Frontier); v != nil && v.k == kSlice && v.sl != nil {
			g.Frontier = sp.Frontier
			for i := 0; i < v.sl.len; i++ {
				st.Frontier = append(st.Frontier, format(*v.sl.at(i)))
			}
			st.Frontier = graphs.Filter(g, st.Frontier)
		}
	}
	for _, name := range sp.Visited {
		if g.Visited != "" && name != g.Visited {
			continue
		}
		v := e.lookup(name)
		if v == nil {
			continue
		}
		if vis, ok := visitedNodes(*v, g); ok {
			g.Visited, st.Visited = name, vis
			break
		}
	}
	return st
}

// elems mengembalikan isi slice/array Go.
func elems(v value) ([]value, bool) {
	switch {
	case v.k == kSlice && v.sl != nil:
		out := make([]value, v.sl.len)
		for i := range out {
			out[i] = *v.sl.at(i)
		}
		return out, true
	case v.k == kArray:
		return v.arr, true
	}
	return nil, false
}

// visitedNodes: map simpul → bool, []bool per indeks, atau slice urutan kunjungan.
func visitedNodes(v value, g *trace.Graph) ([]string, bool) {
	var out []string
	if v.k == kMap {
		if v.m == nil {
			return nil, true
		}
		for _, k := range v.m.keys {
			if x, _ := v.m.get(k); x.k != kBool || x.b {
				out = append(out, format(k))
			}
		}
		return graphs.Filter(g, out), true
	}
	xs, ok := elems(v)
	if !ok {
		return nil, false
	}
	flags := len(xs) == len(g.Nodes)
	for _, x := range xs {
		if x.k != kBool {
			flags = false
		}
	}
	for i, x := range xs {
		switch {
		case flags && x.b:
			out = append(out, g.Nodes[i])
		case !flags:
			out = append(out, format(x))
		}
	}
	return graphs.Filter(g, out), true
}

// adjFrom membaca graf: map simpul → []tetangga, [][]int (indeks = simpul), atau matriks 0/1.
func adjFrom(v value) (*graphs.Adj, bool) {
	adj := &graphs.Adj{}
	link := func(from string, x value) bool {
		xs, ok := elems(x)
		if !ok {
			return false
		}
		for _, n := range xs {
			if n.k != kInt && n.k != kString {
				return false
			}
			adj.Link(from, format(n))
		}
		return true
	}
	if v.k == kMap {
		if v.m == nil || len(v.m.keys) == 0 || len(v.m.keys) > graphs.MaxNodes {
			return nil, false
		}
		for _, k := range v.m.keys {
			adj.Add(format(k))
			x, _ := v.m.get(k)
			if !link(format(k), x) {
				return nil, false
			}
		}
		return adj, true
	}
	rows, ok := elems(v)
	if !ok || len(rows) < 2 || len(rows) > graphs.MaxNodes {
		return nil, false
	}
	if m, ok := matrixAdj(rows); ok {
		return m, true
	}
	for i, row := range rows {
		adj.Add(strconv.Itoa(i))
		if !link(strconv.Itoa(i), row) {
			return nil, false
		}
	}
	for _, x := range adj.Nodes {
		if i, err := strconv.Atoi(x); err != nil || i < 0 || i >= len(rows) {
			return nil, false
		}
	}
	return adj, true
}

// matrixAdj: matriks n×n berisi 0/1 dengan diagonal 0.
func matrixAdj(rows []value) (*graphs.Adj, bool) {
	n := len(rows)
	adj := &graphs.Adj{}
	for i := 0; i < n; i++ {
		adj.Add(strconv.Itoa(i))
	}
	for i, r := range rows {
		xs, ok := elems(r)
		if !ok || len(xs) != n {
			return nil, false
		}
		for j, x := range xs {
			if x.k != kInt || x.i != 0 && x.i != 1 || i == j && x.i != 0 {
				return nil, false
			}
			if x.i == 1 {
				adj.Link(strconv.Itoa(i), strconv.Itoa(j))
			}
		}
	}
	return adj, true
}
