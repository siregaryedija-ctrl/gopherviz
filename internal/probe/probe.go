// Package probe menghitung urutan elemen yang diperiksa fungsi pencarian bawaan
// (indexOf, in, std::lower_bound, bisect, Arrays.binarySearch, slices.BinarySearch, ...).
// Simulator memakai urutan ini untuk merekam satu perbandingan per elemen yang dicek,
// sehingga pencarian satu baris tetap terlihat langkah demi langkah seperti loop manual.
package probe

// Kind adalah algoritma pencarian yang ditiru, persis seperti implementasi pustaka standarnya.
type Kind int

const (
	Linear      Kind = iota // dari depan, berhenti di elemen pertama yang sama (index, indexOf, in, find)
	All                     // semua elemen (count)
	Reverse                 // dari belakang, berhenti di elemen pertama yang sama (lastIndexOf)
	LowerBound              // C++ std::lower_bound / binary_search
	UpperBound              // C++ std::upper_bound
	BisectLeft              // Python bisect_left, Go sort.Search / slices.BinarySearch
	BisectRight             // Python bisect_right / bisect
	Classic                 // Java Arrays.binarySearch / Collections.binarySearch (berhenti saat sama)
)

// Indexes mengembalikan indeks elemen yang diperiksa, berurutan. cmp(i) membandingkan
// elemen ke-i dengan target: negatif bila elemen lebih kecil, 0 bila sama, positif bila lebih besar.
func Indexes(kind Kind, n int, cmp func(i int) int) []int {
	var out []int
	switch kind {
	case Linear:
		for i := 0; i < n; i++ {
			out = append(out, i)
			if cmp(i) == 0 {
				break
			}
		}
	case All:
		for i := 0; i < n; i++ {
			out = append(out, i)
		}
	case Reverse:
		for i := n - 1; i >= 0; i-- {
			out = append(out, i)
			if cmp(i) == 0 {
				break
			}
		}
	case LowerBound, UpperBound:
		first, count := 0, n
		for count > 0 {
			step := count / 2
			i := first + step
			out = append(out, i)
			c := cmp(i)
			if c < 0 || kind == UpperBound && c == 0 {
				first, count = i+1, count-step-1
			} else {
				count = step
			}
		}
	case BisectLeft, BisectRight:
		lo, hi := 0, n
		for lo < hi {
			mid := (lo + hi) / 2
			out = append(out, mid)
			c := cmp(mid)
			if c < 0 || kind == BisectRight && c == 0 {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
	case Classic:
		lo, hi := 0, n-1
		for lo <= hi {
			mid := int(uint(lo+hi) >> 1)
			out = append(out, mid)
			c := cmp(mid)
			switch {
			case c < 0:
				lo = mid + 1
			case c > 0:
				hi = mid - 1
			default:
				return out
			}
		}
	}
	return out
}
