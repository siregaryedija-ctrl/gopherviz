// Mode pohon (Fase 11b, keputusan user 2026-10-09: "pohon + Gopher memanjat"). Pohon digambar dari
// atas (akar) ke bawah; Gopher turun ke anak kiri/kanan dan naik lagi saat fungsi kembali, jadi
// rekursinya terlihat. Jalur dari akar ke simpul aktif menyala, simpul yang sudah dikunjungi diberi
// nomor urut, anak kosong (None/null) tampil sebagai lingkaran putus-putus, nilai return menempel di
// simpulnya, dan pita bawah menunjukkan urutan nilai yang dicetak (preorder/inorder/postorder).
(function (G) {
  'use strict';
  const P = G.pixel;

  const TOP = 20;
  const SX = 24;  // jarak antar kolom simpul
  const SY = 30;  // jarak antar tingkat
  const R = 8;    // jari-jari simpul

  /** Garis piksel padat. */
  function line(w, x0, y0, x1, y1, col) {
    x0 = Math.round(x0); y0 = Math.round(y0); x1 = Math.round(x1); y1 = Math.round(y1);
    const dx = Math.abs(x1 - x0), dy = -Math.abs(y1 - y0), sx = x0 < x1 ? 1 : -1, sy = y0 < y1 ? 1 : -1;
    let err = dx + dy;
    w.fillStyle = col;
    for (;;) {
      w.fillRect(x0, y0, 1, 1);
      if (x0 === x1 && y0 === y1) break;
      const e2 = 2 * err;
      if (e2 >= dy) { err += dy; x0 += sx; }
      if (e2 <= dx) { err += dx; y0 += sy; }
    }
  }

  function ring(w, cx, cy, r, col, dashed) {
    w.fillStyle = col;
    for (let a = 0; a < 64; a++) {
      if (dashed && a % 4 >= 2) continue;
      const t = (a / 64) * Math.PI * 2;
      w.fillRect(Math.round(cx + Math.cos(t) * r), Math.round(cy + Math.sin(t) * r), 1, 1);
    }
  }

  class TreeStage {
    constructor(canvas, hooks) {
      this.cv = canvas;
      this.ctx = canvas.getContext('2d');
      this.world = document.createElement('canvas');
      this.wx = this.world.getContext('2d');
      this.bg = document.createElement('canvas');
      this.hooks = hooks || {};
      this.clock = 0;
      this.parts = [];
      this.snaps = [];
    }

    prepare(trace) {
      const t = trace.tree;
      this.t = t;
      this.byId = {};
      t.nodes.forEach(n => { this.byId[n.id] = n; });
      this.layout();
      const views = [];
      const stack = [];          // simpul per kedalaman panggilan (id, atau { nullOf, side })
      const order = new Map();   // id → urutan kunjungan
      const nulls = {};          // id induk → berapa anak kosong sudah dikunjungi
      const badges = {};         // id → nilai return
      let printed = '';
      let prev = null;
      trace.steps.forEach((s, i) => {
        const d = s.depth || 0;
        const st = s.node || null;
        let cur = null;
        if (st && st.cur) {
          cur = st.cur;
          if (!order.has(cur)) order.set(cur, order.size + 1);
        } else if (st && st.null) {
          const parent = typeof stack[d - 1] === 'string' ? stack[d - 1] : null;
          if (parent) {
            const pn = this.byId[parent];
            const k = (nulls[parent] = (nulls[parent] || 0) + (s.phase === 'call' ? 1 : 0));
            // anak kosong pertama = kiri bila kiri memang kosong, berikutnya kanan
            const side = !pn.left && (k <= 1 || pn.right) ? 'left' : 'right';
            cur = { nullOf: parent, side };
          }
        }
        if (cur) {
          stack.length = d;
          stack[d] = cur;
        }
        if (s.phase === 'return' && typeof stack[d] === 'string' && s.return != null && s.return !== '' && !/^(None|null|undefined|nil)$/.test(s.return)) {
          badges[stack[d]] = s.return;
        }
        printed += s.output || '';
        const path = stack.slice(0, d + 1).filter(x => typeof x === 'string');
        const here = stack[d] || cur || prev;
        views.push({ step: s, i, here, path, order: new Map(order), badges: Object.assign({}, badges), printed, from: prev });
        prev = here;
      });
      this.snaps = views;
      return views;
    }

    /** Posisi simpul: inorder untuk pohon biner (x), rata-rata anak untuk pohon umum; y = tingkat. */
    layout() {
      const pos = {};
      let col = 0, maxDepth = 0;
      const place = (id, depth) => {
        const n = this.byId[id];
        if (!n) return 0;
        maxDepth = Math.max(maxDepth, depth);
        if (n.kids && n.kids.length) {
          const xs = n.kids.map(k => place(k, depth + 1));
          pos[id] = { x: (xs[0] + xs[xs.length - 1]) / 2, depth };
          return pos[id].x;
        }
        if (n.left) place(n.left, depth + 1); else if (n.right) col += 0.5;
        pos[id] = { x: col++, depth };
        if (n.right) place(n.right, depth + 1); else if (n.left) col += 0.5;
        return pos[id].x;
      };
      place(this.t.root, 0);
      const cols = Math.max(1, col);
      // label panjang (pohon umum: nama folder) butuh kolom lebih lebar
      const sx = Math.max(SX, ...this.t.nodes.map(n => P.textW(P.fit(n.label, 4)) + 12));
      this.sx = sx;
      this.W = Math.max(192, cols * sx + 40);
      this.H = TOP + 34 + maxDepth * SY + SY + 30; // + slot anak kosong (NONE) di bawah tingkat terakhir
      const x0 = (this.W - (cols - 1) * sx) / 2;
      this.pos = {};
      for (const id in pos) this.pos[id] = { x: x0 + pos[id].x * sx, y: TOP + 34 + pos[id].depth * SY };
      this.world.width = this.W; this.world.height = this.H;
      this.bg.width = this.W; this.bg.height = this.H;
      P.paintSky(this.bg.getContext('2d'), this.W, this.H);
    }

    /** Titik untuk simpul id, atau slot anak kosong. */
    spot(h) {
      if (!h) return this.pos[this.t.root];
      if (typeof h === 'string') return this.pos[h];
      const p = this.pos[h.nullOf];
      return { x: p.x + (h.side === 'left' ? -this.sx / 2 : this.sx / 2), y: p.y + SY * 0.75, empty: true };
    }

    duration(i) {
      const v = this.snaps[i], last = i === this.snaps.length - 1;
      if (last) return 2000;
      if (v.step.reason === 'panic') return 2000;
      if (v.step.phase === 'call') return 800;
      if (v.step.phase === 'return') return 750;
      return 550;
    }

    cues(i) {
      const v = this.snaps[i], out = [];
      const last = i === this.snaps.length - 1;
      if (v.step.phase === 'call') out.push({ t: 0.05, sound: 'jump' });
      if (v.step.phase === 'return') out.push({ t: 0.2, sound: 'coin' });
      if (v.step.output) out.push({ t: 0.5, sound: 'blip' });
      if (v.step.reason === 'panic') out.push({ t: 0, sound: 'panic' });
      if (last && v.step.reason !== 'panic') out.push({ t: 0.3, sound: 'jingle', fx: () => this.confetti() });
      return out;
    }

    confetti() {
      const cols = ['#f5a3bf', '#f7b779', '#f1d064', '#9fd88f', '#8ec4f0', '#b8a2ee', '#ffffff'];
      for (let k = 0; k < 60; k++) {
        this.parts.push({ x: Math.random() * this.W, y: -2 - Math.random() * 20, vx: (Math.random() - 0.5) * 20, vy: 18 + Math.random() * 25, g: 8, life: 3000, col: cols[k % cols.length] });
      }
    }

    update(dt) {
      this.clock += dt;
      const f = dt / 1000;
      this.parts = this.parts.filter(p => { p.vy += p.g * f; p.x += p.vx * f; p.y += p.vy * f; p.life -= dt; return p.life > 0; });
    }

    render(i, t) {
      const v = this.snaps[i];
      if (!v) return;
      const w = this.wx, now = this.clock, s = v.step;
      const last = i === this.snaps.length - 1;
      P.drawBg(w, this.bg);
      const onPath = new Set(v.path);
      // sisi: jalur akar → simpul aktif menyala
      for (const n of this.t.nodes) {
        const a = this.pos[n.id];
        for (const c of [n.left, n.right, ...(n.kids || [])]) {
          if (!c || !this.pos[c]) continue;
          const b = this.pos[c];
          const lit = onPath.has(n.id) && onPath.has(c);
          line(w, a.x, a.y, b.x, b.y, lit ? '#d9467c' : '#c9b3d8');
          if (lit) line(w, a.x + 1, a.y, b.x + 1, b.y, '#d9467c');
        }
      }
      // slot anak kosong yang sedang dikunjungi
      const here = this.spot(v.here);
      if (here && here.empty) {
        const parent = this.pos[v.here.nullOf];
        line(w, parent.x, parent.y, here.x, here.y - 5, '#e8a9bf');
        ring(w, here.x, here.y, 5, '#c58ba3', true);
        P.textCenter(w, 'NONE', here.x, here.y + 8, '#c58ba3');
      }
      // simpul
      for (const n of this.t.nodes) {
        const p = this.pos[n.id];
        const visited = v.order.has(n.id);
        const active = v.here === n.id;
        const fill = active ? '#f5a3bf' : visited ? '#9fd88f' : '#ffffff';
        P.disc(w, Math.round(p.x), Math.round(p.y), R, onPath.has(n.id) ? '#d9467c' : P.INK);
        P.disc(w, Math.round(p.x), Math.round(p.y), R - 1, fill);
        P.textCenter(w, P.fit(n.label, 4), p.x, p.y - 2, P.INK);
        if (visited && !active) { // nomor urut kunjungan
          const o = String(v.order.get(n.id));
          P.box(w, Math.round(p.x + R - 3), Math.round(p.y - R - 3), P.textW(o) + 4, 8, '#ffffff', '#2f8a5c');
          P.text(w, o, Math.round(p.x + R - 1), Math.round(p.y - R - 1), '#2f8a5c');
        }
        if (v.badges[n.id] != null) { // nilai return
          const r = P.fit(v.badges[n.id], 6);
          P.box(w, Math.round(p.x - P.textW(r) / 2 - 2), Math.round(p.y + R + 1), P.textW(r) + 4, 8, '#fff8fb', '#e0a22a');
          P.textCenter(w, r, p.x, Math.round(p.y + R + 3), '#a5622a');
        }
      }
      // Gopher memanjat: dari simpul sebelumnya ke simpul ini
      const from = v.from ? this.spot(v.from) : here;
      const u = from && here && (Math.abs(from.x - here.x) + Math.abs(from.y - here.y) > 1) ? P.clamp(t / 0.35, 0, 1) : 1;
      if (here) {
        const gx = P.lerp(from.x, here.x, P.ease(u));
        // kaki di puncak lingkaran simpul (labelnya tetap terlihat)
        const gy = P.lerp(from.y, here.y, P.ease(u)) - (here.empty ? 5 : R) - 1 - Math.round(Math.sin(Math.PI * u) * 6);
        const face = here.empty ? 'shock' : s.phase === 'return' ? 'happy' : s.reason === 'panic' ? 'shock' : (now % 3400 < 130 ? 'blink' : 'idle');
        P.drawGopher(w, gx, gy, face, u < 1 ? Math.floor(now / 120) % 2 : 0);
      }
      for (const pt of this.parts) P.rect(w, Math.round(pt.x), Math.round(pt.y), 1, 1, pt.col);

      // HUD
      P.rect(w, 0, 0, this.W, 17, 'rgba(255,248,251,0.8)');
      P.text(w, `DFS POHON  ${this.t.nodes.length} SIMPUL`, 4, 3, P.INK);
      P.text(w, `DIKUNJUNGI ${v.order.size}/${this.t.nodes.length}  KEDALAMAN ${v.path.length}`, 4, 10, '#c2457a');
      // pita urutan: nilai yang dicetak (beda preorder/inorder/postorder), atau urutan kunjungan
      const printed = v.printed.trim().split(/\s+/).filter(Boolean);
      const seq = printed.length ? 'CETAK: ' + printed.join(' ') : 'URUTAN: ' + [...v.order.keys()].map(id => this.byId[id].label).join(' ');
      P.rect(w, 0, this.H - 20, this.W, 10, 'rgba(255,248,251,0.85)');
      P.text(w, P.fit(seq, Math.floor((this.W - 8) / 4)), 4, this.H - 18, '#2f8a5c');
      const code = (s.note || '').replace(/ → .*$/, '');
      P.rect(w, 0, this.H - 10, this.W, 10, 'rgba(255,248,251,0.85)');
      if (code) P.text(w, P.fit('> ' + code, Math.floor((this.W - 8) / 4)), 4, this.H - 8, P.INK);
      if (last && s.reason !== 'panic') {
        const img = P.stamp('SELESAI!', '#2f8a5c');
        w.drawImage(img, Math.round(this.W - img.width * 2 - 4), 2, img.width * 2, img.height * 2);
      }
      P.present(this.ctx, this.cv, this.world, this.W, this.H);
      if (s.reason === 'panic' && t > 0.2) {
        const img = P.stamp('PANIC!', '#d9467c'), K = this.cv.__k || 4;
        this.ctx.drawImage(img, Math.round(this.cv.width / 2 - img.width * K), Math.round(24 * K), img.width * 2 * K, img.height * 2 * K);
      }
    }
  }

  G.TreeStage = TreeStage;
})(window.GV = window.GV || {});
