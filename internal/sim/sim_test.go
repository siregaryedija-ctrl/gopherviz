package sim

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"gopherviz/internal/analyzer"
	"gopherviz/internal/trace"
)

var update = flag.Bool("update", false, "perbarui file *.trace.golden.json")

const loopsDir = "../../testdata/loops"

func load(t testing.TB, path string) *analyzer.Program {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return analyzer.Load(filepath.Base(path), bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")))
}

// run mensimulasikan potongan main dan merekam loop pertama (L1).
func run(t *testing.T, body string, opt Options) trace.Trace {
	t.Helper()
	src := "package main\n\nimport \"fmt\"\n\n" + body + "\nvar _ = fmt.Sprint\n"
	p := analyzer.Load("t.go", []byte(src))
	if len(p.Result.Diagnostics) > 0 {
		t.Fatalf("kode test tidak valid: %v", p.Result.Diagnostics)
	}
	return Trace(p, "L1", opt)
}

func output(tr trace.Trace) string {
	var b strings.Builder
	for _, s := range tr.Steps {
		b.WriteString(s.Output)
	}
	return b.String()
}

func count(tr trace.Trace, op trace.OpKind) int {
	n := 0
	for _, s := range tr.Steps {
		for _, o := range s.Ops {
			if o.Op == op {
				n++
			}
		}
	}
	return n
}

func last(tr trace.Trace) trace.Step { return tr.Steps[len(tr.Steps)-1] }

const blocksDir = "../../testdata/blocks"

// TestGolden: setiap loop di testdata/loops dan setiap blok di testdata/blocks.
func TestGolden(t *testing.T) {
	loopFiles, _ := filepath.Glob(filepath.Join(loopsDir, "*.go"))
	var blockFiles []string
	for _, dir := range []string{blocksDir, dataDir, concDir} {
		f, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		blockFiles = append(blockFiles, f...)
	}
	type target struct{ path, id string }
	var targets []target
	for _, path := range loopFiles {
		for _, l := range load(t, path).Result.Scene.Loops {
			targets = append(targets, target{path, l.ID})
		}
	}
	for _, path := range blockFiles {
		for _, b := range load(t, path).Result.Scene.Blocks {
			targets = append(targets, target{path, b.ID})
		}
	}
	for _, tg := range targets {
		path, id := tg.path, tg.id
		p := load(t, path)
		{
			name := fmt.Sprintf("%s.%s", strings.TrimSuffix(filepath.Base(path), ".go"), id)
			l := struct{ ID string }{id}
			t.Run(name, func(t *testing.T) {
				var buf bytes.Buffer
				enc := json.NewEncoder(&buf)
				enc.SetEscapeHTML(false)
				enc.SetIndent("", "  ")
				if err := enc.Encode(Trace(p, l.ID, Options{})); err != nil {
					t.Fatal(err)
				}
				golden := filepath.Join(filepath.Dir(path), name+".trace.golden.json")
				if *update {
					if err := os.WriteFile(golden, buf.Bytes(), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatalf("golden belum ada; jalankan: go test ./internal/sim -update")
				}
				if !bytes.Equal(buf.Bytes(), bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))) {
					t.Errorf("hasil berbeda dari %s (jalankan -update bila disengaja)", filepath.Base(golden))
				}
			})
		}
	}
}

func TestExamples(t *testing.T) {
	t.Run("for klasik mencetak 0 1 2", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(loopsDir, "01_for3.go")), "L1", Options{})
		if tr.Mode != trace.ModeSimulated || output(tr) != "0\n1\n2\n" || last(tr).Reason != trace.ReasonCondFalse {
			t.Errorf("mode %s, output %q, alasan %q", tr.Mode, output(tr), last(tr).Reason)
		}
	})
	t.Run("bubble sort: 12 perbandingan, 5 tukar, keluar lewat break", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(loopsDir, "11_nested_bubble.go")), "L1", Options{})
		if c, s := count(tr, trace.OpCompare), count(tr, trace.OpSwap); c != 12 || s != 5 {
			t.Errorf("compare %d, swap %d", c, s)
		}
		if tr.Collection == nil || strings.Join(tr.Collection.Items, ",") != "3,1,5,2,6,4" {
			t.Errorf("koleksi awal = %+v", tr.Collection)
		}
		var final []string
		breaks := 0
		for _, s := range tr.Steps {
			if s.Items != nil {
				final = s.Items
			}
			if s.Phase == trace.PhaseBreak {
				breaks++
			}
		}
		if strings.Join(final, ",") != "1,2,3,4,5,6" || breaks != 1 {
			t.Errorf("isi akhir %v, break %d", final, breaks)
		}
		if last(tr).Reason != trace.ReasonBreak || last(tr).Loop != "L1" {
			t.Errorf("langkah terakhir = %+v", last(tr))
		}
	})
	t.Run("for tanpa batas keluar lewat break", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(loopsDir, "03_infinite_break.go")), "L1", Options{})
		if output(tr) != "3\n6\n9\n" || last(tr).Reason != trace.ReasonBreak {
			t.Errorf("output %q, alasan %q", output(tr), last(tr).Reason)
		}
	})
	t.Run("range map memberi peringatan urutan acak", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(loopsDir, "08_range_map.go")), "L1", Options{})
		if len(tr.Warnings) == 0 || !strings.Contains(tr.Warnings[0], "map") || len(strings.Fields(output(tr))) != 6 {
			t.Errorf("warnings %v, output %q", tr.Warnings, output(tr))
		}
	})
	t.Run("range iterator hanya merekam body loop sendiri", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(loopsDir, "10_range_func.go")), "L2", Options{})
		for _, s := range tr.Steps {
			if s.Loop != "L2" {
				t.Fatalf("langkah dari loop lain ikut terekam: %+v", s)
			}
		}
		if output(tr) != "3\n2\n1\n" {
			t.Errorf("output %q", output(tr))
		}
	})
	t.Run("continue dan break berlabel", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(loopsDir, "12_labeled.go")), "L1", Options{})
		var notes []string
		for _, s := range tr.Steps {
			if s.Phase == trace.PhaseBreak || s.Phase == trace.PhaseContinue {
				notes = append(notes, s.Note)
			}
		}
		if len(notes) != 2 || !strings.HasPrefix(notes[0], "continue outer") || !strings.HasPrefix(notes[1], "break outer") {
			t.Errorf("cabang = %v", notes)
		}
		if output(tr) != "" || last(tr).Loop != "L1" || last(tr).Reason != trace.ReasonBreak {
			t.Errorf("output %q, terakhir %+v", output(tr), last(tr))
		}
	})
}

func branches(tr trace.Trace) []int {
	var out []int
	for _, s := range tr.Steps {
		if s.Phase == trace.PhaseBranch && s.Branch != nil {
			out = append(out, *s.Branch)
		}
	}
	return out
}

func TestBlocksTrace(t *testing.T) {
	ifs := load(t, filepath.Join(blocksDir, "01_if_chain.go"))
	t.Run("rantai if memilih cabang sesuai nilai", func(t *testing.T) {
		tr := Trace(ifs, "I1", Options{})
		if got := fmt.Sprint(branches(tr)); got != "[0 1 3]" || tr.Block.Kind != "if" {
			t.Errorf("cabang = %s, block %+v", got, tr.Block)
		}
		if last(tr).Reason != trace.ReasonReturn {
			t.Errorf("alasan akhir = %q", last(tr).Reason)
		}
	})
	t.Run("if tanpa else yang kondisinya false melewati blok", func(t *testing.T) {
		tr := Trace(ifs, "I2", Options{})
		if got := fmt.Sprint(branches(tr)); got != "[1]" || last(tr).Reason != trace.ReasonDone {
			t.Errorf("cabang = %s, terakhir %+v", got, last(tr))
		}
	})
	sw := load(t, filepath.Join(blocksDir, "02_switch.go"))
	t.Run("switch dengan tag", func(t *testing.T) {
		if got := fmt.Sprint(branches(Trace(sw, "S1", Options{}))); got != "[1 0 2]" {
			t.Errorf("cabang = %s", got)
		}
	})
	t.Run("switch tanpa tag", func(t *testing.T) {
		tr := Trace(sw, "S2", Options{})
		if got := fmt.Sprint(branches(tr)); got != "[0]" || output(tr) != "panas\n" {
			t.Errorf("cabang = %s, output %q", got, output(tr))
		}
	})
	rec := load(t, filepath.Join(blocksDir, "03_recursion.go"))
	t.Run("rekursi fib: pohon panggilan dan kedalaman", func(t *testing.T) {
		tr := Trace(rec, "F2", Options{})
		calls, maxDepth := 0, 0
		for _, s := range tr.Steps {
			if s.Phase == trace.PhaseCall {
				calls++
			}
			maxDepth = max(maxDepth, s.Depth)
		}
		if calls != 9 || maxDepth != 3 || last(tr).Phase != trace.PhaseReturn || last(tr).Return != "3" || last(tr).Depth != 0 {
			t.Errorf("calls %d, depth %d, terakhir %+v", calls, maxDepth, last(tr))
		}
	})
	t.Run("rekursi faktorial: nilai kembali dari lantai terdalam ke atas", func(t *testing.T) {
		tr := Trace(rec, "F1", Options{})
		var rets []string
		for _, s := range tr.Steps {
			if s.Phase == trace.PhaseReturn {
				rets = append(rets, fmt.Sprintf("%d:%s", s.Depth, s.Return))
			}
		}
		if got := strings.Join(rets, " "); got != "3:1 2:2 1:6 0:24" {
			t.Errorf("return = %s", got)
		}
	})
	t.Run("timeline main: panggilan fungsi lain jadi satu lantai", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(blocksDir, "04_program.go")), "F3", Options{})
		var calls []string
		for _, s := range tr.Steps {
			if s.Phase == trace.PhaseCall || s.Phase == trace.PhaseReturn {
				calls = append(calls, fmt.Sprintf("%s:%s@%d", s.Phase, s.Func, s.Depth))
			}
		}
		want := "call:main@0 call:sumEven@1 return:sumEven@1 return:main@0"
		if got := strings.Join(calls, " "); got != want || output(tr) != "besar 6\n" {
			t.Errorf("panggilan = %s, output %q", got, output(tr))
		}
	})
}

func TestGoSemantics(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"pembagian integer dan float", `func main() {
	for i := 0; i < 1; i++ {
		x := 7 / 2
		y := 7.0 / 2
		var z float64 = 3
		fmt.Println(x, y, z/2)
	}
}`, "3 3.5 1.5\n"},
		{"append memakai array lama bila kapasitas cukup", `func main() {
	a := []int{1, 2, 3}
	for i := 0; i < 1; i++ {
		b := a[:2]
		b = append(b, 9)
		fmt.Println(a, b, len(b), cap(b))
	}
}`, "[1 2 9] [1 2 9] 3 3\n"},
		{"array disalin, slice berbagi", `func main() {
	arr := [3]int{1, 2, 3}
	s := []int{1, 2, 3}
	for i := 0; i < 1; i++ {
		arr2, s2 := arr, s
		arr2[0], s2[0] = 9, 9
		fmt.Println(arr, s)
	}
}`, "[1 2 3] [9 2 3]\n"},
		{"fungsi, rekursi, dan banyak nilai kembali", `func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func divmod(a, b int) (q, r int) {
	q, r = a/b, a%b
	return
}

func main() {
	for i := 5; i <= 7; i++ {
		q, r := divmod(fib(i), 3)
		fmt.Println(fib(i), q, r)
	}
}`, "5 1 2\n8 2 2\n13 4 1\n"},
		{"closure menyimpan state", `func main() {
	count := 0
	inc := func() int { count++; return count }
	for range 3 {
		fmt.Print(inc(), " ")
	}
}`, "1 2 3 "},
		{"string, byte, rune, dan Printf", `func main() {
	s := "Go!"
	for i := 0; i < len(s); i++ {
		fmt.Printf("%d:%c:%v ", i, s[i], string(s[i]) == "o")
	}
}`, "0:G:false 1:o:true 2:!:false "},
		{"map, delete, dan comma-ok", `func main() {
	m := map[string]int{"a": 1}
	for i := 0; i < 1; i++ {
		m["b"] += 5
		delete(m, "a")
		_, ok := m["a"]
		fmt.Println(m, len(m), ok)
	}
}`, "map[b:5] 1 false\n"},
		{"switch dan byte overflow", `func main() {
	var b byte = 250
	for i := 0; i < 3; i++ {
		switch {
		case i == 0:
			fmt.Print("nol ")
		default:
			b += 5
			fmt.Print(b, " ")
		}
	}
}`, "nol 255 4 "},
		{"strings dan strconv", `import "strings"
import "strconv"

func main() {
	for _, w := range strings.Fields("ayo belajar go") {
		fmt.Print(strings.ToUpper(w[:1])+w[1:], strconv.Itoa(len(w)), " ")
	}
}`, "Ayo3 Belajar7 Go2 "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.body
			src := "package main\n\nimport \"fmt\"\n\n"
			if strings.HasPrefix(body, "import") {
				i := strings.Index(body, "\n\n")
				src = "package main\n\nimport \"fmt\"\n" + body[:i] + "\n\n"
				body = body[i+2:]
			}
			p := analyzer.Load("t.go", []byte(src+body))
			if len(p.Result.Diagnostics) > 0 {
				t.Fatalf("kode test tidak valid: %v", p.Result.Diagnostics)
			}
			tr := Trace(p, "L1", Options{})
			if tr.Mode != trace.ModeSimulated {
				t.Fatalf("mode %s: %v", tr.Mode, tr.Warnings)
			}
			if got := output(tr); got != tt.want {
				t.Errorf("output %q, mau %q", got, tt.want)
			}
		})
	}
}

func TestFallbacks(t *testing.T) {
	t.Run("parameter fungsi yang tidak dipanggil: mode struktur", func(t *testing.T) {
		tr := run(t, "func hitung(n int) {\n\tfor i := 0; i < n; i++ {\n\t}\n}\n\nfunc main() {}\n", Options{})
		if tr.Mode != trace.ModeStructural || len(tr.Warnings) == 0 || !strings.Contains(tr.Warnings[0], "parameter") {
			t.Errorf("mode %s, warnings %v", tr.Mode, tr.Warnings)
		}
	})
	t.Run("fungsi dipanggil dari main memakai argumen aslinya", func(t *testing.T) {
		tr := run(t, "func hitung(n int) {\n\tfor i := 0; i < n; i++ {\n\t\tfmt.Print(i)\n\t}\n}\n\nfunc main() {\n\thitung(2)\n\thitung(1)\n}\n", Options{})
		if tr.Mode != trace.ModeSimulated || output(tr) != "010" || last(tr).Run != 2 {
			t.Errorf("mode %s, output %q, run %d", tr.Mode, output(tr), last(tr).Run)
		}
	})
	t.Run("loop tanpa akhir dipotong", func(t *testing.T) {
		tr := run(t, "func main() {\n\tx := 0\n\tfor {\n\t\tx++\n\t}\n}\n", Options{MaxIter: 10})
		if !tr.Truncated || tr.Mode != trace.ModeSimulated {
			t.Errorf("truncated %v, mode %s", tr.Truncated, tr.Mode)
		}
	})
	t.Run("index di luar batas menjadi langkah panic", func(t *testing.T) {
		tr := run(t, "func main() {\n\ta := []int{1, 2}\n\tfor i := 0; i <= len(a); i++ {\n\t\tfmt.Println(a[i])\n\t}\n}\n", Options{})
		l := last(tr)
		if l.Reason != trace.ReasonPanic || !strings.Contains(l.Note, "index out of range [2] with length 2") || output(tr) != "1\n2\n" {
			t.Errorf("langkah terakhir %+v, output %q", l, output(tr))
		}
	})
	t.Run("input dari luar program: mode struktur", func(t *testing.T) {
		tr := run(t, "import \"os\"\n\nfunc main() {\n\tfor i := 0; i < len(os.Args); i++ {\n\t}\n}\n", Options{})
		if tr.Mode != trace.ModeStructural {
			t.Errorf("mode %s", tr.Mode)
		}
	})
	t.Run("input user diisi angka acak", func(t *testing.T) {
		tr := run(t, "func main() {\n\tvar n int\n\tfmt.Scan(&n)\n\tfor i := 0; i < 2; i++ {\n\t\tfmt.Print(n)\n\t}\n}\n", Options{})
		if tr.Mode != trace.ModeSimulated || output(tr) != "6464" || !strings.Contains(strings.Join(tr.Warnings, " "), "angka acak") {
			t.Errorf("mode %s, output %q, warnings %v", tr.Mode, output(tr), tr.Warnings)
		}
	})
	t.Run("pencarian: variabel range dibandingkan sebagai elemen", func(t *testing.T) {
		tr := run(t, "func main() {\n\tvalues := []int{4, 8, 2, 9, 5}\n\ttarget := 9\n\tfor _, v := range values {\n\t\tif v == target {\n\t\t\tbreak\n\t\t}\n\t}\n}\n", Options{})
		var cmps []trace.Op
		for _, s := range tr.Steps {
			for _, o := range s.Ops {
				if o.Op == trace.OpCompare {
					cmps = append(cmps, o)
				}
			}
		}
		if len(cmps) != 4 || cmps[3].A != 3 || cmps[3].Result != "=" || cmps[3].Cmp != "==" || !slices.Contains(tr.Block.Names, "target") {
			t.Errorf("perbandingan %+v, names %v", cmps, tr.Block.Names)
		}
	})
	t.Run("loop belum lengkap: mode struktur dengan pesan", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(loopsDir, "14_incomplete_noclose.go")), "L1", Options{})
		if tr.Mode != trace.ModeStructural || !strings.Contains(tr.Warnings[0], "belum lengkap") {
			t.Errorf("mode %s, warnings %v", tr.Mode, tr.Warnings)
		}
	})
	t.Run("budget habis sebelum loop: mode struktur", func(t *testing.T) {
		tr := run(t, "func main() {\n\tx := 0\n\tfor x >= 0 {\n\t\tx++\n\t}\n\tfor i := 0; i < 3; i++ {\n\t}\n}\n", Options{})
		tr2 := Trace(analyzer.Load("t.go", []byte("package main\n\nfunc main() {\n\tx := 0\n\tfor x >= 0 {\n\t\tx++\n\t}\n\tfor i := 0; i < 3; i++ {\n\t}\n}\n")), "L2", Options{Budget: 5000})
		if !tr.Truncated || tr2.Mode != trace.ModeStructural {
			t.Errorf("L1 truncated %v; L2 mode %s %v", tr.Truncated, tr2.Mode, tr2.Warnings)
		}
	})
}

const dataDir = "../../testdata/data"

func TestDataStructures(t *testing.T) {
	t.Run("map koleksi: operasi set/delete per key", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(dataDir, "04_map_count.go")), "L1", Options{})
		if tr.Collection == nil || tr.Collection.Kind != "map" {
			t.Fatalf("koleksi = %+v", tr.Collection)
		}
		var sets []string
		for _, s := range tr.Steps {
			for _, o := range s.Ops {
				if o.Op == trace.OpSet {
					sets = append(sets, o.Key+"="+o.Value)
				}
			}
		}
		if got := strings.Join(sets, " "); got != "apel=1 jeruk=1 apel=2 mangga=1 apel=3 jeruk=2" {
			t.Errorf("set = %s", got)
		}
	})
	t.Run("memori: linked list di heap dengan tali Next", func(t *testing.T) {
		p := load(t, filepath.Join(dataDir, "03_linked_list.go"))
		tr := Trace(p, "L2", Options{})
		mem := tr.Steps[0].Mem
		if mem == nil || len(mem.Heap) != 3 {
			t.Fatalf("mem = %+v", mem)
		}
		var head trace.MemVar
		for _, v := range mem.Vars {
			if v.Name == "head" {
				head = v
			}
		}
		if head.Kind != "pointer" || head.Points != "obj1" || mem.Heap[0].Fields[1].Points != "obj2" || mem.Heap[2].Fields[1].Points != "nil" {
			t.Errorf("head %+v, heap %+v", head, mem.Heap)
		}
	})
	t.Run("memori: pointer ke variabel lain", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(dataDir, "02_pointer.go")), "F2", Options{})
		found := false
		for _, s := range tr.Steps {
			if s.Mem == nil {
				continue
			}
			for _, v := range s.Mem.Vars {
				if v.Name == "p" && v.Points == "x" {
					found = true
				}
			}
		}
		if !found {
			t.Error("tidak ada langkah dengan p menunjuk ke x")
		}
	})
	t.Run("defer dan recover tercatat sebagai langkah", func(t *testing.T) {
		tr := Trace(load(t, filepath.Join(dataDir, "05_defer_recover.go")), "F2", Options{})
		var notes []string
		for _, s := range tr.Steps {
			if strings.HasPrefix(s.Note, "menjalankan defer") || strings.HasPrefix(s.Note, "recover()") || strings.Contains(s.Note, "mencari recover") {
				notes = append(notes, s.Note)
			}
		}
		if len(notes) < 4 || !strings.Contains(strings.Join(notes, "|"), "recover() menangkap panic: runtime error: integer divide by zero") {
			t.Errorf("catatan = %v", notes)
		}
	})
}

const concDir = "../../testdata/conc"

func TestConcurrency(t *testing.T) {
	mainOf := func(t *testing.T, name string) *trace.Trace {
		t.Helper()
		p := load(t, filepath.Join(concDir, name))
		for _, b := range p.Result.Scene.Blocks {
			if b.Kind == "func" && b.Name == "main" {
				tr := Trace(p, b.ID, Options{MaxSteps: 5000})
				return &tr
			}
		}
		t.Fatal("main tidak ditemukan")
		return nil
	}
	t.Run("goroutine tercatat per langkah", func(t *testing.T) {
		tr := mainOf(t, "06_pingpong.go")
		gs := map[int]bool{}
		for _, s := range tr.Steps {
			if s.G > 0 {
				gs[s.G] = true
			}
		}
		if !gs[1] || !gs[2] || !gs[3] {
			t.Errorf("goroutine yang tercatat: %v", gs)
		}
		last := tr.Steps[len(tr.Steps)-1]
		if len(last.Gs) != 3 || len(last.Chans) != 3 {
			t.Errorf("potret akhir: gs %+v chans %+v", last.Gs, last.Chans)
		}
	})
	t.Run("deadlock dilaporkan", func(t *testing.T) {
		tr := mainOf(t, "07_deadlock.go")
		last := tr.Steps[len(tr.Steps)-1]
		if last.Reason != trace.ReasonPanic || !strings.Contains(last.Note, "deadlock") {
			t.Errorf("langkah terakhir = %+v", last)
		}
		if !strings.Contains(strings.Join(tr.Warnings, "|"), "deadlock") {
			t.Errorf("peringatan = %v", tr.Warnings)
		}
	})
	t.Run("unbuffered: pengirim menunggu penerima", func(t *testing.T) {
		tr := mainOf(t, "01_unbuffered.go")
		waits := 0
		for _, s := range tr.Steps {
			if strings.Contains(s.Note, "menunggu") {
				waits++
			}
		}
		if waits == 0 {
			t.Error("tidak ada langkah menunggu")
		}
	})
	t.Run("send ke channel tertutup panic", func(t *testing.T) {
		src := `package main

func main() {
	ch := make(chan int, 1)
	close(ch)
	for i := 0; i < 1; i++ {
		ch <- i
	}
}
`
		p := analyzer.Load("x.go", []byte(src))
		tr := Trace(p, "L1", Options{})
		if !strings.Contains(strings.Join(tr.Warnings, "|"), "send on closed channel") {
			t.Errorf("peringatan = %v", tr.Warnings)
		}
	})
}

// TestAgainstGo membandingkan output seluruh program (fokus pada main) dengan `go run`
// sungguhan. Ini bukti terkuat bahwa simulator setia pada aturan Go.
func TestAgainstGo(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh toolchain Go")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tidak ada di PATH")
	}
	var files []string
	for _, dir := range []string{"../../testdata/data", blocksDir, concDir} {
		f, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		files = append(files, f...)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			p := load(t, path)
			mainID := ""
			for _, b := range p.Result.Scene.Blocks {
				if b.Kind == "func" && b.Name == "main" {
					mainID = b.ID
				}
			}
			tr := Trace(p, mainID, Options{MaxSteps: 5000})
			if tr.Mode != trace.ModeSimulated || tr.Truncated {
				t.Fatalf("mode %s, truncated %v: %v", tr.Mode, tr.Truncated, tr.Warnings)
			}
			cmd := exec.Command(goBin, "run", filepath.Base(path))
			cmd.Dir = filepath.Dir(path)
			want, err := cmd.Output()
			var exit *exec.ExitError
			if err != nil && !errors.As(err, &exit) { // program yang deadlock/panic tetap dibandingkan stdout-nya
				t.Fatalf("go run gagal: %v", err)
			}
			if got := output(tr); got != strings.ReplaceAll(string(want), "\r\n", "\n") {
				t.Errorf("output simulator berbeda dari go run\nsimulator:\n%s\ngo run:\n%s", got, want)
			}
		})
	}
}

func FuzzTrace(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join(loopsDir, "*.go"))
	conc, _ := filepath.Glob(filepath.Join(concDir, "*.go"))
	for _, path := range append(files, conc...) {
		src, _ := os.ReadFile(path)
		f.Add(src)
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		p := analyzer.Load("fuzz.go", src)
		for _, l := range p.Result.Scene.Loops {
			tr := Trace(p, l.ID, Options{Budget: 20_000, MaxSteps: 200})
			for _, w := range tr.Warnings {
				if strings.Contains(w, "kesalahan internal") {
					t.Fatalf("%s: %s", l.ID, w)
				}
			}
		}
	})
}

// TestNoGoroutineLeak memastikan goroutine Go milik penjadwal ikut berhenti setelah trace
// selesai, termasuk saat trace dipotong atau program deadlock (gopherviz serve berjalan lama).
func TestNoGoroutineLeak(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(concDir, "*.go"))
	before := runtime.NumGoroutine()
	for i := 0; i < 20; i++ {
		for _, path := range files {
			p := load(t, path)
			for _, b := range p.Result.Scene.Blocks {
				Trace(p, b.ID, Options{MaxSteps: 15}) // sengaja dipotong di tengah jalan
				Trace(p, b.ID, Options{})
			}
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if n := runtime.NumGoroutine(); n > before+2 {
		buf := make([]byte, 1<<16)
		t.Errorf("goroutine bocor: %d sebelum, %d sesudah\n%s", before, n, buf[:runtime.Stack(buf, true)])
	}
}
