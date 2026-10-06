package main

import (
	"errors"
	"fmt"
	"strconv"
)

var ErrKosong = errors.New("input kosong")

type RangeError struct {
	Value int
}

func (e *RangeError) Error() string {
	return "nilai " + strconv.Itoa(e.Value) + " di luar batas"
}

func parse(s string) (int, error) {
	if s == "" {
		return 0, ErrKosong
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", s, err)
	}
	if n > 100 {
		return 0, &RangeError{Value: n}
	}
	return n, nil
}

func main() {
	for _, s := range []string{"42", "", "abc", "500"} {
		n, err := parse(s)
		if err != nil {
			fmt.Println("error:", err, errors.Is(err, ErrKosong))
			continue
		}
		fmt.Println("ok:", n)
	}
}
