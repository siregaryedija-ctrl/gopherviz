package main

import "fmt"

func main() {
	for i := 0; i < 4; i++ {
		fmt.Println("round", i)
		switch i {
		case 1:
			fmt.Println("one")
		case 2:
			fmt.Println("two")
		default:
			fmt.Println("other")
		}
	}
}
