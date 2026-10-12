// Mode class (Fase 10a, keputusan user 2026-10-08: "cetak biru + rumah"). Class = kertas cetak biru
// berisi daftar field dan method; setiap object = rumah yang dibangun dari cetak biru itu, kamarnya
// = field (kubus berisi nilai). Panggil method = Gopher masuk ke rumah object itu (this/self);
// field yang berubah berkedip; field berisi object lain = tali ke rumah itu (mis. linked list).
(function (G) {
  'use strict';
  const P = G.pixel;

  const TOP = 20;      // ruang HUD
  const BP_X = 4;      // cetak biru
  const ROW = 10;      // tinggi satu kamar (field)
  const DOOR = 24;     // ruang pintu di bawah kamar: Gopher berdiri di ambang pintu tanpa menutupi kamar
  const MAX_ROWS = 7;  // kamar yang digambar per rumah
  const ROOF_COL = ['#f39a8a', '#8ec4f0', '#9fd88f', '#b8a2ee', '#f7b779', '#86d6cb'];
  const CTOR = ['__init__', 'constructor', '__construct'];

  /** Teks nilai satu kamar; field berisi object lain = ">" + nama object tujuannya. */
  const valText = (f, nameOf) => (f.points ? '>' + P.fit((nameOf && nameOf(f.points)) || f.points, 9) : P.fit(f.value, 10));

  /** Lebar dan tinggi rumah untuk daftar field (tanpa papan nama). */
  function houseSize(fields) {
    const rows = Math.min(MAX_ROWS, Math.max(1, fields.length));
    let inner = 30;
    for (const f of fields.slice(0, MAX_ROWS)) inner = Math.max(inner, P.textW(P.fit(f.name, 8)) + P.textW(valText(f)) + 14);
    const w = inner + 6;
    const roof = Math.min(14, Math.ceil(w / 4));
    return { w, body: 6 + rows * ROW + DOOR, roof, h: roof + 6 + rows * ROW + DOOR };
  }

  /**
   * Menggambar satu rumah object. x = tepi kiri badan, gy = tanah. o = { name, class, fields }.
   * opt: roof (warna atap), size (houseSize), flash (Set nama field yang baru berubah), old (nilai
   * lama per field, ditampilkan sebelum berubah), self (Gopher sedang di dalam), copy (salinan),
   * now (jam animasi). Mengembalikan posisi kamar per field (untuk tali pointer).
   */
  function drawHouse(w, x, gy, o, opt) {
    const S = opt.size, top = gy - S.h, bodyY = gy - S.body;
    const rows = {};
    // atap bertingkat
    for (let r = 0; r < S.roof; r++) {
      const half = Math.round(((r + 1) / S.roof) * (S.w / 2 + 3));
      P.rect(w, Math.round(x + S.w / 2 - half), top + r, half * 2, 1, r === S.roof - 1 ? P.mix(opt.roof, P.INK, 0.35) : opt.roof);
    }
    const cls = P.fit(o.class || '', Math.floor((S.w - 8) / 4));
    if (S.roof >= 9 && cls) P.textCenter(w, cls, x + S.w / 2, top + S.roof - 7, '#ffffff');
    // badan
    P.box(w, x, bodyY, S.w, S.body, opt.copy ? '#f4f0fb' : '#fff8fb', opt.self ? '#c2457a' : P.INK);
    if (opt.copy) for (let k = 2; k < S.w - 2; k += 3) P.rect(w, x + k, bodyY + 1, 1, 1, '#b8a2ee');
    // kamar = field
    const fields = o.fields || [];
    fields.slice(0, MAX_ROWS).forEach((f, j) => {
      const ry = bodyY + 4 + j * ROW;
      const flash = opt.flash && opt.flash.has(f.name);
      const val = opt.old && opt.old[f.name] != null ? opt.old[f.name] : f;
      P.text(w, P.fit(f.name, 8), x + 4, ry + 2, '#8a6a98');
      const vt = valText(val, opt.nameOf), cw = P.textW(vt) + 5, cx = x + S.w - cw - 6;
      const col = flash && Math.floor(opt.now / 110) % 2 ? '#f3c75a' : f.points ? '#f2c4d2' : '#cfe3f7';
      P.cube(w, cx, ry + 1, cw, 7, col, 2, flash ? '#d9467c' : null);
      P.text(w, vt, cx + 3, ry + 2, P.INK);
      rows[f.name] = { x: cx + cw + 2, y: ry + 4 };
    });
    if (fields.length > MAX_ROWS) P.text(w, `+${fields.length - MAX_ROWS}`, x + 4, bodyY + 4 + MAX_ROWS * ROW - 2, '#8a6a98');
    // pintu setinggi Gopher (terbuka gelap bila Gopher sedang di dalam)
    const dx = Math.round(x + S.w / 2 - 9), dy = gy - 21;
    P.rect(w, dx, dy, 18, 21, P.INK);
    P.rect(w, dx + 1, dy + 1, 16, 20, opt.self ? '#6b5577' : '#d7a2b8');
    if (!opt.self) { P.rect(w, dx + 8, dy + 1, 1, 20, '#c58ba3'); P.rect(w, dx + 12, dy + 11, 1, 2, '#f3c75a'); }
    return { rows, door: { x: x + S.w / 2, y: gy }, top };
  }

  /** Garis piksel putus-putus (tali pointer). */
  function dashLine(w, x0, y0, x1, y1, col) {
    x0 = Math.round(x0); y0 = Math.round(y0); x1 = Math.round(x1); y1 = Math.round(y1);
    const dx = Math.abs(x1 - x0), dy = -Math.abs(y1 - y0), sx = x0 < x1 ? 1 : -1, sy = y0 < y1 ? 1 : -1;
    let err = dx + dy, k = 0;
    w.fillStyle = col;
    for (;;) {
      if (k++ % 4 < 3) w.fillRect(x0, y0, 1, 1);
      if (x0 === x1 && y0 === y1) break;
      const e2 = 2 * err;
      if (e2 >= dy) { err += dy; x0 += sx; }
      if (e2 <= dx) { err += dx; y0 += sy; }
    }
  }

  class ClassStage {
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
      this.info = null;
    }

    /** Blok class dari Scene (sebelum trace datang): cetak biru dengan judul saja. */
    setBlock(block) {
      this.block = block;
      this.info = { name: block.name || '', fields: [], methods: [] };
      this.houses = [];
      this.layout();
      this.snaps = [];
    }

    /** Baris-baris cetak biru. */
    bpLines() {
      const c = this.info, lines = [{ t: 'CLASS ' + P.fit(c.name, 14), kind: 'title' }];
      if (c.super) lines.push({ t: '<- ' + P.fit(c.super, 14), kind: 'super' });
      lines.push({ t: 'FIELD', kind: 'head' });
      (c.fields.length ? c.fields : ['-']).slice(0, 10).forEach(f => lines.push({ t: P.fit(f, 14), kind: 'field', name: f }));
      lines.push({ t: 'METHOD', kind: 'head' });
      (c.methods.length ? c.methods : ['-']).slice(0, 10).forEach(m => lines.push({ t: P.fit(m, 12) + (m === '-' ? '' : '()'), kind: 'method', name: m }));
      if (c.subs && c.subs.length) lines.push({ t: 'SUBCLASS: ' + P.fit(c.subs.join(','), 12), kind: 'super' });
      return lines;
    }

    layout() {
      const lines = this.bpLines();
      this.bp = { x: BP_X, y: TOP + 2, w: Math.max(60, ...lines.map(l => P.textW(l.t) + 10)), h: lines.length * 7 + 8, lines };
      const n = this.houses.length;
      const cols = n <= 4 ? Math.max(1, n) : Math.ceil(Math.sqrt(n * 1.6));
      const rows = Math.max(1, Math.ceil(n / cols));
      const sw = Math.max(52, ...this.houses.map(h => h.size.w + 12));
      const rh = Math.max(46, ...this.houses.map(h => h.size.h + 20));
      const hx0 = this.bp.x + this.bp.w + 14;
      this.houses.forEach((h, k) => {
        const c = k % cols, r = Math.floor(k / cols);
        h.x = Math.round(hx0 + c * sw + (sw - h.size.w) / 2);
        h.gy = TOP + 6 + (r + 1) * rh - 6;
      });
      this.home = { x: this.bp.x + this.bp.w / 2, y: this.bp.y + this.bp.h + 26 };
      this.W = Math.max(192, hx0 + cols * sw + 4);
      this.H = Math.max(96, this.home.y + 6, TOP + 6 + rows * rh) + 12;
      this.world.width = this.W; this.world.height = this.H;
      this.bg.width = this.W; this.bg.height = this.H;
      const c = this.bg.getContext('2d');
      P.paintSky(c, this.W, this.H);
      for (const h of this.houses) { // jalan di depan setiap baris rumah
        P.rect(c, hx0 - 6, h.gy, this.W - hx0 + 6, 2, '#e8a9bf');
        P.rect(c, hx0 - 6, h.gy + 2, this.W - hx0 + 6, 2, '#f7d3dd');
      }
      P.rect(c, 0, this.home.y, hx0 - 6, 2, '#e8a9bf');
      P.rect(c, 0, this.home.y + 2, hx0 - 6, 2, '#f7d3dd');
    }

    prepare(trace) {
      this.info = Object.assign({ fields: [], methods: [] }, trace.class || { name: this.block ? this.block.name : '' });
      // field yang hanya terlihat saat program berjalan (mis. dari class induk) ikut ke cetak biru
      const ids = [], all = {};
      for (const s of trace.steps) {
        for (const o of s.objs || []) {
          if (!all[o.id]) { all[o.id] = []; ids.push(o.id); }
          for (const f of o.fields) if (!this.info.fields.includes(f.name) && o.class === this.info.name) this.info.fields.push(f.name);
          if (o.fields.length >= all[o.id].length) all[o.id] = o.fields.map(f => ({ name: f.name, value: f.value, points: f.points }));
        }
      }
      // atap: warna per class (class fokus dulu)
      const classes = [this.info.name];
      for (const s of trace.steps) for (const o of s.objs || []) if (!classes.includes(o.class)) classes.push(o.class);
      // lebar rumah dari isi terpanjang yang pernah muncul
      const widest = {};
      for (const s of trace.steps) for (const o of s.objs || []) {
        const cur = widest[o.id] || { fields: [] };
        const fs = o.fields.map(f => ({ name: f.name, value: f.value, points: f.points }));
        const sz = houseSize(fs);
        if (!widest[o.id] || sz.w > cur.size.w || sz.h > cur.size.h) widest[o.id] = { fields: fs, size: sz };
      }
      this.houses = ids.map(id => ({ id, size: widest[id].size }));
      this.roofOf = cls => ROOF_COL[Math.max(0, classes.indexOf(cls)) % ROOF_COL.length];
      this.layout();

      const views = [];
      let prev = {}, stack = [], at = { x: this.home.x, y: this.home.y };
      const named = new Set(); // object yang pernah dipegang variabel
      trace.steps.forEach((s, i) => {
        const objs = {};
        for (const o of s.objs || []) { objs[o.id] = o; if (o.name) named.add(o.id); }
        const born = Object.keys(objs).filter(id => !prev[id]);
        const changed = {}, old = {};
        for (const id in objs) {
          if (!prev[id]) continue;
          const before = {};
          for (const f of prev[id].fields) before[f.name] = f;
          for (const f of objs[id].fields) {
            const b = before[f.name];
            if (!b || b.value !== f.value || b.points !== f.points) {
              (changed[id] = changed[id] || new Set()).add(f.name);
              if (b) (old[id] = old[id] || {})[f.name] = { value: b.value, points: b.points };
            }
          }
        }
        if (s.phase === 'call') stack.push(s.func || '');
        const method = stack.length ? stack[stack.length - 1].split('.').pop() : '';
        let act = 'hit';
        if (s.phase === 'call') act = 'call';
        else if (s.phase === 'return') act = 'return';
        else if (s.phase === 'exit') act = 'exit';
        if (s.reason === 'panic') act = 'panic';
        // Gopher: di rumah self; di luar method ke rumah yang baru dibangun/diubah; selain itu di cetak biru
        const target = s.self || born[0] || Object.keys(changed)[0] || '';
        const h = this.houses.find(x => x.id === target);
        const pos = h ? { x: h.x + h.size.w / 2, y: h.gy } : { x: this.home.x, y: this.home.y };
        views.push({ step: s, i, objs, born, changed, old, method, act, self: s.self || '', copy: !!s.selfCopy, from: at, pos, named: new Set(named) });
        if (s.phase === 'return') stack.pop();
        at = pos;
        prev = objs;
      });
      this.snaps = views;
      return views;
    }

    duration(i) {
      const v = this.snaps[i], last = i === this.snaps.length - 1;
      if (v.act === 'panic') return 2000;
      if (v.act === 'exit') return last ? 2200 : 900;
      if (v.born.length) return 1400;
      if (v.act === 'call') return 950;
      if (v.act === 'return') return 1000;
      if (Object.keys(v.changed).length) return 950;
      return 650;
    }

    cues(i) {
      const v = this.snaps[i], out = [];
      const last = i === this.snaps.length - 1;
      if (v.born.length) {
        out.push({ t: 0.05, sound: 'sparkle' });
        out.push({ t: 0.7, sound: 'land', fx: () => v.born.forEach(id => this.dust(id)) });
      }
      if (v.act === 'call') out.push({ t: 0.25, sound: 'door' });
      if (v.act === 'return') out.push({ t: 0.2, sound: 'coin' });
      if (Object.keys(v.changed).length && !v.born.length) out.push({ t: 0.35, sound: 'blip' });
      if (v.act === 'panic') out.push({ t: 0, sound: 'panic' });
      if (v.step.output) out.push({ t: 0.6, sound: 'coin' });
      if (last && v.act === 'exit') out.push({ t: 0.1, sound: 'jingle', fx: () => this.confetti() });
      return out;
    }

    dust(id) {
      const h = this.houses.find(x => x.id === id);
      if (!h) return;
      for (let k = 0; k < 14; k++) {
        this.parts.push({ x: h.x + Math.random() * h.size.w, y: h.gy - 1, vx: (Math.random() - 0.5) * 50, vy: -Math.random() * 25, g: 60, life: 600, col: k % 2 ? '#e8c8d4' : '#ffffff' });
      }
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

    /** Tanpa trace: cetak biru saja, Gopher menunggu. */
    renderIdle() {
      const v = { step: { note: this.block ? this.block.header : '' }, objs: {}, born: [], changed: {}, old: {}, method: '', act: 'idle', self: '', copy: false, from: this.home, pos: this.home, i: -1 };
      this.draw(v, 1);
    }

    render(i, t) {
      const v = this.snaps[i];
      if (v) this.draw(v, t);
    }

    drawBlueprint(w, v) {
      const b = this.bp;
      P.box(w, b.x, b.y, b.w, b.h, '#5f8fd6', '#2f5a9e');
      w.fillStyle = '#7aa5e3';
      for (let x = b.x + 4; x < b.x + b.w - 1; x += 6) w.fillRect(x, b.y + 1, 1, b.h - 2);
      for (let y = b.y + 4; y < b.y + b.h - 1; y += 6) w.fillRect(b.x + 1, y, b.w - 2, 1);
      const ctor = v.method && (v.method === this.info.name || CTOR.includes(v.method));
      b.lines.forEach((l, k) => {
        const y = b.y + 5 + k * 7;
        const on = l.kind === 'method' && v.method && (l.name === v.method || (ctor && (l.name === this.info.name || CTOR.includes(l.name))));
        if (on) P.rect(w, b.x + 2, y - 1, b.w - 4, 7, '#f3c75a');
        const col = on ? P.INK : l.kind === 'title' ? '#ffffff' : l.kind === 'head' ? '#d6e6ff' : l.kind === 'super' ? '#fff3a8' : '#ffffff';
        P.text(w, l.t, b.x + 5, y, col);
        if (l.kind === 'title') P.rect(w, b.x + 4, y + 6, b.w - 8, 1, '#d6e6ff');
      });
    }

    draw(v, t) {
      const w = this.wx, now = this.clock, s = v.step;
      P.drawBg(w, this.bg);
      this.drawBlueprint(w, v);

      // rumah-rumah
      const where = {};
      const flashOn = t > 0.35;
      for (const h of this.houses) {
        const o = v.objs[h.id];
        if (!o) continue;
        const born = v.born.includes(h.id);
        const rise = born ? P.ease(P.clamp((t - 0.25) / 0.45, 0, 1)) : 1;
        if (rise <= 0) continue;
        w.save();
        w.beginPath(); w.rect(0, 0, this.W, h.gy); w.clip();
        const dy = Math.round((1 - rise) * (h.size.h + 4));
        w.translate(0, dy);
        const r = drawHouse(w, h.x, h.gy, o, {
          size: h.size, roof: this.roofOf(o.class), now,
          flash: flashOn && !born ? v.changed[h.id] : null,
          old: !flashOn ? v.old[h.id] : null,
          self: v.self === h.id && v.act !== 'exit', copy: v.copy && v.self === h.id,
          nameOf: id => (v.objs[id] && v.objs[id].name) || id,
        });
        w.restore();
        where[h.id] = r;
        // papan nama: variabel yang memegang object ini
        // tanpa variabel: "baru" selama dibangun (belum disimpan ke variabel), "tanpa nama" bila sudah
        // tidak dipegang variabel lagi atau object sementara (mis. hasil a + b)
        const fresh = !(v.named && v.named.has(h.id)) && (born || v.self === h.id);
        const label = P.fit(o.name || (fresh ? 'new' : 'unnamed'), Math.floor((h.size.w + 10) / 4));
        const lw = P.textW(label) + 6, ly = r.top - 11 + dy;
        P.box(w, Math.round(h.x + h.size.w / 2 - lw / 2), ly, lw, 9, v.self === h.id ? '#fff3a8' : '#ffffff', P.INK);
        P.textCenter(w, label, h.x + h.size.w / 2, ly + 2, P.INK);
        if (v.copy && v.self === h.id) P.textCenter(w, 'COPY', h.x + h.size.w / 2, h.gy + 5, '#8a6ac8');
      }

      // tali pointer: field berisi object lain
      for (const h of this.houses) {
        const o = v.objs[h.id], r = where[h.id];
        if (!o || !r) continue;
        for (const f of o.fields) {
          const to = f.points && where[f.points];
          if (!to || !r.rows[f.name]) continue;
          const th = this.houses.find(x => x.id === f.points);
          dashLine(w, r.rows[f.name].x, r.rows[f.name].y, th.x + th.size.w / 2, to.top - 12, '#c2457a');
          P.rect(w, Math.round(th.x + th.size.w / 2) - 1, to.top - 13, 3, 3, '#c2457a');
        }
      }

      // kertas cetak biru terbang ke rumah baru
      for (const id of v.born) {
        const h = this.houses.find(x => x.id === id);
        const u = P.clamp(t / 0.3, 0, 1);
        if (!h || u >= 1) continue;
        const x = P.lerp(this.bp.x + this.bp.w / 2, h.x + h.size.w / 2, P.ease(u));
        const y = P.lerp(this.bp.y + 8, h.gy - h.size.h, P.ease(u)) - Math.sin(Math.PI * u) * 14;
        P.rect(w, Math.round(x) - 5, Math.round(y) - 4, 10, 8, '#5f8fd6');
        P.rect(w, Math.round(x) - 4, Math.round(y) - 2, 8, 1, '#d6e6ff');
        P.rect(w, Math.round(x) - 4, Math.round(y) + 1, 6, 1, '#d6e6ff');
      }

      // Gopher berjalan (melompat) ke tujuan, lalu berdiri di depan pintu
      const wf = Math.hypot(v.pos.x - v.from.x, v.pos.y - v.from.y) > 2 ? 0.3 : 0;
      const u = wf ? P.clamp(t / wf, 0, 1) : 1;
      const gx = P.lerp(v.from.x, v.pos.x, P.ease(u));
      const gy = P.lerp(v.from.y, v.pos.y, P.ease(u)) - (u < 1 ? Math.round(Math.sin(Math.PI * u) * 12) : 0);
      let face = now % 3400 < 130 ? 'blink' : 'idle';
      if (v.act === 'call' || v.born.length) face = 'happy';
      if (Object.keys(v.changed).length && flashOn) face = 'happy';
      if (v.act === 'panic') face = 'shock';
      const head = P.drawGopher(w, gx, gy, face, u < 1 ? Math.floor(now / 120) % 2 : 0);

      // balon method yang dipanggil / koin nilai return
      if (v.act === 'call' && t > 0.15) {
        // di atas atap rumah self (bukan di kepala Gopher, supaya kamar tidak tertutup)
        const name = (s.func || '').split('.').pop();
        const txt = P.fit(`${name}(${s.args || ''})`, 22), bw = P.textW(txt) + 6;
        const roofTop = where[v.self] ? where[v.self].top - 23 : head - 14;
        const bx = Math.round(P.clamp(gx - bw / 2, 2, this.W - bw - 2)), by = Math.max(TOP, roofTop);
        P.box(w, bx, by, bw, 9, '#ffffff', P.INK);
        P.text(w, txt, bx + 3, by + 2, '#5a43a0');
      }
      if (v.act === 'return' && s.return != null && s.return !== '') {
        const base = where[v.self] ? where[v.self].top - 14 : head - 6;
        const a = P.clamp((t - 0.1) / 0.8, 0, 1), cy = Math.max(TOP, Math.round(base - a * 10));
        const r = P.fit(s.return, 10);
        P.disc(w, Math.round(gx) - 8, cy + 3, 3, '#f3c75a');
        P.box(w, Math.round(gx) - 4, cy - 1, P.textW(r) + 4, 9, '#fff8fb', '#e0a22a');
        P.text(w, r, Math.round(gx) - 2, cy + 1, P.INK);
      }

      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), 1, 1, p.col);

      // HUD + kode yang sedang dijalankan
      P.rect(w, 0, 0, this.W, 17, 'rgba(255,248,251,0.8)');
      const id = this.block ? this.block.id : '';
      P.text(w, `${id} CLASS ${P.fit(this.info.name, 16)}`, 4, 3, P.INK);
      const n = Object.keys(v.objs).length;
      P.text(w, `OBJECT ${n}` + (v.self && v.act !== 'exit' ? `  SELF = ${P.fit((v.objs[v.self] || {}).name || 'new', 10)}` : ''), 4, 10, '#c2457a');
      const code = (s.note || '').replace(/ → .*$/, '');
      if (code) {
        P.rect(w, 0, this.H - 10, this.W, 10, 'rgba(255,248,251,0.85)');
        P.text(w, P.fit('> ' + code, Math.floor((this.W - 8) / 4)), 4, this.H - 8, P.INK);
      }
      if (v.act === 'exit' && v.i === this.snaps.length - 1) {
        const img = P.stamp('DONE!', '#2f8a5c');
        w.drawImage(img, Math.round(this.W - img.width * 2 - 4), 2, img.width * 2, img.height * 2);
      }
      P.present(this.ctx, this.cv, this.world, this.W, this.H);
      if (v.act === 'panic' && t > 0.2) {
        const img = P.stamp('PANIC!', '#d9467c'), K = this.cv.__k || 4;
        this.ctx.drawImage(img, Math.round(this.cv.width / 2 - img.width * K), Math.round(24 * K), img.width * 2 * K, img.height * 2 * K);
      }
    }
  }

  G.ClassStage = ClassStage;
  G.house = { size: houseSize, draw: drawHouse };
})(window.GV = window.GV || {});
