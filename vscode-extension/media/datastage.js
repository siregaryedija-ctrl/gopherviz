// Mode data: elemen slice/array digambar sebagai barisan gopher (tinggi = nilai), seperti
// prototype/bubble-sort.html. Digerakkan langkah demi langkah oleh Trace dari simulator.
(function (G) {
  'use strict';
  const P = G.pixel;

  const VIEW = 192, H = 96, GY = 76;
  const SLOT_MIN = 18, SLOT_MAX = 32;
  // Mode pencarian: nama variabel penunjuk batas kiri/kanan (binary/interpolation search) dan
  // penunjuk lain (mid, pos, step, ...). Hanya tampil bila nilainya indeks yang sah.
  const LO_NAMES = ['lo', 'low', 'left', 'l', 'start', 'begin', 'first', 'kiri', 'awal', 'bawah'];
  const HI_NAMES = ['hi', 'high', 'right', 'r', 'end', 'last', 'kanan', 'akhir', 'atas'];
  const PTR_NAMES = [...LO_NAMES, 'mid', 'm', 'middle', 'tengah', 'pos', 'probe', 'step', 'prev', 'jump', 'idx', 'index', ...HI_NAMES];

  /**
   * Pencarian bawaan (indexOf, in, std::find, bisect, ...) merekam banyak perbandingan dalam satu
   * langkah. Langkah itu dipecah: satu langkah tampilan per elemen yang dicek; output, isi koleksi,
   * dan hasil kondisi hanya dibawa langkah terakhir.
   */
  function expandProbes(steps) {
    const out = [];
    for (const s of steps) {
      const cmps = (s.ops || []).filter(o => o.op === 'compare' && o.b == null);
      if (cmps.length < 2) { out.push(s); continue; }
      const rest = s.ops.filter(o => !cmps.includes(o));
      cmps.forEach((c, k) => {
        const last = k === cmps.length - 1;
        out.push(last ? Object.assign({}, s, { ops: rest.concat([c]) })
          : Object.assign({}, s, { ops: [c], output: '', items: undefined, keys: undefined, result: undefined, phase: 'body', reason: undefined }));
      });
    }
    return out;
  }

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

    // ---------- mode loker (map) ----------

    /** Map: setiap key = satu loker berlabel; snapshot key/nilai per langkah. */
    prepareMap(trace) {
      this.mapMode = true;
      let keys = (trace.collection.keys || []).slice();
      const vals = {};
      keys.forEach((k, i) => { vals[k] = trace.collection.items[i]; });
      this.snaps = trace.steps.map(s => {
        const snap = { step: s, op: null, isNew: false, removed: null };
        for (const o of s.ops || []) {
          if (o.key == null) continue;
          if (o.op === 'set') {
            if (!keys.includes(o.key)) { keys.push(o.key); snap.isNew = true; }
            vals[o.key] = o.value;
            snap.op = o;
          } else if (o.op === 'delete') {
            snap.op = o;
            snap.removed = o.key;
          } else if (o.op === 'read' && !snap.op) {
            snap.op = o;
          }
        }
        if (s.keys && s.items) { // isi map dari simulator adalah sumber kebenaran
          keys = s.keys.slice();
          s.keys.forEach((k, i) => { vals[k] = s.items[i]; });
        }
        snap.keys = keys.slice();
        if (snap.removed && !snap.keys.includes(snap.removed)) snap.keys.push(snap.removed); // tampil sekali lagi saat dihapus
        snap.vals = Object.assign({}, vals);
        if (snap.removed) keys = keys.filter(k => k !== snap.removed);
        return snap;
      });
      const maxN = Math.max(1, ...this.snaps.map(s => s.keys.length));
      this.n = maxN;
      this.slot = 28;
      this.offset = Math.max(10, Math.round((VIEW - this.slot * maxN) / 2));
      this.resize(Math.max(VIEW, this.offset * 2 + this.slot * maxN));
      return this.snaps;
    }

    mapDuration(i) {
      const s = this.snaps[i];
      if (!s.op) return s.step.phase === 'exit' && i === this.snaps.length - 1 ? 2000 : 380;
      return { set: s.isNew ? 950 : 800, read: 650, delete: 900 }[s.op.op] || 500;
    }

    mapCues(i) {
      const s = this.snaps[i], out = [];
      if (s.op) {
        const k = s.keys.indexOf(s.op.key);
        const x = this.slotX(Math.max(0, k));
        if (s.op.op === 'set') out.push({ t: 0.45, sound: s.isNew ? 'land' : 'coin', fx: () => this.burst(x, GY - 34, 8, ['#f3c75a', '#ffffff'], { sp: 50, up: 40, g: 60, life: 500 }) });
        if (s.op.op === 'read') out.push({ t: 0.4, sound: 'blip' });
        if (s.op.op === 'delete') out.push({ t: 0.4, sound: 'door' });
      }
      if (s.step.output) out.push({ t: 0.6, sound: 'coin' });
      if (i === this.snaps.length - 1 && s.step.phase === 'exit') out.push({ t: 0, sound: 'jingle', fx: () => this.confetti() });
      return out;
    }

    renderMap(i, t) {
      const s = this.snaps[i];
      const w = this.wx, now = this.clock;
      w.drawImage(this.bg, 0, 0);
      const target = s.op ? s.keys.indexOf(s.op.key) : -1;
      const prev = this.snaps[i - 1];
      const fromK = prev && prev.op ? prev.keys.indexOf(prev.op.key) : -1;
      const fromX = fromK >= 0 ? this.slotX(fromK) : this.offset - 6;
      const toX = target >= 0 ? this.slotX(target) : fromX;
      const wf = Math.abs(toX - fromX) > 1 ? 0.4 : 0;
      const a = wf ? P.clamp((t - wf) / (1 - wf), 0, 1) : t;

      s.keys.forEach((key, k) => {
        const x = this.slotX(k), active = k === target;
        let y = GY - 30;
        if (active && s.isNew) y -= Math.round((1 - P.bounce(P.clamp(t / 0.5, 0, 1))) * 50);
        const removed = key === s.removed && a > 0.4;
        w.globalAlpha = removed ? 0.35 : 1;
        // badan loker
        P.box(w, x - 10, y, 20, 30, removed ? '#c9c4d0' : '#9fb4d8', P.INK);
        P.rect(w, x - 9, y + 1, 18, 1, '#cfdaf0');
        for (let r = 0; r < 3; r++) P.rect(w, x - 6, y + 4 + r * 2, 12, 1, '#6f84a8'); // ventilasi
        // pelat nilai; berkilau saat ditulis
        const val = String(s.vals[key] != null ? s.vals[key] : '').slice(0, 4);
        const flash = active && s.op.op === 'set' && a > 0.4 && Math.floor(now / 100) % 2;
        P.box(w, x - 8, y + 12, 16, 9, flash ? '#f3c75a' : '#ffffff', P.INK);
        P.textCenter(w, removed ? 'X' : val, x, y + 14, removed ? '#c2457a' : P.INK);
        // label key di atas loker
        P.textCenter(w, String(key).slice(0, 6), x, y - 7, active ? '#c2457a' : '#8a6a98');
        w.globalAlpha = 1;
        if (active && s.op.op === 'read' && a > 0.2) { // panah baca
          const ay = y - 15 + (Math.floor(now / 180) % 2);
          P.rect(w, x - 2, ay, 5, 1, '#d9467c'); P.rect(w, x - 1, ay + 1, 3, 1, '#d9467c'); P.rect(w, x, ay + 2, 1, 1, '#d9467c');
        }
      });

      // gopher berjalan ke loker yang sedang dipakai
      const gx = t < wf ? P.lerp(fromX, toX, P.ease(t / wf)) : toX;
      const face = s.op && s.op.op === 'delete' ? 'shock' : s.op && s.op.op === 'set' && a > 0.4 ? 'happy' : 'idle';
      P.rect(w, 0, GY + 15, this.worldW, 1, '#f2c4d2');
      P.drawGopher(w, gx, GY + 14, face, t < wf ? Math.floor(now / 140) % 2 : 0);
      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), p.s || 1, p.s || 1, p.col);

      P.present(this.ctx, this.cv, this.world, this.worldW, H);
    }

    /** Menyiapkan snapshot per langkah. Mengembalikan daftar langkah untuk pemutar. */
    prepare(trace, loop, loops) {
      this.mapMode = false;
      if (trace.collection.kind === 'map') return this.prepareMap(trace);
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
      this.snaps = expandProbes(trace.steps).map(s => {
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

      // Variabel yang memegang ISI elemen (for v in arr) bukan penunjuk indeks: buang dari penunjuk.
      const elemVars = new Set();
      for (const s of this.snaps) {
        if (s.read < 0 || !s.step.vars) continue;
        for (const name of ptrNames) {
          const v = s.step.vars[name];
          if (v != null && v === String(s.vals[s.order[s.read]]) && +v !== s.read) elemVars.add(name);
        }
      }
      if (elemVars.size) for (const s of this.snaps) s.ptr = s.ptr.filter(p => !elemVars.has(p.name));
      this.prepareSearch(trace);

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
      // mode eliminasi: lebar tetap (balok mengecil), lebih lebar untuk array panjang
      this.resize(this.elim ? (maxN > 24 ? 320 : VIEW) : Math.max(VIEW, this.offset * 2 + this.slot * maxN));
      this.lastIdx = -1;
      return this.snaps;
    }

    /**
     * Pencarian = koleksi hanya dibaca dan elemennya dibandingkan dengan suatu nilai (target).
     * Per langkah: target, elemen yang sudah dicek, elemen yang cocok, rentang lo..hi, dan
     * penunjuk lo/mid/hi. Dihitung ulang setiap kali loop dijalankan lagi (run baru).
     */
    prepareSearch(trace) {
      const snaps = this.snaps;
      this.elim = false;
      this.search = !snaps.some(s => s.swap || s.set >= 0 || s.appended >= 0) && snaps.some(s => s.cmp && s.cmp.b == null);
      if (!this.search) return;
      // "ketemu" hanya bermakna untuk perbandingan kesamaan (==, !=); loop yang hanya memakai < / >
      // (mis. mencari blok di jump search) cukup ditutup dengan SELESAI.
      const eq = c => !c.cmp || c.cmp === '==' || c.cmp === '!=';
      this.eqSearch = snaps.some(s => s.cmp && s.cmp.b == null && eq(s.cmp));
      // penunjuk lo/mid/hi hanya dari nama yang dipakai di blok ini (variabel lama di luar blok diabaikan)
      const names = new Set(trace.block.names || []);
      const used = name => !names.size || names.has(name) || names.has('$' + name);
      let run = null, target = null, checked = new Set(), found = -1, count = 0;
      for (const s of snaps) {
        if (s.step.run !== run) { run = s.step.run; checked = new Set(); found = -1; count = 0; }
        s.checked = new Set(checked); // dicek SEBELUM langkah ini
        s.foundBefore = found;
        if (s.cmp && s.cmp.b == null) {
          target = s.cmp.value;
          count++;
          checked.add(s.cmp.a);
          if (s.cmp.result === '=' && eq(s.cmp) && found < 0) found = s.cmp.a;
        }
        s.target = target;
        s.checks = count;
        s.found = found;
        const n = s.order.length, vars = s.step.vars || {};
        const idx = name => (used(name) && vars[name] != null && /^-?\d+$/.test(vars[name]) ? +vars[name] : null);
        const lo = LO_NAMES.map(idx).find(v => v != null), hi = HI_NAMES.map(idx).find(v => v != null);
        s.range = lo != null && hi != null ? { lo: Math.max(0, lo), hi: Math.min(n - 1, hi) } : null;
        for (const name of PTR_NAMES) {
          const k = idx(name);
          if (k != null && k >= 0 && k < n && !s.ptr.some(p => p.name === name)) s.ptr.push({ name, k });
        }
      }
      this.prepareElimination(trace);
    }

    /**
     * Mode eliminasi (pencarian kesamaan atau dengan rentang lo..hi): setiap langkah menyisakan
     * kandidat. Linear: elemen yang dicek dan tidak cocok dibuang. Koleksi terurut: hasil < / >
     * membuang satu sisi sekaligus (binary search, bisect). Variabel lo/hi: yang di luar rentang
     * dibuang. Ketemu: semua selain target dibuang. s.aliveBefore / s.alive = kandidat sebelum /
     * sesudah langkah (urut indeks); panggung menjatuhkan yang dibuang lalu membesarkan sisanya.
     */
    prepareElimination(trace) {
      const snaps = this.snaps;
      this.elim = this.eqSearch || snaps.some(s => s.range);
      if (!this.elim) return;
      const items = trace.collection.items;
      const nums = items.map(Number);
      const sorted = items.length > 1 && nums.every(x => !isNaN(x)) && nums.every((x, k) => k === 0 || nums[k - 1] <= x);
      const all = () => items.map((_, k) => k);
      let run = null, alive = all();
      for (const s of snaps) {
        if (s.step.run !== run) { run = s.step.run; alive = all(); }
        s.aliveBefore = alive;
        let next = alive;
        const c = s.cmp && s.cmp.b == null ? s.cmp : null;
        if (c && c.result === '=' && (!c.cmp || c.cmp === '==' || c.cmp === '!=')) next = [c.a];
        else if (c && sorted && c.result === '<') next = alive.filter(k => k > c.a);
        else if (c && sorted && c.result === '>') next = alive.filter(k => k < c.a);
        else if (c && c.result !== '=') next = alive.filter(k => k !== c.a);
        if (s.range) next = next.filter(k => k >= s.range.lo && k <= s.range.hi);
        s.alive = alive = next;
      }
    }

    /** Tata letak kandidat: lebar slot (piksel dunia) dan posisi tengah slot ke-r. */
    elimLayout(count) {
      const sw = P.clamp(Math.floor((this.worldW - 16) / Math.max(1, count)), 2, 30);
      const x0 = (this.worldW - sw * count) / 2;
      return { sw, x: r => x0 + (r + 0.5) * sw };
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
      if (this.mapMode) return this.mapDuration(i);
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
      if (this.mapMode) return this.mapCues(i);
      const s = this.snaps[i], st = s.step, out = [];
      const last = i === this.snaps.length - 1;
      if (s.cmp) {
        out.push({ t: 0, sound: 'think' });
        const yes = this.search ? s.cmp.result === '=' : st.result != null ? st.result : s.cmp.result === '>';
        out.push({ t: 0.45, sound: yes ? 'yes' : 'no' });
        if (this.search && this.eqSearch && s.cmp.result === '=' && (!s.cmp.cmp || s.cmp.cmp === '==' || s.cmp.cmp === '!=')) out.push({ t: 0.5, sound: 'sparkle', fx: () => this.burst(this.elim ? this.worldW / 2 : this.slotX(s.cmp.a), GY - 40, 12, ['#f3c75a', '#ffffff', '#9fd88f'], { sp: 70, up: 50, g: 60, life: 700 }) });
        if (this.elim && s.alive.length < s.aliveBefore.length - 1) out.push({ t: 0.6, sound: 'warp' }); // banyak yang dibuang sekaligus
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
      if (this.mapMode) { this.renderMap(i, t); return; }
      if (this.elim) { this.renderElim(i, t); return; }
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

      // pencarian: elemen di luar rentang lo..hi (atau yang sudah dicek dan tidak cocok) meredup
      const matchNow = this.search && s.cmp && s.cmp.result === '=' && s.cmp.b == null && (!s.cmp.cmp || s.cmp.cmp === '==' || s.cmp.cmp === '!=') && t >= 0.45 ? s.cmp.a : -1;
      const foundSlot = matchNow >= 0 ? matchNow : this.search ? s.foundBefore : -1;
      const dim = g => {
        if (!this.search || g.slot === foundSlot) return 1;
        if (s.range) return g.slot < s.range.lo || g.slot > s.range.hi ? 0.3 : 1;
        return s.checked.has(g.slot) && !(s.cmp && s.cmp.a === g.slot) ? 0.45 : 1;
      };
      if (this.search) for (const g of L) {
        if (g.slot === foundSlot) { g.face = 'happy'; if (!done) g.y = -Math.round(Math.abs(Math.sin(now / 150)) * 3); }
        else if (done) { g.face = (now + g.id * 1370) % 3400 < 130 ? 'blink' : 'idle'; g.y = 0; }
      }
      for (const g of L) { w.globalAlpha = dim(g); P.ell(w, Math.round(g.x), GY + 1, Math.min(9, this.slot / 2 - 1), 2, P.mix(P.VCOL[this.levelOf(g.v)], '#ffffff', 0.3)); }
      w.globalAlpha = 1;
      for (const g of [...L].sort((a, b) => a.z - b.z)) {
        const lv = this.levelOf(g.v);
        const spr = P.gopher(lv, g.face, 0);
        const dx = Math.round(g.x - 8), dy = Math.round(GY + 2 - spr.height + g.y);
        w.globalAlpha = dim(g);
        w.drawImage(spr, dx, dy);
        g.top = dy;
        // lencana nilai
        const label = String(g.v).slice(0, 3);
        const bw = Math.max(9, P.textW(label) + 4), bx = Math.round(g.x - bw / 2), by = dy - 10;
        const flash = g.slot === s.set && t > 0.3 && Math.floor(now / 100) % 2;
        P.box(w, bx, by, bw, 9, flash ? '#f3c75a' : P.mix(P.VCOL[lv], P.INK, 0.4), '#ffffff');
        P.textCenter(w, label, g.x, by + 2, '#ffffff');
        w.globalAlpha = 1;
        if (g.slot === foundSlot) this.star(w, g.x, by - 9 - (Math.floor(now / 250) % 2));
      }
      if (this.search && s.range) { // kurung siku rentang pencarian lo..hi di bawah barisan
        const x0 = Math.round(this.slotX(s.range.lo) - this.slot / 2 + 1), x1 = Math.round(this.slotX(s.range.hi) + this.slot / 2 - 2);
        if (x1 > x0) { P.rect(w, x0, GY + 2, x1 - x0 + 1, 1, '#c2457a'); P.rect(w, x0, GY, 1, 3, '#c2457a'); P.rect(w, x1, GY, 1, 3, '#c2457a'); }
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
      else if (done && this.search && this.eqSearch) { label = s.found >= 0 ? 'KETEMU!' : 'TIDAK ADA'; col = s.found >= 0 ? '#2f8a5c' : '#d9467c'; }
      else if (done) { label = 'SELESAI!'; col = '#2f8a5c'; }
      else if (matchNow >= 0 && t > 0.55) { label = 'KETEMU!'; col = '#2f8a5c'; }
      else if (st.phase === 'break' && st.loop === this.snaps[0].step.loop) label = 'BREAK!';
      if (label) {
        const img = P.stamp(label, col), k = t < 0.06 ? 4 : t < 0.12 ? 3 : 2;
        w.drawImage(img, Math.round(this.camX() - (img.width * k) / 2), Math.round(30 - (img.height * k) / 2), img.width * k, img.height * k);
      }
      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), p.s, p.s, p.col);
      if (this.search) { // HUD: nilai yang dicari dan jumlah elemen yang sudah dicek
        P.rect(w, 0, 0, this.worldW, 16, 'rgba(255,248,251,0.75)');
        P.text(w, s.target != null ? `CARI ${String(s.target).slice(0, 10)}` : 'CARI', 4, 3, P.INK);
        P.text(w, `CEK ${s.checks}`, 4, 10, '#c2457a');
      }

      this.present(s);
    }

    /**
     * Panggung eliminasi: kandidat dijejer selebar panggung; yang dibuang jatuh dan memudar,
     * sisanya merapat dan membesar. Tampilan elemen mengikuti lebar slot: Gopher berlencana
     * (lebar), balok berangka (sedang), atau batang tipis (sempit, mis. 100 elemen).
     */
    renderElim(i, t) {
      const s = this.snaps[i], st = s.step, w = this.wx, now = this.clock;
      const last = i === this.snaps.length - 1;
      const done = last && (st.phase === 'exit' || st.phase === 'return') && st.reason !== 'panic';
      w.drawImage(this.bg, 0, 0);
      const before = this.elimLayout(s.aliveBefore.length), after = this.elimLayout(s.alive.length);
      const u = P.ease(P.clamp((t - 0.55) / 0.45, 0, 1)); // 0 = sebelum, 1 = sesudah eliminasi
      const keep = new Set(s.alive);
      const cmpA = s.cmp && s.cmp.b == null ? s.cmp.a : -1;
      const found = s.found >= 0 && keep.has(s.found) ? s.found : -1;
      const items = [];
      s.aliveBefore.forEach((k, r) => {
        const alive = keep.has(k);
        const x = alive ? P.lerp(before.x(r), after.x(s.alive.indexOf(k)), u) : before.x(r);
        const sw = alive ? P.lerp(before.sw, after.sw, u) : before.sw;
        items.push({ k, x, sw, alive, fall: alive ? 0 : u, v: s.vals[s.order[k]] });
      });
      // tanah + garis indeks
      P.rect(w, 0, GY + 1, this.worldW, 1, '#f2c4d2');
      for (const it of items) {
        const lv = this.levelOf(it.v), col = P.VCOL[lv];
        const y = Math.round(it.fall * it.fall * 60);
        w.globalAlpha = 1 - it.fall;
        const isCmp = it.k === cmpA && it.fall === 0, isFound = it.k === found;
        if (it.sw >= 18) { // Gopher berlencana
          let face = (now + it.k * 1370) % 3400 < 130 ? 'blink' : 'idle';
          if (isCmp) face = t < 0.45 ? 'think' : s.cmp.result === '=' ? 'happy' : 'shock';
          if (it.fall > 0) face = 'shock';
          if (isFound) face = 'happy';
          const hop = isFound ? Math.round(Math.abs(Math.sin(now / 150)) * 3) : 0;
          const spr = P.gopher(lv, face, 0);
          const dy = Math.round(GY + 2 - spr.height + y - hop);
          w.drawImage(spr, Math.round(it.x - 8), dy);
          const label = String(it.v).slice(0, 3), bw = Math.max(9, P.textW(label) + 4);
          P.box(w, Math.round(it.x - bw / 2), dy - 10, bw, 9, P.mix(col, P.INK, 0.4), '#ffffff');
          P.textCenter(w, label, it.x, dy - 8, '#ffffff');
          it.top = dy - 10;
        } else { // balok / batang
          const bw = Math.max(1, Math.round(it.sw) - 1), bh = 8 + lv * 4;
          const bx = Math.round(it.x - bw / 2), by = GY + 1 - bh + y;
          P.rect(w, bx, by, bw, bh, col);
          if (bw >= 4) { P.rect(w, bx, by, bw, 1, P.mix(col, '#ffffff', 0.5)); P.rect(w, bx, by + bh - 1, bw, 1, P.mix(col, P.INK, 0.35)); }
          const label = String(it.v).slice(0, 3);
          if (bw >= P.textW(label) + 2) P.textCenter(w, label, it.x, by + 2, '#ffffff');
          if (isCmp || isFound) { P.rect(w, bx - 1, by - 1, bw + 2, 1, '#d9467c'); P.rect(w, bx - 1, by - 1, 1, bh + 1, '#d9467c'); P.rect(w, bx + bw, by - 1, 1, bh + 1, '#d9467c'); }
          it.top = by;
        }
        w.globalAlpha = 1;
        if (isFound) this.star(w, it.x, it.top - 9 - (Math.floor(now / 250) % 2));
      }
      // indeks di bawah: semua bila muat, selain itu ujung kiri/kanan dan yang sedang dicek
      const vis = items.filter(it => it.alive);
      const room = vis.length && vis[0].sw >= 10;
      for (const it of vis) {
        if (room || it === vis[0] || it === vis[vis.length - 1] || it.k === cmpA) P.textCenter(w, String(it.k), it.x, GY + 4, it.k === cmpA ? '#c2457a' : '#d9a0b6');
      }
      const stack = {};
      for (const p of s.ptr) { // penunjuk lo/mid/hi di bawah elemennya
        const it = items.find(x => x.k === p.k && x.alive);
        if (!it) continue;
        const row = stack[p.k] = (stack[p.k] || 0) + 1;
        P.textCenter(w, p.name.slice(0, 3), it.x, GY + 4 + row * 6, '#c2457a');
      }
      // gelembung hasil perbandingan di atas elemen yang dicek
      const ci = items.find(it => it.k === cmpA);
      if (ci && ci.fall < 0.5) {
        const ch = t < 0.45 ? '?' : s.cmp.result, colr = t < 0.45 ? P.INK : s.cmp.result === '=' ? '#2f8a5c' : '#d9467c';
        const by = Math.max(18, ci.top - 26 + (t < 0.45 && Math.floor(now / 200) % 2 ? -1 : 0));
        const bx = P.clamp(ci.x, 8, this.worldW - 30);
        P.bubble(w, bx, by, ch, colr);
        P.text(w, String(ci.v).slice(0, 5), Math.round(bx + 7), by + 3, P.INK);
      }
      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), p.s, p.s, p.col);
      // HUD
      P.rect(w, 0, 0, this.worldW, 16, 'rgba(255,248,251,0.75)');
      P.text(w, s.target != null ? `CARI ${String(s.target).slice(0, 10)}` : 'CARI', 4, 3, P.INK);
      P.text(w, `CEK ${s.checks}  SISA ${s.alive.length}`, 4, 10, '#c2457a');
      let label = null, col = '#2f8a5c';
      if (st.reason === 'panic') { label = 'PANIC!'; col = '#d9467c'; }
      else if (done) { label = !this.eqSearch ? 'SELESAI!' : s.found >= 0 ? 'KETEMU!' : 'TIDAK ADA'; if (this.eqSearch && s.found < 0) col = '#d9467c'; }
      else if (s.cmp && s.cmp.result === '=' && keep.size === 1 && t > 0.6) label = 'KETEMU!';
      if (label) {
        const img = P.stamp(label, col);
        w.drawImage(img, Math.round(this.worldW - img.width * 2 - 6), 2, img.width * 2, img.height * 2);
      }
      this.present(s);
    }

    /** Bintang kecil 7x7 di atas elemen yang cocok dengan target. */
    star(w, cx, y) {
      const rows = ['...#...', '..###..', '#######', '.#####.', '..###..', '.##.##.', '.#...#.'];
      w.fillStyle = '#f3c75a';
      rows.forEach((r, j) => { for (let i = 0; i < 7; i++) if (r[i] === '#') w.fillRect(Math.round(cx) - 3 + i, y + j, 1, 1); });
    }

    focusSlots(s) {
      if (s.swap) return [s.swap.a, s.swap.b];
      if (s.cmp) return s.cmp.b != null ? [s.cmp.a, s.cmp.b] : [s.cmp.a];
      if (s.set >= 0) return [s.set];
      if (s.appended >= 0) return [s.appended];
      return null;
    }

    camX() { return this.worldW / 2; }

    /** Seluruh deretan terlihat sekaligus (tanpa zoom/kamera). */
    present() {
      P.present(this.ctx, this.cv, this.world, this.worldW, H);
    }
  }

  G.DataStage = DataStage;
})(window.GV = window.GV || {});
