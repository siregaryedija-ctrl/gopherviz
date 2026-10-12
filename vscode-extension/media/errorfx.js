// Adegan error (Fase 12b): saat program berhenti karena error, panggung meredup dan sebuah
// adegan pixel kecil menjelaskan penyebabnya sesuai jenisnya (Step.err.kind):
//   index    Gopher berjalan di atas kubus lalu jatuh di kubus yang tidak ada
//   empty    Gopher merogoh wadah yang kosong
//   zero     koin dibagi ke 0 kotak lalu meledak
//   null     peti berlabel None/null/nil ternyata kosong
//   key      Gopher membawa kunci, tidak ada loker yang cocok
//   depth    menara lantai rekursi terus bertambah lalu roboh
//   type     dua kubus beda tipe saling memantul
//   value    teks dimasukkan ke mesin pengubah, keluar asap
//   deadlock dua Gopher saling menunggu dan tertidur
//   other    lampu alarm
// Adegan digambar di dunia kecil 176x104 lalu ditempel di atas panggung dengan skala bulat.
(function (G) {
  'use strict';
  const P = G.pixel;
  const { INK, rect, text, textCenter, textW, fit, cube, disc, clamp, lerp, ease, drawGopher, bubble } = P;

  const W = 176, H = 104, GY = 74; // GY = lantai adegan
  const DUR = 5200;               // lama adegan (ms) pada kecepatan 1x
  const RED = '#e5534b', PAPER = '#fff6f8';
  const world = document.createElement('canvas');
  world.width = W; world.height = H;
  const c = world.getContext('2d');

  const seg = (ms, a, b) => clamp((ms - a) / (b - a), 0, 1); // kemajuan 0..1 di antara a..b ms
  const walkStep = ms => Math.floor(ms / 140) % 2;

  // ---------- adegan ----------

  function sceneIndex(e, ms) {
    const n = e.len != null ? e.len : (e.items || []).length || 4;
    const v = parseInt(e.value, 10);
    const items = e.items || [];
    // slot yang digambar: indeks 0..n-1 (dipadatkan bila panjang) lalu slot hantu di indeks v
    const slots = [];
    if (n <= 6) for (let k = 0; k < n; k++) slots.push(k);
    else slots.push(0, 1, 2, -1, n - 2, n - 1); // -1 = celah ".."
    const neg = !isNaN(v) && v < 0;
    const ghostGap = !isNaN(v) && v > n; // ada indeks di antara akhir koleksi dan v
    if (ghostGap) slots.push(-1);
    if (neg) slots.unshift(-2); // -2 = hantu di kiri
    else slots.push(-2);
    const S = 14, STEP = 18, x0 = Math.round(W / 2 - (slots.length * STEP - 4) / 2), top = GY - S;
    const xs = [];
    slots.forEach((k, j) => {
      const x = x0 + j * STEP;
      xs.push(x + S / 2);
      if (k === -1) { textCenter(c, '..', x + S / 2, top + 5, INK); return; }
      if (k === -2) { // kubus yang tidak ada
        const blink = Math.floor(ms / 300) % 2 ? RED : '#f4a3a0';
        for (let i = 0; i < S; i += 2) { rect(c, x + i, top, 1, 1, blink); rect(c, x + i, top + S - 1, 1, 1, blink); rect(c, x, top + i, 1, 1, blink); rect(c, x + S - 1, top + i, 1, 1, blink); }
        textCenter(c, '?', x + S / 2, top + 5, RED);
        textCenter(c, isNaN(v) ? '?' : String(v), x + S / 2, GY + 4, RED);
        return;
      }
      cube(c, x, top, S, S, P.VCOL[(k % 9) + 1], 2);
      if (items[k] != null) textCenter(c, fit(items[k], 3), x + S / 2, top + 5, INK);
      textCenter(c, String(k), x + S / 2, GY + 4, INK);
    });
    // Gopher: berjalan dari kubus pertama ke kubus asli terakhir, lalu ke hantu, lalu jatuh
    const real = xs.filter((_, j) => slots[j] >= 0);
    const ghostX = xs[slots.indexOf(-2)];
    const startX = neg ? real[real.length - 1] : real[0], endX = neg ? real[0] : real[real.length - 1];
    let gx, gy = top, face = 'idle';
    const walk = seg(ms, 300, 2300), toGhost = seg(ms, 2300, 2700), fall = seg(ms, 2700, 3500);
    if (ms < 2300) gx = lerp(startX, endX, ease(walk));
    else gx = lerp(endX, ghostX, toGhost);
    if (fall > 0) { gy = top + fall * fall * 70; face = 'shock'; }
    const step = ms < 2700 && ms > 300 ? walkStep(ms) : 0;
    c.save();
    c.beginPath(); c.rect(0, 0, W, H - 14); c.clip();
    drawGopher(c, gx, gy, face, step);
    c.restore();
    if (fall >= 1) textCenter(c, 'AAA!', ghostX, top - 20 + Math.round(Math.sin(ms / 90)), RED);
    return n === 0 ? 'EMPTY COLLECTION' : `INDEX ${isNaN(v) ? '?' : v} · VALID 0..${n - 1}`;
  }

  function chest(x, open, label, col) {
    const w = 30, h = 18, y = GY - h;
    rect(c, x - w / 2 - 1, y - 1, w + 2, h + 2, INK);
    rect(c, x - w / 2, y, w, h, col);
    rect(c, x - w / 2, y + 5, w, 1, P.mix(col, INK, 0.35));
    if (open) {
      rect(c, x - w / 2 + 2, y + 1, w - 4, 4, '#2a1f30'); // isi gelap = kosong
      rect(c, x - w / 2 - 1, y - 12, w + 2, 10, INK);
      rect(c, x - w / 2, y - 11, w, 8, P.mix(col, '#ffffff', 0.25));
    } else {
      rect(c, x - w / 2 - 1, y - 5, w + 2, 6, INK);
      rect(c, x - w / 2, y - 4, w, 4, P.mix(col, '#ffffff', 0.25));
      rect(c, x - 1, y - 1, 3, 4, '#f1d064');
    }
    textCenter(c, fit(label, 7), x, y + 9, '#fff6f8');
  }

  function puff(cx, cy, p, col) {
    if (p <= 0 || p >= 1) return;
    for (let k = 0; k < 7; k++) {
      const a = (k / 7) * Math.PI * 2, r = 4 + p * 14;
      disc(c, Math.round(cx + Math.cos(a) * r), Math.round(cy + Math.sin(a) * r * 0.6), Math.max(1, Math.round(3 * (1 - p))), col);
    }
  }

  function sceneChest(e, ms, label) {
    const cx = 108;
    const walk = seg(ms, 200, 1400), open = ms > 1700;
    const gx = lerp(30, cx - 24, ease(walk));
    chest(cx, open, label, e.kind === 'empty' ? '#b48a5a' : '#9a6b4f');
    puff(cx, GY - 22, seg(ms, 1700, 2600), '#d8c8cf');
    const face = ms > 2200 ? 'shock' : 'idle';
    const hy = drawGopher(c, gx, GY, face, ms > 200 && ms < 1400 ? walkStep(ms) : 0);
    if (ms > 2400) bubble(c, gx, hy - 15, '?', RED);
    return e.kind === 'empty' ? 'ALREADY EMPTY' : `IT HOLDS ${fit(e.value || 'NULL', 10)}: NO DATA`;
  }

  function sceneZero(e, ms) {
    const gx = 34, zx = 128;
    cube(c, zx - 11, GY - 22, 22, 22, '#e9e0e6', 3);
    textCenter(c, '0', zx, GY - 14, RED);
    textCenter(c, 'BOX', zx, GY + 4, INK);
    const hy = drawGopher(c, gx, GY, ms > 2900 ? 'shock' : 'idle', 0);
    for (let k = 0; k < 4; k++) { // koin terbang satu per satu ke kotak 0
      const p = seg(ms, 600 + k * 450, 1300 + k * 450);
      if (p <= 0) { disc(c, gx + 7, hy + 8 - k * 3, 3, '#f1d064'); continue; }
      if (p >= 1) continue;
      const x = lerp(gx + 7, zx, p), y = lerp(hy + 8, GY - 30, p) - Math.sin(p * Math.PI) * 26;
      disc(c, Math.round(x), Math.round(y), 3, '#f1d064');
    }
    const boom = seg(ms, 2700, 3700);
    if (boom > 0 && boom < 1) {
      for (let k = 0; k < 10; k++) {
        const a = (k / 10) * Math.PI * 2, r = 6 + boom * 26;
        rect(c, Math.round(zx + Math.cos(a) * r), Math.round(GY - 12 + Math.sin(a) * r), 2, 2, k % 2 ? RED : '#f1d064');
      }
    }
    if (ms > 2900) textCenter(c, 'BOOM!', zx, GY - 40, RED);
    return `${fit(e.expr || 'A / B', 18)} : DIVISOR IS 0`;
  }

  function sceneKey(e, ms) {
    const keys = (e.items && e.items.length ? e.items : ['?', '?', '?']).slice(0, 4);
    const lw = 24, x0 = Math.round(W / 2 - (keys.length * (lw + 4)) / 2) + 14;
    const per = 2400 / keys.length;
    keys.forEach((k, j) => {
      const x = x0 + j * (lw + 4);
      rect(c, x - 1, GY - 31, lw + 2, 32, INK);
      rect(c, x, GY - 30, lw, 30, '#8ec4f0');
      rect(c, x + 2, GY - 27, lw - 4, 1, '#cfe5f8');
      textCenter(c, fit(k, 5), x + lw / 2, GY - 18, INK);
      if (ms > 400 + (j + 0.8) * per) text(c, '✗', x + lw / 2 - 1, GY - 40, RED);
    });
    const p = seg(ms, 300, 300 + 2400);
    const gx = lerp(14, x0 + keys.length * (lw + 4) - 4, p);
    const hy = drawGopher(c, gx, GY, ms > 2900 ? 'shock' : 'think', p > 0 && p < 1 ? walkStep(ms) : 0);
    const tag = fit(e.value || '?', 8);
    rect(c, gx - textW(tag) / 2 - 3, hy - 11, textW(tag) + 6, 9, INK);
    rect(c, gx - textW(tag) / 2 - 2, hy - 10, textW(tag) + 4, 7, '#f1d064');
    textCenter(c, tag, gx, hy - 9, INK);
    return `KEY ${tag} IS MISSING`;
  }

  function sceneDepth(e, ms) {
    const fh = 6, maxF = Math.floor((GY - 10) / fh);
    const grow = seg(ms, 0, 2600);
    const floors = Math.max(1, Math.round(grow * maxF));
    const fall = seg(ms, 3000, 4200);
    const wob = ms > 2600 && ms < 3000 ? Math.round(Math.sin(ms / 30) * 2) : 0;
    for (let k = 0; k < floors; k++) {
      const y0 = GY - (k + 1) * fh;
      const drift = fall > 0 ? (k % 2 ? 1 : -1) * fall * (k * 2.5) : wob * (k / maxF);
      const y = fall > 0 ? Math.min(GY - fh, y0 + fall * fall * 90 * (k / maxF + 0.2)) : y0;
      const col = P.VCOL[(k % 9) + 1];
      rect(c, Math.round(W / 2 - 24 + drift), Math.round(y), 48, fh - 1, col);
      rect(c, Math.round(W / 2 - 24 + drift), Math.round(y + fh - 2), 48, 1, P.mix(col, INK, 0.4));
    }
    const count = Math.round(grow * (parseInt(e.value, 10) || 200));
    textCenter(c, `FLOOR ${count}`, W / 2 + 50, 12, INK);
    drawGopher(c, W / 2 - 40, GY, ms > 2600 ? 'shock' : 'idle', 0);
    return 'NO BASE CASE?';
  }

  function sceneType(e, ms, value) {
    const meet = seg(ms, 300, 1300), back = seg(ms, 1300, 2000);
    const off = lerp(0, 22, ease(meet)) - lerp(0, 14, back);
    const lx = 40 + off, rx = W - 58 - off;
    cube(c, lx, GY - 18, 18, 18, '#8ec4f0', 3);
    cube(c, rx, GY - 18, 18, 18, '#f5a3bf', 3);
    textCenter(c, value ? '"AB"' : '5', lx + 9, GY - 11, INK);
    textCenter(c, value ? 'INT' : '"A"', rx + 9, GY - 11, INK);
    textCenter(c, value ? 'TEXT' : 'NUMBER', lx + 9, GY + 4, INK);
    textCenter(c, value ? 'NUMBER?' : 'TEXT', rx + 9, GY + 4, INK);
    if (ms > 1250) {
      textCenter(c, 'X', W / 2, GY - 30, RED);
      puff(W / 2, GY - 10, seg(ms, 1300, 2200), value ? '#bdb2b8' : '#f4a3a0');
    }
    drawGopher(c, 20, GY, ms > 1300 ? 'shock' : 'think', 0);
    return value ? 'CANNOT CONVERT IT' : 'TYPES DO NOT MATCH';
  }

  function sceneDeadlock(e, ms) {
    rect(c, 46, GY - 14, W - 92, 5, '#9fd88f'); // pipa channel di antara keduanya
    rect(c, 46, GY - 15, W - 92, 1, INK); rect(c, 46, GY - 9, W - 92, 1, INK);
    drawGopher(c, 32, GY, 'blink', 0);
    drawGopher(c, W - 32, GY, 'blink', 0);
    for (let k = 0; k < 3; k++) {
      const p = ((ms / 1400) + k / 3) % 1;
      text(c, 'Z', Math.round(36 + p * 10), Math.round(GY - 30 - p * 24), P.mix(INK, '#ffffff', p));
      text(c, 'Z', Math.round(W - 30 + p * 10), Math.round(GY - 30 - p * 24), P.mix(INK, '#ffffff', p));
    }
    textCenter(c, 'WAITING..', W / 2, GY - 30, INK);
    return 'EVERYONE WAITS';
  }

  function sceneOther(e, ms) {
    const on = Math.floor(ms / 250) % 2 === 0;
    rect(c, W / 2 - 7, GY - 46, 14, 4, INK);
    disc(c, W / 2, GY - 52, 7, on ? RED : '#f4a3a0');
    if (on) for (let k = 0; k < 6; k++) {
      const a = Math.PI + (k / 5) * Math.PI;
      rect(c, Math.round(W / 2 + Math.cos(a) * 14), Math.round(GY - 52 + Math.sin(a) * 12), 2, 2, RED);
    }
    drawGopher(c, W / 2, GY, 'shock', 0);
    return fit(e.name && e.name !== 'panic' ? e.name : 'PROGRAM STOPPED', 30);
  }

  // ---------- bingkai ----------

  function paint(e, ms) {
    rect(c, 0, 0, W, H, PAPER);
    rect(c, 0, GY + 1, W, H - GY - 1, '#fbe3e8');
    rect(c, 0, GY, W, 1, '#f2c4d2');
    let caption;
    switch (e.kind) {
      case 'index': caption = sceneIndex(e, ms); break;
      case 'empty': caption = sceneChest(e, ms, 'EMPTY'); break;
      case 'null': caption = sceneChest(e, ms, e.value || 'NULL'); break;
      case 'zero': caption = sceneZero(e, ms); break;
      case 'key': caption = sceneKey(e, ms); break;
      case 'depth': caption = sceneDepth(e, ms); break;
      case 'type': caption = sceneType(e, ms, false); break;
      case 'value': caption = sceneType(e, ms, true); break;
      case 'deadlock': caption = sceneDeadlock(e, ms); break;
      default: caption = sceneOther(e, ms);
    }
    // judul merah di atas, keterangan di bawah
    const title = fit(e.title || 'ERROR', 40);
    rect(c, 0, 0, W, 11, RED);
    textCenter(c, title, W / 2, 3, '#ffffff');
    rect(c, 0, H - 11, W, 11, INK);
    textCenter(c, fit(caption, 42), W / 2, H - 8, '#ffffff');
    // bingkai
    rect(c, 0, 0, W, 1, INK); rect(c, 0, H - 1, W, 1, INK); rect(c, 0, 0, 1, H, INK); rect(c, W - 1, 0, 1, H, INK);
  }

  /**
   * Menggambar adegan error di atas panggung. ctx/canvas = panggung yang sudah digambar,
   * t = kemajuan langkah error 0..1. Dipanggil setiap frame (juga saat merekam).
   */
  function draw(ctx, canvas, e, t) {
    const ms = t * DUR;
    paint(e, ms);
    const cw = canvas.width, ch = canvas.height;
    const fade = clamp(t * 8, 0, 1);
    ctx.save();
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.fillStyle = `rgba(42, 31, 48, ${0.55 * fade})`;
    ctx.fillRect(0, 0, cw, ch);
    const k = Math.max(1, Math.floor(Math.min((cw * 0.92) / W, (ch * 0.92) / H)));
    const w = W * k, h = H * k;
    ctx.globalAlpha = fade;
    ctx.imageSmoothingEnabled = false;
    ctx.drawImage(world, Math.round((cw - w) / 2), Math.round((ch - h) / 2), w, h);
    ctx.restore();
  }

  /** Bunyi selama adegan (t 0..1). */
  function cues(e) {
    const at = ms => ms / DUR;
    switch (e.kind) {
      case 'index': return [{ t: at(2700), sound: 'warp' }, { t: at(3400), sound: 'panic' }];
      case 'zero': return [0, 1, 2, 3].map(k => ({ t: at(1300 + k * 450), sound: 'tok' })).concat([{ t: at(2800), sound: 'panic' }]);
      case 'null': case 'empty': return [{ t: at(1700), sound: 'door' }, { t: at(2400), sound: 'no' }];
      case 'key': return [{ t: at(2900), sound: 'gateNo' }];
      case 'depth': return [{ t: at(3000), sound: 'panic' }];
      case 'type': case 'value': return [{ t: at(1300), sound: 'thud' }, { t: at(1500), sound: 'no' }];
      case 'deadlock': return [{ t: at(400), sound: 'think' }];
      default: return [{ t: 0.05, sound: 'panic' }];
    }
  }

  G.errorfx = { draw, cues, DUR };
})(window.GV = window.GV || {});
