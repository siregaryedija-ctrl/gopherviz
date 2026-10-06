package main

import (
	"fmt"
	"sync"
)

func square(id int, results []int, wg *sync.WaitGroup) {
	defer wg.Done()
	results[id] = id * id
}

func main() {
	var wg sync.WaitGroup
	results := make([]int, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go square(i, results, &wg)
	}
	wg.Wait()
	fmt.Println(results)
}
