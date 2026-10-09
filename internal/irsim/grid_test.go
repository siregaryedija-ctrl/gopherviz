package irsim

import (
	"testing"

	"gopherviz/internal/lang"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// TestGridTraversal: BFS/DFS di grid (labirin) dikenali di semua bahasa: grid, titik awal, sel aktif,
// dan sel yang dikunjungi terekam per langkah. Matriks ketetanggaan (adj[node][nb]) tetap graf.
func TestGridTraversal(t *testing.T) {
	cases := []struct {
		lang, file, src, kind string
		minVisited            int
	}{
		{"python", "bfs.py", `from collections import deque

grid = [
    "S..#....",
    ".#.#.##.",
    ".#...#..",
    ".####.#.",
    "......#G",
]

def bfs():
    rows, cols = len(grid), len(grid[0])
    start = (0, 0)
    visited = [[False] * cols for _ in range(rows)]
    visited[0][0] = True
    q = deque([(0, 0, 0)])
    while q:
        r, c, d = q.popleft()
        if grid[r][c] == "G":
            return d
        for dr, dc in [(1, 0), (-1, 0), (0, 1), (0, -1)]:
            nr, nc = r + dr, c + dc
            if 0 <= nr < rows and 0 <= nc < cols and not visited[nr][nc] and grid[nr][nc] != "#":
                visited[nr][nc] = True
                q.append((nr, nc, d + 1))
    return -1

print(bfs())
`, "bfs", 20},
		{"javascript", "bfs.js", `const maze = [
  "S.#..",
  ".##.#",
  "....G",
];
function bfs() {
  const rows = maze.length, cols = maze[0].length;
  const seen = maze.map(r => Array(cols).fill(false));
  const queue = [[0, 0]];
  seen[0][0] = true;
  while (queue.length > 0) {
    const [r, c] = queue.shift();
    if (maze[r][c] === "G") return true;
    for (const [dr, dc] of [[1, 0], [-1, 0], [0, 1], [0, -1]]) {
      const nr = r + dr, nc = c + dc;
      if (nr >= 0 && nr < rows && nc >= 0 && nc < cols && !seen[nr][nc] && maze[nr][nc] !== "#") {
        seen[nr][nc] = true;
        queue.push([nr, nc]);
      }
    }
  }
  return false;
}
console.log(bfs());
`, "bfs", 8},
		{"java", "Labirin.java", `import java.util.*;
public class Labirin {
    static int bfs(int[][] grid) {
        int rows = grid.length, cols = grid[0].length;
        boolean[][] visited = new boolean[rows][cols];
        Queue<int[]> q = new LinkedList<>();
        q.add(new int[]{0, 0});
        visited[0][0] = true;
        int count = 0;
        int[][] dirs = {{1, 0}, {-1, 0}, {0, 1}, {0, -1}};
        while (!q.isEmpty()) {
            int[] cur = q.poll();
            int r = cur[0], c = cur[1];
            count++;
            for (int[] d : dirs) {
                int nr = r + d[0], nc = c + d[1];
                if (nr >= 0 && nr < rows && nc >= 0 && nc < cols && !visited[nr][nc] && grid[nr][nc] == 0) {
                    visited[nr][nc] = true;
                    q.add(new int[]{nr, nc});
                }
            }
        }
        return count;
    }
    public static void main(String[] args) {
        int[][] grid = {{0, 0, 1}, {1, 0, 1}, {0, 0, 0}};
        System.out.println(bfs(grid));
    }
}
`, "bfs", 5},
		{"cpp", "bfs.cpp", `#include <iostream>
#include <queue>
#include <vector>
using namespace std;
int main() {
    vector<string> g = {"S.#", "..#", "#.G"};
    vector<vector<int>> dist(3, vector<int>(3, -1));
    queue<pair<int,int>> q;
    q.push({0, 0});
    dist[0][0] = 0;
    while (!q.empty()) {
        auto [r, c] = q.front();
        q.pop();
        int dr[] = {1, -1, 0, 0}, dc[] = {0, 0, 1, -1};
        for (int k = 0; k < 4; k++) {
            int nr = r + dr[k], nc = c + dc[k];
            if (nr >= 0 && nr < 3 && nc >= 0 && nc < 3 && dist[nr][nc] == -1 && g[nr][nc] != '#') {
                dist[nr][nc] = dist[r][c] + 1;
                q.push({nr, nc});
            }
        }
    }
    cout << dist[2][2] << endl;
    return 0;
}
`, "bfs", 5},
		{"php", "bfs.php", `<?php
function bfs($grid) {
    $rows = count($grid); $cols = count($grid[0]);
    $visited = array_fill(0, $rows, array_fill(0, $cols, false));
    $q = [[0, 0]];
    $visited[0][0] = true;
    $n = 0;
    while (count($q) > 0) {
        [$r, $c] = array_shift($q);
        $n++;
        foreach ([[1, 0], [-1, 0], [0, 1], [0, -1]] as [$dr, $dc]) {
            $nr = $r + $dr; $nc = $c + $dc;
            if ($nr >= 0 && $nr < $rows && $nc >= 0 && $nc < $cols && !$visited[$nr][$nc] && $grid[$nr][$nc] == 0) {
                $visited[$nr][$nc] = true;
                $q[] = [$nr, $nc];
            }
        }
    }
    return $n;
}
echo bfs([[0, 0, 1], [1, 0, 0], [0, 0, 0]]);
`, "bfs", 6},
		{"c", "dfs.c", `#include <stdio.h>
char m[3][4] = {"S.#", "..#", "#.G"};
int seen[3][3];
int dfs(int r, int c) {
    if (r < 0 || r >= 3 || c < 0 || c >= 3 || seen[r][c] || m[r][c] == '#') return 0;
    seen[r][c] = 1;
    if (m[r][c] == 'G') return 1;
    return dfs(r + 1, c) || dfs(r, c + 1) || dfs(r - 1, c) || dfs(r, c - 1);
}
int main(void) {
    printf("%d\n", dfs(0, 0));
    return 0;
}
`, "dfs", 4},
		{"python", "pulau.py", `grid = [
    [1, 1, 0, 0],
    [1, 0, 0, 1],
    [0, 0, 1, 1],
]

def isi(r, c):
    if r < 0 or c < 0 or r >= len(grid) or c >= len(grid[0]) or grid[r][c] != 1:
        return
    grid[r][c] = 2
    isi(r + 1, c)
    isi(r - 1, c)
    isi(r, c + 1)
    isi(r, c - 1)

pulau = 0
for r in range(len(grid)):
    for c in range(len(grid[0])):
        if grid[r][c] == 1:
            pulau += 1
            isi(r, c)
print(pulau)
`, "dfs", 5},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			res := lang.Analyze(c.lang, c.file, c.src)
			if len(res.Diagnostics) > 0 {
				t.Fatalf("diagnostik: %v", res.Diagnostics)
			}
			var tr trace.Trace
			for _, b := range res.Scene.Blocks {
				if b.Kind == scene.BlockFunc && b.Name != "main" && b.Name != "Labirin.main" {
					tr = Trace(res, b.ID, Options{})
					break
				}
			}
			if c.file == "bfs.cpp" {
				tr = Trace(res, res.Scene.Blocks[0].ID, Options{})
			}
			if tr.Mode != trace.ModeSimulated || tr.Grid == nil {
				t.Fatalf("grid tidak terdeteksi: mode %s, warnings %v", tr.Mode, tr.Warnings)
			}
			if tr.Grid.Kind != c.kind || len(tr.Grid.Start) != 2 {
				t.Errorf("kind %q start %v, mau %q", tr.Grid.Kind, tr.Grid.Start, c.kind)
			}
			cur, most := 0, 0
			for _, s := range tr.Steps {
				if s.Cells != nil && len(s.Cells.Cur) == 2 {
					cur++
				}
				if s.Cells != nil && len(s.Cells.Visited) > most {
					most = len(s.Cells.Visited)
				}
			}
			if cur == 0 || most < c.minVisited {
				t.Errorf("sel aktif di %d langkah, dikunjungi maks %d (mau >= %d)", cur, most, c.minVisited)
			}
		})
	}
}
