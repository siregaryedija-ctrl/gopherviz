// Pengenal algoritma sorting (Fase 12c): dari trace (nama fungsi, nama variabel, operasi pada
// koleksi, rekursi) ditebak jenis sort-nya dan variabel yang berperan, supaya panggung bisa
// memberi penanda: MIN (selection), PIVOT + rentang partisi (quick), KEY (insertion), belah-gabung
// (merge). Modul murni tanpa canvas (dites di test/media.test.ts).
(function (G) {
  'use strict';

  const LO = ['lo', 'low', 'left', 'l', 'start', 'begin', 'first', 'kiri', 'awal'];
  const HI = ['hi', 'high', 'right', 'r', 'end', 'last', 'kanan', 'akhir'];
  const MIN = ['min_idx', 'minidx', 'min_index', 'minindex', 'imin', 'idx_min', 'min_pos', 'minpos', 'smallest', 'min_i', 'mini', 'kecil', 'terkecil', 'm', 'min', 'idx'];
  const MAX = ['max_idx', 'maxidx', 'max_index', 'maxindex', 'imax', 'largest', 'besar', 'terbesar'];
  const KEY = ['key', 'temp', 'tmp', 'cur', 'current', 'kunci', 'sisip', 'item', 'val', 'value', 'x'];
  const PIVOT = ['pivot', 'pv', 'piv', 'pivot_value', 'pivotvalue', 'pivot_val'];
  const MID = ['mid', 'middle', 'm', 'tengah', 'half'];
  const NAMES = {
    selection: 'SELECTION SORT', insertion: 'INSERTION SORT', quick: 'QUICK SORT', merge: 'MERGE SORT',
    bubble: 'BUBBLE SORT', heap: 'HEAP SORT', shell: 'SHELL SORT', counting: 'COUNTING SORT', gnome: 'GNOME SORT', cocktail: 'COCKTAIL SORT',
  };

  const isInt = v => v != null && /^-?\d+$/.test(String(v));
  const sameFunc = (a, b) => !!a && !!b && (a === b || a.endsWith('.' + b) || b.endsWith('.' + a));

  /** "[5, 2, 8]" / "[5 2 8]" / "array(3) { 5, 2, 8 }" → ['5', '2', '8']; null bila bukan list. */
  function parseList(text) {
    if (text == null) return null;
    const s = String(text);
    const a = s.indexOf('['), b = s.lastIndexOf(']');
    if (a < 0 || b < a) return null;
    const inner = s.slice(a + 1, b).trim();
    if (!inner) return [];
    if (/[[\]{}]/.test(inner)) return null; // list bersarang
    return (inner.includes(',') ? inner.split(',') : inner.split(/\s+/)).map(x => x.trim()).filter(x => x !== '');
  }

  /** Nama variabel pertama dari daftar calon yang muncul di trace (nama asli, bukan huruf kecil). */
  function pick(cands, seen, ok) {
    for (const c of cands) for (const name of seen) if (name.toLowerCase() === c && (!ok || ok(name))) return name;
    return null;
  }

  /**
   * Jenis sort dan variabel perannya. Hasil: { kind, title, min, max, key, pivot, lo, hi, mid, outer,
   * recursive, lists } atau null bila bukan sorting.
   */
  function detect(tr) {
    if (!tr || !tr.steps || !tr.steps.length) return null;
    const fname = (tr.block && tr.block.name) || '';
    const lname = fname.toLowerCase();
    const seen = new Set();
    const ints = {}; // variabel → selalu angka bulat?
    let swaps = 0, sets = 0, cmps = 0, appends = 0, calls = 0, listArgs = 0, listRets = 0;
    for (const s of tr.steps) {
      for (const k in s.vars || {}) {
        seen.add(k);
        ints[k] = (ints[k] !== false) && isInt(s.vars[k]);
      }
      for (const o of s.ops || []) {
        if (o.op === 'swap') swaps++;
        else if (o.op === 'set') sets++;
        else if (o.op === 'compare') cmps++;
        else if (o.op === 'append') appends++;
      }
      if (s.phase === 'call' && sameFunc(s.func, fname)) { calls++; if (parseList(s.args)) listArgs++; }
      if (s.phase === 'return' && sameFunc(s.func, fname) && parseList(s.return)) listRets++;
    }
    const intVar = n => ints[n] === true;
    const recursive = calls > 1;
    const lists = recursive && listArgs === calls && listRets > 0; // rekursi yang membawa list baru (merge sort gaya fungsional)
    const r = {
      kind: null, recursive, lists,
      min: pick(MIN, seen, intVar), max: pick(MAX, seen, intVar), key: pick(KEY, seen),
      pivot: pick(PIVOT, seen), lo: pick(LO, seen, intVar), hi: pick(HI, seen, intVar), mid: pick(MID, seen, intVar),
      outer: seen.has('i') && intVar('i') ? 'i' : null,
    };
    for (const k of Object.keys(NAMES)) if (lname.includes(k)) { r.kind = k; break; }
    if (!r.kind) {
      if (r.pivot) r.kind = 'quick';
      else if (lists && (r.mid || /merge|gabung/.test(lname))) r.kind = 'merge';
      else if (recursive && r.mid && (sets || appends)) r.kind = 'merge';
      else if (r.min && swaps && !sets) r.kind = 'selection';
      else if (r.key && sets && !swaps) r.kind = 'insertion';
      else if (swaps && cmps && !recursive) r.kind = 'bubble';
    }
    if (!r.kind) return null;
    if (!(cmps || swaps || sets || appends || lists)) return null; // tidak menyentuh koleksi
    r.title = NAMES[r.kind];
    if (r.kind !== 'selection') r.min = null;
    if (r.kind !== 'insertion') r.key = null;
    if (r.kind !== 'quick' && r.kind !== 'merge') r.lo = r.hi = null;
    if (r.kind !== 'quick') r.pivot = null;
    return r;
  }

  G.sorts = { detect, parseList, sameFunc, NAMES };
})(typeof window !== 'undefined' ? (window.GV = window.GV || {}) : (globalThis.GV = globalThis.GV || {}));
