package probe

import (
	"slices"
	"sort"
	"testing"
)

func TestIndexes(t *testing.T) {
	a := []int{1, 3, 5, 7, 9, 11, 13}
	cmpTo := func(x int) func(i int) int { return func(i int) int { return a[i] - x } }
	cases := []struct {
		kind Kind
		x    int
		want []int
	}{
		{Linear, 7, []int{0, 1, 2, 3}},
		{Linear, 4, []int{0, 1, 2, 3, 4, 5, 6}},
		{Reverse, 7, []int{6, 5, 4, 3}},
		{All, 7, []int{0, 1, 2, 3, 4, 5, 6}},
		{Classic, 11, []int{3, 5}},
		{Classic, 1, []int{3, 1, 0}},
		{BisectLeft, 11, []int{3, 5, 4}},
		{LowerBound, 11, []int{3, 5, 4}},
		{UpperBound, 11, []int{3, 5, 6}},
		{BisectRight, 11, []int{3, 5, 6}},
	}
	for _, c := range cases {
		if got := Indexes(c.kind, len(a), cmpTo(c.x)); !slices.Equal(got, c.want) {
			t.Errorf("kind %d x %d: %v, ingin %v", c.kind, c.x, got, c.want)
		}
	}
	// indeks terakhir yang diperiksa bisect_left harus konsisten dengan sort.Search asli
	for x := 0; x <= 14; x++ {
		var probed []int
		want := sort.Search(len(a), func(i int) bool { probed = append(probed, i); return a[i] >= x })
		if got := Indexes(BisectLeft, len(a), cmpTo(x)); !slices.Equal(got, probed) {
			t.Errorf("x %d: %v, sort.Search memeriksa %v (hasil %d)", x, got, probed, want)
		}
	}
}
