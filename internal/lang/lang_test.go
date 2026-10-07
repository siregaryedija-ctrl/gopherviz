package lang

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopherviz/internal/ir"
	"gopherviz/internal/scene"
)

var update = flag.Bool("update", false, "tulis ulang file golden")

var languages = []string{"javascript", "typescript", "python", "php", "cpp", "c", "java"}

func samples(t testing.TB) []string {
	var out []string
	for _, l := range languages {
		f, _ := filepath.Glob(filepath.Join("../../testdata", l, "*.*"))
		for _, p := range f {
			if !strings.Contains(p, ".golden") && Detect(p) == l {
				out = append(out, p)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("tidak ada contoh di testdata")
	}
	return out
}

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden belum ada; jalankan: go test ./internal/lang -update")
	}
	if !bytes.Equal(got, bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))) {
		t.Errorf("%s berbeda dari golden (jalankan -update lalu periksa diff)", filepath.Base(path))
	}
}

// TestGolden: setiap contoh di testdata/<bahasa> → dump IR dan Scene dibandingkan dengan golden.
func TestGolden(t *testing.T) {
	for _, path := range samples(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			language := Detect(path)
			res := Analyze(language, filepath.Base(path), string(src))
			for _, d := range res.Diagnostics {
				t.Errorf("contoh harus bebas error: baris %d: %s", d.Line, d.Message)
			}
			base := strings.TrimSuffix(path, filepath.Ext(path))
			golden(t, base+".ir.golden.txt", []byte(ir.Dump(res.File)))
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(struct {
				Scene       scene.Scene        `json:"scene"`
				Diagnostics []scene.Diagnostic `json:"diagnostics"`
			}{res.Scene, res.Diagnostics}); err != nil {
				t.Fatal(err)
			}
			golden(t, base+".scene.golden.json", buf.Bytes())
		})
	}
}

// TestIncomplete: kode yang sedang diketik tetap menghasilkan blok (hantu) tanpa panik.
func TestIncomplete(t *testing.T) {
	cases := []struct {
		lang, src string
		want      string // "ID:state" yang harus ada
	}{
		{"javascript", "for (let i = 0; i < 3; i++) {\n  console.log(i)\n", "L1:incomplete"},
		{"javascript", "function f(x) {\n  if (x > 1) {\n    return 1\n  }\n", "F1:incomplete"},
		{"javascript", "while (", "L1:incomplete"},
		{"typescript", "function f(a: number): number {\n  for (const x of [1, 2]) {\n", "L1:incomplete"},
		{"python", "for i in range(3):\n", "L1:incomplete"},
		{"python", "def f(n):\n    while n > 0:\n        n -= 1\n", "L1:complete"},
		{"python", "if x\n    y = 1\n", "I1:incomplete"},
		{"php", "<?php\nforeach ($xs as $x) {\n  echo $x;\n", "L1:incomplete"},
		{"cpp", "int main() {\n  for (int i = 0; i < 3; i++) {\n", "L1:incomplete"},
		{"c", "int main(void) {\n  while (1) {\n    break;\n  }\n  return 0;\n}\n", "L1:complete"},
		{"java", "class A {\n  void f() {\n    for (int i = 0; i < 3; i++) {\n", "L1:incomplete"},
		{"java", "class A {\n  void f() {\n    for (String s : xs) { }\n  }\n}\n", "L1:complete"},
	}
	for _, c := range cases {
		res := Analyze(c.lang, "x", c.src)
		var got []string
		for _, b := range res.Scene.Blocks {
			got = append(got, b.ID+":"+string(b.State))
		}
		if !strings.Contains(" "+strings.Join(got, " ")+" ", " "+c.want+" ") {
			t.Errorf("%s %q: blok %v, ingin %s (diag %v)", c.lang, c.src, got, c.want, res.Diagnostics)
		}
		for _, d := range res.Diagnostics {
			if strings.Contains(d.Message, "internal") {
				t.Errorf("%s %q: %s", c.lang, c.src, d.Message)
			}
		}
	}
}

// TestLoopKinds memeriksa klasifikasi loop dan variabelnya per bahasa.
func TestLoopKinds(t *testing.T) {
	cases := []struct {
		lang, src string
		kind      scene.Kind
		vars      string
	}{
		{"javascript", "for (let i = 0, j = 9; i < j; i++, j--) {}", scene.KindFor3, "i j"},
		{"javascript", "for (;;) { break }", scene.KindInfinite, ""},
		{"javascript", "while (true) { break }", scene.KindInfinite, ""},
		{"javascript", "for (const [k, v] of Object.entries(o)) {}", scene.KindRangeMap, "k v"},
		{"javascript", "for (const ch of \"abc\") {}", scene.KindRangeString, "ch"},
		{"python", "for i in range(5):\n    pass\n", scene.KindRangeInt, "i"},
		{"python", "for i, x in enumerate(xs):\n    pass\n", scene.KindRangeSlice, "i x"},
		{"python", "while True:\n    break\n", scene.KindInfinite, ""},
		{"python", "for c in \"hai\":\n    pass\n", scene.KindRangeString, "c"},
		{"php", "<?php foreach ($m as $k => $v) {}", scene.KindRangeMap, "$k $v"},
		{"php", "<?php while (true): break; endwhile;", scene.KindInfinite, ""},
		{"cpp", "void f() { for (auto& [k, v] : m) {} }", scene.KindRangeSlice, "k v"},
		{"c", "void f() { do { } while (x); }", scene.KindDoWhile, ""},
		{"java", "class A { void f() { for (char c : s.toCharArray()) {} } }", scene.KindRangeString, "c"},
		{"java", "class A { void f() { for (var e : m.entrySet()) {} } }", scene.KindRangeMap, "e"},
	}
	for _, c := range cases {
		res := Analyze(c.lang, "x", c.src)
		if len(res.Scene.Loops) != 1 {
			t.Errorf("%s %q: %d loop (diag %v)", c.lang, c.src, len(res.Scene.Loops), res.Diagnostics)
			continue
		}
		l := res.Scene.Loops[0]
		if l.Kind != c.kind || strings.Join(l.Vars, " ") != c.vars {
			t.Errorf("%s %q: jenis %s vars %v, ingin %s %q", c.lang, c.src, l.Kind, l.Vars, c.kind, c.vars)
		}
		if len(res.Diagnostics) > 0 {
			t.Errorf("%s %q: diag %v", c.lang, c.src, res.Diagnostics)
		}
	}
}

// TestDetect memeriksa deteksi bahasa dari ekstensi dan languageId editor.
func TestDetect(t *testing.T) {
	for file, want := range map[string]string{
		"a.js": "javascript", "a.mjs": "javascript", "a.tsx": "typescript", "a.py": "python", "a.php": "php",
		"a.cpp": "cpp", "a.hpp": "cpp", "a.c": "c", "a.h": "c", "A.java": "java", "a.go": "go", "a.txt": "",
	} {
		if got := Detect(file); got != want {
			t.Errorf("Detect(%s) = %q, ingin %q", file, got, want)
		}
	}
	if FromEditor("typescriptreact") != "typescript" || FromEditor("plaintext") != "" {
		t.Error("FromEditor salah")
	}
}

// FuzzAnalyze: parser tidak boleh panik atau macet untuk input apa pun.
func FuzzAnalyze(f *testing.F) {
	for _, p := range samples(f) {
		src, _ := os.ReadFile(p)
		f.Add(Detect(p), string(src))
	}
	f.Fuzz(func(t *testing.T, language, src string) {
		ok := false
		for _, l := range languages {
			if l == language {
				ok = true
			}
		}
		if !ok {
			return
		}
		done := make(chan *Result, 1)
		go func() { done <- Analyze(language, "fuzz", src) }()
		select {
		case res := <-done:
			for _, d := range res.Diagnostics {
				if strings.Contains(d.Message, "internal") {
					t.Fatalf("%s: %s", language, d.Message)
				}
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: parser macet", language)
		}
	})
}
