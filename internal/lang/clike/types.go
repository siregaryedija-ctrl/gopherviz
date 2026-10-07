package clike

import (
	"strconv"
	"strings"

	"gopherviz/internal/ir"
)

func builtinTypes(lang string) map[string]bool {
	m := map[string]bool{}
	add := func(s string) {
		for _, w := range strings.Fields(s) {
			m[w] = true
		}
	}
	switch lang {
	case "c", "cpp":
		add("int char float double void short long unsigned signed bool _Bool size_t ssize_t FILE wchar_t " +
			"int8_t int16_t int32_t int64_t uint8_t uint16_t uint32_t uint64_t")
		if lang == "cpp" {
			add("auto string vector map unordered_map set unordered_set multimap multiset pair tuple list deque " +
				"queue stack priority_queue array optional function string_view ostream istream iostream char16_t char32_t std")
		}
	case "java":
		add("int long short byte char boolean float double void var String Integer Long Short Byte Character " +
			"Boolean Float Double Object List ArrayList LinkedList Map HashMap TreeMap LinkedHashMap Set HashSet " +
			"TreeSet Queue Deque ArrayDeque Stack Iterator Scanner StringBuilder Random Optional Exception " +
			"RuntimeException Thread Runnable Comparator Collection Iterable Number")
	}
	return m
}

// prescanTypes mendaftarkan nama kelas/struct/typedef yang dideklarasikan di file supaya
// "Point p;" atau "(Node*) x" dikenali sebagai deklarasi/cast.
func (p *parser) prescanTypes() {
	for i := 0; i+1 < len(p.toks); i++ {
		t := p.toks[i]
		if t.k != tIdent {
			continue
		}
		switch t.text {
		case "class", "struct", "interface", "enum", "union", "record", "trait":
			if n := p.toks[i+1]; n.k == tIdent {
				p.types[n.text] = true
			}
		case "typedef":
			depth, last := 0, ""
			for j := i + 1; j < len(p.toks); j++ {
				tj := p.toks[j]
				if tj.k == tPunct {
					switch tj.text {
					case "{", "(":
						depth++
					case "}", ")":
						depth--
					}
					if tj.text == ";" && depth == 0 {
						break
					}
				}
				if tj.k == tIdent && depth == 0 {
					last = tj.text
				}
				if tj.k == tEOF || tj.k == tPunct && tj.text == ";" && depth == 0 {
					break
				}
			}
			if last != "" {
				p.types[last] = true
			}
		}
	}
}

func isTypeQual(s string) bool {
	switch s {
	case "const", "volatile", "unsigned", "signed", "short", "long", "struct", "enum", "union", "typename",
		"static", "final", "extern", "register", "inline", "constexpr", "mutable", "thread_local":
		return true
	}
	return false
}

// notType: kata yang tidak pernah menjadi awal tipe di posisi deklarasi.
func notType(s string) bool {
	switch s {
	case "return", "new", "delete", "throw", "sizeof", "case", "goto", "else", "do", "if", "for", "while",
		"switch", "break", "continue", "default", "try", "catch", "this", "true", "false", "null", "nullptr",
		"NULL", "super", "typeof", "instanceof", "yield", "co_return", "co_await", "operator", "public",
		"private", "protected", "using", "namespace", "typedef", "template", "friend", "echo", "print":
		return true
	}
	return false
}

// typeText membaca nama tipe C/C++/Java/PHP dan mengembalikan teksnya.
// arr: ikut membaca "[]" (tipe array Java).
func (p *parser) typeText(arr bool) (string, bool) {
	start := p.tok().pos
	consumed := false
	if p.d.Dollar {
		p.accept("?")
		for {
			p.accept("\\")
			if !p.at(tIdent) || strings.HasPrefix(p.tok().text, "$") {
				break
			}
			p.next()
			consumed = true
			for p.is("\\") && p.peekTok(1).k == tIdent {
				p.next()
				p.next()
			}
			if !p.accept("|") && !p.accept("&") {
				break
			}
		}
		return strings.TrimSpace(p.text(p.span(start))), consumed
	}
	for p.at(tIdent) && isTypeQual(p.tok().text) {
		p.next()
		consumed = true
	}
	if p.at(tIdent) && !notType(p.tok().text) && !(consumed && isWordAfterQual(p)) {
		p.next()
		consumed = true
		for {
			if p.is("::") && p.peekTok(1).k == tIdent || p.d.Lang == "java" && p.is(".") && p.peekTok(1).k == tIdent {
				p.next()
				p.next()
				continue
			}
			if p.is("<") {
				if _, ok := p.typeArgsEnd(p.i); ok {
					p.skipBalanced()
					continue
				}
			}
			break
		}
	} else if p.at(tIdent) && notType(p.tok().text) && !consumed {
		return "", false
	}
	for p.at(tIdent) && (p.tok().text == "const" || p.tok().text == "int" && consumed || p.tok().text == "long" || p.tok().text == "double") {
		p.next() // "long long int", "char const"
	}
	if !consumed {
		return "", false
	}
	if arr {
		for p.is("[") && p.isAt(1, "]") {
			p.next()
			p.next()
		}
	}
	if p.d.Lang == "java" && p.is("...") {
		// varargs ditangani pemanggil
	}
	return strings.TrimSpace(p.text(p.span(start))), true
}

// isWordAfterQual: setelah "unsigned"/"long" dsb, identifier berikutnya adalah nama
// variabel (unsigned x;) bila diikuti = ; , [ ) — bukan nama tipe.
func isWordAfterQual(p *parser) bool {
	n := p.peekTok(1)
	if n.k != tPunct {
		return false
	}
	switch n.text {
	case "=", ";", ",", "[", ")", ":":
		return true
	}
	return false
}

// tsType melewati anotasi tipe TypeScript dan mengembalikan teksnya.
// arrowStop: berhenti di "=>" (tipe kembali fungsi panah).
func (p *parser) tsType(arrowStop bool) string {
	start := p.tok().pos
	depth := 0
	consumed := false
	lastOp := true
	for !p.at(tEOF) {
		t := p.tok()
		if depth == 0 {
			if t.k == tPunct {
				switch t.text {
				case "=", ",", ";", ")", "]", "}", ">", ">>":
					return strings.TrimSpace(p.text(p.span(start)))
				case "{":
					if consumed && !lastOp {
						return strings.TrimSpace(p.text(p.span(start)))
					}
				case "=>":
					if arrowStop {
						return strings.TrimSpace(p.text(p.span(start)))
					}
				}
			}
			if t.nl && consumed && !lastOp {
				return strings.TrimSpace(p.text(p.span(start)))
			}
			if t.k == tIdent && consumed && !lastOp && (t.text == "as" || t.text == "of" || t.text == "in") {
				return strings.TrimSpace(p.text(p.span(start)))
			}
		}
		if t.k == tPunct {
			switch t.text {
			case "(", "[", "{", "<":
				depth++
			case ")", "]", "}", ">":
				depth--
			case ">>":
				depth -= 2
			}
			lastOp = strings.Contains("|&:,(<[{=>?.", t.text) || t.text == "=>" || t.text == "..."
			if t.text == ")" || t.text == "]" || t.text == "}" || t.text == ">" {
				lastOp = false
			}
		} else {
			lastOp = t.k == tIdent && (t.text == "keyof" || t.text == "typeof" || t.text == "readonly" || t.text == "infer" || t.text == "unique")
		}
		p.next()
		consumed = true
		if depth < 0 {
			break
		}
	}
	return strings.TrimSpace(p.text(p.span(start)))
}

// tryDecl mencoba membaca deklarasi bertipe (C/C++/Java): variabel, fungsi, atau method.
// Mengembalikan nil (posisi dipulihkan) bila statement ini bukan deklarasi.
func (p *parser) tryDecl(start ir.Pos, inFor bool) ir.Stmt {
	save, saveDiags := p.i, len(p.diags)
	restore := func() ir.Stmt {
		p.i, p.diags = save, p.diags[:saveDiags]
		return nil
	}
	if p.d.Lang == "java" && p.is("<") { // method generik: <T> void f()
		p.skipBalanced()
	}
	typ, ok := p.typeText(true)
	if !ok {
		return restore()
	}
	// konstruktor/destruktor C++ di luar kelas: Point::Point(...) / Point::~Point()
	if p.is("(") && strings.Contains(typ, "::") && !inFor {
		parts := strings.Split(typ, "::")
		n := len(parts)
		if n >= 2 && (parts[n-1] == parts[n-2]) {
			return p.funcRest(start, strings.Join(parts[:n-1], "::"), parts[n-1], "")
		}
	}
	if p.is("~") && strings.HasSuffix(typ, "::") {
		return restore()
	}
	// konstruktor di dalam kelas: Point(int x) { }
	if p.is("(") && p.inCls != "" && typ == p.inCls && !inFor {
		return p.funcRest(start, p.inCls, typ, "")
	}
	baseKnown := p.knownType(typ)
	ptr := ""
	for p.is("*") || p.is("&") || p.is("&&") {
		if !baseKnown {
			return restore() // a * b; / a & b; bukan deklarasi kecuali tipenya dikenal
		}
		ptr += p.next().text
	}
	switch {
	case p.at(tIdent) && !notType(p.tok().text) || p.is("operator"):
	case p.is("[") && (typ == "auto" || strings.HasSuffix(typ, "auto")) && p.d.Lang == "cpp":
	default:
		return restore()
	}
	d := &ir.VarDecl{Kind: typ}
	for first := true; ; first = false {
		ss := p.tok().pos
		if !first {
			for p.is("*") || p.is("&") {
				p.next()
			}
		}
		spec := &ir.VarSpec{Type: typ + ptr, Ref: strings.Contains(ptr, "&")}
		if p.is("[") { // auto [a, b] = ...
			spec.Pattern = p.arrayLit("[", "]")
		} else {
			nameTok := p.next()
			spec.Name = nameTok.text
			if nameTok.text == "operator" { // operator overloading
				for !p.is("(") && !p.at(tEOF) {
					spec.Name += p.next().text
				}
				if p.is("(") && p.isAt(1, ")") && p.isAt(2, "(") {
					p.next()
					p.next()
					spec.Name += "()"
				}
			}
			cls := ""
			for p.is("::") && (p.peekTok(1).k == tIdent || p.isAt(1, "~")) { // int Point::area()
				p.next()
				cls = spec.Name
				if cls != "" && strings.Contains(typ, "") {
				}
				name := ""
				if p.accept("~") {
					name = "~"
				}
				spec.Name = name + p.next().text
			}
			if p.is("(") && first {
				if fn := p.funcOrCtor(start, typ+ptr, spec, cls, inFor); fn != nil {
					return fn
				}
				if spec.Ctor == nil && len(p.diags) > saveDiags {
					return restore()
				}
			}
		}
		for p.is("[") { // int a[5][3]
			p.next()
			if p.is("]") {
				spec.Dims = append(spec.Dims, nil)
			} else {
				spec.Dims = append(spec.Dims, p.expr())
			}
			p.expect("]", "untuk menutup ukuran array")
		}
		if p.is(":") && !inFor && p.peekTok(1).k == tNum { // bitfield
			p.next()
			p.next()
		}
		switch {
		case p.accept("="):
			spec.Init = p.expr()
		case p.is("{") && p.d.Lang == "cpp":
			spec.Init = p.arrayLit("{", "}")
		}
		spec.S = p.span(ss)
		d.Specs = append(d.Specs, spec)
		if !p.accept(",") {
			break
		}
	}
	if inFor && p.is(":") {
		return restore()
	}
	if !inFor {
		p.semi()
	}
	d.S = p.span(start)
	return d
}

func (p *parser) knownType(typ string) bool {
	base := typ
	if i := strings.IndexByte(base, '<'); i >= 0 {
		return true
	}
	if strings.Contains(base, "::") {
		return true
	}
	fs := strings.Fields(base)
	if len(fs) == 0 {
		return false
	}
	for _, w := range fs {
		if isTypeQual(w) && w != "static" && w != "final" {
			return true
		}
	}
	return p.types[fs[len(fs)-1]]
}

// funcOrCtor memutuskan apakah "Tipe nama(" adalah definisi fungsi, prototipe, atau
// variabel dengan argumen konstruktor (C++: Point p(1, 2);).
func (p *parser) funcOrCtor(start ir.Pos, typ string, spec *ir.VarSpec, cls string, inFor bool) ir.Stmt {
	end := p.matchParen(p.i)
	if end < 0 {
		return p.funcRest(start, cls, spec.Name, typ)
	}
	k := end + 1
	for k < len(p.toks) && p.toks[k].k == tIdent && (p.toks[k].text == "const" || p.toks[k].text == "override" ||
		p.toks[k].text == "noexcept" || p.toks[k].text == "final" || p.toks[k].text == "throws" || p.toks[k].text == "volatile") {
		k++
		if p.toks[k-1].text == "throws" {
			for k < len(p.toks) && (p.toks[k].k == tIdent || p.toks[k].k == tPunct && (p.toks[k].text == "," || p.toks[k].text == ".")) {
				k++
			}
		}
	}
	after := p.toks[k]
	if after.k == tPunct && (after.text == "{" || after.text == "->" || after.text == ":" && p.d.Lang == "cpp" && !inFor) || after.k == tEOF {
		return p.funcRest(start, cls, spec.Name, typ)
	}
	if p.d.Lang == "java" && after.k == tPunct && after.text == ";" {
		return p.funcRest(start, cls, spec.Name, typ) // method abstrak/interface
	}
	if after.k == tPunct && (after.text == ";" || after.text == "=") {
		// prototipe bila isi kurung tampak seperti parameter bertipe (atau kosong)
		if p.looksLikeParams(p.i+1, end) {
			p.i = end + 1
			for p.i < k {
				p.next()
			}
			if p.accept("=") { // = 0 / = default / = delete
				p.next()
			}
			p.semi()
			return &ir.Simple{S: p.span(start), Kind: "prototype", Names: []string{spec.Name}}
		}
		spec.Ctor = p.args("(", ")")
		return nil
	}
	if after.k == tPunct && after.text == "," {
		spec.Ctor = p.args("(", ")")
		return nil
	}
	return p.funcRest(start, cls, spec.Name, typ)
}

func (p *parser) looksLikeParams(from, to int) bool {
	if from >= to {
		return true // ()
	}
	t := p.toks[from]
	if t.k != tIdent {
		return false
	}
	if t.text == "void" || isTypeQual(t.text) || p.types[t.text] {
		return true
	}
	// "Tipe nama" — dua identifier berurutan
	return from+1 < to && p.toks[from+1].k == tIdent
}

// funcRest membaca sisa definisi fungsi mulai dari "(".
func (p *parser) funcRest(start ir.Pos, cls, name, ret string) ir.Stmt {
	f := &ir.Func{Name: name, Class: cls, Ret: ret}
	f.Params = p.params()
	if len(f.Params) == 1 && f.Params[0].Type == "void" && f.Params[0].Name == "" {
		f.Params = nil
	}
	for p.is("const") || p.is("override") || p.is("noexcept") || p.is("final") || p.is("volatile") {
		p.next()
	}
	if p.accept("throws") {
		for p.at(tIdent) || p.is(",") || p.is(".") {
			p.next()
		}
	}
	if p.accept("->") {
		f.Ret, _ = p.typeText(true)
	}
	if p.is(":") && p.d.Lang == "cpp" { // daftar inisialisasi anggota: : x(x), y(y)
		p.next()
		for p.at(tIdent) {
			is := p.tok().pos
			name := p.next()
			var args []ir.Expr
			switch {
			case p.is("("):
				args = p.args("(", ")")
			case p.is("{"):
				args = p.args("{", "}")
			default:
				p.errAt(p.tok().pos, "kurang \"(\" di daftar inisialisasi")
			}
			f.Inits = append(f.Inits, &ir.Call{S: p.span(is), Fun: &ir.Ident{S: name.spanOf(), Name: name.text}, Args: args})
			if !p.accept(",") {
				break
			}
		}
	}
	f.Header = p.span(start)
	switch {
	case p.is("{"):
		f.Body = p.block()
	case p.accept("="): // = default / = delete / = 0
		p.next()
		p.semi()
	default:
		p.semi()
	}
	f.S = p.span(start)
	return f
}

// typedef: typedef struct { ... } Point; / typedef int Angka;
func (p *parser) typedef(start ir.Pos) ir.Stmt {
	p.next()
	if (p.is("struct") || p.is("union") || p.is("enum")) && (p.isAt(1, "{") || p.peekTok(1).k == tIdent && p.isAt(2, "{")) {
		c := p.classDecl(start, []string{"typedef"})
		// nama alias setelah }
		for !p.is(";") && !p.at(tEOF) {
			t := p.next()
			if t.k == tIdent {
				if c.Name == "" {
					c.Name = t.text
				}
				p.types[t.text] = true
			}
		}
		p.accept(";")
		c.S = p.span(start)
		return c
	}
	last := ""
	for !p.is(";") && !p.at(tEOF) {
		if p.is("(") {
			p.skipBalanced()
			continue
		}
		t := p.next()
		if t.k == tIdent {
			last = t.text
		}
	}
	p.accept(";")
	if last != "" {
		p.types[last] = true
	}
	return &ir.Simple{S: p.span(start), Kind: "type", Names: []string{last}}
}

// classDecl membaca class/struct/interface/enum/trait/record.
func (p *parser) classDecl(start ir.Pos, mods []string) *ir.Class {
	c := &ir.Class{Kind: p.next().text}
	if c.Kind == "enum" && (p.is("class") || p.is("struct")) {
		p.next()
	}
	if p.at(tIdent) && !p.is("extends") && !p.is("implements") {
		c.Name = p.next().text
		p.types[c.Name] = true
	}
	if p.is("<") {
		p.skipBalanced()
	}
	if c.Kind == "record" && p.is("(") { // Java record Point(int x, int y)
		for _, pr := range p.params() {
			c.Members = append(c.Members, &ir.VarDecl{Kind: pr.Type, Specs: []*ir.VarSpec{{Name: pr.Name, Type: pr.Type}}})
		}
	}
	if p.d.Dollar && c.Kind == "class" && p.is("(") { // new class($x) { }
		p.args("(", ")")
	}
	// pewarisan: extends A implements B, C / : public A
	for !p.is("{") && !p.at(tEOF) && !p.is(";") {
		t := p.next()
		if t.k == tIdent {
			switch t.text {
			case "extends", "implements", "public", "private", "protected", "virtual", "with":
			default:
				c.Extends = append(c.Extends, t.text)
			}
		}
		if t.k == tPunct && t.text == "<" {
			p.i--
			p.skipBalanced()
		}
	}
	c.Header = p.span(start)
	if p.is(";") { // deklarasi maju: struct Node;
		p.next()
		c.S = p.span(start)
		return c
	}
	p.classBody(c)
	if p.d.Types && p.d.Lang != "java" && !contains(mods, "typedef") {
		// struct Point { ... } p1, p2;  → nama variabel dilewati
		for p.at(tIdent) || p.is(",") || p.is("*") {
			p.next()
		}
		if !p.accept(";") {
			p.errAt(p.prevEnd(), "kurang \";\" setelah definisi "+c.Kind)
		}
	} else if p.d.TSTypes || p.d.ASI {
		p.accept(";")
	}
	c.S = p.span(start)
	_ = mods
	return c
}

func (p *parser) classBody(c *ir.Class) {
	open := p.tok().pos
	if !p.expect("{", "untuk badan "+c.Kind) {
		return
	}
	prevCls := p.inCls
	p.inCls = c.Name
	defer func() { p.inCls = prevCls }()
	if c.Kind == "enum" {
		p.enumConstants(c)
	}
	for !p.is("}") && !p.at(tEOF) {
		before := p.i
		if m := p.member(c); m != nil {
			c.Members = append(c.Members, m)
		}
		if p.i == before {
			p.badSkip("tidak bisa dibaca di dalam " + c.Kind)
		}
	}
	if !p.accept("}") {
		p.errAt(open, c.Kind+" belum ditutup dengan }")
	}
}

// enumConstants membaca konstanta enum: A, B = 2, C("x") (Java), lalu ";" opsional.
func (p *parser) enumConstants(c *ir.Class) {
	if p.d.Dollar {
		return // PHP: "case A;" dibaca sebagai member
	}
	n := 0
	for p.at(tIdent) {
		ss := p.tok().pos
		name := p.next().text
		spec := &ir.VarSpec{Name: name}
		if p.accept("=") {
			spec.Init = p.expr()
		} else if p.is("(") {
			spec.Ctor = p.args("(", ")")
		} else {
			spec.Init = &ir.Lit{Kind: ir.LitInt, Value: itoa(n)}
		}
		if lit, ok := spec.Init.(*ir.Lit); ok && lit.Kind == ir.LitInt {
			if v, err := strconv.Atoi(lit.Value); err == nil {
				n = v // nilai berikutnya melanjutkan dari nilai eksplisit
			}
		}
		if p.is("{") { // konstanta enum Java dengan badan
			p.skipBalanced()
		}
		n++
		spec.S = p.span(ss)
		c.Members = append(c.Members, &ir.VarDecl{S: p.span(ss), Kind: "enum", Specs: []*ir.VarSpec{spec}})
		if !p.accept(",") {
			break
		}
	}
	p.accept(";")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// member membaca satu anggota kelas.
func (p *parser) member(c *ir.Class) ir.Stmt {
	start := p.tok().pos
	if p.accept(";") {
		return nil
	}
	for p.is("@") {
		p.annotation()
	}
	// label akses C++: public:
	if (p.is("public") || p.is("private") || p.is("protected")) && p.isAt(1, ":") && p.d.Lang == "cpp" {
		p.next()
		p.next()
		return nil
	}
	static := false
	for isModifier(p.tok().text) && p.at(tIdent) && !p.isAt(1, "(") && !p.isAt(1, "=") && !p.isAt(1, ";") && !p.isAt(1, ":") {
		if p.is("static") {
			static = true
			if p.d.Lang == "java" && p.isAt(1, "{") { // blok static
				p.next()
				b := p.block()
				return b
			}
		}
		if p.is("async") && p.d.ASI {
			break
		}
		p.next()
	}
	switch {
	case p.is("class") || p.is("interface") || p.is("enum") || p.is("struct") || p.is("record") || p.is("trait"):
		if p.peekTok(1).k == tIdent {
			return p.classDecl(p.tok().pos, nil)
		}
	case p.d.ASI:
		return p.jsMember(c, start, static)
	case p.d.Dollar:
		return p.phpMember(c, start, static)
	}
	// C++/Java
	if p.is("~") && p.d.Lang == "cpp" { // destruktor
		p.next()
		name := "~" + p.next().text
		return p.funcRest(start, c.Name, name, "")
	}
	if p.is("{") && p.d.Lang == "java" { // blok inisialisasi instance
		return p.block()
	}
	if p.is("friend") || p.is("using") || p.is("typedef") || p.is("template") {
		return p.stmt()
	}
	if s := p.tryDecl(start, false); s != nil {
		switch s := s.(type) {
		case *ir.Func:
			s.Class, s.Static = c.Name, static
		case *ir.VarDecl:
			if static {
				s.Kind = "static " + s.Kind
			}
		}
		return s
	}
	return p.badSkip("anggota " + c.Kind + " tidak bisa dibaca")
}

// jsMember: method, getter/setter, field (JavaScript/TypeScript).
func (p *parser) jsMember(c *ir.Class, start ir.Pos, static bool) ir.Stmt {
	for p.is("public") || p.is("private") || p.is("protected") || p.is("readonly") || p.is("abstract") || p.is("override") || p.is("declare") || (p.is("static") && !p.isAt(1, "(")) {
		if p.is("static") {
			static = true
			if p.isAt(1, "{") { // blok static
				p.next()
				return p.block()
			}
		}
		p.next()
	}
	async, gen := false, false
	if p.is("async") && !p.isAt(1, "(") && !p.isAt(1, "=") && !p.peekTok(1).nl {
		p.next()
		async = true
	}
	if p.accept("*") {
		gen = true
	}
	if (p.is("get") || p.is("set")) && !p.isAt(1, "(") && !p.isAt(1, "=") && !p.isAt(1, ";") && !p.isAt(1, ":") {
		p.next()
	}
	var name string
	switch t := p.tok(); {
	case p.is("["):
		s := p.tok().pos
		p.skipBalanced()
		name = p.text(p.span(s))
	case p.is("#"):
		p.next()
		name = "#" + p.next().text
	case t.k == tStr:
		name = p.next().val
	default:
		name = p.next().text
	}
	if p.d.TSTypes {
		p.accept("?")
		p.accept("!")
	}
	if p.is("(") || p.is("<") {
		if p.is("<") {
			p.skipBalanced()
		}
		f := &ir.Func{Name: name, Class: c.Name, Static: static, Async: async, Generator: gen}
		f.Params = p.params()
		if p.accept(":") {
			f.Ret = p.tsType(false)
		}
		f.Header = p.span(start)
		if p.is("{") {
			f.Body = p.block()
		} else {
			p.semi()
		}
		f.S = p.span(start)
		return f
	}
	d := &ir.VarDecl{Kind: "field"}
	if static {
		d.Kind = "static field"
	}
	spec := &ir.VarSpec{Name: name}
	if p.d.TSTypes && p.accept(":") {
		spec.Type = p.tsType(false)
	}
	if p.accept("=") {
		spec.Init = p.expr()
	}
	spec.S = p.span(start)
	d.Specs = []*ir.VarSpec{spec}
	p.semi()
	d.S = p.span(start)
	return d
}

// phpMember: method, properti, konstanta, case enum, use trait.
func (p *parser) phpMember(c *ir.Class, start ir.Pos, static bool) ir.Stmt {
	for isModifier(p.tok().text) || p.is("var") {
		if p.is("static") {
			static = true
		}
		p.next()
	}
	switch {
	case p.is("function"):
		f := p.funcDecl(start, c.Name)
		f.Static = static
		return f
	case p.is("const"):
		return p.stmt()
	case p.is("use"):
		return p.skipStmt("import")
	case p.is("case"):
		p.next()
		ss := p.tok().pos
		name := p.next().text
		spec := &ir.VarSpec{Name: name}
		if p.accept("=") {
			spec.Init = p.expr()
		}
		spec.S = p.span(ss)
		p.semi()
		return &ir.VarDecl{S: p.span(start), Kind: "enum", Specs: []*ir.VarSpec{spec}}
	}
	typ := ""
	if !strings.HasPrefix(p.tok().text, "$") {
		typ, _ = p.typeText(false)
	}
	d := &ir.VarDecl{Kind: "field"}
	if static {
		d.Kind = "static field"
	}
	for strings.HasPrefix(p.tok().text, "$") {
		ss := p.tok().pos
		spec := &ir.VarSpec{Name: p.next().text, Type: typ}
		if p.accept("=") {
			spec.Init = p.expr()
		}
		spec.S = p.span(ss)
		d.Specs = append(d.Specs, spec)
		if !p.accept(",") {
			break
		}
	}
	if len(d.Specs) == 0 {
		return p.badSkip("anggota kelas tidak bisa dibaca")
	}
	p.semi()
	d.S = p.span(start)
	return d
}
