package main

import "fmt"

type Shape interface {
	Area() float64
}

type Rect struct {
	W, H float64
}

type Circle struct {
	R float64
}

func (r Rect) Area() float64 { return r.W * r.H }

func (c Circle) Area() float64 { return 3 * c.R * c.R }

func (r Rect) String() string { return fmt.Sprintf("Rect(%vx%v)", r.W, r.H) }

func main() {
	shapes := []Shape{Rect{2, 3}, Circle{1}, Rect{1, 1}}
	total := 0.0
	for _, s := range shapes {
		total += s.Area()
		fmt.Println(s, s.Area())
	}
	fmt.Println("total", total)
}
