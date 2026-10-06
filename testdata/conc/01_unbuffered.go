package main

import "fmt"

func produce(ch chan int) {
	for i := 1; i <= 3; i++ {
		ch <- i * 10
	}
	close(ch)
}

func main() {
	ch := make(chan int)
	go produce(ch)
	for v := range ch {
		fmt.Println("terima", v)
	}
	fmt.Println("selesai")
}
