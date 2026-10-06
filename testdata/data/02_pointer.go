package main

import "fmt"

func swap(a, b *int) {
	*a, *b = *b, *a
}

type Counter struct {
	n int
}

func main() {
	x, y := 1, 2
	swap(&x, &y)
	fmt.Println(x, y)

	p := &x
	*p = 10
	fmt.Println(x, *p, p == &x)

	q := new(int)
	*q += 5
	fmt.Println(*q)

	c := &Counter{}
	for i := 0; i < 3; i++ {
		c.n += i
	}
	fmt.Println(c.n, *c)

	var nothing *Counter
	fmt.Println(nothing == nil)
}
