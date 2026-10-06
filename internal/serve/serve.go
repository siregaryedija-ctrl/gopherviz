// Package serve menjalankan `loopviz serve`: membaca request JSON lines dari
// editor dan membalas satu response per baris (lihat package protocol).
package serve

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"goloopviz/internal/analyzer"
	"goloopviz/internal/protocol"
	"goloopviz/internal/scene"
	"goloopviz/internal/trace"
)

// maxLine adalah ukuran maksimum satu request (isi file Go dikirim utuh).
const maxLine = 32 << 20

// Server menyimpan Scene terakhir per file supaya MethodFocus tidak perlu
// mengirim ulang seluruh teks setiap kali kursor bergerak.
type Server struct {
	version string
	scenes  map[string]scene.Scene
}

// New membuat Server. version adalah versi binary loopviz.
func New(version string) *Server {
	return &Server{version: version, scenes: map[string]scene.Scene{}}
}

// Run memproses request dari r sampai EOF. Hanya response yang ditulis ke w;
// jangan pernah mencetak log ke w karena editor membacanya sebagai JSON.
func (s *Server) Run(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := enc.Encode(s.Handle(line)); err != nil {
			return fmt.Errorf("menulis response: %w", err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("membaca request: %w", err)
	}
	return nil
}

// Handle memproses satu baris request. Tidak pernah panik.
func (s *Server) Handle(line []byte) (resp protocol.Response) {
	var req protocol.Request
	defer func() {
		if r := recover(); r != nil {
			resp = fail(req.ID, fmt.Sprintf("kesalahan internal: %v", r))
		}
	}()
	if err := json.Unmarshal(line, &req); err != nil {
		return fail(0, "request bukan JSON yang valid: "+err.Error())
	}

	switch req.Method {
	case protocol.MethodHello:
		return ok(req.ID, protocol.HelloResult{
			Name: "loopviz", Version: s.version,
			Protocol: protocol.Version, Scene: scene.Version, Trace: trace.Version,
		})

	case protocol.MethodUpdate:
		var p protocol.UpdateParams
		if err := decode(req.Params, &p); err != nil {
			return fail(req.ID, err.Error())
		}
		text := bytes.ReplaceAll([]byte(p.Text), []byte("\r\n"), []byte("\n"))
		res := analyzer.Analyze(p.File, text)
		s.scenes[p.File] = res.Scene
		focus, _ := res.Scene.LoopAt(p.Cursor.Line)
		return ok(req.ID, protocol.UpdateResult{Scene: res.Scene, FocusLoop: focus, Diagnostics: res.Diagnostics})

	case protocol.MethodFocus:
		var p protocol.FocusParams
		if err := decode(req.Params, &p); err != nil {
			return fail(req.ID, err.Error())
		}
		sc, found := s.scenes[p.File]
		if !found {
			return fail(req.ID, "file belum pernah dikirim lewat update: "+p.File)
		}
		focus, _ := sc.LoopAt(p.Cursor.Line)
		return ok(req.ID, protocol.FocusResult{FocusLoop: focus})

	case protocol.MethodClose:
		var p protocol.CloseParams
		if err := decode(req.Params, &p); err != nil {
			return fail(req.ID, err.Error())
		}
		delete(s.scenes, p.File)
		return ok(req.ID, struct{}{})

	case protocol.MethodTrace:
		return fail(req.ID, "trace belum tersedia (direncanakan di Fase 4)")

	default:
		return fail(req.ID, fmt.Sprintf("method tidak dikenal: %q", req.Method))
	}
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return fmt.Errorf("params wajib diisi")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("params tidak valid: %w", err)
	}
	return nil
}

func ok(id int, result any) protocol.Response { return protocol.Response{ID: id, Result: result} }

func fail(id int, msg string) protocol.Response {
	return protocol.Response{ID: id, Error: &protocol.Error{Message: msg}}
}
