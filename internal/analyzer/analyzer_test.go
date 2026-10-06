package analyzer

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"goloopviz/internal/scene"
)

var update = flag.Bool("update", false, "perbarui file *.scene.golden.json")

const loopsDir = "../../testdata/loops"

func examples(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(loopsDir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("contoh loop tidak ditemukan di %s: %v", loopsDir, err)
	}
	return files
}

func analyzeFile(t testing.TB, path string) Result {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Normalisasi CRLF supaya golden sama di Windows dan Linux.
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	return Analyze(filepath.Base(path), src)
}

func TestGolden(t *testing.T) {
	for _, path := range examples(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			got, err := analyzeFile(t, path).JSON()
			if err != nil {
				t.Fatal(err)
			}
			golden := strings.TrimSuffix(path, ".go") + ".scene.golden.json"
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("golden belum ada; jalankan: go test ./internal/analyzer -update")
			}
			want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
			if !bytes.Equal(got, want) {
				t.Errorf("hasil berbeda dari %s (jalankan -update bila perubahan disengaja)\n%s", filepath.Base(golden), got)
			}
		})
	}
}

// TestKinds memeriksa hal-hal penting secara eksplisit, supaya golden tidak
// diterima begitu saja saat -update.
func TestKinds(t *testing.T) {
	type loop struct {
		kind  scene.Kind
		state scene.State
	}
	c, inc := scene.StateComplete, scene.StateIncomplete
	tests := map[string][]loop{
		"01_for3.go":               {{scene.KindFor3, c}},
		"02_cond.go":               {{scene.KindCond, c}},
		"03_infinite_break.go":     {{scene.KindInfinite, c}},
		"04_range_int.go":          {{scene.KindRangeInt, c}, {scene.KindRangeInt, c}},
		"05_range_slice.go":        {{scene.KindRangeSlice, c}},
		"06_range_array.go":        {{scene.KindRangeArray, c}, {scene.KindRangeArray, c}},
		"07_range_string.go":       {{scene.KindRangeString, c}},
		"08_range_map.go":          {{scene.KindRangeMap, c}},
		"09_range_chan.go":         {{scene.KindRangeChan, c}},
		"10_range_func.go":         {{scene.KindFor3, c}, {scene.KindRangeFunc, c}},
		"11_nested_bubble.go":      {{scene.KindFor3, c}, {scene.KindFor3, c}},
		"12_labeled.go":            {{scene.KindRangeSlice, c}, {scene.KindRangeSlice, c}},
		"13_empty_body.go":         {{scene.KindFor3, c}},
		"14_incomplete_noclose.go": {{scene.KindFor3, inc}},
		"15_incomplete_header.go":  {{scene.KindFor3, inc}},
		"16_snippet.go":            {{scene.KindRangeSlice, c}},
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			res := analyzeFile(t, filepath.Join(loopsDir, name))
			var got []loop
			for _, l := range res.Scene.Loops {
				got = append(got, loop{l.Kind, l.State})
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("loop = %v, mau %v", got, want)
			}
		})
	}
}

func TestDetails(t *testing.T) {
	t.Run("nested punya parent dan children", func(t *testing.T) {
		loops := analyzeFile(t, filepath.Join(loopsDir, "11_nested_bubble.go")).Scene.Loops
		if loops[0].Children[0] != "L2" || loops[1].Parent != "L1" {
			t.Errorf("relasi salah: %+v", loops)
		}
		if loops[0].Header.Text != "for i := 0; i < n-1; i++" || loops[0].Header.Cond != "i < n-1" {
			t.Errorf("header salah: %+v", loops[0].Header)
		}
		if loops[0].Line != 8 || loops[0].EndLine != 19 {
			t.Errorf("baris %d-%d, mau 8-19", loops[0].Line, loops[0].EndLine)
		}
		var ref string
		for _, n := range loops[0].Body {
			if n.Kind == scene.NodeLoop {
				ref = n.Ref
			}
		}
		if ref != "L2" {
			t.Errorf("body L1 tidak merujuk L2: %+v", loops[0].Body)
		}
	})
	t.Run("body kosong adalah slice kosong, bukan null", func(t *testing.T) {
		res := analyzeFile(t, filepath.Join(loopsDir, "13_empty_body.go"))
		b, _ := json.Marshal(res.Scene.Loops[0])
		if !strings.Contains(string(b), `"body":[]`) {
			t.Errorf("JSON body kosong salah: %s", b)
		}
	})
	t.Run("label dan break/continue berlabel", func(t *testing.T) {
		loops := analyzeFile(t, filepath.Join(loopsDir, "12_labeled.go")).Scene.Loops
		if loops[0].Label != "outer" {
			t.Errorf("label = %q", loops[0].Label)
		}
		var labels []string
		for _, n := range loops[1].Body {
			for _, m := range n.Then {
				labels = append(labels, string(m.Kind)+" "+m.Label)
			}
		}
		if want := []string{"continue outer", "break outer"}; !reflect.DeepEqual(labels, want) {
			t.Errorf("cabang = %v, mau %v", labels, want)
		}
	})
	t.Run("potongan tanpa package memakai nomor baris asli", func(t *testing.T) {
		res := analyzeFile(t, filepath.Join(loopsDir, "16_snippet.go"))
		if l := res.Scene.Loops[0]; l.Line != 2 || l.EndLine != 4 || len(res.Diagnostics) != 0 {
			t.Errorf("baris %d-%d, diagnostics %v", l.Line, l.EndLine, res.Diagnostics)
		}
	})
	t.Run("kepala setengah jadi tidak menelan baris berikutnya", func(t *testing.T) {
		l := analyzeFile(t, filepath.Join(loopsDir, "15_incomplete_header.go")).Scene.Loops[0]
		if l.Header.Text != "for i := 0; i <" || l.Header.Post != "" || l.EndLine != l.Line {
			t.Errorf("header = %+v, baris %d-%d", l.Header, l.Line, l.EndLine)
		}
	})
	t.Run("variabel loop", func(t *testing.T) {
		loops := analyzeFile(t, filepath.Join(loopsDir, "05_range_slice.go")).Scene.Loops
		if !reflect.DeepEqual(loops[0].Vars, []string{"i", "v"}) {
			t.Errorf("vars = %v", loops[0].Vars)
		}
	})
}

// TestTyping mensimulasikan user yang mengetik loop huruf demi huruf: setiap
// awalan harus bisa dianalisis tanpa panik, dan begitu "for" muncul, loop-nya
// harus langsung terlihat.
func TestTyping(t *testing.T) {
	const prefix = "package main\n\nfunc main() {\n\t"
	const loop = "for i := 0; i < 3; i++ {\n\t\tprintln(i)\n\t}\n}\n"
	for i := 0; i <= len(loop); i++ {
		res := analyze("ketik.go", []byte(prefix+loop[:i]))
		if i >= 3 && len(res.Scene.Loops) != 1 {
			t.Fatalf("setelah mengetik %q: %d loop, mau 1", loop[:i], len(res.Scene.Loops))
		}
	}
}

func TestLoopAt(t *testing.T) {
	sc := analyzeFile(t, filepath.Join(loopsDir, "11_nested_bubble.go")).Scene
	tests := []struct {
		line int
		want string
	}{{5, ""}, {8, "L1"}, {9, "L1"}, {11, "L2"}, {15, "L2"}, {16, "L1"}, {19, "L1"}, {20, ""}}
	for _, tt := range tests {
		got, _ := sc.LoopAt(tt.line)
		if got != tt.want {
			t.Errorf("LoopAt(%d) = %q, mau %q", tt.line, got, tt.want)
		}
	}
}

func FuzzAnalyze(f *testing.F) {
	for _, path := range examples(f) {
		src, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(src)
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		res := analyze("fuzz.go", src) // tanpa recover: panik harus ketahuan
		for _, l := range res.Scene.Loops {
			if l.Line < 1 || l.EndLine < l.Line {
				t.Errorf("baris tidak valid: %+v", l)
			}
		}
	})
}
