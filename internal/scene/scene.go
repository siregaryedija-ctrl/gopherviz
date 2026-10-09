// Package scene mendefinisikan kontrak Scene v2: struktur kode (fungsi, if, switch,
// loop) di sebuah file yang dihitung ulang setiap ketikan dan digambar oleh semua
// renderer. Kontrak ini netral bahasa: dipakai untuk Go maupun bahasa lain.
//
// Ini KONTRAK antara core dan tampilan. Field baru boleh ditambah; mengubah atau
// menghapus field berarti menaikkan Version dan memperbarui CLI + ekstensi sekaligus.
package scene

// Version adalah versi skema Scene.
//
//	v1: hanya loops
//	v2: language + blocks (fungsi, if, switch, loop); loops tetap ada
const Version = 2

// Kind adalah jenis loop. Dipakai juga oleh kontrak Trace.
type Kind string

const (
	KindFor3         Kind = "for3"          // for init; cond; post { }
	KindCond         Kind = "cond"          // for cond { } / while (cond)
	KindDoWhile      Kind = "do-while"      // do { } while (cond): body jalan dulu, baru cek
	KindInfinite     Kind = "infinite"      // for { } / while (true)
	KindRangeInt     Kind = "range-int"     // for i := range 5 / for i in range(5)
	KindRangeSlice   Kind = "range-slice"   // per elemen slice/list/array dinamis
	KindRangeArray   Kind = "range-array"   // array atau pointer ke array
	KindRangeString  Kind = "range-string"  // per karakter
	KindRangeMap     Kind = "range-map"     // per pasangan key-value; urutan tidak dijamin di Go
	KindRangeKeys    Kind = "range-keys"    // per key objek (for...in di JavaScript)
	KindRangeChan    Kind = "range-chan"    // sampai channel ditutup
	KindRangeFunc    Kind = "range-func"    // iterator Go 1.23 / generator
	KindRangeUnknown Kind = "range-unknown" // tipe ekspresi range tidak bisa ditentukan
)

// State menandai apakah sebuah blok sudah lengkap secara sintaks.
type State string

const (
	StateComplete   State = "complete"
	StateIncomplete State = "incomplete" // digambar sebagai blok "hantu"
)

// Scene adalah semua blok di satu file.
type Scene struct {
	Version  int     `json:"version"`
	Language string  `json:"language"` // "go", "javascript", "typescript", "python", "php", "cpp", "c", "java"
	File     string  `json:"file,omitempty"`
	Blocks   []Block `json:"blocks"` // semua blok, urut posisi di file
	Loops    []Loop  `json:"loops"`  // rincian blok loop (ID sama dengan Block.ID)
}

// BlockKind adalah jenis blok yang bisa difokuskan dan divisualkan.
type BlockKind string

const (
	BlockFunc   BlockKind = "func"
	BlockIf     BlockKind = "if"     // if beserta seluruh rantai else if / else
	BlockSwitch BlockKind = "switch" // switch / match
	BlockLoop   BlockKind = "loop"   // rinciannya di Scene.Loops
	BlockClass  BlockKind = "class"  // class / struct (method-methodnya menjadi blok fungsi anaknya)
)

// Block adalah satu blok kode. ID: F1.. fungsi, I1.. if, S1.. switch, L1.. loop, C1.. class.
type Block struct {
	ID       string    `json:"id"`
	Kind     BlockKind `json:"kind"`
	Line     int       `json:"line"`
	EndLine  int       `json:"endLine"`
	State    State     `json:"state"`
	Name     string    `json:"name,omitempty"`     // nama fungsi atau label loop
	Header   string    `json:"header"`             // satu baris, mis. "func fib(n int) int", "if x > 3", "switch day"
	Params   []string  `json:"params,omitempty"`   // nama parameter fungsi
	Branches []Branch  `json:"branches,omitempty"` // cabang if/else atau case switch, urut sumber
	Body     []Node    `json:"body,omitempty"`     // isi fungsi (isi loop ada di Scene.Loops)
	Parent   string    `json:"parent,omitempty"`   // blok terdalam yang memuat blok ini
	Children []string  `json:"children,omitempty"`
}

// BranchKind adalah jenis cabang.
type BranchKind string

const (
	BranchIf      BranchKind = "if"
	BranchElseIf  BranchKind = "else-if"
	BranchElse    BranchKind = "else"
	BranchCase    BranchKind = "case"
	BranchDefault BranchKind = "default"
)

// Branch adalah satu cabang if/else atau satu case switch.
type Branch struct {
	Kind  BranchKind `json:"kind"`
	Label string     `json:"label"` // kondisi ("x > 3"), daftar case ("1, 2"), atau "" untuk else/default
	Line  int        `json:"line"`
	Body  []Node     `json:"body"`
}

// Loop adalah satu loop. Loop bersarang punya entri sendiri dan dirujuk lewat ID.
type Loop struct {
	ID       string   `json:"id"`
	Kind     Kind     `json:"kind"`
	Line     int      `json:"line"`
	EndLine  int      `json:"endLine"`
	Label    string   `json:"label,omitempty"`
	State    State    `json:"state"`
	Header   Header   `json:"header"`
	Vars     []string `json:"vars,omitempty"`
	Body     []Node   `json:"body"` // [] = body kosong, digambar sebagai blok kosong
	Parent   string   `json:"parent,omitempty"`
	Children []string `json:"children,omitempty"`
}

// Header berisi bagian-bagian kepala loop sebagai teks sumber.
type Header struct {
	Text   string `json:"text"` // kepala lengkap satu baris, mis. "for i := 0; i < 3; i++"
	Init   string `json:"init,omitempty"`
	Cond   string `json:"cond,omitempty"`
	Post   string `json:"post,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
	Define bool   `json:"define,omitempty"` // true untuk ":=", false untuk "="
	Range  string `json:"range,omitempty"`  // ekspresi setelah kata kunci range
}

// NodeKind adalah jenis satu bagian di dalam body.
type NodeKind string

const (
	NodeStmt     NodeKind = "stmt"
	NodeIf       NodeKind = "if"
	NodeSwitch   NodeKind = "switch"
	NodeLoop     NodeKind = "loop"
	NodeBreak    NodeKind = "break"
	NodeContinue NodeKind = "continue"
	NodeReturn   NodeKind = "return"
	NodeGoto     NodeKind = "goto"
	NodeBlock    NodeKind = "block"
	NodeBad      NodeKind = "bad" // bagian yang belum bisa di-parse
)

// Node adalah satu bagian di dalam body.
type Node struct {
	Kind    NodeKind `json:"kind"`
	Line    int      `json:"line,omitempty"`
	EndLine int      `json:"endLine,omitempty"` // hanya diisi bila bagian ini lebih dari satu baris
	Text    string   `json:"text,omitempty"`
	Label   string   `json:"label,omitempty"` // label pada break/continue/goto
	Ref     string   `json:"ref,omitempty"`   // ID blok untuk loop, if, dan switch
	Then    []Node   `json:"then,omitempty"`
	Else    []Node   `json:"else,omitempty"`
	Body    []Node   `json:"body,omitempty"` // isi untuk Kind == "block"
}

// Diagnostic adalah pesan dari parser, dengan nomor baris file asli.
type Diagnostic struct {
	Line    int    `json:"line,omitempty"`
	Col     int    `json:"col,omitempty"`
	Message string `json:"message"`
}

// LoopAt mengembalikan ID loop terdalam yang mencakup baris line.
func (s Scene) LoopAt(line int) (string, bool) {
	best, bestSpan := "", -1
	for _, l := range s.Loops {
		if line < l.Line || line > l.EndLine {
			continue
		}
		span := l.EndLine - l.Line
		if bestSpan < 0 || span <= bestSpan {
			best, bestSpan = l.ID, span
		}
	}
	return best, bestSpan >= 0
}

// BlockAt mengembalikan ID blok terdalam (jenis apa pun) yang mencakup baris line.
func (s Scene) BlockAt(line int) (string, bool) {
	best, bestSpan := "", -1
	for _, b := range s.Blocks {
		if line < b.Line || line > b.EndLine {
			continue
		}
		span := b.EndLine - b.Line
		if bestSpan < 0 || span <= bestSpan {
			best, bestSpan = b.ID, span
		}
	}
	return best, bestSpan >= 0
}

// Block mengembalikan blok dengan ID id.
func (s Scene) Block(id string) (Block, bool) {
	for _, b := range s.Blocks {
		if b.ID == id {
			return b, true
		}
	}
	return Block{}, false
}
