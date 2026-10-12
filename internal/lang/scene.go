package lang

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"gopherviz/internal/ir"
	"gopherviz/internal/scene"
)

// Result adalah hasil analisis satu file non-Go: Scene untuk tampilan dan IR untuk simulasi.
type Result struct {
	Src         string // teks sumber (untuk simulator)
	Scene       scene.Scene
	Diagnostics []scene.Diagnostic
	File        *ir.File
	Nodes       map[string]ir.Node // ID blok → node IR (*ir.Func, *ir.If, *ir.Switch, *ir.For, ...)
	IDs         map[ir.Node]string
}

// Analyze mem-parse src lalu membangun Scene v2. Tidak pernah panik.
func Analyze(language, filename, src string) (res *Result) {
	res = &Result{Src: src, Nodes: map[string]ir.Node{}, IDs: map[ir.Node]string{}}
	res.Scene = scene.Scene{Version: scene.Version, Language: language, File: filename, Blocks: []scene.Block{}, Loops: []scene.Loop{}}
	defer func() {
		if r := recover(); r != nil {
			res.Diagnostics = append(res.Diagnostics, scene.Diagnostic{Line: 1, Message: fmt.Sprintf("internal analyzer error: %v", r)})
		}
	}()
	f := Parse(language, src)
	res.File = f
	b := &builder{src: src, res: res, labels: map[ir.Stmt]string{}, elseIf: map[*ir.If]bool{}}
	for _, d := range f.Diags {
		res.Diagnostics = append(res.Diagnostics, scene.Diagnostic{Line: d.Pos.Line, Col: d.Pos.Col, Message: d.Msg})
	}
	b.collect(f)
	b.build()
	return res
}

type builder struct {
	src    string
	res    *Result
	labels map[ir.Stmt]string
	elseIf map[*ir.If]bool
	order  []ir.Node
}

// IsClassBlock: class/struct bernama menjadi blok (interface, enum, trait, dan class anonim tidak).
func IsClassBlock(c *ir.Class) bool {
	return c.Name != "" && (c.Kind == "class" || c.Kind == "struct" || c.Kind == "")
}

// collect memberi ID pada fungsi (berbadan blok), kepala rantai if, switch, dan loop
// dengan urutan kemunculan di sumber.
func (b *builder) collect(f *ir.File) {
	counts := map[string]int{}
	add := func(n ir.Node, prefix string) {
		counts[prefix]++
		id := fmt.Sprintf("%s%d", prefix, counts[prefix])
		b.res.Nodes[id] = n
		b.res.IDs[n] = id
		b.order = append(b.order, n)
	}
	for _, s := range f.Body {
		ir.Walk(s, func(n ir.Node) bool {
			switch x := n.(type) {
			case *ir.Func:
				if x.Body != nil {
					add(x, "F")
				}
			case *ir.If:
				if el, ok := x.Else.(*ir.If); ok {
					b.elseIf[el] = true
				}
				if !b.elseIf[x] {
					add(x, "I")
				}
			case *ir.Switch:
				add(x, "S")
			case *ir.Class:
				if IsClassBlock(x) {
					add(x, "C")
				}
			case *ir.For, *ir.ForEach, *ir.While:
				add(x, "L")
			case *ir.Labeled:
				b.labels[x.Stmt] = x.Label
			}
			return true
		})
	}
}

func (b *builder) build() {
	type entry struct {
		blk      scene.Block
		off, end int
	}
	var all []entry
	for _, n := range b.order {
		sp := n.Span()
		end := sp.End.Off
		if end <= sp.Start.Off {
			end = sp.Start.Off + 1
		}
		var blk scene.Block
		switch x := n.(type) {
		case *ir.For, *ir.ForEach, *ir.While:
			l := b.loop(x.(ir.Stmt))
			b.res.Scene.Loops = append(b.res.Scene.Loops, l)
			blk = scene.Block{ID: l.ID, Kind: scene.BlockLoop, Line: l.Line, EndLine: l.EndLine, State: l.State, Name: l.Label, Header: l.Header.Text}
		default:
			blk = b.block(n)
		}
		all = append(all, entry{blk, sp.Start.Off, end})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].off < all[j].off })
	for i := range all {
		best := -1
		for j := range all {
			if i == j || all[j].off > all[i].off || all[j].end < all[i].end {
				continue
			}
			if all[j].off == all[i].off && all[j].end == all[i].end && j > i {
				continue
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
	for _, e := range all {
		b.res.Scene.Blocks = append(b.res.Scene.Blocks, e.blk)
	}
	// loop: Parent/Children hanya antar loop (seperti analyzer Go)
	loopIdx := map[string]int{}
	for i, l := range b.res.Scene.Loops {
		loopIdx[l.ID] = i
	}
	for _, e := range all {
		if e.blk.Kind != scene.BlockLoop {
			continue
		}
		for p := e.blk.Parent; p != ""; {
			if pi, ok := loopIdx[p]; ok {
				b.res.Scene.Loops[loopIdx[e.blk.ID]].Parent = p
				b.res.Scene.Loops[pi].Children = append(b.res.Scene.Loops[pi].Children, e.blk.ID)
				break
			}
			pb, _ := b.res.Scene.Block(p)
			for _, x := range all {
				if x.blk.ID == p {
					pb = x.blk
				}
			}
			p = pb.Parent
		}
	}
}

// ---------- teks ----------

func (b *builder) raw(s ir.Span) string {
	if s.Start.Off < 0 || s.End.Off > len(b.src) || s.Start.Off >= s.End.Off {
		return ""
	}
	return b.src[s.Start.Off:s.End.Off]
}

func (b *builder) text(n ir.Node) string {
	if n == nil {
		return ""
	}
	return oneLine(b.raw(n.Span()))
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + " …"
	}
	return s
}

const maxText = 60

func short(s string) string {
	if utf8.RuneCountInString(s) <= maxText {
		return s
	}
	r := []rune(s)
	return string(r[:maxText-1]) + "…"
}

// headerText: teks kepala (sampai sebelum badan), tanpa ":" Python / "{" di akhir.
func (b *builder) headerText(s ir.Span) string {
	t := oneLine(b.raw(s))
	t = strings.TrimSuffix(strings.TrimSpace(t), "{")
	t = strings.TrimSuffix(strings.TrimSpace(t), ":")
	return strings.TrimSpace(t)
}

// ---------- kelengkapan ----------

// incomplete: badan belum ditutup, ada bagian yang gagal di-parse, atau ada diagnostik
// di dalam rentang node.
func (b *builder) incomplete(n ir.Node, body ir.Stmt) bool {
	if blk, ok := body.(*ir.Block); ok && blk != nil && !blk.Closed {
		return true
	}
	if body == nil {
		return true
	}
	sp := n.Span()
	for _, d := range b.res.File.Diags {
		if d.Pos.Off >= sp.Start.Off && d.Pos.Off <= sp.End.Off && d.Pos.Line >= sp.Start.Line && d.Pos.Line <= sp.End.Line {
			return true
		}
	}
	bad := false
	ir.Walk(n, func(m ir.Node) bool {
		switch m.(type) {
		case *ir.Bad, *ir.BadExpr:
			bad = true
		}
		return !bad
	})
	return bad
}

func state(incomplete bool) scene.State {
	if incomplete {
		return scene.StateIncomplete
	}
	return scene.StateComplete
}

// ---------- blok ----------

func (b *builder) block(n ir.Node) scene.Block {
	id := b.res.IDs[n]
	sp := n.Span()
	blk := scene.Block{ID: id, Line: sp.Start.Line, EndLine: max(sp.Start.Line, sp.End.Line), State: scene.StateComplete}
	switch x := n.(type) {
	case *ir.Func:
		blk.Kind = scene.BlockFunc
		blk.Name = x.Name
		if x.Name == "" {
			blk.Name = "(anonymous)"
		}
		if x.Class != "" {
			blk.Name = x.Class + "." + x.Name
		}
		blk.Header = short(b.headerText(x.Header))
		for _, p := range x.Params {
			if p.Name != "" {
				blk.Params = append(blk.Params, p.Name)
			}
		}
		blk.Body = b.nodes(x.Body.List)
		blk.State = state(b.incomplete(x, x.Body))
	case *ir.If:
		blk.Kind = scene.BlockIf
		blk.Header = short(b.ifHeader(x))
		blk.State = state(b.incomplete(x, x.Then))
		kind := scene.BranchIf
		for cur := x; cur != nil; kind = scene.BranchElseIf {
			blk.Branches = append(blk.Branches, scene.Branch{Kind: kind, Label: b.text(cur.Cond), Line: cur.S.Start.Line, Body: b.bodyNodes(cur.Then)})
			switch el := cur.Else.(type) {
			case *ir.If:
				cur = el
				continue
			case nil:
			default:
				blk.Branches = append(blk.Branches, scene.Branch{Kind: scene.BranchElse, Line: elseLine(b, el), Body: b.bodyNodes(el)})
			}
			cur = nil
		}
	case *ir.Class:
		blk.Kind = scene.BlockClass
		blk.Name = x.Name
		blk.Header = short(b.headerText(x.Header))
		if blk.Header == "" {
			blk.Header = x.Kind + " " + x.Name
		}
		blk.State = state(b.incomplete(x, &ir.Block{Closed: true}))
	case *ir.Switch:
		blk.Kind = scene.BlockSwitch
		blk.Header = short(b.headerText(x.Header))
		blk.State = state(b.incomplete(x, &ir.Block{Closed: true}))
		for _, c := range x.Cases {
			br := scene.Branch{Kind: scene.BranchCase, Line: c.S.Start.Line, Body: b.nodes(c.Body)}
			if c.Exprs == nil {
				br.Kind = scene.BranchDefault
			} else {
				var labels []string
				for _, e := range c.Exprs {
					labels = append(labels, b.text(e))
				}
				br.Label = strings.Join(labels, ", ")
			}
			if c.Guard != nil {
				br.Label = strings.TrimSpace(br.Label + " if " + b.text(c.Guard))
			}
			blk.Branches = append(blk.Branches, br)
		}
	}
	return blk
}

// elseLine: baris kata kunci else (satu baris sebelum badan bila badan diawali "{" di baris lain).
func elseLine(b *builder, el ir.Stmt) int {
	line := el.Span().Start.Line
	off := el.Span().Start.Off
	// mundur ke kata "else" terdekat
	if i := strings.LastIndex(b.src[:max(0, min(off, len(b.src)))], "else"); i >= 0 {
		l := 1 + strings.Count(b.src[:i], "\n")
		if l <= line && line-l <= 2 {
			return l
		}
	}
	return line
}

func (b *builder) ifHeader(x *ir.If) string {
	then := x.Then.Span().Start.Off
	s := ir.Span{Start: x.S.Start, End: x.Then.Span().Start}
	if then <= x.S.Start.Off {
		return "if " + b.text(x.Cond)
	}
	return b.headerText(s)
}

func (b *builder) bodyNodes(s ir.Stmt) []scene.Node {
	switch x := s.(type) {
	case nil:
		return []scene.Node{}
	case *ir.Block:
		return b.nodes(x.List)
	default:
		return b.nodes([]ir.Stmt{x})
	}
}

// ---------- loop ----------

func (b *builder) loop(n ir.Stmt) scene.Loop {
	id := b.res.IDs[n]
	sp := n.Span()
	l := scene.Loop{ID: id, Label: b.labels[n], Line: sp.Start.Line, EndLine: max(sp.Start.Line, sp.End.Line), Body: []scene.Node{}}
	var body ir.Stmt
	switch x := n.(type) {
	case *ir.For:
		body = x.Body
		l.Header.Text = b.headerText(x.Header)
		l.Header.Init = b.initText(x.Init)
		l.Header.Cond = b.text(x.Cond)
		l.Header.Post = b.text(x.Post)
		switch {
		case x.Cond == nil:
			l.Kind = scene.KindInfinite
		case x.Init == nil && x.Post == nil:
			l.Kind = scene.KindCond
		default:
			l.Kind = scene.KindFor3
		}
		l.Vars = initVars(x.Init)
	case *ir.While:
		body = x.Body
		l.Header.Text = b.headerText(x.Header)
		l.Header.Cond = b.text(x.Cond)
		switch {
		case x.Do:
			l.Kind = scene.KindDoWhile
			l.Header.Text = "do … while (" + l.Header.Cond + ")"
		case isTrue(x.Cond):
			l.Kind = scene.KindInfinite
		default:
			l.Kind = scene.KindCond
		}
	case *ir.ForEach:
		body = x.Body
		l.Header.Text = b.headerText(x.Header)
		l.Header.Key = b.text(x.Key)
		l.Header.Value = b.text(x.Value)
		l.Header.Range = b.text(x.X)
		l.Header.Define = x.Decl != "" || x.Kind == "py" || x.Kind == "foreach"
		l.Kind = foreachKind(x)
		l.Vars = append(targetNames(x.Key), targetNames(x.Value)...)
	}
	l.Body = b.bodyNodes(body)
	if body == nil {
		l.Body = []scene.Node{}
	}
	l.State = state(b.incomplete(n, body))
	return l
}

func (b *builder) initText(s ir.Stmt) string {
	if s == nil {
		return ""
	}
	return oneLine(b.raw(s.Span()))
}

func initVars(s ir.Stmt) []string {
	switch x := s.(type) {
	case *ir.VarDecl:
		var out []string
		for _, sp := range x.Specs {
			if sp.Name != "" {
				out = append(out, sp.Name)
			}
		}
		return out
	case *ir.ExprStmt:
		var out []string
		var visit func(e ir.Expr)
		visit = func(e ir.Expr) {
			switch e := e.(type) {
			case *ir.Assign:
				out = append(out, targetNames(e.L)...)
			case *ir.Seq:
				for _, y := range e.List {
					visit(y)
				}
			}
		}
		visit(x.X)
		return out
	}
	return nil
}

func targetNames(e ir.Expr) []string {
	var out []string
	switch x := e.(type) {
	case nil:
	case *ir.Ident:
		out = append(out, x.Name)
	case *ir.ArrayLit:
		for _, el := range x.Elems {
			out = append(out, targetNames(el)...)
		}
	case *ir.ObjectLit:
		for _, p := range x.Props {
			out = append(out, targetNames(p.Value)...)
		}
	case *ir.Spread:
		out = append(out, targetNames(x.X)...)
	case *ir.Assign:
		out = append(out, targetNames(x.L)...)
	}
	return out
}

func isTrue(e ir.Expr) bool {
	if l, ok := e.(*ir.Lit); ok {
		switch l.Kind {
		case ir.LitBool:
			return l.Value == "true"
		case ir.LitInt:
			return l.Value != "0"
		}
	}
	return false
}

// foreachKind menebak jenis loop for-each dari bentuk sintaksnya.
func foreachKind(x *ir.ForEach) scene.Kind {
	switch x.Kind {
	case "in":
		return scene.KindRangeKeys
	case "foreach":
		if x.Key != nil {
			return scene.KindRangeMap
		}
	}
	switch it := x.X.(type) {
	case *ir.Lit:
		if it.Kind == ir.LitString {
			return scene.KindRangeString
		}
	case *ir.Template:
		return scene.KindRangeString
	case *ir.Call:
		name := ""
		switch f := it.Fun.(type) {
		case *ir.Ident:
			name = f.Name
		case *ir.Member:
			name = f.Name
		}
		switch name {
		case "range":
			if x.Kind == "py" {
				return scene.KindRangeInt
			}
		case "items", "entries", "entrySet":
			return scene.KindRangeMap
		case "keys", "keySet":
			return scene.KindRangeKeys
		case "toCharArray", "chars":
			return scene.KindRangeString
		case "enumerate", "zip", "values":
			return scene.KindRangeSlice
		}
		return scene.KindRangeUnknown
	case *ir.ArrayLit, *ir.Comp:
		return scene.KindRangeSlice
	case *ir.ObjectLit:
		if it.Kind == "dict" {
			return scene.KindRangeKeys
		}
		return scene.KindRangeMap
	}
	return scene.KindRangeSlice
}

// ---------- isi badan ----------

func (b *builder) nodes(list []ir.Stmt) []scene.Node {
	out := []scene.Node{}
	for _, st := range list {
		if st == nil {
			continue
		}
		n, ok := b.node(st)
		if !ok {
			continue
		}
		sp := st.Span()
		if sp.End.Line > n.Line {
			n.EndLine = sp.End.Line
		}
		out = append(out, n)
	}
	return out
}

func (b *builder) node(st ir.Stmt) (scene.Node, bool) {
	line := st.Span().Start.Line
	switch s := st.(type) {
	case *ir.Simple:
		if s.Kind == "empty" {
			return scene.Node{}, false
		}
	case *ir.Labeled:
		return b.node(s.Stmt)
	case *ir.For, *ir.ForEach, *ir.While:
		return scene.Node{Kind: scene.NodeLoop, Line: line, Ref: b.res.IDs[st]}, true
	case *ir.Branch:
		n := scene.Node{Line: line, Text: b.text(s), Label: s.Label}
		switch s.Tok {
		case "break":
			n.Kind = scene.NodeBreak
		case "continue":
			n.Kind = scene.NodeContinue
		case "goto":
			n.Kind = scene.NodeGoto
		default:
			n.Kind = scene.NodeStmt
		}
		return n, true
	case *ir.Return:
		return scene.Node{Kind: scene.NodeReturn, Line: line, Text: short(oneLine(firstLine(b.raw(s.S))))}, true
	case *ir.If:
		n := scene.Node{Kind: scene.NodeIf, Line: line, Ref: b.res.IDs[s], Text: short(b.ifHeader(s))}
		n.Then = b.bodyNodes(s.Then)
		switch e := s.Else.(type) {
		case nil:
		case *ir.If:
			if en, ok := b.node(e); ok {
				n.Else = []scene.Node{en}
			}
		default:
			n.Else = b.bodyNodes(e)
		}
		return n, true
	case *ir.Block:
		return scene.Node{Kind: scene.NodeBlock, Line: line, Body: b.nodes(s.List)}, true
	case *ir.Switch:
		return scene.Node{Kind: scene.NodeSwitch, Line: line, Ref: b.res.IDs[s], Text: short(b.headerText(s.Header))}, true
	case *ir.Bad:
		return scene.Node{Kind: scene.NodeBad, Line: line, Text: short(oneLine(firstLine(b.raw(s.S))))}, true
	}
	return scene.Node{Kind: scene.NodeStmt, Line: line, Text: short(oneLine(firstLine(b.raw(st.Span()))))}, true
}
