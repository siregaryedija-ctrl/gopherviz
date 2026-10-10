package explain

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct{ name, msg, kind string }{
		{"panic", "runtime error: index out of range [5] with length 5", "index"},
		{"IndexError", "list index out of range", "index"},
		{"ArrayIndexOutOfBoundsException", "Index 3 out of bounds for length 3", "index"},
		{"out_of_range", "vector::_M_range_check: __n (which is 4) >= this->size() (which is 4)", "index"},
		{"IndexError", "pop from empty list", "empty"},
		{"ZeroDivisionError", "division by zero", "zero"},
		{"ArithmeticException", "/ by zero", "zero"},
		{"panic", "runtime error: integer divide by zero", "zero"},
		{"DivisionByZeroError", "Division by zero", "zero"},
		{"KeyError", "'umur'", "key"},
		{"out_of_range", "map::at", "key"},
		{"TypeError", "Cannot read properties of undefined (reading 'x')", "null"},
		{"AttributeError", "'NoneType' object has no attribute 'upper'", "null"},
		{"panic", "runtime error: invalid memory address or nil pointer dereference", "null"},
		{"NullPointerException", "", "null"},
		{"RecursionError", "maximum recursion depth exceeded", "depth"},
		{"RangeError", "Maximum call stack size exceeded", "depth"},
		{"SegmentationFault", "stack overflow (rekursi terlalu dalam)", "depth"},
		{"TypeError", "can only concatenate str (not \"int\") to str", "type"},
		{"ValueError", "invalid literal for int() with base 10: 'a'", "value"},
		{"fatal error", "all goroutines are asleep - deadlock!", "deadlock"},
		{"panic", "saldo kurang", "other"},
		{"SaldoError", "kurang", "other"},
	}
	for _, c := range cases {
		if got := classify(c.name, c.msg); got != c.kind {
			t.Errorf("classify(%q, %q) = %s, mau %s", c.name, c.msg, got, c.kind)
		}
	}
}

func TestIndexFromMessage(t *testing.T) {
	e := Error("go", "panic", "runtime error: index out of range [5] with length 5", &Ctx{Expr: "arr[i]", Len: NoLen})
	if e.Value != "5" || e.Len == nil || *e.Len != 5 {
		t.Fatalf("%+v", e)
	}
	if want := "Program mengambil indeks 5 dari arr, padahal panjangnya 5: indeks yang ada hanya 0 sampai 4."; e.Text != want {
		t.Fatalf("text = %q", e.Text)
	}
}
