package main

import (
	"errors"
	"fmt"
)

func order() {
	for i := 1; i <= 3; i++ {
		defer fmt.Println("defer", i)
	}
	fmt.Println("isi fungsi selesai")
}

func safeDivide(a, b int) (result int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("pembagian gagal")
		}
	}()
	result = a / b
	return result, nil
}

func main() {
	order()
	fmt.Println(safeDivide(10, 2))
	fmt.Println(safeDivide(1, 0))
}
