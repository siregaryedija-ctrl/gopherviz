package irsim

import (
	"path/filepath"
	"strings"
	"testing"

	"gopherviz/internal/lang"
	"gopherviz/internal/project"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// TestModules: program yang dipecah ke beberapa file. Dari file utama, langkah di file lain
// mencatat Step.File; dari file modul (tanpa program utama), program dijalankan dari file
// yang meng-import-nya (Trace.Entry) sehingga fungsi modul tampil dengan nilai asli.
func TestModules(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	at := func(name string) string { return filepath.Join(dir, filepath.FromSlash(name)) }
	cases := []struct {
		lang        string
		files       map[string]string
		main, mod   string // file utama dan file modul
		modFunc     string // fungsi di modul yang di-trace dari file modul
		wantOut     string // output program (trace fungsi utama)
		mainFuncSrc string // nama fungsi/blok di file utama yang di-trace ("" = blok pertama)
	}{
		{"python", map[string]string{
			"utils.py": "def diskon(h, p):\n    potong = h * p // 100\n    return h - potong\n",
			"main.py":  "import utils\nfrom utils import diskon as d\n\ndef jalan():\n    print(d(200, 50), utils.diskon(100, 10))\n\njalan()\n",
		}, "main.py", "utils.py", "diskon", "100 90\n", "jalan"},
		{"javascript", map[string]string{
			"utils.js": "export function diskon(h, p) {\n  return h - h * p / 100;\n}\nexport default function sapa(n) { return 'Halo ' + n; }\n",
			"main.js":  "import sapa, { diskon } from './utils.js';\nfunction jalan() {\n  console.log(sapa('Ani'), diskon(200, 50));\n}\njalan();\n",
		}, "main.js", "utils.js", "diskon", "Halo Ani 100\n", "jalan"},
		{"javascript", map[string]string{
			"hitung.js": "function kali(a, b) {\n  return a * b;\n}\nmodule.exports = { kali };\n",
			"app.js":    "const { kali } = require('./hitung');\nfunction jalan() {\n  console.log(kali(3, 4));\n}\njalan();\n",
		}, "app.js", "hitung.js", "kali", "12\n", "jalan"},
		{"php", map[string]string{
			"fungsi.php": "<?php\nfunction luas($p, $l) {\n    return $p * $l;\n}\n",
			"index.php":  "<?php\nrequire_once 'fungsi.php';\nfunction jalan() {\n    echo luas(3, 4);\n}\njalan();\n",
		}, "index.php", "fungsi.php", "luas", "12", "jalan"},
		{"c", map[string]string{
			"mat.h":  "int tambah(int a, int b);\n",
			"mat.c":  "#include \"mat.h\"\nint tambah(int a, int b) {\n    return a + b;\n}\n",
			"main.c": "#include <stdio.h>\n#include \"mat.h\"\nint main() {\n    printf(\"%d\\n\", tambah(2, 3));\n    return 0;\n}\n",
		}, "main.c", "mat.c", "tambah", "5\n", "main"},
		{"cpp", map[string]string{
			"akun.h":   "class Akun {\npublic:\n    int saldo;\n    Akun(int s) : saldo(s) {}\n    void setor(int n);\n};\n",
			"akun.cpp": "#include \"akun.h\"\nvoid Akun::setor(int n) {\n    saldo += n;\n}\n",
			"main.cpp": "#include <iostream>\n#include \"akun.h\"\nint main() {\n    Akun a(100);\n    a.setor(50);\n    std::cout << a.saldo << std::endl;\n    return 0;\n}\n",
		}, "main.cpp", "akun.cpp", "Akun.setor", "150\n", "main"},
		{"java", map[string]string{
			"Kalkulator.java": "public class Kalkulator {\n    public static int kuadrat(int x) {\n        return x * x;\n    }\n}\n",
			"Main.java":       "public class Main {\n    public static void main(String[] args) {\n        System.out.println(Kalkulator.kuadrat(7));\n    }\n}\n",
		}, "Main.java", "Kalkulator.java", "Kalkulator.kuadrat", "49\n", "Main.main"},
	}
	for _, c := range cases {
		t.Run(c.lang+"/"+c.main, func(t *testing.T) {
			files := project.Mem{}
			for name, src := range c.files {
				files[at(name)] = src
			}
			find := func(res *lang.Result, name string) string {
				for _, b := range res.Scene.Blocks {
					if b.Kind == scene.BlockFunc && b.Name == name {
						return b.ID
					}
				}
				t.Fatalf("fungsi %s tidak ada di %s: %+v", name, res.Scene.File, res.Scene.Blocks)
				return ""
			}
			// dari file utama: output benar, langkah modul tercatat di file modul
			main := lang.Analyze(c.lang, at(c.main), c.files[c.main])
			tr := Trace(main, find(main, c.mainFuncSrc), Options{Files: files})
			if tr.Mode != trace.ModeSimulated {
				t.Fatalf("mode %s: %v", tr.Mode, tr.Warnings)
			}
			var out strings.Builder
			fromMod := false
			for _, s := range tr.Steps {
				out.WriteString(s.Output)
				fromMod = fromMod || s.File == c.mod
			}
			if out.String() != c.wantOut {
				t.Errorf("output = %q, mau %q (warnings %v)", out.String(), c.wantOut, tr.Warnings)
			}
			if !fromMod || len(tr.Files) < 2 || tr.Files[0] != c.main {
				t.Errorf("langkah di %s tidak tercatat: files %v", c.mod, tr.Files)
			}
			// dari file modul: program dimulai dari file yang meng-import-nya
			mod := lang.Analyze(c.lang, at(c.mod), c.files[c.mod])
			tr = Trace(mod, find(mod, c.modFunc), Options{Files: files})
			if tr.Mode != trace.ModeSimulated || tr.Entry != c.main {
				t.Fatalf("trace modul: mode %s, entry %q (mau %q), warnings %v", tr.Mode, tr.Entry, c.main, tr.Warnings)
			}
			if tr.Steps[0].Phase != trace.PhaseCall || tr.Steps[0].File != c.main {
				t.Errorf("langkah pertama = %s di %q, mau call dari %s", tr.Steps[0].Phase, tr.Steps[0].File, c.main)
			}
		})
	}
}
