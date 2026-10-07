// Package python mem-parse kode Python 3 menjadi IR (package ir). Toleran terhadap kode
// yang sedang diketik: kesalahan dicatat sebagai Diag lalu parse dilanjutkan.
package python

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
	tName
	tNum
	tStr // string biasa; val sudah di-decode
	tFStr
	tOp
	tNewline
	tIndent
	tDedent
)

type token struct {
	k     tokKind
	text  string
	val   string
	pos   ir.Pos
	end   ir.Pos
	parts []string // f-string
	exprs [][2]int // f-string: rentang offset ekspresi di sumber
	specs []string // f-string: "!r:.2f"
	selfs []bool   // f-string: {x=}
}

var ops = []string{
	"**=", "//=", ">>=", "<<=", "...", "->", ":=", "**", "//", "<<", ">>", "<=", ">=", "==", "!=", "<>",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "@=",
	"(", ")", "[", "]", "{", "}", ",", ":", ".", ";", "@", "=", "+", "-", "*", "/", "%", "&", "|", "^",
	"~", "<", ">", "!",
}

type lexer struct {
	src     string
	off     int
	line    int
	col     int
	toks    []token
	diags   []ir.Diag
	indents []int
	depth   int // kedalaman kurung: di dalamnya baris baru diabaikan
}

func lex(src string) ([]token, []ir.Diag) {
	lx := &lexer{src: src, line: 1, col: 1, indents: []int{0}}
	lx.run()
	return lx.toks, lx.diags
}

func (lx *lexer) pos() ir.Pos { return ir.Pos{Off: lx.off, Line: lx.line, Col: lx.col} }

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

func (lx *lexer) emit(k tokKind, start ir.Pos) *token {
	lx.toks = append(lx.toks, token{k: k, text: lx.src[start.Off:lx.off], pos: start, end: lx.pos()})
	return &lx.toks[len(lx.toks)-1]
}

func (lx *lexer) lastIsNewline() bool {
	if len(lx.toks) == 0 {
		return true
	}
	k := lx.toks[len(lx.toks)-1].k
	return k == tNewline || k == tIndent || k == tDedent
}

func (lx *lexer) run() {
	atLineStart := true
	for lx.off < len(lx.src) {
		if atLineStart && lx.depth == 0 {
			atLineStart = false
			// hitung indentasi; baris kosong / komentar saja tidak berarti
			col := 0
			i := lx.off
			for i < len(lx.src) && (lx.src[i] == ' ' || lx.src[i] == '\t' || lx.src[i] == '\f') {
				if lx.src[i] == '\t' {
					col = (col/8 + 1) * 8
				} else if lx.src[i] == ' ' {
					col++
				}
				i++
			}
			if i >= len(lx.src) || lx.src[i] == '\n' || lx.src[i] == '\r' || lx.src[i] == '#' {
				lx.advance(i - lx.off)
				lx.skipComment()
				if lx.off < len(lx.src) {
					lx.advance(1) // '\n' atau '\r'
					if lx.peek(-1) == '\r' && lx.peek(0) == '\n' {
						lx.advance(1)
					}
				}
				atLineStart = true
				continue
			}
			lx.advance(i - lx.off)
			lx.indentTo(col)
		}
		c := lx.src[lx.off]
		switch {
		case c == '\n' || c == '\r':
			if lx.depth == 0 {
				start := lx.pos()
				if !lx.lastIsNewline() {
					lx.emit(tNewline, start)
				}
				atLineStart = true
			}
			lx.advance(1)
			if c == '\r' && lx.peek(0) == '\n' {
				lx.advance(1)
			}
		case c == ' ' || c == '\t' || c == '\f':
			lx.advance(1)
		case c == '#':
			lx.skipComment()
		case c == '\\' && (lx.peek(1) == '\n' || lx.peek(1) == '\r'):
			lx.advance(2) // sambungan baris
			if lx.peek(-1) == '\r' && lx.peek(0) == '\n' {
				lx.advance(1)
			}
		case isNameStart(c) || c >= utf8.RuneSelf && letterAt(lx.src[lx.off:]):
			if lx.stringPrefixAhead() {
				lx.str()
				continue
			}
			start := lx.pos()
			lx.name()
			lx.emit(tName, start)
		case c >= '0' && c <= '9' || c == '.' && lx.peek(1) >= '0' && lx.peek(1) <= '9':
			start := lx.pos()
			lx.number()
			lx.emit(tNum, start)
		case c == '"' || c == '\'':
			lx.str()
		default:
			lx.op()
		}
	}
	if !lx.lastIsNewline() {
		e := lx.lastEnd()
		lx.toks = append(lx.toks, token{k: tNewline, pos: e, end: e})
	}
	end := lx.lastEnd()
	for len(lx.indents) > 1 {
		lx.indents = lx.indents[:len(lx.indents)-1]
		lx.toks = append(lx.toks, token{k: tDedent, pos: end, end: end})
	}
	lx.toks = append(lx.toks, token{k: tEOF, pos: end, end: end})
}

func (lx *lexer) skipComment() {
	if lx.peek(0) != '#' {
		return
	}
	for lx.off < len(lx.src) && lx.src[lx.off] != '\n' && lx.src[lx.off] != '\r' {
		lx.advance(1)
	}
}

// lastEnd: akhir token nyata terakhir. INDENT/DEDENT memakai posisi ini supaya rentang
// blok berhenti di baris terakhirnya, bukan di baris statement berikutnya.
func (lx *lexer) lastEnd() ir.Pos {
	if len(lx.toks) == 0 {
		return lx.pos()
	}
	return lx.toks[len(lx.toks)-1].end
}

func (lx *lexer) indentTo(col int) {
	p := lx.lastEnd()
	top := lx.indents[len(lx.indents)-1]
	switch {
	case col > top:
		lx.indents = append(lx.indents, col)
		lx.toks = append(lx.toks, token{k: tIndent, pos: p, end: p})
	case col < top:
		for len(lx.indents) > 1 && col < lx.indents[len(lx.indents)-1] {
			lx.indents = lx.indents[:len(lx.indents)-1]
			lx.toks = append(lx.toks, token{k: tDedent, pos: p, end: p})
		}
		if col != lx.indents[len(lx.indents)-1] {
			lx.errf(lx.pos(), "indentasi tidak cocok dengan baris-baris sebelumnya")
			lx.indents = append(lx.indents, col)
			lx.toks = append(lx.toks, token{k: tIndent, pos: p, end: p})
		}
	}
}

// letterAt: rune pertama s adalah huruf (UTF-8 valid). Byte tidak valid bukan awal nama.
func letterAt(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return r != utf8.RuneError && unicode.IsLetter(r)
}

func isNameStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func (lx *lexer) name() {
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		if isNameStart(c) || c >= '0' && c <= '9' {
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
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' || c == '_' }
	if s[lx.off] == '0' && lx.off+1 < len(s) && strings.IndexByte("xXoObB", s[lx.off+1]) >= 0 {
		lx.advance(2)
		for lx.off < len(s) && (isDigit(s[lx.off]) || strings.IndexByte("abcdefABCDEF", s[lx.off]) >= 0) {
			lx.advance(1)
		}
		return
	}
	for lx.off < len(s) && isDigit(s[lx.off]) {
		lx.advance(1)
	}
	if lx.peek(0) == '.' {
		lx.advance(1)
		for lx.off < len(s) && isDigit(s[lx.off]) {
			lx.advance(1)
		}
	}
	if c := lx.peek(0); c == 'e' || c == 'E' {
		k := 1
		if lx.peek(1) == '+' || lx.peek(1) == '-' {
			k = 2
		}
		if d := lx.peek(k); d >= '0' && d <= '9' {
			lx.advance(k)
			for lx.off < len(s) && isDigit(s[lx.off]) {
				lx.advance(1)
			}
		}
	}
	if c := lx.peek(0); c == 'j' || c == 'J' {
		lx.advance(1)
	}
}

func (lx *lexer) stringPrefixAhead() bool {
	for n := 1; n <= 2; n++ {
		if lx.off+n >= len(lx.src) {
			return false
		}
		p := strings.ToLower(lx.src[lx.off : lx.off+n])
		if strings.Trim(p, "rbuf") != "" {
			return false
		}
		if q := lx.src[lx.off+n]; q == '"' || q == '\'' {
			return true
		}
	}
	return false
}

// str membaca string dengan prefix opsional (r, b, u, f, kombinasinya) dan kutip
// tunggal/ganda/tiga.
func (lx *lexer) str() {
	start := lx.pos()
	prefix := ""
	for lx.peek(0) != '"' && lx.peek(0) != '\'' {
		prefix += strings.ToLower(string(lx.src[lx.off]))
		lx.advance(1)
	}
	raw := strings.Contains(prefix, "r")
	fmode := strings.Contains(prefix, "f")
	q := lx.src[lx.off]
	quote := string(q)
	if lx.peek(1) == q && lx.peek(2) == q {
		quote = strings.Repeat(string(q), 3)
	}
	lx.advance(len(quote))
	var b strings.Builder
	var parts, specs []string
	var exprs [][2]int
	var selfs []bool
	closed := false
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		if strings.HasPrefix(lx.src[lx.off:], quote) {
			lx.advance(len(quote))
			closed = true
			break
		}
		if (c == '\n' || c == '\r') && len(quote) == 1 {
			break
		}
		if c == '\\' && lx.off+1 < len(lx.src) {
			if raw {
				b.WriteByte('\\')
				b.WriteByte(lx.src[lx.off+1])
				lx.advance(2)
				continue
			}
			lx.escape(&b)
			continue
		}
		if fmode && c == '{' {
			if lx.peek(1) == '{' {
				b.WriteByte('{')
				lx.advance(2)
				continue
			}
			parts = append(parts, b.String())
			b.Reset()
			r, spec, self := lx.fexpr(quote)
			exprs = append(exprs, r)
			specs = append(specs, spec)
			selfs = append(selfs, self)
			continue
		}
		if fmode && c == '}' && lx.peek(1) == '}' {
			b.WriteByte('}')
			lx.advance(2)
			continue
		}
		b.WriteByte(c)
		lx.advance(1)
	}
	if !closed {
		lx.errf(start, "string belum ditutup")
	}
	if fmode {
		t := lx.emit(tFStr, start)
		t.parts = append(parts, b.String())
		t.exprs, t.specs, t.selfs = exprs, specs, selfs
		return
	}
	t := lx.emit(tStr, start)
	t.val = b.String()
}

// fexpr membaca {ekspresi!konversi:format} di f-string. Mengembalikan rentang ekspresi,
// "!r:.2f" (konversi + format), dan apakah bentuknya {x=}.
func (lx *lexer) fexpr(quote string) ([2]int, string, bool) {
	lx.advance(1) // {
	s := lx.off
	depth := 0
	exprEnd := -1
	specStart := -1
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		if strings.HasPrefix(lx.src[lx.off:], quote) && len(quote) == 3 || c == '\n' && len(quote) == 1 {
			break
		}
		switch {
		case c == '(' || c == '[' || c == '{':
			depth++
		case (c == ')' || c == ']') && depth > 0:
			depth--
		case c == '}' && depth > 0:
			depth--
		case c == '}':
			if exprEnd < 0 {
				exprEnd = lx.off
			}
			spec := ""
			if specStart >= 0 {
				spec = lx.src[specStart:lx.off]
			}
			lx.advance(1)
			text := lx.src[s:exprEnd]
			self := false
			trim := strings.TrimRight(text, " ")
			if strings.HasSuffix(trim, "=") && !strings.HasSuffix(trim, "==") && !strings.HasSuffix(trim, "!=") &&
				!strings.HasSuffix(trim, "<=") && !strings.HasSuffix(trim, ">=") {
				self = true
				exprEnd = s + len(trim) - 1
			}
			return [2]int{s, exprEnd}, spec, self
		case c == '!' && depth == 0 && lx.peek(1) != '=' && exprEnd < 0:
			exprEnd = lx.off
			specStart = lx.off
		case c == ':' && depth == 0 && specStart < 0:
			exprEnd = lx.off
			specStart = lx.off
		case c == '\'' || c == '"':
			// string di dalam ekspresi
			q := c
			lx.advance(1)
			for lx.off < len(lx.src) && lx.src[lx.off] != q && lx.src[lx.off] != '\n' {
				lx.advance(1)
			}
		}
		lx.advance(1)
	}
	lx.errf(lx.pos(), "kurung { di f-string belum ditutup")
	if exprEnd < 0 {
		exprEnd = lx.off
	}
	return [2]int{s, exprEnd}, "", false
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
		b.WriteByte(0)
	case 'a':
		b.WriteByte('\a')
	case 'b':
		b.WriteByte('\b')
	case 'f':
		b.WriteByte('\f')
	case 'v':
		b.WriteByte('\v')
	case '\\', '\'', '"':
		b.WriteByte(n)
	case '\n':
	case 'x', 'u', 'U':
		size := map[byte]int{'x': 2, 'u': 4, 'U': 8}[n]
		if lx.off+2+size <= len(lx.src) {
			if v, err := strconv.ParseUint(lx.src[lx.off+2:lx.off+2+size], 16, 32); err == nil {
				b.WriteRune(rune(v))
				lx.advance(2 + size)
				return
			}
		}
		b.WriteByte('\\')
		b.WriteByte(n)
	default:
		b.WriteByte('\\')
		b.WriteByte(n)
	}
	lx.advance(2)
}

func (lx *lexer) op() {
	start := lx.pos()
	rest := lx.src[lx.off:]
	for _, o := range ops {
		if strings.HasPrefix(rest, o) {
			lx.advance(len(o))
			switch o {
			case "(", "[", "{":
				lx.depth++
			case ")", "]", "}":
				if lx.depth > 0 {
					lx.depth--
				}
			}
			lx.emit(tOp, start)
			return
		}
	}
	r, size := utf8.DecodeRuneInString(rest)
	lx.errf(start, "karakter tidak dikenal: "+strconv.QuoteRune(r))
	lx.advance(size)
}
