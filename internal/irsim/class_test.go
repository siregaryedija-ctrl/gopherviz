package irsim

import (
	"strings"
	"testing"

	"gopherviz/internal/lang"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// TestClassTrace: blok class merekam object (dibuat, diberi nama, field berubah) dan
// object self saat method berjalan, di semua bahasa yang punya class/struct.
func TestClassTrace(t *testing.T) {
	cases := []struct {
		lang, file, src, class string
		want                   map[string]string // nama object → "field=nilai,field=nilai" di langkah terakhir
		method                 string            // method yang dipanggil (langkah call dengan Self terisi)
	}{
		{"python", "a.py", `class Akun:
    def __init__(self, nama, saldo):
        self.nama = nama
        self.saldo = saldo
    def setor(self, n):
        self.saldo += n
a = Akun("Ani", 100)
b = Akun("Budi", 50)
a.setor(25)
b.saldo = 7
print(a.saldo)
`, "Akun", map[string]string{"a": "nama=Ani,saldo=125", "b": "nama=Budi,saldo=7"}, "setor"},
		{"javascript", "a.js", `class Akun {
  constructor(nama, saldo) { this.nama = nama; this.saldo = saldo; }
  setor(n) { this.saldo += n; }
}
const a = new Akun("Ani", 100);
const b = new Akun("Budi", 50);
a.setor(25);
b.saldo = 7;
`, "Akun", map[string]string{"a": "nama=Ani,saldo=125", "b": "nama=Budi,saldo=7"}, "setor"},
		{"php", "a.php", `<?php
class Akun {
    public $nama; public $saldo;
    function __construct($nama, $saldo) { $this->nama = $nama; $this->saldo = $saldo; }
    function setor($n) { $this->saldo += $n; }
}
$a = new Akun("Ani", 100);
$b = new Akun("Budi", 50);
$a->setor(25);
$b->saldo = 7;
`, "Akun", map[string]string{"$a": "nama=Ani,saldo=125", "$b": "nama=Budi,saldo=7"}, "setor"},
		{"java", "A.java", `public class A {
    static class Akun {
        String nama; int saldo;
        Akun(String nama, int saldo) { this.nama = nama; this.saldo = saldo; }
        void setor(int n) { saldo += n; }
    }
    public static void main(String[] args) {
        Akun a = new Akun("Ani", 100);
        Akun b = new Akun("Budi", 50);
        a.setor(25);
        b.saldo = 7;
    }
}
`, "Akun", map[string]string{"a": "nama=Ani,saldo=125", "b": "nama=Budi,saldo=7"}, "setor"},
		{"cpp", "a.cpp", `#include <string>
using namespace std;
class Akun {
public:
    string nama; int saldo;
    Akun(string n, int s) : nama(n), saldo(s) {}
    void setor(int n) { saldo += n; }
};
int main() {
    Akun a("Ani", 100);
    Akun* b = new Akun("Budi", 50);
    a.setor(25);
    b->saldo = 7;
    return 0;
}
`, "Akun", map[string]string{"a": "nama=Ani,saldo=125", "b": "nama=Budi,saldo=7"}, "setor"},
		{"c", "a.c", `struct Titik { int x; int y; };
int main() {
    struct Titik p = {1, 2};
    struct Titik q;
    q.x = 5;
    p.x = 10;
    return 0;
}
`, "Titik", map[string]string{"p": "x=10,y=2", "q": "x=5,y=0"}, ""},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			res := lang.Analyze(c.lang, c.file, c.src)
			id := ""
			for _, b := range res.Scene.Blocks {
				if b.Kind == scene.BlockClass && b.Name == c.class {
					id = b.ID
				}
			}
			if id == "" {
				t.Fatalf("blok class %s tidak ada", c.class)
			}
			tr := Trace(res, id, Options{})
			if tr.Mode != trace.ModeSimulated {
				t.Fatalf("mode %s, warnings %v", tr.Mode, tr.Warnings)
			}
			if tr.Class == nil || tr.Class.Name != c.class {
				t.Fatalf("cetak biru class salah: %+v", tr.Class)
			}
			last := tr.Steps[len(tr.Steps)-1]
			if last.Phase != trace.PhaseExit || last.Reason != trace.ReasonDone {
				t.Errorf("langkah terakhir = %s/%s, mau exit/done", last.Phase, last.Reason)
			}
			got := map[string]string{}
			for _, o := range last.Objs {
				var fs []string
				for _, f := range o.Fields {
					fs = append(fs, f.Name+"="+f.Value)
				}
				got[o.Name] = strings.Join(fs, ",")
			}
			for name, want := range c.want {
				if got[name] != want {
					t.Errorf("object %s = %q, mau %q (semua: %v)", name, got[name], want, got)
				}
			}
			if c.method == "" {
				return
			}
			called := false
			for _, s := range tr.Steps {
				if s.Phase == trace.PhaseCall && strings.HasSuffix(s.Func, c.method) {
					called = true
					if s.Self == "" {
						t.Errorf("langkah call %s tanpa Self", s.Func)
					}
				}
			}
			if !called {
				t.Errorf("panggilan method %s tidak terekam", c.method)
			}
		})
	}
}
