package main

import "fmt"

func grade(score int) string {
	if score >= 90 {
		return "A"
	} else if score >= 75 {
		return "B"
	} else if score >= 60 {
		return "C"
	} else {
		return "D"
	}
}

func main() {
	for _, s := range []int{95, 80, 40} {
		fmt.Println(s, grade(s))
	}
	x := 7
	if x%2 == 0 {
		fmt.Println("genap")
	}
}
