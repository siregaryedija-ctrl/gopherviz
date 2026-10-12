package clike

import (
	"strings"

	"gopherviz/internal/ir"
)

// exprSeq membaca ekspresi dengan operator koma (bagian for, kurung JS/C).
func (p *parser) exprSeq() ir.Expr {
	start := p.tok().pos
	x := p.expr()
	if !p.is(",") || p.d.Dollar || p.d.Lang == "java" {
		return x
	}
	seq := &ir.Seq{List: []ir.Expr{x}}
	for p.accept(",") {
		seq.List = append(seq.List, p.expr())
	}
	seq.S = p.span(start)
	return seq
}

func (p *parser) expr() ir.Expr {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 300 {
		start := p.tok().pos
		p.next()
		return &ir.BadExpr{S: p.span(start), Msg: "expression nested too deep"}
	}
	return p.assign()
}

func isAssignOp(s string) bool {
	switch s {
	case "=", "+=", "-=", "*=", "/=", "%=", "**=", "<<=", ">>=", ">>>=", "&=", "|=", "^=", "&&=", "||=", "??=", ".=":
		return true
	}
	return false
}

func (p *parser) assign() ir.Expr {
	start := p.tok().pos
	if f := p.tryArrow(); f != nil {
		return f
	}
	if p.d.ASI && p.is("yield") {
		p.next()
		op := "yield"
		if p.accept("*") {
			op = "yield*"
		}
		var x ir.Expr
		if !p.is(")") && !p.is("]") && !p.is("}") && !p.is(",") && !p.is(";") && !p.tok().nl && !p.at(tEOF) {
			x = p.assign()
		}
		return &ir.Unary{S: p.span(start), Op: op, X: x}
	}
	x := p.ternary()
	if t := p.tok(); t.k == tPunct && isAssignOp(t.text) {
		op := p.next().text
		if p.d.Dollar && op == "=" && p.is("&") { // PHP: $a = &$b
			p.next()
		}
		r := p.assign()
		return &ir.Assign{S: p.span(start), Op: op, L: x, R: r}
	}
	return x
}

func (p *parser) ternary() ir.Expr {
	start := p.tok().pos
	c := p.binary(0)
	if !p.is("?") {
		return c
	}
	p.next()
	if p.d.Dollar && p.accept(":") { // PHP: a ?: b
		f := p.assign()
		return &ir.Cond{S: p.span(start), C: c, T: c, F: f}
	}
	t := p.assign()
	if !p.expect(":", "in the ? : operator") {
		return &ir.Cond{S: p.span(start), C: c, T: t, F: &ir.BadExpr{S: p.span(start), Msg: "missing :"}}
	}
	f := p.assign()
	return &ir.Cond{S: p.span(start), C: c, T: t, F: f}
}

// binPrec mengembalikan prioritas operator biner (0 = bukan operator biner).
func (p *parser) binPrec() (string, int, bool) {
	t := p.tok()
	if t.k != tPunct && t.k != tIdent {
		return "", 0, false
	}
	op := t.text
	switch op {
	case "??":
		return op, 10, true
	case "||":
		return op, 20, false
	case "&&":
		return op, 30, false
	case "|":
		return op, 40, false
	case "^":
		return op, 50, false
	case "&":
		return op, 60, false
	case "==", "!=", "===", "!==", "<=>", "<>":
		return op, 70, false
	case "<", ">", "<=", ">=":
		return op, 80, false
	case "<<", ">>", ">>>":
		return op, 90, false
	case "+", "-":
		return op, 100, false
	case "*", "/", "%":
		return op, 110, false
	case "**":
		return op, 120, true
	case ".":
		if p.d.Dollar {
			return op, 95, false // PHP: penggabungan string
		}
	}
	if t.k != tIdent {
		return "", 0, false
	}
	switch op {
	case "instanceof":
		if p.d.Dollar || p.d.ASI || p.d.Lang == "java" {
			return op, 80, false
		}
	case "in":
		if p.d.ASI && !p.noIn {
			return op, 80, false
		}
	case "as", "satisfies":
		if p.d.TSTypes && !t.nl {
			return op, 80, false
		}
	case "and":
		if p.d.Dollar {
			return "&&", 5, false
		}
	case "or":
		if p.d.Dollar {
			return "||", 4, false
		}
	case "xor":
		if p.d.Dollar {
			return "xor", 4, false
		}
	}
	return "", 0, false
}

func (p *parser) binary(min int) ir.Expr {
	start := p.tok().pos
	x := p.unary()
	for {
		op, prec, right := p.binPrec()
		if prec == 0 || prec <= min {
			return x
		}
		p.next()
		var y ir.Expr
		switch op {
		case "instanceof", "as", "satisfies":
			ts := p.tok().pos
			var name string
			if p.d.TSTypes {
				name = p.tsType(false)
			} else {
				name, _ = p.typeText(true)
			}
			if name == "" && p.at(tIdent) { // PHP: $x instanceof $cls
				name = p.next().text
			}
			y = &ir.TypeRef{S: p.span(ts), Name: name}
			if op == "instanceof" && p.d.Lang == "java" && p.at(tIdent) && !isKeyword(p.d, p.tok().text) {
				p.next() // pattern matching: x instanceof String s
			}
		default:
			next := prec
			if right {
				next = prec - 1
			}
			y = p.binary(next)
		}
		x = &ir.Binary{S: p.span(start), Op: op, X: x, Y: y}
	}
}

func (p *parser) unary() ir.Expr {
	start := p.tok().pos
	t := p.tok()
	if t.k == tPunct {
		switch t.text {
		case "!", "-", "+", "~", "++", "--":
			p.next()
			x := p.unary()
			return &ir.Unary{S: p.span(start), Op: t.text, X: x}
		case "*", "&":
			if p.d.Lang == "cpp" || p.d.Lang == "c" {
				p.next()
				x := p.unary()
				return &ir.Unary{S: p.span(start), Op: t.text, X: x}
			}
			if t.text == "&" && p.d.Dollar { // PHP: &$x di array/argumen
				p.next()
				return p.unary()
			}
		case "@":
			if p.d.Dollar {
				p.next()
				return p.unary()
			}
		case "(":
			if c := p.tryCast(); c != nil {
				return c
			}
		case "<":
			if p.d.TSTypes { // <T>x (cast lama TypeScript)
				save := p.i
				p.skipBalanced()
				if p.is("(") || p.at(tIdent) {
					return p.unary()
				}
				p.i = save
			}
		}
	}
	if t.k == tIdent {
		switch t.text {
		case "typeof", "void", "delete", "await":
			if p.d.ASI {
				p.next()
				x := p.unary()
				return &ir.Unary{S: p.span(start), Op: t.text, X: x}
			}
		case "sizeof":
			if p.d.Lang == "c" || p.d.Lang == "cpp" {
				p.next()
				if p.is("(") {
					save := p.i
					p.next()
					kw := "" // sizeof(struct Node) / sizeof(union U) / sizeof(enum E)
					if p.is("struct") || p.is("union") || p.is("enum") {
						kw = p.next().text + " "
					}
					if typ, ok := p.typeText(true); ok {
						for p.is("*") { // sizeof(int*)
							typ += "*"
							p.next()
						}
						if p.is(")") {
							p.next()
							return &ir.Unary{S: p.span(start), Op: "sizeof", X: &ir.TypeRef{S: p.span(start), Name: kw + typ}}
						}
					}
					p.i = save
				}
				x := p.unary()
				return &ir.Unary{S: p.span(start), Op: "sizeof", X: x}
			}
		case "clone", "print":
			if p.d.Dollar {
				p.next()
				x := p.unary()
				if t.text == "print" {
					return &ir.Call{S: p.span(start), Fun: &ir.Ident{S: t.spanOf(), Name: "print"}, Args: []ir.Expr{x}}
				}
				return &ir.Unary{S: p.span(start), Op: "clone", X: x}
			}
		case "throw":
			if p.d.Dollar { // PHP 8: throw sebagai ekspresi
				p.next()
				x := p.assign()
				return &ir.Unary{S: p.span(start), Op: "throw", X: x}
			}
		case "new":
			if p.d.Lang == "cpp" || p.d.Lang == "java" || p.d.ASI || p.d.Dollar {
				return p.postfix(p.newExpr(), start)
			}
		}
	}
	return p.postfix(p.primary(), start)
}

// tryCast mengenali (int) x, (double) sum / n, (String) obj, (Point*) p, (int)$x.
func (p *parser) tryCast() ir.Expr {
	if !(p.d.Types || p.d.Dollar) {
		return nil
	}
	start := p.tok().pos
	save, saveDiags := p.i, len(p.diags)
	p.next()
	name, ok := p.typeText(true)
	for ok && (p.is("*") || p.is("&")) {
		name += p.next().text
	}
	if !ok || !p.is(")") || !p.castable(name) {
		p.i, p.diags = save, p.diags[:saveDiags]
		return nil
	}
	p.next()
	nt := p.tok()
	operand := nt.k == tIdent || nt.k == tNum || nt.k == tStr || nt.k == tChar || nt.k == tTemplate ||
		nt.k == tPunct && (nt.text == "(" || nt.text == "!" || nt.text == "~" || nt.text == "&" && p.d.Types && !p.d.Dollar || nt.text == "*" && p.d.Types ||
			(nt.text == "-" || nt.text == "+") && primitive(name) || nt.text == "[" && p.d.Dollar)
	if !operand || nt.k == tIdent && isBinaryWord(nt.text) {
		p.i, p.diags = save, p.diags[:saveDiags]
		return nil
	}
	x := p.unary()
	return &ir.Cast{S: p.span(start), Type: name, X: x}
}

func isBinaryWord(s string) bool {
	return s == "instanceof" || s == "and" || s == "or" || s == "xor"
}

func (p *parser) castable(name string) bool {
	if p.d.Dollar {
		switch strings.ToLower(name) {
		case "int", "integer", "float", "double", "string", "bool", "boolean", "array", "object":
			return true
		}
		return false
	}
	base := strings.TrimRight(name, "*&[] ")
	if i := strings.LastIndexAny(base, " "); i >= 0 {
		base = base[i+1:]
	}
	if j := strings.IndexByte(base, '<'); j >= 0 {
		base = base[:j]
	}
	return primitive(name) || p.types[base] || strings.Contains(name, "::")
}

func primitive(name string) bool {
	for _, w := range strings.Fields(strings.TrimRight(name, "*&[] ")) {
		switch w {
		case "int", "long", "short", "char", "float", "double", "unsigned", "signed", "byte", "boolean", "bool", "size_t", "const":
		default:
			return false
		}
	}
	return name != ""
}

func (p *parser) postfix(x ir.Expr, start ir.Pos) ir.Expr {
	for {
		t := p.tok()
		switch {
		case t.k == tPunct && t.text == "(":
			args := p.args("(", ")")
			x = &ir.Call{S: p.span(start), Fun: x, Args: args}
		case t.k == tPunct && t.text == "." && !p.d.Dollar:
			p.next()
			if p.is("<") && p.d.Lang == "java" { // Collections.<T>emptyList()
				p.skipBalanced()
			}
			nt := p.next()
			if nt.k != tIdent {
				p.errAt(nt.pos, "missing name after \".\"")
			}
			x = &ir.Member{S: p.span(start), X: x, Name: nt.text, Op: "."}
		case t.k == tPunct && (t.text == "?." || t.text == "?->"):
			p.next()
			switch {
			case p.is("("):
				x = &ir.Call{S: p.span(start), Fun: x, Args: p.args("(", ")"), Opt: true}
			case p.is("["):
				p.next()
				i := p.expr()
				p.expect("]", "to close the index")
				x = &ir.Index{S: p.span(start), X: x, I: i, Opt: true}
			default:
				x = &ir.Member{S: p.span(start), X: x, Name: p.next().text, Op: t.text}
			}
		case t.k == tPunct && t.text == "->" && (p.d.Dollar || p.d.Lang == "cpp" || p.d.Lang == "c"):
			p.next()
			nt := p.next()
			name := nt.text
			if nt.k == tPunct && nt.text == "{" { // PHP $obj->{$name}
				p.i--
				p.skipBalanced()
				name = "?"
			}
			x = &ir.Member{S: p.span(start), X: x, Name: name, Op: "->"}
		case t.k == tPunct && t.text == "::" && !(p.d.ASI):
			p.next()
			nt := p.next()
			x = &ir.Member{S: p.span(start), X: x, Name: nt.text, Op: "::"}
		case t.k == tPunct && t.text == "[":
			if p.d.ASI && t.nl && false {
				return x
			}
			p.next()
			if p.d.Dollar && p.is("]") { // PHP $a[] = x
				p.next()
				x = &ir.Index{S: p.span(start), X: x}
				continue
			}
			i := p.exprSeq()
			p.expect("]", "to close the index")
			x = &ir.Index{S: p.span(start), X: x, I: i}
		case t.k == tPunct && (t.text == "++" || t.text == "--"):
			if p.d.ASI && t.nl {
				return x
			}
			p.next()
			x = &ir.Unary{S: p.span(start), Op: t.text, X: x, Postfix: true}
		case t.k == tPunct && t.text == "!" && p.d.TSTypes && !t.nl:
			// non-null assertion x! — hanya bila setelahnya bukan awal operand
			nt := p.peekTok(1)
			if nt.k == tIdent || nt.k == tNum || nt.k == tStr || nt.k == tPunct && (nt.text == "(" || nt.text == "=") {
				return x
			}
			p.next()
		case t.k == tTemplate && p.d.ASI && !t.nl:
			tm := p.template(p.next())
			tm.Tag = x
			tm.S = p.span(start)
			x = tm
		case t.k == tPunct && t.text == "<" && (p.d.Types || p.d.TSTypes) && p.genericCallAhead():
			p.skipBalanced()
		default:
			return x
		}
	}
}

// genericCallAhead: f<int>(x) / List.<T>of() — '<' membuka argumen tipe yang diikuti "(".
func (p *parser) genericCallAhead() bool {
	j, ok := p.typeArgsEnd(p.i)
	return ok && j < len(p.toks) && p.toks[j].k == tPunct && (p.toks[j].text == "(" || p.toks[j].text == "::")
}

// typeArgsEnd memindai argumen tipe mulai dari '<' di indeks i; mengembalikan indeks
// token setelah '>' penutup.
func (p *parser) typeArgsEnd(i int) (int, bool) {
	depth := 0
	for ; i < len(p.toks); i++ {
		t := p.toks[i]
		switch t.k {
		case tIdent, tNum:
			continue
		case tPunct:
			switch t.text {
			case "<":
				depth++
			case ">":
				depth--
			case ">>":
				depth -= 2
			case ">>>":
				depth -= 3
			case ",", ".", "::", "?", "[", "]", "*", "&", "|", "(", ")", "...":
			default:
				return 0, false
			}
			if depth <= 0 {
				return i + 1, depth == 0
			}
		default:
			return 0, false
		}
	}
	return 0, false
}

// args membaca argumen panggilan.
func (p *parser) args(open, close string) []ir.Expr {
	p.expect(open, "")
	var out []ir.Expr
	for !p.is(close) && !p.at(tEOF) {
		start := p.tok().pos
		var a ir.Expr
		switch {
		case p.accept("..."):
			a = &ir.Spread{S: p.span(start), X: p.expr()}
			a.(*ir.Spread).S = p.span(start)
		case p.d.Dollar && p.at(tIdent) && p.isAt(1, ":") && !p.isAt(1, "::"):
			name := p.next().text
			p.next()
			a = &ir.KwArg{Name: name, Value: p.expr()}
			a.(*ir.KwArg).S = p.span(start)
		default:
			a = p.expr()
		}
		out = append(out, a)
		if !p.accept(",") {
			break
		}
	}
	p.expect(close, "to close the argument list")
	return out
}

func (p *parser) primary() ir.Expr {
	t := p.tok()
	start := t.pos
	switch t.k {
	case tNum:
		p.next()
		return numLit(t)
	case tStr:
		p.next()
		return &ir.Lit{S: t.spanOf(), Kind: ir.LitString, Raw: t.text, Value: t.val}
	case tChar:
		p.next()
		return &ir.Lit{S: t.spanOf(), Kind: ir.LitChar, Raw: t.text, Value: t.val}
	case tRegex:
		p.next()
		return &ir.Lit{S: t.spanOf(), Kind: ir.LitRegex, Raw: t.text, Value: t.val}
	case tTemplate:
		return p.template(p.next())
	case tIdent:
		return p.identExpr()
	case tPunct:
		switch t.text {
		case "(":
			p.next()
			if p.is(")") {
				p.next()
				return &ir.BadExpr{S: p.span(start), Msg: "empty parentheses"}
			}
			x := p.exprSeq()
			p.expect(")", "to close the parenthesis")
			return x
		case "[":
			if p.d.Lang == "cpp" {
				return p.cppLambda()
			}
			return p.arrayLit("[", "]")
		case "{":
			if p.d.ASI {
				return p.objectLit()
			}
			return p.arrayLit("{", "}")
		case "...":
			p.next()
			x := p.expr()
			return &ir.Spread{S: p.span(start), X: x}
		case "\\":
			if p.d.Dollar { // \Namespace\Kelas
				p.next()
				return p.primary()
			}
		case "$":
			if p.d.Dollar { // $$nama
				p.next()
				x := p.primary()
				return &ir.Unary{S: p.span(start), Op: "$", X: x}
			}
		case "::":
			if p.d.Lang == "cpp" { // ::fungsi global
				p.next()
				return p.primary()
			}
		}
	case tEOF:
		p.errAt(t.pos, "unfinished expression")
		return &ir.BadExpr{S: t.spanOf(), Msg: "unfinished expression"}
	}
	p.errAt(t.pos, "cannot be read as an expression: "+t.text)
	if !p.is(";") && !p.is("}") && !p.is(")") {
		p.next()
	}
	return &ir.BadExpr{S: p.span(start), Msg: "invalid expression"}
}

func numLit(t *token) *ir.Lit {
	raw := t.text
	v := strings.ReplaceAll(strings.ReplaceAll(raw, "_", ""), "'", "")
	kind := ir.LitInt
	lower := strings.ToLower(v)
	hex := strings.HasPrefix(lower, "0x")
	switch {
	case strings.HasSuffix(lower, "n") && !hex:
		kind = ir.LitBigInt
		v = v[:len(v)-1]
	case strings.ContainsAny(v, ".") || !hex && strings.ContainsAny(v, "eE") || !hex && strings.HasSuffix(lower, "f") || strings.HasSuffix(lower, "d") && !hex:
		kind = ir.LitFloat
	}
	v = strings.TrimRight(v, "lLuUfFdDmM")
	if v == "" {
		v = "0"
	}
	return &ir.Lit{S: t.spanOf(), Kind: kind, Raw: raw, Value: v}
}

func (p *parser) identExpr() ir.Expr {
	t := p.tok()
	start := t.pos
	name := t.text
	lower := strings.ToLower(name)
	switch {
	case name == "true" || name == "false" || p.d.Dollar && (lower == "true" || lower == "false"):
		p.next()
		return &ir.Lit{S: t.spanOf(), Kind: ir.LitBool, Raw: name, Value: lower}
	case name == "null" || name == "nullptr" || name == "NULL" || p.d.Dollar && lower == "null":
		p.next()
		return &ir.Lit{S: t.spanOf(), Kind: ir.LitNull, Raw: name, Value: "null"}
	case name == "undefined" && p.d.ASI:
		p.next()
		return &ir.Lit{S: t.spanOf(), Kind: ir.LitUndefined, Raw: name, Value: "undefined"}
	case name == "this" || name == "super" || p.d.Dollar && (name == "$this" || lower == "self" || lower == "parent" || lower == "static"):
		p.next()
		return &ir.This{S: t.spanOf(), Name: strings.TrimPrefix(name, "$")}
	case name == "function" && (p.d.ASI || p.d.Dollar):
		f := p.funcDecl(start, "")
		return f
	case name == "async" && p.d.ASI && p.isAt(1, "function") && !p.peekTok(1).nl:
		p.next()
		f := p.funcDecl(start, "")
		f.Async = true
		return f
	case name == "fn" && p.d.Dollar && p.isAt(1, "("):
		p.next()
		f := &ir.Func{Arrow: true, Params: p.params()}
		if p.accept(":") {
			f.Ret, _ = p.typeText(false)
		}
		f.Header = p.span(start)
		p.expect("=>", "in the arrow function")
		f.ExprBody = p.assign()
		f.S = p.span(start)
		return f
	case name == "class" && (p.d.ASI || p.d.Dollar):
		return p.classDecl(start, nil)
	case (lower == "array" || lower == "list") && p.d.Dollar && p.isAt(1, "("):
		p.next()
		x := p.arrayLit("(", ")")
		if lower == "list" {
			if a, ok := x.(*ir.ArrayLit); ok {
				a.Kind = "list"
			}
		}
		return x
	case name == "match" && p.d.Dollar && p.isAt(1, "("):
		return p.phpMatch()
	case (name == "static_cast" || name == "dynamic_cast" || name == "reinterpret_cast" || name == "const_cast") && p.d.Lang == "cpp":
		p.next()
		typ := ""
		if p.is("<") {
			s := p.tok().pos
			p.skipBalanced()
			typ = strings.TrimSpace(strings.Trim(p.text(p.span(s)), "<>"))
		}
		p.expect("(", "")
		x := p.expr()
		p.expect(")", "")
		return &ir.Cast{S: p.span(start), Type: typ, X: x}
	}
	p.next()
	id := &ir.Ident{S: t.spanOf(), Name: name}
	// PHP: nama berpaket \A\B
	if p.d.Dollar {
		for p.is("\\") && p.peekTok(1).k == tIdent {
			p.next()
			id.Name += "\\" + p.next().text
		}
		id.S = p.span(start)
	}
	return id
}

// tryArrow mengenali fungsi panah: x => ..., (a, b) => ..., async (x) => ... (JS/TS),
// x -> ..., (a, b) -> ... (Java).
func (p *parser) tryArrow() ir.Expr {
	arrow := ""
	switch {
	case p.d.ASI:
		arrow = "=>"
	case p.d.Lang == "java":
		arrow = "->"
	default:
		return nil
	}
	start := p.tok().pos
	async := false
	j := p.i
	if p.d.ASI && p.is("async") && !p.peekTok(1).nl && (p.peekTok(1).k == tIdent || p.isAt(1, "(")) {
		async = true
		j++
	}
	t := p.toks[j]
	switch {
	case t.k == tIdent && j+1 < len(p.toks) && p.toks[j+1].k == tPunct && p.toks[j+1].text == arrow:
		p.i = j + 1
		p.next()
		f := &ir.Func{Arrow: true, Async: async, Params: []*ir.Param{{Name: t.text}}}
		f.Header = p.span(start)
		p.arrowBody(f)
		f.S = p.span(start)
		return f
	case t.k == tPunct && t.text == "(":
		end := p.matchParen(j)
		if end < 0 || end+1 >= len(p.toks) {
			return nil
		}
		after := p.toks[end+1]
		isArrow := after.k == tPunct && after.text == arrow
		if !isArrow && p.d.TSTypes && after.k == tPunct && after.text == ":" {
			// (a: number): number => ...
			for k := end + 2; k < len(p.toks) && k < end+40; k++ {
				tk := p.toks[k]
				if tk.k == tPunct && tk.text == "=>" {
					isArrow = true
					break
				}
				if tk.k == tPunct && (tk.text == ";" || tk.text == "?" || tk.text == "}" || tk.text == "=") || tk.k == tEOF {
					break
				}
			}
		}
		if !isArrow {
			return nil
		}
		p.i = j
		f := &ir.Func{Arrow: true, Async: async}
		if p.d.Lang == "java" {
			f.Params = p.javaLambdaParams()
		} else {
			f.Params = p.params()
		}
		if p.accept(":") {
			f.Ret = p.tsType(true)
		}
		f.Header = p.span(start)
		p.expect(arrow, "in the arrow function")
		p.arrowBody(f)
		f.S = p.span(start)
		return f
	}
	return nil
}

func (p *parser) javaLambdaParams() []*ir.Param {
	var out []*ir.Param
	p.expect("(", "")
	for !p.is(")") && !p.at(tEOF) {
		var names []string
		for !p.is(",") && !p.is(")") && !p.at(tEOF) {
			t := p.next()
			if t.k == tIdent {
				names = append(names, t.text)
			}
		}
		if len(names) > 0 {
			pr := &ir.Param{Name: names[len(names)-1]}
			if len(names) > 1 {
				pr.Type = strings.Join(names[:len(names)-1], " ")
			}
			out = append(out, pr)
		}
		p.accept(",")
	}
	p.expect(")", "")
	return out
}

func (p *parser) arrowBody(f *ir.Func) {
	if p.is("{") {
		f.Body = p.block()
	} else {
		f.ExprBody = p.assign()
	}
}

// matchParen mengembalikan indeks ')' yang menutup '(' di indeks j, atau -1.
func (p *parser) matchParen(j int) int {
	depth := 0
	for k := j; k < len(p.toks); k++ {
		t := p.toks[k]
		if t.k != tPunct {
			if t.k == tEOF {
				return -1
			}
			continue
		}
		switch t.text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
			if depth == 0 {
				if t.text != ")" {
					return -1
				}
				return k
			}
		}
	}
	return -1
}

// cppLambda: [captures](params) mutable -> T { body }
func (p *parser) cppLambda() ir.Expr {
	start := p.tok().pos
	p.skipBalanced() // daftar capture
	f := &ir.Func{Arrow: true}
	if p.is("(") {
		f.Params = p.params()
	}
	for p.is("mutable") || p.is("constexpr") || p.is("noexcept") {
		p.next()
	}
	if p.accept("->") {
		f.Ret, _ = p.typeText(true)
	}
	f.Header = p.span(start)
	if p.is("{") {
		f.Body = p.block()
	} else {
		p.errAt(p.tok().pos, "missing lambda body { }")
	}
	f.S = p.span(start)
	return f
}

// arrayLit: [1, 2] / {1, 2} / array(1, 2); PHP berkey → ObjectLit.
func (p *parser) arrayLit(open, close string) ir.Expr {
	start := p.tok().pos
	p.next()
	arr := &ir.ArrayLit{Kind: "array"}
	if open == "{" {
		arr.Kind = "init"
	}
	var props []*ir.Prop
	keyed := false
	for !p.is(close) && !p.at(tEOF) {
		es := p.tok().pos
		if p.d.ASI && p.is(",") { // [1, , 3]: lubang
			p.next()
			arr.Elems = append(arr.Elems, &ir.Lit{S: p.span(es), Kind: ir.LitUndefined, Value: "undefined"})
			continue
		}
		var x ir.Expr
		if p.accept("...") {
			x = &ir.Spread{X: p.expr()}
			x.(*ir.Spread).S = p.span(es)
		} else if open == "{" && p.is(".") && p.peekTok(1).k == tIdent && p.isAt(2, "=") {
			// C99 designated initializer: {.x = 1}
			p.next()
			name := p.next().text
			p.next()
			keyed = true
			props = append(props, &ir.Prop{S: p.span(es), Key: &ir.Ident{Name: name}, Value: p.expr()})
			if !p.accept(",") {
				break
			}
			continue
		} else {
			x = p.expr()
		}
		if p.d.Dollar && p.accept("=>") {
			keyed = true
			v := p.expr()
			props = append(props, &ir.Prop{S: p.span(es), Key: x, Value: v, Computed: true})
		} else {
			props = append(props, &ir.Prop{S: p.span(es), Value: x})
			arr.Elems = append(arr.Elems, x)
		}
		if !p.accept(",") {
			break
		}
	}
	p.expect(close, "to close the array")
	if keyed {
		kind := "array"
		if open == "{" {
			kind = "struct"
		}
		return &ir.ObjectLit{S: p.span(start), Props: props, Kind: kind}
	}
	arr.S = p.span(start)
	return arr
}

// objectLit: { a: 1, b, [k]: v, ...rest, m() { } } (JavaScript/TypeScript).
func (p *parser) objectLit() ir.Expr {
	start := p.tok().pos
	p.next()
	obj := &ir.ObjectLit{Kind: "object"}
	for !p.is("}") && !p.at(tEOF) {
		ps := p.tok().pos
		pr := &ir.Prop{}
		if p.accept("...") {
			pr.Spread = true
			pr.Value = p.expr()
			pr.S = p.span(ps)
			obj.Props = append(obj.Props, pr)
			if !p.accept(",") {
				break
			}
			continue
		}
		async, gen := false, false
		if p.is("async") && !p.isAt(1, ":") && !p.isAt(1, "(") && !p.isAt(1, ",") {
			p.next()
			async = true
		}
		if p.accept("*") {
			gen = true
		}
		accessor := ""
		if (p.is("get") || p.is("set")) && !p.isAt(1, ":") && !p.isAt(1, "(") && !p.isAt(1, ",") && !p.isAt(1, "}") {
			accessor = p.next().text
		}
		kt := p.tok()
		switch {
		case p.is("["):
			p.next()
			pr.Key = p.expr()
			pr.Computed = true
			p.expect("]", "")
		case kt.k == tStr || kt.k == tNum:
			p.next()
			pr.Key = &ir.Lit{S: kt.spanOf(), Kind: ir.LitString, Raw: kt.text, Value: litKeyValue(kt)}
		default:
			p.next()
			pr.Key = &ir.Ident{S: kt.spanOf(), Name: kt.text}
		}
		switch {
		case p.is("(") || p.is("<"):
			if p.is("<") {
				p.skipBalanced()
			}
			f := &ir.Func{Async: async, Generator: gen, Kind: accessor, Params: p.params()}
			if id, ok := pr.Key.(*ir.Ident); ok {
				f.Name = id.Name
			}
			if p.accept(":") {
				f.Ret = p.tsType(false)
			}
			f.Header = p.span(ps)
			f.Body = p.block()
			f.S = p.span(ps)
			pr.Value, pr.Method = f, true
		case p.accept(":"):
			pr.Value = p.expr()
		default:
			pr.Short = true
			pr.Value = pr.Key
			if p.accept("=") { // pola dengan nilai bawaan: {a = 1} = obj
				pr.Value = &ir.Assign{S: p.span(ps), Op: "=", L: pr.Key, R: p.expr()}
			}
		}
		pr.S = p.span(ps)
		obj.Props = append(obj.Props, pr)
		if !p.accept(",") {
			break
		}
	}
	p.expect("}", "to close the object")
	obj.S = p.span(start)
	return obj
}

func litKeyValue(t *token) string {
	if t.k == tStr {
		return t.val
	}
	return numLit(t).Value
}

// template membangun ir.Template dari token template/string berinterpolasi.
func (p *parser) template(t *token) *ir.Template {
	tm := &ir.Template{S: t.spanOf(), Parts: t.parts}
	for _, r := range t.exprs {
		var src string
		base := ir.Pos{Off: r[0], Line: t.pos.Line, Col: t.pos.Col}
		if t.text == "<<<" { // heredoc: rentang relatif ke t.val
			if r[1] <= len(t.val) {
				src = t.val[r[0]:r[1]]
			}
			base.Off = t.pos.Off
		} else if r[1] <= len(p.src) {
			src = p.src[r[0]:r[1]]
			base = p.posAt(r[0], t.pos)
		}
		if strings.TrimSpace(src) == "" {
			tm.Exprs = append(tm.Exprs, &ir.BadExpr{S: t.spanOf(), Msg: "empty interpolation"})
			continue
		}
		tm.Exprs = append(tm.Exprs, parseSub(p, src, base))
	}
	if len(tm.Parts) != len(tm.Exprs)+1 {
		for len(tm.Parts) < len(tm.Exprs)+1 {
			tm.Parts = append(tm.Parts, "")
		}
	}
	return tm
}

// posAt menghitung baris/kolom untuk offset off, mulai dari posisi from yang diketahui.
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

// newExpr: new X(args), new int[5], new int[]{1, 2}, new ArrayList<>(), new class { }.
func (p *parser) newExpr() ir.Expr {
	start := p.tok().pos
	p.next()
	n := &ir.New{}
	if p.d.ASI && p.is(".") { // new.target
		p.next()
		p.next()
		return &ir.Ident{S: p.span(start), Name: "new.target"}
	}
	if p.d.Dollar && p.is("class") {
		c := p.classDecl(p.tok().pos, nil)
		n.Anon = c
		n.S = p.span(start)
		return n
	}
	if p.d.Types {
		ts := p.tok().pos
		typ, _ := p.typeText(false)
		n.Type = typ
		n.Class = &ir.Ident{S: p.span(ts), Name: typ}
		for p.is("[") {
			p.next()
			if p.is("]") {
				n.Dims = append(n.Dims, nil)
			} else {
				n.Dims = append(n.Dims, p.expr())
			}
			p.expect("]", "")
			n.Type += "[]"
		}
		if p.is("{") {
			if a, ok := p.arrayLit("{", "}").(*ir.ArrayLit); ok {
				if len(n.Dims) == 0 { // C++ new Point{1, 2}
					n.Args = a.Elems
				} else {
					n.Init = a
				}
			}
		} else if p.is("(") {
			n.Args = p.args("(", ")")
		}
		if p.d.Lang == "java" && p.is("{") { // kelas anonim
			c := &ir.Class{Kind: "class", Name: "", Extends: []string{typ}}
			cs := p.tok().pos
			p.classBody(c)
			c.S = p.span(cs)
			n.Anon = c
		}
		n.S = p.span(start)
		return n
	}
	// JS/TS/PHP: ekspresi kelas tanpa panggilan, lalu argumen
	cs := p.tok().pos
	var cls ir.Expr
	if p.is("(") {
		p.next()
		cls = p.expr()
		p.expect(")", "")
	} else {
		cls = p.primary()
	}
	for {
		if p.is(".") && !p.d.Dollar || p.is("::") {
			op := p.next().text
			cls = &ir.Member{S: p.span(cs), X: cls, Name: p.next().text, Op: op}
			continue
		}
		if p.is("[") && p.d.ASI {
			p.next()
			i := p.expr()
			p.expect("]", "")
			cls = &ir.Index{S: p.span(cs), X: cls, I: i}
			continue
		}
		break
	}
	if p.d.TSTypes && p.is("<") {
		p.skipBalanced()
	}
	n.Class = cls
	n.Type = strings.TrimSpace(p.text(p.span(cs)))
	if p.is("(") {
		n.Args = p.args("(", ")")
	}
	n.S = p.span(start)
	return n
}

// phpMatch mengubah match($x) { 1, 2 => 'a', default => 'b' } menjadi rantai ?: dengan ===.
func (p *parser) phpMatch() ir.Expr {
	start := p.tok().pos
	p.next()
	p.expect("(", "")
	subj := p.expr()
	p.expect(")", "")
	p.expect("{", "after match")
	type arm struct {
		conds []ir.Expr
		val   ir.Expr
	}
	var arms []arm
	var def ir.Expr
	for !p.is("}") && !p.at(tEOF) {
		if p.accept("default") {
			p.expect("=>", "")
			def = p.expr()
		} else {
			var a arm
			for {
				a.conds = append(a.conds, p.expr())
				if !p.accept(",") || p.is("=>") {
					break
				}
			}
			p.expect("=>", "in match")
			a.val = p.expr()
			arms = append(arms, a)
		}
		if !p.accept(",") {
			break
		}
	}
	p.expect("}", "to close match")
	sp := p.span(start)
	var out ir.Expr = def
	if out == nil {
		out = &ir.Unary{S: sp, Op: "throw", X: &ir.New{S: sp, Type: "UnhandledMatchError", Class: &ir.Ident{S: sp, Name: "UnhandledMatchError"}}}
	}
	for k := len(arms) - 1; k >= 0; k-- {
		var c ir.Expr
		for _, x := range arms[k].conds {
			var eq ir.Expr = &ir.Binary{S: x.Span(), Op: "===", X: subj, Y: x}
			if lit, ok := subj.(*ir.Lit); ok && lit.Kind == ir.LitBool && lit.Value == "true" {
				eq = x // match(true) { $x > 3 => ... }
			}
			if c == nil {
				c = eq
			} else {
				c = &ir.Binary{S: sp, Op: "||", X: c, Y: eq}
			}
		}
		out = &ir.Cond{S: sp, C: c, T: arms[k].val, F: out}
	}
	return out
}
