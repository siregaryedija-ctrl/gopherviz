package irsim

import (
	"strings"
	"testing"

	"gopherviz/internal/lang"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// TestErrorExplain: error yang menghentikan program menjadi langkah terakhir dengan penjelasan
// (jenis, nilai penyebab, panjang koleksi) di semua bahasa. Indeks di luar batas di C/C++ dan
// rekursi tanpa akhir juga terekam (dulu beralih ke mode struktur).
func TestErrorExplain(t *testing.T) {
	cases := []struct {
		lang, file, fn, src string
		kind, value         string
		length              int // -1 = tidak dicek
	}{
		{"python", "a.py", "ambil", "def ambil(arr):\n    for i in range(len(arr) + 1):\n        print(arr[i])\n\nambil([4, 7, 9])\n", "index", "3", 3},
		{"python", "b.py", "bagi", "def bagi(a, b):\n    return a / b\n\nprint(bagi(6, 0))\n", "zero", "0", -1},
		{"python", "c.py", "nilai", "def nilai(d):\n    return d['umur']\n\nprint(nilai({'nama': 'Ani'}))\n", "key", "'umur'", -1},
		{"python", "d.py", "turun", "def turun(n):\n    return turun(n - 1)\n\nturun(3)\n", "depth", "200", -1},
		{"python", "e.py", "panjang", "def panjang(s):\n    return s.upper()\n\nprint(panjang(None))\n", "null", "None", -1},
		{"python", "f.py", "ambil", "def ambil(t):\n    t.pop()\n    t.pop()\n\nambil([1])\n", "empty", "", -1},
		{"python", "g.py", "gabung", "def gabung(n):\n    return 'umur ' + n\n\nprint(gabung(5))\n", "type", "", -1},
		{"javascript", "a.js", "bagi", "function bagi(o) {\n  return o.nilai.x;\n}\nbagi({});\n", "null", "undefined", -1},
		{"java", "A.java", "ambil", "public class A {\n    static int ambil(int[] a) {\n        int s = 0;\n        for (int i = 0; i <= a.length; i++) s += a[i];\n        return s;\n    }\n    public static void main(String[] args) {\n        System.out.println(ambil(new int[]{1, 2}));\n    }\n}\n", "index", "2", 2},
		{"java", "B.java", "bagi", "public class B {\n    static int bagi(int a, int b) {\n        return a / b;\n    }\n    public static void main(String[] args) {\n        System.out.println(bagi(1, 0));\n    }\n}\n", "zero", "0", -1},
		{"cpp", "a.cpp", "jumlah", "#include <iostream>\nint jumlah(int a[], int n) {\n    int s = 0;\n    for (int i = 0; i <= n; i++) s += a[i];\n    return s;\n}\nint main() {\n    int a[3] = {1, 2, 3};\n    std::cout << jumlah(a, 3);\n}\n", "index", "3", 3},
		{"c", "a.c", "f", "int f(int n) {\n    return f(n + 1);\n}\nint main() {\n    return f(0);\n}\n", "depth", "200", -1},
		{"php", "a.php", "bagi", "<?php\nfunction bagi($a, $b) {\n    return intdiv($a, $b);\n}\necho bagi(4, 0);\n", "zero", "0", -1},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			res := lang.Analyze(c.lang, c.file, c.src)
			id := ""
			for _, b := range res.Scene.Blocks {
				if b.Kind == scene.BlockFunc && (b.Name == c.fn || strings.HasSuffix(b.Name, "."+c.fn)) {
					id = b.ID
				}
			}
			if id == "" {
				t.Fatalf("fungsi %s tidak ditemukan", c.fn)
			}
			tr := Trace(res, id, Options{})
			if tr.Mode != trace.ModeSimulated || len(tr.Steps) == 0 {
				t.Fatalf("mode %s, peringatan %v", tr.Mode, tr.Warnings)
			}
			last := tr.Steps[len(tr.Steps)-1]
			if last.Reason != trace.ReasonPanic || last.Err == nil {
				t.Fatalf("langkah terakhir bukan error berpenjelasan: %+v", last)
			}
			e := last.Err
			if e.Kind != c.kind || c.value != "" && e.Value != c.value {
				t.Fatalf("err = %+v, mau kind %s value %s", e, c.kind, c.value)
			}
			if c.length >= 0 && (e.Len == nil || *e.Len != c.length) {
				t.Fatalf("len = %v, mau %d", e.Len, c.length)
			}
			if e.Title == "" || e.Text == "" || e.Hint == "" {
				t.Fatalf("penjelasan kosong: %+v", e)
			}
			if c.kind == "index" && c.lang == "python" && strings.Join(e.Items, ",") != "4,7,9" {
				t.Fatalf("items = %v", e.Items)
			}
		})
	}
}

// TestFatalNotCaught: crash C++ (indeks di luar batas) tidak bisa ditangkap try/catch.
func TestFatalNotCaught(t *testing.T) {
	src := "#include <iostream>\nint main() {\n    int a[2] = {1, 2};\n    try {\n        std::cout << a[5];\n    } catch (...) {\n        std::cout << \"tertangkap\";\n    }\n    return 0;\n}\n"
	res := lang.Analyze("cpp", "t.cpp", src)
	tr := Trace(res, res.Scene.Blocks[0].ID, Options{})
	for _, s := range tr.Steps {
		if strings.Contains(s.Output, "tertangkap") {
			t.Fatal("crash tertangkap catch(...)")
		}
	}
	if n := len(tr.Steps); n == 0 || tr.Steps[n-1].Err == nil || tr.Steps[n-1].Err.Kind != "index" {
		t.Fatalf("langkah terakhir: %+v", tr.Steps)
	}
}
