package main

import "fmt"

type Node struct {
	Val  int
	Next *Node
}

func push(head *Node, v int) *Node {
	return &Node{Val: v, Next: head}
}

func main() {
	var head *Node
	for _, v := range []int{3, 2, 1} {
		head = push(head, v)
	}
	total := 0
	for n := head; n != nil; n = n.Next {
		total += n.Val
		fmt.Print(n.Val, " ")
	}
	fmt.Println("total", total)
}
