// Package trace mendefinisikan kontrak Trace v2: eksekusi satu blok (loop, if,
// switch, atau fungsi) langkah demi langkah, yang dianimasikan oleh renderer.
//
// Ini KONTRAK antara core dan tampilan. Field baru boleh ditambah; mengubah atau
// menghapus field berarti menaikkan Version dan memperbarui CLI + ekstensi sekaligus.
package trace

import "gopherviz/internal/scene"

// Version adalah versi skema Trace.
//
//	v1: fokus hanya loop
//	v2: fokus blok apa pun (Trace.Block), langkah call/return/branch, Step.Block/Depth
const Version = 2

// Mode menyatakan cara trace dihasilkan.
type Mode string

const (
	ModeSimulated    Mode = "simulated"    // interpreter mini, tidak menjalankan kode user
	ModeStructural   Mode = "structural"   // nilai tidak diketahui, hanya alur fase
	ModeInstrumented Mode = "instrumented" // kode user benar-benar dijalankan (opt-in)
)

// Phase adalah fase satu langkah.
type Phase string

const (
	PhaseInit       Phase = "init"
	PhaseCond       Phase = "cond"
	PhaseBody       Phase = "body"
	PhasePost       Phase = "post"
	PhaseContinue   Phase = "continue"
	PhaseBreak      Phase = "break"
	PhaseEnterChild Phase = "enter-child"
	PhaseExit       Phase = "exit"
	PhaseBranch     Phase = "branch" // if/switch memilih cabang Step.Branch
	PhaseCall       Phase = "call"   // masuk ke fungsi Step.Func dengan argumen Step.Args
	PhaseReturn     Phase = "return" // keluar dari fungsi membawa Step.Return
)

// Reason menjelaskan kenapa blok berhenti (hanya pada PhaseExit).
type Reason string

const (
	ReasonCondFalse Reason = "cond-false"
	ReasonRangeDone Reason = "range-done"
	ReasonBreak     Reason = "break"
	ReasonContinue  Reason = "continue" // if/switch ditinggalkan karena continue loop di luarnya
	ReasonReturn    Reason = "return"
	ReasonGoto      Reason = "goto"
	ReasonStepLimit Reason = "step-limit"
	ReasonPanic     Reason = "panic" // program akan panic di sini (mis. index di luar batas)
	ReasonDone      Reason = "done"  // if/switch selesai normal
)

// OpKind adalah operasi pada koleksi yang terlihat di mode data.
type OpKind string

const (
	OpRead    OpKind = "read"
	OpCompare OpKind = "compare"
	OpSwap    OpKind = "swap"
	OpSet     OpKind = "set"
	OpAppend  OpKind = "append"
	OpSettle  OpKind = "settle" // elemen sudah di posisi akhirnya
	OpDelete  OpKind = "delete" // key dihapus dari map
	OpSend    OpKind = "send"   // nilai masuk ke channel Key (ID channel) di slot A
	OpRecv    OpKind = "recv"   // nilai diambil dari slot A channel Key
)

// Trace adalah hasil eksekusi satu blok.
type Trace struct {
	Version    int         `json:"version"`
	Language   string      `json:"language"`
	File       string      `json:"file,omitempty"`
	Mode       Mode        `json:"mode"`
	Block      BlockInfo   `json:"block"`
	Loop       LoopInfo    `json:"loop"` // diisi bila Block.Kind == "loop" (kompatibel dengan v1)
	Steps      []Step      `json:"steps"`
	Collection *Collection `json:"collection,omitempty"`
	Graph      *Graph      `json:"graph,omitempty"` // graf yang ditelusuri DFS/BFS di blok fokus
	Grid       *Grid       `json:"grid,omitempty"`  // grid labirin yang ditelusuri BFS/DFS di blok fokus
	Tree       *Tree       `json:"tree,omitempty"`  // pohon yang ditelusuri rekursi node.left/node.right di blok fokus
	Class      *ClassInfo  `json:"class,omitempty"` // cetak biru class (blok fokus class, atau class pemilik method fokus)
	Files      []string    `json:"files,omitempty"` // file program yang ikut berjalan: file fokus dulu, lalu modul (relatif), bila lebih dari satu
	Entry      string      `json:"entry,omitempty"` // file tempat program dimulai bila bukan file fokus (file yang meng-import-nya)
	Truncated  bool        `json:"truncated"`
	Warnings   []string    `json:"warnings"`
	ReadsInput bool        `json:"readsInput,omitempty"` // program membaca input user (panel menampilkan kotak Input)
}

// BlockInfo merangkum blok yang di-trace.
type BlockInfo struct {
	ID      string          `json:"id"`
	Kind    scene.BlockKind `json:"kind"`
	Line    int             `json:"line"`
	EndLine int             `json:"endLine"`
	Name    string          `json:"name,omitempty"`
	Names   []string        `json:"names,omitempty"` // nama yang dipakai di dalam blok (penunjuk lo/mid/hi hanya diambil dari sini)
}

// LoopInfo merangkum loop yang di-trace.
type LoopInfo struct {
	ID       string     `json:"id"`
	Kind     scene.Kind `json:"kind"`
	Line     int        `json:"line"`
	EndLine  int        `json:"endLine"`
	Label    string     `json:"label,omitempty"`
	Vars     []string   `json:"vars,omitempty"`
	Children []string   `json:"children,omitempty"`
}

// Step adalah satu langkah eksekusi. Nilai variabel selalu string supaya
// renderer tidak perlu memahami tipe Go.
type Step struct {
	N         int               `json:"n"`
	Loop      string            `json:"loop"`            // loop terdalam yang sedang berjalan ("" bila tidak di dalam loop)
	Block     string            `json:"block,omitempty"` // blok terdalam (jenis apa pun) yang sedang berjalan
	Run       int               `json:"run,omitempty"`   // eksekusi ke-berapa dari blok fokus
	Depth     int               `json:"depth,omitempty"` // kedalaman panggilan fungsi relatif terhadap awal fokus
	Phase     Phase             `json:"phase"`
	Line      int               `json:"line,omitempty"`
	File      string            `json:"file,omitempty"` // file langkah ini bila bukan file fokus (relatif, mis. "utils.py")
	Iteration *int              `json:"iteration,omitempty"`
	Branch    *int              `json:"branch,omitempty"` // indeks cabang yang dipilih (PhaseBranch); len(branches) = tidak ada yang cocok
	Func      string            `json:"func,omitempty"`   // nama fungsi pada call/return
	Args      string            `json:"args,omitempty"`   // argumen pada call, mis. "n = 3"
	Return    string            `json:"return,omitempty"` // nilai kembali pada return
	Vars      map[string]string `json:"vars,omitempty"`
	Result    *bool             `json:"result,omitempty"`
	Note      string            `json:"note,omitempty"`
	Output    string            `json:"output,omitempty"`
	Reason    Reason            `json:"reason,omitempty"`
	Ops       []Op              `json:"ops,omitempty"`
	Items     []string          `json:"items,omitempty"`    // isi koleksi terbaru, hanya bila berubah di langkah ini
	Keys      []string          `json:"keys,omitempty"`     // key map yang sejajar dengan Items (koleksi map)
	Mem       *Memory           `json:"mem,omitempty"`      // potret memori (bila program memakai struct/pointer)
	Child     string            `json:"child,omitempty"`    // ID loop anak pada PhaseEnterChild
	G         int               `json:"g,omitempty"`        // goroutine yang menjalankan langkah ini (1 = main); 0 bila tanpa goroutine
	Gs        []GState          `json:"gs,omitempty"`       // potret semua goroutine (bila program memakai go)
	Chans     []ChanState       `json:"chans,omitempty"`    // potret semua channel
	Graph     *GraphState       `json:"graph,omitempty"`    // keadaan penelusuran graf (bila Trace.Graph ada)
	Cells     *GridState        `json:"cells,omitempty"`    // keadaan penelusuran grid (bila Trace.Grid ada)
	Node      *TreeState        `json:"node,omitempty"`     // simpul pohon yang dipegang parameter (bila Trace.Tree ada)
	Objs      []ObjState        `json:"objs,omitempty"`     // potret object class fokus (trace class) atau object self (trace method)
	Self      string            `json:"self,omitempty"`     // ID object yang menjadi this/self di langkah ini
	SelfCopy  bool              `json:"selfCopy,omitempty"` // Go: method value receiver bekerja pada salinan object Self
}

// ClassInfo adalah "cetak biru" class fokus: field dan method yang dideklarasikan.
type ClassInfo struct {
	Name    string   `json:"name"`
	Super   string   `json:"super,omitempty"`   // class induk (pewarisan)
	Subs    []string `json:"subs,omitempty"`    // class anak di file yang sama
	Fields  []string `json:"fields,omitempty"`  // field yang terlihat di kode (deklarasi atau self.x = ...)
	Methods []string `json:"methods,omitempty"` // nama method, urut sumber
}

// ObjState adalah satu object pada satu langkah: rumah yang dibangun dari cetak biru class.
type ObjState struct {
	ID     string     `json:"id"`             // "obj1", stabil sepanjang trace
	Class  string     `json:"class"`          // class object ini (bisa class anak)
	Name   string     `json:"name,omitempty"` // variabel yang memegangnya, mis. "a" atau "siswa[0]"
	Fields []MemField `json:"fields"`         // urut field dibuat; Points = ID object lain bila field berisi object
}

// Graph adalah graf (daftar ketetanggaan atau matriks) yang ditelusuri di blok fokus.
type Graph struct {
	Var      string      `json:"var"`                // nama variabel graf
	Kind     string      `json:"kind,omitempty"`     // "bfs" (antrian) | "dfs" (tumpukan/rekursi) | "" (tidak diketahui)
	Nodes    []string    `json:"nodes"`              // simpul, urut seperti di program
	Edges    [][2]string `json:"edges"`              // sisi from → to
	Directed bool        `json:"directed,omitempty"` // false bila setiap sisi punya pasangan balik
	Frontier string      `json:"frontier,omitempty"` // nama variabel antrian/tumpukan
	Visited  string      `json:"visited,omitempty"`  // nama variabel penanda simpul yang sudah dikunjungi
}

// Grid adalah grid 2D (labirin) yang ditelusuri BFS/DFS di blok fokus. Rows = isi awal setiap sel
// sebagai teks; panel menentukan tembok dari nilai sel yang tidak pernah dikunjungi.
type Grid struct {
	Var      string     `json:"var"`
	Rows     [][]string `json:"rows"`
	Start    []int      `json:"start,omitempty"`    // [baris, kolom] titik awal
	Goal     []int      `json:"goal,omitempty"`     // [baris, kolom] tujuan
	Kind     string     `json:"kind,omitempty"`     // "bfs" | "dfs"
	Frontier string     `json:"frontier,omitempty"` // variabel antrian/tumpukan
	Visited  string     `json:"visited,omitempty"`  // variabel penanda kunjungan / jarak
}

// GridState adalah keadaan penelusuran grid pada satu langkah: sel [baris, kolom].
type GridState struct {
	Cur      []int    `json:"cur,omitempty"`      // sel yang sedang diproses
	Visited  [][2]int `json:"visited,omitempty"`  // sel yang sudah ditandai (penanda kunjungan / jarak / isi grid yang diubah)
	Frontier [][2]int `json:"frontier,omitempty"` // isi antrian/tumpukan (depan/bawah dulu)
}

// Tree adalah pohon yang ditelusuri fungsi rekursif (node.left / node.right / node.children).
type Tree struct {
	Var   string     `json:"var"`  // parameter simpul, mis. "node"
	Root  string     `json:"root"` // ID simpul akar
	Nodes []TreeNode `json:"nodes"`
}

// TreeNode adalah satu simpul pohon; ID stabil sepanjang trace ("n1", "n2", ...).
type TreeNode struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Left  string   `json:"left,omitempty"`
	Right string   `json:"right,omitempty"`
	Kids  []string `json:"kids,omitempty"` // pohon umum: anak-anak berurutan (bukan left/right)
}

// TreeState: simpul yang dipegang parameter pada langkah ini; Null = parameter berisi
// None/null/nil (pemanggilan untuk anak yang kosong).
type TreeState struct {
	Cur  string `json:"cur,omitempty"`
	Null bool   `json:"null,omitempty"`
}

// GraphState adalah keadaan penelusuran graf pada satu langkah.
type GraphState struct {
	Cur      string   `json:"cur,omitempty"`      // simpul yang sedang diproses
	Visited  []string `json:"visited,omitempty"`  // simpul yang sudah ditandai dikunjungi
	Frontier []string `json:"frontier,omitempty"` // isi antrian/tumpukan (depan/bawah dulu)
}

// GState adalah keadaan satu goroutine: jalan, siap, menunggu, tidur, atau selesai.
type GState struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
	Why   string `json:"why,omitempty"`
}

// ChanState adalah isi satu channel. Items = nilai di buffer, lalu nilai yang dibawa
// pengirim yang sedang menunggu (Senders) pada channel unbuffered.
type ChanState struct {
	ID        string   `json:"id"`
	Name      string   `json:"name,omitempty"`
	Cap       int      `json:"cap"`
	Closed    bool     `json:"closed,omitempty"`
	Items     []string `json:"items"`
	Senders   int      `json:"senders,omitempty"`
	Receivers int      `json:"receivers,omitempty"`
}

// Memory adalah potret variabel fungsi yang sedang berjalan dan objek di heap yang
// dijangkau lewat pointer. Dipakai untuk diagram peti (variabel) dan tali (pointer).
type Memory struct {
	Vars []MemVar `json:"vars"`
	Heap []MemObj `json:"heap,omitempty"`
}

// MemVar adalah satu variabel. Points diisi untuk pointer: nama variabel ("x"),
// field ("p.Age"), atau objek heap ("obj1") yang ditunjuk; "nil" untuk pointer nil.
type MemVar struct {
	Name   string     `json:"name"`
	Kind   string     `json:"kind"` // value | struct | pointer | slice | map | error | func
	Type   string     `json:"type,omitempty"`
	Value  string     `json:"value,omitempty"`
	Fields []MemField `json:"fields,omitempty"`
	Points string     `json:"points,omitempty"`
}

// MemField adalah satu field struct.
type MemField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Points string `json:"points,omitempty"`
}

// MemObj adalah nilai di heap yang hanya bisa dicapai lewat pointer (mis. &Node{...}).
type MemObj struct {
	ID     string     `json:"id"`
	Kind   string     `json:"kind"`
	Type   string     `json:"type,omitempty"`
	Value  string     `json:"value,omitempty"`
	Fields []MemField `json:"fields,omitempty"`
}

// Op adalah satu operasi pada koleksi. A dan B adalah indeks koleksi.
type Op struct {
	Op     OpKind `json:"op"`
	A      int    `json:"a"`
	B      *int   `json:"b,omitempty"`
	Result string `json:"result,omitempty"` // ">", "<", "=" untuk compare
	Cmp    string `json:"cmp,omitempty"`    // operator compare dengan elemen A di kiri: "==", "!=", "<", "<=", ">", ">="
	Value  string `json:"value,omitempty"`  // nilai baru untuk set/append
	Key    string `json:"key,omitempty"`    // key untuk operasi pada map (A tidak dipakai)
}

// Elem adalah variabel for-each yang sedang memegang salinan elemen koleksi ke-Idx
// (for v in arr / for _, v := range arr): perbandingan v == x dicatat sebagai perbandingan elemen.
type Elem struct {
	Idx int
	Val string // nilai elemen saat diikat; variabel yang sudah diubah tidak dianggap elemen lagi
}

// FlipCmp membalik operator perbandingan saat kedua sisinya ditukar (a < b menjadi b > a).
func FlipCmp(op string) string {
	switch op {
	case "<":
		return ">"
	case ">":
		return "<"
	case "<=":
		return ">="
	case ">=":
		return "<="
	}
	return op
}

// Collection adalah koleksi yang digambar sebagai barisan elemen (slice/array) atau loker (map).
type Collection struct {
	Kind  string   `json:"kind"` // slice | array | map
	Items []string `json:"items"`
	Keys  []string `json:"keys,omitempty"` // untuk map: key yang sejajar dengan Items
}
