// Package userinput memberi nilai untuk input user (keyboard) saat simulasi.
//
// Simulator tidak pernah menjalankan kode user, jadi tidak ada orang yang mengetik. Nilainya datang
// dari kotak "Input" di panel (keputusan user 2026-10-07). Bila kotak itu kosong atau isinya
// habis, sisanya diisi angka acak supaya logikanya tetap bisa diputar; tombol acak di panel
// mengganti seed sehingga angka acaknya berubah. Dengan seed yang sama, angka yang sama muncul
// setiap kali kode dianalisis ulang (gambar tidak berubah-ubah saat mengetik).
package userinput

import (
	"math/rand"
	"strconv"
	"strings"
)

// Max adalah batas atas angka acak (0..Max).
const Max = 100

// MaxReads adalah jumlah baris yang "diketik" sebelum input dianggap habis (EOF), supaya loop
// seperti `while (cin >> x)` atau `while sc.hasNext()` tetap berhenti.
const MaxReads = 20

// MaxEOFReads: program yang masih mencoba membaca sebanyak ini SESUDAH input habis mengabaikan
// tanda EOF (mis. menu `do { if (!(cin >> x)) { cin.clear(); continue; } } while (x != 0)`), dan
// akan berputar sampai batas putaran. Simulasi dihentikan dengan penjelasan "input habis"
// (Ended). Program yang benar seperti `while (cin >> x)` hanya sekali membaca saat input habis.
const MaxEOFReads = 3

// Ended adalah nama error penghenti simulasi saat input habis (lihat explain).
const Ended = "InputEnded"

// EndedMsg adalah pesan error penghenti itu.
const EndedMsg = "user input ran out, but the program keeps reading"

// DefaultSeed dipakai bila editor tidak mengirim seed.
const DefaultSeed = 20261007

// Warning ditambahkan ke trace bila ada input yang diisi angka acak.
const Warning = "user input is empty, so random numbers 0–100 are used as an example"

// Source menghasilkan baris input: nilai dari user dulu, lalu angka acak.
type Source struct {
	given  []string
	r      *rand.Rand
	reads  int
	random bool
	// eofReads: berapa kali program mencoba membaca sesudah input habis
	eofReads int
}

// Stalled melaporkan bahwa program terus membaca sesudah input habis (lihat MaxEOFReads).
func (s *Source) Stalled() bool { return s.eofReads >= MaxEOFReads }

// New membuat sumber baru; setiap simulasi memakai sumber sendiri. seed 0 = DefaultSeed.
func New(given []string, seed int64) *Source {
	if seed == 0 {
		seed = DefaultSeed
	}
	return &Source{given: given, r: rand.New(rand.NewSource(seed))}
}

// Split memecah isi kotak input menjadi nilai-nilai: dipisah koma atau baris baru.
func Split(text string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		for _, v := range strings.Split(line, ",") {
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func (s *Source) limit() int { return max(MaxReads, len(s.given)) }

// Left melaporkan apakah masih ada baris input (untuk hasNext / EOF tanpa membaca).
func (s *Source) Left() bool { return s.reads < s.limit() }

// Reads adalah jumlah baris yang sudah dibaca program.
func (s *Source) Reads() int { return s.reads }

// UsedRandom melaporkan apakah ada baris yang diisi angka acak.
func (s *Source) UsedRandom() bool { return s.random }

// Next mengembalikan baris berikutnya, atau ok=false bila input habis.
func (s *Source) Next() (line string, ok bool) {
	if s.reads >= s.limit() {
		s.eofReads++
		return "", false
	}
	s.reads++
	if s.reads <= len(s.given) {
		return s.given[s.reads-1], true
	}
	s.random = true
	return strconv.FormatInt(s.r.Int63n(Max+1), 10), true
}
