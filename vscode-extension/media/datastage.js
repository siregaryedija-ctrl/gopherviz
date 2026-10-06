// Mode data: elemen slice/array digambar sebagai barisan gopher (tinggi = nilai), seperti
// prototype/bubble-sort.html. Digerakkan langkah demi langkah oleh Trace dari simulator.
(function (G) {
  'use strict';
  const P = G.pixel;

  const VIEW = 192, H = 96, GY = 76;
  const SLOT_MIN = 18, SLOT_MAX = 32;

  class DataStage {
    constructor(canvas, hooks) {
      this.cv = canvas;
      this.ctx = canvas.getContext('2d');
      this.world = document.createElement('canvas');
      this.world.height = H;
      this.wx = this.world.getContext('2d');
      this.bg = document.createElement('canvas');
      this.bg.height = H;
      this.hooks = hooks;
      this.parts = [];
      this.clock = 0;
      this.cam = { x: VIEW / 2, y: H / 2, z: 1 };
      this.worldW = 0;
      this.snaps = [];
    }

    /** Menyiapkan snapshot per langkah. Mengembalikan daftar langkah untuk pemutar. */
    prepare(trace, loop, loops) {
      let ids = trace.collection.items.map((_, k) => k);
      let next = ids.length;
      const vals = {};
      const seen = new Set();
      trace.collection.items.forEach((v, k) => { vals[k] = v; seen.add(v); });

      // Variabel loop (fokus dan loop di dalamnya) menjadi penunjuk indeks: I, J, ...
      const ptrNames = [];
      const addVars = l => {
        (l.vars || []).forEach(v => { if (!ptrNames.includes(v)) ptrNames.push(v); });
        (l.children || []).forEach(c => { const cl = loops.find(x => x.id === c); if (cl) addVars(cl); });
      };
      addVars(loop);

      let cmp = 0, swp = 0, maxN = ids.length;
      this.snaps = trace.steps.map(s => {
        const snap = { step: s, prevOrder: null, cmp: null, swap: null, read: -1, set: -1, appended: -1 };
        for (const o of s.ops || []) {
          if (o.op === 'compare') { snap.cmp = o; cmp++; }
          else if (o.op === 'swap' && o.b != null && o.b < ids.length) {
            snap.prevOrder = ids.slice();
            [ids[o.a], ids[o.b]] = [ids[o.b], ids[o.a]];
            snap.swap = o; swp++;
          } else if (o.op === 'set' && o.a < ids.length) { vals[ids[o.a]] = o.value; snap.set = o.a; seen.add(o.value); }
          else if (o.op === 'append') { ids.push(next); vals[next++] = o.value; snap.appended = ids.length - 1; seen.add(o.value); }
          else if (o.op === 'read') snap.read = o.a;
        }
        if (s.items) { // isi koleksi dari simulator adalah sumber kebenaran
          while (ids.length < s.items.length) ids.push(next++);
          if (ids.length > s.items.length) ids = ids.slice(0, s.items.length);
          s.items.forEach((v, p) => { vals[ids[p]] = v; seen.add(v); });
        }
        maxN = Math.max(maxN, ids.length);
        snap.order = ids.slice();
        snap.vals = Object.assign({}, vals);
        snap.cmpCount = cmp;
        snap.swpCount = swp;
        snap.ptr = [];
        for (const name of ptrNames) {
          const raw = s.vars && s.vars[name];
          if (raw && /^-?\d+$/.test(raw) && +raw >= 0 && +raw < ids.length) snap.ptr.push({ name, k: +raw });
        }
        return snap;
      });

      // Tinggi gopher = peringkat nilai (1..9), supaya angka besar maupun string tetap terbaca.
      const list = [...seen];
      const numeric = list.length > 0 && list.every(v => v !== '' && !isNaN(+v));
      const uniq = [...new Set(numeric ? list.map(Number) : list)].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
      this.levelOf = v => {
        if (uniq.length <= 1) return 3;
        const r = uniq.indexOf(numeric ? +v : v);
        return 1 + Math.round((Math.max(0, r) / (uniq.length - 1)) * 8);
      };

      this.n = maxN;
      this.slot = P.clamp(Math.floor((VIEW - 16) / Math.max(1, maxN)), SLOT_MIN, SLOT_MAX);
      this.offset = Math.max(8, Math.round((VIEW - this.slot * maxN) / 2));
      this.resize(Math.max(VIEW, this.offset * 2 + this.slot * maxN));
      this.lastIdx = -1;
      return this.snaps;
    }

    resize(w) {
      if (w === this.worldW) return;
      this.worldW = w;
      this.world.width = w;
      this.bg.width = w;
      P.paintWorld(this.bg.getContext('2d'), w, H, GY);
    }

    slotX(k) { return this.offset + (k + 0.5) * this.slot; }

    duration(i) {
      const s = this.snaps[i];
      if (s.swap) return 1150;
      if (s.cmp) return 1250;
      if (s.set >= 0 || s.appended >= 0) return 800;
      if (s.step.phase === 'exit') return i === this.snaps.length - 1 ? 2200 : 900;
      if (s.step.phase === 'break') return 900;
      if (s.read >= 0) return 650;
      return 380;
    }

    /** Suara dan efek pada titik waktu tertentu di dalam langkah i (t = 0..1). */
    cues(i) {
      const s = this.snaps[i], st = s.step, out = [];
      const last = i === this.snaps.length - 1;
      if (s.cmp) {
        out.push({ t: 0, sound: 'think' });
        const yes = st.result != null ? st.result : s.cmp.result === '>';
        out.push({ t: 0.45, sound: yes ? 'yes' : 'no' });
      }
      if (s.swap) {
        out.push({ t: 0, sound: 'jump' });
        out.push({ t: 0.9, sound: 'thud', fx: () => [s.swap.a, s.swap.b].forEach(k => this.burst(this.slotX(k), GY, 6, ['#f2c4d2', '#ffffff'], { sp: 60, up: 25, g: 90, life: 380 })) });
      }
      if (s.set >= 0) out.push({ t: 0.3, sound: 'sparkle', fx: () => this.burst(this.slotX(s.set), GY - 30, 10, ['#f3c75a', '#ffffff'], { sp: 60, up: 50, g: 60, life: 600 }) });
      if (s.appended >= 0) out.push({ t: 0.4, sound: 'land' });
      if (s.read >= 0 && !s.cmp) out.push({ t: 0, sound: 'blip' });
      if (st.output) out.push({ t: 0.5, sound: 'coin' });
      if (st.phase === 'break') out.push({ t: 0, sound: 'pipe' });
      if (st.reason === 'panic') out.push({ t: 0, sound: 'panic' });
      else if (last && st.phase === 'exit') {
        out.push({ t: 0, sound: 'jingle', fx: () => this.confetti() });
      }
      return out;
    }

    burst(x, y, n, cols, o) {
      for (let k = 0; k < n; k++) {
        this.parts.push({
          x: x + (Math.random() - 0.5) * (o.spread || 0), y,
          vx: (Math.random() - 0.5) * o.sp, vy: -Math.random() * o.up, g: o.g,
          life: o.life * (0.6 + Math.random() * 0.6), col: cols[k % cols.length], s: o.s || 1,
        });
      }
    }
    confetti() {
      const cols = ['#f5a3bf', '#f7b779', '#f1d064', '#9fd88f', '#8ec4f0', '#b8a2ee', '#ffffff'];
      for (let k = 0; k < 90; k++) {
        this.parts.push({ x: Math.random() * this.worldW, y: -Math.random() * 30, vx: (Math.random() - 0.5) * 20, vy: 18 + Math.random() * 25, g: 8, life: 3500, col: cols[k % cols.length], s: Math.random() < 0.3 ? 2 : 1 });
      }
    }

    update(dt) {
      this.clock += dt;
      const f = dt / 1000;
      this.parts = this.parts.filter(p => {
        p.vy += p.g * f; p.x += p.vx * f; p.y += p.vy * f; p.life -= dt;
        return p.life > 0 && p.y < H + 4;
      });
    }

    render(i, t) {
      const s = this.snaps[i];
      if (!s) return;
      const w = this.wx, now = this.clock, st = s.step;
      const last = i === this.snaps.length - 1;
      const done = last && st.phase === 'exit' && st.reason !== 'panic';
      w.drawImage(this.bg, 0, 0);

      for (let k = 0; k < s.order.length; k++) P.textCenter(w, String(k), this.slotX(k), GY + 4, '#d9a0b6');
      // penunjuk indeks (I, J, ...) di bawah nomor indeks
      const stack = {};
      for (const p of s.ptr) {
        const row = stack[p.k] = (stack[p.k] || 0) + 1;
        P.textCenter(w, p.name.slice(0, 3), this.slotX(p.k), GY + 4 + row * 6, '#c2457a');
      }

      // posisi tiap gopher
      const L = s.order.map((id, slot) => {
        let x = this.slotX(slot), y = 0, z = 0;
        let face = (now + id * 1370) % 3400 < 130 ? 'blink' : 'idle';
        if (s.prevOrder) {
          const ps = s.prevOrder.indexOf(id);
          if (ps !== slot && ps >= 0) {
            const e = P.ease(t);
            x = P.lerp(this.slotX(ps), x, e);
            y = -Math.sin(Math.PI * e) * (slot > ps ? 18 : 7);
            face = slot > ps ? 'happy' : 'shock';
            z = slot > ps ? 2 : 1;
          }
        } else if (s.cmp && (slot === s.cmp.a || slot === s.cmp.b)) {
          face = t < 0.45 ? 'think' : s.cmp.result === '>' ? (slot === s.cmp.a ? 'happy' : 'shock') : 'idle';
        }
        if (slot === s.appended) {
          const tk = P.clamp(t / 0.6, 0, 1);
          y = -Math.round(70 * (1 - P.bounce(tk)));
          if (tk < 1) face = 'shock';
        }
        if (slot === s.set && t > 0.3) face = 'happy';
        if (st.reason === 'panic') face = 'shock';
        if (done) { face = 'happy'; y = -Math.round(Math.abs(Math.sin(now / 170 + id * 0.8)) * 4); }
        return { id, slot, x, y, z, face, top: 0, v: s.vals[id] };
      });

      for (const g of L) P.ell(w, Math.round(g.x), GY + 1, Math.min(9, this.slot / 2 - 1), 2, P.mix(P.VCOL[this.levelOf(g.v)], '#ffffff', 0.3));
      for (const g of [...L].sort((a, b) => a.z - b.z)) {
        const lv = this.levelOf(g.v);
        const spr = P.gopher(lv, g.face, 0);
        const dx = Math.round(g.x - 8), dy = Math.round(GY + 2 - spr.height + g.y);
        w.drawImage(spr, dx, dy);
        g.top = dy;
        // lencana nilai
        const label = String(g.v).slice(0, 3);
        const bw = Math.max(9, P.textW(label) + 4), bx = Math.round(g.x - bw / 2), by = dy - 10;
        const flash = g.slot === s.set && t > 0.3 && Math.floor(now / 100) % 2;
        P.box(w, bx, by, bw, 9, flash ? '#f3c75a' : P.mix(P.VCOL[lv], P.INK, 0.4), '#ffffff');
        P.textCenter(w, label, g.x, by + 2, '#ffffff');
      }

      const at = slot => L.find(g => g.slot === slot);
      if (s.cmp) {
        const ga = at(s.cmp.a), gb = s.cmp.b != null ? at(s.cmp.b) : null;
        if (ga) {
          const tops = gb ? Math.min(ga.top, gb.top) : ga.top;
          if (t > 0.15 && gb) {
            P.dashedLine(w, Math.round(Math.min(ga.x, gb.x) - 10), Math.round(Math.max(ga.x, gb.x) + 10), ga.top, '#c2457a');
            if (gb.top !== ga.top) P.dashedLine(w, Math.round(Math.min(ga.x, gb.x) - 10), Math.round(Math.max(ga.x, gb.x) + 10), gb.top, '#c2457a');
          }
          const ch = t < 0.45 ? '?' : s.cmp.result;
          const col = t < 0.45 ? P.INK : s.cmp.result === '>' ? '#d9467c' : '#2f8a5c';
          const cx = gb ? (ga.x + gb.x) / 2 : ga.x;
          P.bubble(w, cx, tops - 26 + (t < 0.45 && Math.floor(now / 200) % 2 ? -1 : 0), ch, col);
          if (!gb && s.cmp.value != null && t >= 0.45) P.text(w, String(s.cmp.value).slice(0, 4), Math.round(cx + 7), tops - 23, col);
        }
      }
      if (s.read >= 0 && !s.cmp && !s.swap) {
        const g = at(s.read);
        if (g) { // panah kecil yang memantul di atas elemen yang sedang dibaca
          const ay = g.top - 15 + (Math.floor(now / 180) % 2);
          w.fillStyle = '#d9467c';
          w.fillRect(Math.round(g.x) - 2, ay, 5, 1); w.fillRect(Math.round(g.x) - 1, ay + 1, 3, 1); w.fillRect(Math.round(g.x), ay + 2, 1, 1);
        }
      }
      if (s.swap && t > 0.55) {
        const ga = at(s.swap.a), gb = at(s.swap.b);
        if (ga && gb) P.textCenter(w, 'TUKAR!', (ga.x + gb.x) / 2, Math.min(ga.top, gb.top) - 20 - Math.round((t - 0.55) * 16), '#d9467c');
      }
      let label = null, col = '#d9467c';
      if (st.reason === 'panic') label = 'PANIC!';
      else if (done) { label = 'SELESAI!'; col = '#2f8a5c'; }
      else if (st.phase === 'break' && st.loop === this.snaps[0].step.loop) label = 'BREAK!';
      if (label) {
        const img = P.stamp(label, col), k = t < 0.06 ? 4 : t < 0.12 ? 3 : 2;
        w.drawImage(img, Math.round(this.camX() - (img.width * k) / 2), Math.round(30 - (img.height * k) / 2), img.width * k, img.height * k);
      }
      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), p.s, p.s, p.col);

      this.present(s);
    }

    focusSlots(s) {
      if (s.swap) return [s.swap.a, s.swap.b];
      if (s.cmp) return s.cmp.b != null ? [s.cmp.a, s.cmp.b] : [s.cmp.a];
      if (s.set >= 0) return [s.set];
      if (s.appended >= 0) return [s.appended];
      return null;
    }

    camX() { return this.cam.x; }

    present(s) {
      const f = this.focusSlots(s);
      const zoomable = this.n > 3;
      const tg = f && zoomable
        ? { x: f.reduce((a, k) => a + this.slotX(k), 0) / f.length, y: GY - 26, z: 1.6 }
        : { x: this.worldW / 2, y: H / 2, z: this.worldW > VIEW ? VIEW / this.worldW : 1 };
      if (!f && this.worldW > VIEW) tg.z = 1; // terlalu lebar: tetap skala 1 dan geser kamera
      const c = this.cam;
      c.x += (tg.x - c.x) * 0.1; c.y += (tg.y - c.y) * 0.1; c.z += (tg.z - c.z) * 0.1;
      const z = c.z, S = this.cv.width / VIEW, sc = S * z;
      const hw = VIEW / (2 * z), hh = H / (2 * z);
      const cx = P.clamp(c.x, hw, Math.max(hw, this.worldW - hw)), cy = P.clamp(c.y, hh, H - hh);
      const ctx = this.ctx;
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      ctx.imageSmoothingEnabled = false;
      ctx.fillStyle = '#fde3eb';
      ctx.fillRect(0, 0, this.cv.width, this.cv.height);
      ctx.drawImage(this.world, Math.round(this.cv.width / 2 - cx * sc), Math.round(this.cv.height / 2 - cy * sc), Math.round(this.worldW * sc), Math.round(H * sc));
    }
  }

  G.DataStage = DataStage;
})(window.GLV = window.GLV || {});
