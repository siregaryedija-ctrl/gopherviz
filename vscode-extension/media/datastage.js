// Mode data: elemen slice/array digambar sebagai barisan kubus berangka (warna = peringkat nilai);
// Gopher berdiri di atas kubus yang sedang dikerjakan. Pencarian memakai grid kubus yang dipotong
// (referensi: video binary search penguin). Digerakkan langkah demi langkah oleh Trace dari simulator.
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
      this.wx = this.world.getContext('2d');
      this.bg = document.createElement('canvas');
      this.worldH = H;
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
      this.sort = null;
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
      this.resize(Math.max(VIEW, this.offset * 2 + this.slot * maxN), H, false);
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

      P.present(this.ctx, this.cv, this.world, this.worldW, this.worldH);
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

      let cmp = 0, swp = 0, wr = 0, maxN = ids.length;
      this.snaps = expandProbes(trace.steps).map(s => {
        const snap = { step: s, prevOrder: null, cmp: null, swap: null, read: -1, set: -1, appended: -1 };
        for (const o of s.ops || []) {
          if (o.op === 'compare') { snap.cmp = o; cmp++; }
          else if (o.op === 'swap' && o.b != null && o.b < ids.length) {
            snap.prevOrder = ids.slice();
            [ids[o.a], ids[o.b]] = [ids[o.b], ids[o.a]];
            snap.swap = o; swp++;
          } else if (o.op === 'set' && o.a < ids.length) { vals[ids[o.a]] = o.value; snap.set = o.a; seen.add(o.value); wr++; }
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
        snap.setCount = wr;
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
      this.prepareSort(trace);

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
      // mode eliminasi: panggung grid berukuran tetap (kubus mengecil), lebih besar untuk array panjang
      if (this.elim) this.resize(maxN > 24 ? 320 : VIEW, maxN > 24 ? 176 : 120, true);
      else this.resize(Math.max(VIEW, this.offset * 2 + this.slot * maxN), H, false);
      // posisi Gopher per langkah: di atas kubus yang sedang dikerjakan, tetap di tempat bila tidak ada
      let gx = this.slotX(0);
      for (const s of this.snaps) {
        s.gxFrom = gx;
        const f = this.focusSlots(s) || (s.read >= 0 ? [s.read] : null);
        if (f) gx = f.reduce((a, k) => a + this.slotX(k), 0) / f.length;
        s.gx = gx;
      }
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
      // target sudah tampil sejak awal run (sebelum perbandingan pertama)
      let ahead = null;
      run = null;
      for (let k = snaps.length - 1; k >= 0; k--) {
        const s = snaps[k];
        if (s.step.run !== run) { run = s.step.run; ahead = null; }
        if (s.target == null || (s.checks === 0 && ahead != null)) s.target = ahead; else ahead = s.target;
      }
      this.prepareElimination(trace);
    }

    /**
     * Sorting (Fase 12c): penanda sesuai algoritma yang dikenali sorts.js. Per langkah:
     * minK (selection: indeks nilai terkecil sementara), sortedTo (selection: indeks < i sudah final),
     * pivotK + range (quick: pivot dan rentang partisi lo..hi), key + keyAt (insertion: nilai yang
     * sedang disisipkan melayang di atas celahnya).
     */
    prepareSort(trace) {
      this.sort = !this.search && G.sorts ? G.sorts.detect(trace) : null;
      const so = this.sort;
      if (!so) return;
      const idx = (s, name) => {
        const v = name && s.step.vars ? s.step.vars[name] : null;
        return v != null && /^-?\d+$/.test(v) ? +v : null;
      };
      const marker = new Set([so.min, so.lo, so.hi, so.key, so.pivot].filter(Boolean));
      for (const s of this.snaps) {
        const n = s.order.length, inRange = k => k != null && k >= 0 && k < n;
        s.ptr = s.ptr.filter(p => !marker.has(p.name)); // nama yang sudah punya penanda khusus
        if (so.kind === 'selection') {
          const m = idx(s, so.min), i = idx(s, so.outer);
          s.minK = inRange(m) ? m : null;
          s.sortedTo = i != null ? Math.max(0, Math.min(n, i)) : 0;
        }
        if (so.kind === 'quick' || so.kind === 'merge') {
          const lo = idx(s, so.lo), hi = idx(s, so.hi);
          s.part = lo != null && hi != null && hi >= lo ? { lo: Math.max(0, lo), hi: Math.min(n - 1, hi) } : null;
        }
        if (so.kind === 'quick' && s.step.vars && s.step.vars[so.pivot] != null) {
          const pv = s.step.vars[so.pivot], val = k => String(s.vals[s.order[k]]);
          const lo = s.part ? s.part.lo : 0, hi = s.part ? s.part.hi : n - 1;
          let at = null;
          if (inRange(hi) && val(hi) === pv) at = hi; // Lomuto: pivot di ujung kanan
          else if (inRange(lo) && val(lo) === pv) at = lo; // Hoare: pivot di ujung kiri
          else for (let k = lo; k <= hi; k++) if (val(k) === pv) { at = k; break; }
          s.pivotV = pv;
          s.pivotK = at;
        }
        if (so.kind === 'insertion' && s.step.vars && s.step.vars[so.key] != null) {
          const j = idx(s, 'j');
          s.keyV = s.step.vars[so.key];
          s.keyAt = j != null ? Math.max(0, Math.min(n - 1, j + 1)) : idx(s, so.outer);
        }
      }
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

    /**
     * Grid kandidat seperti lantai petak di video referensi: m kubus diurutkan indeks, baris demi
     * baris, sebesar mungkin yang muat (sedikit kandidat = satu baris). p = jarak antarsel,
     * s = sisi depan kubus, d = kedalaman; pos(r) = pojok kiri atas sisi depan sel ke-r.
     */
    gridLayout(m) {
      const top = 22, AW = this.worldW - 16, AH = this.worldH - top - 6;
      m = Math.max(1, m);
      let best = null;
      if (AW / m >= 18) best = { cols: m, rows: 1, p: Math.min(Math.floor(AW / m), AH, 40) };
      else for (let cols = 1; cols <= m; cols++) {
        const rows = Math.ceil(m / cols), p = Math.min(Math.floor(AW / cols), Math.floor(AH / rows), 40);
        if (!best || p > best.p) best = { cols, rows, p };
      }
      const cols = best.cols, rows = best.rows, p = Math.max(1, best.p);
      const d = p < 6 ? 0 : Math.max(1, Math.round(p / 5)), gap = p >= 8 ? 1 : 0, s = Math.max(1, p - d - gap);
      const x0 = Math.round((this.worldW - cols * p) / 2), y0 = Math.round(top + (AH - rows * p) / 2);
      return { cols, rows, p, s, d, pos: r => ({ x: x0 + (r % cols) * p, y: y0 + d + Math.floor(r / cols) * p }) };
    }

    resize(w, h, grid) {
      if (w === this.worldW && h === this.worldH && grid === this.gridBg) return;
      this.worldW = w; this.worldH = h; this.gridBg = grid;
      this.world.width = this.bg.width = w;
      this.world.height = this.bg.height = h;
      const c = this.bg.getContext('2d');
      if (!grid) { P.paintWorld(c, w, h, GY); return; }
      P.paintSky(c, w, h); // langit + alas petak tempat grid kubus
      P.box(c, 4, 19, w - 8, h - 22, '#fff4f7', '#f2c4d2');
      c.fillStyle = '#fbe3e8';
      for (let y = 21; y < h - 4; y += 4) for (let x = 6 + (y % 8 ? 2 : 0); x < w - 6; x += 4) c.fillRect(x, y, 1, 1);
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
        if (this.search && this.eqSearch && s.cmp.result === '=' && (!s.cmp.cmp || s.cmp.cmp === '==' || s.cmp.cmp === '!=')) out.push({ t: 0.5, sound: 'sparkle', fx: () => this.burst(this.elim ? this.worldW / 2 : this.slotX(s.cmp.a), this.elim ? this.worldH / 2 : GY - 40, 12, ['#f3c75a', '#ffffff', '#9fd88f'], { sp: 70, up: 50, g: 60, life: 700 }) });
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
        return p.life > 0 && p.y < this.worldH + 4;
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
        if (this.sort && s.part) return g.slot < s.part.lo || g.slot > s.part.hi ? 0.35 : 1; // quick: di luar partisi
        if (!this.search || g.slot === foundSlot) return 1;
        if (s.range) return g.slot < s.range.lo || g.slot > s.range.hi ? 0.3 : 1;
        return s.checked.has(g.slot) && !(s.cmp && s.cmp.a === g.slot) ? 0.45 : 1;
      };
      if (this.search) for (const g of L) {
        if (g.slot === foundSlot) { g.face = 'happy'; if (!done) g.y = -Math.round(Math.abs(Math.sin(now / 150)) * 3); }
        else if (done) { g.face = (now + g.id * 1370) % 3400 < 130 ? 'blink' : 'idle'; g.y = 0; }
      }
      // kubus berangka: warna = peringkat nilai; yang dibandingkan terangkat dan berbingkai
      const d = Math.max(2, Math.round(this.slot / 6)), cs = this.slot - d - 4;
      const cmpSlot = g => s.cmp && !s.prevOrder && (g.slot === s.cmp.a || g.slot === s.cmp.b);
      for (const g of L) { w.globalAlpha = dim(g); P.rect(w, Math.round(g.x - cs / 2), GY + 1, cs + d, 1, P.mix(P.VCOL[this.levelOf(g.v)], P.INK, 0.15)); }
      w.globalAlpha = 1;
      for (const g of [...L].sort((a, b) => a.z - b.z || a.slot - b.slot)) {
        const lv = this.levelOf(g.v);
        const flash = g.slot === s.set && t > 0.3 && Math.floor(now / 100) % 2;
        const lift = cmpSlot(g) ? Math.round(2 * P.clamp(t / 0.15, 0, 1)) : 0;
        const x = Math.round(g.x - (cs + d) / 2), y = Math.round(GY + 1 - cs + g.y) - lift;
        w.globalAlpha = dim(g);
        const ring = cmpSlot(g) || g.slot === foundSlot ? '#d9467c' : g.slot === s.pivotK ? '#e0a22a' : g.slot === s.minK ? '#2f8a5c' : null;
        P.cube(w, x, y, cs, cs, flash ? '#f3c75a' : P.VCOL[lv], d, ring);
        P.textCenter(w, String(g.v).slice(0, Math.max(1, Math.floor(cs / 4))), x + cs / 2, y + Math.floor((cs - 5) / 2), P.INK);
        w.globalAlpha = 1;
        g.top = y - d;
        if (g.slot === foundSlot) this.star(w, g.x, g.top - 9 - (Math.floor(now / 250) % 2));
      }

      if (this.sort) this.drawSortMarks(w, s, L, cs, d, now);

      // Gopher berdiri di atas kubus yang sedang dikerjakan; melompat dari kubus sebelumnya
      const hopT = P.clamp(t / 0.3, 0, 1), moving = Math.abs(s.gx - s.gxFrom) > 1 && hopT < 1;
      const gx = moving ? P.lerp(s.gxFrom, s.gx, P.ease(hopT)) : s.gx;
      let gy = GY - cs - Math.ceil(d / 2) - (moving ? Math.round(Math.sin(Math.PI * hopT) * 10) : 0);
      let face = (now % 3400) < 130 ? 'blink' : 'idle';
      if (s.cmp && !s.prevOrder) {
        const yes = this.search ? s.cmp.result === '=' : st.result != null ? st.result : s.cmp.result === '>';
        face = t < 0.45 ? 'think' : yes ? 'happy' : 'shock';
      }
      if (s.swap) { face = 'happy'; gy -= Math.round(Math.sin(Math.PI * t) * 20); }
      if (s.set >= 0 && t > 0.3) face = 'happy';
      if (st.reason === 'panic') face = 'shock';
      if (done) { face = 'happy'; gy -= Math.round(Math.abs(Math.sin(now / 150)) * 4); }
      const head = P.drawGopher(w, gx, gy, face, moving ? Math.floor(now / 120) % 2 : 0);
      if (this.search && s.range) { // kurung siku rentang pencarian lo..hi di bawah barisan
        const x0 = Math.round(this.slotX(s.range.lo) - this.slot / 2 + 1), x1 = Math.round(this.slotX(s.range.hi) + this.slot / 2 - 2);
        if (x1 > x0) { P.rect(w, x0, GY + 2, x1 - x0 + 1, 1, '#c2457a'); P.rect(w, x0, GY, 1, 3, '#c2457a'); P.rect(w, x1, GY, 1, 3, '#c2457a'); }
      }

      const at = slot => L.find(g => g.slot === slot);
      if (s.cmp && !s.prevOrder) { // balon hasil perbandingan di atas kepala Gopher
        const ch = t < 0.45 ? '?' : s.cmp.result;
        const col = t < 0.45 ? P.INK : s.cmp.result === '>' ? '#d9467c' : '#2f8a5c';
        const by = Math.max(1, head - 14 + (t < 0.45 && Math.floor(now / 200) % 2 ? -1 : 0));
        P.bubble(w, gx, by, ch, col);
        if (s.cmp.b == null && s.cmp.value != null && t >= 0.45) P.text(w, String(s.cmp.value).slice(0, 4), Math.round(gx + 7), by + 3, col);
      }
      if (s.swap && t > 0.55) {
        const ga = at(s.swap.a), gb = at(s.swap.b);
        if (ga && gb) P.textCenter(w, 'TUKAR!', (ga.x + gb.x) / 2, Math.max(1, head - 8 - Math.round((t - 0.55) * 16)), '#d9467c');
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
      if (this.sort) { // HUD: nama algoritma + jumlah perbandingan dan pertukaran
        P.rect(w, 0, 0, this.worldW, 16, 'rgba(255,248,251,0.75)');
        P.text(w, this.sort.title, 4, 3, P.INK);
        const moves = this.sort.kind === 'insertion' ? `GESER ${s.setCount}` : this.sort.kind === 'merge' ? `TULIS ${s.setCount}` : `TUKAR ${s.swpCount}`;
        P.text(w, `BANDING ${s.cmpCount}  ${moves}`, 4, 10, '#c2457a');
      }
      if (this.search) { // HUD: nilai yang dicari dan jumlah elemen yang sudah dicek
        P.rect(w, 0, 0, this.worldW, 16, 'rgba(255,248,251,0.75)');
        P.text(w, s.target != null ? `CARI ${String(s.target).slice(0, 10)}` : 'CARI', 4, 3, P.INK);
        P.text(w, `CEK ${s.checks}`, 4, 10, '#c2457a');
      }

      this.present(s);
    }

    /** Penanda sorting: MIN, PIVOT, KEY melayang, kurung partisi, dan alas hijau bagian yang sudah final. */
    drawSortMarks(w, s, L, cs, d, now) {
      const at = slot => L.find(g => g.slot === slot);
      const tag = (g, label, col) => {
        if (!g) return;
        const y = g.top - 8 - (Math.floor(now / 300) % 2);
        P.box(w, Math.round(g.x - P.textW(label) / 2 - 2), y - 1, P.textW(label) + 4, 7, '#fff8fb', col);
        P.textCenter(w, label, g.x, y, col);
      };
      if (s.sortedTo) { // selection: indeks < i sudah di posisi akhirnya
        const x0 = Math.round(this.slotX(0) - this.slot / 2 + 1), x1 = Math.round(this.slotX(s.sortedTo - 1) + this.slot / 2 - 2);
        if (x1 > x0) P.rect(w, x0, GY + 1, x1 - x0 + 1, 2, '#7cc68b');
      }
      if (s.part) { // quick/merge: kurung siku rentang lo..hi
        const x0 = Math.round(this.slotX(s.part.lo) - this.slot / 2 + 1), x1 = Math.round(this.slotX(s.part.hi) + this.slot / 2 - 2);
        if (x1 > x0) { P.rect(w, x0, GY + 2, x1 - x0 + 1, 1, '#c2457a'); P.rect(w, x0, GY, 1, 3, '#c2457a'); P.rect(w, x1, GY, 1, 3, '#c2457a'); }
      }
      if (s.minK != null) tag(at(s.minK), 'MIN', '#2f8a5c');
      if (s.pivotK != null) tag(at(s.pivotK), 'PIVOT', '#c27a12');
      else if (s.pivotV != null) P.text(w, `PIVOT ${String(s.pivotV).slice(0, 4)}`, this.worldW - 44, 3, '#c27a12');
      if (s.keyV != null && s.keyAt != null) { // insertion: nilai key melayang di atas celahnya
        const ks = Math.max(9, Math.round(cs * 0.65)), kd = Math.max(1, d - 1);
        const x = Math.round(this.slotX(s.keyAt) - (ks + kd) / 2), y = Math.max(26, Math.round(GY + 1 - cs - ks - 10 + Math.sin(now / 260) * 2));
        P.cube(w, x, y, ks, ks, '#fff3a8', kd, '#c27a12');
        P.textCenter(w, String(s.keyV).slice(0, Math.max(1, Math.floor(ks / 4))), x + ks / 2, y + Math.floor((ks - 5) / 2), P.INK);
        P.textCenter(w, 'KEY', x + ks / 2, y - kd - 7, '#c27a12');
      }
    }

    /**
     * Panggung eliminasi (referensi: video binary search penguin): kandidat = grid kubus. Langkah:
     * kubus yang dicek terangkat di bawah sorot cahaya dan balon "nilai ? target" muncul; garis
     * potong bersinar di antara yang tersisa dan yang dibuang; yang dibuang meluncur keluar
     * (indeks lebih kecil ke kiri, lebih besar ke kanan, di tengah jatuh); sisanya disusun ulang ke
     * grid yang lebih kecil sehingga kubusnya membesar. Yang ketemu dinaiki Gopher.
     */
    renderElim(i, t) {
      const s = this.snaps[i], st = s.step, w = this.wx, now = this.clock, W = this.worldW;
      const last = i === this.snaps.length - 1;
      const done = last && (st.phase === 'exit' || st.phase === 'return') && st.reason !== 'panic';
      w.drawImage(this.bg, 0, 0);
      const A = this.gridLayout(s.aliveBefore.length), B = this.gridLayout(s.alive.length);
      const keep = new Set(s.alive), rankB = new Map(s.alive.map((k, r) => [k, r]));
      const lo = s.alive.length ? s.alive[0] : Infinity, hi = s.alive.length ? s.alive[s.alive.length - 1] : -Infinity;
      const cmpA = s.cmp && s.cmp.b == null ? s.cmp.a : -1;
      const found = s.found >= 0 && keep.has(s.found) ? s.found : -1;
      const cut = P.clamp((t - 0.5) / 0.28, 0, 1), slide = cut * cut; // yang dibuang meluncur keluar
      const zoom = P.ease(P.clamp((t - 0.72) / 0.28, 0, 1)); // sisanya membesar mengisi alas
      const items = s.aliveBefore.map((k, r) => {
        const a = A.pos(r), v = s.vals[s.order[k]];
        const it = { k, r, v, alive: keep.has(k), cell: a };
        if (it.alive) {
          const b = B.pos(rankB.get(k));
          Object.assign(it, { x: P.lerp(a.x, b.x, zoom), y: P.lerp(a.y, b.y, zoom), s: P.lerp(A.s, B.s, zoom), d: P.lerp(A.d, B.d, zoom), alpha: 1 });
        } else {
          const dir = k > hi ? 1 : k < lo ? -1 : 0;
          Object.assign(it, { x: a.x + dir * slide * W * 0.9, y: a.y + (dir ? 0 : slide * 50), s: A.s, d: A.d, alpha: 1 - cut });
        }
        if (k === cmpA) it.y -= Math.round(3 * P.clamp(t / 0.15, 0, 1) * (1 - cut));
        return it;
      });
      const ci = items.find(it => it.k === cmpA);

      // sorot cahaya dari atas ke kubus yang sedang dicek
      if (ci && t < 0.6) {
        const f = 1 - P.clamp((t - 0.45) / 0.15, 0, 1), bx = Math.round(ci.x), top = Math.round(ci.y - ci.d);
        w.globalAlpha = 0.28 * f;
        P.rect(w, bx, 18, Math.round(ci.s + ci.d), Math.max(0, top - 18), '#ffffff');
        w.globalAlpha = 0.7 * f;
        P.rect(w, Math.round(ci.x + (ci.s + ci.d) / 2), 18, 1, Math.max(0, top - 18), '#ffffff');
        w.globalAlpha = 1;
      }

      // kubus: baris belakang dulu, lalu kiri ke kanan (sisi atas/kanan tertutup tetangga depan)
      const order = items.slice().sort((p, q) => Math.round(p.y) - Math.round(q.y) || p.x - q.x);
      for (const it of order) {
        if (it.alpha <= 0.02 || it.x > W + 4 || it.x + it.s + it.d < -4) continue;
        w.globalAlpha = it.alpha;
        const lv = this.levelOf(it.v), sz = Math.round(it.s), dd = Math.round(it.d);
        const ring = it.k === found ? '#f3c75a' : it.k === cmpA && cut < 1 ? '#d9467c' : null;
        P.cube(w, it.x, it.y, sz, sz, P.VCOL[lv], dd, ring);
        const fit = Math.floor((sz - 1) / 4);
        if (sz >= 8 && fit >= 1) P.textCenter(w, String(it.v).slice(0, fit), Math.round(it.x) + sz / 2, Math.round(it.y) + Math.floor((sz - 5) / 2), P.INK);
        w.globalAlpha = 1;
      }

      // garis potong bersinar di batas antara yang tersisa dan yang dibuang (posisi grid lama)
      const glow = P.clamp((t - 0.45) / 0.08, 0, 1) * (1 - zoom);
      if (glow > 0 && s.alive.length < s.aliveBefore.length && s.alive.length) {
        const gone = r => r >= 0 && r < items.length && !items[r].alive;
        const p = A.p, cols = A.cols;
        w.globalAlpha = glow * (0.75 + 0.25 * Math.sin(now / 60));
        for (const it of items) {
          if (!it.alive) continue;
          const c = it.r % cols, x = it.cell.x, y = it.cell.y - A.d;
          if (c < cols - 1 && gone(it.r + 1)) P.rect(w, x + p - 1, y - 1, 2, p + 1, '#f3b13a');
          if (c > 0 && gone(it.r - 1)) P.rect(w, x - 2, y - 1, 2, p + 1, '#f3b13a');
          if (gone(it.r + cols)) P.rect(w, x - 1, y + p - 1, p + 1, 2, '#f3b13a');
          if (gone(it.r - cols)) P.rect(w, x - 1, y - 2, p + 1, 2, '#f3b13a');
        }
        w.globalAlpha = 1;
      }

      // Gopher naik ke kubus yang ketemu
      const fi = found >= 0 ? items.find(it => it.k === found) : null;
      if (fi) {
        const fresh = s.foundBefore !== found;
        const drop = fresh ? P.clamp((t - 0.5) / 0.4, 0, 1) : 1;
        const sz = Math.round(fi.s), dd = Math.round(fi.d);
        const gy = Math.round(fi.y) - Math.ceil(dd / 2) - 1 - Math.round((1 - P.bounce(drop)) * 60) - (drop >= 1 ? Math.round(Math.abs(Math.sin(now / 150)) * 3) : 0);
        if (drop > 0) {
          const head = P.drawGopher(w, Math.round(fi.x) + (sz + dd) / 2, gy, 'happy', 0, P.clamp(drop * 4, 0, 1));
          if (drop >= 1) this.star(w, Math.round(fi.x) + (sz + dd) / 2, head - 9 - (Math.floor(now / 250) % 2));
        }
      }

      // balon "nilai ? target" di atas kubus yang dicek (di bawahnya bila tidak muat)
      if (ci && cut < 0.6) {
        const sym = t < 0.45 ? '?' : s.cmp.result;
        const colS = t < 0.45 ? P.INK : s.cmp.result === '=' ? '#2f8a5c' : '#d9467c';
        const va = String(ci.v).slice(0, 7), vb = String(s.cmp.value != null ? s.cmp.value : s.target != null ? s.target : '').slice(0, 7);
        const bw = P.textW(va) + P.textW(sym) + P.textW(vb) + 14;
        const cx = ci.x + (ci.s + ci.d) / 2;
        const bx = Math.round(P.clamp(cx - bw / 2, 2, W - bw - 2));
        let by = Math.round(ci.y - ci.d - 15);
        if (by < 18) by = Math.round(ci.y + ci.s + 3);
        w.globalAlpha = 1 - P.clamp((cut - 0.3) / 0.3, 0, 1);
        P.box(w, bx, by, bw, 11, '#ffffff', P.INK);
        let tx = bx + 4;
        P.text(w, va, tx, by + 3, P.INK); tx += P.textW(va) + 3;
        P.text(w, sym, tx, by + 3, colS); tx += P.textW(sym) + 3;
        P.text(w, vb, tx, by + 3, '#c2457a');
        w.globalAlpha = 1;
      }
      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), p.s, p.s, p.col);

      // HUD: target, jumlah cek, sisa kandidat, penunjuk (lo/mid/hi, i)
      P.rect(w, 0, 0, W, 17, 'rgba(255,248,251,0.8)');
      P.text(w, s.target != null ? `CARI ${String(s.target).slice(0, 10)}` : 'CARI', 4, 3, P.INK);
      let hud = `CEK ${s.checks}  SISA ${s.alive.length}`;
      for (const q of s.ptr.slice(0, 4)) hud += `  ${q.name.slice(0, 4)} ${q.k}`;
      P.text(w, hud, 4, 10, '#c2457a');
      let label = null, col = '#2f8a5c';
      if (st.reason === 'panic') { label = 'PANIC!'; col = '#d9467c'; }
      else if (done) { label = !this.eqSearch ? 'SELESAI!' : s.found >= 0 ? 'KETEMU!' : 'TIDAK ADA'; if (this.eqSearch && s.found < 0) col = '#d9467c'; }
      else if (s.cmp && s.cmp.result === '=' && keep.size === 1 && t > 0.6) label = 'KETEMU!';
      if (label) {
        const img = P.stamp(label, col);
        w.drawImage(img, Math.round(W - img.width * 2 - 4), 2, img.width * 2, img.height * 2);
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
      P.present(this.ctx, this.cv, this.world, this.worldW, this.worldH);
    }
  }

  G.DataStage = DataStage;
})(window.GV = window.GV || {});
