package main

import "fmt"

func main() {
	a := []int{3, 1, 5, 2, 6, 4}
	n := len(a)
	for i := 0; i < n-1; i++ {
		swapped := false
		for j := 0; j < n-1-i; j++ {
			if a[j] > a[j+1] {
				a[j], a[j+1] = a[j+1], a[j]
				swapped = true
			}
		}
		if !swapped {
			break
		}
	}
	fmt.Println(a)
}
