// Mode persimpangan: if / else if / else dan switch. Setiap cabang = satu jalur bertingkat.
// Gopher menuruni tangga sambil mengecek papan kondisi dari atas; cabang pertama yang
// true pintunya terbuka dan Gopher berjalan menyusuri ubin-ubin isinya (berlabel nama perintahnya,
// mis. PRINT, BREAK). Jalur lain meredup. Baris paling bawah = kode yang sedang dijalankan.
(function (G) {
  'use strict';
  const P = G.pixel;

  const LANE = 32;    // jarak antar jalur
  const X_START = 8;  // posisi awal gopher di jalur pertama
  const X_SHAFT = 20; // tangga turun antar jalur
  const X_SIGN = 30;  // papan kondisi
  const X_DOOR = 76;  // pintu jalur
  const X_TILES = 86; // tepi kiri ubin pertama
  const LABEL = { 'if': 'IF', 'else-if': 'ELSE IF', 'else': 'ELSE', 'case': 'CASE', 'default': 'DEFAULT', 'skip': 'SKIP' };
  class BranchStage {
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
    }

    groundY(k) { return this.top + 30 + k * LANE; }

    /** Membangun jalur dari blok if/switch (Scene). Cabang "LEWATI" ditambahkan bila tidak ada else/default. */
    setBlock(block) {
      this.block = block;
      const brs = block.branches || [];
      this.lanes = brs.map((b, k) => ({ k, kind: b.kind, label: b.label, line: b.line, body: b.body || [] }));
      if (!brs.some(b => b.kind === 'else' || b.kind === 'default')) {
        this.lanes.push({ k: this.lanes.length, kind: 'skip', label: '', line: 0, body: [] });
      }
      // ubin selebar labelnya; jalur kosong selebar tulisannya (supaya tidak tertimpa pipa keluar)
      let end = X_TILES + 16;
      for (const ln of this.lanes) {
        let x = X_TILES;
        ln.tiles = ln.body.map(n => {
          const label = P.stmtLabel(n), w = Math.max(12, P.textW(label) + 6);
          const t = { x, w, cx: x + w / 2, label };
          x += w + 3;
          return t;
        });
        ln.empty = ln.body.length ? '' : ln.kind === 'skip' ? 'NOTHING MATCHES' : 'EMPTY';
        end = Math.max(end, ln.empty ? X_TILES + P.textW(ln.empty) + 4 : x + 1);
      }
      this.exitX = end;
      // variabel yang disebut di kepala blok atau label cabang → tampil di HUD
      const text = [block.header, ...brs.map(b => b.label)].join(' ');
      this.hudVars = [...new Set((text.match(/[A-Za-z_]\w*/g) || []).filter(n => !/^(if|else|switch|case|default|true|false|nil|len)$/.test(n)))];
      this.layout(1);
      this.snaps = [];
    }

    /** Ukuran dunia; HUD variabel bisa beberapa baris (yang tidak muat pindah ke baris bawahnya). */
    layout(hudLines) {
      this.hudLines = hudLines;
      this.top = 16 + (hudLines - 1) * 7;
      this.W = Math.max(192, this.exitX + 20);
      this.H = Math.max(96, this.groundY(this.lanes.length - 1) + 16);
      this.world.width = this.W; this.world.height = this.H;
      this.bg.width = this.W; this.bg.height = this.H;
      P.paintSky(this.bg.getContext('2d'), this.W, this.H);
    }

    /** Baris HUD variabel: "nama=nilai" dijejer, pindah baris bila tidak muat; nilai panjang dipotong. */
    hudText(s) {
      const max = Math.floor((this.W - 8) / 4), lines = [];
      let cur = '';
      for (const n of this.hudVars) {
        if (!s.vars || s.vars[n] == null) continue;
        const item = P.fit(`${n}=${s.vars[n]}`, max);
        if (cur && cur.length + 2 + item.length > max) { lines.push(cur); cur = item; }
        else cur = cur ? `${cur}  ${item}` : item;
      }
      if (cur) lines.push(cur);
      return lines.slice(0, 4);
    }

    /** Mengubah langkah trace menjadi posisi dan aksi Gopher. */
    prepare(trace) {
      const focus = trace.block.id;
      this.layout(Math.max(1, ...trace.steps.map(s => this.hudText(s).length)));
      const views = [];
      let run = null, st = null;
      for (const s of trace.steps) {
        let warp = false;
        if (s.run !== run) {
          warp = run !== null || false;
          run = s.run;
          st = { tested: {}, chosen: -1, x: X_START, lane: 0 };
        }
        const v = { step: s, from: { x: st.x, lane: st.lane }, warp, act: 'hit', lane: st.lane, x: st.x, tile: -1 };
        const own = s.block === focus;
        if (s.reason === 'panic') {
          v.act = 'panic';
        } else if (own && s.phase === 'init') {
          v.act = 'tag'; v.lane = 0; v.x = X_START;
        } else if (own && s.result != null && (s.phase === 'body' || s.phase === 'cond')) {
          let k = this.lanes.findIndex(l => l.kind !== 'skip' && l.line === s.line);
          if (k < 0) k = st.lane;
          v.act = 'check'; v.lane = k; v.x = X_SIGN - 4;
          st.tested = Object.assign({}, st.tested, { [k]: s.result });
        } else if (own && s.phase === 'branch') {
          const k = Math.min(s.branch, this.lanes.length - 1);
          v.act = 'enter'; v.lane = k; v.x = X_DOOR + 8;
          st.chosen = k;
        } else if (own && s.phase === 'exit') {
          v.act = 'exit'; v.x = this.exitX; v.lane = st.chosen >= 0 ? st.chosen : st.lane;
        } else {
          let k = st.chosen;
          if (k < 0) k = this.lanes.findIndex(l => P.nodeAt(l.body, s.line) >= 0);
          if (k < 0) k = st.lane;
          const ti = P.nodeAt(this.lanes[k].body, s.line);
          v.lane = k; v.tile = ti;
          v.x = ti >= 0 ? this.lanes[k].tiles[ti].cx : st.x;
        }
        v.tested = st.tested;
        v.chosen = st.chosen;
        st.x = v.x; st.lane = v.lane;
        views.push(v);
      }
      this.snaps = views;
      return views;
    }

    duration(i) {
      const v = this.snaps[i], last = i === this.snaps.length - 1;
      return { tag: 800, check: 950, enter: 700, hit: 750, exit: last ? 1900 : 1100, panic: 2000 }[v.act] + (v.warp ? 400 : 0);
    }

    cues(i) {
      const v = this.snaps[i], s = v.step, out = [];
      const last = i === this.snaps.length - 1;
      switch (v.act) {
        case 'tag': out.push({ t: 0, sound: 'blip' }); break;
        case 'check':
          out.push({ t: 0.1, sound: 'think' });
          out.push({ t: 0.55, sound: s.result ? 'yes' : 'no' });
          break;
        case 'enter': out.push({ t: 0, sound: 'gate' }); break;
        case 'hit': out.push({ t: 0, sound: 'jump' }); out.push({ t: 0.5, sound: 'blip' }); break;
        case 'exit':
          out.push({ t: 0.2, sound: { 'return': 'door', 'break': 'pipe', 'continue': 'boing' }[s.reason] || 'tickLow' });
          if (last) out.push({ t: 0.5, sound: 'jingle', fx: () => this.confetti() });
          break;
        case 'panic': out.push({ t: 0, sound: 'panic' }); break;
      }
      if (s.output) out.push({ t: 0.6, sound: 'coin' });
      return out;
    }

    confetti() {
      const cols = ['#f5a3bf', '#f7b779', '#f1d064', '#9fd88f', '#8ec4f0', '#b8a2ee', '#ffffff'];
      for (let k = 0; k < 50; k++) {
        this.parts.push({ x: this.W / 2 + (Math.random() - 0.5) * this.W, y: -2 - Math.random() * 20, vx: (Math.random() - 0.5) * 20, vy: 18 + Math.random() * 25, g: 8, life: 3000, col: cols[k % cols.length] });
      }
    }

    update(dt) {
      this.clock += dt;
      const f = dt / 1000;
      this.parts = this.parts.filter(p => { p.vy += p.g * f; p.x += p.vx * f; p.y += p.vy * f; p.life -= dt; return p.life > 0; });
    }

    /** Tampilan tanpa trace: semua jalur netral, Gopher menunggu di awal. */
    renderIdle() {
      const v = { act: 'idle', lane: 0, x: X_START, tested: {}, chosen: -1, step: { vars: {} } };
      this.draw(v, { x: X_START, y: this.groundY(0) }, 1, false, 1, 'think');
    }

    render(i, t) {
      const v = this.snaps[i];
      if (!v) return;
      const to = { x: v.x, y: this.groundY(v.lane) };
      const from = { x: v.from.x, y: this.groundY(v.from.lane) };
      let pos = to, a = 1, walking = false, alpha = 1;
      if (v.warp) {
        if (t < 0.3) { pos = from; alpha = 1 - t / 0.3; a = 0; }
        else if (t < 0.5) { alpha = (t - 0.3) / 0.2; a = 0; }
        else a = (t - 0.5) / 0.5;
      } else {
        const path = v.from.lane !== v.lane
          ? [from, { x: X_SHAFT, y: from.y }, { x: X_SHAFT, y: to.y }, to]
          : [from, to];
        let len = 0;
        for (let k = 1; k < path.length; k++) len += Math.hypot(path[k].x - path[k - 1].x, path[k].y - path[k - 1].y);
        const wf = len > 1 ? Math.min(0.5, len / 90) : 0;
        if (t < wf) { pos = P.along(path, t / wf); walking = true; a = 0; }
        else a = wf < 1 ? (t - wf) / (1 - wf) : 1;
      }
      let face = this.clock % 3400 < 130 ? 'blink' : 'idle';
      if (v.act === 'check') face = a < 0.55 ? 'think' : v.step.result ? 'happy' : 'shock';
      if (v.act === 'enter') face = 'happy';
      if (v.act === 'panic') face = 'shock';
      if (v.act === 'exit' && i === this.snaps.length - 1 && a > 0.5) face = 'happy';
      this.draw(v, pos, a, walking, alpha, face);
    }

    /** Teks kode untuk langkah ini: perintah di ubin, kondisi yang dicek, atau kepala blok. */
    codeOf(v) {
      const ln = this.lanes[v.lane];
      if (v.act === 'hit' && ln && v.tile >= 0) return ln.body[v.tile].text || ln.tiles[v.tile].label;
      if (v.act === 'check' && ln) return `${LABEL[ln.kind].toLowerCase()} ${ln.label || ''}`;
      if (v.act === 'tag' || v.act === 'idle') return this.block.header || '';
      if (v.act === 'exit' && v.step.reason) return v.step.reason;
      return '';
    }

    draw(v, pos, a, walking, alpha, face) {
      const w = this.wx, now = this.clock;
      P.drawBg(w, this.bg);

      // tangga di kiri
      const y0 = this.groundY(0) - 20, y1 = this.groundY(this.lanes.length - 1);
      P.rect(w, X_SHAFT - 4, y0, 1, y1 - y0, '#c58ba3');
      P.rect(w, X_SHAFT + 4, y0, 1, y1 - y0, '#c58ba3');
      for (let y = y0 + 3; y < y1; y += 5) P.rect(w, X_SHAFT - 3, y, 7, 1, '#e6b3c6');
      // pipa keluar di kanan, menyatukan semua jalur
      P.rect(w, this.exitX + 6, y0 + 6, 6, y1 - y0 - 4, '#2f6b3d');
      P.rect(w, this.exitX + 7, y0 + 6, 4, y1 - y0 - 4, '#7cc68b');
      P.rect(w, this.exitX + 8, y0 + 6, 1, y1 - y0 - 4, '#b5e6bf');

      this.lanes.forEach((ln, k) => {
        const gy = this.groundY(k);
        const dim = v.chosen >= 0 && v.chosen !== k;
        w.globalAlpha = dim ? 0.35 : 1;
        P.rect(w, X_SHAFT - 6, gy, this.exitX - X_SHAFT + 12, 2, '#e8a9bf');
        P.rect(w, X_SHAFT - 6, gy + 2, this.exitX - X_SHAFT + 12, 2, '#f7d3dd');
        P.text(w, '>', this.exitX + 1, gy - 6, '#c58ba3');

        // papan kondisi: putih = belum dicek, kedip = sedang dicek, hijau ✓ / merah ✗ = hasilnya
        const tested = v.tested[k];
        const testing = v.act === 'check' && v.lane === k && a < 0.55;
        const label = LABEL[ln.kind] + (testing ? ' ?' : tested === true ? ' ✓' : tested === false ? ' ✗' : '');
        let fill = '#ffffff', col = P.INK;
        if (testing) col = Math.floor(now / 180) % 2 ? P.INK : '#c2457a';
        else if (tested === true || v.chosen === k) { fill = '#dcf3e6'; col = '#2f8a5c'; }
        else if (tested === false) { fill = '#fde2e7'; col = '#c2457a'; }
        const bw = P.textW(label) + 6, by = gy - 28;
        P.rect(w, X_SIGN + 2, by + 9, 1, gy - by - 9, P.INK);
        P.box(w, X_SIGN, by, bw, 9, fill, P.INK);
        P.text(w, label, X_SIGN + 3, by + 2, col);

        // pintu jalur: terbuka bila cabang ini dipilih
        const opening = v.chosen === k ? P.clamp(v.act === 'enter' && v.lane === k ? a * 2 : 1, 0, 1) : 0;
        const doorH = Math.round(16 * (1 - opening));
        P.rect(w, X_DOOR - 2, gy - 18, 1, 18, P.INK);
        P.rect(w, X_DOOR + 4, gy - 18, 1, 18, P.INK);
        P.rect(w, X_DOOR - 2, gy - 19, 7, 1, P.INK);
        if (doorH > 0) P.rect(w, X_DOOR - 1, gy - 18, 5, doorH, tested === false ? '#f39a8a' : '#c9b3d8');

        // ubin isi cabang, berlabel nama perintahnya
        if (ln.empty) P.text(w, ln.empty, X_TILES, gy - 7, '#c58ba3');
        ln.body.forEach((n, j) => {
          const t = ln.tiles[j], hit = v.act === 'hit' && v.lane === k && v.tile === j;
          P.labelTile(w, t, gy, n.kind, hit && a > 0.45 ? (a - 0.45) / 0.4 : 0);
        });
        w.globalAlpha = 1;
      });

      // gopher
      const jump = (v.act === 'hit' && v.tile >= 0) ? Math.round(Math.sin(Math.PI * a) * 8) : 0;
      // saat mengecek kondisi tanda tanya ada di papan kondisi (balon di kepala akan menimpanya)
      const head = P.drawGopher(w, pos.x, pos.y - jump, face, walking ? Math.floor(now / 140) % 2 : 0, alpha);
      if (v.act === 'idle') P.bubble(w, pos.x, head - 14 + (Math.floor(now / 300) % 2 ? -1 : 0), '?', P.INK);

      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), 1, 1, p.col);

      // seluruh dunia terlihat (tanpa kamera); HUD di pojok kiri atas
      const ox = 0, oy = 0;
      const s = v.step;
      P.rect(w, 0, 0, this.W, this.top + 1, 'rgba(255,248,251,0.75)'); // latar HUD supaya awan tidak menimpa teks
      P.text(w, `${this.block.id} ${this.block.kind === 'switch' ? 'SWITCH' : 'IF'}${s.run > 1 ? ' #' + s.run : ''}`, ox + 4, oy + 3, P.INK);
      this.hudText(s).slice(0, this.hudLines).forEach((ln, j) => P.text(w, ln, ox + 4, oy + 10 + j * 7, '#c2457a'));
      // baris paling bawah: kode yang sedang dijalankan Gopher
      const code = this.codeOf(v);
      if (code) {
        const y = this.H - 8;
        P.rect(w, 0, y - 2, this.W, 9, 'rgba(255,248,251,0.85)');
        P.text(w, P.fit('> ' + code, Math.floor((this.W - 8) / 4)), 4, y, P.INK);
      }
      P.present(this.ctx, this.cv, this.world, this.W, this.H);
      if (v.act === 'panic' && a > 0.2) {
        const img = P.stamp('PANIC!', '#d9467c'), S = this.cv.__k || 4;
        this.ctx.drawImage(img, Math.round(this.cv.width / 2 - img.width * S), Math.round(24 * S), img.width * 2 * S, img.height * 2 * S);
      }
    }
  }

  G.BranchStage = BranchStage;
})(window.GV = window.GV || {});
