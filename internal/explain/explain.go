// Package explain mengubah error program yang disimulasikan (panic Go, exception Python/JS/Java/PHP,
// crash C/C++) menjadi penjelasan sederhana dalam Bahasa Indonesia untuk panel (Fase 12b):
// jenis error (untuk memilih animasi), judul, kalimat dengan nilai penyebabnya, dan saran perbaikan.
package explain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopherviz/internal/trace"
)

// Ctx adalah keterangan tambahan dari tempat error terjadi; field kosong = tidak diketahui.
type Ctx struct {
	Expr  string   // potongan kode penyebab, mis. "arr[i]" atau "a / b"
	Coll  string   // index/key: nama koleksi (diambil dari Expr bila kosong)
	Value string   // nilai penyebab: indeks, key, pembagi
	Len   int      // index: panjang koleksi; < 0 = tidak diketahui
	Items []string // index/key: isi koleksi
	Depth int      // depth: batas kedalaman yang dilewati
}

// NoLen menandai panjang koleksi yang tidak diketahui.
const NoLen = -1

// MaxItems adalah jumlah isi koleksi yang dikirim untuk animasi.
const MaxItems = 12

// Error membuat penjelasan untuk error bernama name dengan pesan msg di bahasa lang.
// c boleh nil.
func Error(lang, name, msg string, c *Ctx) *trace.ErrInfo {
	if c == nil {
		c = &Ctx{Len: NoLen}
	}
	e := &trace.ErrInfo{Kind: classify(name, msg), Name: name, Msg: msg, Expr: clip(c.Expr, 60), Value: clip(c.Value, 30)}
	if c.Len >= 0 && (e.Kind == "index" || e.Kind == "empty") {
		n := c.Len
		e.Len = &n
	}
	if len(c.Items) > 0 {
		items := c.Items
		if len(items) > MaxItems {
			items = items[:MaxItems]
		}
		e.Items = items
	}
	fill(e, lang, c)
	return e
}

var (
	reGoIndex  = regexp.MustCompile(`index out of range \[(-?\d+)\] with length (\d+)`)
	reJavaIdx  = regexp.MustCompile(`Index (-?\d+) out of bounds for length (\d+)`)
	reCppIdx   = regexp.MustCompile(`__n \(which is (\d+)\) >= this->size\(\) \(which is (\d+)\)`)
	reCollName = regexp.MustCompile(`^\$?([A-Za-z_][\w.]*?)\s*\[`)
	reRecv     = regexp.MustCompile(`^\$?([A-Za-z_][\w.]*?)\s*(?:\.|->)\s*([A-Za-z_]\w*)\s*(?:\(.*\))?$`)
)

// classify memilih jenis error dari nama dan pesannya.
func classify(name, msg string) string {
	m := strings.ToLower(msg)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(m, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("deadlock"):
		return "deadlock"
	case name == "RecursionError" || name == "StackOverflowError" ||
		has("maximum call stack", "recursion depth", "stack overflow", "rekursi terlalu dalam", "nesting level"):
		return "depth"
	case name == "ZeroDivisionError" || name == "DivisionByZeroError" ||
		has("by zero", "divide by zero", "division by zero", "modulo by zero", "float modulo", "cannot be raised to a negative power"):
		return "zero"
	case has("pop from empty", "pop from an empty", "empty sequence", "empty iterable", "dictionary is empty", "empty set",
		"empty list", "nosuchelement", "empty stack", "emptystack"):
		return "empty"
	case name == "KeyError" || m == "map::at" || name == "out_of_range" && strings.HasPrefix(m, "map::"):
		return "key"
	case name == "IndexError" || strings.HasSuffix(name, "IndexOutOfBoundsException") || name == "out_of_range" ||
		has("index out of range", "out of bounds", "slice bounds out of range", "di luar batas"):
		return "index"
	case name == "NullPointerException" || name == "SegmentationFault" ||
		has("nil pointer", "nil map", "of undefined", "of null", "nonetype", "is null", "because value is null", "null pointer", "pointer null"):
		return "null"
	case name == "ValueError" || name == "NumberFormatException" || name == "invalid_argument" || has("invalid literal"):
		return "value"
	case name == "TypeError" || name == "ClassCastException" || name == "ArgumentCountError" ||
		has("unsupported operand", "not supported between", "can only concatenate", "is not a function", "must be of type"):
		return "type"
	}
	return "other"
}

// kata-kata yang berbeda per bahasa
func nullWord(lang, msg string) string {
	switch lang {
	case "go":
		return "nil"
	case "python":
		return "None"
	case "javascript", "typescript":
		if strings.Contains(msg, "undefined") {
			return "undefined"
		}
		return "null"
	case "c", "cpp":
		return "NULL"
	}
	return "null"
}

func lenOf(lang, coll string) string {
	if coll == "" {
		coll = "koleksi"
	}
	switch lang {
	case "go", "python":
		return "len(" + coll + ")"
	case "javascript", "typescript", "java":
		return coll + ".length"
	case "php":
		return "count($" + strings.TrimPrefix(coll, "$") + ")"
	}
	return "ukuran " + coll
}

func collOf(c *Ctx) string {
	if c.Coll != "" {
		return c.Coll
	}
	if m := reCollName.FindStringSubmatch(c.Expr); m != nil {
		return m[1]
	}
	if m := reRecv.FindStringSubmatch(c.Expr); m != nil { // antrian.pop(0)
		return m[1]
	}
	return ""
}

func fill(e *trace.ErrInfo, lang string, c *Ctx) {
	switch e.Kind {
	case "index":
		fillIndex(e, lang, c)
	case "empty":
		e.Title = "Mengambil dari wadah kosong"
		what := collOf(c)
		if what == "" {
			what = "list/antrian"
		}
		e.Text = fmt.Sprintf("Program mengambil isi dari %s, padahal isinya sudah habis (kosong).", what)
		ex := map[string]string{
			"python": "if " + orName(collOf(c), "antrian") + ":", "go": "if len(" + orName(collOf(c), "s") + ") > 0 {",
			"java": "if (!" + orName(collOf(c), "stack") + ".isEmpty())",
		}[lang]
		e.Hint = "Cek dulu apakah masih ada isinya sebelum mengambil (panjangnya > 0)."
		if ex != "" {
			e.Hint = "Cek dulu apakah masih ada isinya sebelum mengambil, mis. " + ex
		}
	case "zero":
		e.Title = "Membagi dengan nol"
		e.Value = "0"
		at := ""
		if c.Expr != "" {
			at = " pada " + e.Expr
		}
		e.Text = fmt.Sprintf("Pembaginya%s bernilai 0. Membagi dengan nol tidak punya hasil, jadi program berhenti.", at)
		e.Hint = "Cek dulu pembaginya sebelum membagi, mis. " + ifNotZero(lang, divisor(c.Expr))
	case "null":
		w := nullWord(lang, e.Msg)
		e.Title = "Nilai kosong (" + w + ")"
		e.Value = w
		what := "Sebuah variabel"
		if m := reRecv.FindStringSubmatch(c.Expr); m != nil { // nama.upper(): yang kosong adalah nama
			what = m[1]
		} else if c.Expr != "" {
			what = "Bagian " + e.Expr
		}
		if strings.Contains(e.Msg, "nil map") {
			e.Text = "Map-nya masih nil (belum dibuat dengan make), jadi belum bisa diisi."
			e.Hint = "Buat map-nya dulu: m := make(map[string]int) atau map[string]int{}."
			return
		}
		e.Text = fmt.Sprintf("%s berisi %s, artinya tidak menunjuk ke data apa pun, lalu dipakai seolah-olah berisi data.", what, w)
		e.Hint = "Pastikan nilainya sudah diisi, atau cek dulu sebelum dipakai: " + notNull(lang)
	case "key":
		e.Title = "Key tidak ditemukan"
		k := c.Value
		if k == "" {
			k = strings.TrimSpace(strings.TrimPrefix(e.Msg, "KeyError:"))
		}
		e.Value = clip(k, 30)
		e.Text = fmt.Sprintf("Key %s tidak ada di %s. Mengambil key yang tidak ada membuat program berhenti.", e.Value, orName(collOf(c), "dictionary/map itu"))
		e.Hint = map[string]string{
			"python": "Pakai d.get(key) yang mengembalikan None, atau cek dulu: if key in d:",
			"cpp":    "Pakai m.count(key) atau m.find(key) != m.end() sebelum m.at(key).",
		}[lang]
		if e.Hint == "" {
			e.Hint = "Cek dulu apakah key-nya ada sebelum mengambil nilainya."
		}
	case "depth":
		e.Title = "Rekursi terlalu dalam"
		n := c.Depth
		if n <= 0 {
			n = 200
		}
		e.Value = strconv.Itoa(n)
		e.Text = fmt.Sprintf("Fungsi sudah memanggil dirinya sendiri %d lantai bertumpuk dan belum juga berhenti. Biasanya ini tanda base case tidak pernah tercapai (rekursi tanpa akhir).", n)
		e.Hint = "Pastikan ada base case (kondisi berhenti) yang pasti tercapai, dan setiap panggilan rekursif makin dekat ke base case itu."
	case "type":
		e.Title = "Tipe data tidak cocok"
		e.Text = "Operasi ini dipakai pada tipe data yang tidak cocok (" + clip(e.Msg, 90) + ")."
		e.Hint = "Ubah dulu tipenya supaya sama, mis. " + map[string]string{
			"python": "str(angka) untuk teks atau int(teks) untuk angka", "javascript": "String(x) atau Number(x)",
			"typescript": "String(x) atau Number(x)", "php": "(string)$x atau (int)$x", "java": "String.valueOf(x) atau Integer.parseInt(s)",
		}[lang] + "."
		if strings.HasSuffix(e.Hint, "mis. .") {
			e.Hint = "Periksa tipe setiap nilai yang dipakai di baris ini."
		}
		if strings.Contains(e.Msg, "is not a function") {
			e.Text = "Nilai yang dipanggil seperti fungsi ternyata bukan fungsi (" + clip(e.Msg, 90) + ")."
			e.Hint = "Periksa ejaan nama fungsinya dan pastikan isinya benar-benar fungsi."
		}
	case "value":
		e.Title = "Nilai tidak bisa diubah"
		e.Text = "Nilainya tidak bisa diubah ke bentuk yang diminta (" + clip(e.Msg, 90) + ")."
		e.Hint = "Pastikan teks yang diubah ke angka memang berisi angka."
	case "deadlock":
		e.Title = "Deadlock: semua menunggu"
		e.Text = "Semua goroutine sedang menunggu satu sama lain (kirim/terima channel, Wait, Lock), jadi tidak ada yang bisa jalan lagi."
		e.Hint = "Pastikan setiap kirim ke channel punya penerima (atau channel-nya ber-buffer), dan channel ditutup sebelum di-range."
	default:
		e.Title = "Program berhenti karena error"
		name := e.Name
		if name == "" || name == "panic" {
			e.Text = "Program sengaja dihentikan (" + clip(e.Msg, 90) + ") dan tidak ada yang menangkapnya."
		} else {
			e.Text = fmt.Sprintf("Program melempar %s (%s) dan tidak ada yang menangkapnya.", name, clip(e.Msg, 80))
		}
		e.Hint = map[string]string{
			"go":     "Tangkap dengan defer + recover(), atau kembalikan error sebagai nilai kedua.",
			"python": "Tangkap dengan try/except bila error itu memang bisa terjadi.",
			"php":    "Tangkap dengan try/catch bila error itu memang bisa terjadi.",
			"c":      "Periksa kondisi sebelum baris ini.",
		}[lang]
		if e.Hint == "" {
			e.Hint = "Tangkap dengan try/catch bila error itu memang bisa terjadi."
		}
	}
}

func fillIndex(e *trace.ErrInfo, lang string, c *Ctx) {
	e.Title = "Indeks di luar batas"
	idx, n := c.Value, c.Len
	for _, re := range []*regexp.Regexp{reGoIndex, reJavaIdx, reCppIdx} {
		if m := re.FindStringSubmatch(e.Msg); m != nil {
			if idx == "" {
				idx = m[1]
			}
			if n < 0 {
				n, _ = strconv.Atoi(m[2])
			}
		}
	}
	if n >= 0 && e.Len == nil {
		e.Len = &n
	}
	e.Value = idx
	coll := collOf(c)
	what := orName(coll, "koleksi itu")
	switch {
	case idx != "" && n == 0:
		e.Text = fmt.Sprintf("Program mengambil indeks %s dari %s, padahal %s masih kosong (panjangnya 0).", idx, what, what)
	case idx != "" && n > 0:
		e.Text = fmt.Sprintf("Program mengambil indeks %s dari %s, padahal panjangnya %d: indeks yang ada hanya 0 sampai %d.", idx, what, n, n-1)
	case idx != "":
		e.Text = fmt.Sprintf("Indeks %s tidak ada di %s.", idx, what)
	default:
		e.Text = "Program mengambil indeks yang tidak ada di " + what + "."
	}
	if strings.HasPrefix(idx, "-") {
		e.Hint = "Indeks tidak boleh negatif di sini; periksa perhitungan indeksnya."
	} else {
		e.Hint = fmt.Sprintf("Pastikan indeks < %s. Di loop biasanya ini karena memakai <= padahal seharusnya <.", lenOf(lang, coll))
	}
	if lang == "c" || lang == "cpp" {
		e.Text += " Di C/C++ ini perilaku tak terdefinisi: program bisa membaca sampah memori atau crash."
	}
}

func orName(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// divisor mengambil sisi kanan pembagian dari potongan kode "a / b" atau "a % b".
func divisor(expr string) string {
	for _, op := range []string{"//", "/", "%"} {
		if i := strings.LastIndex(expr, op); i >= 0 {
			if d := strings.TrimSpace(expr[i+len(op):]); d != "" {
				return d
			}
		}
	}
	return "b"
}

func ifNotZero(lang, d string) string {
	switch lang {
	case "python":
		return "if " + d + " != 0:"
	case "php":
		return "if ($" + strings.TrimPrefix(d, "$") + " != 0)"
	case "go":
		return "if " + d + " != 0 {"
	}
	return "if (" + d + " != 0)"
}

func notNull(lang string) string {
	switch lang {
	case "go":
		return "if p != nil { ... }"
	case "python":
		return "if x is not None:"
	case "php":
		return "if ($x !== null)"
	case "c", "cpp":
		return "if (p != NULL)"
	}
	return "if (x != null)"
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
