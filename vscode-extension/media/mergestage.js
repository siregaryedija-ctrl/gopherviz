// Panggung belah-gabung (Fase 12c, merge sort gaya fungsional: merge_sort(arr) mengembalikan list
// baru). Setiap pemanggilan = satu potongan kubus di baris sesuai kedalamannya: saat dipanggil,
// kubus potongan itu jatuh dari baris induknya (terbelah); saat induknya menggabung (append),
// kubus terkecil dari depan potongan kiri/kanan terbang naik ke tempatnya; saat kembali, potongan
// itu sudah urut (warna = peringkat nilai). Gopher berdiri di potongan yang sedang bekerja.
(function (G) {
  'use strict';
  const P = G.pixel;
  const S = G.sorts;

  const TOP = 22;   // baris kedalaman 0
  const GAP = 6;    // celah tambahan di setiap titik belah
  // ukuran kubus menyesuaikan panjang list (list pendek = kubus besar)
  let CELL = 15, CS = 12, ROW = 30;

  class MergeStage {
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

    prepare(trace) {
      const fname = trace.block.name;
      const nodes = [], stack = [], views = [];
      const info = S.detect(trace);
      this.title = info && info.kind === 'quick' ? 'QUICK SORT' : 'MERGE SORT';
      for (const s of trace.steps) {
        const v = { step: s, ev: 'hit', node: stack.length ? stack[stack.length - 1].id : -1, moves: [] };
        if (s.phase === 'call' && S.sameFunc(s.func, fname)) {
          const parent = stack.length ? stack[stack.length - 1] : null;
          const input = S.parseList(s.args) || [];
          const nd = { id: nodes.length, depth: parent ? parent.depth + 1 : 0, input, offset: parent ? parent.cursor : 0, parent, kids: [], out: [], sorted: null };
          nd.cursor = nd.offset;
          if (parent) { parent.cursor += input.length; parent.kids.push(nd); }
          nodes.push(nd);
          stack.push(nd);
          v.ev = 'call';
          v.node = nd.id;
        } else if (s.phase === 'return' && S.sameFunc(s.func, fname) && stack.length) {
          const nd = stack.pop();
          nd.sorted = S.parseList(s.return) || nd.out.slice();
          // nilai yang belum diambil lewat append (extend / slice ...): terbang sekaligus sekarang
          while (nd.out.length < nd.sorted.length) v.moves.push(this.take(nd, nd.sorted[nd.out.length]));
          v.ev = 'return';
          v.node = nd.id;
        } else if (stack.length) {
          const nd = stack[stack.length - 1];
          for (const o of s.ops || []) if (o.op === 'append' && nd.kids.length) { v.moves.push(this.take(nd, o.value)); v.ev = 'append'; }
        }
        v.created = nodes.length;
        v.out = nodes.map(n => n.out.length);
        v.used = nodes.map(n => n.used || 0);
        v.done = nodes.map(n => n.sorted != null);
        views.push(v);
      }
      this.nodes = nodes;
      this.layout();
      this.snaps = views;
      return views;
    }

    /** Induk nd mengambil nilai val dari depan potongan anak (kiri dulu bila sama). */
    take(nd, val) {
      const k = nd.out.length;
      nd.out.push(val);
      for (const kid of nd.kids) {
        const list = kid.sorted || kid.input, u = kid.used || 0;
        if (u < list.length && String(list[u]) === String(val)) { kid.used = u + 1; return { to: nd.id, k, from: kid.id, fk: u, val }; }
      }
      return { to: nd.id, k, from: -1, fk: 0, val };
    }

    layout() {
      const root = this.nodes[0];
      const n = root ? root.input.length : 0;
      this.splits = [];
      for (const nd of this.nodes) nd.kids.slice(1).forEach(k => this.splits.push({ pos: k.offset, depth: k.depth }));
      const maxD = Math.max(0, ...this.nodes.map(x => x.depth));
      CELL = P.clamp(Math.floor((200 - this.splits.length * GAP) / Math.max(1, n)), 14, 26);
      CS = CELL - 4;
      ROW = CS + 16;
      const content = n * CELL + this.splits.length * GAP;
      this.W = Math.max(192, content + 28);
      this.x0 = Math.round((this.W - content) / 2);
      this.H = TOP + (maxD + 1) * ROW + 12;
      this.world.width = this.bg.width = this.W;
      this.world.height = this.bg.height = this.H;
      const c = this.bg.getContext('2d');
      P.paintSky(c, this.W, this.H);
      for (let d = 0; d <= maxD; d++) P.rect(c, 4, this.rowY(d) + 1, this.W - 8, 1, '#f2c4d2');
      // nilai → peringkat warna
      const all = root ? root.input.slice() : [];
      const num = all.every(v => !isNaN(+v));
      const uniq = [...new Set(num ? all.map(Number) : all)].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
      this.col = v => {
        const r = Math.max(0, uniq.indexOf(num ? +v : v));
        return P.VCOL[1 + Math.round((r / Math.max(1, uniq.length - 1)) * 8)];
      };
    }

    rowY(d) { return TOP + d * ROW + CS + 4; } // lantai baris d (kubus berdiri di atasnya)
    xAt(k, d) {
      const gaps = this.splits.filter(sp => sp.depth <= d && sp.pos <= k).length;
      return this.x0 + k * CELL + gaps * GAP;
    }

    duration(i) {
      const v = this.snaps[i];
      if (v.ev === 'call') return 700;
      if (v.ev === 'append') return 520;
      if (v.ev === 'return') return 650 + Math.min(6, v.moves.length) * 90;
      return i === this.snaps.length - 1 ? 1800 : 300;
    }

    cues(i) {
      const v = this.snaps[i], out = [];
      if (v.ev === 'call') out.push({ t: 0.1, sound: 'warp' });
      if (v.ev === 'append') out.push({ t: 0.7, sound: 'land' });
      if (v.ev === 'return') out.push({ t: 0.75, sound: v.node === 0 ? 'jingle' : 'coin' });
      if (v.step.result != null) out.push({ t: 0, sound: 'think' });
      if (v.step.output) out.push({ t: 0.5, sound: 'coin' });
      return out;
    }

    update(dt) {
      this.clock += dt;
      const f = dt / 1000;
      this.parts = this.parts.filter(p => { p.vy += 60 * f; p.x += p.vx * f; p.y += p.vy * f; p.life -= dt; return p.life > 0; });
    }

    /** Kubus satu nilai; x = kiri sisi depan, y = lantai. */
    cube(w, x, y, val, alpha, ring) {
      w.globalAlpha = alpha;
      P.cube(w, Math.round(x), Math.round(y - CS), CS, CS, this.col(val), 2, ring);
      P.textCenter(w, String(val).slice(0, 3), Math.round(x + CS / 2), Math.round(y - CS + 4), P.INK);
      w.globalAlpha = 1;
    }

    ghost(w, x, y) {
      const x0 = Math.round(x), y0 = Math.round(y - CS);
      for (let k = 0; k < CS; k += 2) {
        P.rect(w, x0 + k, y0, 1, 1, '#e0b4c6'); P.rect(w, x0 + k, y0 + CS - 1, 1, 1, '#e0b4c6');
        P.rect(w, x0, y0 + k, 1, 1, '#e0b4c6'); P.rect(w, x0 + CS - 1, y0 + k, 1, 1, '#e0b4c6');
      }
    }

    render(i, t) {
      const v = this.snaps[i];
      if (!v) return;
      const w = this.wx, now = this.clock;
      P.drawBg(w, this.bg);
      const moving = new Set((v.moves || []).map(m => `${m.to}:${m.k}`));
      const leaving = new Set((v.moves || []).filter(m => m.from >= 0).map(m => `${m.from}:${m.fk}`));
      const mt = P.clamp((t - 0.15) / 0.7, 0, 1);

      for (let id = 0; id < v.created; id++) {
        const nd = this.nodes[id], d = nd.depth, y = this.rowY(d);
        const done = v.done[id], kidsOn = nd.kids.some(k => k.id < v.created);
        const parentDone = nd.parent && v.done[nd.parent.id];
        const list = done ? nd.sorted : kidsOn ? nd.out.slice(0, v.out[id]) : nd.input;
        const len = nd.input.length;
        for (let k = 0; k < len; k++) {
          const x = this.xAt(nd.offset + k, d);
          const used = k < v.used[id] || (leaving.has(`${id}:${k}`) && mt > 0);
          const val = list[k];
          if (val == null || used || moving.has(`${id}:${k}`)) { this.ghost(w, x, y); continue; }
          // kubus yang baru terbelah jatuh dari baris induk
          let yy = y, xx = x;
          if (v.ev === 'call' && v.node === id && nd.parent) {
            const e = P.ease(P.clamp(t / 0.6, 0, 1));
            yy = P.lerp(this.rowY(d - 1), y, e);
            xx = P.lerp(this.xAt(nd.offset + k, d - 1), x, e);
          }
          const dim = parentDone ? 0.35 : done ? 1 : kidsOn ? 1 : 0.85;
          this.cube(w, xx, yy, val, dim, done && v.node === id && v.ev === 'return' && t > 0.7 ? '#2f8a5c' : null);
        }
        if (done && !parentDone && len > 1) P.textCenter(w, '✓', this.xAt(nd.offset, d) + (len * CELL) / 2, y + 3, '#2f8a5c');
      }
      // kubus yang sedang terbang naik dari potongan anak ke induknya
      for (const m of v.moves || []) {
        const to = this.nodes[m.to], from = m.from >= 0 ? this.nodes[m.from] : null;
        const x1 = this.xAt(to.offset + m.k, to.depth), y1 = this.rowY(to.depth);
        const x0 = from ? this.xAt(from.offset + m.fk, from.depth) : x1, y0 = from ? this.rowY(from.depth) : y1 - 20;
        const e = P.ease(mt);
        this.cube(w, P.lerp(x0, x1, e), P.lerp(y0, y1, e) - Math.sin(Math.PI * e) * 14, m.val, 1, '#d9467c');
      }

      // Gopher di potongan aktif
      const act = this.nodes[v.node] || this.nodes[0];
      if (act) {
        const cx = this.xAt(act.offset, act.depth) + (act.input.length * CELL) / 2 - 1;
        let gy = this.rowY(act.depth) - CS - 2;
        if (v.ev === 'call' && act.parent) gy = P.lerp(this.rowY(act.depth - 1) - CS - 2, gy, P.bounce(P.clamp(t / 0.6, 0, 1)));
        const face = v.ev === 'return' && t > 0.6 ? 'happy' : v.step.result != null ? 'think' : (now % 3400) < 130 ? 'blink' : 'idle';
        P.drawGopher(w, cx, gy, face, 0);
      }
      for (const p of this.parts) P.rect(w, Math.round(p.x), Math.round(p.y), 1, 1, p.col);

      // HUD: judul + fase
      P.rect(w, 0, 0, this.W, 16, 'rgba(255,248,251,0.75)');
      P.text(w, this.title, 4, 3, P.INK);
      const phase = v.ev === 'call' ? 'BELAH' : v.ev === 'append' || v.ev === 'return' ? 'GABUNG' : '';
      const merges = v.done.filter(Boolean).length;
      P.text(w, `${phase ? phase + '  ' : ''}SELESAI ${merges}/${this.nodes.length}`, 4, 10, '#c2457a');
      if (i === this.snaps.length - 1 && v.done[0]) {
        const img = P.stamp('URUT!', '#2f8a5c'), k = 2;
        w.drawImage(img, Math.round(this.W / 2 - (img.width * k) / 2), Math.round(this.H - 24 - img.height), img.width * k, img.height * k);
      }
      P.present(this.ctx, this.cv, this.world, this.W, this.H);
    }
  }

  G.MergeStage = MergeStage;
})(window.GV = window.GV || {});
