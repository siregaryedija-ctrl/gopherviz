package irsim

import (
	"strconv"
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/userinput"
)

// userText memberi baris pengganti input user (angka acak, lihat package userinput). Bila echo,
// barisnya ditampilkan di output seperti yang diketik di terminal. ok=false bila input sudah habis.
func (it *interp) userText(echo bool) (string, bool) {
	if it.input == nil {
		it.input = userinput.New()
	}
	it.warn(userinput.Warning)
	s, ok := it.input.Next()
	if ok && echo {
		it.out(s + "\n")
	}
	return s, ok
}

// inputLeft: masih ada baris input (Scanner.hasNext dan sejenisnya).
func (it *interp) inputLeft() bool {
	if it.input == nil {
		it.input = userinput.New()
	}
	return it.input.Left()
}

// readToken membaca satu nilai (cin >>, scanf("%d"), Scanner.nextInt): spasi/Enter di depan
// dilewati, dan Enter setelah angka tetap tertinggal di input.
func (it *interp) readToken() (string, bool) {
	it.pendingNL = false
	s, ok := it.userText(true)
	it.pendingNL = ok
	return s, ok
}

// readLine membaca satu baris (getline, fgets, Scanner.nextLine). Bila sebelumnya ada readToken,
// yang terbaca hanya sisa baris itu: string kosong. Ini jebakan klasik nextInt() + nextLine().
func (it *interp) readLine() (string, bool) {
	if it.pendingNL {
		it.pendingNL = false
		return "", true
	}
	return it.userText(true)
}

// stdinChar membaca satu karakter (getchar): baris input dipecah per karakter, diakhiri '\n'.
func (it *interp) stdinChar() (byte, bool) {
	if it.stdinBuf == "" && it.pendingNL {
		it.pendingNL = false
		return '\n', true
	}
	if it.stdinBuf == "" {
		s, ok := it.userText(true)
		if !ok {
			return 0, false
		}
		it.stdinBuf = s + "\n"
	}
	c := it.stdinBuf[0]
	it.stdinBuf = it.stdinBuf[1:]
	return c, true
}

// inputValue mengubah teks input menjadi nilai bertipe sama dengan variabel tujuannya
// (cin >> x, scanf("%d", &x)).
func (it *interp) inputValue(cur Value, typ, text string) Value {
	n, _ := strconv.ParseInt(text, 10, 64)
	if typ == "" {
		typ = cur.t
	}
	switch {
	case cur.k == kStr || strings.Contains(typ, "string"):
		return strV(text)
	case typ == "char":
		return typedInt(int64(text[0]), "char")
	case cur.k == kFloat || isFloatType(typ):
		if typ == "" {
			return floatV(float64(n))
		}
		return it.sem.convert(it, floatV(float64(n)), typ)
	case typ != "":
		return it.sem.convert(it, typedInt(n, "long long"), typ)
	}
	return intV(n)
}

// storeInput menulis input ke tujuan pointer/referensi sesuai tipenya.
func (it *interp) storeInput(p *Ptr, text string) {
	typ := ""
	if p.cell != nil {
		typ = p.cell.typ
	}
	p.store(it.inputValue(p.load(), typ, text))
}

// isCin: ekspresi istream C++ di kiri >> (cin, std::cin, atau rantai cin >> a >> b).
func isCin(x ir.Expr) bool {
	switch t := x.(type) {
	case *ir.Ident:
		return t.Name == "cin" || t.Name == "std::cin"
	case *ir.Binary:
		return t.Op == ">>" && isCin(t.X)
	}
	return false
}

// cinRead menjalankan `cin >> x`: x diisi angka acak; setelah input habis, cin gagal (false).
func (it *interp) cinRead(n *ir.Binary, e *Env) Value {
	in := it.eval(n.X, e)
	if in.k != kObj {
		return in
	}
	if failed, _ := in.o.fields.get(strV("fail")); failed.k == kBool && failed.b {
		return in
	}
	p := it.addrOf(n.Y, e)
	if p == nil {
		it.structural("tujuan cin >> harus berupa variabel")
	}
	s, ok := it.readToken()
	if !ok {
		in.o.fields.set(strV("fail"), boolV(true))
		return in
	}
	it.storeInput(p, s)
	return in
}
