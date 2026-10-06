package main

import "fmt"

func main() {
	arr := [3]string{"a", "b", "c"}
	for _, s := range arr {
		fmt.Println(s)
	}
	for i := range &arr {
		arr[i] = "x"
	}
}
