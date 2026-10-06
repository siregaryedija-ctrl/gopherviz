package main

import "fmt"

func main() {
	grid := [][]int{{1, 2}, {3, 4}}
outer:
	for _, row := range grid {
		for _, v := range row {
			if v == 1 {
				continue outer
			}
			if v == 3 {
				break outer
			}
			fmt.Println(v)
		}
	}
}
