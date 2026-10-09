// Mode labirin (Fase 11a, keputusan user 2026-10-09: "labirin kubus + gelombang"). Grid = lantai
// kubus: tembok = kubus tinggi gelap, jalan = ubin. BFS menyebar seperti gelombang dari titik awal
// (angka = jarak), antrian kuning, Gopher di sel yang sedang diproses. Setelah tujuan ketemu,
// jalur terpendek menyala dan Gopher berjalan dari awal ke tujuan.
(function (G) {
  'use strict';
  const P = G.pixel;

  const TOP = 20;
  const WAVE = ['#9fd88f', '#86d6cb', '#8ec4f0', '#b8a2ee', '#f5a3bf', '#f7b779'];
  const WALL_HINT = new Set(['#', 'X', 'x', '█', '*', 'W']);

  const key = (r, c) => r * 1000 + c;

  class GridStage {
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
      const g = trace.grid;
      this.g = g;
      this.R = g.rows.length;
      this.C = g.rows[0].length;
      const steps = trace.steps;
      // sel yang ditandai dikunjungi → nilainya bisa dilewati; sel bernilai lain = tembok. Sel aktif
      // tidak dipakai: DFS rekursif juga dipanggil untuk tembok/air lalu langsung kembali (mis. hitung pulau).
      const ever = new Set();
      const touch = (r, c) => ever.add(key(r, c));
      for (const s of steps) (s.cells && s.cells.visited || []).forEach(([r, c]) => touch(r, c));
      if (!ever.size) {
        for (const s of steps) {
          const st = s.cells || {};
          (st.frontier || []).forEach(([r, c]) => touch(r, c));
          if (st.cur) touch(st.cur[0], st.cur[1]);
        }
      }
      const open = new Set();
      ever.forEach(k => open.add(g.rows[Math.floor(k / 1000)][k % 1000]));
      if (g.start) open.add(g.rows[g.start[0]][g.start[1]]);
      if (g.goal) open.add(g.rows[g.goal[0]][g.goal[1]]);
      this.wall = (r, c) => {
        const v = g.rows[r][c];
        return open.size ? !open.has(v) : WALL_HINT.has(v);
      };
      // jarak BFS dari titik awal (angka gelombang) dan urutan kunjungan (DFS)
      this.dist = new Map();
      if (g.start) {
        const q = [[g.start[0], g.start[1]]];
        this.dist.set(key(g.start[0], g.start[1]), 0);
        while (q.length) {
          const [r, c] = q.shift();
          for (const [dr, dc] of [[1, 0], [-1, 0], [0, 1], [0, -1]]) {
            const nr = r + dr, nc = c + dc;
            if (nr < 0 || nc < 0 || nr >= this.R || nc >= this.C || this.wall(nr, nc) || this.dist.has(key(nr, nc))) continue;
            this.dist.set(key(nr, nc), this.dist.get(key(r, c)) + 1);
            q.push([nr, nc]);
          }
        }
      }
      this.order = new Map();
      const views = [];
      let prevSeen = new Set(), prevCur = null;
      steps.forEach((s, i) => {
        const st = s.cells || {};
        const seen = new Set((st.visited || []).map(([r, c]) => key(r, c)));
        if (st.cur && !this.wall(st.cur[0], st.cur[1])) seen.add(key(st.cur[0], st.cur[1]));
        const fresh = [...seen].filter(k => !prevSeen.has(k));
        fresh.forEach(k => { if (!this.order.has(k)) this.order.set(k, this.order.size + 1); });
        const front = new Set((st.frontier || []).map(([r, c]) => key(r, c)));
        const cur = st.cur ? [st.cur[0], st.cur[1]] : prevCur;
        views.push({ step: s, i, seen, fresh: new Set(fresh), front, cur, from: prevCur });
        prevSeen = seen;
        prevCur = cur;
      });
      // akhir: jalur terpendek ke tujuan bila tujuan sudah dikunjungi
      this.path = null;
      const last = views[views.length - 1];
      if (g.goal && g.start && last && last.seen.has(key(g.goal[0], g.goal[1])) && this.dist.has(key(g.goal[0], g.goal[1]))) {
        const path = [[g.goal[0], g.goal[1]]];
        let [r, c] = g.goal;
        while (this.dist.get(key(r, c)) > 0) {
          const d = this.dist.get(key(r, c));
          const nb = [[1, 0], [-1, 0], [0, 1], [0, -1]].map(([dr, dc]) => [r + dr, c + dc])
            .find(([nr, nc]) => this.dist.get(key(nr, nc)) === d - 1);
          if (!nb) break;
          [r, c] = nb;
          path.unshift([r, c]);
        }
        this.path = path;
      }
      this._md = null;
      this.openCells = 0;
      for (let r = 0; r < this.R; r++) for (let c = 0; c < this.C; c++) if (!this.wall(r, c)) this.openCells++;
      this.layout();
      this.snaps = views;
      return views;
    }

    layout() {
      const R = this.R, C = this.C;
      this.p = P.clamp(Math.floor(Math.min(300 / C, 170 / R)), 8, 26);
      this.d = this.p >= 12 ? 3 : 2;
      const gw = C * this.p, gh = R * this.p;
      this.W = Math.max(192, gw + 24);
      this.H = TOP + 12 + gh + this.d + 14;
      this.x0 = Math.round((this.W - gw) / 2);
      this.y0 = TOP + 10 + this.d;
      this.world.width = this.W; this.world.height = this.H;
      this.bg.width = this.W; this.bg.height = this.H;
      const c = this.bg.getContext('2d');
      P.paintSky(c, this.W, this.H);
      P.box(c, this.x0 - 4, this.y0 - this.d - 4, gw + 8 + this.d, gh + this.d + 8, '#fff4f7', '#f2c4d2');
    }

    cellXY(r, c) { return { x: this.x0 + c * this.p, y: this.y0 + r * this.p }; }

    duration(i) {
      const v = this.snaps[i], last = i === this.snaps.length - 1;
      if (last) return 1800 + (this.path ? this.path.length * 160 : 0);
      if (v.step.reason === 'panic') return 2000;
      if (v.fresh.size) return 600;
      return 420;
    }

    cues(i) {
      const v = this.snaps[i], out = [];
      const last = i === this.snaps.length - 1;
      if (v.fresh.size) out.push({ t: 0.1, sound: 'blip' });
      if (v.step.reason === 'panic') out.push({ t: 0, sound: 'panic' });
      if (last) {
        if (this.path) out.push({ t: 0.05, sound: 'sparkle' });
        out.push({ t: 0.9, sound: 'jingle', fx: () => this.confetti() });
      }
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
      const w = this.wx, now = this.clock, g = this.g, p = this.p, d = this.d;
      const last = i === this.snaps.length - 1;
      w.drawImage(this.bg, 0, 0);
      const bfs = g.kind !== 'dfs';
      const pathOn = last && this.path;
      const pathSet = new Set(pathOn ? this.path.map(([r, c]) => key(r, c)) : []);
      const pathU = pathOn ? P.clamp(t * (1800 + this.path.length * 160) / (this.path.length * 160 + 400), 0, 1) : 0;
      // lantai & tembok: baris belakang dulu (sisi atas tembok tertutup baris depan)
      for (let r = 0; r < this.R; r++) {
        for (let c = 0; c < this.C; c++) {
          const { x, y } = this.cellXY(r, c);
          const k = key(r, c);
          if (this.wall(r, c)) {
            P.cube(w, x, y - 3, p - 1 - d + 2, p - 1 - d + 5, '#6b5577', d);
            continue;
          }
          let col = '#fbeef3';
          const seen = v.seen.has(k);
          if (seen) {
            const n = bfs ? (this.dist.get(k) || 0) : (this.order.get(k) || 1);
            col = WAVE[Math.min(WAVE.length - 1, Math.floor(n / Math.max(1, Math.ceil((bfs ? this.maxDist() : this.order.size) / WAVE.length))))];
          }
          if (v.front.has(k) && !seen) col = '#f3c75a';
          if (pathSet.has(k)) col = Math.floor(now / 160) % 2 ? '#f3c75a' : '#ffe08a';
          // sel yang baru dikunjungi: muncul membesar (gelombang)
          const pop = v.fresh.has(k) ? P.ease(P.clamp(t / 0.35, 0, 1)) : 1;
          const s = Math.max(2, Math.round((p - 1) * (0.55 + 0.45 * pop)));
          const off = Math.round((p - 1 - s) / 2);
          P.rect(w, x + off, y + off, s, s, col);
          P.rect(w, x + off, y + off + s - 1, s, 1, P.mix(col, P.INK, 0.25));
          if (v.front.has(k) && !seen) P.rect(w, x + Math.floor(p / 2) - 1, y + Math.floor(p / 2) - 1, 2, 2, '#c2457a');
          if (seen && p >= 12) {
            const n = String(bfs ? (this.dist.has(k) ? this.dist.get(k) : '') : this.order.get(k) || '');
            if (P.textW(n) <= p - 3) P.textCenter(w, n, x + (p - 1) / 2, y + Math.floor((p - 6) / 2), P.INK);
          }
        }
      }
      // penanda awal (S) dan tujuan (G)
      const flag = (rc, col, label) => {
        if (!rc) return;
        const { x, y } = this.cellXY(rc[0], rc[1]);
        P.rect(w, x + 2, y - 7, 1, 9, P.INK);
        P.rect(w, x + 3, y - 7, 5, 4, col);
        if (p >= 12) P.text(w, label, x + p - 6, y + p - 7, col === '#d9467c' ? '#d9467c' : '#2f8a5c');
      };
      flag(g.start, '#d9467c', 'S');
      flag(g.goal, '#2f8a5c', 'G');

      // Gopher: di sel aktif (melompat dari sel sebelumnya); di akhir berjalan menyusuri jalur
      let pos = null, walk = false;
      if (pathOn) {
        const idx = pathU * (this.path.length - 1);
        const a = this.path[Math.floor(idx)], b = this.path[Math.min(this.path.length - 1, Math.floor(idx) + 1)];
        const f = idx - Math.floor(idx);
        pos = { r: P.lerp(a[0], b[0], f), c: P.lerp(a[1], b[1], f) };
        walk = pathU < 1;
      } else if (v.cur) {
        const u = v.from ? P.clamp(t / 0.3, 0, 1) : 1;
        const from = v.from || v.cur;
        pos = { r: P.lerp(from[0], v.cur[0], P.ease(u)), c: P.lerp(from[1], v.cur[1], P.ease(u)) };
        walk = u < 1;
      }
      if (pos) {
        const gx = this.x0 + pos.c * p + (p - 1) / 2;
        const gy = this.y0 + pos.r * p + p - 2 - (walk ? Math.round(Math.abs(Math.sin(now / 90)) * 2) : 0);
        const face = pathOn && pathU >= 1 ? 'happy' : v.step.reason === 'panic' ? 'shock' : 'idle';
        P.drawGopher(w, gx, gy, face, walk ? Math.floor(now / 120) % 2 : 0);
      }
      for (const pt of this.parts) P.rect(w, Math.round(pt.x), Math.round(pt.y), 1, 1, pt.col);

      // HUD
      P.rect(w, 0, 0, this.W, 17, 'rgba(255,248,251,0.8)');
      const total = this.openCells;
      P.text(w, `${bfs ? 'BFS' : 'DFS'} LABIRIN ${this.R}x${this.C}`, 4, 3, P.INK);
      let info = `DIKUNJUNGI ${v.seen.size}/${total}`;
      if (v.front.size) info += `  ${bfs ? 'ANTRIAN' : 'TUMPUKAN'} ${v.front.size}`;
      if (pathOn) info += `  JARAK ${this.path.length - 1}`;
      P.text(w, P.fit(info, Math.floor((this.W - 8) / 4)), 4, 10, '#c2457a');
      const code = (v.step.note || '').replace(/ → .*$/, '');
      if (code) {
        P.rect(w, 0, this.H - 10, this.W, 10, 'rgba(255,248,251,0.85)');
        P.text(w, P.fit('> ' + code, Math.floor((this.W - 8) / 4)), 4, this.H - 8, P.INK);
      }
      if (last && v.step.reason !== 'panic') {
        const img = P.stamp(pathOn ? 'KETEMU!' : 'SELESAI!', '#2f8a5c');
        w.drawImage(img, Math.round(this.W - img.width * 2 - 4), 2, img.width * 2, img.height * 2);
      }
      P.present(this.ctx, this.cv, this.world, this.W, this.H);
      if (v.step.reason === 'panic' && t > 0.2) {
        const img = P.stamp('PANIC!', '#d9467c'), K = this.cv.__k || 4;
        this.ctx.drawImage(img, Math.round(this.cv.width / 2 - img.width * K), Math.round(24 * K), img.width * 2 * K, img.height * 2 * K);
      }
    }

    maxDist() {
      if (this._md == null) this._md = Math.max(1, ...this.dist.values());
      return this._md;
    }
  }

  G.GridStage = GridStage;
})(window.GV = window.GV || {});
