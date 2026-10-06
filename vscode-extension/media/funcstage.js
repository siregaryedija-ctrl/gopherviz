// Mode lantai: fungsi, rekursi, dan timeline seluruh program. Setiap pemanggilan fungsi
// = Gopher masuk pipa dan turun ke lantai baru (call stack tumbuh ke bawah); return =
// koin berisi nilai kembali melayang naik ke lantai pemanggil dan lantai itu hilang.
(function (G) {
  'use strict';
  const P = G.pixel;

  const TOP = 16;     // ruang HUD
  const FLOOR = 36;   // jarak antar lantai
  const X_PIPE = 12;  // pipa masuk di kiri setiap lantai
  const X_TILES = 40; // ubin pertama
  const TILE = 18;

  class FuncStage {
    constructor(canvas, hooks) {
      this.cv = canvas;
      this.ctx = canvas.getContext('2d');
      this.world = document.createElement('canvas');
      this.wx = this.world.getContext('2d');
      this.bg = document.createElement('canvas');
      this.hooks = hooks || {};
      this.clock = 0;
      this.parts = [];
      this.cam = { x: 96, y: 48 };
      this.snaps = [];
      this.blocks = [];
    }

    groundY(d) { return TOP + 30 + d * FLOOR; }

    /** block = fungsi fokus; blocks = semua blok di file (untuk mencari isi fungsi lain). */
    setBlock(block, blocks) {
      this.block = block;
      this.blocks = blocks || [];
      this.layout(1, [block]);
      this.snaps = [];
    }

    funcBlock(name) {
      if (name === this.block.name) return this.block;
      return this.blocks.find(b => b.kind === 'func' && b.name === name) || null;
    }

    layout(floors, funcs) {
      const maxTiles = Math.max(1, ...funcs.map(f => (f && f.body ? f.body.length : 1)));
      this.exitX = X_TILES + maxTiles * TILE + 8;
      this.W = Math.max(192, this.exitX + 24);
      this.H = Math.max(96, this.groundY(floors - 1) + 12);
      this.world.width = this.W; this.world.height = this.H;
      this.bg.width = this.W; this.bg.height = this.H;
      P.paintSky(this.bg.getContext('2d'), this.W, this.H);
    }

    prepare(trace) {
      const views = [];
      const stack = []; // lantai yang sedang ada: { fn, args, block, x }
      let x = X_PIPE, depth = 0, maxDepth = 0;
      const used = new Set([this.block]);
      for (const s of trace.steps) {
        const d = s.depth || 0;
        const v = { step: s, from: { x, d: depth }, act: 'hit', d, x, tile: -1 };
        if (s.reason === 'panic') v.act = 'panic';
        else if (s.phase === 'call') {
          stack.length = d;
          const fb = this.funcBlock(s.func);
          if (fb) used.add(fb);
          stack[d] = { fn: s.func, args: s.args || '', block: fb, callerX: x };
          v.act = 'call'; v.x = X_PIPE + 10;
        } else if (s.phase === 'return') {
          v.act = 'return'; v.x = this.exitX - 6; v.ret = s.return;
          v.callerX = stack[d] ? stack[d].callerX : X_PIPE;
        } else {
          const fb = stack[d] ? stack[d].block : this.block;
          const ti = fb ? P.nodeAt(fb.body, s.line) : -1;
          v.tile = ti;
          v.x = ti >= 0 ? X_TILES + ti * TILE : x;
        }
        v.stack = stack.slice(0, Math.max(d + 1, 1)).map(f => Object.assign({}, f));
        if (v.act === 'return') { x = v.callerX; depth = Math.max(0, d - 1); }
        else { x = v.x; depth = d; }
        maxDepth = Math.max(maxDepth, d);
        views.push(v);
      }
      this.layout(maxDepth + 1, [...used]);
      this.snaps = views;
      return views;
    }

    duration(i) {
      const v = this.snaps[i], last = i === this.snaps.length - 1;
      return { call: 900, 'return': last ? 1900 : 1100, hit: 700, panic: 2000 }[v.act] || 700;
    }

    cues(i) {
      const v = this.snaps[i], out = [];
      const last = i === this.snaps.length - 1;
      if (v.act === 'call') out.push({ t: 0.1, sound: 'pipe' });
      if (v.act === 'return') {
        out.push({ t: 0.1, sound: 'coin' });
        out.push({ t: 0.5, sound: 'warp' });
        if (last) out.push({ t: 0.7, sound: 'jingle', fx: () => this.confetti() });
      }
      if (v.act === 'hit') { out.push({ t: 0, sound: 'jump' }); out.push({ t: 0.5, sound: 'blip' }); }
      if (v.act === 'panic') out.push({ t: 0, sound: 'panic' });
      if (v.step.output) out.push({ t: 0.6, sound: 'coin' });
      return out;
    }

    confetti() {
      const cols = ['#f5a3bf', '#f7b779', '#f1d064', '#9fd88f', '#8ec4f0', '#b8a2ee', '#ffffff'];
      for (let k = 0; k < 50; k++) {
        this.parts.push({ x: this.cam.x + (Math.random() - 0.5) * 180, y: this.cam.y - 50 - Math.random() * 20, vx: (Math.random() - 0.5) * 20, vy: 18 + Math.random() * 25, g: 8, life: 3000, col: cols[k % cols.length] });
      }
    }

    update(dt) {
      this.clock += dt;
      const f = dt / 1000;
      this.parts = this.parts.filter(p => { p.vy += p.g * f; p.x += p.vx * f; p.y += p.vy * f; p.life -= dt; return p.life > 0; });
    }

    renderIdle() {
      const v = { act: 'idle', d: 0, x: X_PIPE + 10, tile: -1, step: {}, stack: [{ fn: this.block.name, args: (this.block.params || []).join(', '), block: this.block }] };
      this.draw(v, { x: X_PIPE + 10, y: this.groundY(0) }, 1, false, 1, 'idle');
    }

    render(i, t) {
      const v = this.snaps[i];
      if (!v) return;
      let pos = { x: v.x, y: this.groundY(v.d) }, a = 1, walking = false, alpha = 1;
      const from = { x: v.from.x, y: this.groundY(v.from.d) };
      if (v.act === 'call') {
        // masuk pipa di lantai pemanggil, lalu jatuh dari pipa di lantai baru
        if (t < 0.3) { pos = { x: from.x, y: from.y }; alpha = 1 - t / 0.3; a = 0; }
        else if (t < 0.6) {
          const u = (t - 0.3) / 0.3;
          pos = { x: X_PIPE + 10, y: P.lerp(this.groundY(v.d) - 26, this.groundY(v.d), P.bounce(u)) };
          alpha = 1; a = 0;
        } else a = (t - 0.6) / 0.4;
      } else if (v.act === 'return') {
        // jalan ke ujung lantai, koin melayang naik, lalu Gopher muncul lagi di lantai pemanggil
        const dist = Math.abs(v.x - from.x);
        const wf = dist > 1 ? Math.min(0.35, dist / 120) : 0;
        if (t < wf) { pos = { x: P.lerp(from.x, v.x, t / wf), y: from.y }; walking = true; a = 0; }
        else if (t < 0.65) { pos = { x: v.x, y: this.groundY(v.d) }; alpha = 1 - (t - wf) / (0.65 - wf); a = (t - wf) / (1 - wf); }
        else { pos = { x: v.callerX, y: this.groundY(Math.max(0, v.d - 1)) }; alpha = v.d === 0 ? 0 : (t - 0.65) / 0.35; a = (t - wf) / (1 - wf); }
      } else {
        const sameFloor = v.from.d === v.d;
        const dist = sameFloor ? Math.abs(v.x - from.x) : 0;
        const wf = dist > 1 ? Math.min(0.5, dist / 90) : 0;
        if (t < wf) { pos = { x: P.lerp(from.x, v.x, t / wf), y: pos.y }; walking = true; a = 0; }
        else a = wf < 1 ? (t - wf) / (1 - wf) : 1;
      }
      let face = this.clock % 3400 < 130 ? 'blink' : 'idle';
      if (v.act === 'return') face = 'happy';
      if (v.act === 'panic') face = 'shock';
      this.draw(v, pos, a, walking, alpha, face);
    }

    draw(v, pos, a, walking, alpha, face) {
      const w = this.wx, now = this.clock;
      w.drawImage(this.bg, 0, 0);
      const floors = v.stack && v.stack.length ? v.stack : [{ fn: this.block.name, block: this.block }];

      floors.forEach((f, d) => {
        const gy = this.groundY(d);
        const active = d === v.d;
        w.globalAlpha = active ? 1 : 0.45;
        // lantai
        P.rect(w, 2, gy, this.exitX + 12, 2, '#d7a2c9');
        P.rect(w, 2, gy + 2, this.exitX + 12, 2, '#efd3ea');
        // pipa masuk dari langit-langit
        const pipeTop = d === 0 ? TOP + 2 : this.groundY(d - 1) + 4;
        P.rect(w, X_PIPE - 5, pipeTop, 10, gy - 22 - pipeTop, '#5a43a0');
        P.rect(w, X_PIPE - 4, pipeTop, 8, gy - 22 - pipeTop, '#b8a2ee');
        P.rect(w, X_PIPE - 6, gy - 24, 12, 3, '#5a43a0');
        // label lantai: nama fungsi + argumen
        const label = `${d}. ${f.fn || ''}(${f.args || ''})`.toUpperCase().slice(0, 34);
        P.text(w, label, X_TILES - 8, gy - 21, active ? '#5a43a0' : '#b090c8');
        // ubin isi fungsi
        const body = f.block && f.block.body ? f.block.body : [];
        if (!body.length) P.text(w, f.block ? 'KOSONG' : 'FUNGSI LUAR', X_TILES - 6, gy - 7, '#c58ba3');
        body.forEach((n, j) => {
          const hit = active && v.act === 'hit' && v.tile === j;
          P.tile(w, X_TILES + j * TILE, gy, n.kind, String(j + 1), hit && a > 0.45 ? (a - 0.45) / 0.4 : 0, false);
        });
        // tempat koin return di ujung kanan
        P.rect(w, this.exitX, gy - 3, 8, 3, '#e0a22a');
        w.globalAlpha = 1;
      });

      // koin return yang melayang ke lantai pemanggil
      if (v.act === 'return' && a > 0 && a < 1) {
        const y0 = this.groundY(v.d) - 10, y1 = v.d > 0 ? this.groundY(v.d - 1) - 18 : TOP;
        const cy = P.lerp(y0, y1, P.ease(a)), cx = P.lerp(this.exitX + 4, v.callerX, P.ease(a));
        P.disc(w, Math.round(cx), Math.round(cy), 4, '#e0a22a');
        P.disc(w, Math.round(cx), Math.round(cy), 3, '#f3c75a');
        if (v.ret) {
          const r = String(v.ret).slice(0, 8);
          P.box(w, Math.round(cx + 6), Math.round(cy - 4), P.textW(r) + 4, 9, '#fff8fb', '#e0a22a');
          P.text(w, r, Math.round(cx + 8), Math.round(cy - 2), P.INK);
        }
      }

      const jump = v.act === 'hit' && v.tile >= 0 ? Math.round(Math.sin(Math.PI * a) * 8) : 0;
      P.drawGopher(w, pos.x, pos.y - jump, face, walking ? Math.floor(now / 140) % 2 : 0, alpha);

      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), 1, 1, p.col);

      // kamera dulu, supaya HUD bisa digambar di pojok layar (bukan di pojok dunia)
      this.cam.x += (pos.x + 50 - this.cam.x) * 0.12;
      this.cam.y += (pos.y - 30 - this.cam.y) * 0.12;
      const ox = Math.round(P.clamp(this.cam.x - 96, 0, Math.max(0, this.W - 192)));
      const oy = Math.round(P.clamp(this.cam.y - 48, 0, Math.max(0, this.H - 96)));
      const s = v.step || {};
      P.rect(w, ox, oy, 192, 16, 'rgba(255,248,251,0.75)');
      P.text(w, `${this.block.id} ${(this.block.name || 'FUNGSI').toUpperCase()}`, ox + 4, oy + 3, P.INK);
      P.text(w, `CALL STACK ${floors.length}`, ox + 4, oy + 10, '#c2457a');
      if (s.return != null && v.act === 'return') P.text(w, `RETURN ${String(s.return).slice(0, 12)}`, ox + 80, oy + 10, '#2f8a5c');
      P.present(this.ctx, this.cv, this.world, this.W, this.H, this.cam.x, this.cam.y);
      if (v.act === 'panic' && a > 0.2) {
        const img = P.stamp('PANIC!', '#d9467c'), S = this.cv.width / 192;
        this.ctx.drawImage(img, Math.round(this.cv.width / 2 - img.width * S), Math.round(24 * S), img.width * 2 * S, img.height * 2 * S);
      }
    }
  }

  G.FuncStage = FuncStage;
})(window.GV = window.GV || {});
