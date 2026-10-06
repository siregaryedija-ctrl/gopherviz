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
	Truncated  bool        `json:"truncated"`
	Warnings   []string    `json:"warnings"`
}

// BlockInfo merangkum blok yang di-trace.
type BlockInfo struct {
	ID      string          `json:"id"`
	Kind    scene.BlockKind `json:"kind"`
	Line    int             `json:"line"`
	EndLine int             `json:"endLine"`
	Name    string          `json:"name,omitempty"`
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
	Items     []string          `json:"items,omitempty"` // isi koleksi terbaru, hanya bila berubah di langkah ini
	Child     string            `json:"child,omitempty"` // ID loop anak pada PhaseEnterChild
}

// Op adalah satu operasi pada koleksi. A dan B adalah indeks koleksi.
type Op struct {
	Op     OpKind `json:"op"`
	A      int    `json:"a"`
	B      *int   `json:"b,omitempty"`
	Result string `json:"result,omitempty"` // ">", "<", "=" untuk compare
	Value  string `json:"value,omitempty"`  // nilai baru untuk set/append
}

// Collection adalah koleksi yang digambar sebagai barisan elemen.
type Collection struct {
	Kind  string   `json:"kind"`
	Items []string `json:"items"`
}
