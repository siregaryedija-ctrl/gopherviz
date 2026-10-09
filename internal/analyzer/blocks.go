package analyzer

import (
	"fmt"
	"go/ast"
	"sort"

	"gopherviz/internal/scene"
)

// blockCand adalah fungsi, kepala rantai if, atau switch yang akan menjadi Block.
type blockCand struct {
	node     ast.Node
	kind     scene.BlockKind
	id       string
	off, end int
}

// collectBlocks memberi ID pada fungsi, if (hanya kepala rantai else-if), dan switch.
// Loop sudah diberi ID oleh collect(); semua ID disimpan di b.ids.
func (b *builder) collectBlocks() []*blockCand {
	elseIf := map[*ast.IfStmt]bool{}
	var out []*blockCand
	counts := map[scene.BlockKind]int{}
	add := func(n ast.Node, kind scene.BlockKind, prefix string) {
		counts[kind]++
		c := &blockCand{node: n, kind: kind, id: fmt.Sprintf("%s%d", prefix, counts[kind]),
			off: b.src.offset(n.Pos()), end: b.src.offset(n.End())}
		b.ids[n] = c.id
		out = append(out, c)
	}
	ast.Inspect(b.src.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			// "_" di baris pembungkus adalah fungsi buatan untuk potongan kode tanpa package.
			if x.Body != nil && !(b.src.shift > 0 && x.Name.Name == "_" && b.src.fset.Position(x.Pos()).Line <= b.src.shift) {
				add(x, scene.BlockFunc, "F")
			}
		case *ast.FuncLit:
			add(x, scene.BlockFunc, "F")
		case *ast.IfStmt:
			if el, ok := x.Else.(*ast.IfStmt); ok {
				elseIf[el] = true
			}
			if !elseIf[x] {
				add(x, scene.BlockIf, "I")
			}
		case *ast.SwitchStmt:
			add(x, scene.BlockSwitch, "S")
		case *ast.TypeSwitchStmt:
			add(x, scene.BlockSwitch, "S")
		case *ast.TypeSpec:
			// type X struct { ... } = "class" Go: method-methodnya dicari lewat receiver
			if _, ok := x.Type.(*ast.StructType); ok && x.Name != nil && x.Name.Name != "_" {
				add(x, scene.BlockClass, "C")
			}
		}
		return true
	})
	return out
}

// blocks membangun daftar Block untuk semua jenis (termasuk loop) beserta relasi induk-anak.
func (b *builder) blocks(cands []*blockCand, loopCands []*cand, loops []scene.Loop) []scene.Block {
	type entry struct {
		blk      scene.Block
		off, end int
	}
	var all []entry
	for _, c := range cands {
		all = append(all, entry{b.block(c), c.off, c.end})
	}
	for i, l := range loops {
		c := loopCands[i]
		blk := scene.Block{
			ID: l.ID, Kind: scene.BlockLoop, Line: l.Line, EndLine: l.EndLine, State: l.State,
			Name: l.Label, Header: l.Header.Text,
		}
		end := c.end
		if end <= c.off {
			end = c.off + 1
		}
		all = append(all, entry{blk, c.off, end})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].off < all[j].off })

	// induk = blok terkecil lain yang memuat blok ini
	for i := range all {
		best := -1
		for j := range all {
			if i == j || all[j].off > all[i].off || all[j].end < all[i].end {
				continue
			}
			if all[j].off == all[i].off && all[j].end == all[i].end && j > i {
				continue // rentang sama persis: yang lebih dulu jadi induk
			}
			if best < 0 || all[j].end-all[j].off < all[best].end-all[best].off {
				best = j
			}
		}
		if best >= 0 {
			all[i].blk.Parent = all[best].blk.ID
			all[best].blk.Children = append(all[best].blk.Children, all[i].blk.ID)
		}
	}
	out := make([]scene.Block, len(all))
	for i, e := range all {
		out[i] = e.blk
	}
	return out
}

func (b *builder) block(c *blockCand) scene.Block {
	blk := scene.Block{ID: c.id, Kind: c.kind, State: scene.StateComplete}
	blk.Line = b.src.line(c.node.Pos())
	blk.EndLine = max(blk.Line, b.src.line(c.node.End()-1))
	head := func(body *ast.BlockStmt) string {
		start := b.src.offset(c.node.Pos())
		end := b.src.lineEnd(start)
		if body != nil && b.isChar(body.Lbrace, '{') {
			end = b.src.offset(body.Lbrace)
		}
		return short(oneLine(b.src.slice(start, end)))
	}
	mark := func(body *ast.BlockStmt) {
		if body == nil || !b.isChar(body.Lbrace, '{') || b.incomplete(c.node, body) {
			blk.State = scene.StateIncomplete
		}
	}

	switch n := c.node.(type) {
	case *ast.FuncDecl:
		blk.Name = n.Name.Name
		blk.Header = head(n.Body)
		blk.Params = paramNames(n.Type)
		blk.Body = b.nodes(n.Body.List)
		mark(n.Body)
	case *ast.FuncLit:
		blk.Name = "func"
		blk.Header = head(n.Body)
		blk.Params = paramNames(n.Type)
		blk.Body = b.nodes(n.Body.List)
		mark(n.Body)
	case *ast.IfStmt:
		blk.Header = head(n.Body)
		mark(n.Body)
		for cur, kind := n, scene.BranchIf; cur != nil; kind = scene.BranchElseIf {
			blk.Branches = append(blk.Branches, scene.Branch{
				Kind: kind, Label: b.text(cur.Cond), Line: b.src.line(cur.Pos()), Body: b.bodyNodes(cur.Body),
			})
			switch el := cur.Else.(type) {
			case *ast.IfStmt:
				cur = el
				continue
			case *ast.BlockStmt:
				blk.Branches = append(blk.Branches, scene.Branch{Kind: scene.BranchElse, Line: b.src.line(el.Pos()), Body: b.bodyNodes(el)})
			}
			cur = nil
		}
	case *ast.SwitchStmt:
		blk.Header = head(n.Body)
		mark(n.Body)
		blk.Branches = b.caseBranches(n.Body)
	case *ast.TypeSwitchStmt:
		blk.Header = head(n.Body)
		mark(n.Body)
		blk.Branches = b.caseBranches(n.Body)
	case *ast.TypeSpec:
		blk.Name = n.Name.Name
		blk.Header = "type " + n.Name.Name + " struct"
		if st, ok := n.Type.(*ast.StructType); ok && (st.Fields == nil || b.incomplete(n, &ast.BlockStmt{Rbrace: st.Fields.Closing})) {
			blk.State = scene.StateIncomplete
		}
	}
	return blk
}

func (b *builder) bodyNodes(body *ast.BlockStmt) []scene.Node {
	if body == nil {
		return []scene.Node{}
	}
	return b.nodes(body.List)
}

func (b *builder) caseBranches(body *ast.BlockStmt) []scene.Branch {
	if body == nil {
		return nil
	}
	var out []scene.Branch
	for _, s := range body.List {
		cc, ok := s.(*ast.CaseClause)
		if !ok {
			continue
		}
		br := scene.Branch{Kind: scene.BranchCase, Line: b.src.line(cc.Pos()), Body: b.nodes(cc.Body)}
		if cc.List == nil {
			br.Kind = scene.BranchDefault
		} else {
			for i, x := range cc.List {
				if i > 0 {
					br.Label += ", "
				}
				br.Label += b.text(x)
			}
		}
		out = append(out, br)
	}
	return out
}

func paramNames(ft *ast.FuncType) []string {
	var out []string
	if ft == nil || ft.Params == nil {
		return nil
	}
	for _, f := range ft.Params.List {
		for _, n := range f.Names {
			out = append(out, n.Name)
		}
	}
	return out
}
