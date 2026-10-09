package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiskOverlay(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.py")
	if err := os.WriteFile(a, []byte("x = 1\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := Disk{Overlay: map[string]string{filepath.Join(dir, "b.py"): "y = 2\n"}}
	if got, ok := d.Read(a); !ok || got != "x = 1\n" {
		t.Errorf("Read(a) = %q, %v", got, ok)
	}
	d.Overlay[a] = "x = 9\n" // buffer editor menang atas disk
	if got, _ := d.Read(a); got != "x = 9\n" {
		t.Errorf("overlay tidak dipakai: %q", got)
	}
	if got := d.List(dir); len(got) != 2 {
		t.Errorf("List = %v, mau a.py dan b.py (b.py hanya ada di buffer)", got)
	}
	if got := Rel(dir, filepath.Join(dir, "lib", "u.js")); got != "lib/u.js" {
		t.Errorf("Rel = %q", got)
	}
}
