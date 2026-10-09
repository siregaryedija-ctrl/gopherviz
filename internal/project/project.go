// Package project memberi simulator akses BACA ke file lain di proyek user (modul yang
// di-import, file lain di package yang sama). Isi editor yang belum disimpan menang atas
// isi disk. File hanya dibaca dan dianalisis, tidak pernah dijalankan.
package project

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MaxFileSize = 1 << 20 // file yang lebih besar dilewati
	MaxDirFiles = 200     // file per folder yang dipertimbangkan
)

// Files adalah sumber file proyek.
type Files interface {
	// Read mengembalikan isi file di path absolut.
	Read(path string) (string, bool)
	// List mengembalikan path absolut file biasa di folder dir (tidak rekursif), urut nama.
	List(dir string) []string
}

// Disk membaca dari disk dengan lapisan buffer editor (Overlay: path absolut → isi).
type Disk struct {
	Overlay map[string]string
}

func key(path string) string { return strings.ToLower(filepath.Clean(path)) }

// Read: buffer editor dulu, lalu disk (maks MaxFileSize).
func (d Disk) Read(path string) (string, bool) {
	for p, text := range d.Overlay {
		if key(p) == key(path) {
			return text, true
		}
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() > MaxFileSize {
		return "", false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n"), true
}

// List: file di dir (dari disk dan buffer editor yang belum pernah disimpan).
func (d Disk) List(dir string) []string {
	seen := map[string]bool{}
	var out []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || len(out) >= MaxDirFiles {
			continue
		}
		p := filepath.Join(dir, e.Name())
		seen[key(p)] = true
		out = append(out, p)
	}
	for p := range d.Overlay {
		if key(filepath.Dir(p)) == key(dir) && !seen[key(p)] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Rel menampilkan path relatif terhadap folder base dengan garis miring "/" (mis. "lib/utils.js").
func Rel(base, path string) string {
	r, err := filepath.Rel(base, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(r)
}

// Mem adalah sistem file di memori (path → isi), untuk test.
type Mem map[string]string

func (m Mem) Read(path string) (string, bool) {
	for p, text := range m {
		if key(p) == key(path) {
			return text, true
		}
	}
	return "", false
}

func (m Mem) List(dir string) []string {
	var out []string
	for p := range m {
		if key(filepath.Dir(p)) == key(dir) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
