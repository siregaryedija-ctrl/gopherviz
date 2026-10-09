package clike

import (
	"fmt"
	"strings"

	"gopherviz/internal/ir"
)

const maxDiags = 40

type parser struct {
	d      *Dialect
	src    string // sumber penuh (untuk teks dan sub-parse interpolasi)
	toks   []token
	i      int
	diags  []ir.Diag
	types  map[string]bool // nama tipe yang dikenal (C/C++/Java): bawaan + kelas/struct/typedef di file
	noIn   bool            // JS: `in` bukan operator (bagian init for-in)
	depth  int             // kedalaman rekursi (pengaman untuk kode yang sangat bersarang)
	inCls  string          // nama kelas yang sedang di-parse (konstruktor C++/Java)
	inSwit int             // > 0 di dalam switch (label "case x:" bukan statement berlabel)
}

// Parse mem-parse src dalam bahasa lang (javascript, typescript, php, cpp, c, java).
// Tidak pernah panik: kegagalan internal dilaporkan sebagai Diag.
func Parse(lang, src string) (f *ir.File) {
	d := DialectFor(lang)
	f = &ir.File{Lang: lang}
	if d == nil {
		f.Diags = []ir.Diag{{Pos: ir.Pos{Line: 1, Col: 1}, Msg: "bahasa tidak dikenal: " + lang}}
		return f
	}
	defer func() {
		if r := recover(); r != nil {
			f.Diags = append(f.Diags, ir.Diag{Pos: ir.Pos{Line: 1, Col: 1}, Msg: fmt.Sprintf("kesalahan internal parser: %v", r)})
		}
	}()
	toks, ldiags := lex(d, src, ir.Pos{})
	p := &parser{d: d, src: src, toks: toks, diags: ldiags, types: builtinTypes(lang)}
	p.prescanTypes()
	for !p.at(tEOF) {
		before := p.i
		if s := p.stmt(); s != nil {
			f.Body = append(f.Body, s)
		}
		if p.i == before {
			p.badSkip("tidak bisa dibaca di sini: " + p.tok().text)
		}
	}
	f.Diags = p.diags
	return f
}

// ParseExpr mem-parse satu ekspresi (dipakai untuk interpolasi string).
func parseSub(parent *parser, src string, base ir.Pos) ir.Expr {
	toks, ldiags := lex(parent.d, src, base)
	p := &parser{d: parent.d, src: parent.src, toks: toks, types: parent.types}
	if base.Line == 0 {
		p.src = src
	}
	x := p.expr()
	parent.diags = append(parent.diags, ldiags...)
	parent.diags = append(parent.diags, p.diags...)
	return x
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

// is: token sekarang adalah tanda baca atau kata dengan teks s.
func (p *parser) is(s string) bool {
	t := p.tok()
	return (t.k == tPunct || t.k == tIdent) && t.text == s
}

func (p *parser) isAt(k int, s string) bool {
	t := p.peekTok(k)
	return (t.k == tPunct || t.k == tIdent) && t.text == s
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

func (p *parser) expect(s string, what string) bool {
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

// badSkip mencatat kesalahan lalu melompat ke batas statement berikutnya.
func (p *parser) badSkip(msg string) *ir.Bad {
	start := p.tok().pos
	p.errAt(start, msg)
	line := start.Line
	p.next()
	for !p.at(tEOF) && !p.is(";") && !p.is("}") && !p.is("{") && p.tok().pos.Line == line {
		p.next()
	}
	p.accept(";")
	return &ir.Bad{S: p.span(start), Msg: msg}
}

func (p *parser) text(s ir.Span) string {
	if s.Start.Off < 0 || s.End.Off > len(p.src) || s.Start.Off > s.End.Off {
		return ""
	}
	return p.src[s.Start.Off:s.End.Off]
}

// semi mengakhiri statement: titik koma, atau (JS) titik koma otomatis.
func (p *parser) semi() {
	if p.accept(";") {
		return
	}
	t := p.tok()
	if t.k == tEOF || p.is("}") || t.k == tInline {
		if !p.d.ASI && t.k == tEOF {
			p.errAt(p.prevEnd(), "kurang \";\" di akhir statement")
		}
		return
	}
	if p.d.ASI && t.nl {
		return
	}
	if t.nl {
		p.errAt(p.prevEnd(), "kurang \";\" di akhir baris")
		return
	}
	p.badSkip(fmt.Sprintf("kurang \";\" sebelum %q", t.text))
}

// ---------- statement ----------

func (p *parser) stmt() ir.Stmt {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 200 {
		return p.badSkip("kode terlalu dalam bersarang")
	}
	t := p.tok()
	start := t.pos
	switch t.k {
	case tEOF:
		return nil
	case tInline:
		p.next()
		return &ir.ExprStmt{S: p.span(start), X: &ir.Call{S: p.span(start), Fun: &ir.Ident{S: p.span(start), Name: "echo"},
			Args: []ir.Expr{&ir.Lit{S: p.span(start), Kind: ir.LitString, Raw: t.text, Value: t.val}}}}
	case tDirective:
		p.next()
		return &ir.Simple{S: p.span(start), Kind: "directive", Names: []string{strings.TrimSpace(t.text)}}
	case tPunct:
		switch t.text {
		case "{":
			return p.block()
		case ";":
			p.next()
			return &ir.Simple{S: p.span(start), Kind: "empty"}
		case "}":
			return p.badSkip("kurung kurawal } berlebih")
		case "@":
			if p.d.TSTypes || p.d.Lang == "java" {
				p.annotation()
				return p.stmt()
			}
		case "#":
			if p.d.Lang == "php" {
				p.skipLineTokens()
				return p.stmt()
			}
		}
	case tIdent:
		if s := p.keywordStmt(); s != nil {
			return s
		}
		// label: for (...) — bukan di PHP ($x tidak bisa jadi label) dan bukan "case x:"
		if p.peekTok(1).k == tPunct && p.peekTok(1).text == ":" && !p.d.Dollar && p.inSwit == 0 && !isKeyword(p.d, t.text) {
			p.next()
			p.next()
			body := p.stmt()
			if body == nil { // label di akhir file
				body = &ir.Simple{S: p.span(start), Kind: "empty"}
			}
			return &ir.Labeled{S: p.span(start), Label: t.text, Stmt: body}
		}
		if p.d.Types {
			if s := p.tryDecl(start, false); s != nil {
				return s
			}
		}
	}
	x := p.expr()
	p.semi()
	return &ir.ExprStmt{S: p.span(start), X: x}
}

func (p *parser) skipLineTokens() {
	line := p.tok().pos.Line
	for !p.at(tEOF) && p.tok().pos.Line == line {
		p.next()
	}
}

// annotation melewati @Override / @decorator(...) (Java, TypeScript).
func (p *parser) annotation() {
	p.next()
	p.next() // nama
	for p.accept(".") {
		p.next()
	}
	if p.is("(") && !p.tok().nl {
		p.skipBalanced()
	}
}

// skipBalanced melewati ( ... ), [ ... ], { ... } atau < ... > beserta isinya.
func (p *parser) skipBalanced() {
	open := p.tok().text
	close := map[string]string{"(": ")", "[": "]", "{": "}", "<": ">"}[open]
	if close == "" {
		p.next()
		return
	}
	depth := 0
	for !p.at(tEOF) {
		t := p.next()
		if t.k != tPunct {
			continue
		}
		switch {
		case t.text == open:
			depth++
		case t.text == close:
			depth--
		case close == ">" && t.text == ">>":
			depth -= 2
		case close == ">" && t.text == ">>>":
			depth -= 3
		}
		if depth <= 0 {
			return
		}
	}
}

func (p *parser) block() *ir.Block {
	start := p.tok().pos
	if !p.accept("{") {
		p.errAt(start, "kurang \"{\"")
		b := &ir.Block{Closed: false}
		if s := p.stmt(); s != nil {
			b.List = append(b.List, s)
		}
		b.S = p.span(start)
		return b
	}
	b := &ir.Block{}
	for !p.at(tEOF) && !p.is("}") {
		before := p.i
		if s := p.stmt(); s != nil {
			b.List = append(b.List, s)
		}
		if p.i == before {
			b.List = append(b.List, p.badSkip("tidak bisa dibaca di sini: "+p.tok().text))
		}
	}
	if p.accept("}") {
		b.Closed = true
	} else {
		p.errAt(start, "kurung kurawal { belum ditutup")
	}
	b.S = p.span(start)
	return b
}

// body membaca badan if/for/while: blok { } atau satu statement. PHP juga mendukung
// sintaks alternatif "if (x): ... endif;".
func (p *parser) body(alt string) ir.Stmt {
	if p.d.Dollar && alt != "" && p.is(":") {
		start := p.tok().pos
		p.next()
		b := &ir.Block{}
		for !p.at(tEOF) && !p.is(alt) && !p.is("else") && !p.is("elseif") && !(alt == "endswitch" && (p.is("case") || p.is("default"))) {
			before := p.i
			if s := p.stmt(); s != nil {
				b.List = append(b.List, s)
			}
			if p.i == before {
				p.badSkip("tidak bisa dibaca di sini")
			}
		}
		if p.is(alt) {
			p.next()
			p.accept(";")
			b.Closed = true
		}
		b.S = p.span(start)
		return b
	}
	if p.at(tEOF) {
		p.errAt(p.prevEnd(), "badan belum ditulis")
		return &ir.Block{S: p.span(p.tok().pos)}
	}
	return p.stmt()
}

func (p *parser) parenCond(what string) ir.Expr {
	if !p.expect("(", "setelah "+what) {
		if p.is("{") {
			return &ir.BadExpr{S: p.span(p.tok().pos), Msg: "kondisi kosong"}
		}
		return p.expr()
	}
	if p.is(")") {
		p.errAt(p.tok().pos, "kondisi "+what+" masih kosong")
		p.next()
		return &ir.BadExpr{S: p.span(p.tok().pos), Msg: "kondisi kosong"}
	}
	x := p.exprSeq()
	p.expect(")", "untuk menutup kondisi "+what)
	return x
}

// keywordStmt mem-parse statement yang diawali kata kunci; nil bila bukan.
func (p *parser) keywordStmt() ir.Stmt {
	t := p.tok()
	start := t.pos
	kw := t.text
	switch kw {
	case "if":
		return p.ifStmt()
	case "for":
		return p.forStmt()
	case "foreach":
		if p.d.Dollar {
			return p.foreachStmt()
		}
	case "while":
		p.next()
		cond := p.parenCond("while")
		hdr := p.span(start)
		body := p.body("endwhile")
		return &ir.While{S: p.span(start), Cond: cond, Body: body, Header: hdr}
	case "do":
		if p.isAt(1, "{") || !p.d.ASI || true {
			p.next()
			hdr := p.span(start)
			body := p.stmt()
			var cond ir.Expr
			if p.expect("while", "setelah badan do") {
				cond = p.parenCond("do-while")
			} else {
				cond = &ir.BadExpr{S: p.span(p.tok().pos), Msg: "kondisi kosong"}
			}
			if !p.accept(";") && !p.d.ASI && !p.at(tEOF) {
				p.errAt(p.prevEnd(), "kurang \";\" setelah do-while")
			}
			return &ir.While{S: p.span(start), Cond: cond, Body: body, Do: true, Header: hdr}
		}
	case "switch":
		return p.switchStmt()
	case "break", "continue":
		p.next()
		b := &ir.Branch{Tok: kw}
		if p.at(tIdent) && !p.tok().nl && !p.d.Dollar {
			b.Label = p.next().text
		} else if p.at(tNum) && p.d.Dollar {
			fmt.Sscan(p.next().text, &b.Count)
		}
		p.semi()
		b.S = p.span(start)
		return b
	case "return":
		p.next()
		var x ir.Expr
		if !p.is(";") && !p.is("}") && !p.at(tEOF) && !(p.d.ASI && p.tok().nl) {
			x = p.exprSeq()
		}
		p.semi()
		return &ir.Return{S: p.span(start), X: x}
	case "throw":
		p.next()
		x := p.expr()
		p.semi()
		return &ir.Throw{S: p.span(start), X: x}
	case "try":
		return p.tryStmt()
	case "goto":
		p.next()
		name := p.next().text
		p.semi()
		return &ir.Branch{S: p.span(start), Tok: "goto", Label: name}
	case "function":
		if p.d.ASI || p.d.Dollar {
			if p.peekTok(1).k == tIdent || p.isAt(1, "*") || (p.isAt(1, "&") && p.d.Dollar) {
				return p.funcDecl(start, "")
			}
		}
	case "async":
		if p.d.ASI && p.isAt(1, "function") && !p.peekTok(1).nl {
			p.next()
			f := p.funcDecl(start, "")
			f.Async = true
			return f
		}
	case "class", "interface", "trait", "enum", "struct", "union":
		if p.d.Lang == "c" && kw != "struct" && kw != "union" && kw != "enum" {
			break
		}
		if p.d.Types && (kw == "struct" || kw == "union" || kw == "enum") {
			// struct X { ... }; atau struct X var; — biarkan tryDecl yang memutuskan bila bukan definisi
			if !(p.peekTok(1).k == tIdent && p.isAt(2, "{")) && !p.isAt(1, "{") && !(kw == "enum" && p.isAt(1, "class")) {
				break
			}
		}
		return p.classDecl(start, nil)
	case "abstract", "final", "public", "private", "protected", "static", "export", "default", "declare", "readonly", "sealed":
		return p.modifiedStmt(start)
	case "let", "const", "var":
		if p.d.ASI || (kw == "var" && p.d.Lang == "java") {
			if kw == "const" && p.isAt(1, "enum") {
				p.next()
				return p.classDecl(start, nil)
			}
			if kw == "var" && p.d.Lang == "java" {
				return p.tryDecl(start, false)
			}
			if kw == "let" && !(p.peekTok(1).k == tIdent || p.isAt(1, "[") || p.isAt(1, "{")) {
				break // "let" sebagai nama variabel biasa
			}
			d := p.varDecl()
			p.semi()
			return d
		}
		if kw == "const" && p.d.Dollar {
			// PHP: const A = 1;
			p.next()
			d := &ir.VarDecl{Kind: "const"}
			for {
				ss := p.tok().pos
				name := p.next().text
				var init ir.Expr
				if p.accept("=") {
					init = p.expr()
				}
				d.Specs = append(d.Specs, &ir.VarSpec{S: p.span(ss), Name: name, Init: init})
				if !p.accept(",") {
					break
				}
			}
			p.semi()
			d.S = p.span(start)
			return d
		}
	case "import":
		if p.d.ASI && !p.isAt(1, "(") && !p.isAt(1, ".") || p.d.Lang == "java" {
			return p.skipStmt("import")
		}
	case "package":
		if p.d.Lang == "java" {
			return p.skipStmt("package")
		}
	case "namespace", "use", "require", "require_once", "include", "include_once":
		if p.d.Dollar {
			if kw == "namespace" && p.peekTok(1).k == tIdent && p.isAt(2, "{") {
				break
			}
			return p.skipStmt("import")
		}
		if kw == "namespace" && p.d.Lang == "cpp" {
			p.next()
			for p.at(tIdent) || p.is("::") {
				p.next()
			}
			if p.is("{") {
				b := p.block()
				return b
			}
			p.semi()
			return &ir.Simple{S: p.span(start), Kind: "import"}
		}
	case "using", "typedef", "template", "friend":
		if p.d.Lang == "cpp" || p.d.Lang == "c" {
			if kw == "template" {
				p.next()
				if p.is("<") {
					p.skipBalanced()
				}
				return p.stmt()
			}
			if kw == "typedef" {
				return p.typedef(start)
			}
			if kw == "friend" {
				p.next()
				return p.stmt()
			}
			return p.skipStmt("import")
		}
	case "type":
		if p.d.TSTypes && p.peekTok(1).k == tIdent && !p.peekTok(1).nl {
			return p.skipStmt("type")
		}
	case "echo", "print":
		if p.d.Dollar {
			p.next()
			var args []ir.Expr
			for {
				args = append(args, p.expr())
				if !p.accept(",") {
					break
				}
			}
			p.semi()
			return &ir.ExprStmt{S: p.span(start), X: &ir.Call{S: p.span(start), Fun: &ir.Ident{S: t.spanOf(), Name: "echo"}, Args: args}}
		}
	case "global":
		if p.d.Dollar {
			p.next()
			s := &ir.Simple{Kind: "global"}
			for p.at(tIdent) {
				s.Names = append(s.Names, p.next().text)
				if !p.accept(",") {
					break
				}
			}
			p.semi()
			s.S = p.span(start)
			return s
		}
	case "unset":
		if p.d.Dollar {
			p.next()
			x := p.expr()
			p.semi()
			return &ir.Simple{S: p.span(start), Kind: "del", X: callArgs(x)}
		}
	case "delete":
		if p.d.Lang == "cpp" {
			p.next()
			if p.is("[") {
				p.skipBalanced()
			}
			x := p.expr()
			p.semi()
			return &ir.Simple{S: p.span(start), Kind: "del", X: []ir.Expr{x}}
		}
	case "synchronized":
		if p.d.Lang == "java" && p.isAt(1, "(") {
			p.next()
			p.parenCond("synchronized")
			return p.block()
		}
	case "assert":
		if p.d.Lang == "java" {
			p.next()
			x := p.expr()
			if p.accept(":") {
				p.expr()
			}
			p.semi()
			return &ir.Simple{S: p.span(start), Kind: "assert", X: []ir.Expr{x}}
		}
	case "yield":
		if p.d.Lang == "java" && p.inSwit > 0 {
			p.next()
			x := p.expr()
			p.semi()
			return &ir.Return{S: p.span(start), X: x}
		}
	}
	return nil
}

func callArgs(x ir.Expr) []ir.Expr {
	if s, ok := x.(*ir.Seq); ok {
		return s.List
	}
	return []ir.Expr{x}
}

func (t *token) spanOf() ir.Span { return ir.Span{Start: t.pos, End: t.end} }

// skipStmt melewati statement yang tidak memengaruhi tampilan (import, type alias, ...).
func (p *parser) skipStmt(kind string) ir.Stmt {
	start := p.tok().pos
	line := start.Line
	p.next()
	for !p.at(tEOF) && !p.is(";") {
		if p.is("{") || p.is("(") || p.is("[") {
			p.skipBalanced()
			continue
		}
		if p.d.ASI && p.tok().nl && p.tok().pos.Line != line && !p.is("from") && !p.is("|") && !p.is("&") {
			break
		}
		p.next()
	}
	p.accept(";")
	return &ir.Simple{S: p.span(start), Kind: kind}
}

func (p *parser) ifStmt() ir.Stmt {
	start := p.tok().pos
	p.next()
	cond := p.parenCond("if")
	then := p.body("endif")
	n := &ir.If{Cond: cond, Then: then}
	switch {
	case p.is("else") && p.isAt(1, "if"):
		p.next()
		n.Else = p.ifStmt()
	case p.is("elseif") && p.d.Dollar:
		n.Else = p.ifStmt()
	case p.is("else"):
		p.next()
		n.Else = p.body("endif")
	}
	n.S = p.span(start)
	return n
}

func (p *parser) forStmt() ir.Stmt {
	start := p.tok().pos
	p.next()
	if p.d.ASI && p.is("await") {
		p.next()
	}
	if !p.expect("(", "setelah for") {
		hdr := p.span(start)
		return &ir.For{S: p.span(start), Body: p.body("endfor"), Header: hdr}
	}
	// for-each: for (const x of xs), for (k in obj), for (int x : xs)
	if fe := p.tryForEach(start); fe != nil {
		return fe
	}
	n := &ir.For{}
	if !p.is(";") {
		is := p.tok().pos
		switch {
		case p.d.ASI && (p.is("let") || p.is("const") || p.is("var")):
			p.noIn = true
			n.Init = p.varDecl()
			p.noIn = false
		case p.d.Types:
			if d := p.tryDecl(is, true); d != nil {
				n.Init = d
			} else {
				n.Init = &ir.ExprStmt{X: p.exprSeq()}
			}
		default:
			x := p.exprSeq()
			n.Init = &ir.ExprStmt{S: p.span(is), X: x}
		}
		if es, ok := n.Init.(*ir.ExprStmt); ok {
			es.S = p.span(is)
		}
	}
	p.expect(";", "setelah bagian awal for")
	if !p.is(";") {
		n.Cond = p.exprSeq()
	}
	p.expect(";", "setelah kondisi for")
	if !p.is(")") {
		n.Post = p.exprSeq()
	}
	p.expect(")", "untuk menutup kepala for")
	n.Header = p.span(start)
	n.Body = p.body("endfor")
	n.S = p.span(start)
	return n
}

// tryForEach mencoba membaca kepala for-each setelah "(". Mengembalikan nil (posisi
// dipulihkan) bila ini for klasik.
func (p *parser) tryForEach(start ir.Pos) ir.Stmt {
	save, saveDiags := p.i, len(p.diags)
	fe := &ir.ForEach{}
	if p.d.ASI {
		if p.is("let") || p.is("const") || p.is("var") {
			fe.Decl = p.next().text
		}
		p.noIn = true
		target := p.unary()
		p.noIn = false
		switch {
		case p.is("of"):
			fe.Kind = "of"
		case p.is("in"):
			fe.Kind = "in"
		default:
			p.i, p.diags = save, p.diags[:saveDiags]
			return nil
		}
		p.next()
		fe.Value = target
		fe.X = p.expr()
	} else if p.d.Types {
		// for (Tipe nama : xs)
		ts := p.i
		typ, ok := p.typeText(false)
		for ok && p.is("[") && p.isAt(1, "]") { // Java: for (int[] baris : grid)
			p.next()
			p.next()
			typ += "[]"
		}
		if !ok || !(p.at(tIdent) || p.is("[") || p.is("&") || p.is("*")) {
			p.i, p.diags = save, p.diags[:saveDiags]
			return nil
		}
		_ = ts
		ref := false
		for p.is("&") || p.is("&&") || p.is("*") {
			ref = true
			p.next()
		}
		var target ir.Expr
		if p.is("[") { // structured binding C++17: for (auto [k, v] : m)
			target = p.arrayLit("[", "]")
		} else {
			nt := p.next()
			target = &ir.Ident{S: nt.spanOf(), Name: nt.text}
		}
		if !p.is(":") {
			p.i, p.diags = save, p.diags[:saveDiags]
			return nil
		}
		p.next()
		_ = ref
		fe.Kind, fe.Decl, fe.Value = "colon", typ, target
		fe.X = p.expr()
	} else {
		return nil
	}
	p.expect(")", "untuk menutup kepala for")
	fe.Header = p.span(start)
	fe.Body = p.body("endfor")
	fe.S = p.span(start)
	return fe
}

// foreachStmt: PHP foreach ($xs as $v) / foreach ($xs as $k => $v).
func (p *parser) foreachStmt() ir.Stmt {
	start := p.tok().pos
	p.next()
	fe := &ir.ForEach{Kind: "foreach"}
	if p.expect("(", "setelah foreach") {
		fe.X = p.expr()
		if p.expect("as", "di foreach") {
			ref := p.accept("&")
			first := p.expr()
			if p.accept("=>") {
				ref = p.accept("&")
				fe.Key, fe.Value = first, p.expr()
			} else {
				fe.Value = first
			}
			if ref {
				fe.Decl = "&" // foreach dengan referensi: mengubah elemen array asli
			}
		}
		p.expect(")", "untuk menutup kepala foreach")
	}
	if fe.X == nil {
		fe.X = &ir.BadExpr{S: p.span(start), Msg: "foreach tanpa koleksi"}
	}
	fe.Header = p.span(start)
	fe.Body = p.body("endforeach")
	fe.S = p.span(start)
	return fe
}

func (p *parser) switchStmt() ir.Stmt {
	start := p.tok().pos
	p.next()
	tag := p.parenCond("switch")
	hdr := p.span(start)
	n := &ir.Switch{Tag: tag, Header: hdr}
	alt := p.d.Dollar && p.is(":")
	if !alt && !p.expect("{", "setelah switch") {
		n.S = p.span(start)
		return n
	}
	if alt {
		p.next()
	}
	p.inSwit++
	defer func() { p.inSwit-- }()
	var cur *ir.Case
	for !p.at(tEOF) && !p.is("}") && !(alt && p.is("endswitch")) {
		cs := p.tok().pos
		switch {
		case p.is("case"):
			p.next()
			cur = &ir.Case{}
			for {
				cur.Exprs = append(cur.Exprs, p.caseExpr())
				if !p.accept(",") {
					break
				}
			}
			p.caseColon(cur)
			n.Cases = append(n.Cases, cur)
		case p.is("default"):
			p.next()
			cur = &ir.Case{}
			p.caseColon(cur)
			n.Cases = append(n.Cases, cur)
		default:
			before := p.i
			s := p.stmt()
			if p.i == before {
				s = p.badSkip("tidak bisa dibaca di sini")
			}
			if cur == nil {
				p.errAt(cs, "statement di switch harus berada di bawah case")
				cur = &ir.Case{S: p.span(cs)}
				n.Cases = append(n.Cases, cur)
			}
			cur.Body = append(cur.Body, s)
			cur.S.End = p.prevEnd()
			continue
		}
		cur.S = p.span(cs)
		if cur.Arrow {
			// Java "case 1 -> stmt;" : satu statement atau blok, tanpa fallthrough
			s := p.stmt()
			cur.Body = append(cur.Body, s)
			cur.S = p.span(cs)
			cur = nil
		}
	}
	closed := p.accept("}") || (alt && p.accept("endswitch"))
	if alt {
		p.accept(";")
	}
	if !closed {
		p.errAt(start, "switch belum ditutup dengan }")
	}
	n.S = p.span(start)
	return n
}

// caseExpr membaca ekspresi case tanpa menelan ":" sebagai bagian ternary.
func (p *parser) caseExpr() ir.Expr {
	return p.binary(0)
}

func (p *parser) caseColon(c *ir.Case) {
	switch {
	case p.accept(":"), p.d.Dollar && p.accept(";"):
	case p.d.Lang == "java" && p.accept("->"):
		c.Arrow = true
	default:
		p.errAt(p.tok().pos, "kurang \":\" setelah case")
	}
}

func (p *parser) tryStmt() ir.Stmt {
	start := p.tok().pos
	p.next()
	n := &ir.Try{}
	if p.d.Lang == "java" && p.is("(") { // try-with-resources
		p.skipBalanced()
	}
	n.Body = p.block()
	for p.is("catch") {
		cs := p.tok().pos
		p.next()
		c := &ir.Catch{}
		if p.accept("(") {
			if p.is("...") { // C++ catch (...)
				p.next()
			} else {
				// tipe-tipe (Java: A | B e; PHP: Exception $e; JS: e; C++: const std::exception& e)
				var names []string
				for !p.is(")") && !p.at(tEOF) {
					t := p.next()
					if t.k == tIdent {
						names = append(names, t.text)
					}
				}
				if len(names) > 0 {
					if p.d.ASI || (len(names) == 1 && (p.d.Dollar || p.d.Types) && strings.HasPrefix(names[0], "$")) {
						c.Name = names[len(names)-1]
						if !p.d.ASI {
							c.Types = names[:len(names)-1]
						}
					} else if len(names) == 1 && p.d.Dollar {
						c.Types = names // PHP 8: catch (Exception) tanpa variabel
					} else {
						c.Name = names[len(names)-1]
						c.Types = names[:len(names)-1]
					}
				}
			}
			p.expect(")", "untuk menutup catch")
		}
		c.Body = p.block()
		c.S = p.span(cs)
		n.Catches = append(n.Catches, c)
	}
	if p.accept("finally") {
		n.Finally = p.block()
	}
	if len(n.Catches) == 0 && n.Finally == nil {
		p.errAt(start, "try harus diikuti catch atau finally")
	}
	n.S = p.span(start)
	return n
}

// varDecl: let/const/var a = 1, [b, c] = xs (JavaScript/TypeScript).
func (p *parser) varDecl() *ir.VarDecl {
	start := p.tok().pos
	d := &ir.VarDecl{Kind: p.next().text}
	for {
		ss := p.tok().pos
		spec := &ir.VarSpec{}
		if p.is("[") || p.is("{") {
			spec.Pattern = p.primary()
		} else if p.at(tIdent) {
			spec.Name = p.next().text
		} else {
			p.errAt(p.tok().pos, "kurang nama variabel")
			if !p.is(";") && !p.is("=") {
				p.next()
			}
		}
		if p.d.TSTypes {
			p.accept("!")
			if p.accept(":") {
				spec.Type = p.tsType(false)
			}
		}
		if p.accept("=") {
			spec.Init = p.expr()
		}
		spec.S = p.span(ss)
		d.Specs = append(d.Specs, spec)
		if !p.accept(",") {
			break
		}
	}
	d.S = p.span(start)
	return d
}

// funcDecl: function nama(params) { } (JS/PHP).
func (p *parser) funcDecl(start ir.Pos, class string) *ir.Func {
	p.next() // function
	f := &ir.Func{Class: class}
	if p.accept("*") {
		f.Generator = true
	}
	p.accept("&") // PHP: function &f()
	if p.at(tIdent) && !p.is("(") {
		f.Name = p.next().text
	}
	if p.d.TSTypes && p.is("<") {
		p.skipBalanced()
	}
	f.Params = p.params()
	if p.d.Dollar && p.accept("use") {
		if p.accept("(") {
			for !p.is(")") && !p.at(tEOF) {
				p.accept("&")
				f.Uses = append(f.Uses, p.next().text)
				p.accept(",")
			}
			p.expect(")", "untuk menutup use")
		}
	}
	if p.accept(":") {
		if p.d.TSTypes {
			f.Ret = p.tsType(false)
		} else {
			f.Ret, _ = p.typeText(false)
		}
	}
	f.Header = p.span(start)
	if p.is("{") {
		f.Body = p.block()
	} else {
		p.semi() // deklarasi tanpa badan (TS overload / method abstrak)
	}
	f.S = p.span(start)
	return f
}

// params membaca daftar parameter "( ... )" untuk semua dialek.
func (p *parser) params() []*ir.Param {
	var out []*ir.Param
	if !p.expect("(", "untuk parameter") {
		return nil
	}
	for !p.is(")") && !p.at(tEOF) {
		before := p.i
		out = append(out, p.param())
		if !p.accept(",") {
			break
		}
		if p.i == before {
			p.next()
		}
	}
	p.expect(")", "untuk menutup parameter")
	return out
}

func (p *parser) param() *ir.Param {
	pr := &ir.Param{}
	for p.is("public") || p.is("private") || p.is("protected") || p.is("readonly") || p.is("final") || p.is("override") {
		if t := p.next().text; t != "final" && t != "override" {
			pr.Promote = true // TS/PHP: parameter konstruktor sekaligus properti
		}
	}
	for p.is("@") {
		p.annotation()
	}
	switch {
	case p.d.ASI:
		if p.accept("...") {
			pr.Rest = true
		}
		if p.is("[") || p.is("{") {
			pr.Pattern = p.primary()
		} else {
			pr.Name = p.next().text
		}
		if p.d.TSTypes {
			p.accept("?")
			if p.accept(":") {
				pr.Type = p.tsType(false)
			}
		}
	case p.d.Dollar:
		if !strings.HasPrefix(p.tok().text, "$") && !p.is("&") && !p.is("...") {
			pr.Type, _ = p.typeText(false)
		}
		if p.accept("&") {
			pr.Ref = true
		}
		if p.accept("...") {
			pr.Rest = true
		}
		pr.Name = p.next().text
	default:
		if p.is("...") { // C varargs
			p.next()
			pr.Rest, pr.Name = true, "..."
			return pr
		}
		pr.Type, _ = p.typeText(p.d.Lang == "java")
		for p.is("&") || p.is("&&") || p.is("*") {
			if p.next().text != "*" {
				pr.Ref = true
			} else {
				pr.Type += "*"
			}
		}
		if p.accept("...") { // Java varargs
			pr.Rest = true
		}
		if p.at(tIdent) {
			pr.Name = p.next().text
		}
		for p.is("[") { // int arr[] / int m[][3]
			p.skipBalanced()
			pr.Type += "[]"
		}
	}
	if p.accept("=") {
		pr.Default = p.expr()
	}
	return pr
}

// modifiedStmt menangani deklarasi berawalan modifier: export, public, static, abstract, ...
func (p *parser) modifiedStmt(start ir.Pos) ir.Stmt {
	kw := p.tok().text
	switch {
	case kw == "export" && p.d.ASI:
		p.next()
		if p.accept("default") {
			if p.is("function") || p.is("class") || p.is("async") {
				return p.stmt()
			}
			x := p.expr()
			p.semi()
			return &ir.ExprStmt{S: p.span(start), X: x}
		}
		if p.is("{") || p.is("*") {
			return p.skipStmt("import")
		}
		return p.stmt()
	case kw == "default":
		return nil
	case kw == "declare" && p.d.TSTypes:
		return p.skipStmt("type")
	case kw == "declare" && p.d.Dollar:
		return p.skipStmt("directive") // declare(strict_types=1);
	case kw == "static" && p.d.Dollar && strings.HasPrefix(p.peekTok(1).text, "$"):
		// PHP: static $count = 0; di dalam fungsi
		p.next()
		d := &ir.VarDecl{Kind: "static"}
		for {
			ss := p.tok().pos
			name := p.next().text
			var init ir.Expr
			if p.accept("=") {
				init = p.expr()
			}
			d.Specs = append(d.Specs, &ir.VarSpec{S: p.span(ss), Name: name, Init: init})
			if !p.accept(",") {
				break
			}
		}
		p.semi()
		d.S = p.span(start)
		return d
	case kw == "static" && p.d.Dollar && (p.isAt(1, "::") || p.isAt(1, "fn") || p.isAt(1, "function")):
		return nil // ekspresi static::x / static fn
	}
	// abstract class / final class / public class (Java) / static int x (C/C++)
	mods := []string{}
	for isModifier(p.tok().text) && p.peekTok(1).k == tIdent || p.is("export") {
		mods = append(mods, p.next().text)
	}
	if p.is("class") || p.is("interface") || p.is("enum") || p.is("trait") || p.is("record") {
		return p.classDecl(start, mods)
	}
	if p.is("function") {
		f := p.funcDecl(start, "")
		return f
	}
	if p.d.Types {
		if s := p.tryDecl(start, false); s != nil {
			if d, ok := s.(*ir.VarDecl); ok && contains(mods, "static") {
				d.Kind = "static " + d.Kind
			}
			if f, ok := s.(*ir.Func); ok && contains(mods, "static") {
				f.Static = true
			}
			return s
		}
	}
	return nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func isModifier(s string) bool {
	switch s {
	case "public", "private", "protected", "static", "final", "abstract", "export", "readonly", "sealed",
		"virtual", "inline", "extern", "constexpr", "volatile", "register", "synchronized", "native", "transient",
		"strictfp", "default", "override", "async", "declare", "mutable", "explicit", "thread_local", "non-sealed":
		return true
	}
	return false
}

// isKeyword: kata yang tidak bisa menjadi label/nama variabel.
func isKeyword(d *Dialect, s string) bool {
	switch s {
	case "if", "else", "for", "while", "do", "switch", "case", "default", "break", "continue", "return",
		"try", "catch", "finally", "throw", "new", "class", "struct", "function", "public", "private",
		"protected", "static", "const", "goto":
		return true
	}
	return false
}
