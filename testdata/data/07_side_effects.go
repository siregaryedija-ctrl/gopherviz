package main

import "fmt"

var counter = 0

func tick() int {
	counter++
	fmt.Println("tick", counter)
	return counter
}

func main() {
	arr := []int{0, 0, 0, 0, 0}
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	ch <- 9
	for <-ch+0 < 5 {
		fmt.Println("recv")
	}
	if tick()+0 > 5 {
		fmt.Println("big")
	}
	total := 0
	total += tick()
	arr[tick()] = 7
	switch {
	case tick()+0 > 10:
		fmt.Println("no")
	case counter > 0:
		fmt.Println("yes")
	}
	fmt.Println(arr, counter, total, len(ch))
}
