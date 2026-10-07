// Package graphs berisi bagian deteksi DFS/BFS yang sama untuk kedua simulator: tebakan
// peran variabel dari namanya, dan pembentukan trace.Graph dari daftar ketetanggaan.
// Analisis sintaks (graph[node], queue.popleft(), ...) dan pembacaan nilai ada di simulator
// masing-masing (internal/sim untuk Go, internal/irsim untuk bahasa lain).
package graphs

import (
	"regexp"
	"slices"

	"gopherviz/internal/trace"
)

// MaxNodes membatasi besar graf yang digambar.
const MaxNodes = 30

// Spec adalah hasil analisis statis blok fokus.
type Spec struct {
	Graphs   []string          // kandidat variabel graf, urut dari yang paling sering dipakai
	Cur      map[string]string // variabel graf → variabel simpul yang dipakai sebagai indeksnya (graph[node])
	Visited  []string          // kandidat variabel penanda kunjungan
	Frontier string            // variabel antrian/tumpukan
	Kind     string            // "bfs" | "dfs" | "" (ditentukan lagi dari jenis nilainya bila perlu)
}

// Empty: blok ini tidak terlihat seperti penelusuran graf.
func (s *Spec) Empty() bool { return s == nil || len(s.Graphs) == 0 }

var visitedName = regexp.MustCompile(`(?i)visit|seen|kunjung|explored|marked|discovered|dilewati|dilihat|ditandai`)

// IsVisitedName menebak variabel penanda kunjungan dari namanya.
func IsVisitedName(name string) bool { return visitedName.MatchString(name) }

// Queue/stack: operasi pengambilan dari antrian (BFS) atau tumpukan (DFS).
var (
	QueueOps = []string{"popleft", "shift", "poll", "pop_front", "dequeue", "front", "remove", "peekFirst", "pollFirst"}
	StackOps = []string{"pop", "pop_back", "top", "peek", "pollLast", "peekLast"}
)

// Adj adalah graf yang sudah dibaca dari nilai program: simpul dan tetangganya.
type Adj struct {
	Nodes []string
	Next  map[string][]string
}

// Add menambah simpul (bila belum ada).
func (a *Adj) Add(n string) {
	if a.Next == nil {
		a.Next = map[string][]string{}
	}
	if _, ok := a.Next[n]; !ok {
		a.Nodes = append(a.Nodes, n)
		a.Next[n] = nil
	}
}

// Link menambah sisi from → to.
func (a *Adj) Link(from, to string) {
	a.Add(from)
	a.Add(to)
	a.Next[from] = append(a.Next[from], to)
}

// Graph membentuk trace.Graph; nil bila graf kosong atau terlalu besar untuk digambar.
func (a *Adj) Graph(name string) *trace.Graph {
	if len(a.Nodes) < 2 || len(a.Nodes) > MaxNodes {
		return nil
	}
	g := &trace.Graph{Var: name, Nodes: a.Nodes, Edges: [][2]string{}}
	for _, from := range a.Nodes {
		for _, to := range a.Next[from] {
			g.Edges = append(g.Edges, [2]string{from, to})
			if !slices.Contains(a.Next[to], from) {
				g.Directed = true
			}
		}
	}
	if len(g.Edges) == 0 {
		return nil
	}
	return g
}

// Contains: apakah n simpul graf.
func Contains(g *trace.Graph, n string) bool { return g != nil && slices.Contains(g.Nodes, n) }

// Filter menyisakan simpul graf saja (nilai lain, mis. -1, dibuang), urutan tetap.
func Filter(g *trace.Graph, xs []string) []string {
	var out []string
	for _, x := range xs {
		if Contains(g, x) {
			out = append(out, x)
		}
	}
	return out
}
