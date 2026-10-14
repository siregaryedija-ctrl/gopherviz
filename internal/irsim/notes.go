package irsim

import "gopherviz/internal/ir"

// Catatan langkah tanpa menghitung ulang.
//
// Catatan seperti "i < n → 0 < 3 → true" dan "a[n++] = 7 → a[0] = 7" butuh nilai sub-ekspresi.
// Dulu nilainya dihitung ulang sesudah program menghitungnya, sehingga efek samping terjadi dua
// kali: `cin >> x` membaca dua input, `i++` menambah dua kali, fungsi dipanggil dua kali, dan
// nilai yang ditampilkan adalah nilai sesudah efek sampingnya. Sekarang setiap pernyataan dan
// kondisi yang direkam dihitung dengan evalCapture: nilai setiap sub-ekspresi yang dihitung
// program (pada kedalaman panggilan yang sama) disimpan, lalu catatan memakai simpanan itu.
// Ekspresi tanpa efek samping yang tidak sempat disimpan masih boleh dihitung ulang.

// evalCapture mengevaluasi x sambil menyimpan nilai sub-ekspresinya; sesudahnya it.notes berisi
// simpanan itu sampai evalCapture berikutnya selesai.
func (it *interp) evalCapture(x ir.Expr, e *Env) Value {
	if !it.recording() {
		return it.eval(x, e)
	}
	old, oldDepth := it.capt, it.captDepth
	it.capt, it.captDepth = map[ir.Expr]Value{}, it.depth
	defer func() {
		it.notes = it.capt
		it.capt, it.captDepth = old, oldDepth
	}()
	return it.eval(x, e)
}

// captured mencatat nilai x bila sedang menyimpan dan x dihitung di kedalaman yang disimpan.
func (it *interp) captured(x ir.Expr, v Value) {
	if it.capt != nil && it.depth == it.captDepth {
		it.capt[x] = v
	}
}

// noted: nilai x dari simpanan yang BARU SELESAI (kondisi/pernyataan yang sedang dicatat).
// Hanya untuk catatan yang dibuat tepat sesudah evalCapture/withCapture.
func (it *interp) noted(x ir.Expr) (Value, bool) {
	v, ok := it.notes[x]
	return v, ok
}

// live: nilai x dari simpanan yang SEDANG BERJALAN (op koleksi saat assignment dijalankan).
func (it *interp) live(x ir.Expr) (Value, bool) {
	if it.capt == nil || it.depth != it.captDepth {
		return Value{}, false
	}
	v, ok := it.capt[x]
	return v, ok
}

// noteValue: nilai x untuk catatan/op rekaman. Nilai yang sudah dihitung program bila ada; bila
// tidak, ekspresi tanpa efek samping dihitung ulang dengan tenang; selain itu ok=false.
func (it *interp) noteValue(x ir.Expr, e *Env) (Value, bool) {
	if v, ok := it.noted(x); ok {
		return v, true
	}
	return it.pureValue(x, e)
}

// liveValue sama dengan noteValue, tetapi dari simpanan yang sedang berjalan (op koleksi saat
// assignment dijalankan).
func (it *interp) liveValue(x ir.Expr, e *Env) (Value, bool) {
	if v, ok := it.live(x); ok {
		return v, true
	}
	return it.pureValue(x, e)
}

// pureValue menghitung ulang x dengan tenang, hanya bila x tidak punya efek samping.
func (it *interp) pureValue(x ir.Expr, e *Env) (Value, bool) {
	if hasEffects(x) {
		return Value{}, false
	}
	v := it.evalNote(x, e)
	return v, v.k != kUnknown
}

// targetValue: isi target assignment (a[i], obj.f) sesudah assignment, untuk catatan. Indeks yang
// punya efek samping (a[n++]) memakai nilai indeks yang sudah dihitung program, dan labelnya
// ditulis dengan nilai itu ("a[0]"), karena teks aslinya sudah menunjuk ke elemen berikutnya.
func (it *interp) targetValue(t ir.Expr, e *Env) (string, Value, bool) {
	if !hasEffects(t) {
		v := it.evalNote(t, e)
		return it.text(t), v, v.k != kUnknown
	}
	ix, ok := t.(*ir.Index)
	if !ok || ix.I == nil || hasEffects(ix.X) {
		return "", Value{}, false
	}
	idx, ok := it.noted(ix.I)
	if !ok || idx.k == kUnknown {
		return "", Value{}, false
	}
	base := it.evalNote(ix.X, e)
	if base.k == kUnknown {
		return "", Value{}, false
	}
	v := it.indexNote(base, idx, ix)
	return it.text(ix.X) + "[" + it.display(idx, true) + "]", v, v.k != kUnknown
}

// indexNote: base[idx] untuk catatan; error (indeks di luar batas) tidak menghentikan program.
func (it *interp) indexNote(base, idx Value, at ir.Node) (v Value) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(thrown); !ok {
				panic(r)
			}
			v = unknown("error while building the note")
		}
	}()
	it.quiet++
	defer func() { it.quiet-- }()
	return it.index(base, idx, at)
}

// hasEffects melaporkan apakah menghitung x bisa mengubah sesuatu (assignment, ++/--, pemanggilan
// fungsi selain yang sederhana, pembacaan input `cin >> x`, new, await, yield).
func hasEffects(x ir.Expr) bool {
	found := false
	ir.Walk(x, func(n ir.Node) bool {
		if found {
			return false
		}
		switch t := n.(type) {
		case *ir.Assign, *ir.New, *ir.Func, *ir.Class:
			found = true
		case *ir.Unary:
			switch t.Op {
			case "++", "--", "await", "yield", "delete", "new":
				found = true
			}
		case *ir.Call:
			if !isSimpleCall(t) {
				found = true
			}
		case *ir.Binary:
			if (t.Op == ">>" || t.Op == "<<") && (isCin(t.X) || isCout(t.X)) {
				found = true
			}
		}
		return !found
	})
	return found
}

// isCout: ekspresi ostream C++ di kiri << (cout, cerr, atau rantai cout << a << b).
func isCout(x ir.Expr) bool {
	switch t := x.(type) {
	case *ir.Ident:
		switch t.Name {
		case "cout", "std::cout", "cerr", "std::cerr", "clog", "std::clog":
			return true
		}
	case *ir.Binary:
		return t.Op == "<<" && isCout(t.X)
	}
	return false
}
