package irsim

import (
	"strings"
	"testing"

	"gopherviz/internal/lang"
)

// TestGraphTraversal: DFS/BFS dikenali di semua bahasa; graf, jenis penelusuran, dan keadaan
// (simpul aktif, visited, antrian/tumpukan) terekam per langkah.
func TestGraphTraversal(t *testing.T) {
	cases := []struct {
		lang, file, src, kind string
		nodes                 int
		frontier              bool
	}{
		{"python", "bfs.py", `from collections import deque
graph = {'A': ['B', 'C'], 'B': ['D'], 'C': ['D'], 'D': []}
def bfs(s):
    seen = {s}
    q = deque([s])
    while q:
        node = q.popleft()
        for nb in graph[node]:
            if nb not in seen:
                seen.add(nb)
                q.append(nb)
bfs('A')
`, "bfs", 4, true},
		{"python", "dfs.py", `graph = {0: [1, 2], 1: [3], 2: [3], 3: []}
def dfs(node, visited):
    visited.add(node)
    for nb in graph[node]:
        if nb not in visited:
            dfs(nb, visited)
dfs(0, set())
`, "dfs", 4, false},
		{"javascript", "bfs.js", `const graph = { a: ['b', 'c'], b: ['d'], c: ['d'], d: [] };
function bfs(start) {
  const visited = new Set([start]);
  const queue = [start];
  while (queue.length > 0) {
    const node = queue.shift();
    for (const nb of graph[node]) {
      if (!visited.has(nb)) { visited.add(nb); queue.push(nb); }
    }
  }
}
bfs('a');
`, "bfs", 4, true},
		{"php", "bfs.php", `<?php
$graph = ['A' => ['B', 'C'], 'B' => ['D'], 'C' => ['D'], 'D' => []];
function bfs($graph, $start) {
  $visited = [$start => true];
  $queue = [$start];
  while (count($queue) > 0) {
    $node = array_shift($queue);
    foreach ($graph[$node] as $nb) {
      if (!isset($visited[$nb])) { $visited[$nb] = true; $queue[] = $nb; }
    }
  }
}
bfs($graph, 'A');
`, "bfs", 4, true},
		{"java", "Main.java", `import java.util.*;
public class Main {
  static void bfs(List<List<Integer>> adj, int s) {
    boolean[] visited = new boolean[adj.size()];
    Queue<Integer> q = new LinkedList<>();
    visited[s] = true;
    q.add(s);
    while (!q.isEmpty()) {
      int node = q.poll();
      for (int nb : adj.get(node)) {
        if (!visited[nb]) { visited[nb] = true; q.add(nb); }
      }
    }
  }
  public static void main(String[] args) {
    List<List<Integer>> adj = new ArrayList<>();
    adj.add(List.of(1, 2)); adj.add(List.of(3)); adj.add(List.of(3)); adj.add(List.of());
    bfs(adj, 0);
  }
}
`, "bfs", 4, true},
		{"cpp", "bfs.cpp", `#include <vector>
#include <queue>
using namespace std;
void bfs(vector<vector<int>>& adj, int s) {
  vector<bool> visited(adj.size(), false);
  queue<int> q;
  visited[s] = true;
  q.push(s);
  while (!q.empty()) {
    int node = q.front();
    q.pop();
    for (int nb : adj[node]) {
      if (!visited[nb]) { visited[nb] = true; q.push(nb); }
    }
  }
}
int main() {
  vector<vector<int>> adj = {{1, 2}, {3}, {3}, {}};
  bfs(adj, 0);
}
`, "bfs", 4, true},
		{"cpp", "dfs.cpp", `#include <vector>
using namespace std;
vector<vector<int>> adj = {{1, 2}, {3}, {3}, {}};
vector<bool> visited(4, false);
void dfs(int node) {
  visited[node] = true;
  for (int nb : adj[node]) {
    if (!visited[nb]) dfs(nb);
  }
}
int main() { dfs(0); }
`, "dfs", 4, false},
		{"c", "dfs.c", `int adj[4][4] = {{0, 1, 1, 0}, {1, 0, 0, 1}, {1, 0, 0, 1}, {0, 1, 1, 0}};
int visited[4];
void dfs(int node) {
  visited[node] = 1;
  for (int nb = 0; nb < 4; nb++) {
    if (adj[node][nb] && !visited[nb]) dfs(nb);
  }
}
int main(void) { dfs(0); return 0; }
`, "dfs", 4, false},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			res := lang.Analyze(c.lang, c.file, c.src)
			if len(res.Diagnostics) > 0 {
				t.Fatalf("diagnostik: %+v", res.Diagnostics)
			}
			blk := ""
			for _, b := range res.Scene.Blocks {
				if b.Kind == "func" && (strings.HasSuffix(b.Name, "bfs") || strings.HasSuffix(b.Name, "dfs")) {
					blk = b.ID
				}
			}
			tr := Trace(res, blk, Options{})
			g := tr.Graph
			if g == nil {
				t.Fatalf("graf tidak terdeteksi (mode %s, %v)", tr.Mode, tr.Warnings)
			}
			if g.Kind != c.kind || len(g.Nodes) != c.nodes || g.Visited == "" {
				t.Errorf("graf = %+v", g)
			}
			sawFrontier, last := false, tr.Steps[len(tr.Steps)-1].Graph
			curs := map[string]bool{}
			for _, s := range tr.Steps {
				if s.Graph != nil && len(s.Graph.Frontier) > 0 {
					sawFrontier = true
				}
				if s.Graph != nil && s.Graph.Cur != "" {
					curs[s.Graph.Cur] = true
				}
			}
			if sawFrontier != c.frontier || last == nil || len(last.Visited) != c.nodes || len(curs) != c.nodes {
				t.Errorf("frontier %v, simpul aktif %v, langkah terakhir %+v", sawFrontier, curs, last)
			}
		})
	}
}
