package main

import "fmt"

func main() {
	words := []string{"apel", "jeruk", "apel", "mangga", "apel", "jeruk"}
	counts := map[string]int{}
	for _, w := range words {
		counts[w]++
	}
	delete(counts, "mangga")
	n, ok := counts["mangga"]
	fmt.Println(counts["apel"], counts["jeruk"], n, ok, len(counts))
	fmt.Println(counts)
}
