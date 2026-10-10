// Event loop JavaScript (Fase 12d): tiga jalur tiket. Kiri = CALL STACK (fungsi yang sedang
// berjalan, menumpuk ke atas); kanan atas = antrian MICROTASK (Promise.then, lanjutan await);
// kanan bawah = antrian TIMER (setTimeout). Tiket baru meluncur masuk dari kanan; saat call stack
// kosong, Gopher (si event loop) mengambil tiket terdepan — microtask dulu, baru timer — dan
// membawanya ke call stack. Digerakkan oleh Step.evloop dari simulator.
(function (G) {
  'use strict';
  const P = G.pixel;

  const W = 300, H = 136;
  const SX = 10, SW = 74, SY0 = 122;     // call stack: kiri, dasar di SY0
  const LX = 110, TW = 58, TH = 13;      // jalur tiket: mulai x, lebar & tinggi tiket
  const MY = 38, TY = 88;                // y jalur microtask & timer
  const COL = { micro: '#f1d064', macro: '#8ec4f0', frame: '#b8a2ee' };

  /** Teks tiket: "then: () => console.log("promise")" → 'log("promise")', timer diberi jedanya. */
  function shortLabel(label) {
    const ms = /^setTimeout (\d+(?:\.\d+)?)ms: /i.exec(label);
    let s = label.replace(/^(then|queueMicrotask): /i, '').replace(/^setTimeout [\d.]+ms: /i, '');
    s = s.replace(/^(async\s*)?\(?[\w\s,$]*\)?\s*=>\s*\{?\s*/, '').replace(/console\./, '').replace(/ setelah await$/, '');
    return (ms ? ms[1] + 'ms ' : '') + s;
  }

  const pickOf = s => {
    const m = /event loop mengambil tiket (microtask|timer)/.exec(s.note || '');
    return m ? (m[1] === 'timer' ? 'macro' : 'micro') : null;
  };

  class EvStage {
    constructor(canvas) {
      this.cv = canvas;
      this.ctx = canvas.getContext('2d');
      this.world = document.createElement('canvas');
      this.world.width = W; this.world.height = H;
      this.wx = this.world.getContext('2d');
      this.bg = document.createElement('canvas');
      this.bg.width = W; this.bg.height = H;
      this.clock = 0;
      this.parts = [];
      this.snaps = [];
      this.paintBg();
    }

    paintBg() {
      const c = this.bg.getContext('2d');
      P.paintSky(c, W, H);
      // call stack: bingkai menara
      P.box(c, SX - 3, 20, SW + 6, SY0 - 18, '#fff4f7', '#d7a2c9');
      P.textCenter(c, 'CALL STACK', SX + SW / 2, 23, '#5a43a0');
      // dua jalur tiket
      for (const [y, label, col] of [[MY, 'MICROTASK  THEN / AWAIT', '#c27a12'], [TY, 'TIMER  SETTIMEOUT', '#2f6fa8']]) {
        P.text(c, label, LX, y - 12, col);
        P.rect(c, LX - 4, y + TH + 2, W - LX - 2, 3, '#8a7a92');
        for (let x = LX - 2; x < W - 6; x += 6) P.rect(c, x, y + TH + 3, 2, 1, '#cfc2d6');
      }
      P.text(c, 'DEPAN', LX - 4, TY + TH + 8, '#b090c8');
    }

    prepare(trace) {
      let prev = { stack: [], micro: [], macro: [] };
      this.snaps = trace.steps.map(s => {
        const ev = s.evloop || prev;
        const v = { step: s, ev, prev, pick: pickOf(s) };
        prev = ev;
        return v;
      });
      return this.snaps;
    }

    duration(i) {
      const v = this.snaps[i];
      if (v.pick) return 1300;
      const grew = v.ev.micro.length > v.prev.micro.length || v.ev.macro.length > v.prev.macro.length;
      if (grew) return 900;
      if (v.ev.stack.length !== v.prev.stack.length) return 650;
      return i === this.snaps.length - 1 ? 1800 : 520;
    }

    cues(i) {
      const v = this.snaps[i], out = [];
      if (v.pick) out.push({ t: 0.1, sound: 'jump' }, { t: 0.75, sound: 'land' });
      else if (v.ev.micro.length > v.prev.micro.length || v.ev.macro.length > v.prev.macro.length) out.push({ t: 0.5, sound: 'appear' });
      if (v.ev.stack.length > v.prev.stack.length) out.push({ t: 0.2, sound: 'pipe' });
      if (v.step.output) out.push({ t: 0.6, sound: 'coin' });
      if (i === this.snaps.length - 1) out.push({ t: 0.4, sound: 'jingle' });
      return out;
    }

    update(dt) {
      this.clock += dt;
      const f = dt / 1000;
      this.parts = this.parts.filter(p => { p.vy += 60 * f; p.x += p.vx * f; p.y += p.vy * f; p.life -= dt; return p.life > 0; });
    }

    ticket(w, x, y, label, col, ring) {
      x = Math.round(x); y = Math.round(y);
      if (ring) P.rect(w, x - 1, y - 1, TW + 2, TH + 2, ring);
      P.rect(w, x, y, TW, TH, col);
      P.rect(w, x, y, TW, 1, P.mix(col, '#ffffff', 0.5));
      P.rect(w, x, y + TH - 1, TW, 1, P.mix(col, P.INK, 0.4));
      for (let k = 2; k < TH - 2; k += 2) P.rect(w, x + 3, y + k, 1, 1, P.mix(col, P.INK, 0.35)); // sobekan tiket
      P.text(w, P.fit(shortLabel(label), 12), x + 6, y + 4, P.INK);
    }

    frame(w, k, name, alpha, dy) {
      const fh = 13, y = SY0 - (k + 1) * (fh + 2) + (dy || 0);
      w.globalAlpha = alpha == null ? 1 : alpha;
      P.box(w, SX, Math.round(y), SW, fh, k === 0 ? '#d8ccf5' : COL.frame, '#5a43a0');
      P.text(w, P.fit(name, 17), SX + 4, Math.round(y) + 4, P.INK);
      w.globalAlpha = 1;
      return y;
    }

    render(i, t) {
      const v = this.snaps[i];
      if (!v) return;
      const w = this.wx, now = this.clock, ev = v.ev, pv = v.prev;
      w.drawImage(this.bg, 0, 0);

      // jalur tiket: tiket yang baru masuk meluncur dari kanan; tiket yang diambil terbang ke call stack
      const lane = (key, y, col) => {
        const cur = ev[key], before = pv[key];
        const picked = v.pick === key;
        const list = picked ? before : cur;
        const shift = picked ? P.ease(P.clamp((t - 0.45) / 0.4, 0, 1)) : 0;
        list.forEach((label, k) => {
          if (picked && k === 0) return; // digambar sebagai tiket terbang
          let x = LX + k * (TW + 4) - shift * (TW + 4);
          const fresh = !picked && k >= before.length;
          if (fresh) x = P.lerp(W + 4, x, P.ease(P.clamp(t / 0.6, 0, 1)));
          if (x < W) this.ticket(w, x, y, label, col, fresh ? '#d9467c' : null);
        });
        if (list.length > 4) P.text(w, `+${list.length - 4}`, W - 14, y + 4, P.INK);
        if (picked && before.length) { // tiket terdepan: diangkat Gopher, dibawa ke call stack
          const e = P.ease(P.clamp(t / 0.7, 0, 1));
          const x = P.lerp(LX, SX + 18, e), yy = P.lerp(y, SY0 - 16, e) - Math.sin(Math.PI * e) * 24;
          this.ticket(w, x, yy, before[0], col, '#2f8a5c');
        }
      };
      lane('micro', MY, COL.micro);
      lane('macro', TY, COL.macro);

      // call stack: frame baru jatuh masuk, frame yang selesai memudar
      const st = ev.stack, ps = pv.stack;
      const n = Math.max(st.length, ps.length);
      for (let k = 0; k < n; k++) {
        if (k < st.length) {
          const isNew = k >= ps.length || ps[k] !== st[k];
          const dy = isNew && !v.pick ? -Math.round((1 - P.bounce(P.clamp(t / 0.5, 0, 1))) * 20) : 0;
          this.frame(w, k, st[k], v.pick && isNew ? P.clamp((t - 0.7) / 0.3, 0, 1) : 1, dy);
        } else if (t < 0.5) {
          this.frame(w, k, ps[k], 1 - t * 2, -t * 20); // keluar dari stack
        }
      }
      if (!st.length) P.textCenter(w, 'KOSONG', SX + SW / 2, SY0 - 12, '#b090c8');

      // Gopher = event loop: berdiri di antara call stack dan jalur tiket, ikut mengangkat tiket
      let gx = 96, gy = SY0;
      let face = (now % 3400) < 130 ? 'blink' : 'idle';
      if (v.pick) {
        const e = P.ease(P.clamp(t / 0.7, 0, 1));
        gx = P.lerp(LX + 8, SX + SW + 10, e);
        gy = SY0 - Math.round(Math.sin(Math.PI * e) * 10);
        face = 'happy';
      } else if (!st.length && (ev.micro.length || ev.macro.length)) face = 'think';
      const head = P.drawGopher(w, gx, gy, face, v.pick && t < 0.7 ? Math.floor(now / 120) % 2 : 0);
      if (!st.length && !v.pick && (ev.micro.length || ev.macro.length)) P.bubble(w, gx, head - 14, '?', '#c2457a');

      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), 1, 1, p.col);
      // HUD
      P.rect(w, 0, 0, W, 16, 'rgba(255,248,251,0.75)');
      P.text(w, 'EVENT LOOP', 4, 3, P.INK);
      const now2 = ev.task ? `TIKET: ${P.fit(ev.task, 34)}` : st.length ? `JALAN: ${P.fit(st[st.length - 1], 30)}` : 'MENUNGGU TIKET';
      P.text(w, now2, 4, 10, '#c2457a');
      P.present(this.ctx, this.cv, this.world, W, H);
    }
  }

  G.EvStage = EvStage;
})(window.GV = window.GV || {});
