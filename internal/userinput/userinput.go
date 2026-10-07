// Package userinput memberi nilai pengganti untuk input user (keyboard) saat simulasi.
//
// Simulator tidak pernah menjalankan kode user, jadi tidak ada orang yang mengetik. Keputusan user
// (2026-10-07): bila input tidak diketahui, isi dengan angka acak supaya logikanya tetap bisa
// diputar, karena GopherViz dibuat untuk belajar logika program. Seed-nya tetap, sehingga angka
// yang sama muncul setiap kali kode dianalisis ulang (gambar tidak berubah-ubah saat mengetik).
package userinput

import (
	"math/rand"
	"strconv"
)

// Max adalah batas atas angka acak (0..Max).
const Max = 100

// MaxReads adalah jumlah baris yang "diketik" sebelum input dianggap habis (EOF), supaya loop
// seperti `while (cin >> x)` atau `while sc.hasNext()` tetap berhenti.
const MaxReads = 20

// Warning ditambahkan ke trace saat program membaca input.
const Warning = "input user tidak diketahui, jadi diisi angka acak 0–100 sebagai contoh"

// Source menghasilkan baris input berurutan dengan seed tetap.
type Source struct {
	r     *rand.Rand
	reads int
}

// New membuat sumber baru; setiap simulasi memakai sumber sendiri.
func New() *Source { return &Source{r: rand.New(rand.NewSource(20261007))} }

// Left melaporkan apakah masih ada baris input (untuk hasNext / EOF tanpa membaca).
func (s *Source) Left() bool { return s.reads < MaxReads }

// Next mengembalikan baris berikutnya (angka acak sebagai teks), atau ok=false bila input habis.
func (s *Source) Next() (line string, ok bool) {
	if s.reads >= MaxReads {
		return "", false
	}
	s.reads++
	return strconv.FormatInt(s.r.Int63n(Max+1), 10), true
}
