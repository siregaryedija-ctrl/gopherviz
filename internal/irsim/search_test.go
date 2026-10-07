package irsim

import (
	"slices"
	"testing"

	"gopherviz/internal/lang"
	"gopherviz/internal/trace"
)

// TestSearchCompare: variabel elemen for-each (for v in arr) yang dibandingkan dengan target
// dicatat sebagai perbandingan elemen ke-i, lengkap dengan operatornya, di semua bahasa.
func TestSearchCompare(t *testing.T) {
	cases := []struct{ lang, file, src string }{
		{"python", "a.py", "values = [4, 8, 2, 9, 5]\ntarget = 9\nfor v in values:\n    if v == target:\n        break\n"},
		{"python", "b.py", "values = [4, 8, 2, 9, 5]\ntarget = 9\nfor i, v in enumerate(values):\n    if target == v:\n        break\n"},
		{"javascript", "a.js", "const values = [4, 8, 2, 9, 5];\nconst target = 9;\nfor (const v of values) {\n  if (v === target) break;\n}\n"},
		{"javascript", "b.js", "const values = [4, 8, 2, 9, 5];\nconst target = 9;\nfor (const [i, v] of values.entries()) {\n  if (v === target) break;\n}\n"},
		{"typescript", "a.ts", "const values: number[] = [4, 8, 2, 9, 5];\nconst target = 9;\nfor (const v of values) {\n  if (v === target) break;\n}\n"},
		{"php", "a.php", "<?php\n$values = [4, 8, 2, 9, 5];\n$target = 9;\nforeach ($values as $v) {\n  if ($v == $target) break;\n}\n"},
		{"java", "Main.java", "public class Main {\n  public static void main(String[] args) {\n    int[] values = {4, 8, 2, 9, 5};\n    int target = 9;\n    for (int v : values) {\n      if (v == target) break;\n    }\n  }\n}\n"},
		{"cpp", "a.cpp", "#include <vector>\nint main() {\n  std::vector<int> values = {4, 8, 2, 9, 5};\n  int target = 9;\n  for (int v : values) {\n    if (v == target) break;\n  }\n}\n"},
		{"c", "a.c", "int main(void) {\n  int values[] = {4, 8, 2, 9, 5};\n  int target = 9;\n  for (int i = 0; i < 5; i++) {\n    if (values[i] == target) break;\n  }\n  return 0;\n}\n"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			res := lang.Analyze(c.lang, c.file, c.src)
			if len(res.Scene.Loops) == 0 {
				t.Fatalf("tidak ada loop: %+v", res.Diagnostics)
			}
			tr := Trace(res, res.Scene.Loops[0].ID, Options{})
			if tr.Mode != trace.ModeSimulated || tr.Collection == nil {
				t.Fatalf("mode %s, koleksi %+v, peringatan %v", tr.Mode, tr.Collection, tr.Warnings)
			}
			var cmps []trace.Op
			for _, s := range tr.Steps {
				for _, o := range s.Ops {
					if o.Op == trace.OpCompare {
						cmps = append(cmps, o)
					}
				}
			}
			if len(cmps) != 4 {
				t.Fatalf("perbandingan = %+v, ingin 4 (indeks 0..3)", cmps)
			}
			last := cmps[3]
			if last.A != 3 || last.Result != "=" || last.Cmp != "==" || last.Value != "9" {
				t.Errorf("perbandingan terakhir = %+v", last)
			}
			if cmps[0].A != 0 || cmps[0].Result != "<" {
				t.Errorf("perbandingan pertama = %+v", cmps[0])
			}
			if !slices.ContainsFunc(tr.Block.Names, func(n string) bool { return n == "target" || n == "$target" }) {
				t.Errorf("names = %v", tr.Block.Names)
			}
		})
	}
}

// TestSearchBuiltins: fungsi pencarian bawaan direkam sebagai perbandingan per elemen dengan
// urutan pustaka aslinya, dan hasil (stdout) tetap benar.
func TestSearchBuiltins(t *testing.T) {
	lin := []int{0, 1, 2, 3}
	cases := []struct {
		lang, file, src, out string
		want                 []int
	}{
		{"python", "in.py", "values = [4, 8, 2, 9, 5]\nif 9 in values:\n    print('ada')\n", "ada\n", lin},
		{"python", "index.py", "values = [4, 8, 2, 9, 5]\ndef cari():\n    return values.index(9)\nprint(cari())\n", "3\n", lin},
		{"python", "count.py", "values = [4, 8, 2, 9, 5]\ndef hitung():\n    return values.count(9)\nprint(hitung())\n", "1\n", []int{0, 1, 2, 3, 4}},
		{"python", "bisect.py", "import bisect\na = [1, 3, 5, 7, 9, 11, 13]\ndef cari():\n    return bisect.bisect_left(a, 11)\nprint(cari())\n", "5\n", []int{3, 5, 4}},
		{"python", "bisect2.py", "from bisect import bisect_right\na = [1, 3, 5, 7, 9, 11, 13]\ndef cari():\n    return bisect_right(a, 11)\nprint(cari())\n", "6\n", []int{3, 5, 6}},
		{"javascript", "indexOf.js", "const values = [4, 8, 2, 9, 5];\nfunction cari() { return values.indexOf(9); }\nconsole.log(cari());\n", "3\n", lin},
		{"javascript", "includes.js", "const values = [4, 8, 2, 9, 5];\nfunction ada() { return values.includes(9); }\nconsole.log(ada());\n", "true\n", lin},
		{"javascript", "last.js", "const values = [4, 8, 2, 9, 5];\nfunction cari() { return values.lastIndexOf(9); }\nconsole.log(cari());\n", "3\n", []int{4, 3}},
		{"javascript", "find.js", "const values = [4, 8, 2, 9, 5];\nfunction cari() { return values.find(v => v === 9); }\nconsole.log(cari());\n", "9\n", lin},
		{"javascript", "findIndex.js", "const values = [4, 8, 2, 9, 5];\nfunction cari() { return values.findIndex((v) => v === 9); }\nconsole.log(cari());\n", "3\n", lin},
		{"php", "in_array.php", "<?php\n$values = [4, 8, 2, 9, 5];\nif (in_array(9, $values)) {\n  echo \"ada\\n\";\n}\n", "ada\n", lin},
		{"php", "search.php", "<?php\n$values = [4, 8, 2, 9, 5];\nfunction cari($values) { return array_search(9, $values); }\necho cari($values), \"\\n\";\n", "3\n", lin},
		{"java", "Main.java", "import java.util.*;\npublic class Main {\n  static int cari(int[] a) { return Arrays.binarySearch(a, 11); }\n  public static void main(String[] args) {\n    int[] a = {1, 3, 5, 7, 9, 11, 13};\n    System.out.println(cari(a));\n  }\n}\n", "5\n", []int{3, 5}},
		{"java", "Main.java", "import java.util.*;\npublic class Main {\n  static boolean ada(List<Integer> a) { return a.contains(9); }\n  public static void main(String[] args) {\n    List<Integer> a = new ArrayList<>(List.of(4, 8, 2, 9, 5));\n    System.out.println(ada(a));\n  }\n}\n", "true\n", lin},
		{"cpp", "find.cpp", "#include <iostream>\n#include <vector>\n#include <algorithm>\nint cari(std::vector<int>& v) {\n  auto it = std::find(v.begin(), v.end(), 9);\n  return it - v.begin();\n}\nint main() {\n  std::vector<int> v = {4, 8, 2, 9, 5};\n  std::cout << cari(v) << std::endl;\n}\n", "3\n", lin},
		{"cpp", "lb.cpp", "#include <iostream>\n#include <vector>\n#include <algorithm>\nusing namespace std;\nint cari(vector<int>& a) {\n  return lower_bound(a.begin(), a.end(), 11) - a.begin();\n}\nint main() {\n  vector<int> a = {1, 3, 5, 7, 9, 11, 13};\n  cout << cari(a) << endl;\n}\n", "5\n", []int{3, 5, 4}},
		{"cpp", "findif.cpp", "#include <iostream>\n#include <vector>\n#include <algorithm>\nusing namespace std;\nint cari(vector<int>& v) {\n  auto it = find_if(v.begin(), v.end(), [](int x) { return x == 9; });\n  return it - v.begin();\n}\nint main() {\n  vector<int> v = {4, 8, 2, 9, 5};\n  cout << cari(v) << endl;\n}\n", "3\n", lin},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			res := lang.Analyze(c.lang, c.file, c.src)
			if len(res.Diagnostics) > 0 {
				t.Fatalf("diagnostik: %+v", res.Diagnostics)
			}
			if out, why := Run(res, Options{}); why != "" || out != c.out {
				t.Fatalf("output %q (%s), ingin %q", out, why, c.out)
			}
			blk := ""
			for _, b := range res.Scene.Blocks {
				if b.Kind == "func" && b.Name != "main" || b.Kind == "if" {
					blk = b.ID
					break
				}
			}
			tr := Trace(res, blk, Options{})
			var got []int
			for _, s := range tr.Steps {
				for _, o := range s.Ops {
					if o.Op == trace.OpCompare {
						got = append(got, o.A)
					}
				}
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("blok %s: indeks dicek %v, ingin %v (mode %s, %v)", blk, got, c.want, tr.Mode, tr.Warnings)
			}
		})
	}
}
