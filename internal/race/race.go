// Package race menyiapkan "lomba algoritma" (Fase 12c): fungsi fokus dijalankan langsung dengan
// list angka acak sepanjang N (bukan dari main), supaya dua fungsi bisa dibandingkan dengan input
// yang sama dan jumlah operasinya diukur untuk beberapa ukuran input (grafik Big-O).
//
// Parameter fungsi diisi menurut nama/tipenya: parameter list/array pertama = list acak, batas
// kiri (lo, low, left, start, ...) = 0, batas kanan (hi, high, right, end, ...) = N-1, panjang
// (n, size, len, length, ...) = N, lainnya 0.
package race

import (
	"math/rand"
	"strings"
)

// Spec meminta simulator memanggil fungsi fokus langsung.
type Spec struct {
	N     int   // panjang list acak
	Seed  int64 // benih angka acak (sama untuk semua fungsi yang dilombakan)
	Count bool  // true = hanya mengukur jumlah operasi, tanpa merekam langkah
}

// MaxN membatasi ukuran list.
const MaxN = 64

// List membuat N angka acak 1..99 yang sama untuk benih yang sama.
func List(n int, seed int64) []int {
	if n < 0 {
		n = 0
	}
	if n > MaxN {
		n = MaxN
	}
	r := rand.New(rand.NewSource(seed*7919 + 17))
	out := make([]int, n)
	for i := range out {
		out[i] = 1 + r.Intn(99)
	}
	return out
}

// Role adalah peran satu parameter.
type Role int

const (
	RoleOther Role = iota // 0
	RoleList              // list acak
	RoleLo                // 0
	RoleHi                // N-1
	RoleLen               // N
)

var (
	loNames   = set("lo", "low", "left", "l", "start", "begin", "first", "kiri", "awal", "from", "p", "lb")
	hiNames   = set("hi", "high", "right", "r", "end", "last", "kanan", "akhir", "to", "q", "ub")
	lenNames  = set("n", "size", "len", "length", "count", "jumlah", "panjang", "sz", "num", "cnt")
	listNames = set("arr", "a", "nums", "numbers", "data", "list", "lst", "array", "xs", "items", "values", "vals",
		"angka", "deret", "larik", "daftar", "v", "vec", "seq", "elements", "input", "A", "b")
)

func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// RoleOf menentukan peran parameter bernama name bertipe typ (boleh kosong). haveList = sudah ada
// parameter list sebelumnya (hanya satu list yang diisi).
func RoleOf(name, typ string, haveList bool) Role {
	n := strings.TrimLeft(strings.ToLower(name), "$&*")
	t := strings.ToLower(typ)
	listType := strings.Contains(t, "[") || strings.Contains(t, "list") || strings.Contains(t, "vector") ||
		strings.Contains(t, "array") || strings.Contains(t, "*") || strings.Contains(t, "slice")
	intType := t == "int" || t == "long" || t == "number" || t == "size_t" || t == "integer" || t == "int64"
	switch {
	case !haveList && (listType || !intType && listNames[strings.TrimLeft(name, "$&*")]):
		return RoleList
	case loNames[n]:
		return RoleLo
	case hiNames[n]:
		return RoleHi
	case lenNames[n]:
		return RoleLen
	case !haveList && !intType && t == "":
		return RoleList // parameter pertama tanpa tipe dan bukan nama angka: anggap list
	}
	return RoleOther
}

// Int adalah nilai untuk parameter angka berperan r pada list sepanjang n.
func Int(r Role, n int) int {
	switch r {
	case RoleHi:
		return n - 1
	case RoleLen:
		return n
	}
	return 0
}
