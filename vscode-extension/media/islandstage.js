// Mode pulau (Fase 10b, keputusan user 2026-10-09: "pulau + jembatan"). Program yang dipecah ke
// beberapa file: setiap file = pulau berpapan nama, fungsi-fungsinya = pondok di atas pulau.
// Memanggil fungsi dari file lain = Gopher menyeberang jembatan ke pulau itu dan menjalankan
// fungsinya di sana; return = koin berisi nilai kembali dibawa pulang ke pondok pemanggil.
(function (G) {
  'use strict';
  const P = G.pixel;

  const TOP = 18;      // HUD
  const GROUND = 84;   // permukaan pulau
  const BRIDGE = 28;   // panjang jembatan antar pulau
  const HUT_GAP = 6;
  const SEA = '#8ec4f0';

  /** Nama fungsi pendek untuk papan pondok: "Akun.setor" tetap, "utils.diskon" → "diskon". */
  function hutName(fn) {
    return fn || '?';
  }

  class IslandStage {
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
      const focus = (trace.files && trace.files[0]) || (trace.file || '').split(/[\\/]/).pop();
      const fileOf = f => f || focus;
      // urutan pulau: file tempat program dimulai di kiri, lalu sisanya
      const order = [];
      const addFile = f => { if (f && !order.includes(f)) order.push(f); };
      addFile(trace.entry);
      (trace.files || [focus]).forEach(addFile);
      const steps = trace.steps;
      for (const s of steps) addFile(fileOf(s.file));

      // pondok (fungsi) per pulau, dari langkah call; file fungsi = file langkah berikutnya di lantainya
      const huts = {};
      order.forEach(f => { huts[f] = []; });
      const addHut = (file, fn) => { if (!huts[file].includes(fn)) huts[file].push(fn); };
      const calleeFile = i => {
        const d = steps[i].depth || 0;
        for (let j = i + 1; j < steps.length; j++) {
          const dj = steps[j].depth || 0;
          if (dj < d) break;
          if (dj === d) return fileOf(steps[j].file);
        }
        return fileOf(steps[i].file);
      };
      const views = [];
      const stack = []; // [{ fn, file, args }]
      const blockFn = trace.block && trace.block.name ? trace.block.name : 'program';
      const first = steps[0];
      if (!first || first.phase !== 'call') {
        stack[0] = { fn: blockFn, file: fileOf(first && first.file), args: '' };
        addHut(stack[0].file, blockFn);
      }
      let at = null;
      steps.forEach((s, i) => {
        const d = s.depth || 0;
        const v = { step: s, i, act: 'hit' };
        if (s.reason === 'panic') v.act = 'panic';
        if (s.phase === 'call') {
          const file = calleeFile(i);
          stack.length = d;
          v.caller = stack[d - 1] || { fn: null, file: fileOf(s.file) };
          stack[d] = { fn: hutName(s.func), file, args: s.args || '' };
          addHut(file, stack[d].fn);
          v.act = 'call';
        } else if (s.phase === 'return') {
          v.act = 'return';
          v.ret = s.return;
          v.caller = stack[d - 1] || null;
        } else if (s.phase === 'exit' && i === steps.length - 1) {
          v.act = v.act === 'panic' ? 'panic' : 'exit';
        }
        v.cur = Object.assign({}, stack[Math.min(d, stack.length - 1)] || stack[stack.length - 1] || { fn: blockFn, file: fileOf(s.file) });
        v.stack = stack.slice(0, d + 1).filter(Boolean).map(f => Object.assign({}, f));
        v.from = at;
        at = { fn: v.cur.fn, file: v.cur.file };
        if (v.act === 'return') {
          stack.length = d; // lantai ini selesai: Gopher kembali ke pemanggil
          if (v.caller) at = { fn: v.caller.fn, file: v.caller.file };
        }
        views.push(v);
      });
      this.order = order;
      this.huts = huts;
      this.focusName = focus;
      this.blockId = trace.block ? trace.block.id : '';
      this.blockName = blockFn;
      this.layout();
      this.snaps = views;
      return views;
    }

    layout() {
      let x = 10;
      this.isles = {};
      for (const f of this.order) {
        const names = this.huts[f].length ? this.huts[f] : [];
        const hw = names.map(n => Math.max(26, P.textW(P.fit(n + '()', 14)) + 8));
        const inner = hw.reduce((a, b) => a + b, 0) + Math.max(0, hw.length - 1) * HUT_GAP;
        const w = Math.max(76, inner + 34, P.textW(P.fit(f, 18)) + 16);
        const isle = { x, w, file: f, huts: {} };
        let hx = x + Math.round((w - inner) / 2);
        names.forEach((n, k) => {
          isle.huts[n] = { x: hx, w: hw[k], cx: hx + hw[k] / 2 };
          hx += hw[k] + HUT_GAP;
        });
        isle.dock = x + 14; // tempat Gopher menunggu bila bukan di pondok
        this.isles[f] = isle;
        x += w + BRIDGE;
      }
      this.W = Math.max(192, x - BRIDGE + 10);
      this.H = 128;
      this.world.width = this.W; this.world.height = this.H;
      this.bg.width = this.W; this.bg.height = this.H;
      this.paintBg();
    }

    paintBg() {
      const c = this.bg.getContext('2d');
      P.paintSky(c, this.W, this.H);
      // laut berombak
      P.rect(c, 0, GROUND + 6, this.W, this.H - GROUND - 6, SEA);
      for (let y = GROUND + 10; y < this.H; y += 7) for (let x = (y * 3) % 11; x < this.W; x += 11) P.rect(c, x, y, 4, 1, '#b5dcf6');
      const isles = this.order.map(f => this.isles[f]);
      isles.forEach((s, k) => {
        // jembatan papan ke pulau berikutnya
        const nx = isles[k + 1];
        if (nx) {
          const x0 = s.x + s.w - 6, x1 = nx.x + 6;
          P.rect(c, x0, GROUND - 1, x1 - x0, 3, '#a8741f');
          for (let x = x0; x < x1; x += 4) P.rect(c, x, GROUND - 1, 1, 3, '#7a5418');
          P.rect(c, x0, GROUND - 7, x1 - x0, 1, '#7a5418');
          for (let x = x0 + 2; x < x1; x += 8) P.rect(c, x, GROUND - 7, 1, 7, '#7a5418');
        }
        // pulau: pasir + rumput
        P.ell(c, Math.round(s.x + s.w / 2), GROUND + 6, Math.round(s.w / 2), 7, '#f3dca0');
        P.rect(c, s.x + 2, GROUND, s.w - 4, 6, '#f3dca0');
        P.rect(c, s.x + 2, GROUND - 1, s.w - 4, 2, '#9fd88f');
        P.rect(c, s.x + 2, GROUND + 1, s.w - 4, 1, '#7cc68b');
        // pohon kelapa kecil di ujung kiri
        P.rect(c, s.x + 6, GROUND - 14, 2, 13, '#a8741f');
        P.ell(c, s.x + 7, GROUND - 15, 5, 2, '#7cc68b');
      });
    }

    duration(i) {
      const v = this.snaps[i], last = i === this.snaps.length - 1;
      if (v.act === 'panic') return 2000;
      if (v.act === 'exit' || last) return 2000;
      if (v.act === 'call') return this.crosses(v) ? 1400 : 950;
      if (v.act === 'return') return this.crosses(v) ? 1300 : 1000;
      return 650;
    }

    /** Langkah ini menyeberang ke pulau lain? */
    crosses(v) {
      if (v.act === 'call') return v.caller && v.caller.file !== v.cur.file;
      if (v.act === 'return') return v.caller && v.caller.file !== v.cur.file;
      return false;
    }

    cues(i) {
      const v = this.snaps[i], out = [];
      const last = i === this.snaps.length - 1;
      if (v.act === 'call') out.push({ t: 0.05, sound: this.crosses(v) ? 'gate' : 'jump' });
      if (v.act === 'return') out.push({ t: 0.15, sound: 'coin' });
      if (v.act === 'hit') out.push({ t: 0.1, sound: 'blip' });
      if (v.act === 'panic') out.push({ t: 0, sound: 'panic' });
      if (v.step.output) out.push({ t: 0.6, sound: 'coin' });
      if (last && v.act !== 'panic') out.push({ t: 0.3, sound: 'jingle', fx: () => this.confetti() });
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

    /** Titik x Gopher untuk fungsi fn di pulau file (dermaga bila bukan pondok). */
    spot(pos) {
      const isle = pos && this.isles[pos.file];
      if (!isle) return this.isles[this.order[0]].dock;
      const h = pos.fn && isle.huts[pos.fn];
      return h ? h.cx : isle.dock;
    }

    render(i, t) {
      const v = this.snaps[i];
      if (!v) return;
      const w = this.wx, now = this.clock, s = v.step;
      w.drawImage(this.bg, 0, 0);
      const curFile = v.cur.file;

      // papan nama pulau + pondok fungsi
      for (const f of this.order) {
        const isle = this.isles[f];
        const active = f === curFile;
        w.globalAlpha = active ? 1 : 0.6;
        const label = P.fit(f, 18), lw = P.textW(label) + 8;
        const lx = Math.round(isle.x + isle.w / 2 - lw / 2);
        P.box(w, lx, TOP + 4, lw, 11, active ? '#fff3a8' : '#ffffff', P.INK);
        P.text(w, label, lx + 4, TOP + 7, P.INK);
        P.rect(w, Math.round(isle.x + isle.w / 2), TOP + 15, 1, 6, P.INK);
        for (const fn in isle.huts) {
          const h = isle.huts[fn];
          const on = active && fn === v.cur.fn;
          const roof = on ? '#f39a8a' : '#c9b3d8';
          const by = GROUND - 18;
          for (let r = 0; r < 6; r++) { // atap
            const half = Math.round(((r + 1) / 6) * (h.w / 2 + 2));
            P.rect(w, Math.round(h.cx - half), by - 6 + r, half * 2, 1, roof);
          }
          P.box(w, h.x, by, h.w, 17, on ? '#fff8fb' : '#f6eef2', on ? '#c2457a' : P.INK);
          P.rect(w, Math.round(h.cx) - 4, by + 6, 8, 11, on ? '#6b5577' : '#d7a2b8'); // pintu
          // papan nama fungsi di atas atap (Gopher di depan pintu tidak menutupinya)
          const name = P.fit(fn + '()', 14), nw = P.textW(name) + 6;
          P.box(w, Math.round(h.cx - nw / 2), by - 17, nw, 9, on ? '#fff3a8' : '#ffffff', on ? '#c2457a' : P.INK);
          P.textCenter(w, name, h.cx, by - 15, on ? '#c2457a' : P.INK);
        }
        w.globalAlpha = 1;
      }

      // Gopher: berjalan dari tempat sebelumnya (menyeberang jembatan bila pulau berbeda)
      const fromX = v.from ? this.spot(v.from) : this.spot(v.act === 'call' && v.caller ? v.caller : v.cur);
      const toX = this.spot(v.cur);
      let walk = 0, gx = toX;
      const dist = Math.abs(toX - fromX);
      if (v.act === 'return') {
        // berjalan ke pondok (koin di tangan), lalu pulang ke pemanggil
        const back = v.caller ? this.spot(v.caller) : toX;
        const u = P.clamp((t - 0.25) / 0.55, 0, 1);
        gx = P.lerp(toX, back, P.ease(u));
        walk = u > 0 && u < 1 && Math.abs(back - toX) > 2 ? 1 : 0;
      } else if (dist > 2) {
        const wf = Math.min(0.7, dist / 160 + 0.2);
        const u = P.clamp(t / wf, 0, 1);
        gx = P.lerp(fromX, toX, P.ease(u));
        walk = u < 1 ? 1 : 0;
      }
      let face = now % 3400 < 130 ? 'blink' : 'idle';
      if (v.act === 'call') face = 'happy';
      if (v.act === 'return') face = 'happy';
      if (v.act === 'panic') face = 'shock';
      const hop = v.act === 'hit' && t < 0.4 ? Math.round(Math.sin(Math.PI * t / 0.4) * 4) : 0;
      const head = P.drawGopher(w, gx, GROUND - hop, face, walk ? Math.floor(now / 120) % 2 : 0);

      // balon panggilan / koin return
      if (v.act === 'call' && t > 0.05) {
        const txt = P.fit(`${v.cur.fn}(${v.cur.args})`, 24), bw = P.textW(txt) + 6;
        const bx = Math.round(P.clamp(gx - bw / 2, 2, this.W - bw - 2)), by = Math.max(TOP + 18, Math.min(head - 13, GROUND - 48)); // di atas papan nama pondok
        P.box(w, bx, by, bw, 9, '#ffffff', P.INK);
        P.text(w, txt, bx + 3, by + 2, '#5a43a0');
      }
      if (v.act === 'return' && v.ret != null && v.ret !== '') {
        const r = P.fit(v.ret, 10), cy = Math.max(TOP + 18, Math.min(head - 10, GROUND - 48));
        P.disc(w, Math.round(gx) - 6, cy + 4, 3, '#f3c75a');
        P.box(w, Math.round(gx) - 2, cy, P.textW(r) + 4, 9, '#fff8fb', '#e0a22a');
        P.text(w, r, Math.round(gx), cy + 2, P.INK);
      }

      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), 1, 1, p.col);

      // HUD: fungsi fokus, tumpukan panggilan (file: fungsi), kode yang sedang dijalankan
      P.rect(w, 0, 0, this.W, 17, 'rgba(255,248,251,0.8)');
      P.text(w, P.fit(`${this.blockId || ''} ${this.blockName || ''}  ${this.order.length} FILE`, Math.floor((this.W - 8) / 4)), 4, 3, P.INK);
      const chain = v.stack.map(f => `${f.fn}`).join(' > ');
      P.text(w, P.fit(`CALL STACK ${v.stack.length}: ${chain}`, Math.floor((this.W - 8) / 4)), 4, 10, '#c2457a');
      const code = (s.note || '').replace(/ → .*$/, '');
      if (code) {
        P.rect(w, 0, this.H - 10, this.W, 10, 'rgba(255,248,251,0.85)');
        P.text(w, P.fit(`> ${fileTag(s, this)}${code}`, Math.floor((this.W - 8) / 4)), 4, this.H - 8, P.INK);
      }
      if (v.act === 'exit' || (v.i === this.snaps.length - 1 && v.act !== 'panic')) {
        const img = P.stamp('SELESAI!', '#2f8a5c');
        w.drawImage(img, Math.round(this.W - img.width * 2 - 4), 2, img.width * 2, img.height * 2);
      }
      P.present(this.ctx, this.cv, this.world, this.W, this.H);
      if (v.act === 'panic' && t > 0.2) {
        const img = P.stamp('PANIC!', '#d9467c'), K = this.cv.__k || 4;
        this.ctx.drawImage(img, Math.round(this.cv.width / 2 - img.width * K), Math.round(24 * K), img.width * 2 * K, img.height * 2 * K);
      }
    }
  }

  /** "utils.py:5 " di depan kode: file dan baris langkah ini. */
  function fileTag(s, stage) {
    return s.line ? `${s.file || stage.focusName}:${s.line} ` : '';
  }

  G.IslandStage = IslandStage;
})(window.GV = window.GV || {});
