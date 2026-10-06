package main

import (
	"fmt"
	"time"
)

func main() {
	done := make(chan string)
	go func() {
		time.Sleep(45 * time.Millisecond)
		done <- "beres"
	}()
	for {
		select {
		case msg := <-done:
			fmt.Println("dapat:", msg)
			return
		case <-time.After(10 * time.Millisecond):
			fmt.Println("menunggu...")
		}
	}
}
