// Package clike mem-parse bahasa keluarga C (JavaScript, TypeScript, PHP, C++, C, Java)
// menjadi IR (package ir). Satu lexer dan satu parser dipakai bersama; perbedaan antar
// bahasa diatur oleh Dialect. Parser toleran terhadap kode yang sedang diketik: kesalahan
// dicatat sebagai Diag lalu parse dilanjutkan.
package clike

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopherviz/internal/ir"
)

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tNum
	tStr      // string tanpa interpolasi; Val sudah di-decode
	tChar     // 'a' di C/C++/Java
	tTemplate // `..${x}..` (JS) atau "..$x.." (PHP): Parts + Exprs
	tRegex
	tPunct
	tDirective // baris preprosesor C/C++ (#include, #define)
	tInline    // teks di luar <?php ?> (PHP)
)

type token struct {
	k     tokKind
	text  string // teks sumber
	val   string // nilai string yang sudah di-decode
	pos   ir.Pos
	end   ir.Pos
	nl    bool // ada baris baru sebelum token ini (untuk ASI JavaScript)
	parts []string
	exprs [][2]int // rentang offset sumber untuk ekspresi interpolasi
}

// Dialect mengatur perbedaan sintaks antar bahasa keluarga C.
type Dialect struct {
	Lang     string
	Template bool // template literal `...`
	Regex    bool // literal /re/
	HashCmt  bool // komentar # (PHP)
	Dollar   bool // variabel $x (PHP)
	PHPTags  bool // kode di dalam <?php ... ?>
	Preproc  bool // baris # preprosesor (C/C++)
	CharLit  bool // 'a' adalah karakter, bukan string
	RawStr   bool // R"(...)" C++
	TextBlk  bool // """...""" Java
	Types    bool // deklarasi diawali tipe (C/C++/Java)
	TSTypes  bool // anotasi tipe TypeScript
	ASI      bool // titik koma otomatis (JavaScript/TypeScript)
}

var dialects = map[string]*Dialect{
	"javascript": {Lang: "javascript", Template: true, Regex: true, ASI: true},
	"typescript": {Lang: "typescript", Template: true, Regex: true, ASI: true, TSTypes: true},
	"php":        {Lang: "php", HashCmt: true, Dollar: true, PHPTags: true},
	"cpp":        {Lang: "cpp", Preproc: true, CharLit: true, RawStr: true, Types: true},
	"c":          {Lang: "c", Preproc: true, CharLit: true, Types: true},
	"java":       {Lang: "java", CharLit: true, TextBlk: true, Types: true},
}

// DialectFor mengembalikan dialek untuk bahasa lang, atau nil bila tidak dikenal.
func DialectFor(lang string) *Dialect { return dialects[lang] }

// puncts diurutkan dari yang terpanjang supaya pencocokan selalu mengambil token terpanjang.
var puncts = []string{
	">>>=", "...", "===", "!==", "**=", "<<=", ">>=", ">>>", "??=", "&&=", "||=", "<=>", "?->",
	"::", "->", "=>", "?.", "??", "**", "++", "--", "<<", ">>", "<=", ">=", "==", "!=", "&&", "||",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", ".=",
	"{", "}", "(", ")", "[", "]", ";", ",", ".", "<", ">", "+", "-", "*", "/", "%", "&", "|", "^",
	"!", "~", "?", ":", "=", "@", "\\", "$", "#",
}

type lexer struct {
	d     *Dialect
	src   string
	off   int
	line  int
	col   int
	toks  []token
	diags []ir.Diag
	inPHP bool
	nl    bool
}

func lex(d *Dialect, src string, base ir.Pos) ([]token, []ir.Diag) {
	lx := &lexer{d: d, src: src, off: 0, line: base.Line, col: base.Col}
	if base.Line == 0 {
		lx.line, lx.col = 1, 1
	}
	lx.inPHP = !d.PHPTags || base.Line != 0 // potongan (interpolasi) selalu dianggap kode
	lx.run(base.Off)
	return lx.toks, lx.diags
}

func (lx *lexer) pos(base int) ir.Pos { return ir.Pos{Off: base + lx.off, Line: lx.line, Col: lx.col} }

func (lx *lexer) advance(n int) {
	for i := 0; i < n && lx.off < len(lx.src); i++ {
		if lx.src[lx.off] == '\n' {
			lx.line++
			lx.col = 1
		} else {
			lx.col++
		}
		lx.off++
	}
}

func (lx *lexer) peek(k int) byte {
	if lx.off+k < len(lx.src) {
		return lx.src[lx.off+k]
	}
	return 0
}

func (lx *lexer) errf(p ir.Pos, msg string) { lx.diags = append(lx.diags, ir.Diag{Pos: p, Msg: msg}) }

func (lx *lexer) emit(k tokKind, start ir.Pos, base int) *token {
	t := token{k: k, text: lx.src[start.Off-base : lx.off], pos: start, end: lx.pos(base), nl: lx.nl}
	lx.nl = false
	lx.toks = append(lx.toks, t)
	return &lx.toks[len(lx.toks)-1]
}

func (lx *lexer) run(base int) {
	for lx.off < len(lx.src) {
		if !lx.inPHP {
			lx.phpOutside(base)
			continue
		}
		c := lx.src[lx.off]
		switch {
		case c == '\n':
			lx.nl = true
			lx.advance(1)
			continue
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			lx.advance(1)
			continue
		case c == '/' && lx.peek(1) == '/':
			lx.skipLine()
			continue
		case c == '/' && lx.peek(1) == '*':
			start := lx.pos(base)
			end := strings.Index(lx.src[lx.off+2:], "*/")
			if end < 0 {
				lx.errf(start, "comment /* is not closed with */")
				lx.advance(len(lx.src) - lx.off)
			} else {
				lx.advance(end + 4)
			}
			continue
		case c == '#' && lx.d.Template && isIdentStart(lx.peek(1)):
			start := lx.pos(base)
			lx.advance(1)
			lx.ident()
			lx.emit(tIdent, start, base) // nama privat JavaScript: #count
			continue
		case c == '#' && lx.d.HashCmt:
			lx.skipLine()
			continue
		case c == '#' && lx.d.Preproc && lx.atLineStart():
			start := lx.pos(base)
			for lx.off < len(lx.src) && lx.src[lx.off] != '\n' {
				if lx.src[lx.off] == '\\' && lx.peek(1) == '\n' {
					lx.advance(2)
					continue
				}
				lx.advance(1)
			}
			lx.emit(tDirective, start, base)
			continue
		case c == '?' && lx.peek(1) == '>' && lx.d.PHPTags:
			start := lx.pos(base)
			lx.advance(2)
			// ?> berlaku seperti titik koma
			t := lx.emit(tPunct, start, base)
			t.text = ";"
			lx.inPHP = false
			if lx.off < len(lx.src) && lx.src[lx.off] == '\n' {
				lx.advance(1)
			}
			continue
		}
		start := lx.pos(base)
		switch {
		case isIdentStart(c) || c >= utf8.RuneSelf && letterAt(lx.src[lx.off:]):
			if lx.d.RawStr && lx.rawStringAhead() {
				lx.rawString(start, base)
				continue
			}
			lx.ident()
			t := lx.emit(tIdent, start, base)
			// prefix string C++: L"..", u8"..", u'..'
			if (lx.d.CharLit) && (t.text == "L" || t.text == "u" || t.text == "U" || t.text == "u8") && (lx.peek(0) == '"' || lx.peek(0) == '\'') {
				lx.toks = lx.toks[:len(lx.toks)-1]
				lx.str(start, base, lx.peek(0))
			}
		case c == '$' && lx.d.Dollar && (isIdentStart(lx.peek(1)) || lx.peek(1) >= utf8.RuneSelf && letterAt(lx.src[lx.off+1:])):
			lx.advance(1)
			lx.ident()
			lx.emit(tIdent, start, base)
		case c >= '0' && c <= '9' || c == '.' && lx.peek(1) >= '0' && lx.peek(1) <= '9':
			lx.number()
			lx.emit(tNum, start, base)
		case c == '"' && lx.d.TextBlk && strings.HasPrefix(lx.src[lx.off:], `"""`):
			lx.textBlock(start, base)
		case c == '"' || c == '\'':
			lx.str(start, base, c)
		case c == '`' && lx.d.Template:
			lx.template(start, base)
		case c == '<' && lx.d.Dollar && strings.HasPrefix(lx.src[lx.off:], "<<<"):
			lx.heredoc(start, base)
		case c == '/' && lx.d.Regex && lx.regexAllowed():
			lx.regex(start, base)
		default:
			lx.punct(start, base)
		}
	}
	lx.toks = append(lx.toks, token{k: tEOF, pos: lx.pos(base), end: lx.pos(base), nl: true})
}

func (lx *lexer) atLineStart() bool {
	for i := lx.off - 1; i >= 0; i-- {
		switch lx.src[i] {
		case ' ', '\t':
			continue
		case '\n':
			return true
		default:
			return false
		}
	}
	return true
}

func (lx *lexer) skipLine() {
	for lx.off < len(lx.src) && lx.src[lx.off] != '\n' {
		if lx.d.PHPTags && lx.src[lx.off] == '?' && lx.peek(1) == '>' {
			return // ?> mengakhiri komentar satu baris di PHP
		}
		lx.advance(1)
	}
}

// phpOutside memproses teks di luar <?php ?>: menjadi token tInline (dicetak apa adanya).
func (lx *lexer) phpOutside(base int) {
	start := lx.pos(base)
	i := strings.Index(lx.src[lx.off:], "<?")
	if i < 0 {
		i = len(lx.src) - lx.off
	}
	if i > 0 {
		lx.advance(i)
		t := lx.emit(tInline, start, base)
		t.val = t.text
		if strings.TrimSpace(t.val) == "" {
			lx.toks = lx.toks[:len(lx.toks)-1]
		}
	}
	if lx.off >= len(lx.src) {
		return
	}
	switch {
	case strings.HasPrefix(lx.src[lx.off:], "<?php"):
		lx.advance(5)
	case strings.HasPrefix(lx.src[lx.off:], "<?="):
		p := lx.pos(base)
		lx.advance(3)
		t := lx.emit(tIdent, p, base)
		t.text = "echo"
	default:
		lx.advance(2)
	}
	lx.inPHP = true
}

// letterAt: rune pertama s adalah huruf (UTF-8 valid). Byte tidak valid bukan awal nama.
func letterAt(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return r != utf8.RuneError && (unicode.IsLetter(r) || r == '_')
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '$' && false
}

func isIdentPart(c byte) bool { return isIdentStart(c) || c >= '0' && c <= '9' }

func (lx *lexer) ident() {
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		if isIdentPart(c) || (c == '$' && lx.d.Template) {
			lx.advance(1)
			continue
		}
		if c >= utf8.RuneSelf {
			r, size := utf8.DecodeRuneInString(lx.src[lx.off:])
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				lx.advance(size)
				continue
			}
		}
		return
	}
}

func (lx *lexer) number() {
	s := lx.src
	if s[lx.off] == '0' && lx.off+1 < len(s) && strings.IndexByte("xXbBoO", s[lx.off+1]) >= 0 {
		lx.advance(2)
		for lx.off < len(s) && (isHex(s[lx.off]) || s[lx.off] == '_' || s[lx.off] == '\'') {
			lx.advance(1)
		}
	} else {
		for lx.off < len(s) && (s[lx.off] >= '0' && s[lx.off] <= '9' || s[lx.off] == '_' || (s[lx.off] == '\'' && lx.d.CharLit && lx.peek(1) >= '0' && lx.peek(1) <= '9')) {
			lx.advance(1)
		}
		if lx.off < len(s) && s[lx.off] == '.' && !(lx.peek(1) == '.') {
			lx.advance(1)
			for lx.off < len(s) && (s[lx.off] >= '0' && s[lx.off] <= '9' || s[lx.off] == '_') {
				lx.advance(1)
			}
		}
		if lx.off < len(s) && (s[lx.off] == 'e' || s[lx.off] == 'E') {
			k := 1
			if lx.peek(1) == '+' || lx.peek(1) == '-' {
				k = 2
			}
			if c := lx.peek(k); c >= '0' && c <= '9' {
				lx.advance(k)
				for lx.off < len(s) && s[lx.off] >= '0' && s[lx.off] <= '9' {
					lx.advance(1)
				}
			}
		}
	}
	// sufiks: 10L, 1.5f, 10u, 10n (BigInt)
	for lx.off < len(s) && strings.IndexByte("lLfFuUnmMdD", s[lx.off]) >= 0 {
		lx.advance(1)
	}
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// str membaca string "..." atau '...'. Di PHP, string kutip ganda bisa berinterpolasi.
func (lx *lexer) str(start ir.Pos, base int, q byte) {
	lx.advance(1)
	var b strings.Builder
	var parts []string
	var exprs [][2]int
	interp := lx.d.Dollar && q == '"'
	closed := false
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		if c == q {
			lx.advance(1)
			closed = true
			break
		}
		if c == '\n' && !lx.d.Dollar {
			break // string biasa tidak boleh melewati baris (kecuali PHP)
		}
		if c == '\\' && lx.off+1 < len(lx.src) {
			if lx.d.Dollar && q == '\'' {
				// PHP kutip tunggal: hanya \\ dan \'
				if n := lx.peek(1); n == '\\' || n == '\'' {
					b.WriteByte(n)
					lx.advance(2)
				} else {
					b.WriteByte('\\')
					lx.advance(1)
				}
				continue
			}
			lx.escape(&b)
			continue
		}
		if interp && (c == '$' && (isIdentStart(lx.peek(1)) || lx.peek(1) >= utf8.RuneSelf) || c == '{' && lx.peek(1) == '$') {
			parts = append(parts, b.String())
			b.Reset()
			exprs = append(exprs, lx.phpInterp(base))
			continue
		}
		b.WriteByte(c)
		lx.advance(1)
	}
	if !closed {
		lx.errf(start, "string is not closed")
	}
	k := tStr
	if q == '\'' && lx.d.CharLit {
		k = tChar
	}
	if len(exprs) > 0 {
		k = tTemplate
	}
	t := lx.emit(k, start, base)
	if k == tTemplate {
		t.parts = append(parts, b.String())
		t.exprs = exprs
	} else {
		t.val = b.String()
	}
}

// phpInterp membaca $nama, $nama[...], $nama->prop, atau {$ekspresi} di dalam string PHP
// dan mengembalikan rentang offset ekspresinya.
func (lx *lexer) phpInterp(base int) [2]int {
	if lx.src[lx.off] == '{' {
		lx.advance(1)
		s := lx.off
		depth := 1
		for lx.off < len(lx.src) {
			c := lx.src[lx.off]
			if c == '{' {
				depth++
			} else if c == '}' {
				depth--
				if depth == 0 {
					e := lx.off
					lx.advance(1)
					return [2]int{base + s, base + e}
				}
			}
			lx.advance(1)
		}
		return [2]int{base + s, base + lx.off}
	}
	s := lx.off
	lx.advance(1)
	lx.ident()
	switch {
	case lx.peek(0) == '[':
		for lx.off < len(lx.src) && lx.src[lx.off] != ']' && lx.src[lx.off] != '"' {
			lx.advance(1)
		}
		if lx.peek(0) == ']' {
			lx.advance(1)
		}
	case lx.peek(0) == '-' && lx.peek(1) == '>' && isIdentStart(lx.peek(2)):
		lx.advance(2)
		lx.ident()
	}
	return [2]int{base + s, base + lx.off}
}

func (lx *lexer) escape(b *strings.Builder) {
	n := lx.peek(1)
	switch n {
	case 'n':
		b.WriteByte('\n')
	case 't':
		b.WriteByte('\t')
	case 'r':
		b.WriteByte('\r')
	case '0':
		if c := lx.peek(2); c >= '0' && c <= '7' {
			break
		}
		b.WriteByte(0)
	case 'b':
		b.WriteByte('\b')
	case 'f':
		b.WriteByte('\f')
	case 'v':
		b.WriteByte('\v')
	case 'a':
		b.WriteByte('\a')
	case 'e':
		b.WriteByte(27)
	case '\n':
		// sambungan baris: tidak menghasilkan apa-apa
	case 'x':
		end := 2
		for end < 4 && isHex(lx.peek(end)) {
			end++
		}
		if v, err := strconv.ParseUint(lx.src[lx.off+2:lx.off+end], 16, 8); err == nil && end > 2 {
			b.WriteByte(byte(v))
			lx.advance(end)
			return
		}
		b.WriteByte('x')
	case 'u':
		if lx.peek(2) == '{' {
			end := strings.IndexByte(lx.src[lx.off:], '}')
			if end > 3 {
				if v, err := strconv.ParseUint(lx.src[lx.off+3:lx.off+end], 16, 32); err == nil {
					b.WriteRune(rune(v))
					lx.advance(end + 1)
					return
				}
			}
		} else if lx.off+6 <= len(lx.src) {
			if v, err := strconv.ParseUint(lx.src[lx.off+2:lx.off+6], 16, 32); err == nil {
				b.WriteRune(rune(v))
				lx.advance(6)
				return
			}
		}
		b.WriteByte('u')
	case '$':
		b.WriteByte('$')
	default:
		if lx.d.Dollar && n != '"' && n != '\\' && n != '\'' {
			b.WriteByte('\\') // PHP: escape yang tidak dikenal tetap ditulis apa adanya
		}
		b.WriteByte(n)
	}
	lx.advance(2)
}

// template membaca `...${expr}...` (JavaScript/TypeScript).
func (lx *lexer) template(start ir.Pos, base int) {
	lx.advance(1)
	var b strings.Builder
	var parts []string
	var exprs [][2]int
	closed := false
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		if c == '`' {
			lx.advance(1)
			closed = true
			break
		}
		if c == '\\' && lx.off+1 < len(lx.src) {
			lx.escape(&b)
			continue
		}
		if c == '$' && lx.peek(1) == '{' {
			lx.advance(2)
			s := lx.off
			lx.skipBalanced('}')
			parts = append(parts, b.String())
			b.Reset()
			exprs = append(exprs, [2]int{base + s, base + lx.off})
			if lx.peek(0) == '}' {
				lx.advance(1)
			}
			continue
		}
		b.WriteByte(c)
		lx.advance(1)
	}
	if !closed {
		lx.errf(start, "template string is not closed with `")
	}
	t := lx.emit(tTemplate, start, base)
	t.parts = append(parts, b.String())
	t.exprs = exprs
}

// skipBalanced maju sampai kurung penutup close pada kedalaman 0, melewati string dan template di dalamnya.
func (lx *lexer) skipBalanced(close byte) {
	depth := 0
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		switch c {
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			if depth == 0 && c == close {
				return
			}
			depth--
		case '"', '\'':
			q := c
			lx.advance(1)
			for lx.off < len(lx.src) && lx.src[lx.off] != q && lx.src[lx.off] != '\n' {
				if lx.src[lx.off] == '\\' {
					lx.advance(1)
				}
				lx.advance(1)
			}
		case '`':
			lx.advance(1)
			for lx.off < len(lx.src) && lx.src[lx.off] != '`' {
				if lx.src[lx.off] == '\\' {
					lx.advance(1)
				} else if lx.src[lx.off] == '$' && lx.peek(1) == '{' {
					lx.advance(2)
					lx.skipBalanced('}')
				}
				lx.advance(1)
			}
		}
		lx.advance(1)
	}
}

func (lx *lexer) textBlock(start ir.Pos, base int) {
	lx.advance(3)
	end := strings.Index(lx.src[lx.off:], `"""`)
	body := ""
	if end < 0 {
		lx.errf(start, `text block is not closed with """`)
		body = lx.src[lx.off:]
		lx.advance(len(lx.src) - lx.off)
	} else {
		body = lx.src[lx.off : lx.off+end]
		lx.advance(end + 3)
	}
	t := lx.emit(tStr, start, base)
	t.val = dedentBlock(strings.TrimPrefix(strings.TrimPrefix(body, "\r"), "\n"))
}

// dedentBlock membuang indentasi bersama dari text block Java / heredoc PHP.
func dedentBlock(s string) string {
	lines := strings.Split(s, "\n")
	minIndent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if minIndent < 0 || n < minIndent {
			minIndent = n
		}
	}
	// baris penutup (hanya spasi sebelum """) juga menentukan indentasi
	if last := lines[len(lines)-1]; strings.TrimSpace(last) == "" && (minIndent < 0 || len(last) < minIndent) {
		minIndent = len(last)
	}
	for i, l := range lines {
		if len(l) >= minIndent && minIndent > 0 {
			lines[i] = l[minIndent:]
		} else {
			lines[i] = strings.TrimLeft(l, " \t")
		}
	}
	return strings.Join(lines, "\n")
}

func (lx *lexer) rawStringAhead() bool {
	s := lx.src[lx.off:]
	for _, p := range []string{"R\"", "u8R\"", "LR\"", "uR\"", "UR\""} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func (lx *lexer) rawString(start ir.Pos, base int) {
	q := strings.IndexByte(lx.src[lx.off:], '"')
	lx.advance(q + 1)
	open := strings.IndexByte(lx.src[lx.off:], '(')
	if open < 0 {
		lx.errf(start, "invalid raw string")
		lx.advance(len(lx.src) - lx.off)
		lx.emit(tStr, start, base)
		return
	}
	delim := lx.src[lx.off : lx.off+open]
	lx.advance(open + 1)
	closing := ")" + delim + "\""
	end := strings.Index(lx.src[lx.off:], closing)
	body := ""
	if end < 0 {
		lx.errf(start, "raw string is not closed")
		body = lx.src[lx.off:]
		lx.advance(len(lx.src) - lx.off)
	} else {
		body = lx.src[lx.off : lx.off+end]
		lx.advance(end + len(closing))
	}
	t := lx.emit(tStr, start, base)
	t.val = body
}

// heredoc membaca <<<EOT ... EOT (berinterpolasi) atau <<<'EOT' ... EOT (nowdoc) di PHP.
func (lx *lexer) heredoc(start ir.Pos, base int) {
	lx.advance(3)
	for lx.peek(0) == ' ' {
		lx.advance(1)
	}
	quoted := lx.peek(0) == '\'' || lx.peek(0) == '"'
	nowdoc := lx.peek(0) == '\''
	if quoted {
		lx.advance(1)
	}
	s := lx.off
	lx.ident()
	label := lx.src[s:lx.off]
	if quoted {
		lx.advance(1)
	}
	if label == "" {
		lx.errf(start, "heredoc without a label")
		lx.emit(tPunct, start, base)
		return
	}
	// badan dimulai di baris berikutnya dan berakhir pada baris yang hanya berisi label
	nl := strings.IndexByte(lx.src[lx.off:], '\n')
	if nl < 0 {
		lx.errf(start, "heredoc is not closed")
		lx.advance(len(lx.src) - lx.off)
		lx.emit(tStr, start, base)
		return
	}
	lx.advance(nl + 1)
	bodyStart := lx.off
	bodyEnd := -1
	for i := lx.off; i <= len(lx.src); {
		j := strings.IndexByte(lx.src[i:], '\n')
		lineEnd := len(lx.src)
		if j >= 0 {
			lineEnd = i + j
		}
		if l := strings.TrimLeft(lx.src[i:lineEnd], " \t"); strings.HasPrefix(l, label) && (len(l) == len(label) || !isIdentPart(l[len(label)])) {
			bodyEnd = i
			lx.advance(i - lx.off + (len(lx.src[i:lineEnd]) - len(l)) + len(label))
			break
		}
		if j < 0 {
			break
		}
		i = lineEnd + 1
	}
	if bodyEnd < 0 {
		lx.errf(start, "heredoc is not closed with "+label)
		bodyEnd = len(lx.src)
		lx.advance(len(lx.src) - lx.off)
	}
	body := strings.TrimSuffix(lx.src[bodyStart:bodyEnd], "\n")
	t := lx.emit(tStr, start, base)
	t.val = dedentBlock(body)
	if !nowdoc && strings.ContainsAny(body, "$") {
		// heredoc berinterpolasi: lex ulang badannya sebagai string kutip ganda
		sub := &lexer{d: lx.d, src: `"` + strings.ReplaceAll(t.val, `"`, `\"`) + `"`, inPHP: true, line: start.Line, col: start.Col}
		sub.run(0)
		if len(sub.toks) > 0 && sub.toks[0].k == tTemplate {
			t.k = tTemplate
			t.parts = sub.toks[0].parts
			// rentang ekspresi relatif ke string buatan: simpan teksnya langsung lewat val
			t.val = sub.src
			t.exprs = sub.toks[0].exprs
			t.text = "<<<" // penanda: ekspresi dibaca dari val, bukan dari sumber
		}
	}
}

// regexAllowed: '/' memulai regex bila tidak bisa menjadi operator bagi.
func (lx *lexer) regexAllowed() bool {
	if len(lx.toks) == 0 {
		return true
	}
	t := lx.toks[len(lx.toks)-1]
	switch t.k {
	case tNum, tStr, tTemplate, tRegex, tChar:
		return false
	case tIdent:
		switch t.text {
		case "return", "typeof", "case", "in", "of", "new", "delete", "void", "throw", "else", "do", "instanceof", "yield", "await":
			return true
		}
		return false
	case tPunct:
		switch t.text {
		case ")", "]", "}", "++", "--":
			return false
		}
	}
	return true
}

func (lx *lexer) regex(start ir.Pos, base int) {
	lx.advance(1)
	inClass := false
	for lx.off < len(lx.src) && lx.src[lx.off] != '\n' {
		c := lx.src[lx.off]
		if c == '\\' {
			lx.advance(2)
			continue
		}
		if c == '[' {
			inClass = true
		} else if c == ']' {
			inClass = false
		} else if c == '/' && !inClass {
			break
		}
		lx.advance(1)
	}
	if lx.peek(0) != '/' {
		lx.errf(start, "regex is not closed with /")
	} else {
		lx.advance(1)
	}
	for lx.off < len(lx.src) && isIdentPart(lx.src[lx.off]) {
		lx.advance(1)
	}
	t := lx.emit(tRegex, start, base)
	t.val = t.text
}

func (lx *lexer) punct(start ir.Pos, base int) {
	rest := lx.src[lx.off:]
	for _, p := range puncts {
		if strings.HasPrefix(rest, p) {
			// "?." diikuti angka adalah ternary dengan bilangan desimal: a ? .5 : 1
			if p == "?." && len(rest) > 2 && rest[2] >= '0' && rest[2] <= '9' {
				continue
			}
			if p == ".=" && !lx.d.Dollar {
				continue
			}
			lx.advance(len(p))
			lx.emit(tPunct, start, base)
			return
		}
	}
	r, size := utf8.DecodeRuneInString(rest)
	lx.errf(start, "unknown character: "+strconv.QuoteRune(r))
	lx.advance(size)
}
