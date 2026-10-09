// Package analyzer menemukan dan mengklasifikasi loop di kode Go, termasuk kode
// yang sedang diketik dan belum valid. Hasilnya adalah Scene (lihat package scene).
package analyzer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"unicode/utf8"

	"gopherviz/internal/lang"
	"gopherviz/internal/scene"
)

// Result adalah hasil analisis satu file.
type Result struct {
	Scene       scene.Scene        `json:"scene"`
	Diagnostics []scene.Diagnostic `json:"diagnostics"`
}

// JSON mengubah r menjadi JSON berindentasi tanpa escape HTML, supaya kode
// seperti "i < 3" tetap terbaca (bukan "i < 3").
func (r Result) JSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Program adalah hasil parse + cek tipe satu file beserta Scene-nya. Dipakai
// bersama oleh analyzer dan simulator supaya keduanya melihat AST yang sama.
type Program struct {
	Fset       *token.FileSet
	File       *ast.File
	Info       *types.Info
	Result     Result
	LoopNodes  map[string]ast.Stmt // ID loop → *ast.ForStmt / *ast.RangeStmt
	LoopIDs    map[ast.Node]string
	BlockNodes map[string]ast.Node // ID blok apa pun → node (FuncDecl, FuncLit, IfStmt, SwitchStmt, loop)
	BlockIDs   map[ast.Node]string
	src        source

	// Bahasa selain Go: Lang diisi (javascript, python, ...) dan Other berisi IR + indeks
	// blok. Fset/File/Info tetap terisi kosong supaya kode lama aman.
	Lang  string
	Other *lang.Result

	// Siblings: file Go lain di package yang sama (lihat WithPackage); posisinya di Fset yang sama.
	Siblings []*Sibling
}

// Line mengembalikan nomor baris di teks asli (sudah dikoreksi bila potongan kode dibungkus).
func (p *Program) Line(pos token.Pos) int {
	if p.sibling(pos) != nil {
		return p.Fset.Position(pos).Line
	}
	return p.src.line(pos)
}

// Text mengembalikan sumber node dalam satu baris.
func (p *Program) Text(n ast.Node) string {
	if n == nil {
		return ""
	}
	if s := p.sibling(n.Pos()); s != nil {
		from, to := p.Fset.Position(n.Pos()).Offset, p.Fset.Position(n.End()).Offset
		from, to = max(0, from), min(len(s.text), to)
		if from >= to {
			return ""
		}
		return oneLine(string(s.text[from:to]))
	}
	return oneLine(p.src.slice(p.src.offset(n.Pos()), p.src.offset(n.End())))
}

// Analyze menganalisis src. Fungsi ini tidak pernah panik: kegagalan internal
// dilaporkan sebagai diagnostic supaya tampilan realtime tidak ikut mati.
func Analyze(filename string, src []byte) Result {
	return Load(filename, src).Result
}

// Load seperti Analyze, tetapi juga mengembalikan AST untuk simulator.
func Load(filename string, src []byte) (p *Program) {
	defer func() {
		if r := recover(); r != nil {
			p = &Program{
				Fset: token.NewFileSet(), File: &ast.File{Name: ast.NewIdent("")}, Info: &types.Info{},
				LoopNodes: map[string]ast.Stmt{}, LoopIDs: map[ast.Node]string{},
				BlockNodes: map[string]ast.Node{}, BlockIDs: map[ast.Node]string{},
				Result: Result{
					Scene: scene.Scene{Version: scene.Version, Language: "go", File: filename,
						Blocks: []scene.Block{}, Loops: []scene.Loop{}},
					Diagnostics: []scene.Diagnostic{{Message: fmt.Sprintf("kesalahan internal analyzer: %v", r)}},
				},
			}
		}
	}()
	if l := lang.Detect(filename); l != "" && l != "go" {
		return loadOther(filename, src, l)
	}
	return load(filename, src)
}

// LoadLang seperti Load, tetapi bahasanya ditentukan pemanggil (mis. languageId editor
// untuk file tanpa ekstensi). language "" atau "go" = Go.
func LoadLang(filename string, src []byte, language string) *Program {
	if language == "" || language == "go" {
		if l := lang.Detect(filename); l != "" && l != "go" {
			return Load(filename, src) // ekstensi file menang bila editor tidak tahu
		}
		return Load(filename, src)
	}
	return loadOther(filename, src, language)
}

func loadOther(filename string, src []byte, language string) *Program {
	res := lang.Analyze(language, filename, string(src))
	diags := res.Diagnostics
	if diags == nil {
		diags = []scene.Diagnostic{}
	}
	return &Program{
		Fset: token.NewFileSet(), File: &ast.File{Name: ast.NewIdent("")}, Info: &types.Info{},
		LoopNodes: map[string]ast.Stmt{}, LoopIDs: map[ast.Node]string{},
		BlockNodes: map[string]ast.Node{}, BlockIDs: map[ast.Node]string{},
		Result: Result{Scene: res.Scene, Diagnostics: diags},
		Lang:   language, Other: res,
	}
}

func analyze(filename string, src []byte) Result { return load(filename, src).Result }

func load(filename string, src []byte) *Program { return loadWith(filename, src, nil) }

// loadWith seperti load, ditambah file lain di package yang sama (ikut dicek tipenya).
func loadWith(filename string, src []byte, sibs []sibSrc) *Program {
	s := parse(filename, src)
	var siblings []*Sibling
	if len(sibs) > 0 && s.shift == 0 && s.file.Name != nil {
		siblings = parseSiblings(s.fset, s.file, sibs)
	}
	var extra []*ast.File
	for _, sb := range siblings {
		extra = append(extra, sb.File)
	}
	info := typeCheck(s, extra...)
	b := &builder{src: s, info: info, ids: map[ast.Node]string{}}
	cands := b.collect()

	sort.SliceStable(cands, func(i, j int) bool { return cands[i].off < cands[j].off })
	for i, c := range cands {
		c.id = fmt.Sprintf("L%d", i+1)
		if c.node != nil {
			b.ids[c.node] = c.id
		}
	}
	blockCands := b.collectBlocks() // ID if/switch harus ada sebelum node() dibangun

	loops := make([]scene.Loop, 0, len(cands))
	for _, c := range cands {
		l := b.loop(c)
		if c.parent != nil {
			l.Parent = c.parent.id
		}
		for _, d := range cands {
			if d.parent == c {
				l.Children = append(l.Children, d.id)
			}
		}
		loops = append(loops, l)
	}

	blocks := b.blocks(blockCands, cands, loops)

	loopNodes := map[string]ast.Stmt{}
	loopIDs := map[ast.Node]string{}
	blockNodes := map[string]ast.Node{}
	for n, id := range b.ids {
		blockNodes[id] = n
		if isLoop(n) {
			loopNodes[id] = n.(ast.Stmt)
			loopIDs[n] = id
		}
	}
	return &Program{
		Fset: s.fset, File: s.file, Info: info, src: s, Siblings: siblings,
		LoopNodes: loopNodes, LoopIDs: loopIDs, BlockNodes: blockNodes, BlockIDs: b.ids,
		Result: Result{
			Scene: scene.Scene{
				Version: scene.Version, Language: "go", File: filename,
				Blocks: blocks, Loops: loops,
			},
			Diagnostics: s.diagnostics(src),
		},
	}
}

// ---------- parsing ----------

type source struct {
	fset  *token.FileSet
	file  *ast.File
	text  []byte // sumber yang di-parse, mungkin sudah dibungkus
	shift int    // jumlah baris pembungkus di depan
	errs  scanner.ErrorList
}

const (
	wrapPkg  = "package p\n"
	wrapFunc = "package p\nfunc _() {\n"
)

// parse mem-parse src. Potongan kode tanpa klausa package (mis. contoh yang
// ditempel) dibungkus dulu supaya loop di dalamnya tetap ditemukan.
func parse(filename string, src []byte) source {
	s := parseOnce(filename, src, 0)
	if s.file.Package.IsValid() {
		return s
	}
	prefix, suffix := wrapFunc, "\n}\n"
	if hasTopLevelFunc(src) {
		prefix, suffix = wrapPkg, "\n"
	}
	wrapped := make([]byte, 0, len(prefix)+len(src)+len(suffix))
	wrapped = append(append(append(wrapped, prefix...), src...), suffix...)
	return parseOnce(filename, wrapped, strings.Count(prefix, "\n"))
}

func parseOnce(filename string, text []byte, shift int) source {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, text, parser.AllErrors|parser.SkipObjectResolution)
	if f == nil {
		f = &ast.File{Name: ast.NewIdent("")}
	}
	s := source{fset: fset, file: f, text: text, shift: shift}
	if el, ok := err.(scanner.ErrorList); ok {
		s.errs = el
	} else if err != nil {
		s.errs = scanner.ErrorList{{Msg: err.Error()}}
	}
	return s
}

func hasTopLevelFunc(src []byte) bool {
	for _, line := range bytes.Split(src, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("func ")) {
			return true
		}
	}
	return false
}

func (s source) offset(p token.Pos) int {
	if !p.IsValid() {
		return -1
	}
	tf := s.fset.File(p)
	if tf == nil {
		return -1
	}
	return tf.Offset(p)
}

func (s source) line(p token.Pos) int {
	return max(1, s.fset.Position(p).Line-s.shift)
}

func (s source) slice(from, to int) string {
	from, to = max(0, from), min(len(s.text), to)
	if from >= to {
		return ""
	}
	return string(s.text[from:to])
}

// lineEnd mengembalikan offset akhir baris yang memuat off.
func (s source) lineEnd(off int) int {
	if off < 0 || off >= len(s.text) {
		return len(s.text)
	}
	if i := bytes.IndexByte(s.text[off:], '\n'); i >= 0 {
		return off + i
	}
	return len(s.text)
}

func (s source) diagnostics(orig []byte) []scene.Diagnostic {
	lastLine := bytes.Count(orig, []byte("\n")) + 1
	out := []scene.Diagnostic{}
	seen := map[scene.Diagnostic]bool{} // parser Go kadang melaporkan pesan yang sama dua kali
	for _, e := range s.errs {
		d := scene.Diagnostic{Message: e.Msg}
		if e.Pos.Line > 0 {
			d.Line = min(max(1, e.Pos.Line-s.shift), lastLine)
			d.Col = e.Pos.Column
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// ---------- tipe ----------

// stubImporter ada di stubs.go: package standar yang sering dipakai punya tanda tangan
// tiruan, package lain kosong (tipenya tidak diketahui, tapi pemeriksaan tetap cepat).

func typeCheck(s source, extra ...*ast.File) *types.Info {
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	conf := types.Config{Importer: stubImporter{}, Error: func(error) {}, FakeImportC: true}
	func() {
		// AST dari kode setengah jadi bisa aneh; kegagalan di sini cukup berarti "tipe tidak diketahui".
		defer func() { _ = recover() }()
		_, _ = conf.Check("p", s.fset, append([]*ast.File{s.file}, extra...), info)
	}()
	return info
}

// ---------- pengumpulan loop ----------

type cand struct {
	node   ast.Stmt // *ast.ForStmt atau *ast.RangeStmt; nil jika hanya ditemukan oleh scanner
	off    int
	end    int
	label  string
	parent *cand
	header string // untuk loop dari scanner
	line   int    // untuk loop dari scanner (baris di sumber yang di-parse)
	id     string
}

type builder struct {
	src  source
	info *types.Info
	ids  map[ast.Node]string
}

func isLoop(n ast.Node) bool {
	switch n.(type) {
	case *ast.ForStmt, *ast.RangeStmt:
		return true
	}
	return false
}

func (b *builder) collect() []*cand {
	var cands []*cand
	byNode := map[ast.Node]*cand{}
	var stack []ast.Node
	var open []*cand
	ast.Inspect(b.src.file, func(n ast.Node) bool {
		if n == nil {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, ok := byNode[top]; ok {
				open = open[:len(open)-1]
			}
			return true
		}
		if isLoop(n) {
			c := &cand{node: n.(ast.Stmt), off: b.src.offset(n.Pos()), end: b.src.offset(n.End())}
			if len(stack) > 0 {
				if ls, ok := stack[len(stack)-1].(*ast.LabeledStmt); ok && ls.Stmt == n {
					c.label = ls.Label.Name
				}
			}
			if len(open) > 0 {
				c.parent = open[len(open)-1]
			}
			cands = append(cands, c)
			byNode[n] = c
			open = append(open, c)
		}
		stack = append(stack, n)
		return true
	})
	return append(cands, b.scanOrphans(cands)...)
}

// scanOrphans mencari kata kunci `for` yang tidak menghasilkan node loop di AST,
// mis. karena kepalanya baru setengah diketik. Loop ini ditandai incomplete.
func (b *builder) scanOrphans(found []*cand) []*cand {
	lines := map[int]bool{}
	for _, c := range found {
		lines[b.src.fset.Position(c.node.Pos()).Line] = true
	}
	fset := token.NewFileSet()
	tf := fset.AddFile("", -1, len(b.src.text))
	var sc scanner.Scanner
	sc.Init(tf, b.src.text, nil, 0)
	var out []*cand
	for {
		pos, tok, _ := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.FOR {
			continue
		}
		p := fset.Position(pos)
		if lines[p.Line] {
			continue
		}
		end := b.src.lineEnd(p.Offset)
		header := b.src.slice(p.Offset, end)
		if i := strings.IndexByte(header, '{'); i >= 0 {
			header = header[:i]
		}
		c := &cand{off: p.Offset, end: end, line: p.Line, header: oneLine(header)}
		for _, f := range found {
			if f.off < c.off && c.off < f.end && (c.parent == nil || f.off > c.parent.off) {
				c.parent = f
			}
		}
		out = append(out, c)
		lines[p.Line] = true
	}
	return out
}

// ---------- membangun Scene ----------

func (b *builder) loop(c *cand) scene.Loop {
	l := scene.Loop{ID: c.id, Label: c.label, Body: []scene.Node{}}
	if c.node == nil {
		l.Kind = guessKind(c.header)
		l.Line = max(1, c.line-b.src.shift)
		l.EndLine = l.Line
		l.State = scene.StateIncomplete
		l.Header.Text = c.header
		return l
	}

	var body *ast.BlockStmt
	switch n := c.node.(type) {
	case *ast.ForStmt:
		body = n.Body
	case *ast.RangeStmt:
		body = n.Body
	}
	// Kalau "{" belum diketik, parser menelan baris-baris berikutnya ke kepala loop.
	// Batasi kepala sampai "{" yang benar-benar ada, atau sampai akhir baris "for".
	start := b.src.offset(c.node.Pos())
	hasBody := body != nil && b.isChar(body.Lbrace, '{')
	limit := b.src.lineEnd(start)
	if hasBody {
		limit = b.src.offset(body.Lbrace)
	}
	part := func(n ast.Node) string {
		if n == nil || b.src.offset(n.Pos()) >= limit {
			return ""
		}
		return oneLine(b.src.slice(b.src.offset(n.Pos()), min(b.src.offset(n.End()), limit)))
	}

	switch n := c.node.(type) {
	case *ast.ForStmt:
		l.Kind = forKind(n)
		l.Header.Init = part(n.Init)
		l.Header.Cond = part(n.Cond)
		l.Header.Post = part(n.Post)
		if as, ok := n.Init.(*ast.AssignStmt); ok {
			l.Vars = identNames(as.Lhs...)
		}
	case *ast.RangeStmt:
		l.Kind = rangeKind(b.info, n)
		l.Header.Key = part(n.Key)
		l.Header.Value = part(n.Value)
		l.Header.Define = n.Tok == token.DEFINE
		l.Header.Range = part(n.X)
		l.Vars = identNames(n.Key, n.Value)
	}

	l.Header.Text = oneLine(b.src.slice(start, limit))
	l.Line = b.src.line(c.node.Pos())
	l.EndLine = l.Line
	if hasBody {
		l.EndLine = max(l.Line, b.src.line(c.node.End()-1))
		l.Body = b.nodes(body.List)
	}
	l.State = scene.StateComplete
	if !hasBody || b.incomplete(c.node, body) {
		l.State = scene.StateIncomplete
	}
	return l
}

// isChar melaporkan apakah p menunjuk karakter ch di sumber. Parser Go mengisi
// posisi kurung kurawal walau kurungnya tidak ada, jadi IsValid saja tidak cukup.
func (b *builder) isChar(p token.Pos, ch byte) bool {
	off := b.src.offset(p)
	return off >= 0 && off < len(b.src.text) && b.src.text[off] == ch
}

func (b *builder) incomplete(n ast.Node, body *ast.BlockStmt) bool {
	if !b.isChar(body.Rbrace, '}') {
		return true
	}
	from, to := b.src.offset(n.Pos()), b.src.offset(n.End())
	for _, e := range b.src.errs {
		if e.Pos.Offset >= from && e.Pos.Offset <= to {
			return true
		}
	}
	bad := false
	ast.Inspect(n, func(m ast.Node) bool {
		switch m.(type) {
		case *ast.BadExpr, *ast.BadStmt, *ast.BadDecl:
			bad = true
		}
		return !bad
	})
	return bad
}

func (b *builder) nodes(list []ast.Stmt) []scene.Node {
	out := []scene.Node{}
	for _, st := range list {
		if n, ok := b.node(st); ok {
			if end := b.src.line(st.End() - 1); end > n.Line {
				n.EndLine = end
			}
			out = append(out, n)
		}
	}
	return out
}

func (b *builder) node(st ast.Stmt) (scene.Node, bool) {
	line := b.src.line(st.Pos())
	switch s := st.(type) {
	case *ast.EmptyStmt:
		return scene.Node{}, false
	case *ast.LabeledStmt:
		return b.node(s.Stmt)
	case *ast.ForStmt, *ast.RangeStmt:
		return scene.Node{Kind: scene.NodeLoop, Line: line, Ref: b.ids[st]}, true
	case *ast.BranchStmt:
		n := scene.Node{Line: line, Text: b.text(s)}
		switch s.Tok {
		case token.BREAK:
			n.Kind = scene.NodeBreak
		case token.CONTINUE:
			n.Kind = scene.NodeContinue
		case token.GOTO:
			n.Kind = scene.NodeGoto
		default:
			n.Kind = scene.NodeStmt
		}
		if s.Label != nil {
			n.Label = s.Label.Name
		}
		return n, true
	case *ast.ReturnStmt:
		return scene.Node{Kind: scene.NodeReturn, Line: line, Text: short(b.text(s))}, true
	case *ast.IfStmt:
		n := scene.Node{Kind: scene.NodeIf, Line: line, Ref: b.ids[s]} // else-if tidak punya ID sendiri
		headEnd := b.src.lineEnd(b.src.offset(s.Pos()))
		if s.Body != nil && b.isChar(s.Body.Lbrace, '{') {
			headEnd = b.src.offset(s.Body.Lbrace)
			n.Then = b.nodes(s.Body.List)
		}
		n.Text = short(oneLine(b.src.slice(b.src.offset(s.Pos()), headEnd)))
		switch e := s.Else.(type) {
		case *ast.BlockStmt:
			n.Else = b.nodes(e.List)
		case *ast.IfStmt:
			if en, ok := b.node(e); ok {
				n.Else = []scene.Node{en}
			}
		}
		return n, true
	case *ast.BlockStmt:
		return scene.Node{Kind: scene.NodeBlock, Line: line, Body: b.nodes(s.List)}, true
	case *ast.SwitchStmt, *ast.TypeSwitchStmt:
		return scene.Node{Kind: scene.NodeSwitch, Line: line, Ref: b.ids[s], Text: short(oneLine(firstLine(b.raw(s))))}, true
	case *ast.BadStmt:
		return scene.Node{Kind: scene.NodeBad, Line: line, Text: short(b.text(s))}, true
	default:
		// Statement multi-baris (mis. switch, closure) cukup diwakili baris pertamanya.
		return scene.Node{Kind: scene.NodeStmt, Line: line, Text: short(oneLine(firstLine(b.raw(s))))}, true
	}
}

// raw mengembalikan sumber node apa adanya; "" jika node kosong.
func (b *builder) raw(n ast.Node) string {
	if n == nil {
		return ""
	}
	return b.src.slice(b.src.offset(n.Pos()), b.src.offset(n.End()))
}

// text mengembalikan sumber node dalam satu baris.
func (b *builder) text(n ast.Node) string { return oneLine(b.raw(n)) }

// ---------- klasifikasi ----------

func forKind(n *ast.ForStmt) scene.Kind {
	switch {
	case n.Init == nil && n.Cond == nil && n.Post == nil:
		return scene.KindInfinite
	case n.Init == nil && n.Post == nil:
		return scene.KindCond
	default:
		return scene.KindFor3
	}
}

func rangeKind(info *types.Info, r *ast.RangeStmt) scene.Kind {
	if r.X == nil {
		return scene.KindRangeUnknown
	}
	if tv, ok := info.Types[r.X]; ok && tv.Type != nil {
		if k, ok := kindOfType(tv.Type); ok {
			return k
		}
	}
	// Tipe tidak terbaca (mis. kode belum valid): tebak dari bentuk ekspresinya.
	switch x := r.X.(type) {
	case *ast.BasicLit:
		switch x.Kind {
		case token.INT:
			return scene.KindRangeInt
		case token.STRING:
			return scene.KindRangeString
		}
	case *ast.CompositeLit:
		switch t := x.Type.(type) {
		case *ast.ArrayType:
			if t.Len == nil {
				return scene.KindRangeSlice
			}
			return scene.KindRangeArray
		case *ast.MapType:
			return scene.KindRangeMap
		}
	}
	return scene.KindRangeUnknown
}

func kindOfType(t types.Type) (scene.Kind, bool) {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsString != 0:
			return scene.KindRangeString, true
		case u.Info()&types.IsInteger != 0:
			return scene.KindRangeInt, true
		}
	case *types.Slice:
		return scene.KindRangeSlice, true
	case *types.Array:
		return scene.KindRangeArray, true
	case *types.Pointer:
		if _, ok := u.Elem().Underlying().(*types.Array); ok {
			return scene.KindRangeArray, true
		}
	case *types.Map:
		return scene.KindRangeMap, true
	case *types.Chan:
		return scene.KindRangeChan, true
	case *types.Signature:
		return scene.KindRangeFunc, true
	}
	return "", false
}

// guessKind menebak jenis loop dari teks kepala yang belum bisa di-parse.
func guessKind(header string) scene.Kind {
	rest := strings.TrimSpace(strings.TrimPrefix(header, "for"))
	switch {
	case strings.Contains(rest, "range"):
		return scene.KindRangeUnknown
	case strings.Contains(rest, ";"):
		return scene.KindFor3
	case rest == "":
		return scene.KindInfinite
	default:
		return scene.KindCond
	}
}

func identNames(exprs ...ast.Expr) []string {
	var out []string
	for _, e := range exprs {
		if id, ok := e.(*ast.Ident); ok && id != nil && id.Name != "_" {
			out = append(out, id.Name)
		}
	}
	return out
}

// ---------- teks ----------

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstLine(s string) string {
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
