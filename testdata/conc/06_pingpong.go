package main

import "fmt"

func player(name string, in <-chan int, out chan<- int, done chan<- bool) {
	for ball := range in {
		fmt.Println(name, ball)
		if ball >= 4 {
			close(out)
			done <- true
			return
		}
		out <- ball + 1
	}
	done <- true
}

func main() {
	ping := make(chan int)
	pong := make(chan int)
	done := make(chan bool)
	go player("ping", ping, pong, done)
	go player("pong", pong, ping, done)
	ping <- 1
	<-done
	<-done
	fmt.Println("permainan selesai")
}
