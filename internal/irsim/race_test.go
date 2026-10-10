package irsim

import (
	"strings"
	"testing"

	"gopherviz/internal/lang"
	"gopherviz/internal/race"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// TestRace: fungsi fokus dipanggil langsung dengan list acak (lomba), di semua bahasa: langkahnya
// terekam dari pemanggilan itu (bukan dari program utama), dan mode hitung mengukur operasi.
func TestRace(t *testing.T) {
	cases := []struct{ lang, file, fn, src string }{
		{"python", "s.py", "bubble", "def bubble(arr):\n    n = len(arr)\n    for i in range(n):\n        for j in range(n - 1 - i):\n            if arr[j] > arr[j + 1]:\n                arr[j], arr[j + 1] = arr[j + 1], arr[j]\n    return arr\n\nprint(bubble([3, 1, 2]))\nif __name__ == '__main__':\n    print(bubble([9, 8]))\n"},
		{"javascript", "s.js", "bubble", "function bubble(a) {\n  for (let i = 0; i < a.length; i++)\n    for (let j = 0; j + 1 < a.length - i; j++)\n      if (a[j] > a[j + 1]) { const t = a[j]; a[j] = a[j + 1]; a[j + 1] = t; }\n  return a;\n}\nconsole.log(bubble([3, 1, 2]));\n"},
		{"java", "S.java", "bubble", "public class S {\n    static void bubble(int[] a) {\n        for (int i = 0; i < a.length; i++)\n            for (int j = 0; j + 1 < a.length - i; j++)\n                if (a[j] > a[j + 1]) { int t = a[j]; a[j] = a[j + 1]; a[j + 1] = t; }\n    }\n    public static void main(String[] args) {\n        bubble(new int[]{3, 1, 2});\n    }\n}\n"},
		{"c", "s.c", "bubble", "void bubble(int a[], int n) {\n    for (int i = 0; i < n; i++)\n        for (int j = 0; j + 1 < n - i; j++)\n            if (a[j] > a[j + 1]) { int t = a[j]; a[j] = a[j + 1]; a[j + 1] = t; }\n}\nint main(void) {\n    int a[3] = {3, 1, 2};\n    bubble(a, 3);\n    return 0;\n}\n"},
		{"cpp", "s.cpp", "bubble", "#include <vector>\nvoid bubble(std::vector<int>& a) {\n    for (size_t i = 0; i < a.size(); i++)\n        for (size_t j = 0; j + 1 < a.size() - i; j++)\n            if (a[j] > a[j + 1]) std::swap(a[j], a[j + 1]);\n}\nint main() {\n    std::vector<int> a = {3, 1, 2};\n    bubble(a);\n}\n"},
		{"php", "s.php", "bubble", "<?php\nfunction bubble($a) {\n    $n = count($a);\n    for ($i = 0; $i < $n; $i++)\n        for ($j = 0; $j + 1 < $n - $i; $j++)\n            if ($a[$j] > $a[$j + 1]) { $t = $a[$j]; $a[$j] = $a[$j + 1]; $a[$j + 1] = $t; }\n    return $a;\n}\nprint_r(bubble([3, 1, 2]));\n"},
		{"python", "q.py", "quick", "def quick(arr, lo, hi):\n    if lo < hi:\n        p = arr[hi]\n        i = lo - 1\n        for j in range(lo, hi):\n            if arr[j] <= p:\n                i += 1\n                arr[i], arr[j] = arr[j], arr[i]\n        arr[i + 1], arr[hi] = arr[hi], arr[i + 1]\n        quick(arr, lo, i)\n        quick(arr, i + 2, hi)\n"},
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
				t.Fatal("fungsi tidak ditemukan")
			}
			tr := Trace(res, id, Options{Call: &race.Spec{N: 6, Seed: 3}})
			if tr.Mode != trace.ModeSimulated || len(tr.Steps) == 0 {
				t.Fatalf("mode %s, peringatan %v", tr.Mode, tr.Warnings)
			}
			if tr.Steps[0].Phase != trace.PhaseCall || tr.Steps[0].Run != 1 {
				t.Fatalf("langkah pertama: %+v", tr.Steps[0])
			}
			want := race.List(6, 3)
			if tr.Collection == nil || len(tr.Collection.Items) != 6 || tr.Collection.Items[0] != itoa(want[0]) {
				t.Fatalf("koleksi %+v, mau %v", tr.Collection, want)
			}
			for _, s := range tr.Steps {
				if s.Run > 1 {
					t.Fatal("fungsi direkam lebih dari sekali (program utama ikut jalan?)")
				}
			}
			// mode hitung: operasi bertambah kira-kira kuadrat (bubble/quick acak tetap > linear)
			w := func(n int) int {
				return Trace(res, id, Options{Call: &race.Spec{N: n, Seed: 3, Count: true}}).Work
			}
			w10, w40 := w(10), w(40)
			if w10 <= 0 || w40 <= w10*4 {
				t.Fatalf("operasi n=10: %d, n=40: %d", w10, w40)
			}
		})
	}
}
