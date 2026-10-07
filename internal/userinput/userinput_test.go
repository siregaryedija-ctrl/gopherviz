package userinput

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	got := Split(" 25, Budi Santoso \n\n3,4 ,")
	want := []string{"25", "Budi Santoso", "3", "4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Split = %q", got)
	}
}

func TestNext(t *testing.T) {
	s := New([]string{"7"}, 0)
	if v, ok := s.Next(); v != "7" || !ok || s.UsedRandom() {
		t.Fatalf("nilai user: %q %v", v, ok)
	}
	if _, ok := s.Next(); !ok || !s.UsedRandom() {
		t.Fatal("sesudah nilai user habis seharusnya angka acak")
	}
	for s.Left() {
		s.Next()
	}
	if _, ok := s.Next(); ok || s.Reads() != MaxReads {
		t.Fatalf("seharusnya EOF setelah %d baca, reads %d", MaxReads, s.Reads())
	}
}
