package main

import "fmt"

func main() {
	for i := range 5 {
		fmt.Println(i)
	}
	for range 3 {
		fmt.Println("halo")
	}
}
