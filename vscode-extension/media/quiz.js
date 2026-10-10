// Mode Tebak (Fase 12b): pemutar berhenti di beberapa titik penting trace (maks 5) dan bertanya
// apa yang akan terjadi: cabang if/switch yang dipilih, nilai kondisi, apakah loop lanjut, output
// yang dicetak, nilai return, dan error. Modul ini hanya memilih dan menyusun pertanyaan dari
// tampilan trace (view); main.js yang menampilkan kartunya dan menghentikan pemutar.
(function (G) {
  'use strict';

  const MAX = 5;
  const LETTERS = ['A', 'B', 'C', 'D'];
  const BRANCH_KW = { 'if': 'if', 'else-if': 'else if', 'else': 'else', 'case': 'case', 'default': 'default' };
  const ERR_TITLES = ['Indeks di luar batas', 'Membagi dengan nol', 'Nilai kosong', 'Key tidak ditemukan', 'Rekursi terlalu dalam', 'Tipe data tidak cocok'];

  /** "if a[j] > a[j+1] → 3 > 1 → true" → { expr, subst, result } (null bila bukan catatan kondisi). */
  function parseCond(note) {
    if (!note) return null;
    const first = note.indexOf(' → '), last = note.lastIndexOf(' → ');
    if (first < 0) return null;
    const res = note.slice(last + 3).trim().toLowerCase();
    if (res !== 'true' && res !== 'false') return null;
    let expr = note.slice(0, first).trim();
    const kw = expr.match(/^(if|elif|else if|elseif|while|case|for)\s+/i);
    if (kw) expr = expr.slice(kw[0].length);
    return { expr, subst: first === last ? '' : note.slice(first + 3, last).trim(), result: res === 'true', kw: kw ? kw[1].toLowerCase() : '' };
  }

  const boolText = (lang, b) => (lang === 'python' ? (b ? 'True' : 'False') : String(b));
  const stepsOf = v => v.group || [v.step];
  const outOf = v => stepsOf(v).map(s => s.output || '').join('');
  const show = s => s.replace(/\n$/, '').replace(/\n/g, ' ⏎ ');

  /** Variabel yang disebut di ekspresi (untuk ditampilkan sebagai petunjuk). */
  function varsIn(expr, vars) {
    const names = new Set((expr.match(/[A-Za-z_$][\w$]*/g) || []));
    return Object.keys(vars || {}).filter(k => names.has(k)).sort().map(k => [k, vars[k]]);
  }

  /** Pilihan jawaban: jawaban benar + pengecoh, diacak dengan benih tetap (stabil per pertanyaan). */
  function options(correct, pool, seed) {
    const out = [correct];
    for (const p of pool) {
      if (out.length >= 4) break;
      if (p != null && p !== '' && !out.includes(p)) out.push(p);
    }
    let x = seed * 9301 + 49297;
    for (let i = out.length - 1; i > 0; i--) {
      x = (x * 9301 + 49297) % 233280;
      const j = Math.floor((x / 233280) * (i + 1));
      [out[i], out[j]] = [out[j], out[i]];
    }
    return { options: out, answer: out.indexOf(correct) };
  }

  const BOOLS = [['True', 'False'], ['true', 'false']];

  /**
   * Pengecoh untuk sebuah nilai. Boolean: hanya lawannya (True ↔ False). Angka: nilai lain di trace
   * lalu variasi angka. Lainnya (teks, list, None): hanya nilai lain yang benar-benar muncul di trace,
   * tanpa variasi buatan (dulu "FALSE" / "FalseFalse" ikut muncul sebagai pilihan).
   */
  function decoys(correct, others) {
    for (const pair of BOOLS) if (pair.includes(correct)) return pair.filter(x => x !== correct);
    const isBool = x => BOOLS.some(p => p.includes(x));
    const pool = others.filter(o => o !== correct && !isBool(o));
    const n = Number(correct);
    if (correct !== '' && !isNaN(n) && isFinite(n)) {
      const isInt = Number.isInteger(n);
      const vary = isInt ? [n + 1, n - 1, n * 2, n + 2] : [n + 1, n / 2, n * 2];
      pool.push(...vary.map(String));
    } else {
      const list = /^\[(.*)\]$/.exec(correct); // list: urutan terbalik sebagai pengecoh
      if (list && list[1].includes(',')) pool.push('[' + list[1].split(/,\s*/).reverse().join(', ') + ']');
    }
    return pool;
  }

  /**
   * Menyusun pertanyaan dari tampilan trace. ctx: { lang, block (blok fokus dari scene),
   * code: { lines } }. Hasil: [{ at, type, prompt, code, vars, options, answer, explain }] urut at.
   */
  function build(view, ctx) {
    ctx = ctx || {};
    const lang = ctx.lang || 'go', block = ctx.block || {};
    const lines = (ctx.code && ctx.code.lines) || [];
    const n = view.length;
    const cands = [];
    const seen = new Set();
    const push = (q, key) => {
      if (q.options.length < 2) return; // tanpa pengecoh yang masuk akal: bukan pertanyaan
      if (key && seen.has(key)) return;
      if (key) seen.add(key);
      cands.push(q);
    };
    const allOut = view.map(outOf).filter(Boolean).map(show);
    const allRet = view.map(v => v.step.phase === 'return' ? v.step.return : null).filter(x => x != null && x !== '');

    // if/switch fokus: cabang yang dipilih, ditanya di kondisi pertama setiap run
    const isBranch = block.kind === 'if' || block.kind === 'switch';
    if (isBranch) {
      const brs = block.branches || [];
      const labels = brs.map(b => `${BRANCH_KW[b.kind] || b.kind} ${b.label || ''}`.trim());
      const none = !brs.some(b => b.kind === 'else' || b.kind === 'default');
      if (none) labels.push(block.kind === 'if' ? 'tidak ada (dilewati)' : 'tidak ada yang cocok');
      let lastRun = null;
      view.forEach((v, i) => {
        const s = v.step;
        if (i === 0 || s.phase !== 'cond' || s.run === lastRun) return;
        lastRun = s.run;
        const br = view.slice(i).find(x => x.step.phase === 'branch' && x.step.run === s.run);
        if (!br || br.step.branch == null || labels.length < 2) return;
        const correct = labels[Math.min(br.step.branch, labels.length - 1)];
        // maks 4 pilihan: jawaban benar + cabang di sekitarnya
        const others = labels.filter(l => l !== correct);
        const o = options(correct, others, i);
        push({
          at: i, type: 'branch', weight: 4, prompt: `${block.kind === 'if' ? 'Cabang' : 'Case'} mana yang akan dijalankan?`,
          code: block.header || '', vars: Object.entries(s.vars || {}).slice(0, 6), ...o,
          explain: br.step.note || '',
        }, `branch:${correct}`);
      });
    }

    const lineAsked = {};
    view.forEach((v, i) => {
      if (i === 0) return;
      const s = v.step;
      // kondisi if di dalam loop/fungsi, atau kondisi loop
      const pc = s.result != null ? parseCond(s.note) : null;
      if (pc && !(isBranch && s.phase === 'cond')) {
        const prev = view[i - 1].step;
        const vars = varsIn(pc.expr, s.vars || prev.vars);
        if (s.phase === 'cond' && s.loop) {
          const correct = pc.result ? 'Ya, putaran berikutnya' : 'Tidak, keluar dari loop';
          push({
            at: i, type: 'loop', weight: pc.result ? 1 : 3, prompt: 'Apakah loop masih lanjut?', code: pc.expr, vars,
            options: ['Ya, putaran berikutnya', 'Tidak, keluar dari loop'], answer: pc.result ? 0 : 1, correct,
            explain: `${pc.expr}${pc.subst ? ' → ' + pc.subst : ''} → ${boolText(lang, pc.result)}`,
          }, `loop:${s.line}:${pc.result}`);
        } else {
          push({
            at: i, type: 'cond', weight: 2, prompt: 'Kondisi ini bernilai apa?', code: pc.expr, vars,
            options: [boolText(lang, true), boolText(lang, false)], answer: pc.result ? 0 : 1,
            explain: `${pc.expr}${pc.subst ? ' → ' + pc.subst : ''} → ${boolText(lang, pc.result)}`,
          }, `cond:${s.line}:${pc.result}`);
        }
      }
      // penugasan: "jumlah += nilai[i] → jumlah = 11" → berapa nilai jumlah sesudahnya?
      const asg = s.result == null && s.phase === 'body' && /^(.+?) → ([A-Za-z_$][\w$.]*(?:\[[^\]]*\])?) = (.+)$/.exec(s.note || '');
      if (asg && asg[3].length <= 20 && !/ → /.test(asg[3])) {
        const name = asg[2], correct = asg[3].trim();
        const per = (lineAsked[s.line] = (lineAsked[s.line] || 0) + 1);
        const before = (view[i - 1].step.vars || {})[name];
        if (per <= 2 && before !== correct) {
          const others = view.map(x => (x.step.vars || {})[name]).filter(x => x != null);
          const o = options(correct, decoys(correct, others), i);
          push({
            at: i, type: 'assign', weight: 2, prompt: `Berapa nilai ${name} setelah baris ini?`, code: asg[1].trim(),
            vars: varsIn(asg[1], view[i - 1].step.vars), ...o, explain: s.note,
          });
        }
      }
      // output
      const out = outOf(v);
      if (out && show(out).length <= 32 && !s.err) {
        const correct = show(out);
        const o = options(correct, decoys(correct, allOut), i);
        push({
          at: i, type: 'output', weight: 3, prompt: 'Apa yang akan dicetak?', code: (lines[(s.line || 0) - 1] || s.note || '').trim(),
          vars: Object.entries(s.vars || {}).slice(0, 6), ...o, explain: `Tercetak: ${correct}`,
        }, `out:${correct}`);
      }
      // return
      if (s.phase === 'return' && s.return != null && s.return !== '' && String(s.return).length <= 24) {
        const call = (s.note || '').split(' mengembalikan')[0] || s.func || 'fungsi';
        const correct = String(s.return);
        const o = options(correct, decoys(correct, allRet.map(String)), i);
        push({
          at: i, type: 'return', weight: 3, prompt: `${call} mengembalikan ${isNaN(Number(correct)) ? "apa" : "nilai berapa"}?`, code: call, vars: [], ...o,
          explain: s.note || '',
        }, `ret:${call}`);
      }
      // error yang menghentikan program
      if (s.err) {
        const correct = s.err.title;
        const pool = ['Tidak ada masalah, program lanjut', ...ERR_TITLES];
        const o = options(correct, pool, i);
        push({
          at: i, type: 'error', weight: 5, prompt: `Apa yang terjadi saat baris ${s.line || '?'} dijalankan?`,
          code: (lines[(s.line || 0) - 1] || '').trim(), vars: Object.entries(s.vars || (view[i - 1].step.vars) || {}).slice(0, 6), ...o,
          explain: s.err.text,
        }, 'err');
      }
    });

    return pick(cands, n);
  }

  /** Maks 5 pertanyaan yang tersebar sepanjang trace; bobot tinggi (error, cabang, return) didahulukan. */
  function pick(cands, n) {
    if (cands.length <= MAX) return cands.sort((a, b) => a.at - b.at);
    const chosen = new Set();
    const err = cands.find(q => q.type === 'error');
    if (err) chosen.add(err);
    const slots = MAX - chosen.size;
    for (let k = 0; k < slots; k++) {
      const lo = (k * n) / slots, hi = ((k + 1) * n) / slots;
      const inSeg = cands.filter(q => q.at >= lo && q.at < hi && !chosen.has(q));
      inSeg.sort((a, b) => b.weight - a.weight || a.at - b.at);
      if (inSeg[0]) chosen.add(inSeg[0]);
    }
    const rest = cands.filter(q => !chosen.has(q)).sort((a, b) => b.weight - a.weight || a.at - b.at);
    while (chosen.size < MAX && rest.length) chosen.add(rest.shift());
    return [...chosen].sort((a, b) => a.at - b.at);
  }

  G.quiz = { build, parseCond, LETTERS, MAX };
})(typeof window !== 'undefined' ? (window.GV = window.GV || {}) : globalThis.GV = globalThis.GV || {});
