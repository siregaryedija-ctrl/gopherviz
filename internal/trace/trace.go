// Package trace mendefinisikan kontrak Trace v1: eksekusi satu loop langkah demi
// langkah, yang dianimasikan oleh renderer.
//
// Ini KONTRAK antara core dan tampilan. Field baru boleh ditambah; mengubah atau
// menghapus field berarti menaikkan Version dan memperbarui CLI + ekstensi sekaligus.
package trace

import "goloopviz/internal/scene"

// Version adalah versi skema Trace.
const Version = 1

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
)

// Reason menjelaskan kenapa loop berhenti (hanya pada PhaseExit).
type Reason string

const (
	ReasonCondFalse Reason = "cond-false"
	ReasonRangeDone Reason = "range-done"
	ReasonBreak     Reason = "break"
	ReasonReturn    Reason = "return"
	ReasonGoto      Reason = "goto"
	ReasonStepLimit Reason = "step-limit"
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

// Trace adalah hasil eksekusi satu loop.
type Trace struct {
	Version    int         `json:"version"`
	File       string      `json:"file,omitempty"`
	Mode       Mode        `json:"mode"`
	Loop       LoopInfo    `json:"loop"`
	Steps      []Step      `json:"steps"`
	Collection *Collection `json:"collection,omitempty"`
	Truncated  bool        `json:"truncated"`
	Warnings   []string    `json:"warnings"`
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
	Phase     Phase             `json:"phase"`
	Line      int               `json:"line,omitempty"`
	Iteration *int              `json:"iteration,omitempty"`
	Vars      map[string]string `json:"vars,omitempty"`
	Result    *bool             `json:"result,omitempty"`
	Note      string            `json:"note,omitempty"`
	Output    string            `json:"output,omitempty"`
	Reason    Reason            `json:"reason,omitempty"`
	Ops       []Op              `json:"ops,omitempty"`
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
