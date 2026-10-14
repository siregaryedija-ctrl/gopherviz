// Package explain mengubah error program yang disimulasikan (panic Go, exception Python/JS/Java/PHP,
// crash C/C++) menjadi penjelasan sederhana (bahasa Inggris) untuk panel (Fase 12b):
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
	case name == "InputEnded":
		return "input"
	case has("deadlock"):
		return "deadlock"
	case name == "RecursionError" || name == "StackOverflowError" ||
		has("maximum call stack", "recursion depth", "stack overflow", "recursion too deep", "nesting level"):
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
		has("index out of range", "out of bounds", "slice bounds out of range"):
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
		coll = "collection"
	}
	switch lang {
	case "go", "python":
		return "len(" + coll + ")"
	case "javascript", "typescript", "java":
		return coll + ".length"
	case "php":
		return "count($" + strings.TrimPrefix(coll, "$") + ")"
	}
	return "size of " + coll
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
		e.Title = "Taking from an empty container"
		what := collOf(c)
		if what == "" {
			what = "the list/queue"
		}
		e.Text = fmt.Sprintf("The program takes an item from %s, but it is already empty.", what)
		ex := map[string]string{
			"python": "if " + orName(collOf(c), "queue") + ":", "go": "if len(" + orName(collOf(c), "s") + ") > 0 {",
			"java": "if (!" + orName(collOf(c), "stack") + ".isEmpty())",
		}[lang]
		e.Hint = "Check that it still has items before taking one (length > 0)."
		if ex != "" {
			e.Hint = "Check that it still has items before taking one, e.g. " + ex
		}
	case "zero":
		e.Title = "Division by zero"
		e.Value = "0"
		at := ""
		if c.Expr != "" {
			at = " in " + e.Expr
		}
		e.Text = fmt.Sprintf("The divisor%s is 0. Dividing by zero has no result, so the program stops.", at)
		e.Hint = "Check the divisor before dividing, e.g. " + ifNotZero(lang, divisor(c.Expr))
	case "null":
		w := nullWord(lang, e.Msg)
		e.Title = "Empty value (" + w + ")"
		e.Value = w
		what := "A variable"
		if m := reRecv.FindStringSubmatch(c.Expr); m != nil { // nama.upper(): yang kosong adalah nama
			what = m[1]
		} else if c.Expr != "" {
			what = "The part " + e.Expr
		}
		if strings.Contains(e.Msg, "nil map") {
			e.Text = "The map is still nil (not created with make yet), so nothing can be stored in it."
			e.Hint = "Create the map first: m := make(map[string]int) or map[string]int{}."
			return
		}
		e.Text = fmt.Sprintf("%s holds %s, meaning it points to no data, but it is used as if it held data.", what, w)
		e.Hint = "Make sure it has a value, or check before using it: " + notNull(lang)
	case "key":
		e.Title = "Key not found"
		k := c.Value
		if k == "" {
			k = strings.TrimSpace(strings.TrimPrefix(e.Msg, "KeyError:"))
		}
		e.Value = clip(k, 30)
		e.Text = fmt.Sprintf("Key %s is not in %s. Reading a key that does not exist stops the program.", e.Value, orName(collOf(c), "that dictionary/map"))
		e.Hint = map[string]string{
			"python": "Use d.get(key), which returns None, or check first: if key in d:",
			"cpp":    "Use m.count(key) or m.find(key) != m.end() before m.at(key).",
		}[lang]
		if e.Hint == "" {
			e.Hint = "Check that the key exists before reading its value."
		}
	case "depth":
		e.Title = "Recursion too deep"
		n := c.Depth
		if n <= 0 {
			n = 200
		}
		e.Value = strconv.Itoa(n)
		e.Text = fmt.Sprintf("The function has called itself %d floors deep and still has not stopped. This usually means the base case is never reached (endless recursion).", n)
		e.Hint = "Make sure there is a base case (stop condition) that is always reached, and that every recursive call gets closer to it."
	case "type":
		e.Title = "Type mismatch"
		e.Text = "This operation is used on values of the wrong type (" + clip(e.Msg, 90) + ")."
		e.Hint = "Convert the values to the same type first, e.g. " + map[string]string{
			"python": "str(number) for text or int(text) for a number", "javascript": "String(x) or Number(x)",
			"typescript": "String(x) or Number(x)", "php": "(string)$x or (int)$x", "java": "String.valueOf(x) or Integer.parseInt(s)",
		}[lang] + "."
		if strings.HasSuffix(e.Hint, "e.g. .") {
			e.Hint = "Check the type of every value used on this line."
		}
		if strings.Contains(e.Msg, "is not a function") {
			e.Text = "The value called like a function is not a function (" + clip(e.Msg, 90) + ")."
			e.Hint = "Check the spelling of the function name and make sure it really is a function."
		}
	case "value":
		e.Title = "Value cannot be converted"
		e.Text = "The value cannot be converted to the requested form (" + clip(e.Msg, 90) + ")."
		e.Hint = "Make sure text converted to a number really contains a number."
	case "input":
		e.Title = "User input ran out"
		e.Text = "The program keeps asking for input, but every value has already been used. GopherViz only types the values from the Input box (and up to 20 random numbers), so the simulation stops here instead of looping forever."
		e.Hint = "Type more values in the Input box, separated by commas, e.g. 1, Ani, 2, Budi, 3, 0. Include the value that ends the loop (such as 0)."
	case "deadlock":
		e.Title = "Deadlock: everyone is waiting"
		e.Text = "All goroutines are waiting for each other (channel send/receive, Wait, Lock), so none of them can run anymore."
		e.Hint = "Make sure every channel send has a receiver (or the channel is buffered), and close a channel before ranging over it."
	default:
		e.Title = "The program stopped with an error"
		name := e.Name
		if name == "" || name == "panic" {
			e.Text = "The program was stopped on purpose (" + clip(e.Msg, 90) + ") and nothing caught it."
		} else {
			e.Text = fmt.Sprintf("The program threw %s (%s) and nothing caught it.", name, clip(e.Msg, 80))
		}
		e.Hint = map[string]string{
			"go":     "Catch it with defer + recover(), or return the error as a second value.",
			"python": "Catch it with try/except if that error can really happen.",
			"php":    "Catch it with try/catch if that error can really happen.",
			"c":      "Check the condition before this line.",
		}[lang]
		if e.Hint == "" {
			e.Hint = "Catch it with try/catch if that error can really happen."
		}
	}
}

func fillIndex(e *trace.ErrInfo, lang string, c *Ctx) {
	e.Title = "Index out of range"
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
	what := orName(coll, "that collection")
	switch {
	case idx != "" && n == 0:
		e.Text = fmt.Sprintf("The program reads index %s of %s, but %s is still empty (length 0).", idx, what, what)
	case idx != "" && n > 0:
		e.Text = fmt.Sprintf("The program reads index %s of %s, but its length is %d: the valid indexes are only 0 to %d.", idx, what, n, n-1)
	case idx != "":
		e.Text = fmt.Sprintf("Index %s does not exist in %s.", idx, what)
	default:
		e.Text = "The program reads an index that does not exist in " + what + "."
	}
	if strings.HasPrefix(idx, "-") {
		e.Hint = "The index cannot be negative here; check how the index is calculated."
	} else {
		e.Hint = fmt.Sprintf("Make sure the index is < %s. In a loop this usually comes from using <= where it should be <.", lenOf(lang, coll))
	}
	if lang == "c" || lang == "cpp" {
		e.Text += " In C/C++ this is undefined behavior: the program may read garbage memory or crash."
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
