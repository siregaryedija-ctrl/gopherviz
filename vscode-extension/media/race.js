// Lomba algoritma (Fase 12c): dua fungsi dijalankan dengan list acak yang sama, berdampingan.
// Setiap lajur memakai panggungnya sendiri (data/merge/fungsi) di canvas tersembunyi; lomba
// memutar keduanya dengan durasi langkah yang SAMA, jadi yang langkahnya lebih sedikit selesai
// duluan. Di bawahnya grafik jumlah operasi untuk n = 5, 10, 20, 40 dan tebakan Big-O.
(function (G) {
  'use strict';
  const P = G.pixel;
  const COLS = ['#d9467c', '#3b82c4'];

  /** Tebakan Big-O dari pertumbuhan operasi (pangkat dari dua ukuran terbesar). */
  function bigO(sizes, work) {
    const pts = sizes.map((n, k) => [n, work[k]]).filter(p => p[1] > 0);
    if (pts.length < 2) return '?';
    const [a, b] = [pts[pts.length - 2], pts[pts.length - 1]];
    const e = Math.log(b[1] / a[1]) / Math.log(b[0] / a[0]);
    if (e < 0.5) return '1';
    if (e < 1.15) return 'n';
    if (e < 1.6) return 'n log n';
    if (e < 2.45) return 'n²';
    return 'n³';
  }

  /** Grafik garis jumlah operasi vs n untuk setiap peserta, di canvas biasa (tajam). */
  function drawChart(cv, sizes, runs, names) {
    const c = cv.getContext('2d');
    const W = cv.width, H = cv.height, L = 52, R = 12, T = 14, B = 30;
    const css = getComputedStyle(document.body);
    const fg = css.getPropertyValue('--vscode-foreground').trim() || '#ccc';
    const muted = css.getPropertyValue('--vscode-descriptionForeground').trim() || '#999';
    c.clearRect(0, 0, W, H);
    const maxW = Math.max(1, ...runs.flatMap(r => r.work.filter(x => x > 0)));
    const maxN = Math.max(...sizes);
    const X = n => L + ((W - L - R) * n) / maxN, Y = w => H - B - ((H - T - B) * w) / maxW;
    c.strokeStyle = muted; c.lineWidth = 1; c.globalAlpha = 0.5;
    c.beginPath(); c.moveTo(L, T); c.lineTo(L, H - B); c.lineTo(W - R, H - B); c.stroke();
    c.globalAlpha = 1;
    c.font = '11px var(--vscode-editor-font-family, monospace)';
    c.fillStyle = muted; c.textAlign = 'center';
    for (const n of sizes) c.fillText(`n=${n}`, X(n), H - B + 14);
    c.fillText('ukuran input', (L + W - R) / 2, H - 4);
    c.textAlign = 'right';
    for (const f of [0, 0.5, 1]) c.fillText(String(Math.round(maxW * f)), L - 6, Y(maxW * f) + 4);
    runs.forEach((r, k) => {
      c.strokeStyle = c.fillStyle = COLS[k % COLS.length];
      c.lineWidth = 2;
      c.beginPath();
      let started = false;
      sizes.forEach((n, j) => {
        const w = r.work[j];
        if (w <= 0) return;
        if (!started) { c.moveTo(X(n), Y(w)); started = true; } else c.lineTo(X(n), Y(w));
      });
      c.stroke();
      sizes.forEach((n, j) => { if (r.work[j] > 0) c.fillRect(Math.round(X(n)) - 3, Math.round(Y(r.work[j])) - 3, 6, 6); });
    });
    c.textAlign = 'left';
    c.fillStyle = fg;
    names.forEach((nm, k) => {
      c.fillStyle = COLS[k % COLS.length];
      c.fillRect(L + 8, T + 4 + k * 14, 8, 8);
      c.fillStyle = fg;
      c.fillText(nm, L + 20, T + 12 + k * 14);
    });
  }

  /** Satu lajur: panggungnya sendiri di canvas tersembunyi. */
  function makeLane(tr, block, blocks, loops) {
    const cv = document.createElement('canvas');
    cv.width = 768; cv.height = 384;
    const info = G.sorts ? G.sorts.detect(tr) : null;
    const coll = tr.collection;
    const scalar = v => v.length <= 12 && !/^(\[|map\[|\{|&)/.test(v);
    let st, view;
    if (info && info.lists) {
      st = new G.MergeStage(cv);
      view = st.prepare(tr);
    } else if (coll && coll.kind !== 'map' && coll.items.every(scalar) && tr.steps.some(s => (s.ops || []).some(o => o.op !== 'read'))) {
      st = new G.DataStage(cv, {});
      view = st.prepare(tr, { vars: [], children: (block.children || []).filter(id => loops.some(l => l.id === id)) }, loops);
    } else {
      st = new G.FuncStage(cv, {});
      st.setBlock(block, blocks);
      view = st.prepare(tr);
    }
    return { cv, st, view, name: block.name || block.id, title: info ? info.title : (block.name || '').toUpperCase(), steps: view.length, done: false };
  }

  class RaceView {
    constructor(canvas) {
      this.cv = canvas;
      this.ctx = canvas.getContext('2d');
      this.active = false;
    }

    /** res = hasil server (input, sizes, runs); blocks/loops = scene file. */
    start(res, blocks, loops) {
      this.res = res;
      this.lanes = res.runs.map(r => {
        const b = blocks.find(x => x.id === r.block) || { id: r.block, name: r.block, kind: 'func', body: [] };
        const ok = r.trace.mode === 'simulated' && r.trace.steps.length;
        return ok ? makeLane(r.trace, b, blocks, loops) : { failed: (r.trace.warnings || [])[0] || 'tidak bisa disimulasikan', name: b.name || b.id, steps: 0 };
      });
      const max = Math.max(1, ...this.lanes.map(l => l.steps));
      this.stepMs = P.clamp(28000 / max, 90, 320); // lomba ±30 detik paling lama, durasi per langkah sama
      this.T = 0;
      this.playing = true;
      this.finishOrder = [];
      this.active = true;
    }

    stop() { this.active = false; }

    restart() { this.T = 0; this.finishOrder = []; this.playing = true; for (const l of this.lanes) l.done = false; }

    update(dt, speed) {
      if (!this.active) return;
      for (const l of this.lanes) if (l.st) l.st.update(dt);
      if (!this.playing) return;
      this.T += dt * (speed || 1);
      for (const [k, l] of this.lanes.entries()) {
        if (!l.st || l.done) continue;
        if (this.T >= l.steps * this.stepMs) { l.done = true; this.finishOrder.push(k); }
      }
    }

    /** Pemenang: lajur dengan langkah paling sedikit (yang gagal tidak ikut dihitung). */
    winner() {
      const ok = this.lanes.map((l, k) => [l, k]).filter(([l]) => l.st);
      if (ok.length < 2) return null;
      const min = Math.min(...ok.map(([l]) => l.steps));
      const best = ok.filter(([l]) => l.steps === min);
      return best.length > 1 ? -1 : best[0][1];
    }

    finished() { return this.lanes.every(l => !l.st || l.done); }

    render() {
      const n = this.lanes.length, LW = 720, GAP = 24, HEAD = 64, FOOT = 48;
      // tinggi lajur mengikuti bentuk panggung tertinggi (dibatasi)
      let LH = 360;
      for (const l of this.lanes) if (l.st) LH = Math.max(LH, Math.min(720, Math.round((LW * l.cv.height) / l.cv.width)));
      const W = n * LW + (n - 1) * GAP, H = HEAD + LH + FOOT;
      if (this.cv.width !== W || this.cv.height !== H) { this.cv.width = W; this.cv.height = H; }
      const c = this.ctx;
      c.setTransform(1, 0, 0, 1, 0, 0);
      c.imageSmoothingEnabled = false;
      c.fillStyle = '#fde3eb';
      c.fillRect(0, 0, W, H);
      const K = 4; // skala teks pixel
      const text = (s, x, y, col, center) => {
        c.save(); c.setTransform(K, 0, 0, K, Math.round(center ? x - (P.textW(s) * K) / 2 : x), y);
        P.text(c, s, 0, 0, col); c.restore();
      };
      const win = this.finished() ? this.winner() : null;
      this.lanes.forEach((l, k) => {
        const x0 = k * (LW + GAP);
        // kepala lajur: nama + penghitung langkah
        c.fillStyle = '#fff8fb'; c.fillRect(x0, 0, LW, HEAD - 4);
        c.fillStyle = COLS[k % COLS.length]; c.fillRect(x0, HEAD - 8, LW, 4);
        text(P.fit(l.name, 26), x0 + 12, 10, P.INK);
        if (!l.st) { text(P.fit('GAGAL: ' + l.failed, 34), x0 + 12, 36, '#d9467c'); return; }
        const i = Math.min(l.steps - 1, Math.floor(this.T / this.stepMs)), t = l.done ? 1 : (this.T / this.stepMs) % 1;
        l.st.render(i, t);
        text(`LANGKAH ${l.done ? l.steps : i + 1}`, x0 + 12, 36, COLS[k % COLS.length]);
        const lh = Math.min(LH, Math.round((LW * l.cv.height) / l.cv.width)), lw = Math.round((lh * l.cv.width) / l.cv.height);
        c.drawImage(l.cv, x0 + Math.round((LW - lw) / 2), HEAD + Math.round((LH - lh) / 2), lw, lh);
        // kaki: bilah kemajuan + bendera finis
        const p = l.done ? 1 : Math.min(1, this.T / (l.steps * this.stepMs));
        c.fillStyle = '#f2c4d2'; c.fillRect(x0, H - FOOT + 14, LW, 12);
        c.fillStyle = COLS[k % COLS.length]; c.fillRect(x0, H - FOOT + 14, Math.round(LW * p), 12);
        if (l.done) {
          const place = this.finishOrder.indexOf(k) + 1;
          const label = win === k ? 'MENANG!' : win === -1 ? 'SERI' : `FINIS #${place}`;
          const img = P.stamp(label, win === k ? '#2f8a5c' : '#c2457a'), S = 5;
          c.drawImage(img, x0 + Math.round(LW / 2 - (img.width * S) / 2), HEAD + 70, img.width * S, img.height * S);
        }
      });
    }
  }

  G.race = { RaceView, bigO, drawChart };
})(window.GV = window.GV || {});
