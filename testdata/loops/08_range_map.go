package main

import "fmt"

func main() {
	ages := map[string]int{"ani": 20, "budi": 25, "citra": 22}
	for name, age := range ages {
		fmt.Println(name, age)
	}
}
