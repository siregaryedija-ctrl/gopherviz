// Renderer mode struktur: satu loop digambar sebagai level platformer.
//   MULAI/INIT â†’ gerbang kondisi â†’ satu blok per bagian body â†’ POST â†’ kembali ke gerbang.
// Body kosong digambar sebagai satu blok bata kosong. Bagian yang bukan statement biasa
// (break, continue, return, loop dalam) berupa benda di tanah: pipa, pegas, pintu.
// Mode trace: di bawah tanah ada tembok penghitung, satu balok setiap putaran selesai.
(function (G) {
  'use strict';
  const P = G.pixel;

  const VIEW = 192;        // lebar pandang kamera (piksel dunia)
  const H = 96;            // tinggi dunia
  const GY = 78;           // garis tanah
  const SEG = 26;          // jarak antar bagian level
  const BLOCK_Y = 34;      // tepi atas blok di udara
  const WALK = 48;         // kecepatan jalan, piksel per detik
  const ITER = 3;          // contoh jumlah putaran di mode struktur
  const GOPHER_V = 2;      // tinggi gopher (pakai warna biru gopher klasik)
  const AIR = new Set(['stmt', 'if', 'block', 'bad', 'empty']);

  const BLOCK = {
    stmt: { fill: '#f3c75a', line: '#a8741f' },
    if: { fill: '#86d6cb', line: '#2f8a7a' },
    block: { fill: '#e9a28d', line: '#a4594a' },
    empty: { fill: '#ead0c4', line: '#b98a78' },
    bad: { fill: '#f6eef2', line: '#b07a92' },
  };
  const GROUND = {
    break: { fill: '#7cc68b', hi: '#b5e6bf', line: '#2f6b3d', label: 'BREAK' },
    continue: { fill: '#f7b779', hi: '#fbd7b0', line: '#a5622a', label: 'CONTINUE' },
    return: { fill: '#f39a8a', hi: '#f8c3b9', line: '#9a3f33', label: 'RETURN' },
    goto: { fill: '#c9b3d8', hi: '#e2d6eb', line: '#6b5280', label: 'GOTO' },
    loop: { fill: '#b8a2ee', hi: '#d9cdf7', line: '#5a43a0', label: '' },
  };
  const GROUND_SOUND = { break: 'pipe', continue: 'boing', return: 'door', goto: 'warp', loop: 'warp' };
  const HIT_SOUND = { stmt: 'coin', if: 'blip', block: 'blip', empty: 'tok', bad: 'tok' };

  class Level {
    /**
     * @param {HTMLCanvasElement} canvas
     * @param {{onPart(i: number|null): void, onPhase(p: string|null): void, sound(name: string): void}} hooks
     */
    constructor(canvas, hooks) {
      this.cv = canvas;
      this.ctx = canvas.getContext('2d');
      this.world = document.createElement('canvas');
      this.world.height = H;
      this.wx = this.world.getContext('2d');
      this.bg = document.createElement('canvas');
      this.bg.height = H;
      this.hooks = hooks;
      this.loop = null;
      this.items = [];
      this.worldW = 0;
      this.worldH = H;
      this.wall = null; // tata letak tembok putaran (mode trace)
      this.wallNow = null; // { labels, count, pop, ghost } untuk langkah yang sedang digambar
      this.speed = 1;
      this.camX = 0;
      this.parts = [];
      this.gateOpen = 0;
      this.gateTarget = 0;
      this.clock = 0; // satu-satunya sumber waktu (ms), dimajukan oleh update()
      this.mode = 'struct'; // 'struct' = contoh alur; 'trace' = nilai asli lewat renderTrace()
      this.reset();
      this.resize(VIEW);
    }

    reset() {
      const start = this.items.find(i => i.type === 'start');
      this.gx = start ? start.x : VIEW / 2;
      this.gy = 0;
      this.alpha = 1;
      this.face = 'idle';
      this.iter = 1;
      this.beat = null;
      this.walking = false;
      this.gateRed = false;
      this.gateResult = null;
      this.gateTarget = 0;
      this.flagT = 0;
    }

    // ---------- membangun level dari Scene ----------

    setLoop(loop) {
      const prev = this.loop;
      const same = !!(prev && loop && prev.id === loop.id && prev.kind === loop.kind);
      const prevCount = prev ? prev.body.length : 0;
      this.loop = loop;
      this.waiting = !!loop && loop.state === 'incomplete';
      this.build();
      const now = this.clock;
      if (!same) {
        this.reset();
        this.nodes().forEach((it, k) => { it.pop = now + 120 + k * 90; });
        if (loop) this.hooks.sound('appear');
        this.hooks.onPart(null);
        this.hooks.onPhase(null);
        return;
      }
      let added = false;
      this.nodes().forEach((it, k) => {
        if (loop.body.length > prevCount && k >= prevCount) { it.pop = now; added = true; }
      });
      if (added) this.hooks.sound('appear');
      if (this.waiting) {
        this.beat = null;
      } else if (this.beat) {
        this.beat = this.retarget(this.beat);
      }
    }

    build() {
      const loop = this.loop;
      this.items = [];
      if (!loop) {
        this.resize(VIEW);
        return;
      }
      const ranged = loop.kind.startsWith('range');
      const items = [];
      let x = 0;
      const add = it => { it.x = x; items.push(it); x += SEG; };
      add({ type: 'start', label: loop.kind === 'for3' ? 'INIT' : ranged ? 'RANGE' : 'MULAI' });
      if (loop.kind !== 'infinite') add({ type: 'gate', label: ranged ? 'LAGI?' : 'COND' });
      if (loop.body.length === 0) {
        add({ type: 'node', idx: 0, kind: 'empty', air: true, label: 'KOSONG' });
      } else {
        loop.body.forEach((n, i) => add({ type: 'node', idx: i, kind: n.kind, ref: n.ref, air: AIR.has(n.kind), label: String(i + 1) }));
      }
      if (loop.kind === 'for3' && loop.header.post) add({ type: 'post', label: 'POST' });
      x += 6;
      add({ type: 'finish', label: 'KELUAR' });
      // Level pendek diletakkan di tengah pandangan.
      const content = x - SEG + 24;
      const shift = Math.max(16, Math.round((VIEW - content) / 2) + 12);
      items.forEach(it => { it.x += shift; });
      this.items = items;
      this.resize(Math.max(VIEW, items[items.length - 1].x + 24));
    }

    nodes() { return this.items.filter(i => i.type === 'node'); }
    find(type) { return this.items.find(i => i.type === type); }

    resize(w) {
      if (w === this.worldW) return;
      this.worldW = w;
      this.world.width = w;
      this.bg.width = w;
      this.paintBg();
    }

    /** Tinggi dunia: H, ditambah ruang tembok putaran di mode trace. */
    setHeight(h) {
      if (h === this.worldH) return;
      this.worldH = h;
      this.world.height = h;
      this.bg.height = h;
      this.paintBg();
    }

    // ---------- alur animasi ----------

    makeBeat(name, idx) {
      idx = idx || 0;
      let item = null, dur = 450, walk = true;
      switch (name) {
        case 'init': item = this.find('start'); dur = 600; break;
        case 'gate': item = this.find('gate'); dur = 500; break;
        case 'leap': item = this.find('gate'); dur = 1100; break;
        case 'node': item = this.nodes()[idx]; dur = item.air ? 420 : 700; break;
        case 'post': item = this.find('post'); dur = 450; break;
        case 'back': walk = false; dur = 700; break;
        case 'finish': item = this.find('finish'); dur = 1700; break;
        case 'rest': walk = false; dur = 900; break;
      }
      // Benda di tanah (pipa, pegas, pintu) didatangi dari samping supaya tidak tertutup gopher.
      const x = !item ? this.gx : item.type === 'node' && !item.air ? item.x - 13 : item.x;
      return { name, idx, item, x, dur, phase: walk ? 'walk' : 'act', t: 0, fired: false };
    }

    /** Menyesuaikan langkah yang sedang berjalan setelah level dibangun ulang. */
    retarget(b) {
      if (b.name === 'node' && b.idx >= this.nodes().length) return this.makeBeat(this.find('post') ? 'post' : 'back');
      if (b.name === 'post' && !this.find('post')) return this.makeBeat('back');
      if ((b.name === 'gate' || b.name === 'leap') && !this.find('gate')) return this.makeBeat('node', 0);
      const nb = this.makeBeat(b.name, b.idx);
      nb.phase = b.phase === 'act' && nb.phase === 'walk' && Math.abs(nb.x - this.gx) > 1 ? 'walk' : b.phase;
      nb.t = b.t;
      nb.fired = b.fired;
      return nb;
    }

    advance() {
      const b = this.beat;
      const n = this.nodes().length;
      const hasGate = !!this.find('gate');
      const first = () => (hasGate ? this.makeBeat('gate') : this.makeBeat('node', 0));
      let next;
      switch (b ? b.name : null) {
        case null: next = this.makeBeat('init'); break;
        case 'init': next = first(); break;
        case 'gate': next = this.makeBeat('node', 0); break;
        case 'node': next = b.idx + 1 < n ? this.makeBeat('node', b.idx + 1) : this.makeBeat(this.find('post') ? 'post' : 'back'); break;
        case 'post': next = this.makeBeat('back'); break;
        case 'back':
          this.iter++;
          next = hasGate && this.iter > ITER ? this.makeBeat('leap') : first();
          break;
        case 'leap': next = this.makeBeat('finish'); break;
        case 'finish': next = this.makeBeat('rest'); break;
        default: this.reset(); next = this.makeBeat('init'); break;
      }
      this.beat = next;
      if (next.phase === 'act') this.startAct(next);
    }

    startAct(b) {
      const h = this.hooks;
      switch (b.name) {
        case 'init':
          h.onPhase('init'); h.onPart(null); h.sound('blip');
          break;
        case 'gate':
          this.gateResult = true; this.gateTarget = 1;
          h.onPhase('cond'); h.onPart(null); h.sound('gate');
          break;
        case 'leap':
          this.gateResult = false; this.gateRed = true;
          h.onPhase('cond'); h.onPart(null); h.sound('gateNo');
          break;
        case 'node':
          h.onPhase(null); h.onPart(this.loop.body.length ? b.idx : 0);
          if (b.item.air) h.sound('jump');
          else { b.item.pulse = this.clock; h.sound(GROUND_SOUND[b.item.kind] || 'blip'); }
          break;
        case 'post':
          b.item.pulse = this.clock;
          h.onPhase('post'); h.onPart(null); h.sound('tickLow');
          break;
        case 'back':
          this.gateTarget = 0; this.gateResult = null;
          h.onPhase(null); h.onPart(null); h.sound('warp');
          break;
        case 'finish':
          this.face = 'happy';
          h.onPhase('exit'); h.onPart(null); h.sound('jingle');
          this.confetti(b.item.x);
          break;
        case 'rest':
          h.onPhase(null);
          break;
      }
    }

    act(b, t, now) {
      switch (b.name) {
        case 'node':
          if (!b.item.air) break;
          this.gy = -Math.round(Math.sin(Math.PI * t) * 12);
          if (!b.fired && t >= 0.45) {
            b.fired = true;
            b.item.bump = now;
            this.hooks.sound(HIT_SOUND[b.item.kind] || 'blip');
            this.hitFx(b.item);
          }
          break;
        case 'leap': {
          const gate = this.find('gate'), fin = this.find('finish');
          if (t < 0.3) break;
          const u = (t - 0.3) / 0.7;
          this.gx = P.lerp(gate.x, fin.x - 14, P.ease(u));
          this.gy = -Math.round(Math.sin(Math.PI * u) * 26);
          break;
        }
        case 'back': {
          if (t < 0.5) { this.alpha = 1 - t * 2; break; }
          if (!b.fired) {
            b.fired = true;
            const target = this.find('gate') || this.nodes()[0];
            this.gx = target.x - 12;
          }
          this.alpha = (t - 0.5) * 2;
          break;
        }
        case 'finish':
          this.flagT = t;
          break;
        case 'rest':
          this.alpha = 1 - t;
          break;
      }
    }

    endAct(b) {
      this.gy = 0;
      this.alpha = 1;
      if (b.name !== 'finish') this.face = 'idle';
      if (b.name === 'rest') { this.flagT = 0; this.gateRed = false; }
    }

    update(dt) {
      this.clock += dt;
      const now = this.clock;
      this.updateParticles(dt);
      if (this.mode === 'trace') return; // posisi diatur renderTrace() oleh pemutar langkah
      this.gateOpen += P.clamp(this.gateTarget - this.gateOpen, -dt / 220, dt / 220);
      const b0 = this.beat;
      this.arrowActive = !!(b0 && b0.name === 'back');
      this.activeItem = b0 && b0.phase === 'act' ? b0.item : null;
      this.hud = null;
      if (!this.loop || this.waiting) {
        this.walking = false;
        this.gy = 0;
        this.alpha = 1;
        this.face = this.waiting ? 'think' : (now % 3400 < 130 ? 'blink' : 'idle');
        if (this.waiting) {
          const s = this.find('start');
          if (s) this.gx = s.x;
        }
        return;
      }
      if (!this.beat) this.advance();
      const b = this.beat;
      if (b.phase === 'walk') {
        const dx = b.x - this.gx, step = (WALK * this.speed * dt) / 1000;
        if (Math.abs(dx) <= step) {
          this.gx = b.x;
          b.phase = 'act';
          this.walking = false;
          this.startAct(b);
        } else {
          this.gx += Math.sign(dx) * step;
          this.walking = true;
        }
      }
      if (b.phase === 'act') {
        b.t += (dt * this.speed) / b.dur;
        this.act(b, Math.min(1, b.t), now);
        if (b.t >= 1) {
          this.endAct(b);
          this.advance();
        }
      }
      if (this.face !== 'happy' && !this.walking && now % 3400 < 130) this.face = 'blink';
      else if (this.face === 'blink') this.face = 'idle';
    }

    // ---------- mode trace: nilai asli dari simulator ----------

    /** Mengubah langkah trace menjadi aksi di level. Langkah loop anak yang berurutan digabung jadi satu. */
    prepareTrace(trace) {
      this.mode = 'trace';
      this.trace = trace;
      const focus = this.loop.id;
      const nodes = this.nodes();
      const start = this.find('start'), gate = this.find('gate'), post = this.find('post'), fin = this.find('finish');
      const partOf = line => {
        let best = -1;
        this.loop.body.forEach((n, i) => { if (line >= n.line && line <= (n.endLine || n.line)) best = i; });
        return best;
      };
      const view = [];
      for (const s of trace.steps) {
        if (s.loop !== focus && s.reason !== 'panic') {
          const idx = partOf(s.line);
          const prev = view[view.length - 1];
          if (prev && prev.act === 'child' && prev.idx === idx) { prev.group.push(s); prev.step = s; continue; }
          view.push({ step: s, act: 'child', idx, item: nodes[idx] || gate || start, group: [s] });
          continue;
        }
        let act = 'hit', item = gate || start, idx = -1;
        switch (s.phase) {
          case 'init': act = 'init'; item = start; break;
          case 'cond': act = s.result ? 'open' : 'closed'; item = gate || start; break;
          case 'post': act = 'post'; item = post || gate || start; break;
          case 'exit':
            item = fin;
            act = s.reason === 'panic' ? 'panic' : s.reason === 'cond-false' || s.reason === 'range-done' ? 'leap' : 'warp-exit';
            if (act === 'panic') item = view.length ? view[view.length - 1].item : start;
            break;
          default:
            idx = partOf(s.line);
            item = nodes[idx] || gate || start;
            act = item.type === 'node' && !item.air ? 'visit' : s.result != null ? 'if' : 'hit';
        }
        view.push({ step: s, act, item, idx });
      }
      // posisi tujuan, perpindahan mundur (putaran baru = warp), dan keadaan gerbang per langkah
      let px = start.x, gateState = { open: 0, red: false, sign: '?' }, iter = 0;
      for (const v of view) {
        v.x = v.item.type === 'node' && !v.item.air ? v.item.x - 13 : v.item.x;
        v.fromX = px;
        v.warp = v.act === 'warp-exit' || (v.x < px - 2 && v.act !== 'leap');
        if (v.warp) gateState = { open: 0, red: false, sign: '?' };
        v.gateFrom = gateState;
        if (v.act === 'open') gateState = { open: 1, red: false, sign: 'T' };
        else if (v.act === 'closed' || v.act === 'leap') gateState = { open: 0, red: true, sign: 'F' };
        v.gateTo = gateState;
        if (v.step.loop === focus && v.step.iteration != null) iter = v.step.iteration;
        v.iter = iter;
        px = v.act === 'panic' ? px : v.x;
      }
      // "closed" langsung disusul "leap": gabungkan supaya gerbang merah lalu gopher melompat
      this.view = view.filter((v, i) => !(v.act === 'closed' && view[i + 1] && view[i + 1].act === 'leap'));
      this.buildWall(trace);
      return this.view;
    }

    /**
     * Tembok putaran: satu balok untuk setiap putaran yang sudah selesai, dihitung ulang tiap kali loop
     * ini mulai lagi (loop dalam yang dijalankan loop luar). Label balok = nilai variabel loop pertama
     * di putaran itu bila semuanya pendek, selain itu nomor putaran.
     */
    buildWall(trace) {
      const focus = this.loop.id, name = (trace.loop.vars || [])[0];
      const runs = [];
      let cur = null, done = 0;
      this.view.forEach((v, i) => {
        const s = v.step;
        if (s.loop === focus) {
          if (!cur || s.run !== cur.run) { cur = { run: s.run, vals: [] }; runs.push(cur); done = 0; }
          const it = s.iteration || 0;
          const enters = s.phase === 'cond' ? !!s.result : s.phase !== 'init' && s.phase !== 'post' && s.phase !== 'exit';
          if (enters && cur.vals.length === it) cur.vals.push(name && s.vars ? s.vars[name] : null);
          const last = i === this.view.length - 1;
          done = s.phase === 'exit' || (last && trace.truncated) ? cur.vals.length : Math.max(done, Math.min(cur.vals.length, it));
        }
        v.wallRun = cur;
        v.wallPop = i > 0 && cur && this.view[i - 1].wallRun === cur && done > this.view[i - 1].wallCount;
        v.wallCount = cur ? done : 0;
      });
      const all = runs.flatMap(r => r.vals.map(P.wallLabel));
      const useVals = all.length > 0 && all.every(l => l != null);
      for (const r of runs) r.labels = r.vals.map((x, k) => (useVals ? P.wallLabel(x) : String(k + 1)));
      const n = Math.max(0, ...runs.map(r => r.vals.length));
      if (!n) { this.wall = null; this.setHeight(H); return; }
      this.wall = P.wallLayout(n, runs.flatMap(r => r.labels), this.worldW, !!trace.truncated);
      this.setHeight(H + this.wall.h);
    }

    traceDuration(i) {
      const v = this.view[i], last = i === this.view.length - 1;
      switch (v.act) {
        case 'init': return 650;
        case 'open': return 650;
        case 'closed': return 900;
        case 'post': return 500;
        case 'visit': return 850;
        case 'child': return 1000;
        case 'if': return 850;
        case 'leap': case 'warp-exit': return last ? 2400 : 1400;
        case 'panic': return 2200;
      }
      return 750;
    }

    traceCues(i) {
      const v = this.view[i], st = v.step, out = [];
      const last = i === this.view.length - 1;
      const sound = {
        init: 'blip', open: 'gate', closed: 'gateNo', post: 'tickLow', child: 'warp', panic: 'panic',
        visit: GROUND_SOUND[v.item.kind] || 'blip', leap: 'gateNo', 'warp-exit': 'warp',
      }[v.act];
      if (sound) out.push({ t: 0, sound });
      if (v.act === 'hit' || v.act === 'if') {
        out.push({ t: 0, sound: 'jump' });
        const hit = v.act === 'if' ? (st.result ? 'yes' : 'no') : HIT_SOUND[v.item.kind] || 'blip';
        out.push({ t: 0.5, sound: hit, fx: () => { if (v.item.type === 'node') this.hitFx(v.item); } });
      }
      const outs = v.group ? v.group.map(s => s.output || '').join('') : st.output;
      if (outs) out.push({ t: 0.6, sound: 'coin', fx: () => this.burst(this.gx, GY - 26, 6, ['#f3c75a', '#fff3b8'], { sp: 30, up: 60, g: 140, life: 500, s: 2 }) });
      if (v.wallPop) out.push({ t: 0.75, sound: 'tok' });
      if (last && (v.act === 'leap' || v.act === 'warp-exit')) out.push({ t: 0.7, sound: 'jingle', fx: () => this.confetti(this.find('finish').x) });
      return out;
    }

    /** Menggambar langkah i pada waktu t (0..1). Tanpa state: maju/mundur cukup memanggil ulang. */
    renderTrace(i, t) {
      const v = this.view[i];
      if (!v) return;
      const now = this.clock;
      let gx = v.x, gy = 0, alpha = 1, a = 1, walking = false;
      if (v.act === 'leap') {
        const gate = this.find('gate') || this.find('start');
        if (t < 0.3) { gx = v.fromX; a = 0; } else {
          const u = (t - 0.3) / 0.7;
          gx = P.lerp(gate.x, v.x, P.ease(u));
          gy = -Math.sin(Math.PI * u) * 26;
          a = u;
        }
      } else if (v.warp) {
        if (t < 0.3) { gx = v.fromX; alpha = 1 - t / 0.3; a = 0; }
        else if (t < 0.5) { alpha = (t - 0.3) / 0.2; a = 0; }
        else a = (t - 0.5) / 0.5;
      } else {
        const dist = Math.abs(v.x - v.fromX);
        const wf = dist > 1 ? Math.min(0.5, dist / 80) : 0;
        if (t < wf) { gx = P.lerp(v.fromX, v.x, t / wf); walking = true; a = 0; } else a = wf < 1 ? (t - wf) / (1 - wf) : 1;
      }
      let face = now % 3400 < 130 ? 'blink' : 'idle';
      if (v.act === 'hit' || v.act === 'if') {
        gy = -Math.round(Math.sin(Math.PI * a) * 12);
        if (a >= 0.45 && v.item.type === 'node') v.item.bump = now - (a - 0.45) * 400;
      }
      if ((v.act === 'visit' || v.act === 'child' || v.act === 'post') && a > 0) v.item.pulse = now - a * 300;
      if (v.act === 'panic') face = 'shock';
      if ((v.act === 'leap' || v.act === 'warp-exit') && i === this.view.length - 1 && a > 0.5) face = 'happy';
      if (v.act === 'if' && a > 0.5) face = v.step.result ? 'happy' : 'idle';

      // gerbang
      const g0 = v.gateFrom, g1 = v.gateTo;
      const ga = v.act === 'open' || v.act === 'closed' || v.act === 'leap' ? P.clamp(a * 2, 0, 1) : 1;
      this.gateOpen = P.lerp(g0.open, g1.open, ga);
      this.gateRed = ga >= 0.5 ? g1.red : g0.red;
      const sign = ga >= 0.5 ? g1.sign : g0.sign;
      this.gateResult = sign === 'T' ? true : null;
      this.flagT = (v.act === 'leap' || v.act === 'warp-exit') ? P.clamp((a - 0.3) / 0.7, 0, 1) : 0;

      this.gx = gx; this.gy = gy; this.alpha = alpha; this.face = face; this.walking = walking;
      this.arrowActive = v.warp && v.act !== 'warp-exit' && t < 0.5;
      this.activeItem = a > 0 ? v.item : null;
      const vars = (this.trace.loop.vars || []).map(n => v.step.vars && v.step.vars[n] != null ? `${n}=${v.step.vars[n]}` : null).filter(Boolean);
      this.hud = { left: v.act === 'panic' ? 'PANIC!' : `PUTARAN ${v.iter + 1}`, vars: vars.join(' ') };
      if (v.act === 'child') this.hud.vars = `${this.hud.vars}  ${v.item.ref || ''} X${v.group.length}`.trim();
      // balok putaran yang baru selesai jatuh ke tembok saat gopher kembali ke awal putaran
      const run = v.wallRun;
      this.wallNow = run && {
        labels: run.labels, count: v.wallCount, pop: v.wallPop ? P.clamp((t - 0.45) / 0.5, 0, 1) : null,
        ghost: !!this.trace.truncated && i === this.view.length - 1,
      };
      this.render();
      if (v.act === 'panic' && a > 0.2) { // stempel di atas layar, ukuran 2x piksel dunia
        const img = P.stamp('PANIC!', '#d9467c'), S = this.cv.__k || 4, k = 2 * S;
        this.ctx.drawImage(img, Math.round(this.cv.width / 2 - (img.width * k) / 2), Math.round(26 * S), img.width * k, img.height * k);
      }
    }

    /** Kembali ke mode struktur (mis. kode berubah dan trace lama tidak berlaku lagi). */
    stopTrace() {
      this.mode = 'struct';
      this.view = null;
      this.wall = null;
      this.wallNow = null;
      this.setHeight(H);
      this.reset();
    }

    // ---------- partikel ----------

    burst(x, y, n, cols, o) {
      for (let k = 0; k < n; k++) {
        this.parts.push({
          x: x + (Math.random() - 0.5) * (o.spread || 0), y,
          vx: (Math.random() - 0.5) * o.sp, vy: -Math.random() * o.up,
          g: o.g, life: o.life * (0.6 + Math.random() * 0.6), col: cols[k % cols.length], s: o.s || 1,
        });
      }
    }
    hitFx(item) {
      const x = item.x, y = BLOCK_Y - 2;
      if (item.kind === 'stmt') this.burst(x, y, 6, ['#f3c75a', '#fff3b8'], { sp: 30, up: 60, g: 140, life: 500, s: 2 });
      else if (item.kind === 'if') this.burst(x, y, 8, ['#86d6cb', '#ffffff'], { sp: 60, up: 40, g: 60, life: 500 });
      else this.burst(x, BLOCK_Y + 15, 5, ['#d8b4a6', '#ffffff'], { sp: 40, up: 10, g: 80, life: 350 });
    }
    confetti(x) {
      const cols = ['#f5a3bf', '#f7b779', '#f1d064', '#9fd88f', '#8ec4f0', '#b8a2ee', '#ffffff'];
      this.burst(x, GY - 34, 40, cols, { sp: 120, up: 90, g: 90, life: 1600, spread: 10 });
    }
    updateParticles(dt) {
      const f = dt / 1000;
      this.parts = this.parts.filter(p => {
        p.vy += p.g * f; p.x += p.vx * f; p.y += p.vy * f; p.life -= dt;
        return p.life > 0 && p.y < this.worldH;
      });
    }

    // ---------- gambar ----------

    paintBg() {
      P.paintWorld(this.bg.getContext('2d'), this.worldW, this.worldH, GY);
    }

    render() {
      const now = this.clock;
      const w = this.wx;
      w.globalAlpha = 1;
      w.drawImage(this.bg, 0, 0);
      if (this.loop) {
        w.globalAlpha = this.waiting ? 0.45 : 1;
        this.drawArrow(w, now);
        for (const it of this.items) this.drawItem(w, it, now);
        w.globalAlpha = 1;
        if (this.waiting) {
          for (const it of this.nodes()) P.dashedBox(w, it.x - 8, it.air ? BLOCK_Y - 1 : GY - 15, 16, it.air ? 16 : 15, '#b07a92');
        }
      }
      if (this.wall && this.wallNow) {
        const wn = this.wallNow;
        P.drawWall(w, this.wall, H - 4, wn.labels, wn.count, wn.pop, wn.ghost);
      }
      this.drawGopher(w, now);
      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), p.s, p.s, p.col);

      // seluruh dunia terlihat (tanpa kamera)
      this.drawHud(w, 0);
      P.present(this.ctx, this.cv, this.world, this.worldW, this.worldH);
    }

    drawHud(w, cx) {
      if (!this.loop) return;
      if (this.hud) {
        P.text(w, this.hud.left, cx + 4, 3, P.INK);
        if (this.hud.vars) P.text(w, this.hud.vars, cx + 4, 10, '#c2457a');
      } else if (this.waiting) {
        P.text(w, 'BELUM LENGKAP', cx + 4, 3, '#c2457a');
      } else {
        const it = this.loop.kind === 'infinite' ? 'PUTARAN ' + this.iter : 'PUTARAN ' + Math.min(this.iter, ITER);
        P.text(w, it, cx + 4, 3, P.INK);
      }
      P.text(w, this.loop.id, this.worldW - 4 - P.textW(this.loop.id), 3, P.INK);
    }

    drawArrow(w, now) {
      const from = this.find('post') || this.nodes()[this.nodes().length - 1];
      const to = this.find('gate') || this.nodes()[0];
      if (!from || !to) return;
      const active = this.arrowActive;
      const col = active ? '#d9467c' : '#e6b3c6';
      const y = 18, x0 = to.x, x1 = from.x;
      const off = active ? Math.floor(now / 60) % 4 : 0;
      w.fillStyle = col;
      for (let x = x0 + 3 + off; x <= x1; x += 4) w.fillRect(x, y, 2, 1);
      w.fillRect(x1, y, 1, 6);
      w.fillRect(x0 + 1, y - 1, 1, 3); w.fillRect(x0 + 2, y - 2, 1, 5);
      w.fillRect(x0, y, 1, 1);
    }

    labelFor(w, it, now) {
      const hl = this.activeItem === it;
      P.textCenter(w, it.label, it.x, GY + 6, hl ? '#c2457a' : '#c58ba3');
    }

    drawItem(w, it, now) {
      switch (it.type) {
        case 'start': {
          const wave = this.beat && this.beat.name === 'init' && Math.floor(now / 120) % 2;
          P.rect(w, it.x - 6, GY - 18, 1, 18, P.INK);
          w.fillStyle = '#f5a3bf';
          for (let r = 0; r < 5; r++) w.fillRect(it.x - 5, GY - 18 + r + (wave ? 1 : 0), 6 - Math.abs(2 - r) * 2, 1);
          break;
        }
        case 'gate': this.drawGate(w, it); break;
        case 'node':
          if (it.pop && now < it.pop) return; // menunggu giliran muncul
          if (it.air) this.drawBlock(w, it, now); else this.drawGround(w, it, now);
          break;
        case 'post': {
          const p = it.pulse && now - it.pulse < 400;
          P.rect(w, it.x, GY - 14, 1, 14, P.INK);
          P.disc(w, it.x, GY - 19, 6, P.INK);
          P.disc(w, it.x, GY - 19, 5, p ? '#f6cf5a' : '#e6dcff');
          P.text(w, '+', it.x - 1, GY - 21, P.INK);
          break;
        }
        case 'finish': {
          P.rect(w, it.x, GY - 34, 1, 34, P.INK);
          P.disc(w, it.x, GY - 35, 1, '#f3c75a');
          const fy = Math.round(P.lerp(GY - 9, GY - 32, this.flagT));
          w.fillStyle = '#d9467c';
          for (let r = 0; r < 7; r++) w.fillRect(it.x + 1, fy + r, 8 - Math.abs(3 - r) * 2, 1);
          break;
        }
      }
      this.labelFor(w, it, now);
    }

    drawGate(w, it) {
      const top = GY - 26, x = it.x;
      P.rect(w, x - 10, top, 4, 26, P.INK); P.rect(w, x - 9, top + 1, 2, 25, '#e6dcff');
      P.rect(w, x + 6, top, 4, 26, P.INK); P.rect(w, x + 7, top + 1, 2, 25, '#e6dcff');
      P.rect(w, x - 11, top - 3, 22, 4, P.INK); P.rect(w, x - 10, top - 2, 20, 2, '#b8a2ee');
      const doorH = Math.round(23 * (1 - this.gateOpen));
      if (doorH > 0) {
        const col = this.gateRed ? '#f39a8a' : '#c9b3d8';
        P.rect(w, x - 6, top + 1, 12, doorH, col);
        w.fillStyle = P.mix(col, P.INK, 0.3);
        for (let y = top + 3; y < top + 1 + doorH; y += 4) w.fillRect(x - 6, y, 12, 1);
      }
      // papan hasil kondisi: ? (belum dicek), T (true), F (false)
      const sign = this.gateRed ? 'F' : this.gateResult ? 'T' : '?';
      const col = this.gateRed ? '#c2457a' : this.gateResult ? '#2f8a5c' : P.INK;
      P.box(w, x - 5, top - 14, 11, 10, '#ffffff', P.INK);
      P.text(w, sign, x - 1, top - 12, col);
    }

    drawBlock(w, it, now) {
      const st = BLOCK[it.kind] || BLOCK.stmt;
      let y = BLOCK_Y;
      if (it.pop && now - it.pop < 450) y -= Math.round((1 - P.bounce((now - it.pop) / 450)) * 22);
      if (it.bump && now - it.bump < 220) y -= Math.round(Math.sin((Math.PI * (now - it.bump)) / 220) * 3);
      const x = it.x - 7;
      if (it.kind === 'bad') {
        P.rect(w, x, y, 14, 14, st.fill);
        P.dashedBox(w, x, y, 14, 14, st.line);
        P.text(w, '?', x + 6, y + 5, st.line);
        return;
      }
      P.box(w, x, y, 14, 14, st.line, P.INK);
      P.rect(w, x + 1, y + 1, 12, 12, st.fill);
      P.rect(w, x + 1, y + 1, 12, 1, P.mix(st.fill, '#ffffff', 0.5));
      if (it.kind === 'empty' || it.kind === 'block') {
        w.fillStyle = st.line; // pola bata
        w.fillRect(x + 1, y + 5, 12, 1); w.fillRect(x + 1, y + 9, 12, 1);
        w.fillRect(x + 7, y + 1, 1, 4); w.fillRect(x + 4, y + 6, 1, 3); w.fillRect(x + 10, y + 6, 1, 3); w.fillRect(x + 7, y + 10, 1, 3);
      } else {
        w.fillStyle = st.line; // paku sudut
        w.fillRect(x + 2, y + 2, 1, 1); w.fillRect(x + 11, y + 2, 1, 1); w.fillRect(x + 2, y + 11, 1, 1); w.fillRect(x + 11, y + 11, 1, 1);
      }
      if (it.kind !== 'empty') {
        const s = it.label;
        P.text(w, s, Math.round(it.x - P.textW(s) / 2) + 1, y + 5, st.line);
        P.text(w, s, Math.round(it.x - P.textW(s) / 2), y + 4, '#ffffff');
      }
    }

    drawGround(w, it, now) {
      const st = GROUND[it.kind] || GROUND.goto;
      const pulse = it.pulse && now - it.pulse < 700 && Math.floor((now - it.pulse) / 120) % 2 === 0;
      let base = GY;
      if (it.pop && now - it.pop < 450) base -= Math.round((1 - P.bounce((now - it.pop) / 450)) * 22);
      const x = it.x;
      if (it.kind === 'continue') {
        P.rect(w, x - 5, base - 2, 10, 2, st.line);
        w.fillStyle = st.fill;
        for (let r = 0; r < 3; r++) { w.fillRect(x - 4 + (r % 2) * 2, base - 4 - r * 2, 6, 1); }
        P.rect(w, x - 5, base - 10 - (pulse ? 2 : 0), 10, 2, st.line);
      } else if (it.kind === 'return') {
        P.box(w, x - 5, base - 16, 10, 16, st.fill, st.line);
        P.rect(w, x - 4, base - 15, 8, 1, st.hi);
        P.rect(w, x + 2, base - 8, 1, 1, pulse ? '#ffffff' : '#f3c75a');
      } else {
        // pipa (break, goto, loop dalam)
        P.rect(w, x - 5, base - 10, 10, 10, st.line);
        P.rect(w, x - 4, base - 10, 8, 10, st.fill);
        P.rect(w, x - 3, base - 10, 2, 10, st.hi);
        P.rect(w, x - 6, base - 14, 12, 4, st.line);
        P.rect(w, x - 5, base - 13, 10, 2, pulse ? '#ffffff' : st.fill);
      }
      const label = it.kind === 'loop' ? (it.ref || 'LOOP') : st.label;
      P.textCenter(w, label, x, base - 22, pulse ? '#d9467c' : st.line);
    }

    drawGopher(w, now) {
      const step = this.walking ? Math.floor(now / 140) % 2 : 0;
      const spr = P.gopher(GOPHER_V, this.face, step);
      const bob = this.walking && step ? -1 : 0;
      const x = Math.round(this.gx - 8), y = GY + 2 - spr.height + Math.round(this.gy) + bob;
      w.globalAlpha = P.clamp(this.alpha, 0, 1);
      w.drawImage(spr, x, y);
      w.globalAlpha = 1;
      if (this.waiting) {
        const by = y - 14 + (Math.floor(now / 300) % 2 ? -1 : 0);
        P.box(w, x + 3, by, 11, 10, '#ffffff', P.INK);
        P.text(w, '?', x + 7, by + 3, P.INK);
      }
    }
  }

  G.Level = Level;
})(window.GV = window.GV || {});
