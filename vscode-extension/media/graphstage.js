// Mode graf: penelusuran DFS/BFS. Simpul disusun berlapis dari simpul awal (lapis = jarak),
// Gopher melompat dari simpul ke simpul yang sedang diproses. Warna simpul: putih = belum,
// kuning = menunggu di antrian/tumpukan, hijau = sudah dikunjungi (angka = urutan kunjungan).
// Di bawah: isi ANTRIAN (BFS, depan di kiri) atau TUMPUKAN (DFS, atas di kanan).
(function (G) {
  'use strict';
  const P = G.pixel;

  const TOP = 30;      // ruang HUD + Gopher yang berdiri di simpul lapis atas
  const DX = 44;       // jarak antar lapis
  const DY = 24;       // jarak antar simpul dalam satu lapis
  const R = 7;         // jari-jari simpul
  const BAR = 30;      // tinggi bar antrian/tumpukan
  const SLOT = 15;

  class GraphStage {
    constructor(canvas, hooks) {
      this.cv = canvas;
      this.ctx = canvas.getContext('2d');
      this.world = document.createElement('canvas');
      this.wx = this.world.getContext('2d');
      this.bg = document.createElement('canvas');
      this.hooks = hooks || {};
      this.clock = 0;
      this.parts = [];
      this.views = [];
    }

    prepare(trace) {
      const g = trace.graph;
      this.g = g;
      // tetangga tanpa arah (untuk tata letak)
      const nb = {};
      g.nodes.forEach(n => { nb[n] = []; });
      for (const [a, b] of g.edges) { nb[a].push(b); nb[b].push(a); }
      // keadaan per langkah (langkah tanpa potret graf memakai keadaan sebelumnya)
      let st = { cur: '', visited: [], frontier: [] };
      const order = [];
      this.views = trace.steps.map(s => {
        // simpul aktif yang kosong (variabelnya sudah keluar cakupan) memakai yang terakhir
        if (s.graph) st = { cur: s.graph.cur || st.cur, visited: s.graph.visited || [], frontier: s.graph.frontier || [] };
        for (const v of st.visited) if (!order.includes(v)) order.push(v);
        if (st.cur && !order.includes(st.cur) && !g.visited) order.push(st.cur);
        return { step: s, cur: st.cur, visited: new Set(st.visited), frontier: st.frontier, order: order.slice() };
      });
      // akar = simpul pertama yang diproses
      const first = this.views.find(v => v.cur || v.frontier.length);
      const root = first ? first.cur || first.frontier[0] : g.nodes[0];
      const layer = { [root]: 0 };
      const queue = [root];
      while (queue.length) {
        const x = queue.shift();
        for (const y of nb[x]) if (layer[y] == null) { layer[y] = layer[x] + 1; queue.push(y); }
      }
      let maxL = Math.max(0, ...Object.values(layer));
      g.nodes.forEach(n => { if (layer[n] == null) layer[n] = maxL + 1; });
      maxL = Math.max(0, ...Object.values(layer));
      const layers = [];
      g.nodes.forEach(n => { (layers[layer[n]] = layers[layer[n]] || []).push(n); });
      const tallest = Math.max(...layers.map(l => (l ? l.length : 0)));
      const gh = Math.max(56, tallest * DY + 12);
      this.W = Math.max(192, 48 + maxL * DX);
      this.H = TOP + gh + BAR;
      const x0 = Math.round((this.W - maxL * DX) / 2);
      this.pos = {};
      layers.forEach((l, k) => {
        (l || []).forEach((n, j) => {
          this.pos[n] = { x: x0 + k * DX, y: Math.round(TOP + gh / 2 + (j - (l.length - 1) / 2) * DY) };
        });
      });
      this.world.width = this.bg.width = this.W;
      this.world.height = this.bg.height = this.H;
      P.paintSky(this.bg.getContext('2d'), this.W, this.H);
      return this.views;
    }

    duration(i) {
      const v = this.views[i], prev = this.views[i - 1];
      if (i === this.views.length - 1) return 2000;
      if (prev && v.cur !== prev.cur && v.cur) return 900;
      if (prev && (v.visited.size !== prev.visited.size || v.frontier.join() !== prev.frontier.join())) return 650;
      return 420;
    }

    cues(i) {
      const v = this.views[i], prev = this.views[i - 1], out = [];
      if (prev && v.cur && v.cur !== prev.cur) out.push({ t: 0.05, sound: 'jump' }, { t: 0.5, sound: 'land' });
      if (prev && v.visited.size > prev.visited.size) {
        const added = [...v.visited].find(n => !prev.visited.has(n));
        out.push({ t: 0.55, sound: 'coin', fx: () => { const p = this.pos[added]; if (p) this.burst(p.x, p.y - 8, ['#9fd88f', '#ffffff']); } });
      }
      if (prev && v.frontier.length > prev.frontier.length) out.push({ t: 0.3, sound: 'blip' });
      if (v.step.output) out.push({ t: 0.6, sound: 'coin' });
      if (i === this.views.length - 1) out.push({ t: 0.2, sound: 'jingle', fx: () => this.confetti() });
      return out;
    }

    burst(x, y, cols) {
      for (let k = 0; k < 10; k++) this.parts.push({ x, y, vx: (Math.random() - 0.5) * 60, vy: -Math.random() * 50, g: 80, life: 600, col: cols[k % cols.length] });
    }
    confetti() {
      const cols = ['#f5a3bf', '#f7b779', '#f1d064', '#9fd88f', '#8ec4f0', '#b8a2ee', '#ffffff'];
      for (let k = 0; k < 60; k++) this.parts.push({ x: Math.random() * this.W, y: -Math.random() * 20, vx: (Math.random() - 0.5) * 20, vy: 18 + Math.random() * 25, g: 8, life: 3000, col: cols[k % cols.length] });
    }

    update(dt) {
      this.clock += dt;
      const f = dt / 1000;
      this.parts = this.parts.filter(p => { p.vy += p.g * f; p.x += p.vx * f; p.y += p.vy * f; p.life -= dt; return p.life > 0; });
    }

    line(w, a, b, col) {
      let x0 = Math.round(a.x), y0 = Math.round(a.y);
      const x1 = Math.round(b.x), y1 = Math.round(b.y);
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

    render(i, t) {
      const v = this.views[i];
      if (!v) return;
      const w = this.wx, now = this.clock, g = this.g, prev = this.views[i - 1];
      const last = i === this.views.length - 1;
      P.drawBg(w, this.bg);

      // sisi; sisi yang sedang dilewati Gopher berwarna merah muda
      const moving = prev && prev.cur && v.cur && prev.cur !== v.cur ? [prev.cur, v.cur] : null;
      const drawn = new Set();
      for (const [a, b] of g.edges) {
        const key = a < b ? a + '|' + b : b + '|' + a;
        if (!g.directed && drawn.has(key)) continue;
        drawn.add(key);
        const hot = moving && (moving[0] === a && moving[1] === b || !g.directed && moving[0] === b && moving[1] === a);
        const done = v.visited.has(a) && v.visited.has(b);
        this.line(w, this.pos[a], this.pos[b], hot ? '#d9467c' : done ? '#7cc68b' : '#d8b4c8');
        if (g.directed) { // mata panah kecil dekat ujung
          const pa = this.pos[a], pb = this.pos[b], d = Math.hypot(pb.x - pa.x, pb.y - pa.y) || 1;
          const ax = pb.x - (pb.x - pa.x) / d * (R + 2), ay = pb.y - (pb.y - pa.y) / d * (R + 2);
          P.rect(w, Math.round(ax) - 1, Math.round(ay) - 1, 3, 3, hot ? '#d9467c' : '#b08aa0');
        }
      }

      // simpul
      const inFront = new Set(v.frontier);
      for (const n of g.nodes) {
        const p = this.pos[n], cur = n === v.cur;
        const fill = v.visited.has(n) ? '#9fd88f' : inFront.has(n) ? '#f3c75a' : '#fff8fb';
        if (cur) P.disc(w, p.x, p.y, R + 2, '#d9467c');
        P.disc(w, p.x, p.y, R, P.INK);
        P.disc(w, p.x, p.y, R - 1, fill);
        P.textCenter(w, String(n).slice(0, 3), p.x, p.y - 2, P.INK);
        const k = v.order.indexOf(n);
        if (k >= 0 && v.visited.has(n)) { // urutan kunjungan
          const lbl = String(k + 1);
          P.box(w, p.x + R - 3, p.y + R - 4, P.textW(lbl) + 4, 8, '#ffffff', '#2f8a5c');
          P.text(w, lbl, p.x + R - 1, p.y + R - 2, '#2f8a5c');
        }
      }

      // Gopher di atas simpul yang sedang diproses (melompat dari simpul sebelumnya)
      if (v.cur) {
        let p = this.pos[v.cur], hop = 0, walking = false;
        if (moving && t < 0.5) {
          const a = this.pos[moving[0]], u = P.ease(t / 0.5);
          p = { x: P.lerp(a.x, p.x, u), y: P.lerp(a.y, p.y, u) };
          hop = Math.sin(Math.PI * u) * 10;
          walking = true;
        }
        P.drawGopher(w, Math.round(p.x), Math.round(p.y - R - hop), last ? 'happy' : 'idle', walking ? Math.floor(now / 140) % 2 : 0);
      }

      // antrian / tumpukan
      const by = this.H - BAR + 4;
      P.rect(w, 0, by - 3, this.W, 1, '#f2c4d2');
      const stack = g.kind === 'dfs';
      if (g.frontier) {
        const label = stack ? 'STACK' : 'QUEUE';
        P.text(w, label, 4, by + 1, '#8a6a98');
        const sx = 4 + P.textW(label) + 6;
        v.frontier.forEach((n, k) => {
          const x = sx + k * (SLOT + 1);
          if (x + SLOT > this.W - 4) return;
          const edge = stack ? k === v.frontier.length - 1 : k === 0; // yang akan diambil berikutnya
          P.box(w, x, by - 2, SLOT, 12, edge ? '#f3c75a' : '#fff8fb', P.INK);
          P.textCenter(w, String(n).slice(0, 3), x + SLOT / 2, by + 1, P.INK);
        });
        if (!v.frontier.length) P.text(w, 'EMPTY', sx, by + 1, '#c58ba3');
        P.text(w, stack ? 'TOP ON THE RIGHT' : 'FRONT ON THE LEFT', 4, by + 13, '#c58ba3');
      } else if (g.kind === 'dfs') {
        P.text(w, 'RECURSIVE DFS', 4, by + 4, '#8a6a98');
      }

      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), 1, 1, p.col);

      // HUD
      P.rect(w, 0, 0, this.W, 16, 'rgba(255,248,251,0.75)');
      P.text(w, g.kind ? g.kind.toUpperCase() + ' ' + g.var : g.var, 4, 3, P.INK);
      P.text(w, `VISITED ${v.visited.size}/${g.nodes.length}`, 4, 10, '#c2457a');
      if (last) {
        const img = P.stamp('DONE!', '#2f8a5c');
        w.drawImage(img, Math.round(this.W - img.width - 4), this.H - BAR + 2, img.width, img.height);
      }
      P.present(this.ctx, this.cv, this.world, this.W, this.H);
    }
  }

  G.GraphStage = GraphStage;
})(window.GV = window.GV || {});
