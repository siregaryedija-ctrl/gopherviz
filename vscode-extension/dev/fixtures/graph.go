package main

import "fmt"

func bfs(adj [][]int, start int) []int {
	visited := make([]bool, len(adj))
	visited[start] = true
	queue := []int{start}
	order := []int{}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)
		for _, nb := range adj[node] {
			if !visited[nb] {
				visited[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	return order
}

func main() {
	adj := [][]int{{1, 2}, {0, 3, 4}, {0, 5}, {1}, {1, 5}, {2, 4}}
	fmt.Println(bfs(adj, 0))
}
