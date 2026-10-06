package main

import "fmt"

func dayName(day int) string {
	switch day {
	case 1, 7:
		return "akhir pekan"
	case 2, 3, 4, 5, 6:
		return "hari kerja"
	default:
		return "bukan hari"
	}
}

func main() {
	fmt.Println(dayName(3))
	fmt.Println(dayName(7))
	fmt.Println(dayName(9))
	temp := 31
	switch {
	case temp > 30:
		fmt.Println("panas")
	case temp > 20:
		fmt.Println("hangat")
	default:
		fmt.Println("dingin")
	}
}
