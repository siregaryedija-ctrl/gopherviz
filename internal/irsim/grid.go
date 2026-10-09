package irsim

import (
	"slices"
	"sort"

	"gopherviz/internal/graphs"
	"gopherviz/internal/ir"
	"gopherviz/internal/trace"
)

// Penelusuran grid / labirin (Fase 11a): grid[r][c] bersama antrian/tumpukan pasangan (r, c)
// atau rekursi pada (r, c). Lihat juga graphs/grid.go dan sim/grid.go (Go).

type gridUse struct{ a, b ir.Expr }

// findGrid menganalisis sintaks blok fokus; nil bila tidak terlihat seperti penelusuran grid.
func findGrid(node ir.Node) *graphs.GridSpec {
	sp, recursive, _ := scanTraversal(node)
	uses := map[string][]gridUse{}
	var names []string
	ir.Walk(node, func(n ir.Node) bool {
		if x, ok := n.(*ir.Index); ok {
			if in, ok := x.X.(*ir.Index); ok {
				if g, ok := in.X.(*ir.Ident); ok {
					if len(uses[g.Name]) == 0 {
						names = append(names, g.Name)
					}
					uses[g.Name] = append(uses[g.Name], gridUse{in.I, x.I})
				}
			}
		}
		return true
	})
	if len(names) == 0 {
		return nil
	}
	gs := &graphs.GridSpec{Frontier: sp.Frontier, Kind: sp.Kind}
	// grid = kandidat yang bukan penanda (visited/dist); nama lazim (grid, maze, ...) lebih dulu
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
	sort.SliceStable(gs.Visited, func(i, j int) bool { // penanda kunjungan sebelum jarak
		return graphs.IsVisitedName(gs.Visited[i]) && !graphs.IsVisitedName(gs.Visited[j])
	})
	if gs.Frontier == "" && !recursive && len(gs.Visited) == 0 {
		return nil
	}
	if gs.Frontier == "" && recursive {
		gs.Kind = "dfs"
	}
	// Yakin grid bila sel aktif diambil dari antrian/tumpukan atau parameter rekursi (r, c). Bila
	// hanya dari pasangan indeks, harus ada penanda 2D (visited[r][c] / dist[r][c] / grid[r][c] = ..):
	// matriks ketetanggaan adj[node][nb] dengan visited[node] tetap graf.
	if via := gridCur(node, gs, uses); via >= 3 && !marker2D(node, gs, uses) || via == 0 {
		return nil
	}
	return gs
}

// cellUse: variabel p dipakai sebagai sel di mana pun di blok: p[0] dan p[1] (field kosong), atau
// p.r dan p.c (row/col, y/x, ...).
func cellUse(node ir.Node, p string) ([2]string, bool) {
	idx := map[string]bool{}
	mem := map[string]bool{}
	ir.Walk(node, func(n ir.Node) bool {
		switch x := n.(type) {
		case *ir.Index:
			if b, ok := x.X.(*ir.Ident); ok && b.Name == p {
				if l, ok := x.I.(*ir.Lit); ok {
					idx[l.Value] = true
				}
			}
		case *ir.Member:
			if b, ok := x.X.(*ir.Ident); ok && b.Name == p {
				mem[x.Name] = true
			}
		}
		return true
	})
	if idx["0"] && idx["1"] {
		return [2]string{}, true
	}
	for _, f := range [][2]string{{"r", "c"}, {"row", "col"}, {"y", "x"}, {"i", "j"}, {"baris", "kolom"}, {"first", "second"}} {
		if mem[f[0]] && mem[f[1]] {
			return f, true
		}
	}
	return [2]string{}, false
}

// marker2D: ada penanda kunjungan 2D, atau grid sendiri diubah (grid[r][c] = '#').
func marker2D(node ir.Node, gs *graphs.GridSpec, uses map[string][]gridUse) bool {
	for _, v := range gs.Visited {
		if len(uses[v]) > 0 {
			return true
		}
	}
	found := false
	ir.Walk(node, func(n ir.Node) bool {
		if a, ok := n.(*ir.Assign); ok {
			if x, ok := a.L.(*ir.Index); ok {
				if in, ok := x.X.(*ir.Index); ok {
					if g, ok := in.X.(*ir.Ident); ok && g.Name == gs.Grid {
						found = true
					}
				}
			}
		}
		return !found
	})
	return found
}

// gridCur memilih variabel sel aktif: hasil pengambilan dari antrian/tumpukan (1), parameter fungsi
// rekursif (2), pasangan indeks grid yang paling sering dipakai, bukan tetangga nr, nc (3), atau
// object/list sel p.r, p[0] (4). Kembaliannya = cara yang dipakai (0 = tidak ketemu).
func gridCur(node ir.Node, gs *graphs.GridSpec, uses map[string][]gridUse) int {
	var all []gridUse
	for _, u := range uses {
		all = append(all, u...)
	}
	pairOf := func(a, b string) bool {
		for _, u := range all {
			x, ok1 := u.a.(*ir.Ident)
			y, ok2 := u.b.(*ir.Ident)
			if ok1 && ok2 && x.Name == a && y.Name == b {
				return true
			}
		}
		return false
	}
	// p.r, p.c → field; p[0], p[1] → indeks (field kosong)
	fieldsOf := func(p string) ([2]string, bool) {
		for _, u := range all {
			if x, ok := u.a.(*ir.Member); ok {
				if y, ok := u.b.(*ir.Member); ok {
					bx, ok3 := x.X.(*ir.Ident)
					by, ok4 := y.X.(*ir.Ident)
					if ok3 && ok4 && bx.Name == p && by.Name == p {
						return [2]string{x.Name, y.Name}, true
					}
				}
			}
			if x, ok := u.a.(*ir.Index); ok {
				if y, ok := u.b.(*ir.Index); ok {
					bx, ok3 := x.X.(*ir.Ident)
					by, ok4 := y.X.(*ir.Ident)
					if ok3 && ok4 && bx.Name == p && by.Name == p {
						return [2]string{}, true
					}
				}
			}
		}
		return [2]string{}, false
	}
	// 1. r, c = queue.popleft() / [r, c] = queue.shift() / cur = q.poll() / array_shift($q)
	fromPop := func(rhs ir.Expr) bool {
		call, ok := rhs.(*ir.Call)
		if !ok || gs.Frontier == "" {
			return false
		}
		switch f := call.Fun.(type) {
		case *ir.Member:
			q, ok := f.X.(*ir.Ident)
			return ok && q.Name == gs.Frontier
		case *ir.Ident:
			if len(call.Args) == 1 {
				q, ok := call.Args[0].(*ir.Ident)
				return ok && q.Name == gs.Frontier
			}
		}
		return false
	}
	bind := func(targets []ir.Expr) bool {
		var ids []string
		for _, t := range targets {
			if id, ok := t.(*ir.Ident); ok {
				ids = append(ids, id.Name)
			}
		}
		switch {
		case len(ids) >= 2 && pairOf(ids[1], ids[0]) && !pairOf(ids[0], ids[1]):
			gs.Row, gs.Col = ids[1], ids[0]
			return true
		case len(ids) >= 2: // r, c = q.popleft(): pasangan dari antrian = sel aktif (urutan seperti ditulis)
			gs.Row, gs.Col = ids[0], ids[1]
			return true
		case len(ids) == 1:
			// satu variabel: hanya bila dipakai sebagai sel (p.r / p[0]) di indeks grid atau di mana pun
			f, ok := fieldsOf(ids[0])
			if !ok {
				f, ok = cellUse(node, ids[0])
			}
			if ok {
				gs.Pair, gs.Fields = ids[0], f
			}
			return ok
		}
		return false
	}
	found := false
	ir.Walk(node, func(n ir.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ir.Assign:
			if fromPop(x.R) {
				found = bind(patternIdents(x.L))
			}
		case *ir.VarDecl:
			for _, sp := range x.Specs {
				if found || sp.Init == nil || !fromPop(sp.Init) {
					continue
				}
				if sp.Pattern != nil {
					found = bind(patternIdents(sp.Pattern))
				} else {
					found = bind([]ir.Expr{&ir.Ident{Name: sp.Name}})
				}
			}
		}
		return true
	})
	if found {
		return 1
	}
	// 2. fungsi rekursif dfs(r, c)
	if f, ok := node.(*ir.Func); ok {
		for i := 0; i+1 < len(f.Params); i++ {
			if pairOf(f.Params[i].Name, f.Params[i+1].Name) {
				gs.Row, gs.Col = f.Params[i].Name, f.Params[i+1].Name
				return 2
			}
		}
	}
	// 3. pasangan indeks yang paling mungkin sel aktif
	counts := map[[2]string]int{}
	for _, u := range all {
		x, ok1 := u.a.(*ir.Ident)
		y, ok2 := u.b.(*ir.Ident)
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
	// 4. object / list sel: p.r, p.c atau p[0], p[1]
	for _, u := range all {
		var base ir.Expr
		switch x := u.a.(type) {
		case *ir.Member:
			base = x.X
		case *ir.Index:
			base = x.X
		}
		if b, ok := base.(*ir.Ident); ok {
			if f, ok := fieldsOf(b.Name); ok {
				gs.Pair, gs.Fields = b.Name, f
				return 4
			}
		}
	}
	return 0
}

// gridRows membaca grid: list baris berisi nilai sederhana (atau string per baris), persegi panjang.
func (it *interp) gridRows(v Value) ([][]string, bool) {
	list := func(x Value) ([]Value, bool) {
		switch {
		case x.k == kList:
			return x.l.elems, true
		case x.k == kDict && x.d.kind == "array" && phpIsList(x.d):
			return x.d.vals, true
		}
		return nil, false
	}
	rowsV, ok := list(v)
	if !ok || len(rowsV) == 0 {
		return nil, false
	}
	var rows [][]string
	nulEnd := true // C: char m[3][4] = {"S.#", ...} → kolom terakhir '\0' di setiap baris
	for _, rv := range rowsV {
		var row []string
		if rv.k == kStr {
			nulEnd = false
			for _, ch := range rv.s {
				row = append(row, string(ch))
			}
		} else {
			cells, ok := list(rv)
			if !ok {
				return nil, false
			}
			for _, c := range cells {
				switch c.k {
				case kInt, kStr, kBool, kFloat, kNull, kUndef:
				default:
					return nil, false
				}
				txt := it.format(c)
				if len(txt) >= 3 && txt[0] == '\'' && txt[len(txt)-1] == '\'' { // char C/C++/Java: 'S' → S
					txt = txt[1 : len(txt)-1]
				}
				row = append(row, txt)
			}
			if n := len(cells); n == 0 || !(cells[n-1].k == kInt && cells[n-1].i == 0 && normType(cells[n-1].t) == "char") {
				nulEnd = false
			}
		}
		if len(row) == 0 || len(rows) > 0 && len(row) != len(rows[0]) {
			return nil, false
		}
		rows = append(rows, row)
	}
	if nulEnd && len(rows[0]) > 1 {
		for i := range rows {
			rows[i] = rows[i][:len(rows[i])-1]
		}
	}
	if n := len(rows) * len(rows[0]); n > graphs.MaxCells || n < 4 {
		return nil, false
	}
	return rows, true
}

// cellOf membaca satu sel dari nilai: (r, c) / [r, c] / object {r, c} / "r,c".
func (it *interp) cellOf(v Value, fields [2]string) (int, int, bool) {
	num := cellNum
	two := func(xs []Value) (int, int, bool) {
		if len(xs) < 2 {
			return 0, 0, false
		}
		a, ok1 := num(xs[0])
		b, ok2 := num(xs[1])
		return a, b, ok1 && ok2
	}
	switch v.k {
	case kList:
		return two(v.l.elems)
	case kStr:
		return graphs.ParsePair(v.s)
	case kDict, kObj:
		d := v.d
		if v.k == kObj {
			d = v.o.fields
		}
		pairs := [][2]string{{"r", "c"}, {"row", "col"}, {"y", "x"}, {"i", "j"}, {"baris", "kolom"}, {"first", "second"}}
		if fields[0] != "" {
			pairs = append([][2]string{fields}, pairs...)
		}
		for _, p := range pairs {
			x, ok1 := d.get(strV(p[0]))
			y, ok2 := d.get(strV(p[1]))
			if ok1 && ok2 {
				a, ok3 := num(x)
				b, ok4 := num(y)
				return a, b, ok3 && ok4
			}
		}
		if v.k == kDict && v.d.kind == "array" && phpIsList(v.d) {
			return two(v.d.vals)
		}
	}
	return 0, 0, false
}

// cellNum: indeks sel dari bilangan bulat (JS menyimpan angka sebagai float).
func cellNum(x Value) (int, bool) {
	switch {
	case x.k == kInt:
		return int(x.i), true
	case x.k == kFloat && x.f == float64(int(x.f)):
		return int(x.f), true
	}
	return 0, false
}

func (it *interp) intVar(e *Env, name string) (int, bool) {
	if c := it.findVar(e, name); c != nil {
		return cellNum(c.get())
	}
	return 0, false
}

// gridStep membaca keadaan penelusuran grid pada langkah ini.
func (it *interp) gridStep(e *Env) *trace.GridState {
	gs := it.rec.gridSpec
	if gs == nil || e == nil {
		return nil
	}
	r := it.rec
	if r.grid == nil {
		c := it.findVar(e, gs.Grid)
		if c == nil {
			return nil
		}
		rows, ok := it.gridRows(c.get())
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
		a, ok1 := it.intVar(e, gs.Row)
		b, ok2 := it.intVar(e, gs.Col)
		if ok1 && ok2 && in(a, b) {
			st.Cur = []int{a, b}
		}
	case gs.Pair != "":
		if c := it.findVar(e, gs.Pair); c != nil {
			if a, b, ok := it.cellOf(c.get(), gs.Fields); ok && in(a, b) {
				st.Cur = []int{a, b}
			}
		}
	}
	// titik awal/tujuan dari variabel start/goal bila grid tidak memuat 'S'/'G'
	if g.Start == nil || g.Goal == nil {
		for s := e; s != nil; s = s.parent {
			for _, name := range s.order {
				v := s.vars[name].get()
				if g.Start == nil && graphs.IsStartName(name) {
					if a, b, ok := it.cellOf(v, [2]string{}); ok && in(a, b) {
						g.Start = []int{a, b}
					}
				}
				if g.Goal == nil && graphs.IsGoalName(name) {
					if a, b, ok := it.cellOf(v, [2]string{}); ok && in(a, b) {
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
		if c := it.findVar(e, gs.Frontier); c != nil {
			v := c.get()
			if v.k == kDict && v.d.kind == "array" && phpIsList(v.d) {
				v = Value{k: kList, l: &List{elems: v.d.vals, kind: "array"}}
			}
			if v.k == kList {
				if g.Kind == "?" {
					g.Kind = map[bool]string{true: "bfs", false: "dfs"}[v.l.kind == "queue" || v.l.kind == "deque"]
				}
				for _, x := range v.l.elems {
					if a, b, ok := it.cellOf(x, gs.Fields); ok && in(a, b) {
						st.Frontier = append(st.Frontier, [2]int{a, b})
					}
				}
			}
		}
	}
	if g.Kind == "?" || g.Kind == "" {
		g.Kind = "bfs"
	}
	seen := map[[2]int]bool{}
	for _, name := range gs.Visited {
		c := it.findVar(e, name)
		if c == nil {
			continue
		}
		v := c.get()
		if rows, ok := it.gridRows(v); ok && len(rows) == R && len(rows[0]) == C {
			for _, cell := range graphs.MarkedCells(rows) {
				seen[cell] = true
			}
			g.Visited = name
			continue
		}
		var elems []Value
		switch v.k {
		case kSet, kDict:
			elems = v.d.keys
		case kList:
			elems = v.l.elems
		}
		for _, x := range elems {
			if a, b, ok := it.cellOf(x, [2]string{}); ok && in(a, b) {
				seen[[2]int{a, b}] = true
				g.Visited = name
			}
		}
	}
	// isi grid yang diubah program (mis. grid[r][c] = '#'): sel itu sudah dikunjungi
	if c := it.findVar(e, gs.Grid); c != nil {
		if rows, ok := it.gridRows(c.get()); ok && len(rows) == R && len(rows[0]) == C {
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
