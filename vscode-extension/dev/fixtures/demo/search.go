package main

import "fmt"

func main() {
	nums := []int{2, 5, 8, 12, 16, 23, 38, 56, 72, 91}
	target := 23
	lo, hi := 0, len(nums)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		if nums[mid] == target {
			fmt.Println("found at", mid)
			break
		} else if nums[mid] < target {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
}
