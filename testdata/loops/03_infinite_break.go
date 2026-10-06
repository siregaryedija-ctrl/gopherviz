package main

import "fmt"

func main() {
	x := 0
	for {
		x += 3
		if x > 10 {
			break
		}
		fmt.Println(x)
	}
}
