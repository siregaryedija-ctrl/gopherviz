package sim

import (
	"go/ast"
	"go/token"
)

// Catatan langkah tanpa menghitung ulang (sama dengan irsim/notes.go).
//
// Catatan seperti "next()+0 < 3 → 1 < 3 → true" butuh nilai sub-ekspresi. Dulu nilainya dihitung
// ulang sesudah program menghitungnya, sehingga fungsi dipanggil dua kali dan penerimaan channel
// terjadi dua kali. Sekarang kondisi dan pernyataan yang direkam dihitung dengan withCapture:
// nilai setiap sub-ekspresi yang dihitung program (pada kedalaman panggilan yang sama) disimpan,
// lalu catatan memakai simpanan itu. Ekspresi tanpa efek samping masih boleh dihitung ulang.

// withCapture menjalankan f sambil menyimpan nilai sub-ekspresi; sesudahnya it.notes berisi
// simpanan itu sampai withCapture berikutnya selesai.
func (it *interp) withCapture(f func()) {
	if !it.recording() {
		f()
		return
	}
	old, oldDepth := it.capt, it.captDepth
	it.capt, it.captDepth = map[ast.Expr]value{}, it.depth
	defer func() {
		it.notes = it.capt
		it.capt, it.captDepth = old, oldDepth
	}()
	f()
}

func (it *interp) evalCapture(x ast.Expr, e *env) (v value) {
	it.withCapture(func() { v = it.eval(x, e) })
	return v
}

// noted: nilai x dari simpanan yang BARU SELESAI (kondisi/pernyataan yang sedang dicatat).
// Hanya untuk catatan yang dibuat tepat sesudah evalCapture/withCapture.
func (it *interp) noted(x ast.Expr) (value, bool) {
	v, ok := it.notes[x]
	return v, ok
}

// live: nilai x dari simpanan yang SEDANG BERJALAN (op koleksi saat assignment dijalankan).
func (it *interp) live(x ast.Expr) (value, bool) {
	if it.capt == nil || it.depth != it.captDepth {
		return value{}, false
	}
	v, ok := it.capt[x]
	return v, ok
}

// noteValue: nilai x untuk catatan (dari simpanan yang baru selesai) tanpa menjalankan efek
// samping lagi; liveValue sama, tetapi dari simpanan yang sedang berjalan (op koleksi).
func (it *interp) noteValue(x ast.Expr, e *env) (value, bool) {
	if v, ok := it.noted(x); ok {
		return v, true
	}
	return it.pureValue(x, e)
}

func (it *interp) liveValue(x ast.Expr, e *env) (value, bool) {
	if v, ok := it.live(x); ok {
		return v, true
	}
	return it.pureValue(x, e)
}

// pureValue menghitung ulang x dengan tenang, hanya bila x tidak punya efek samping.
func (it *interp) pureValue(x ast.Expr, e *env) (value, bool) {
	if it.hasEffects(x) {
		return value{}, false
	}
	return it.evalNote(x, e)
}

// hasEffects: menghitung x bisa mengubah sesuatu (pemanggilan fungsi selain builtin murni dan
// konversi tipe, penerimaan channel).
func (it *interp) hasEffects(x ast.Expr) bool {
	found := false
	ast.Inspect(x, func(n ast.Node) bool {
		if found {
			return false
		}
		switch t := n.(type) {
		case *ast.FuncLit:
			return false // hanya definisi, belum dijalankan
		case *ast.UnaryExpr:
			if t.Op == token.ARROW {
				found = true
			}
		case *ast.CallExpr:
			if tv, ok := it.p.Info.Types[t.Fun]; ok && tv.IsType() {
				return true // konversi tipe: int(x)
			}
			if id, ok := t.Fun.(*ast.Ident); ok && it.isBuiltin(id) {
				switch id.Name {
				case "len", "cap", "min", "max", "real", "imag", "complex":
					return true
				}
			}
			found = true
		}
		return !found
	})
	return found
}
