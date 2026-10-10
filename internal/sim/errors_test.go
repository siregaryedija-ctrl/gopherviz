package sim

import (
	"fmt"
	"testing"

	"gopherviz/internal/analyzer"
	"gopherviz/internal/race"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// TestErrorExplain: panic Go menjadi langkah terakhir dengan penjelasan, termasuk panic di fungsi
// yang dipanggil dari blok fokus (tidak sedang direkam) dan rekursi tanpa akhir.
func TestErrorExplain(t *testing.T) {
	cases := []struct {
		name, fn, src string
		kind, value   string
		length        int
	}{
		{"index", "jumlah", `package main

import "fmt"

func jumlah(a []int) int {
	s := 0
	for i := 0; i <= len(a); i++ {
		s += a[i]
	}
	return s
}

func main() { fmt.Println(jumlah([]int{1, 2, 3})) }
`, "index", "3", 3},
		{"zero", "rata", `package main

import "fmt"

func bagi(a, b int) int { return a / b }

func rata(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return bagi(t, len(xs))
}

func main() { fmt.Println(rata(nil)) }
`, "zero", "0", -1},
		{"nilmap", "main", `package main

func main() {
	var m map[string]int
	m["a"] = 1
}
`, "null", "nil", -1},
		{"depth", "turun", `package main

func turun(n int) int { return turun(n-1) + 1 }

func main() { turun(5) }
`, "depth", "200", -1},
		{"user", "main", `package main

func main() {
	panic("saldo kurang")
}
`, "other", "", -1},
		{"deadlock", "main", `package main

func main() {
	ch := make(chan int)
	ch <- 1
}
`, "deadlock", "", -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := analyzer.Load("t.go", []byte(c.src))
			id := ""
			for _, b := range p.Result.Scene.Blocks {
				if b.Kind == scene.BlockFunc && b.Name == c.fn {
					id = b.ID
				}
			}
			tr := Trace(p, id, Options{})
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
		})
	}
}

// TestRace: fungsi fokus Go dipanggil langsung dengan list acak; main tidak dijalankan.
func TestRace(t *testing.T) {
	src := `package main

import "fmt"

func quick(a []int, lo, hi int) {
	if lo >= hi {
		return
	}
	p, i := a[hi], lo
	for j := lo; j < hi; j++ {
		if a[j] < p {
			a[i], a[j] = a[j], a[i]
			i++
		}
	}
	a[i], a[hi] = a[hi], a[i]
	quick(a, lo, i-1)
	quick(a, i+1, hi)
}

func main() {
	a := []int{3, 1, 2}
	quick(a, 0, len(a)-1)
	fmt.Println(a)
}
`
	p := analyzer.Load("t.go", []byte(src))
	id := ""
	for _, b := range p.Result.Scene.Blocks {
		if b.Kind == scene.BlockFunc && b.Name == "quick" {
			id = b.ID
		}
	}
	tr := Trace(p, id, Options{Call: &race.Spec{N: 7, Seed: 5}})
	if tr.Mode != trace.ModeSimulated || tr.Collection == nil || len(tr.Collection.Items) != 7 {
		t.Fatalf("mode %s koleksi %+v peringatan %v", tr.Mode, tr.Collection, tr.Warnings)
	}
	if want := fmt.Sprint(race.List(7, 5)[0]); tr.Collection.Items[0] != want {
		t.Fatalf("items %v, mau mulai %s", tr.Collection.Items, want)
	}
	for _, s := range tr.Steps {
		if s.Output != "" {
			t.Fatalf("main ikut berjalan: output %q", s.Output)
		}
	}
	w := func(n int) int { return Trace(p, id, Options{Call: &race.Spec{N: n, Seed: 5, Count: true}}).Work }
	if w10, w40 := w(10), w(40); w10 <= 0 || w40 <= w10*3 {
		t.Fatalf("operasi n=10: %d, n=40: %d", w10, w40)
	}
}
