package irsim

import (
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/probe"
	"gopherviz/internal/trace"
)

// Fungsi pencarian bawaan (indexOf, in, std::find, bisect, Arrays.binarySearch, ...) pada
// koleksi mode data direkam sebagai satu perbandingan per elemen yang diperiksa, dalam
// urutan yang sama dengan pustaka aslinya (lihat package probe). Pencarian dengan callback
// (find, findIndex, some, find_if) mengikat parameter callback ke elemen yang sedang dicek,
// sehingga perbandingan di dalam callback tercatat sebagai perbandingan elemen.

// searchSpec menjelaskan satu panggilan pencarian.
type searchSpec struct {
	coll   ir.Expr    // ekspresi koleksi (atau iterator awalnya, C++)
	target int        // indeks argumen nilai yang dicari; -1 = pakai callback
	cb     int        // indeks argumen callback (target == -1)
	kind   probe.Kind // urutan pemeriksaan
	iter   bool       // C++: argumen 0 dan 1 adalah iterator first/last
}

var searchMethods = map[string]probe.Kind{
	"index": probe.Linear, "indexOf": probe.Linear, "includes": probe.Linear, "contains": probe.Linear,
	"count": probe.All, "lastIndexOf": probe.Reverse,
}

var callbackMethods = map[string]int{ // arah penelusuran
	"find": 1, "findIndex": 1, "some": 1, "every": 1, "filter": 1, "findLast": -1, "findLastIndex": -1,
}

// searchCall mengenali panggilan pencarian bawaan dari bentuk sintaksnya.
func searchCall(c *ir.Call) (searchSpec, bool) {
	fun := c.Fun
	if m, ok := fun.(*ir.Member); ok && m.Op == "::" { // std::find(...) diurai sebagai member std::find
		if id, ok := m.X.(*ir.Ident); ok && id.Name == "std" {
			fun = &ir.Ident{S: m.S, Name: m.Name}
		}
	}
	switch f := fun.(type) {
	case *ir.Member:
		if mod, ok := f.X.(*ir.Ident); ok {
			switch {
			case mod.Name == "bisect" && len(c.Args) >= 2:
				if k, ok := bisectKind(f.Name); ok {
					return searchSpec{coll: c.Args[0], target: 1, kind: k}, true
				}
			case (mod.Name == "Arrays" || mod.Name == "Collections") && f.Name == "binarySearch" && len(c.Args) == 2:
				return searchSpec{coll: c.Args[0], target: 1, kind: probe.Classic}, true
			}
		}
		if k, ok := searchMethods[f.Name]; ok && len(c.Args) >= 1 {
			return searchSpec{coll: f.X, target: 0, kind: k}, true
		}
		if dir, ok := callbackMethods[f.Name]; ok && len(c.Args) >= 1 {
			k := probe.Linear
			if dir < 0 {
				k = probe.Reverse
			}
			return searchSpec{coll: f.X, target: -1, cb: 0, kind: k}, true
		}
	case *ir.Ident:
		name := strings.TrimPrefix(f.Name, "std::")
		switch {
		case (name == "in_array" || name == "array_search") && len(c.Args) >= 2:
			return searchSpec{coll: c.Args[1], target: 0, kind: probe.Linear}, true
		case len(c.Args) == 3 && (name == "find" || name == "count" || name == "binary_search" || name == "lower_bound" || name == "upper_bound"):
			k := map[string]probe.Kind{"find": probe.Linear, "count": probe.All, "binary_search": probe.LowerBound,
				"lower_bound": probe.LowerBound, "upper_bound": probe.UpperBound}[name]
			return searchSpec{coll: c.Args[0], target: 2, kind: k, iter: true}, true
		case name == "find_if" && len(c.Args) == 3:
			return searchSpec{coll: c.Args[0], target: -1, cb: 2, kind: probe.Linear, iter: true}, true
		case len(c.Args) >= 2:
			if k, ok := bisectKind(name); ok {
				return searchSpec{coll: c.Args[0], target: 1, kind: k}, true
			}
		}
	}
	return searchSpec{}, false
}

func bisectKind(name string) (probe.Kind, bool) {
	switch name {
	case "bisect_left":
		return probe.BisectLeft, true
	case "bisect_right", "bisect":
		return probe.BisectRight, true
	}
	return 0, false
}

// collName mengembalikan nama variabel koleksi dari ekspresi koleksi/iterator:
// arr, arr.begin(), begin(arr), std::begin(arr), arr + k.
func collName(x ir.Expr) string {
	switch n := x.(type) {
	case *ir.Ident:
		return n.Name
	case *ir.Call:
		if m, ok := n.Fun.(*ir.Member); ok && (m.Name == "begin" || m.Name == "cbegin") {
			return collName(m.X)
		}
		if id, ok := n.Fun.(*ir.Ident); ok && (id.Name == "begin" || id.Name == "std::begin") && len(n.Args) == 1 {
			return collName(n.Args[0])
		}
	case *ir.Binary:
		if n.Op == "+" {
			return collName(n.X)
		}
	}
	return ""
}

// cbAlias: callback pencarian yang sedang dipanggil builtin; parameter pertamanya memegang elemen.
type cbAlias struct {
	decl  *ir.Func
	param string
	next  int // indeks elemen untuk panggilan berikutnya
	dir   int
}

// searchProbe merekam perbandingan pencarian bawaan sebelum builtin dijalankan. Fungsi
// kembaliannya dipanggil setelah builtin selesai (melepas pengikat callback).
func (it *interp) searchProbe(c *ir.Call, args argList, e *Env) func() {
	noop := func() {}
	if it.rec.coll == "" || it.rec.collKind == "map" || !it.recording() {
		return noop
	}
	sp, ok := searchCall(c)
	if !ok || collName(sp.coll) != it.rec.coll {
		return noop
	}
	elems := it.searchElems(e)
	if elems == nil {
		return noop
	}
	lo, hi := 0, len(elems)
	if sp.iter && len(args.pos) >= 2 {
		if p := args.pos[0]; p.k == kPtr && p.p.list != nil {
			lo = p.p.idx
		}
		if p := args.pos[1]; p.k == kPtr && p.p.list != nil {
			hi = p.p.idx
		}
		if lo < 0 || hi > len(elems) || lo > hi {
			return noop
		}
	}
	if sp.target < 0 {
		if sp.cb >= len(args.pos) {
			return noop
		}
		f := args.pos[sp.cb]
		if f.k != kFunc || f.fn.decl == nil || len(f.fn.decl.Params) == 0 || f.fn.decl.Params[0].Name == "" {
			return noop
		}
		start, dir := lo, 1
		if sp.kind == probe.Reverse {
			start, dir = hi-1, -1
		}
		prev := it.rec.cb
		it.rec.cb = &cbAlias{decl: f.fn.decl, param: f.fn.decl.Params[0].Name, next: start, dir: dir}
		return func() { it.rec.cb = prev }
	}
	if sp.target >= len(args.pos) {
		return noop
	}
	it.probeOps(sp.kind, elems[lo:hi], lo, args.pos[sp.target])
	return noop
}

// probeOps merekam satu perbandingan untuk setiap elemen yang diperiksa algoritma kind.
func (it *interp) probeOps(kind probe.Kind, elems []Value, offset int, target Value) {
	rel := func(i int) string { return it.relation(elems[i], target) }
	for _, i := range probe.Indexes(kind, len(elems), func(i int) int {
		return map[string]int{"<": -1, "=": 0, ">": 1}[rel(i)]
	}) {
		it.rec.op(trace.Op{Op: trace.OpCompare, A: offset + i, Result: rel(i), Cmp: "==", Value: it.format(target)})
	}
}

// searchElems mengembalikan elemen koleksi mode data (list, atau array PHP berindeks 0..n-1).
func (it *interp) searchElems(e *Env) []Value {
	v := it.collValue(e)
	switch v.k {
	case kList:
		return v.l.elems
	case kDict:
		if v.d.kind == "array" && phpIsList(v.d) { // jenis koleksi mungkin belum terdeteksi (langkah pertama blok)
			return v.d.vals
		}
	}
	return nil
}

// inProbe: Python "x in arr" / "x not in arr" pada koleksi mode data.
func (it *interp) inProbe(n *ir.Binary, target, coll Value) {
	if it.rec.coll == "" || !it.recording() || coll.k != kList {
		return
	}
	if id, ok := n.Y.(*ir.Ident); ok && id.Name == it.rec.coll {
		it.probeOps(probe.Linear, coll.l.elems, 0, target)
	}
}

// bindCallbackElem dipanggil di awal callFunc: bila fungsi ini callback pencarian yang
// sedang berjalan, parameter pertamanya memegang elemen berikutnya.
func (it *interp) bindCallbackElem(decl *ir.Func, args argList) func() {
	cb := it.rec.cb
	if cb == nil || cb.decl != decl || len(args.pos) == 0 {
		return func() {}
	}
	idx := cb.next
	if len(args.pos) > 1 && args.pos[1].k == kInt && (it.lang == "javascript" || it.lang == "typescript") {
		idx = int(args.pos[1].i) // (elemen, indeks, array)
	}
	cb.next = idx + cb.dir
	return it.rec.bindElem(cb.param, idx, it.format(args.pos[0]))
}
