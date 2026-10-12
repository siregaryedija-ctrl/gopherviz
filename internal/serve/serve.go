// Package serve menjalankan `gopherviz serve`: membaca request JSON lines dari
// editor dan membalas satu response per baris (lihat package protocol).
package serve

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gopherviz/internal/analyzer"
	"gopherviz/internal/lang"
	"gopherviz/internal/project"
	"gopherviz/internal/protocol"
	"gopherviz/internal/race"
	"gopherviz/internal/scene"
	"gopherviz/internal/sim"
	"gopherviz/internal/trace"
	"gopherviz/internal/userinput"
)

// maxLine adalah ukuran maksimum satu request (isi file Go dikirim utuh).
const maxLine = 32 << 20

// Server menyimpan hasil parse terakhir per file supaya MethodFocus dan
// MethodTrace tidak perlu mengirim ulang seluruh teks.
type Server struct {
	version string
	progs   map[string]*analyzer.Program
	texts   map[string]string // isi buffer editor per file: modul yang di-import dibaca dari sini dulu, baru disk
}

// New membuat Server. version adalah versi binary gopherviz.
func New(version string) *Server {
	return &Server{version: version, progs: map[string]*analyzer.Program{}, texts: map[string]string{}}
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
			return fmt.Errorf("writing response: %w", err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("reading request: %w", err)
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
		return fail(0, "request is not valid JSON: "+err.Error())
	}

	switch req.Method {
	case protocol.MethodHello:
		return ok(req.ID, protocol.HelloResult{
			Name: "gopherviz", Version: s.version,
			Protocol: protocol.Version, Scene: scene.Version, Trace: trace.Version,
		})

	case protocol.MethodUpdate:
		var p protocol.UpdateParams
		if err := decode(req.Params, &p); err != nil {
			return fail(req.ID, err.Error())
		}
		text := bytes.ReplaceAll([]byte(p.Text), []byte("\r\n"), []byte("\n"))
		prog := analyzer.LoadLang(p.File, text, lang.FromEditor(p.Language))
		s.progs[p.File] = prog
		s.texts[p.File] = string(text)
		res := prog.Result
		loop, _ := res.Scene.LoopAt(p.Cursor.Line)
		block, _ := res.Scene.BlockAt(p.Cursor.Line)
		return ok(req.ID, protocol.UpdateResult{Scene: res.Scene, FocusLoop: loop, FocusBlock: block, Diagnostics: res.Diagnostics})

	case protocol.MethodFocus:
		var p protocol.FocusParams
		if err := decode(req.Params, &p); err != nil {
			return fail(req.ID, err.Error())
		}
		prog, found := s.progs[p.File]
		if !found {
			return fail(req.ID, "file was never sent with update: "+p.File)
		}
		loop, _ := prog.Result.Scene.LoopAt(p.Cursor.Line)
		block, _ := prog.Result.Scene.BlockAt(p.Cursor.Line)
		return ok(req.ID, protocol.FocusResult{FocusLoop: loop, FocusBlock: block})

	case protocol.MethodClose:
		var p protocol.CloseParams
		if err := decode(req.Params, &p); err != nil {
			return fail(req.ID, err.Error())
		}
		delete(s.progs, p.File)
		delete(s.texts, p.File)
		return ok(req.ID, struct{}{})

	case protocol.MethodTrace:
		var p protocol.TraceParams
		if err := decode(req.Params, &p); err != nil {
			return fail(req.ID, err.Error())
		}
		prog, found := s.progs[p.File]
		if !found {
			return fail(req.ID, "file was never sent with update: "+p.File)
		}
		id := p.BlockID
		if id == "" {
			id = p.LoopID
		}
		return ok(req.ID, protocol.TraceResult{Trace: sim.Trace(prog, id, sim.Options{Input: userinput.Split(p.Input), InputSeed: p.Seed, Files: project.Disk{Overlay: s.texts}})})

	case protocol.MethodRace:
		var p protocol.RaceParams
		if err := decode(req.Params, &p); err != nil {
			return fail(req.ID, err.Error())
		}
		prog, found := s.progs[p.File]
		if !found {
			return fail(req.ID, "file was never sent with update: "+p.File)
		}
		return ok(req.ID, s.race(prog, p))

	default:
		return fail(req.ID, fmt.Sprintf("unknown method: %q", req.Method))
	}
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return fmt.Errorf("params are required")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}

func ok(id int, result any) protocol.Response { return protocol.Response{ID: id, Result: result} }

func fail(id int, msg string) protocol.Response {
	return protocol.Response{ID: id, Error: &protocol.Error{Message: msg}}
}

// race menjalankan lomba algoritma (Fase 12c): setiap fungsi dipanggil dengan list acak yang sama.
func (s *Server) race(prog *analyzer.Program, p protocol.RaceParams) protocol.RaceResult {
	n := p.N
	if n <= 0 {
		n = 8
	}
	sizes := p.Sizes
	if len(sizes) == 0 {
		sizes = []int{5, 10, 20, 40}
	}
	seed := p.Seed
	if seed == 0 {
		seed = 1
	}
	files := project.Disk{Overlay: s.texts}
	out := protocol.RaceResult{Input: race.List(n, seed), Sizes: sizes, Runs: []protocol.RaceRun{}}
	for _, id := range p.Blocks {
		run := protocol.RaceRun{Block: id, Trace: sim.Trace(prog, id, sim.Options{Call: &race.Spec{N: n, Seed: seed}, Files: files})}
		for _, size := range sizes {
			tr := sim.Trace(prog, id, sim.Options{Call: &race.Spec{N: size, Seed: seed, Count: true}, Files: files})
			w := tr.Work
			if len(tr.Warnings) > 0 || w <= 0 {
				w = -1
			}
			run.Work = append(run.Work, w)
		}
		out.Runs = append(out.Runs, run)
	}
	return out
}
