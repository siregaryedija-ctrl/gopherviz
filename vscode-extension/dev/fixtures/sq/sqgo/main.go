package main

import "fmt"

func balik(kata []string) {
	var tumpukan []string
	for _, k := range kata {
		tumpukan = append(tumpukan, k)
	}
	for len(tumpukan) > 0 {
		top := tumpukan[len(tumpukan)-1]
		tumpukan = tumpukan[:len(tumpukan)-1]
		fmt.Println(top)
	}
}

func main() { balik([]string{"a", "b", "c"}) }
