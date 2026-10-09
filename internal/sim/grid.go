package sim

import (
	"go/ast"
	"go/token"
	"slices"
	"sort"

	"gopherviz/internal/graphs"
	"gopherviz/internal/trace"
)

// Penelusuran grid / labirin di Go (Fase 11a): grid[r][c] bersama antrian (cur := q[0]; q = q[1:])
// atau rekursi dfs(r, c). Lihat juga graphs/grid.go dan irsim/grid.go.

type gridUse struct{ a, b ast.Expr }

// findGrid menganalisis sintaks blok fokus; nil bila tidak terlihat seperti penelusuran grid.
func findGrid(node ast.Node) *graphs.GridSpec {
	sp, recursive, _ := scanTraversal(node)
	uses := map[string][]gridUse{}
	var names []string
	ast.Inspect(node, func(n ast.Node) bool {
		if x, ok := n.(*ast.IndexExpr); ok {
			if in, ok := unparen(x.X).(*ast.IndexExpr); ok {
				if g, ok := unparen(in.X).(*ast.Ident); ok {
					if len(uses[g.Name]) == 0 {
						names = append(names, g.Name)
					}
					uses[g.Name] = append(uses[g.Name], gridUse{unparen(in.Index), unparen(x.Index)})
				}
			}
		}
		return true
	})
	if len(names) == 0 {
		return nil
	}
	gs := &graphs.GridSpec{Frontier: sp.Frontier, Kind: sp.Kind}
	best := -1
	for _, n := range names {
		if graphs.IsVisitedName(n) || graphs.IsDistName(n) {
			if !slices.Contains(gs.Visited, n) {
				gs.Visited = append(gs.Visited, n)
			}
			continue
		}
		score := len(uses[n])
		if graphs.IsGridName(n) {
			score += 100
		}
		if score > best {
			best, gs.Grid = score, n
		}
	}
	if gs.Grid == "" {
		return nil
	}
	for _, v := range sp.Visited {
		if v != gs.Grid && !slices.Contains(gs.Visited, v) {
			gs.Visited = append(gs.Visited, v)
		}
	}
	sort.SliceStable(gs.Visited, func(i, j int) bool {
		return graphs.IsVisitedName(gs.Visited[i]) && !graphs.IsVisitedName(gs.Visited[j])
	})
	if gs.Frontier == "" && !recursive && len(gs.Visited) == 0 {
		return nil
	}
	if gs.Frontier == "" && recursive {
		gs.Kind = "dfs"
	}
	if via := gridCur(node, gs, uses); via == 0 || via >= 3 && !marker2D(node, gs, uses) {
		return nil
	}
	return gs
}

// marker2D: ada penanda kunjungan 2D, atau grid sendiri diubah (grid[r][c] = '#').
func marker2D(node ast.Node, gs *graphs.GridSpec, uses map[string][]gridUse) bool {
	for _, v := range gs.Visited {
		if len(uses[v]) > 0 {
			return true
		}
	}
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if a, ok := n.(*ast.AssignStmt); ok {
			for _, l := range a.Lhs {
				if x, ok := unparen(l).(*ast.IndexExpr); ok {
					if in, ok := unparen(x.X).(*ast.IndexExpr); ok {
						if g, ok := unparen(in.X).(*ast.Ident); ok && g.Name == gs.Grid {
							found = true
						}
					}
				}
			}
		}
		return !found
	})
	return found
}

// cellParts: p.r/p.c (field) atau p[0]/p[1] (indeks) dari satu variabel p.
func cellParts(a, b ast.Expr) (base string, fields [2]string, ok bool) {
	if x, ok1 := a.(*ast.SelectorExpr); ok1 {
		if y, ok2 := b.(*ast.SelectorExpr); ok2 {
			bx, ok3 := unparen(x.X).(*ast.Ident)
			by, ok4 := unparen(y.X).(*ast.Ident)
			if ok3 && ok4 && bx.Name == by.Name {
				return bx.Name, [2]string{x.Sel.Name, y.Sel.Name}, true
			}
		}
	}
	if x, ok1 := a.(*ast.IndexExpr); ok1 {
		if y, ok2 := b.(*ast.IndexExpr); ok2 {
			bx, ok3 := unparen(x.X).(*ast.Ident)
			by, ok4 := unparen(y.X).(*ast.Ident)
			if ok3 && ok4 && bx.Name == by.Name {
				return bx.Name, [2]string{}, true
			}
		}
	}
	return "", [2]string{}, false
}

// gridCur memilih variabel sel aktif (lihat irsim/grid.go): 1 = diambil dari antrian/tumpukan,
// 2 = parameter rekursi (r, c), 3 = pasangan indeks, 4 = struct/array sel; 0 = tidak ketemu.
func gridCur(node ast.Node, gs *graphs.GridSpec, uses map[string][]gridUse) int {
	var all []gridUse
	for _, u := range uses {
		all = append(all, u...)
	}
	pairOf := func(a, b string) bool {
		for _, u := range all {
			x, ok1 := u.a.(*ast.Ident)
			y, ok2 := u.b.(*ast.Ident)
			if ok1 && ok2 && x.Name == a && y.Name == b {
				return true
			}
		}
		return false
	}
	partsOf := func(p string) ([2]string, bool) {
		for _, u := range all {
			if base, f, ok := cellParts(u.a, u.b); ok && base == p {
				return f, true
			}
		}
		return [2]string{}, false
	}
	// cellUse: p dipakai sebagai sel di mana pun (p.r & p.c, p[0] & p[1])
	cellUse := func(p string) ([2]string, bool) {
		if f, ok := partsOf(p); ok {
			return f, true
		}
		idx, sel := map[string]bool{}, map[string]bool{}
		ast.Inspect(node, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.IndexExpr:
				if b, ok := unparen(x.X).(*ast.Ident); ok && b.Name == p {
					if l, ok := unparen(x.Index).(*ast.BasicLit); ok {
						idx[l.Value] = true
					}
				}
			case *ast.SelectorExpr:
				if b, ok := unparen(x.X).(*ast.Ident); ok && b.Name == p {
					sel[x.Sel.Name] = true
				}
			}
			return true
		})
		if idx["0"] && idx["1"] {
			return [2]string{}, true
		}
		for _, f := range [][2]string{{"r", "c"}, {"R", "C"}, {"row", "col"}, {"Row", "Col"}, {"y", "x"}, {"Y", "X"}, {"i", "j"}} {
			if sel[f[0]] && sel[f[1]] {
				return f, true
			}
		}
		return [2]string{}, false
	}
	// 1. cur := q[0] / r, c := q[0][0], q[0][1] / cur := st[len(st)-1]
	fromFrontier := func(x ast.Expr) bool {
		ix, ok := unparen(x).(*ast.IndexExpr)
		if !ok || gs.Frontier == "" {
			return false
		}
		q, ok := unparen(ix.X).(*ast.Ident)
		return ok && q.Name == gs.Frontier
	}
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		a, ok := n.(*ast.AssignStmt)
		if !ok || a.Tok != token.DEFINE && a.Tok != token.ASSIGN {
			return true
		}
		switch {
		case len(a.Lhs) == 1 && len(a.Rhs) == 1 && fromFrontier(a.Rhs[0]):
			if id, ok := a.Lhs[0].(*ast.Ident); ok {
				if f, ok := cellUse(id.Name); ok {
					gs.Pair, gs.Fields, found = id.Name, f, true
				}
			}
		case len(a.Lhs) >= 2 && len(a.Rhs) >= 2:
			// r, c := q[0][0], q[0][1] / r, c := cur.r, cur.c
			x, ok1 := a.Lhs[0].(*ast.Ident)
			y, ok2 := a.Lhs[1].(*ast.Ident)
			_, _, isCell := cellParts(unparen(a.Rhs[0]), unparen(a.Rhs[1]))
			if ok1 && ok2 && (isCell || fromFrontier(a.Rhs[0])) && (pairOf(x.Name, y.Name) || isCell) {
				gs.Row, gs.Col, found = x.Name, y.Name, true
			}
		}
		return true
	})
	if found {
		return 1
	}
	// 2. func dfs(r, c int)
	if f, ok := node.(*ast.FuncDecl); ok && f.Type.Params != nil {
		var ps []string
		for _, fl := range f.Type.Params.List {
			for _, n := range fl.Names {
				ps = append(ps, n.Name)
			}
		}
		for i := 0; i+1 < len(ps); i++ {
			if pairOf(ps[i], ps[i+1]) {
				gs.Row, gs.Col = ps[i], ps[i+1]
				return 2
			}
		}
	}
	// 3. pasangan indeks yang paling mungkin sel aktif
	counts := map[[2]string]int{}
	for _, u := range all {
		x, ok1 := u.a.(*ast.Ident)
		y, ok2 := u.b.(*ast.Ident)
		if ok1 && ok2 {
			counts[[2]string{x.Name, y.Name}]++
		}
	}
	bestScore, bestCount := -1<<30, 0
	for k, n := range counts {
		s := graphs.CurScore(k[0], k[1])
		if s > bestScore || s == bestScore && (n > bestCount || n == bestCount && k[0] < gs.Row) {
			bestScore, bestCount, gs.Row, gs.Col = s, n, k[0], k[1]
		}
	}
	if gs.Row != "" {
		return 3
	}
	// 4. struct / array sel di indeks grid: grid[p.r][p.c], grid[p[0]][p[1]]
	for _, u := range all {
		if base, f, ok := cellParts(u.a, u.b); ok {
			gs.Pair, gs.Fields = base, f
			return 4
		}
	}
	return 0
}

// gridRows membaca grid Go: []string (setiap string satu baris) atau slice/array 2D berisi nilai sederhana.
func gridRows(v value) ([][]string, bool) {
	elems := func(x value) ([]value, bool) {
		switch x.k {
		case kSlice:
			if x.sl == nil {
				return nil, false
			}
			return x.sl.elems(), true
		case kArray:
			return x.arr, true
		}
		return nil, false
	}
	rowsV, ok := elems(v)
	if !ok || len(rowsV) == 0 {
		return nil, false
	}
	var rows [][]string
	for _, rv := range rowsV {
		var row []string
		if rv.k == kString {
			for _, ch := range rv.s {
				row = append(row, string(ch))
			}
		} else {
			cells, ok := elems(rv)
			if !ok {
				return nil, false
			}
			for _, c := range cells {
				switch c.k {
				case kInt, kBool, kString, kFloat:
				default:
					return nil, false
				}
				row = append(row, format(c))
			}
		}
		if len(row) == 0 || len(rows) > 0 && len(row) != len(rows[0]) {
			return nil, false
		}
		rows = append(rows, row)
	}
	if n := len(rows) * len(rows[0]); n > graphs.MaxCells || n < 4 {
		return nil, false
	}
	return charRows(rows), true
}

// charRows: [][]byte / [][]rune berisi kode karakter ('#' = 35, '.' = 46) ditampilkan sebagai karakter.
func charRows(rows [][]string) [][]string {
	marks := false
	for _, row := range rows {
		for _, v := range row {
			n := 0
			for _, ch := range v {
				if ch < '0' || ch > '9' {
					return rows
				}
				n = n*10 + int(ch-'0')
			}
			if n < 32 || n > 126 {
				return rows
			}
			marks = marks || n == '#' || n == '.'
		}
	}
	if !marks {
		return rows
	}
	out := make([][]string, len(rows))
	for i, row := range rows {
		for _, v := range row {
			n := 0
			for _, ch := range v {
				n = n*10 + int(ch-'0')
			}
			out[i] = append(out[i], string(rune(n)))
		}
	}
	return out
}

// cellOf membaca satu sel: struct {r, c}, [2]int / []int, atau "r,c".
func cellOf(v value, fields [2]string) (int, int, bool) {
	if v.k == kPtr && v.ptr != nil {
		v = *v.ptr
	}
	two := func(xs []value) (int, int, bool) {
		if len(xs) < 2 || xs[0].k != kInt || xs[1].k != kInt {
			return 0, 0, false
		}
		return int(xs[0].i), int(xs[1].i), true
	}
	switch v.k {
	case kArray:
		return two(v.arr)
	case kSlice:
		if v.sl != nil {
			return two(v.sl.elems())
		}
	case kString:
		return graphs.ParsePair(v.s)
	case kStruct:
		if v.st == nil {
			return 0, 0, false
		}
		pairs := [][2]string{{"r", "c"}, {"R", "C"}, {"row", "col"}, {"Row", "Col"}, {"y", "x"}, {"Y", "X"}, {"i", "j"}}
		if fields[0] != "" {
			pairs = append([][2]string{fields}, pairs...)
		}
		for _, p := range pairs {
			a, b := v.st.index(p[0]), v.st.index(p[1])
			if a >= 0 && b >= 0 {
				return two([]value{v.st.fields[a], v.st.fields[b]})
			}
		}
		if len(v.st.fields) >= 2 {
			return two(v.st.fields)
		}
	}
	return 0, 0, false
}

// gridStep membaca keadaan penelusuran grid pada langkah ini.
func (it *interp) gridStep(e *env) *trace.GridState {
	r := it.rec
	gs := r.gridSpec
	if gs == nil || e == nil {
		return nil
	}
	if r.grid == nil {
		v := e.lookup(gs.Grid)
		if v == nil {
			return nil
		}
		rows, ok := gridRows(*v)
		if !ok {
			return nil
		}
		r.grid = &trace.Grid{Var: gs.Grid, Rows: rows, Kind: gs.Kind, Frontier: gs.Frontier}
		r.grid.Start, r.grid.Goal = graphs.StartGoal(rows)
	}
	g := r.grid
	R, C := len(g.Rows), len(g.Rows[0])
	in := func(a, b int) bool { return graphs.InGrid(a, b, R, C) }
	st := &trace.GridState{}
	switch {
	case gs.Row != "":
		a, b := e.lookup(gs.Row), e.lookup(gs.Col)
		if a != nil && b != nil && a.k == kInt && b.k == kInt && in(int(a.i), int(b.i)) {
			st.Cur = []int{int(a.i), int(b.i)}
		}
	case gs.Pair != "":
		if v := e.lookup(gs.Pair); v != nil {
			if a, b, ok := cellOf(*v, gs.Fields); ok && in(a, b) {
				st.Cur = []int{a, b}
			}
		}
	}
	if g.Start == nil || g.Goal == nil {
		for s := e; s != nil; s = s.parent {
			for _, name := range s.order {
				v := *s.vars[name]
				if g.Start == nil && graphs.IsStartName(name) {
					if a, b, ok := cellOf(v, [2]string{}); ok && in(a, b) {
						g.Start = []int{a, b}
					}
				}
				if g.Goal == nil && graphs.IsGoalName(name) {
					if a, b, ok := cellOf(v, [2]string{}); ok && in(a, b) {
						g.Goal = []int{a, b}
					}
				}
			}
		}
	}
	if g.Start == nil && st.Cur != nil {
		g.Start = append([]int(nil), st.Cur...)
	}
	if gs.Frontier != "" {
		if v := e.lookup(gs.Frontier); v != nil && v.k == kSlice && v.sl != nil {
			for _, x := range v.sl.elems() {
				if a, b, ok := cellOf(x, gs.Fields); ok && in(a, b) {
					st.Frontier = append(st.Frontier, [2]int{a, b})
				}
			}
		}
	}
	if g.Kind == "" || g.Kind == "?" {
		g.Kind = "bfs"
	}
	seen := map[[2]int]bool{}
	for _, name := range gs.Visited {
		v := e.lookup(name)
		if v == nil {
			continue
		}
		if rows, ok := gridRows(*v); ok && len(rows) == R && len(rows[0]) == C {
			for _, cell := range graphs.MarkedCells(rows) {
				seen[cell] = true
			}
			g.Visited = name
			continue
		}
		if v.k == kMap && v.m != nil {
			for _, k := range v.m.keys {
				if x, _ := v.m.get(k); x.k == kBool && !x.b {
					continue
				}
				if a, b, ok := cellOf(k, gs.Fields); ok && in(a, b) {
					seen[[2]int{a, b}] = true
					g.Visited = name
				}
			}
		}
	}
	if v := e.lookup(gs.Grid); v != nil {
		if rows, ok := gridRows(*v); ok && len(rows) == R && len(rows[0]) == C {
			for a := range rows {
				for b := range rows[a] {
					if rows[a][b] != g.Rows[a][b] {
						seen[[2]int{a, b}] = true
					}
				}
			}
		}
	}
	for cell := range seen {
		st.Visited = append(st.Visited, cell)
	}
	sort.Slice(st.Visited, func(i, j int) bool {
		a, b := st.Visited[i], st.Visited[j]
		return a[0] < b[0] || a[0] == b[0] && a[1] < b[1]
	})
	return st
}
