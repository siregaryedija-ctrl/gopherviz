package main

import "fmt"

func isEven(n int) bool {
	return n%2 == 0
}

func sumEven(nums []int) int {
	total := 0
	for _, n := range nums {
		if isEven(n) {
			total += n
		}
	}
	return total
}

func main() {
	nums := []int{1, 2, 3, 4}
	total := sumEven(nums)
	if total > 5 {
		fmt.Println("besar", total)
	} else {
		fmt.Println("kecil", total)
	}
}
