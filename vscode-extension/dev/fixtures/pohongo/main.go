package main

import "fmt"

type Node struct {
	Val         int
	Left, Right *Node
}

func inorder(n *Node) {
	if n == nil {
		return
	}
	inorder(n.Left)
	fmt.Print(n.Val, " ")
	inorder(n.Right)
}

func main() {
	root := &Node{Val: 4, Left: &Node{Val: 2, Left: &Node{Val: 1}, Right: &Node{Val: 3}}, Right: &Node{Val: 6}}
	inorder(root)
	fmt.Println()
}
