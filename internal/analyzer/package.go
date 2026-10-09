package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"gopherviz/internal/project"
)

// Sibling adalah file Go lain di package yang sama dengan file fokus (Fase 10b): fungsi, method,
// tipe, dan variabel globalnya ikut dikenal simulator, tetapi Scene tetap hanya file fokus.
type Sibling struct {
	Name string // relatif terhadap folder file fokus, mis. "utils.go"
	File *ast.File
	text []byte
}

// Files mengembalikan file fokus diikuti file lain di package yang sama.
func (p *Program) Files() []*ast.File {
	out := []*ast.File{p.File}
	for _, s := range p.Siblings {
		out = append(out, s.File)
	}
	return out
}

// sibling: file lain tempat pos berada (nil = file fokus atau tidak diketahui).
func (p *Program) sibling(pos token.Pos) *Sibling {
	if len(p.Siblings) == 0 || !pos.IsValid() {
		return nil
	}
	for _, s := range p.Siblings {
		if s.File.FileStart <= pos && pos <= s.File.FileEnd {
			return s
		}
	}
	return nil
}

// FileOf mengembalikan nama file (relatif) tempat pos berada; "" = file fokus.
func (p *Program) FileOf(pos token.Pos) string {
	if s := p.sibling(pos); s != nil {
		return s.Name
	}
	return ""
}

// WithPackage memuat ulang p bersama file .go lain di folder yang sama dengan nama package
// sama (tanpa _test.go). p tidak diubah; bila tidak ada file lain, p dikembalikan apa adanya.
func WithPackage(p *Program, files project.Files) *Program {
	if files == nil || p.Lang != "" || p.src.shift > 0 || p.File == nil || p.File.Name == nil || p.File.Name.Name == "" {
		return p
	}
	path := p.Result.Scene.File
	dir := filepath.Dir(path)
	var sibs []sibSrc
	for _, f := range files.List(dir) {
		if !strings.HasSuffix(f, ".go") || strings.HasSuffix(f, "_test.go") || strings.EqualFold(filepath.Clean(f), filepath.Clean(path)) {
			continue
		}
		text, ok := files.Read(f)
		if !ok {
			continue
		}
		sibs = append(sibs, sibSrc{path: f, name: project.Rel(dir, f), text: []byte(text)})
		if len(sibs) >= 30 {
			break
		}
	}
	if len(sibs) == 0 {
		return p
	}
	q := loadWith(path, p.src.text, sibs)
	if len(q.Siblings) == 0 {
		return p
	}
	return q
}

type sibSrc struct {
	path, name string
	text       []byte
}

// parseSiblings mem-parse file lain ke fset yang sama. Dilewati: file dengan error, package lain,
// atau yang mendeklarasikan ulang nama yang sudah ada (mis. folder berisi contoh-contoh terpisah
// yang masing-masing punya func main: itu bukan satu program).
func parseSiblings(fset *token.FileSet, focus *ast.File, sibs []sibSrc) []*Sibling {
	declared := topNames(focus)
	var out []*Sibling
	for _, s := range sibs {
		f, err := parser.ParseFile(fset, s.path, s.text, parser.AllErrors|parser.SkipObjectResolution)
		if err != nil || f == nil || f.Name == nil || f.Name.Name != focus.Name.Name {
			continue
		}
		names := topNames(f)
		clash := false
		for n := range names {
			clash = clash || declared[n]
		}
		if clash {
			continue
		}
		for n := range names {
			declared[n] = true
		}
		out = append(out, &Sibling{Name: s.name, File: f, text: s.text})
	}
	return out
}

// topNames: nama tingkat atas yang dideklarasikan file (fungsi, method sebagai Tipe.Nama, tipe,
// variabel, konstanta). init dan "_" boleh berulang di Go.
func topNames(f *ast.File) map[string]bool {
	out := map[string]bool{}
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			name := x.Name.Name
			if name == "init" || name == "_" {
				continue
			}
			if x.Recv != nil && len(x.Recv.List) > 0 {
				t := x.Recv.List[0].Type
				if s, ok := t.(*ast.StarExpr); ok {
					t = s.X
				}
				if id, ok := t.(*ast.Ident); ok {
					name = id.Name + "." + name
				}
			}
			out[name] = true
		case *ast.GenDecl:
			for _, sp := range x.Specs {
				switch s := sp.(type) {
				case *ast.TypeSpec:
					out[s.Name.Name] = true
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if n.Name != "_" {
							out[n.Name] = true
						}
					}
				}
			}
		}
	}
	return out
}
