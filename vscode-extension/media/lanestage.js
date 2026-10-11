// Mode jalur goroutine: setiap goroutine = satu Gopher di jalurnya sendiri (G1 = main).
// Hanya satu Gopher yang berjalan pada satu waktu (penjadwal simulator meniru Go dengan satu
// prosesor); yang lain menunggu ("..."), tidur ("Z"), atau sudah selesai (bendera).
// Channel digambar sebagai pipa di kanan; nilai = bola yang dilempar masuk (kirim) dan
// ditarik keluar (terima). Channel unbuffered punya satu titik serah-terima bergaris putus.
(function (G) {
  'use strict';
  const P = G.pixel;

  const TOP = 18;      // ruang HUD
  const LANE = 30;     // jarak antar jalur
  const X_TAG = 4;     // label G1, G2, ...
  const X_GOPHER = 30; // posisi gopher di jalurnya
  const X_PIPE = 112;  // awal pipa channel
  const SLOT = 14;     // lebar satu slot buffer
  const PIPE_GAP = 26; // jarak antar pipa
  // warna lencana per goroutine (berulang)
  const GCOL = ['#8ec4f0', '#f7b779', '#9fd88f', '#f5a3bf', '#b8a2ee', '#f1d064', '#86d6cb'];
  const STATE = { jalan: 'JALAN', siap: 'SIAP', menunggu: 'MENUNGGU', tidur: 'TIDUR', selesai: 'SELESAI' };

  const short = v => {
    const s = String(v).replace(/^"(.*)"$/, '$1');
    return s.length > 4 ? s.slice(0, 3) + '.' : s;
  };

  class LaneStage {
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

    laneY(k) { return TOP + 26 + k * LANE; }
    pipeY(j) { return TOP + 16 + j * PIPE_GAP; }

    /** Potret goroutine/channel per langkah; langkah sebelum `go` pertama hanya punya main. */
    prepare(trace) {
      this.trace = trace;
      let gs = [{ id: 1, name: 'main', state: 'jalan' }], chans = [];
      let maxG = 1, maxChans = 0, maxSlots = 1;
      const views = [];
      trace.steps.forEach((s, i) => {
        const prevGs = gs, prevChans = chans;
        if (s.gs) gs = s.gs;
        if (s.chans) chans = s.chans;
        const g = s.g || 1;
        const v = { step: s, g, gs, chans, prevGs, prevChans, act: 'run', fly: null };
        const me = gs.find(x => x.id === g);
        // kirim/terima channel dibawa sebagai op terstruktur: Key = ID channel, A = slot di pipa
        const comm = (s.ops || []).find(o => o.op === 'send' || o.op === 'recv');
        const j = comm ? chans.findIndex(c => c.id === comm.key) : -1;
        if (s.reason === 'panic') v.act = 'panic';
        else if (j >= 0) { v.act = comm.op; v.fly = { val: comm.value, j, slot: comm.a }; }
        else if (gs.length > prevGs.length) { v.act = 'spawn'; v.spawn = gs.length; }
        else if (me && me.state === 'tidur') v.act = 'sleep';
        else if (me && me.state === 'menunggu') v.act = 'wait';
        else if (s.phase === 'return' && i === trace.steps.length - 1) v.act = 'end';
        maxG = Math.max(maxG, gs.length);
        maxChans = Math.max(maxChans, chans.length);
        for (const c of chans) maxSlots = Math.max(maxSlots, c.cap || 1, c.items.length);
        views.push(v);
      });
      this.snaps = views;
      this.W = Math.max(192, maxChans ? X_PIPE + Math.min(maxSlots, 8) * SLOT + 18 : 120);
      this.H = Math.max(96, this.laneY(maxG - 1) + 10, maxChans ? this.pipeY(maxChans - 1) + 18 : 0);
      this.world.width = this.W; this.world.height = this.H;
      this.bg.width = this.W; this.bg.height = this.H;
      P.paintSky(this.bg.getContext('2d'), this.W, this.H);
      return views;
    }

    duration(i) {
      const v = this.snaps[i], last = i === this.snaps.length - 1;
      return ({ run: 650, spawn: 900, send: 1000, recv: 1000, wait: 900, sleep: 900, end: 1500, panic: 2000 }[v.act] || 700) + (last ? 500 : 0);
    }

    cues(i) {
      const v = this.snaps[i], s = v.step, out = [];
      const last = i === this.snaps.length - 1;
      switch (v.act) {
        case 'spawn': out.push({ t: 0.1, sound: 'appear' }); break;
        case 'send': out.push({ t: 0, sound: 'jump' }); out.push({ t: 0.6, sound: 'tok' }); break;
        case 'recv': out.push({ t: 0, sound: 'pipe' }); out.push({ t: 0.6, sound: 'blip' }); break;
        case 'wait': out.push({ t: 0.1, sound: 'think' }); break;
        case 'sleep': out.push({ t: 0.1, sound: 'tickLow' }); break;
        case 'panic': out.push({ t: 0, sound: 'panic' }); break;
        default: out.push({ t: 0, sound: 'tick' });
      }
      if (s.output) out.push({ t: 0.6, sound: 'coin' });
      if (last && v.act !== 'panic') out.push({ t: 0.5, sound: 'jingle', fx: () => this.confetti() });
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

    /** Posisi slot ke-k pada pipa ke-j (tengah bola). */
    slotPos(j, k) { return { x: X_PIPE + 8 + k * SLOT, y: this.pipeY(j) + 6 }; }

    render(i, t) {
      const v = this.snaps[i];
      if (!v) return;
      const w = this.wx, now = this.clock;
      P.drawBg(w, this.bg);
      const gIdx = id => v.gs.findIndex(g => g.id === id);
      const lane = Math.max(0, gIdx(v.g));
      const gopherAt = k => ({ x: X_GOPHER, y: this.laneY(k) });

      // ---------- jalur goroutine ----------
      v.gs.forEach((g, k) => {
        const gy = this.laneY(k);
        const running = g.id === v.g && v.act !== 'panic';
        const col = GCOL[(g.id - 1) % GCOL.length];
        const born = v.act === 'spawn' && k === v.spawn - 1;
        const alpha = born ? P.clamp(t * 2, 0, 1) : g.state === 'selesai' ? 0.45 : 1;
        // lantai jalur; jalur yang sedang jalan disorot
        P.rect(w, 2, gy, X_PIPE - 10, 2, running ? '#e86f9c' : '#e8a9bf');
        P.rect(w, 2, gy + 2, X_PIPE - 10, 2, '#f7d3dd');
        // lencana G#
        const tag = 'G' + g.id;
        P.box(w, X_TAG, gy - 13, P.textW(tag) + 4, 9, col, P.INK);
        P.text(w, tag, X_TAG + 2, gy - 11, P.INK);
        // gopher: berjalan di tempat bila sedang jalan
        let face = now % 3400 < 130 ? 'blink' : 'idle';
        if (g.state === 'menunggu') face = 'think';
        if (g.state === 'tidur') face = 'blink';
        if (v.act === 'panic') face = 'shock';
        if (running && v.act === 'recv' && t > 0.6) face = 'happy';
        const hop = running && (v.act === 'send') ? Math.round(Math.sin(Math.PI * P.clamp(t * 2, 0, 1)) * 4) : 0;
        const head = P.drawGopher(w, X_GOPHER, gy - hop, face, running ? Math.floor(now / 140) % 2 : 0, alpha);
        if (g.state === 'menunggu') P.bubble(w, X_GOPHER, head - 13 + (Math.floor(now / 300) % 2 ? -1 : 0), '.', P.INK);
        if (g.state === 'tidur') P.text(w, Math.floor(now / 400) % 2 ? 'Z' : 'Z Z', X_GOPHER + 7, head - 6, '#7a6aa8');
        if (g.state === 'selesai') {
          P.rect(w, X_GOPHER + 10, gy - 14, 1, 14, P.INK);
          P.rect(w, X_GOPHER + 11, gy - 14, 6, 4, '#7cc68b');
        }
        // status + nama fungsi goroutine
        const label = STATE[g.state] || g.state.toUpperCase();
        P.text(w, label, X_GOPHER + 12, gy - 13, running ? '#c2457a' : '#9a7f8f');
        const name = g.id === 1 ? 'MAIN' : String(g.name).replace(/\(.*$/, '()');
        P.text(w, name.slice(0, 14), X_GOPHER + 12, gy - 6, '#b48aa0');
      });

      // ---------- pipa channel ----------
      // Saat terima, bola diambil dari isi channel SEBELUM langkah ini; saat kirim, bola baru
      // baru tampil di pipa setelah mendarat.
      const flyJ = v.fly ? v.fly.j : -1;
      v.chans.forEach((c, j) => {
        const py = this.pipeY(j);
        const slots = Math.max(1, Math.min(c.cap || 1, 8));
        const pw = Math.max(slots, Math.min(c.items.length, 8)) * SLOT + 4;
        P.text(w, `${c.name || c.id}${c.cap ? ' [' + c.cap + ']' : ''}${c.closed ? ' X' : ''}`, X_PIPE, py - 7, c.closed ? '#9a7f8f' : P.INK);
        // badan pipa
        P.box(w, X_PIPE, py, pw, 12, c.closed ? '#d9cfd6' : '#bfe7c9', P.INK);
        P.rect(w, X_PIPE + 1, py + 1, pw - 2, 2, c.closed ? '#eee8ec' : '#e2f6e7');
        if (!c.cap) P.dashedBox(w, X_PIPE + 2, py + 2, SLOT - 2, 8, '#2f6b3d'); // titik serah-terima
        else for (let k = 1; k < slots; k++) P.rect(w, X_PIPE + 2 + k * SLOT - 1, py + 2, 1, 8, '#7cc68b');
        // selama bola terbang: kirim → slot tujuan masih kosong; terima → slot asal sudah kosong
        let items = c.items.map((val, k) => ({ val, k }));
        if (j === flyJ && t < 0.6) {
          if (v.act === 'send') items = items.filter(x => x.k !== v.fly.slot);
          else {
            const prev = v.prevChans.find(x => x.id === c.id);
            if (prev) items = prev.items.map((val, k) => ({ val, k })).filter(x => x.k !== v.fly.slot);
          }
        }
        const waitingFrom = c.items.length - (c.senders || 0); // nilai milik pengirim yang masih menunggu
        items.filter(x => x.k < 8).forEach(x => {
          const p = this.slotPos(j, x.k);
          this.ball(w, p.x, p.y, x.val, x.k >= waitingFrom);
        });
        if (c.receivers) P.text(w, '<' + c.receivers, X_PIPE + pw + 2, py + 3, '#c2457a');
      });

      // ---------- bola terbang ----------
      if (v.fly && t < 0.6) {
        const a = P.clamp(t / 0.6, 0, 1);
        const g = gopherAt(lane);
        const hand = { x: g.x + 8, y: g.y - 12 };
        const slot = this.slotPos(v.fly.j, Math.min(v.fly.slot, 7));
        const from = v.act === 'send' ? hand : slot;
        const to = v.act === 'send' ? slot : hand;
        const x = P.lerp(from.x, to.x, a);
        const y = P.lerp(from.y, to.y, a) - Math.sin(Math.PI * a) * 18;
        this.ball(w, x, y, v.fly.val, false);
      }

      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), 1, 1, p.col);

      // seluruh dunia terlihat (tanpa kamera); HUD di pojok kiri atas
      const ox = 0, oy = 0;
      const alive = v.gs.filter(g => g.state !== 'selesai').length;
      P.box(w, ox + 2, oy + 2, P.textW(`GOROUTINE ${alive}/${v.gs.length}`) + 4, 9, '#fff8fb', P.INK);
      P.text(w, `GOROUTINE ${alive}/${v.gs.length}`, ox + 4, oy + 4, P.INK);
      P.present(this.ctx, this.cv, this.world, this.W, this.H);
      if (v.act === 'panic' && t > 0.1) {
        const img = P.stamp(/deadlock/.test(v.step.note || '') ? 'DEADLOCK!' : 'PANIC!', '#d9467c'), S = this.cv.__k || 4;
        this.ctx.drawImage(img, Math.round(this.cv.width / 2 - img.width * S), Math.round(24 * S), img.width * 2 * S, img.height * 2 * S);
      }
    }

    /** Bola nilai; waiting = nilai dibawa pengirim unbuffered yang masih menunggu penerima. */
    ball(w, x, y, val, waiting) {
      const s = short(val);
      const bw = Math.max(9, P.textW(s) + 4);
      P.box(w, Math.round(x - bw / 2), Math.round(y - 5), bw, 9, waiting ? '#fff1c9' : '#ffffff', P.INK);
      P.textCenter(w, s, Math.round(x), Math.round(y - 3), P.INK);
    }
  }

  G.LaneStage = LaneStage;
})(window.GV = window.GV || {});
