// Tumpukan & antrian (Fase 12d). Koleksi yang diisi lalu diambil satu per satu dikenali dari
// perubahan isinya per langkah: masuk dan keluar di ujung yang SAMA = tumpukan (LIFO, menara
// kubus: push jatuh ke puncak, pop terbang ke tangan Gopher); masuk di satu ujung dan keluar di
// ujung lain = antrian (FIFO, ban berjalan: masuk dari belakang, keluar dari depan).
(function (G) {
  'use strict';
  const P = G.pixel;

  /**
   * Isi koleksi per langkah dan kejadiannya. Hasil: { kind: 'stack'|'queue', pushEnd: 'back'|'front',
   * lists: [[...]] (isi SESUDAH langkah i), events: [[{type, end, value}]] } atau null.
   */
  function detect(tr) {
    if (!tr || !tr.collection || tr.collection.kind === 'map' || tr.graph || tr.grid || tr.tree) return null;
    let cur = (tr.collection.items || []).slice();
    const lists = [], events = [];
    let pushB = 0, pushF = 0, popB = 0, popF = 0, other = 0;
    const eq = (a, b) => a.length === b.length && a.every((x, k) => x === b[k]);
    const first = tr.steps.find(s => s.items);
    if (first) { // isi awal = sebelum operasi pertama langkah itu
      const ops = first.ops || [];
      if (ops.length === 1 && ops[0].op === 'append') cur = first.items.slice(0, -1);
      else if (ops.length === 1 && ops[0].op === 'pop') cur = ops[0].a === 0 ? [ops[0].value, ...first.items] : [...first.items, ops[0].value];
      else cur = first.items.slice();
    }
    const init = cur.slice();
    for (const s of tr.steps) {
      const prev = cur.slice(), ev = [];
      for (const o of s.ops || []) {
        if (o.op === 'swap' || o.op === 'set') other++;
      }
      if (s.items) cur = s.items.slice();
      if (!eq(prev, cur)) {
        if (cur.length === prev.length + 1 && eq(cur.slice(0, -1), prev)) { ev.push({ type: 'push', end: 'back', value: cur[cur.length - 1] }); pushB++; }
        else if (cur.length === prev.length + 1 && eq(cur.slice(1), prev)) { ev.push({ type: 'push', end: 'front', value: cur[0] }); pushF++; }
        else if (cur.length === prev.length - 1 && eq(cur, prev.slice(0, -1))) { ev.push({ type: 'pop', end: 'back', value: prev[prev.length - 1] }); popB++; }
        else if (cur.length === prev.length - 1 && eq(cur, prev.slice(1))) { ev.push({ type: 'pop', end: 'front', value: prev[0] }); popF++; }
        else { ev.push({ type: 'change' }); other++; }
      }
      lists.push(cur.slice());
      events.push(ev);
    }
    const pushes = pushB + pushF, pops = popB + popF;
    if (!pushes || !pops || other > (pushes + pops) / 2) return null;
    const pushEnd = pushB >= pushF ? 'back' : 'front';
    const same = pushEnd === 'back' ? popB : popF, opp = pops - same;
    return { kind: same >= opp ? 'stack' : 'queue', pushEnd, init, lists, events };
  }

  class SQStage {
    constructor(canvas) {
      this.cv = canvas;
      this.ctx = canvas.getContext('2d');
      this.world = document.createElement('canvas');
      this.wx = this.world.getContext('2d');
      this.bg = document.createElement('canvas');
      this.clock = 0;
      this.parts = [];
      this.snaps = [];
    }

    prepare(trace, info) {
      this.info = info;
      this.stack = info.kind === 'stack';
      // urutan tampilan: tumpukan = dasar → puncak; antrian = depan (keluar) → belakang (masuk).
      // Barang masuk di ujung depan (Java ArrayDeque.push, appendleft): urutannya dibalik.
      const norm = l => (info.pushEnd === 'front' ? l.slice().reverse() : l.slice());
      const views = trace.steps.map((s, i) => ({
        step: s, list: norm(info.lists[i]), before: norm(i > 0 ? info.lists[i - 1] : info.init),
        ev: info.events[i].find(e => e.type === 'push' || e.type === 'pop') || null,
      }));
      const maxN = Math.max(1, ...views.map(v => Math.max(v.list.length, v.before.length)));
      this.maxN = maxN;
      if (this.stack) {
        this.cs = P.clamp(Math.floor(84 / maxN) - 2, 9, 18);
        this.W = 192; this.H = 128; this.GY = 112;
      } else {
        this.cs = P.clamp(Math.floor(150 / maxN) - 4, 10, 20);
        this.W = Math.max(192, 60 + maxN * (this.cs + 4) + 30); this.H = 96; this.GY = 70;
      }
      this.world.width = this.bg.width = this.W;
      this.world.height = this.bg.height = this.H;
      P.paintWorld(this.bg.getContext('2d'), this.W, this.H, this.GY);
      // nilai → warna (urut kemunculan, supaya setiap barang punya warna tetap)
      const seen = [];
      for (const v of views) for (const x of v.list) if (!seen.includes(x)) seen.push(x);
      this.col = x => P.VCOL[1 + (Math.max(0, seen.indexOf(x)) % 9)];
      this.snaps = views;
      return views;
    }

    duration(i) {
      const v = this.snaps[i];
      if (v.ev) return 900;
      return i === this.snaps.length - 1 ? 1800 : 380;
    }

    cues(i) {
      const v = this.snaps[i], out = [];
      if (v.ev && v.ev.type === 'push') out.push({ t: 0.55, sound: 'land' });
      if (v.ev && v.ev.type === 'pop') out.push({ t: 0.15, sound: 'jump' }, { t: 0.7, sound: 'coin' });
      if (v.step.output) out.push({ t: 0.5, sound: 'coin' });
      if (v.step.reason === 'panic') out.push({ t: 0, sound: 'panic' });
      return out;
    }

    update(dt) {
      this.clock += dt;
      const f = dt / 1000;
      this.parts = this.parts.filter(p => { p.vy += 60 * f; p.x += p.vx * f; p.y += p.vy * f; p.life -= dt; return p.life > 0; });
    }

    box(w, x, y, val, ring) { // x = kiri, y = lantai
      const cs = this.cs, d = Math.max(1, Math.round(cs / 6));
      P.cube(w, Math.round(x), Math.round(y - cs), cs, cs, this.col(val), d, ring);
      P.textCenter(w, P.fit(String(val).replace(/^['"]|['"]$/g, ''), Math.max(1, Math.floor(cs / 4))), Math.round(x + cs / 2), Math.round(y - cs + (cs - 5) / 2), P.INK);
    }

    render(i, t) {
      const v = this.snaps[i];
      if (!v) return;
      const w = this.wx, now = this.clock;
      P.drawBg(w, this.bg);
      if (this.stack) this.renderStack(w, v, t, now); else this.renderQueue(w, v, t, now);
      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), 1, 1, p.col);
      P.rect(w, 0, 0, this.W, 16, 'rgba(255,248,251,0.75)');
      P.text(w, this.stack ? 'TUMPUKAN (STACK) LIFO' : 'ANTRIAN (QUEUE) FIFO', 4, 3, P.INK);
      P.text(w, `ISI ${v.list.length}`, 4, 10, '#c2457a');
      P.present(this.ctx, this.cv, this.world, this.W, this.H);
    }

    renderStack(w, v, t, now) {
      const cs = this.cs, gap = 2, cx = Math.round(this.W / 2 + 10), x = cx - cs / 2, GY = this.GY;
      const yAt = k => GY - k * (cs + gap);
      // alas menara
      P.rect(w, x - 6, GY, cs + 12, 3, '#c9a2b8');
      const push = v.ev && v.ev.type === 'push', pop = v.ev && v.ev.type === 'pop';
      const list = pop ? v.before : v.list;
      list.forEach((val, k) => {
        const top = k === list.length - 1;
        if (push && top) { // jatuh dari atas ke puncak
          const e = P.bounce(P.clamp(t / 0.6, 0, 1));
          this.box(w, x, P.lerp(20, yAt(k), e), val, '#d9467c');
          return;
        }
        if (pop && top) { // naik lalu terbang ke tangan Gopher
          const e = P.ease(P.clamp(t / 0.7, 0, 1));
          this.box(w, P.lerp(x, 34, e), P.lerp(yAt(k), GY - 30, e) - Math.sin(Math.PI * e) * 26, val, '#e0a22a');
          return;
        }
        this.box(w, x, yAt(k), val, null);
      });
      // penanda puncak
      const topK = v.list.length - 1;
      if (topK >= 0 && !(push && t < 0.6)) {
        const ty = yAt(topK) - cs - 4;
        P.text(w, 'TOP', x + cs + 6, ty + cs / 2, '#c2457a');
        P.rect(w, x + cs + 2, ty + cs / 2 + 2, 3, 1, '#c2457a');
      }
      const face = pop && t > 0.6 ? 'happy' : push ? 'idle' : (now % 3400) < 130 ? 'blink' : 'idle';
      const hy = P.drawGopher(w, 34, GY, face, 0);
      if (pop && t > 0.7) P.text(w, `POP ${P.fit(String(v.ev.value).replace(/^['"]|['"]$/g, ''), 6)}`, 18, hy - 10, '#2f8a5c');
      if (push && t < 0.5) P.text(w, `PUSH ${P.fit(String(v.ev.value).replace(/^['"]|['"]$/g, ''), 6)}`, cx - 20, 20, '#d9467c');
    }

    renderQueue(w, v, t, now) {
      const cs = this.cs, step = cs + 4, GY = this.GY, x0 = 40;
      const xAt = k => x0 + k * step;
      // ban berjalan
      const bw = this.maxN * step + 16;
      P.rect(w, x0 - 8, GY, bw, 5, '#8a7a92');
      const roll = (v.ev ? Math.floor(t * 12) : 0) + Math.floor(now / 400);
      for (let k = 0; k < bw; k += 6) P.rect(w, x0 - 8 + ((k + roll) % bw), GY + 2, 2, 1, '#cfc2d6');
      P.text(w, 'DEPAN', x0 - 8, GY + 8, '#c2457a');
      P.text(w, 'BELAKANG', x0 - 8 + bw - P.textW('BELAKANG'), GY + 8, '#c2457a');
      const push = v.ev && v.ev.type === 'push', pop = v.ev && v.ev.type === 'pop';
      if (pop) {
        const e = P.ease(P.clamp(t / 0.7, 0, 1));
        v.before.forEach((val, k) => {
          if (k === 0) { this.box(w, P.lerp(xAt(0), 12, e), GY - Math.sin(Math.PI * e) * 18, val, '#e0a22a'); return; }
          this.box(w, P.lerp(xAt(k), xAt(k - 1), e), GY, val, null); // maju satu langkah
        });
      } else {
        v.list.forEach((val, k) => {
          if (push && k === v.list.length - 1) { // masuk dari ujung kanan
            const e = P.ease(P.clamp(t / 0.6, 0, 1));
            this.box(w, P.lerp(this.W + 4, xAt(k), e), GY, val, '#d9467c');
            return;
          }
          this.box(w, xAt(k), GY, val, null);
        });
      }
      const face = pop && t > 0.6 ? 'happy' : (now % 3400) < 130 ? 'blink' : 'idle';
      const hy = P.drawGopher(w, 16, GY, face, 0);
      if (pop && t > 0.7) P.text(w, `DEQUEUE ${P.fit(String(v.ev.value).replace(/^['"]|['"]$/g, ''), 6)}`, 8, hy - 10, '#2f8a5c');
      if (push && t < 0.6) P.text(w, `ENQUEUE ${P.fit(String(v.ev.value).replace(/^['"]|['"]$/g, ''), 6)}`, this.W - 70, 22, '#d9467c');
    }
  }

  G.SQStage = SQStage;
  G.sq = { detect };
})(typeof window !== 'undefined' ? (window.GV = window.GV || {}) : (globalThis.GV = globalThis.GV || {}));
