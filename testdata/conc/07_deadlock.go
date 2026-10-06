package main

import "fmt"

func main() {
	ch := make(chan int)
	fmt.Println("mulai")
	ch <- 1
	fmt.Println("tidak pernah sampai")
}
