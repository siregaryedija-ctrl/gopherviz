package main

import "fmt"

func main() {
	nums := []int{4, 8, 15, 16}
	sum := 0
	for i, v := range nums {
		if v%2 == 1 {
			continue
		}
		sum += v
		fmt.Println(i, v)
	}
	fmt.Println(sum)
}
