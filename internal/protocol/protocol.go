// Package protocol mendefinisikan pesan JSON lines v2 antara editor dan
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

// Version adalah versi protokol (v2: focusBlock, trace per blok, Scene/Trace v2).
const Version = 2

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
	FocusLoop  string `json:"focusLoop,omitempty"`  // loop terdalam di kursor
	FocusBlock string `json:"focusBlock,omitempty"` // blok terdalam (jenis apa pun) di kursor
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
	// Language adalah languageId editor (mis. "javascript", "typescriptreact", "python").
	// Opsional: bila kosong, bahasa ditebak dari ekstensi file.
	Language string `json:"language,omitempty"`
}

// UpdateResult adalah hasil MethodUpdate.
type UpdateResult struct {
	Scene       scene.Scene        `json:"scene"`
	FocusLoop   string             `json:"focusLoop,omitempty"`
	FocusBlock  string             `json:"focusBlock,omitempty"`
	Diagnostics []scene.Diagnostic `json:"diagnostics"`
}

// TraceParams adalah parameter MethodTrace. BlockID boleh ID blok apa pun;
// LoopID dipertahankan untuk klien v1.
type TraceParams struct {
	File    string `json:"file"`
	BlockID string `json:"blockId,omitempty"`
	LoopID  string `json:"loopId,omitempty"`
	// Input adalah isi kotak Input di panel (nilai dipisah koma atau baris baru); kosong = angka acak.
	Input string `json:"input,omitempty"`
	// Seed menentukan angka acak untuk input yang belum diisi (0 = bawaan).
	Seed int64 `json:"seed,omitempty"`
}

// TraceResult adalah hasil MethodTrace.
type TraceResult struct {
	Trace trace.Trace `json:"trace"`
}
