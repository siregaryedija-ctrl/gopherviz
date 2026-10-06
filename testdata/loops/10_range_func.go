package main

import "fmt"

func countdown(n int) func(yield func(int) bool) {
	return func(yield func(int) bool) {
		for i := n; i > 0; i-- {
			if !yield(i) {
				return
			}
		}
	}
}

func main() {
	for v := range countdown(3) {
		fmt.Println(v)
	}
}
