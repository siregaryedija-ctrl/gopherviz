package python

import (
	"fmt"
	"strings"

	"gopherviz/internal/ir"
)

const maxDiags = 40

type parser struct {
	src   string
	toks  []token
	i     int
	diags []ir.Diag
	depth int
}

// Parse mem-parse kode Python. Tidak pernah panik: kegagalan internal menjadi Diag.
func Parse(src string) (f *ir.File) {
	f = &ir.File{Lang: "python"}
	defer func() {
		if r := recover(); r != nil {
			f.Diags = append(f.Diags, ir.Diag{Pos: ir.Pos{Line: 1, Col: 1}, Msg: fmt.Sprintf("kesalahan internal parser: %v", r)})
		}
	}()
	toks, ldiags := lex(src)
	p := &parser{src: src, toks: toks, diags: ldiags}
	for !p.at(tEOF) {
		before := p.i
		if p.at(tIndent) {
			p.errAt(p.peekTok(1).pos, "indentasi tidak terduga")
			p.next()
			continue
		}
		if p.at(tDedent) || p.at(tNewline) {
			p.next()
			continue
		}
		f.Body = append(f.Body, p.stmt()...)
		if p.i == before {
			p.badLine("tidak bisa dibaca di sini: " + p.tok().text)
		}
	}
	f.Diags = p.diags
	return f
}

// ---------- token ----------

func (p *parser) tok() *token { return &p.toks[p.i] }

func (p *parser) peekTok(k int) *token {
	if p.i+k < len(p.toks) {
		return &p.toks[p.i+k]
	}
	return &p.toks[len(p.toks)-1]
}

func (p *parser) at(k tokKind) bool { return p.tok().k == k }

func (p *parser) is(s string) bool {
	t := p.tok()
	return (t.k == tOp || t.k == tName) && t.text == s
}

func (p *parser) isAt(k int, s string) bool {
	t := p.peekTok(k)
	return (t.k == tOp || t.k == tName) && t.text == s
}

func (p *parser) next() *token {
	t := &p.toks[p.i]
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}

func (p *parser) accept(s string) bool {
	if p.is(s) {
		p.next()
		return true
	}
	return false
}

func (p *parser) expect(s, what string) bool {
	if p.accept(s) {
		return true
	}
	p.errAt(p.tok().pos, fmt.Sprintf("kurang %q %s", s, what))
	return false
}

func (p *parser) prevEnd() ir.Pos {
	if p.i == 0 {
		return p.toks[0].pos
	}
	return p.toks[p.i-1].end
}

func (p *parser) span(start ir.Pos) ir.Span { return ir.Span{Start: start, End: p.prevEnd()} }

func (p *parser) errAt(pos ir.Pos, msg string) {
	if len(p.diags) >= maxDiags {
		return
	}
	for _, d := range p.diags {
		if d.Pos.Line == pos.Line && d.Msg == msg {
			return
		}
	}
	p.diags = append(p.diags, ir.Diag{Pos: pos, Msg: msg})
}

func (p *parser) badLine(msg string) ir.Stmt {
	start := p.tok().pos
	p.errAt(start, msg)
	for !p.at(tEOF) && !p.at(tNewline) && !p.at(tDedent) && !p.at(tIndent) {
		p.next()
	}
	p.accept("\n")
	if p.at(tNewline) {
		p.next()
	}
	return &ir.Bad{S: p.span(start), Msg: msg}
}

func (p *parser) text(s ir.Span) string {
	if s.Start.Off < 0 || s.End.Off > len(p.src) || s.Start.Off > s.End.Off {
		return ""
	}
	return p.src[s.Start.Off:s.End.Off]
}

func (t *token) spanOf() ir.Span { return ir.Span{Start: t.pos, End: t.end} }

// endLine mengakhiri statement sederhana: NEWLINE (atau ';' bila ada statement lain).
func (p *parser) endLine() {
	switch {
	case p.at(tNewline):
		p.next()
	case p.at(tEOF), p.at(tDedent):
	default:
		p.badLine(fmt.Sprintf("tidak terduga: %q", p.tok().text))
	}
}

// ---------- statement ----------

func (p *parser) stmt() []ir.Stmt {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 150 {
		return []ir.Stmt{p.badLine("kode terlalu dalam bersarang")}
	}
	start := p.tok().pos
	if p.at(tName) {
		switch p.tok().text {
		case "if":
			return []ir.Stmt{p.ifStmt(false)}
		case "while":
			p.next()
			cond := p.test()
			hdr := p.span(start)
			body := p.suite("while")
			w := &ir.While{Cond: cond, Body: body, Header: hdr}
			if p.is("else") && p.isAt(1, ":") {
				p.next()
				w.Else = p.suite("else")
			}
			w.S = p.span(start)
			return []ir.Stmt{w}
		case "for":
			return []ir.Stmt{p.forStmt(start)}
		case "def":
			return []ir.Stmt{p.funcDef(start, false)}
		case "class":
			return []ir.Stmt{p.classDef(start)}
		case "try":
			return []ir.Stmt{p.tryStmt()}
		case "with":
			return []ir.Stmt{p.withStmt(start)}
		case "async":
			if p.isAt(1, "def") || p.isAt(1, "for") || p.isAt(1, "with") {
				p.next()
				if p.is("def") {
					return []ir.Stmt{p.funcDef(start, true)}
				}
				return p.stmt()
			}
		case "match":
			// "match" hanya kata kunci bila diikuti subjek lalu ":" di akhir baris
			if s := p.tryMatch(start); s != nil {
				return []ir.Stmt{s}
			}
		}
	}
	if p.is("@") { // dekorator: @property, @staticmethod, @x.setter, ...
		var decor []string
		for p.accept("@") {
			x := p.test()
			decor = append(decor, strings.TrimSpace(p.text(x.Span())))
			p.endLine()
		}
		out := p.stmt()
		if len(out) == 1 {
			if f, ok := out[0].(*ir.Func); ok {
				f.Decor = decor
			}
		}
		return out
	}
	return p.simpleStmts()
}

// simpleStmts membaca satu baris statement sederhana (bisa dipisah ';').
func (p *parser) simpleStmts() []ir.Stmt {
	var out []ir.Stmt
	for {
		out = append(out, p.smallStmt())
		if !p.accept(";") || p.at(tNewline) || p.at(tEOF) {
			break
		}
	}
	p.endLine()
	return out
}

func (p *parser) smallStmt() ir.Stmt {
	t := p.tok()
	start := t.pos
	if t.k == tName {
		switch t.text {
		case "pass":
			p.next()
			return &ir.Simple{S: p.span(start), Kind: "pass"}
		case "break", "continue":
			p.next()
			return &ir.Branch{S: p.span(start), Tok: t.text}
		case "return":
			p.next()
			var x ir.Expr
			if !p.lineEnd() {
				x = p.exprList()
			}
			return &ir.Return{S: p.span(start), X: x}
		case "raise":
			p.next()
			var x ir.Expr
			if !p.lineEnd() {
				x = p.test()
				if p.accept("from") {
					p.test()
				}
			}
			return &ir.Throw{S: p.span(start), X: x}
		case "global", "nonlocal":
			p.next()
			s := &ir.Simple{Kind: t.text}
			for p.at(tName) {
				s.Names = append(s.Names, p.next().text)
				if !p.accept(",") {
					break
				}
			}
			s.S = p.span(start)
			return s
		case "del":
			p.next()
			x := p.exprList()
			s := &ir.Simple{Kind: "del", X: tupleItems(x)}
			s.S = p.span(start)
			return s
		case "assert":
			p.next()
			s := &ir.Simple{Kind: "assert", X: []ir.Expr{p.test()}}
			if p.accept(",") {
				s.X = append(s.X, p.test())
			}
			s.S = p.span(start)
			return s
		case "import", "from":
			s := &ir.Simple{Kind: "import"}
			for !p.lineEnd() && !p.is(";") {
				tt := p.next()
				if tt.k == tName && tt.text != "import" && tt.text != "from" && tt.text != "as" {
					s.Names = append(s.Names, tt.text)
				}
			}
			s.S = p.span(start)
			return s
		}
	}
	// ekspresi / assignment
	x := p.exprListStar()
	switch {
	case p.is(":") && !p.isAt(1, "="): // anotasi: x: int = 5
		p.next()
		typ := p.test()
		if p.accept("=") {
			r := p.exprListStar()
			return &ir.VarDecl{S: p.span(start), Kind: "annot", Specs: []*ir.VarSpec{{S: p.span(start), Name: nameOf(x), Pattern: patternOf(x), Type: p.text(typ.Span()), Init: r}}}
		}
		return &ir.VarDecl{S: p.span(start), Kind: "annot", Specs: []*ir.VarSpec{{S: p.span(start), Name: nameOf(x), Pattern: patternOf(x), Type: p.text(typ.Span())}}}
	case p.is("="):
		// a = b = nilai → Assign(a, Assign(b, nilai))
		targets := []ir.Expr{x}
		var val ir.Expr
		for p.accept("=") {
			v := p.exprListStar()
			targets = append(targets, v)
		}
		val = targets[len(targets)-1]
		targets = targets[:len(targets)-1]
		for k := len(targets) - 1; k >= 0; k-- {
			val = &ir.Assign{S: p.span(start), Op: "=", L: targets[k], R: val}
		}
		return &ir.ExprStmt{S: p.span(start), X: val}
	case p.tok().k == tOp && isAugAssign(p.tok().text):
		op := p.next().text
		r := p.exprList()
		return &ir.ExprStmt{S: p.span(start), X: &ir.Assign{S: p.span(start), Op: op, L: x, R: r}}
	}
	return &ir.ExprStmt{S: p.span(start), X: x}
}

func nameOf(x ir.Expr) string {
	if id, ok := x.(*ir.Ident); ok {
		return id.Name
	}
	return ""
}

func patternOf(x ir.Expr) ir.Expr {
	if _, ok := x.(*ir.Ident); ok {
		return nil
	}
	return x
}

func isAugAssign(s string) bool {
	switch s {
	case "+=", "-=", "*=", "/=", "//=", "%=", "**=", "&=", "|=", "^=", ">>=", "<<=", "@=":
		return true
	}
	return false
}

func (p *parser) lineEnd() bool {
	return p.at(tNewline) || p.at(tEOF) || p.at(tDedent) || p.is(";")
}

// suite membaca ":" lalu badan berindentasi (atau statement di baris yang sama).
func (p *parser) suite(what string) *ir.Block {
	start := p.tok().pos
	b := &ir.Block{}
	if !p.expect(":", "di akhir baris "+what) {
		// lompati sisa baris kepala
		for !p.at(tNewline) && !p.at(tEOF) {
			p.next()
		}
	}
	if !p.at(tNewline) && !p.at(tEOF) {
		// satu baris: if x: return 1
		b.List = p.simpleStmts()
		b.Closed = true
		b.S = p.span(start)
		return b
	}
	if p.at(tNewline) {
		p.next()
	}
	if !p.at(tIndent) {
		p.errAt(p.prevEnd(), "badan "+what+" belum ditulis (harus menjorok ke dalam)")
		b.S = p.span(start)
		return b
	}
	p.next()
	for !p.at(tDedent) && !p.at(tEOF) {
		before := p.i
		if p.at(tNewline) {
			p.next()
			continue
		}
		if p.at(tIndent) {
			p.errAt(p.tok().pos, "indentasi tidak terduga")
			p.next()
			continue
		}
		b.List = append(b.List, p.stmt()...)
		if p.i == before {
			b.List = append(b.List, p.badLine("tidak bisa dibaca di sini"))
		}
	}
	if p.at(tDedent) {
		p.next()
	}
	b.Closed = true
	b.S = p.span(start)
	return b
}

func (p *parser) ifStmt(elif bool) ir.Stmt {
	start := p.tok().pos
	p.next() // if / elif
	cond := p.namedExpr()
	n := &ir.If{Cond: cond, Elif: elif}
	n.Then = p.suite("if")
	switch {
	case p.is("elif"):
		n.Else = p.ifStmt(true)
	case p.is("else"):
		p.next()
		n.Else = p.suite("else")
	}
	n.S = p.span(start)
	return n
}

func (p *parser) forStmt(start ir.Pos) ir.Stmt {
	p.next()
	fe := &ir.ForEach{Kind: "py"}
	fe.Value = p.targetList()
	if p.expect("in", "di for") {
		fe.X = p.exprListStar()
	} else {
		fe.X = &ir.BadExpr{S: p.span(start), Msg: "for tanpa in"}
	}
	fe.Header = p.span(start)
	fe.Body = p.suite("for")
	if p.is("else") && p.isAt(1, ":") {
		p.next()
		fe.Else = p.suite("else")
	}
	fe.S = p.span(start)
	return fe
}

// targetList membaca target for/comprehension: x, (a, b), i, x — berhenti di "in".
func (p *parser) targetList() ir.Expr {
	start := p.tok().pos
	var items []ir.Expr
	for {
		items = append(items, p.starOr(func() ir.Expr { return p.binary(precBitOr) }))
		if !p.accept(",") || p.is("in") {
			break
		}
	}
	if len(items) == 1 {
		return items[0]
	}
	return &ir.ArrayLit{S: p.span(start), Elems: items, Kind: "tuple"}
}

func (p *parser) funcDef(start ir.Pos, async bool) ir.Stmt {
	p.next() // def
	f := &ir.Func{Async: async}
	if p.at(tName) {
		f.Name = p.next().text
	} else {
		p.errAt(p.tok().pos, "kurang nama fungsi")
	}
	if p.is("[") { // parameter tipe generik Python 3.12
		for !p.is("]") && !p.at(tEOF) {
			p.next()
		}
		p.next()
	}
	f.Params = p.params(")")
	if p.accept("->") {
		f.Ret = p.text(p.test().Span())
	}
	f.Header = p.span(start)
	f.Body = p.suite("def")
	f.S = p.span(start)
	return f
}

// params membaca parameter def (sampai ")") atau lambda (sampai ":").
func (p *parser) params(close string) []*ir.Param {
	var out []*ir.Param
	if close == ")" && !p.expect("(", "setelah nama fungsi") {
		return nil
	}
	for !p.is(close) && !p.at(tEOF) && !p.at(tNewline) {
		pr := &ir.Param{}
		switch {
		case p.accept("**"):
			pr.KwRest = true
		case p.accept("*"):
			pr.Rest = true
			if p.is(",") { // penanda keyword-only
				p.next()
				continue
			}
		case p.accept("/"): // penanda positional-only
			p.accept(",")
			continue
		}
		if !p.at(tName) {
			p.errAt(p.tok().pos, "kurang nama parameter")
			break
		}
		pr.Name = p.next().text
		if close == ")" && p.accept(":") {
			pr.Type = p.text(p.test().Span())
		}
		if p.accept("=") {
			pr.Default = p.test()
		}
		out = append(out, pr)
		if !p.accept(",") {
			break
		}
	}
	if close == ")" {
		p.expect(")", "untuk menutup parameter")
	}
	return out
}

func (p *parser) classDef(start ir.Pos) ir.Stmt {
	p.next()
	c := &ir.Class{Kind: "class"}
	if p.at(tName) {
		c.Name = p.next().text
	}
	if p.accept("(") {
		for !p.is(")") && !p.at(tEOF) && !p.at(tNewline) {
			x := p.argument()
			if id, ok := x.(*ir.Ident); ok {
				c.Extends = append(c.Extends, id.Name)
			} else if m, ok := x.(*ir.Member); ok {
				c.Extends = append(c.Extends, m.Name)
			}
			if !p.accept(",") {
				break
			}
		}
		p.expect(")", "untuk menutup daftar kelas induk")
	}
	c.Header = p.span(start)
	body := p.suite("class")
	for _, s := range body.List {
		if f, ok := s.(*ir.Func); ok {
			f.Class = c.Name
		}
		c.Members = append(c.Members, s)
	}
	c.S = p.span(start)
	return c
}

func (p *parser) tryStmt() ir.Stmt {
	start := p.tok().pos
	p.next()
	n := &ir.Try{Body: p.suite("try")}
	for p.is("except") {
		cs := p.tok().pos
		p.next()
		p.accept("*")
		c := &ir.Catch{}
		if !p.is(":") {
			x := p.test()
			for _, t := range tupleItems(x) {
				c.Types = append(c.Types, strings.TrimSpace(p.text(t.Span())))
			}
			if p.accept("as") && p.at(tName) {
				c.Name = p.next().text
			} else if p.accept(",") && p.at(tName) { // gaya Python 2
				c.Name = p.next().text
			}
		}
		c.Body = p.suite("except")
		c.S = p.span(cs)
		n.Catches = append(n.Catches, c)
	}
	if p.is("else") {
		p.next()
		n.Else = p.suite("else")
	}
	if p.is("finally") {
		p.next()
		n.Finally = p.suite("finally")
	}
	if len(n.Catches) == 0 && n.Finally == nil {
		p.errAt(start, "try harus diikuti except atau finally")
	}
	n.S = p.span(start)
	return n
}

// withStmt: with A() as a, B() as b: badan → blok berisi assignment lalu badan.
func (p *parser) withStmt(start ir.Pos) ir.Stmt {
	p.next()
	paren := p.is("(") && p.parenWithItems()
	if paren {
		p.next()
	}
	b := &ir.Block{}
	for !p.is(":") && !p.at(tNewline) && !p.at(tEOF) {
		is := p.tok().pos
		ctx := p.test()
		var target ir.Expr
		if p.accept("as") {
			target = p.binary(precBitOr)
		}
		s := &ir.Simple{S: p.span(is), Kind: "with", X: []ir.Expr{ctx}}
		if target != nil {
			s.X = append(s.X, target)
		}
		b.List = append(b.List, s)
		if !p.accept(",") {
			break
		}
	}
	if paren {
		p.expect(")", "")
	}
	body := p.suite("with")
	b.List = append(b.List, body.List...)
	b.Closed = body.Closed
	b.S = p.span(start)
	return b
}

func (p *parser) parenWithItems() bool {
	// with (open(a) as f, open(b) as g):  vs  with (x):
	depth := 0
	for k := p.i; k < len(p.toks); k++ {
		t := p.toks[k]
		if t.k == tNewline || t.k == tEOF {
			return false
		}
		if t.k == tOp && (t.text == "(" || t.text == "[" || t.text == "{") {
			depth++
		}
		if t.k == tOp && (t.text == ")" || t.text == "]" || t.text == "}") {
			depth--
			if depth == 0 {
				return k+1 < len(p.toks) && p.toks[k+1].k == tOp && p.toks[k+1].text == ":"
			}
		}
		if t.k == tName && t.text == "as" && depth == 1 {
			return true
		}
	}
	return false
}

// tryMatch: match subjek: \n case pola [if guard]: badan
func (p *parser) tryMatch(start ir.Pos) ir.Stmt {
	// cari ":" di akhir baris dengan NEWLINE + INDENT + "case"
	k := p.i + 1
	depth := 0
	for ; k < len(p.toks); k++ {
		t := p.toks[k]
		if t.k == tNewline || t.k == tEOF {
			return nil
		}
		if t.k == tOp && (t.text == "(" || t.text == "[" || t.text == "{") {
			depth++
		} else if t.k == tOp && (t.text == ")" || t.text == "]" || t.text == "}") {
			depth--
		} else if t.k == tOp && t.text == ":" && depth == 0 {
			break
		} else if t.k == tOp && t.text == "=" && depth == 0 {
			return nil // match = 5
		}
	}
	if k+2 >= len(p.toks) || p.toks[k+1].k != tNewline || p.toks[k+2].k != tIndent {
		return nil
	}
	if k == p.i+1 {
		return nil
	}
	p.next()
	sw := &ir.Switch{Match: true, Tag: p.exprListStar()}
	sw.Header = p.span(start)
	p.expect(":", "")
	p.next() // NEWLINE
	p.next() // INDENT
	for p.is("case") {
		cs := p.tok().pos
		p.next()
		c := &ir.Case{}
		pat := p.pattern()
		if id, ok := pat.(*ir.Ident); !ok || id.Name != "_" {
			c.Exprs = []ir.Expr{pat}
		}
		if p.accept("if") {
			c.Guard = p.namedExpr()
		}
		body := p.suite("case")
		c.Body = body.List
		c.S = p.span(cs)
		sw.Cases = append(sw.Cases, c)
		for p.at(tNewline) {
			p.next()
		}
	}
	if p.at(tDedent) {
		p.next()
	} else if !p.at(tEOF) {
		p.errAt(p.tok().pos, "di dalam match hanya boleh ada case")
	}
	sw.S = p.span(start)
	return sw
}

// pattern membaca pola case: literal, nama, A | B, [a, b], {"k": v}, Point(x=1).
func (p *parser) pattern() ir.Expr {
	start := p.tok().pos
	var items []ir.Expr
	for {
		x := p.binary(precCmp) // termasuk A | B
		if p.accept("as") && p.at(tName) {
			name := p.next()
			x = &ir.Assign{S: p.span(start), Op: "as", L: &ir.Ident{S: name.spanOf(), Name: name.text}, R: x}
		}
		items = append(items, x)
		if !p.accept(",") || p.is(":") || p.is("if") {
			break
		}
	}
	if len(items) == 1 {
		return items[0]
	}
	return &ir.ArrayLit{S: p.span(start), Elems: items, Kind: "tuple"}
}

// ---------- ekspresi ----------

const (
	precOr     = 10
	precAnd    = 20
	precNot    = 30
	precCmp    = 40
	precBitOr  = 50
	precBitXor = 60
	precBitAnd = 70
	precShift  = 80
	precArith  = 90
	precTerm   = 100
)

// exprList: a, b, c → tuple (tanpa *).
func (p *parser) exprList() ir.Expr { return p.listOf(p.test) }

// exprListStar: seperti exprList tetapi elemen boleh *x.
func (p *parser) exprListStar() ir.Expr {
	return p.listOf(func() ir.Expr { return p.starOr(p.namedExpr) })
}

func (p *parser) starOr(f func() ir.Expr) ir.Expr {
	if p.is("*") {
		start := p.tok().pos
		p.next()
		x := p.binary(precBitOr)
		return &ir.Spread{S: p.span(start), X: x}
	}
	return f()
}

func (p *parser) listOf(f func() ir.Expr) ir.Expr {
	start := p.tok().pos
	x := f()
	if !p.is(",") {
		return x
	}
	items := []ir.Expr{x}
	for p.accept(",") {
		if p.lineEnd() || p.is("=") || p.is(")") || p.is(":") || p.at(tOp) && isAugAssign(p.tok().text) {
			break
		}
		items = append(items, f())
	}
	return &ir.ArrayLit{S: p.span(start), Elems: items, Kind: "tuple"}
}

func tupleItems(x ir.Expr) []ir.Expr {
	if a, ok := x.(*ir.ArrayLit); ok && a.Kind == "tuple" {
		return a.Elems
	}
	return []ir.Expr{x}
}

// namedExpr: test atau walrus nama := nilai.
func (p *parser) namedExpr() ir.Expr {
	start := p.tok().pos
	if p.at(tName) && p.isAt(1, ":=") {
		name := p.next()
		p.next()
		v := p.test()
		return &ir.Assign{S: p.span(start), Op: ":=", L: &ir.Ident{S: name.spanOf(), Name: name.text}, R: v}
	}
	return p.test()
}

// test: lambda, atau a if c else b.
func (p *parser) test() ir.Expr {
	p.depth++
	defer func() { p.depth-- }()
	start := p.tok().pos
	if p.depth > 300 {
		p.next()
		return &ir.BadExpr{S: p.span(start), Msg: "ekspresi terlalu dalam"}
	}
	if p.is("lambda") {
		p.next()
		f := &ir.Func{Arrow: true, Params: p.params(":")}
		f.Header = p.span(start)
		p.expect(":", "di lambda")
		f.ExprBody = p.test()
		f.S = p.span(start)
		return f
	}
	x := p.binary(precOr)
	if p.is("if") {
		p.next()
		c := p.binary(precOr)
		if !p.expect("else", "di ekspresi ... if ... else ...") {
			return &ir.Cond{S: p.span(start), C: c, T: x, F: &ir.BadExpr{S: p.span(start)}}
		}
		f := p.test()
		return &ir.Cond{S: p.span(start), C: c, T: x, F: f}
	}
	return x
}

func (p *parser) binOp() (string, int) {
	t := p.tok()
	switch t.k {
	case tName:
		switch t.text {
		case "or":
			return "or", precOr
		case "and":
			return "and", precAnd
		case "in":
			return "in", precCmp
		case "is":
			if p.isAt(1, "not") {
				return "is not", precCmp
			}
			return "is", precCmp
		case "not":
			if p.isAt(1, "in") {
				return "not in", precCmp
			}
		}
	case tOp:
		switch t.text {
		case "<", ">", "==", ">=", "<=", "!=", "<>":
			return t.text, precCmp
		case "|":
			return "|", precBitOr
		case "^":
			return "^", precBitXor
		case "&":
			return "&", precBitAnd
		case "<<", ">>":
			return t.text, precShift
		case "+", "-":
			return t.text, precArith
		case "*", "/", "//", "%", "@":
			return t.text, precTerm
		}
	}
	return "", 0
}

func (p *parser) binary(min int) ir.Expr {
	start := p.tok().pos
	var x ir.Expr
	switch {
	case p.is("not") && min <= precNot:
		p.next()
		y := p.binary(precNot)
		x = &ir.Unary{S: p.span(start), Op: "not", X: y}
	default:
		x = p.unary()
	}
	var lastCmp *ir.Binary // perbandingan berantai: a < b < c → (a < b) and (b < c)
	for {
		op, prec := p.binOp()
		if prec == 0 || prec <= min {
			return x
		}
		p.next()
		if op == "is not" || op == "not in" {
			p.next()
		}
		y := p.binary(prec)
		if prec == precCmp {
			cmp := &ir.Binary{S: p.span(start), Op: op, X: x, Y: y}
			if lastCmp != nil {
				cmp.X = lastCmp.Y
				x = &ir.Binary{S: p.span(start), Op: "and", X: x, Y: cmp}
			} else {
				x = cmp
			}
			lastCmp = cmp
			continue
		}
		lastCmp = nil
		x = &ir.Binary{S: p.span(start), Op: op, X: x, Y: y}
	}
}

func (p *parser) unary() ir.Expr {
	start := p.tok().pos
	if t := p.tok(); t.k == tOp && (t.text == "-" || t.text == "+" || t.text == "~") {
		p.next()
		x := p.unary()
		return &ir.Unary{S: p.span(start), Op: t.text, X: x}
	}
	if p.is("await") {
		p.next()
		x := p.unary()
		return &ir.Unary{S: p.span(start), Op: "await", X: x}
	}
	x := p.postfix(p.atom(), start)
	if p.is("**") {
		p.next()
		y := p.unary()
		return &ir.Binary{S: p.span(start), Op: "**", X: x, Y: y}
	}
	return x
}

func (p *parser) postfix(x ir.Expr, start ir.Pos) ir.Expr {
	for {
		switch {
		case p.is("("):
			p.next()
			var args []ir.Expr
			for !p.is(")") && !p.at(tEOF) {
				args = append(args, p.argument())
				if !p.accept(",") {
					break
				}
			}
			// generator sebagai satu-satunya argumen: sum(x for x in xs)
			if len(args) == 1 && p.is("for") {
				args[0] = p.comprehension("gen", args[0], nil, nil, start)
			}
			p.expect(")", "untuk menutup argumen")
			x = &ir.Call{S: p.span(start), Fun: x, Args: args}
		case p.is("["):
			p.next()
			x = p.subscript(x, start)
		case p.is("."):
			p.next()
			if !p.at(tName) {
				p.errAt(p.tok().pos, "kurang nama setelah \".\"")
				return x
			}
			name := p.next().text
			x = &ir.Member{S: p.span(start), X: x, Name: name, Op: "."}
		default:
			return x
		}
	}
}

func (p *parser) argument() ir.Expr {
	start := p.tok().pos
	switch {
	case p.accept("**"):
		v := p.test()
		return &ir.KwArg{S: p.span(start), Value: v, Star2: true}
	case p.accept("*"):
		v := p.test()
		return &ir.Spread{S: p.span(start), X: v}
	case p.at(tName) && p.isAt(1, "="):
		name := p.next().text
		p.next()
		v := p.test()
		return &ir.KwArg{S: p.span(start), Name: name, Value: v}
	}
	return p.namedExpr()
}

// subscript: a[i], a[lo:hi:step], a[i, j]
func (p *parser) subscript(x ir.Expr, start ir.Pos) ir.Expr {
	var items []ir.Expr
	isSlice := false
	for !p.is("]") && !p.at(tEOF) {
		is := p.tok().pos
		var lo, hi, step ir.Expr
		if !p.is(":") {
			lo = p.namedExpr()
		}
		if p.accept(":") {
			isSlice = true
			if !p.is(":") && !p.is("]") && !p.is(",") {
				hi = p.test()
			}
			if p.accept(":") && !p.is("]") && !p.is(",") {
				step = p.test()
			}
			items = append(items, &ir.Slice{S: p.span(is), X: x, Lo: lo, Hi: hi, Step: step})
		} else {
			items = append(items, lo)
		}
		if !p.accept(",") {
			break
		}
	}
	p.expect("]", "untuk menutup indeks")
	if isSlice && len(items) == 1 {
		s := items[0].(*ir.Slice)
		s.S = p.span(start)
		return s
	}
	var idx ir.Expr
	if len(items) == 1 {
		idx = items[0]
	} else {
		idx = &ir.ArrayLit{S: p.span(start), Elems: items, Kind: "tuple"}
	}
	return &ir.Index{S: p.span(start), X: x, I: idx}
}

func (p *parser) atom() ir.Expr {
	t := p.tok()
	start := t.pos
	switch t.k {
	case tNum:
		p.next()
		return numLit(t)
	case tStr, tFStr:
		return p.strings()
	case tName:
		switch t.text {
		case "True", "False":
			p.next()
			return &ir.Lit{S: t.spanOf(), Kind: ir.LitBool, Raw: t.text, Value: strings.ToLower(t.text)}
		case "None":
			p.next()
			return &ir.Lit{S: t.spanOf(), Kind: ir.LitNull, Raw: t.text, Value: "None"}
		case "yield":
			p.next()
			op := "yield"
			if p.accept("from") {
				op = "yield from"
			}
			var x ir.Expr
			if !p.lineEnd() && !p.is(")") {
				x = p.exprList()
			}
			return &ir.Unary{S: p.span(start), Op: op, X: x}
		case "lambda", "not":
			return p.test()
		}
		if isKeyword(t.text) {
			p.errAt(t.pos, fmt.Sprintf("kata kunci %q tidak bisa dipakai di sini", t.text))
			p.next()
			return &ir.BadExpr{S: t.spanOf(), Msg: "kata kunci"}
		}
		p.next()
		return &ir.Ident{S: t.spanOf(), Name: t.text}
	case tOp:
		switch t.text {
		case "(":
			p.next()
			if p.accept(")") {
				return &ir.ArrayLit{S: p.span(start), Kind: "tuple"}
			}
			first := p.starOr(p.namedExpr)
			if p.is("for") {
				c := p.comprehension("gen", first, nil, nil, start)
				p.expect(")", "")
				return c
			}
			if !p.is(",") {
				p.expect(")", "untuk menutup kurung")
				return first
			}
			items := []ir.Expr{first}
			for p.accept(",") && !p.is(")") {
				items = append(items, p.starOr(p.namedExpr))
			}
			p.expect(")", "untuk menutup tuple")
			return &ir.ArrayLit{S: p.span(start), Elems: items, Kind: "tuple"}
		case "[":
			p.next()
			if p.accept("]") {
				return &ir.ArrayLit{S: p.span(start), Kind: "list"}
			}
			first := p.starOr(p.namedExpr)
			if p.is("for") {
				c := p.comprehension("list", first, nil, nil, start)
				p.expect("]", "")
				return c
			}
			items := []ir.Expr{first}
			for p.accept(",") && !p.is("]") {
				items = append(items, p.starOr(p.namedExpr))
			}
			p.expect("]", "untuk menutup list")
			return &ir.ArrayLit{S: p.span(start), Elems: items, Kind: "list"}
		case "{":
			return p.dictOrSet()
		case "...":
			p.next()
			return &ir.Ident{S: t.spanOf(), Name: "Ellipsis"}
		}
	case tNewline, tEOF, tDedent, tIndent:
		p.errAt(t.pos, "ekspresi belum selesai")
		return &ir.BadExpr{S: t.spanOf(), Msg: "ekspresi belum selesai"}
	}
	p.errAt(t.pos, "tidak bisa dibaca sebagai ekspresi: "+t.text)
	p.next()
	return &ir.BadExpr{S: p.span(start), Msg: "ekspresi tidak valid"}
}

func isKeyword(s string) bool {
	switch s {
	case "if", "elif", "else", "for", "while", "def", "class", "return", "break", "continue", "pass",
		"import", "from", "as", "try", "except", "finally", "raise", "with", "global", "nonlocal", "del",
		"assert", "in", "is", "and", "or":
		return true
	}
	return false
}

func (p *parser) dictOrSet() ir.Expr {
	start := p.tok().pos
	p.next()
	if p.accept("}") {
		return &ir.ObjectLit{S: p.span(start), Kind: "dict"}
	}
	// elemen pertama menentukan dict atau set
	if p.accept("**") {
		obj := &ir.ObjectLit{Kind: "dict"}
		obj.Props = append(obj.Props, &ir.Prop{Spread: true, Value: p.binary(precBitOr)})
		return p.dictRest(obj, start)
	}
	first := p.starOr(p.namedExpr)
	if p.accept(":") {
		v := p.test()
		if p.is("for") {
			c := p.comprehension("dict", nil, first, v, start)
			p.expect("}", "")
			return c
		}
		obj := &ir.ObjectLit{Kind: "dict", Props: []*ir.Prop{{S: p.span(start), Key: first, Value: v, Computed: true}}}
		return p.dictRest(obj, start)
	}
	if p.is("for") {
		c := p.comprehension("set", first, nil, nil, start)
		p.expect("}", "")
		return c
	}
	items := []ir.Expr{first}
	for p.accept(",") && !p.is("}") {
		items = append(items, p.starOr(p.namedExpr))
	}
	p.expect("}", "untuk menutup set")
	return &ir.ArrayLit{S: p.span(start), Elems: items, Kind: "set"}
}

func (p *parser) dictRest(obj *ir.ObjectLit, start ir.Pos) ir.Expr {
	for p.accept(",") && !p.is("}") {
		ps := p.tok().pos
		if p.accept("**") {
			obj.Props = append(obj.Props, &ir.Prop{S: p.span(ps), Spread: true, Value: p.binary(precBitOr)})
			continue
		}
		k := p.test()
		p.expect(":", "di dict")
		v := p.test()
		obj.Props = append(obj.Props, &ir.Prop{S: p.span(ps), Key: k, Value: v, Computed: true})
	}
	p.expect("}", "untuk menutup dict")
	obj.S = p.span(start)
	return obj
}

func (p *parser) comprehension(kind string, elt, key, val ir.Expr, start ir.Pos) ir.Expr {
	c := &ir.Comp{Kind: kind, Elt: elt, Key: key, Value: val}
	for p.is("for") || p.is("async") && p.isAt(1, "for") {
		p.accept("async")
		p.next()
		f := &ir.CompFor{Target: p.targetList()}
		p.expect("in", "di comprehension")
		f.Iter = p.binary(precOr)
		for p.is("if") {
			p.next()
			f.Ifs = append(f.Ifs, p.binary(precOr))
		}
		c.Fors = append(c.Fors, f)
	}
	c.S = p.span(start)
	return c
}

func numLit(t *token) *ir.Lit {
	v := strings.ReplaceAll(t.text, "_", "")
	lower := strings.ToLower(v)
	kind := ir.LitInt
	if !strings.HasPrefix(lower, "0x") && (strings.ContainsAny(v, ".eE") || strings.HasSuffix(lower, "j")) {
		kind = ir.LitFloat
	}
	return &ir.Lit{S: t.spanOf(), Kind: kind, Raw: t.text, Value: v}
}

// strings menggabungkan string yang berdampingan ("a" "b") termasuk f-string.
func (p *parser) strings() ir.Expr {
	start := p.tok().pos
	tm := &ir.Template{Parts: []string{""}}
	isF := false
	raw := ""
	for p.at(tStr) || p.at(tFStr) {
		t := p.next()
		raw += t.text
		if t.k == tStr {
			tm.Parts[len(tm.Parts)-1] += t.val
			continue
		}
		isF = true
		for i, part := range t.parts {
			tm.Parts[len(tm.Parts)-1] += part
			if i < len(t.exprs) {
				r := t.exprs[i]
				src := ""
				if r[1] <= len(p.src) && r[0] <= r[1] {
					src = p.src[r[0]:r[1]]
				}
				spec := t.specs[i]
				if t.selfs[i] {
					tm.Parts[len(tm.Parts)-1] += strings.TrimSpace(src) + "="
					if spec == "" {
						spec = "!r"
					}
				}
				tm.Exprs = append(tm.Exprs, p.subExpr(src, r[0], t.pos))
				tm.Specs = append(tm.Specs, spec)
				tm.Parts = append(tm.Parts, "")
			}
		}
	}
	tm.S = p.span(start)
	if !isF {
		return &ir.Lit{S: tm.S, Kind: ir.LitString, Raw: raw, Value: tm.Parts[0]}
	}
	return tm
}

// subExpr mem-parse ekspresi di dalam f-string.
func (p *parser) subExpr(src string, off int, near ir.Pos) ir.Expr {
	if strings.TrimSpace(src) == "" {
		p.errAt(near, "ekspresi kosong di f-string")
		return &ir.BadExpr{S: ir.Span{Start: near, End: near}, Msg: "kosong"}
	}
	toks, diags := lex("(" + src + ")")
	// geser posisi supaya menunjuk ke sumber asli ("(" tambahan ada di offset off-1)
	pos := p.posAt(off-1, near)
	line, col := pos.Line, pos.Col
	for i := range toks {
		shift := func(q ir.Pos) ir.Pos {
			q.Off += off - 1
			if q.Line == 1 {
				q.Col += col - 1
			}
			q.Line += line - 1
			return q
		}
		toks[i].pos, toks[i].end = shift(toks[i].pos), shift(toks[i].end)
	}
	sub := &parser{src: p.src, toks: toks}
	x := sub.test()
	p.diags = append(p.diags, diags...)
	p.diags = append(p.diags, sub.diags...)
	return x
}

// posAt menghitung baris/kolom offset off dengan memindai dari posisi from.
func (p *parser) posAt(off int, from ir.Pos) ir.Pos {
	pos := from
	for i := from.Off; i < off && i < len(p.src); i++ {
		if p.src[i] == '\n' {
			pos.Line++
			pos.Col = 1
		} else {
			pos.Col++
		}
	}
	pos.Off = off
	return pos
}
