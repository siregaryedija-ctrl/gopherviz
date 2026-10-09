package main

import "fmt"

type cell struct{ r, c int }

func bfs(grid []string) int {
	rows, cols := len(grid), len(grid[0])
	dist := make([][]int, rows)
	for i := range dist {
		dist[i] = make([]int, cols)
		for j := range dist[i] {
			dist[i][j] = -1
		}
	}
	dist[0][0] = 0
	q := []cell{{0, 0}}
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if grid[cur.r][cur.c] == 'G' {
			return dist[cur.r][cur.c]
		}
		for _, d := range dirs {
			nr, nc := cur.r+d[0], cur.c+d[1]
			if nr >= 0 && nr < rows && nc >= 0 && nc < cols && dist[nr][nc] == -1 && grid[nr][nc] != '#' {
				dist[nr][nc] = dist[cur.r][cur.c] + 1
				q = append(q, cell{nr, nc})
			}
		}
	}
	return -1
}

func main() {
	fmt.Println(bfs([]string{"S.#.", "..#.", "#..G"}))
}
