package irsim

import (
	"regexp"
	"strconv"
	"strings"

	"gopherviz/internal/explain"
	"gopherviz/internal/ir"
	"gopherviz/internal/trace"
)

// Penjelasan error (Fase 12b): tempat yang melempar error menyimpan keterangan penyebabnya
// (indeks, panjang, isi koleksi) di it.errCtx; throw membawanya ke langkah terakhir.

// indexCtx mencatat akses indeks i di luar batas pada koleksi base yang panjangnya n.
func (it *interp) indexCtx(base Value, i, n int) {
	c := &explain.Ctx{Value: strconv.Itoa(i), Len: n}
	it.quiet++ // menampilkan nilai bisa menjalankan __repr__/toString milik user
	defer func() { it.quiet-- }()
	switch {
	case base.k == kList && base.l != nil:
		for k, v := range base.l.elems {
			if k >= explain.MaxItems {
				break
			}
			c.Items = append(c.Items, it.display(v, true))
		}
	case base.k == kStr:
		for k, r := range []rune(base.s) {
			if k >= explain.MaxItems {
				break
			}
			c.Items = append(c.Items, string(r))
		}
	}
	it.errCtx = c
}

// takeErrCtx mengambil keterangan yang disiapkan untuk exception ini (lalu mengosongkannya)
// dan melengkapinya dengan potongan kode penyebab.
func (it *interp) takeErrCtx(at ir.Node) *explain.Ctx {
	c := it.errCtx
	it.errCtx = nil
	if c == nil {
		c = &explain.Ctx{Len: explain.NoLen}
	}
	if c.Expr == "" {
		c.Expr = strings.TrimSuffix(it.text(at), ";")
	}
	return c
}

// fatal menghentikan program dengan crash yang tidak bisa ditangkap try/catch (mis. indeks di
// luar batas di C/C++, stack overflow).
func (it *interp) fatal(at ir.Node, name, msg string) {
	panic(thrown{msg: name + ": " + msg, at: at, ctx: it.takeErrCtx(at), fatal: true, depth: it.rec.depth + 1})
}

var reExcName = regexp.MustCompile(`^([A-Za-z_][\w.:]*)(?::\s*(.*))?$`)

// errInfo menjelaskan exception yang tidak tertangkap. msg = "Nama: pesan".
func (it *interp) errInfo(x thrown, msg string) *trace.ErrInfo {
	name, text := "", msg
	if m := reExcName.FindStringSubmatch(strings.TrimSpace(msg)); m != nil && !strings.Contains(m[1], " ") {
		name, text = m[1], m[2]
	}
	return explain.Error(it.lang, name, text, x.ctx)
}
