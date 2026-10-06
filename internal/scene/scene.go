// Package scene mendefinisikan kontrak Scene v1: struktur loop di sebuah file Go
// yang dihitung ulang setiap ketikan dan digambar oleh semua renderer.
//
// Ini KONTRAK antara core dan tampilan. Field baru boleh ditambah; mengubah atau
// menghapus field berarti menaikkan Version dan memperbarui CLI + ekstensi sekaligus.
package scene

// Version adalah versi skema Scene.
const Version = 1

// Kind adalah jenis loop. Dipakai juga oleh kontrak Trace.
type Kind string

const (
	KindFor3         Kind = "for3"          // for init; cond; post { }
	KindCond         Kind = "cond"          // for cond { } (seperti while)
	KindInfinite     Kind = "infinite"      // for { }
	KindRangeInt     Kind = "range-int"     // for i := range 5 { }
	KindRangeSlice   Kind = "range-slice"   // for i, v := range []T{} { }
	KindRangeArray   Kind = "range-array"   // array atau pointer ke array
	KindRangeString  Kind = "range-string"  // per rune, bukan per byte
	KindRangeMap     Kind = "range-map"     // urutan tidak dijamin
	KindRangeChan    Kind = "range-chan"    // sampai channel ditutup
	KindRangeFunc    Kind = "range-func"    // iterator Go 1.23
	KindRangeUnknown Kind = "range-unknown" // tipe ekspresi range tidak bisa ditentukan
)

// State menandai apakah loop sudah lengkap secara sintaks.
type State string

const (
	StateComplete   State = "complete"
	StateIncomplete State = "incomplete" // digambar sebagai blok "hantu"
)

// Scene adalah semua loop di satu file.
type Scene struct {
	Version int    `json:"version"`
	File    string `json:"file,omitempty"`
	Loops   []Loop `json:"loops"`
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

// NodeKind adalah jenis satu bagian di dalam body loop.
type NodeKind string

const (
	NodeStmt     NodeKind = "stmt"
	NodeIf       NodeKind = "if"
	NodeLoop     NodeKind = "loop"
	NodeBreak    NodeKind = "break"
	NodeContinue NodeKind = "continue"
	NodeReturn   NodeKind = "return"
	NodeGoto     NodeKind = "goto"
	NodeBlock    NodeKind = "block"
	NodeBad      NodeKind = "bad" // bagian yang belum bisa di-parse
)

// Node adalah satu bagian di dalam body loop.
type Node struct {
	Kind    NodeKind `json:"kind"`
	Line    int      `json:"line,omitempty"`
	EndLine int      `json:"endLine,omitempty"` // hanya diisi bila bagian ini lebih dari satu baris
	Text    string   `json:"text,omitempty"`
	Label   string   `json:"label,omitempty"` // label pada break/continue/goto
	Ref     string   `json:"ref,omitempty"`   // ID loop untuk Kind == "loop"
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
