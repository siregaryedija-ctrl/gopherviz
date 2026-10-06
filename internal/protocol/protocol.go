// Package protocol mendefinisikan pesan JSON lines v1 antara editor dan
// `gopherviz serve` (satu objek JSON per baris, lewat stdin/stdout).
//
// Ini KONTRAK. Field baru boleh ditambah; mengubah atau menghapus field berarti
// menaikkan Version dan memperbarui CLI + ekstensi sekaligus.
package protocol

import (
	"encoding/json"

	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// Version adalah versi protokol.
const Version = 1

// Nama method.
const (
	MethodHello  = "hello"  // jabat tangan: cek versi
	MethodUpdate = "update" // editor mengirim isi buffer terbaru
	MethodFocus  = "focus"  // kursor pindah tanpa perubahan teks
	MethodClose  = "close"  // file ditutup, buang cache-nya
	MethodTrace  = "trace"  // editor meminta trace satu loop
)

// HelloParams adalah parameter MethodHello.
type HelloParams struct {
	Client   string `json:"client"`
	Protocol int    `json:"protocol"`
}

// HelloResult adalah hasil MethodHello.
type HelloResult struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
	Scene    int    `json:"scene"`
	Trace    int    `json:"trace"`
}

// FocusParams adalah parameter MethodFocus. File harus sudah dikirim lewat MethodUpdate.
type FocusParams struct {
	File   string `json:"file"`
	Cursor Cursor `json:"cursor"`
}

// FocusResult adalah hasil MethodFocus.
type FocusResult struct {
	FocusLoop string `json:"focusLoop,omitempty"`
}

// CloseParams adalah parameter MethodClose.
type CloseParams struct {
	File string `json:"file"`
}

// Request dikirim editor ke gopherviz.
type Request struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response dikirim gopherviz ke editor. Tepat satu dari Result atau Error terisi.
type Response struct {
	ID     int    `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

// Error adalah kegagalan memproses request.
type Error struct {
	Message string `json:"message"`
}

// Cursor adalah posisi kursor, 1-based.
type Cursor struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// UpdateParams adalah parameter MethodUpdate.
type UpdateParams struct {
	File   string `json:"file"`
	Text   string `json:"text"`
	Cursor Cursor `json:"cursor"`
}

// UpdateResult adalah hasil MethodUpdate.
type UpdateResult struct {
	Scene       scene.Scene        `json:"scene"`
	FocusLoop   string             `json:"focusLoop,omitempty"`
	Diagnostics []scene.Diagnostic `json:"diagnostics"`
}

// TraceParams adalah parameter MethodTrace.
type TraceParams struct {
	File   string `json:"file"`
	LoopID string `json:"loopId"`
}

// TraceResult adalah hasil MethodTrace.
type TraceResult struct {
	Trace trace.Trace `json:"trace"`
}
