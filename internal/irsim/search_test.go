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
