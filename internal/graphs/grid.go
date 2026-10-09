package graphs

import (
	"regexp"
	"strconv"
	"strings"
)

// Labirin / grid (Fase 11a): BFS atau DFS di grid 2D (grid[r][c]) dengan antrian/tumpukan berisi
// pasangan (r, c) atau rekursi pada (r, c). Analisis sintaks dan pembacaan nilai ada di simulator
// masing-masing; bagian yang sama (tebakan nama, titik awal/tujuan) ada di sini.

// MaxCells membatasi besar grid yang digambar (30×30).
const MaxCells = 900

// GridSpec adalah hasil analisis statis penelusuran grid di blok fokus.
type GridSpec struct {
	Grid     string    // variabel grid
	Row, Col string    // variabel indeks sel aktif (r, c); kosong bila memakai Pair
	Pair     string    // variabel berisi pasangan sel aktif ((r, c), [r, c], struct/object)
	Fields   [2]string // nama field baris/kolom bila Pair berupa struct/object (p.r, p.c); kosong = indeks 0/1
	Frontier string    // variabel antrian/tumpukan
	Kind     string    // "bfs" | "dfs" | "?"
	Visited  []string  // kandidat penanda kunjungan (visited/seen) atau jarak (dist)
}

var (
	gridName  = regexp.MustCompile(`(?i)^(grid|maze|board|peta|labirin|matrix|matriks|field|papan|map|m|g|lab)\w*$`)
	distName  = regexp.MustCompile(`(?i)^(dist|distance|jarak|steps|langkah|level|depth|d)\w*$`)
	nextName  = regexp.MustCompile(`(?i)^(n|new|next|nx|ny|nr|nc|ni|nj)[a-z]?\w*$`)
	startName = regexp.MustCompile(`(?i)^(start|awal|mulai|src|source|begin|asal)$`)
	goalName  = regexp.MustCompile(`(?i)^(goal|end|target|tujuan|finish|exit|dest|destination|akhir|keluar)$`)
)

// IsGridName: nama yang lazim untuk grid labirin (dipakai bila ada beberapa kandidat).
func IsGridName(name string) bool { return gridName.MatchString(name) }

// IsDistName: variabel jarak per sel (dist[r][c]) juga menandai sel yang sudah dikunjungi.
func IsDistName(name string) bool { return distName.MatchString(name) }

// CurScore: pasangan indeks yang lebih mungkin sel aktif (r, c) daripada tetangga (nr, nc).
func CurScore(a, b string) int {
	a, b = strings.TrimPrefix(a, "$"), strings.TrimPrefix(b, "$")
	s := 0
	if nextName.MatchString(a) && len(a) > 1 {
		s -= 2
	}
	if nextName.MatchString(b) && len(b) > 1 {
		s -= 2
	}
	return s
}

// IsStartName / IsGoalName: variabel titik awal / tujuan.
func IsStartName(n string) bool { return startName.MatchString(n) }
func IsGoalName(n string) bool  { return goalName.MatchString(n) }

// StartGoal mencari sel awal ('S') dan tujuan ('G', 'E', 'T') di isi grid.
func StartGoal(rows [][]string) (start, goal []int) {
	for r, row := range rows {
		for c, v := range row {
			switch strings.Trim(v, `'"`) {
			case "S", "s", "A":
				if start == nil {
					start = []int{r, c}
				}
			case "G", "E", "T", "g", "e", "F":
				if goal == nil {
					goal = []int{r, c}
				}
			}
		}
	}
	return start, goal
}

// ParsePair membaca "r,c" / "(r, c)" / "[r c]" sebagai pasangan sel.
func ParsePair(s string) (int, int, bool) {
	s = strings.Trim(strings.TrimSpace(s), "()[]{}")
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	if len(parts) < 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	return a, b, err1 == nil && err2 == nil
}

// InGrid: (r, c) di dalam grid rows×cols.
func InGrid(r, c, rows, cols int) bool { return r >= 0 && c >= 0 && r < rows && c < cols }

// MarkedCells membaca grid penanda (visited[r][c] / dist[r][c]) sebagai teks: sel yang sudah
// dikunjungi. Bool: true. Bilangan: bila ada -1 = semua selain -1; bila hanya 0/1 = yang 1;
// selain itu jarak = bilangan > 0 yang tidak "tak hingga". None/null/inf = belum.
func MarkedCells(vals [][]string) [][2]int {
	hasNeg, onlyBin := false, true
	for _, row := range vals {
		for _, v := range row {
			switch v {
			case "-1":
				hasNeg = true
			case "0", "1", "true", "false", "True", "False":
			default:
				onlyBin = false
			}
		}
	}
	var out [][2]int
	for r, row := range vals {
		for c, v := range row {
			s := strings.ToLower(strings.TrimSpace(v))
			mark := false
			switch s {
			case "true":
				mark = true
			case "false", "none", "null", "nil", "undefined", "", "inf", "+inf", "infinity", "nan":
			default:
				n, err := strconv.ParseFloat(s, 64)
				switch {
				case err != nil:
				case hasNeg:
					mark = n != -1
				case onlyBin:
					mark = n == 1
				default:
					mark = n > 0 && n < 1e8
				}
			}
			if mark {
				out = append(out, [2]int{r, c})
			}
		}
	}
	return out
}

// ---------- pohon (Fase 11b) ----------

// MaxTreeNodes membatasi besar pohon yang digambar.
const MaxTreeNodes = 31

// TreeSpec adalah hasil analisis statis penelusuran pohon: parameter simpul dan field anaknya.
type TreeSpec struct {
	Param  string   // parameter yang memegang simpul, mis. "node"
	Fields []string // field anak yang dipakai pada panggilan rekursif (left, right, ...); urut sumber
	Kids   string   // field daftar anak (children) bila pohon umum
}

var (
	leftName  = regexp.MustCompile(`(?i)^(left|l|kiri|lo|lchild|leftchild)$`)
	childName = regexp.MustCompile(`(?i)^(children|kids|anak|childs|child_nodes|nodes)$`)
	labelName = []string{"val", "value", "data", "key", "item", "v", "nilai", "isi", "name", "label", "Val", "Value", "Data", "Key", "Name"}
)

// IsLeftName / IsChildrenName: field anak kiri / daftar anak.
func IsLeftName(n string) bool     { return leftName.MatchString(n) }
func IsChildrenName(n string) bool { return childName.MatchString(n) }

// LabelFields: field yang dipakai sebagai label simpul, urut prioritas.
func LabelFields() []string { return labelName }
