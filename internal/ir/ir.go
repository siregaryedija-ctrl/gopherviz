// Package ir adalah pohon sintaks bersama untuk bahasa selain Go (JavaScript, TypeScript,
// Python, PHP, C++, C, Java). Parser tiap bahasa (internal/lang/...) menghasilkan IR ini;
// pembangun Scene (mode struktur) dan interpreter (mode trace, Fase 9) hanya membaca IR.
//
// IR sengaja dekat dengan sintaks: konstruksi yang mirip antar bahasa memakai node yang
// sama (if, for klasik, for-each, while, switch, fungsi, kelas), dan perbedaan semantik
// diputuskan oleh interpreter per bahasa, bukan oleh parser.
package ir

// Pos adalah posisi di sumber: offset byte (0-based), baris dan kolom (1-based, kolom dalam byte).
type Pos struct {
	Off, Line, Col int
}

// Span adalah rentang [Start, End) di sumber.
type Span struct {
	Start, End Pos
}

// Node adalah semua node IR.
type Node interface {
	Span() Span
}

// Stmt adalah statement.
type Stmt interface {
	Node
	stmtNode()
}

// Expr adalah ekspresi.
type Expr interface {
	Node
	exprNode()
}

// File adalah hasil parse satu file.
type File struct {
	Lang  string // javascript | typescript | python | php | cpp | c | java
	Body  []Stmt // statement tingkat atas (termasuk deklarasi fungsi/kelas)
	Diags []Diag
}

// Diag adalah pesan kesalahan sintaks dalam Bahasa Indonesia.
type Diag struct {
	Pos Pos
	Msg string
}

// ---------- statement ----------

// Block adalah daftar statement di dalam { } (atau badan berindentasi di Python).
// Closed = false bila kurung/indentasi penutup belum ada (kode sedang diketik).
type Block struct {
	S      Span
	List   []Stmt
	Closed bool
}

type ExprStmt struct {
	S Span
	X Expr
}

// VarDecl mendeklarasikan satu atau lebih variabel: let/const/var (JS), tipe (C/Java), dll.
type VarDecl struct {
	S     Span
	Kind  string // "let", "const", "var", "auto", nama tipe C/Java, "global"/"static" (PHP), ...
	Specs []*VarSpec
}

type VarSpec struct {
	S       Span
	Name    string
	Pattern Expr   // destructuring JS ([a, b] / {x, y}) atau tuple Python; nil bila Name dipakai
	Type    string // anotasi/tipe sebagai teks, mis. "int", "int[]", "number"
	Dims    []Expr // ukuran array C/C++: int a[5] → [5]
	Init    Expr   // nil bila tanpa nilai awal
	Ctor    []Expr // C++ Point p(1, 2) → argumen konstruktor
	Ref     bool   // C++ int& r = x
}

// If adalah satu if; rantai else if = Else berisi *If lagi.
type If struct {
	S    Span
	Cond Expr
	Then Stmt
	Else Stmt // nil, *If (else if / elif), atau statement lain
	Elif bool // Python: if ini ditulis sebagai elif
}

// For adalah for klasik: for (init; cond; post).
type For struct {
	S      Span
	Init   Stmt // VarDecl atau ExprStmt; nil bila kosong
	Cond   Expr // nil = selamanya
	Post   Expr // nil bila kosong
	Body   Stmt
	Header Span // teks kepala sampai sebelum badan
}

// ForEach mencakup for...of / for...in (JS), for x in xs (Python), foreach (PHP),
// for (x : xs) (C++/Java).
type ForEach struct {
	S      Span
	Kind   string // "of", "in" (JS: key), "py", "foreach", "colon"
	Decl   string // "let", "const", "var", tipe C++/Java, "" bila tanpa deklarasi
	Key    Expr   // PHP $k => $v, Python "for i, x in enumerate(..)" memakai Value tuple
	Value  Expr   // target per elemen (Ident, atau pola/tuple)
	X      Expr   // yang diiterasi
	Body   Stmt
	Else   Stmt // Python for-else
	Header Span
}

// While adalah while (cond) atau do { } while (cond).
type While struct {
	S      Span
	Cond   Expr
	Body   Stmt
	Do     bool
	Else   Stmt // Python while-else
	Header Span
}

// Switch adalah switch (JS/C/Java/PHP) atau match (Python/PHP 8).
type Switch struct {
	S      Span
	Tag    Expr
	Cases  []*Case
	Match  bool // match: tanpa fallthrough, case = pola
	Header Span
}

// Case: Exprs nil = default (atau "case _" di Python).
type Case struct {
	S     Span
	Exprs []Expr
	Guard Expr // Python: case x if cond
	Body  []Stmt
	Arrow bool // Java switch baru "case 1 ->": tanpa fallthrough
}

// Branch adalah break atau continue.
type Branch struct {
	S     Span
	Tok   string // "break" | "continue"
	Label string
	Count int // PHP: break 2
}

type Return struct {
	S Span
	X Expr
}

// Throw: throw (JS/Java/C++/PHP) atau raise (Python).
type Throw struct {
	S Span
	X Expr
}

type Try struct {
	S       Span
	Body    *Block
	Catches []*Catch
	Else    *Block // Python try-else
	Finally *Block
}

type Catch struct {
	S     Span
	Types []string // tipe yang ditangkap (kosong = semua)
	Name  string   // nama variabel exception
	Body  *Block
}

// Func adalah deklarasi fungsi, method, fungsi anonim, atau arrow function.
type Func struct {
	S         Span
	Name      string // "" untuk fungsi anonim
	Params    []*Param
	Ret       string // tipe kembali sebagai teks
	Body      *Block // nil untuk prototipe / method abstrak
	ExprBody  Expr   // arrow function / lambda berbadan ekspresi
	Class     string // nama kelas bila method
	Static    bool
	Async     bool
	Generator bool
	Arrow     bool
	Kind      string   // "get" / "set" untuk getter/setter JS/TS, "" untuk fungsi biasa
	Decor     []string // dekorator Python: "property", "staticmethod", "classmethod", "x.setter", ...
	Uses      []string // PHP closure use ($x)
	Inits     []Expr   // C++: daftar inisialisasi konstruktor ": x(a), y(b)" sebagai Call{Fun: x, Args}
	Header    Span
}

type Param struct {
	Name    string
	Pattern Expr // destructuring
	Type    string
	Default Expr
	Rest    bool // ...args / *args
	KwRest  bool // **kwargs
	Ref     bool // PHP &$x, C++ int& x
	Promote bool // parameter konstruktor yang sekaligus properti (TS/PHP: public x)
}

// Class adalah kelas (JS/TS/Python/PHP/C++/Java) atau struct C/C++.
type Class struct {
	S       Span
	Kind    string // "class", "struct", "interface", "enum", "trait"
	Name    string
	Extends []string
	Members []Stmt // *VarDecl (field), *Func (method), statement lain (Python)
	Header  Span
}

// Labeled adalah statement berlabel (label: for ...).
type Labeled struct {
	S     Span
	Label string
	Stmt  Stmt
}

// Simple adalah statement sederhana tanpa struktur yang penting untuk tampilan atau
// simulasi awal: import/include/using, pass, global, typedef, deklarasi tipe TS, dll.
type Simple struct {
	S     Span
	Kind  string // "import", "pass", "global", "nonlocal", "del", "assert", "type", "directive", "empty", ...
	Names []string
	X     []Expr
}

// Bad adalah bagian yang gagal di-parse.
type Bad struct {
	S   Span
	Msg string
}

// ---------- ekspresi ----------

type Ident struct {
	S    Span
	Name string // PHP: termasuk "$"
}

type LitKind int

const (
	LitInt LitKind = iota
	LitFloat
	LitString
	LitChar
	LitBool
	LitNull // null / None / nullptr / NULL
	LitUndefined
	LitRegex
	LitBigInt
)

type Lit struct {
	S     Span
	Kind  LitKind
	Raw   string // teks sumber
	Value string // nilai yang sudah di-decode (string tanpa kutip, escape diproses)
}

// Template adalah string berinterpolasi: `a${x}` (JS), f"a{x}" (Python), "a $x" (PHP).
// len(Parts) == len(Exprs)+1.
type Template struct {
	S     Span
	Parts []string
	Exprs []Expr
	Specs []string // Python f-string: format spec per ekspresi (":.2f"), "" bila tidak ada
	Tag   Expr     // JS tagged template
}

type Unary struct {
	S       Span
	Op      string // "-", "+", "!", "~", "not", "typeof", "void", "delete", "await", "++", "--", "*", "&", "sizeof", "yield", "new"...
	X       Expr
	Postfix bool
}

type Binary struct {
	S  Span
	Op string // termasuk "&&", "||", "??", "and", "or", "in", "not in", "is", "is not", "instanceof", "===", "**", "//", ".", "<=>", "as"
	X  Expr
	Y  Expr
}

type Assign struct {
	S  Span
	Op string // "=", "+=", ..., ".=", "??=", ":=" (walrus)
	L  Expr
	R  Expr
}

// Cond adalah ternary c ? a : b, atau a if c else b di Python.
type Cond struct {
	S    Span
	C    Expr
	T, F Expr
}

type Call struct {
	S    Span
	Fun  Expr
	Args []Expr // *Spread untuk ...x / *x; *KwArg untuk name=value (Python) / name: value (PHP 8)
	Opt  bool   // a?.()
	Tmpl string // C++/Java/TS: argumen generik eksplisit sebagai teks, mis. "int"
}

type KwArg struct {
	S     Span
	Name  string
	Value Expr
	Star2 bool // **kwargs
}

type New struct {
	S     Span
	Type  string // nama kelas/tipe sebagai teks, mis. "Point", "int[]", "ArrayList<Integer>"
	Class Expr   // JS: ekspresi kelas
	Args  []Expr
	Dims  []Expr    // Java/C++: new int[5][3]
	Init  *ArrayLit // Java: new int[]{1, 2}
	Anon  *Class    // Java: kelas anonim
}

// Member: x.name, x?.name, x->name (PHP/C++), X::name (PHP/C++ statis).
type Member struct {
	S    Span
	X    Expr
	Name string
	Op   string // ".", "?.", "->", "::", "?->"
}

type Index struct {
	S   Span
	X   Expr
	I   Expr // nil untuk PHP $a[] (append)
	Opt bool // a?.[i]
}

// Slice adalah irisan Python a[lo:hi:step].
type Slice struct {
	S            Span
	X            Expr
	Lo, Hi, Step Expr
}

// ArrayLit: [1, 2] (JS/Python/PHP), {1, 2} (C/C++/Java initializer), array(1, 2) (PHP).
// Elemen bisa *Spread. PHP dengan key memakai ObjectLit.
type ArrayLit struct {
	S     Span
	Elems []Expr
	Kind  string // "array", "list" (Python), "tuple", "set", "init" (C/Java {..})
}

// ObjectLit: {a: 1} (JS), {"a": 1} (Python dict), ['a' => 1] (PHP).
type ObjectLit struct {
	S     Span
	Props []*Prop
	Kind  string // "object", "dict", "array" (PHP berkey)
}

type Prop struct {
	S        Span
	Key      Expr // Ident (nama biasa), Lit, atau ekspresi (computed)
	Value    Expr
	Computed bool
	Spread   bool // ...x / **x
	Short    bool // {x} singkatan {x: x}
	Method   bool
}

// Func juga dipakai sebagai ekspresi (fungsi anonim, arrow, lambda).
func (*Func) exprNode() {}

// Class juga bisa muncul sebagai ekspresi (JS class expression).
func (*Class) exprNode() {}

type Spread struct {
	S    Span
	X    Expr
	Dict bool // **x
}

// Cast: (int)x di C/C++/Java, static_cast<int>(x) di C++.
type Cast struct {
	S    Span
	Type string
	X    Expr
}

// Comp adalah list/dict/set comprehension atau generator expression Python.
type Comp struct {
	S     Span
	Kind  string // "list", "dict", "set", "gen"
	Elt   Expr   // list/set/gen
	Key   Expr   // dict
	Value Expr   // dict
	Fors  []*CompFor
}

type CompFor struct {
	Target Expr
	Iter   Expr
	Ifs    []Expr
}

type This struct {
	S    Span
	Name string // "this", "self" (PHP self::), "super", "parent", "static"
}

// Seq adalah operator koma C/JS: a, b (dipakai di bagian for).
type Seq struct {
	S    Span
	List []Expr
}

// TypeRef adalah nama tipe yang muncul sebagai ekspresi (sizeof(int), instanceof X, int.class).
type TypeRef struct {
	S    Span
	Name string
}

// BadExpr adalah ekspresi yang gagal di-parse.
type BadExpr struct {
	S   Span
	Msg string
}

// ---------- implementasi interface ----------

func (n *Block) Span() Span    { return n.S }
func (n *VarSpec) Span() Span  { return n.S }
func (n *Case) Span() Span     { return n.S }
func (n *Catch) Span() Span    { return n.S }
func (n *ExprStmt) Span() Span { return n.S }
func (n *VarDecl) Span() Span  { return n.S }
func (n *If) Span() Span       { return n.S }
func (n *For) Span() Span      { return n.S }
func (n *ForEach) Span() Span  { return n.S }
func (n *While) Span() Span    { return n.S }
func (n *Switch) Span() Span   { return n.S }
func (n *Branch) Span() Span   { return n.S }
func (n *Return) Span() Span   { return n.S }
func (n *Throw) Span() Span    { return n.S }
func (n *Try) Span() Span      { return n.S }
func (n *Func) Span() Span     { return n.S }
func (n *Class) Span() Span    { return n.S }
func (n *Labeled) Span() Span  { return n.S }
func (n *Simple) Span() Span   { return n.S }
func (n *Bad) Span() Span      { return n.S }

func (*Block) stmtNode()    {}
func (*ExprStmt) stmtNode() {}
func (*VarDecl) stmtNode()  {}
func (*If) stmtNode()       {}
func (*For) stmtNode()      {}
func (*ForEach) stmtNode()  {}
func (*While) stmtNode()    {}
func (*Switch) stmtNode()   {}
func (*Branch) stmtNode()   {}
func (*Return) stmtNode()   {}
func (*Throw) stmtNode()    {}
func (*Try) stmtNode()      {}
func (*Func) stmtNode()     {}
func (*Class) stmtNode()    {}
func (*Labeled) stmtNode()  {}
func (*Simple) stmtNode()   {}
func (*Bad) stmtNode()      {}

func (n *Ident) Span() Span     { return n.S }
func (n *Lit) Span() Span       { return n.S }
func (n *Template) Span() Span  { return n.S }
func (n *Unary) Span() Span     { return n.S }
func (n *Binary) Span() Span    { return n.S }
func (n *Assign) Span() Span    { return n.S }
func (n *Cond) Span() Span      { return n.S }
func (n *Call) Span() Span      { return n.S }
func (n *KwArg) Span() Span     { return n.S }
func (n *New) Span() Span       { return n.S }
func (n *Member) Span() Span    { return n.S }
func (n *Index) Span() Span     { return n.S }
func (n *Slice) Span() Span     { return n.S }
func (n *ArrayLit) Span() Span  { return n.S }
func (n *ObjectLit) Span() Span { return n.S }
func (n *Spread) Span() Span    { return n.S }
func (n *Cast) Span() Span      { return n.S }
func (n *Comp) Span() Span      { return n.S }
func (n *This) Span() Span      { return n.S }
func (n *Seq) Span() Span       { return n.S }
func (n *TypeRef) Span() Span   { return n.S }
func (n *BadExpr) Span() Span   { return n.S }

func (*Ident) exprNode()     {}
func (*Lit) exprNode()       {}
func (*Template) exprNode()  {}
func (*Unary) exprNode()     {}
func (*Binary) exprNode()    {}
func (*Assign) exprNode()    {}
func (*Cond) exprNode()      {}
func (*Call) exprNode()      {}
func (*KwArg) exprNode()     {}
func (*New) exprNode()       {}
func (*Member) exprNode()    {}
func (*Index) exprNode()     {}
func (*Slice) exprNode()     {}
func (*ArrayLit) exprNode()  {}
func (*ObjectLit) exprNode() {}
func (*Spread) exprNode()    {}
func (*Cast) exprNode()      {}
func (*Comp) exprNode()      {}
func (*This) exprNode()      {}
func (*Seq) exprNode()       {}
func (*TypeRef) exprNode()   {}
func (*BadExpr) exprNode()   {}

// Walk mengunjungi n dan semua anaknya (pre-order). fn mengembalikan false untuk
// melewati anak-anak node itu.
func Walk(n Node, fn func(Node) bool) {
	if n == nil || isNil(n) || !fn(n) {
		return
	}
	stmts := func(l []Stmt) {
		for _, s := range l {
			Walk(s, fn)
		}
	}
	exprs := func(l []Expr) {
		for _, x := range l {
			Walk(x, fn)
		}
	}
	switch n := n.(type) {
	case *Block:
		stmts(n.List)
	case *ExprStmt:
		Walk(n.X, fn)
	case *VarDecl:
		for _, s := range n.Specs {
			Walk(s.Pattern, fn)
			exprs(s.Dims)
			Walk(s.Init, fn)
			exprs(s.Ctor)
		}
	case *If:
		Walk(n.Cond, fn)
		Walk(n.Then, fn)
		Walk(n.Else, fn)
	case *For:
		Walk(n.Init, fn)
		Walk(n.Cond, fn)
		Walk(n.Post, fn)
		Walk(n.Body, fn)
	case *ForEach:
		Walk(n.Key, fn)
		Walk(n.Value, fn)
		Walk(n.X, fn)
		Walk(n.Body, fn)
		Walk(n.Else, fn)
	case *While:
		Walk(n.Cond, fn)
		Walk(n.Body, fn)
		Walk(n.Else, fn)
	case *Switch:
		Walk(n.Tag, fn)
		for _, c := range n.Cases {
			exprs(c.Exprs)
			Walk(c.Guard, fn)
			stmts(c.Body)
		}
	case *Return:
		Walk(n.X, fn)
	case *Throw:
		Walk(n.X, fn)
	case *Try:
		Walk(n.Body, fn)
		for _, c := range n.Catches {
			Walk(c.Body, fn)
		}
		Walk(n.Else, fn)
		Walk(n.Finally, fn)
	case *Func:
		for _, p := range n.Params {
			Walk(p.Default, fn)
		}
		Walk(n.Body, fn)
		Walk(n.ExprBody, fn)
	case *Class:
		stmts(n.Members)
	case *Labeled:
		Walk(n.Stmt, fn)
	case *Simple:
		exprs(n.X)
	case *Template:
		exprs(n.Exprs)
		Walk(n.Tag, fn)
	case *Unary:
		Walk(n.X, fn)
	case *Binary:
		Walk(n.X, fn)
		Walk(n.Y, fn)
	case *Assign:
		Walk(n.L, fn)
		Walk(n.R, fn)
	case *Cond:
		Walk(n.C, fn)
		Walk(n.T, fn)
		Walk(n.F, fn)
	case *Call:
		Walk(n.Fun, fn)
		exprs(n.Args)
	case *KwArg:
		Walk(n.Value, fn)
	case *New:
		Walk(n.Class, fn)
		exprs(n.Args)
		exprs(n.Dims)
		if n.Init != nil {
			Walk(n.Init, fn)
		}
		if n.Anon != nil {
			Walk(n.Anon, fn)
		}
	case *Member:
		Walk(n.X, fn)
	case *Index:
		Walk(n.X, fn)
		Walk(n.I, fn)
	case *Slice:
		Walk(n.X, fn)
		Walk(n.Lo, fn)
		Walk(n.Hi, fn)
		Walk(n.Step, fn)
	case *ArrayLit:
		exprs(n.Elems)
	case *ObjectLit:
		for _, p := range n.Props {
			if p.Computed {
				Walk(p.Key, fn)
			}
			Walk(p.Value, fn)
		}
	case *Spread:
		Walk(n.X, fn)
	case *Cast:
		Walk(n.X, fn)
	case *Comp:
		Walk(n.Elt, fn)
		Walk(n.Key, fn)
		Walk(n.Value, fn)
		for _, f := range n.Fors {
			Walk(f.Target, fn)
			Walk(f.Iter, fn)
			exprs(f.Ifs)
		}
	case *Seq:
		exprs(n.List)
	}
}

// isNil mendeteksi interface berisi pointer nil (mis. Stmt(nil *Block)).
func isNil(n Node) bool {
	switch n := n.(type) {
	case *Block:
		return n == nil
	case *If:
		return n == nil
	case *Func:
		return n == nil
	case *Class:
		return n == nil
	case *ArrayLit:
		return n == nil
	}
	return false
}
