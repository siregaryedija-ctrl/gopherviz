// Tema latar (Fase 13, keputusan user 2026-10-10/11): Siang (padang rumput), Malam (bintang &
// bulan), Gua (obor & kristal), Pantai (senja, laut, kelapa). Latar BERGERAK: awan berjalan,
// bintang berkelip + bintang jatuh + kunang-kunang, obor berkedip + tetes air, ombak + camar.
// Checkbox "Pixel" hanya memengaruhi latar: pixel = digambar di dunia beresolusi rendah (kotak-kotak,
// gradasi berdither); halus = digambar di canvas beresolusi penuh di BAWAH dunia (gradien dan
// bentuk mulus), sementara kubus, Gopher, dan teks tetap pixel.
//
// Panggung tidak tahu tema: P.paintWorld / P.paintSky hanya menandai canvas latarnya (jenis +
// garis tanah); setiap frame P.drawBg menggambar tema di bawah isi latar panggung, dan P.present
// memanggil underlay() untuk mode halus.
(function (G) {
  'use strict';
  const P = G.pixel;
  const NAMES = ['day', 'night', 'cave', 'beach'];

  // ---------- alat gambar: satu API untuk mode pixel dan halus ----------

  /** Mode pixel: koordinat bulat, gradasi bertingkat dengan dither seperti latar lama. */
  function pixelDraw(c) {
    const r = Math.round;
    return {
      grad(x, y, w, h, stops) {
        const n = Math.max(2, Math.min(7, Math.round(h / 12)));
        const bh = Math.ceil(h / n);
        for (let k = 0; k < n; k++) {
          const col = at(stops, n === 1 ? 0 : k / (n - 1));
          P.rect(c, r(x), r(y + k * bh), r(w), bh + 1, col);
          if (k > 0) { // dither di batas pita
            c.fillStyle = at(stops, (k - 1) / (n - 1));
            for (let xx = r(x); xx < x + w; xx += 2) c.fillRect(xx, r(y + k * bh), 1, 1);
          }
        }
      },
      circle(cx, cy, rad, col) { P.disc(c, r(cx), r(cy), Math.max(0, r(rad)), col); },
      ellipse(cx, cy, rx, ry, col) { P.ell(c, r(cx), r(cy), Math.max(0, r(rx)), Math.max(0, r(ry)), col); },
      rect(x, y, w, h, col) { P.rect(c, r(x), r(y), Math.max(1, r(w)), Math.max(1, r(h)), col); },
      poly(pts, col) { fillPoly(c, pts, col); },
      dot(x, y, s, col) { P.rect(c, r(x), r(y), Math.max(1, r(s)), Math.max(1, r(s)), col); },
      alpha(a) { c.globalAlpha = a; },
    };
  }

  /** Mode halus: canvas beresolusi penuh, koordinat dunia diskalakan k. */
  function smoothDraw(c, k) {
    c.setTransform(k, 0, 0, k, 0, 0);
    c.imageSmoothingEnabled = true;
    return {
      grad(x, y, w, h, stops) {
        const g = c.createLinearGradient(0, y, 0, y + h);
        for (const [t, col] of stops) g.addColorStop(t, col);
        c.fillStyle = g;
        c.fillRect(x, y, w, h);
      },
      circle(cx, cy, rad, col) { c.fillStyle = col; c.beginPath(); c.arc(cx, cy, Math.max(0, rad), 0, Math.PI * 2); c.fill(); },
      ellipse(cx, cy, rx, ry, col) { c.fillStyle = col; c.beginPath(); c.ellipse(cx, cy, Math.max(0, rx), Math.max(0, ry), 0, 0, Math.PI * 2); c.fill(); },
      rect(x, y, w, h, col) { c.fillStyle = col; c.fillRect(x, y, w, h); },
      poly(pts, col) {
        c.fillStyle = col; c.beginPath();
        pts.forEach(([x, y], i) => (i ? c.lineTo(x, y) : c.moveTo(x, y)));
        c.closePath(); c.fill();
      },
      dot(x, y, s, col) { c.fillStyle = col; c.beginPath(); c.arc(x + s / 2, y + s / 2, s * 0.6, 0, Math.PI * 2); c.fill(); },
      alpha(a) { c.globalAlpha = a; },
    };
  }

  /** Isi poligon piksel per baris (mode pixel). */
  function fillPoly(c, pts, col) {
    c.fillStyle = col;
    const ys = pts.map(p => p[1]);
    const y0 = Math.floor(Math.min(...ys)), y1 = Math.ceil(Math.max(...ys));
    for (let y = y0; y <= y1; y++) {
      const xs = [];
      for (let i = 0; i < pts.length; i++) {
        const [ax, ay] = pts[i], [bx, by] = pts[(i + 1) % pts.length];
        if ((ay <= y + 0.5 && by > y + 0.5) || (by <= y + 0.5 && ay > y + 0.5)) xs.push(ax + ((y + 0.5 - ay) / (by - ay)) * (bx - ax));
      }
      xs.sort((a, b) => a - b);
      for (let i = 0; i + 1 < xs.length; i += 2) c.fillRect(Math.round(xs[i]), y, Math.max(1, Math.round(xs[i + 1]) - Math.round(xs[i])), 1);
    }
  }

  /** Warna gradasi pada t (0..1) dari daftar [t, warna]. */
  function at(stops, t) {
    for (let i = 1; i < stops.length; i++) {
      if (t <= stops[i][0]) {
        const [t0, c0] = stops[i - 1], [t1, c1] = stops[i];
        return P.mix(c0, c1, t1 > t0 ? (t - t0) / (t1 - t0) : 0);
      }
    }
    return stops[stops.length - 1][1];
  }

  /** Bilangan acak tetap per indeks (posisi bintang, batu, dll. tidak berubah antar frame). */
  const rnd = (i, s) => { const x = Math.sin(i * 127.1 + (s || 0) * 311.7) * 43758.5453; return x - Math.floor(x); };

  // ---------- tema ----------
  // static(D, W, H, hz, world): lapisan diam (digambar sekali, disimpan); hz = garis tanah/cakrawala.
  // ambient(D, W, H, hz, world, t): lapisan bergerak (setiap frame), t dalam milidetik.

  const THEMES = {
    day: {
      label: 'Day',
      static(D, W, H, hz, world) {
        D.grad(0, 0, W, hz, [[0, '#fcd3e1'], [1, '#fff1f4']]);
        if (!world) return;
        for (let x = 30, k = 0; x < W; x += 58, k++) { // pohon lolipop
          const col = k % 2 ? '#dcefb8' : '#fbe7a6', r = 10 + (k % 3) * 2, cy = hz - 38 + (k % 2) * 8;
          D.rect(x - 1, cy + r - 2, 3, hz - (cy + r - 2), '#efcab9');
          D.circle(x, cy, r, P.mix(col, '#fde3eb', 0.35));
        }
        D.rect(0, hz, W, H - hz, '#fbe3e8');
        D.rect(0, hz, W, 1, '#f2c4d2');
        D.rect(0, hz + 1, W, 1, '#f7d3dd');
        for (let x = 3; x < W; x += 8) D.ellipse(x, H - 4, 2, 1, '#f5b3c7');
      },
      ambient(D, W, H, hz, world, t) { // awan berjalan pelan ke kanan
        const span = W + 60;
        for (let k = 0; k < Math.ceil(W / 64) + 1; k++) {
          const x = ((k * 64 + 20 + t * 0.006 * (1 + (k % 3) * 0.3)) % span) - 30;
          const y = 8 + ((k * 7 * 64) % 14);
          D.ellipse(x, y, 8, 3, '#fff8fb'); D.ellipse(x - 6, y + 1, 5, 2, '#fff8fb'); D.ellipse(x + 7, y + 1, 6, 2, '#fff8fb');
        }
      },
    },

    night: {
      label: 'Night',
      // langit senja keunguan (tidak terlalu gelap supaya teks pixel di panggung tetap terbaca)
      static(D, W, H, hz, world) {
        D.grad(0, 0, W, hz, [[0, '#7f83cf'], [0.6, '#a59fdc'], [1, '#c7bcec']]);
        D.circle(W - 26, 16, 9, '#fff4cf'); // bulan sabit
        D.circle(W - 22, 13, 8, '#9497d8');
        if (!world) return;
        for (let x = -10, k = 0; x < W + 20; x += 46, k++) D.ellipse(x, hz + 2, 30 + (k % 3) * 8, 12 + (k % 2) * 5, '#9a93cf'); // bukit
        D.rect(0, hz, W, H - hz, '#d6cff0');
        D.rect(0, hz, W, 1, '#aea5da');
        for (let x = 5; x < W; x += 11) D.ellipse(x, H - 4, 2, 1, '#b8afe0');
      },
      ambient(D, W, H, hz, world, t) {
        const n = Math.round((W * hz) / 260);
        for (let i = 0; i < n; i++) { // bintang berkelip
          const x = rnd(i, 1) * W, y = rnd(i, 2) * hz * 0.7;
          D.alpha(0.35 + 0.65 * Math.abs(Math.sin(t * 0.0018 + i * 1.7)));
          D.dot(x, y, rnd(i, 3) < 0.15 ? 2 : 1, '#ffffff');
        }
        D.alpha(1);
        const per = 6500, s = t % per; // bintang jatuh
        if (s < 700) {
          const k = Math.floor(t / per), p = s / 700;
          const x0 = 20 + rnd(k, 4) * (W - 60), y0 = 4 + rnd(k, 5) * hz * 0.3;
          for (let j = 0; j < 5; j++) { D.alpha((1 - p) * (1 - j / 5)); D.dot(x0 + p * 50 - j * 3, y0 + p * 22 - j * 1.3, 1, '#fffbe0'); }
          D.alpha(1);
        }
        if (world) for (let i = 0; i < 6; i++) { // kunang-kunang di dekat tanah
          const x = (rnd(i, 6) * W + Math.sin(t * 0.0007 + i) * 18 + W) % W, y = hz - 8 - rnd(i, 7) * 22 + Math.sin(t * 0.0011 + i * 2) * 4;
          D.alpha(0.3 + 0.7 * Math.max(0, Math.sin(t * 0.003 + i * 1.3)));
          D.dot(x, y, 1, '#fff27a');
        }
        D.alpha(1);
      },
    },

    cave: {
      label: 'Cave',
      torches: W => { const out = []; for (let x = 40; x < W - 20; x += 96) out.push(x); return out; },
      static(D, W, H, hz, world) {
        D.grad(0, 0, W, hz, [[0, '#a9918f'], [1, '#d7c2bb']]);
        for (let i = 0; i < Math.round((W * hz) / 500); i++) { // tekstur batu
          D.alpha(0.18); D.ellipse(rnd(i, 1) * W, rnd(i, 2) * hz, 3 + rnd(i, 3) * 4, 1 + rnd(i, 4) * 1.5, '#9c8380'); D.alpha(1);
        }
        for (let x = 6, k = 0; x < W; x += 22 + (k % 3) * 6, k++) { // stalaktit
          const len = 6 + rnd(k, 5) * 12;
          D.poly([[x - 4, 0], [x + 4, 0], [x, len]], '#8c7273');
        }
        for (const x of this.torches(W)) { // gagang obor
          const y = Math.round(hz * 0.38);
          D.rect(x - 1, y, 3, 9, '#6b4a3a');
          D.rect(x - 2, y - 1, 5, 2, '#4f3428');
        }
        if (!world) {
          for (let i = 0; i < 3; i++) crystal(D, (i + 0.5) * (W / 3) + rnd(i, 9) * 20, hz - 2, i);
          return;
        }
        for (let i = 0; i < Math.max(2, Math.round(W / 110)); i++) crystal(D, 14 + rnd(i, 8) * (W - 28), hz, i);
        D.rect(0, hz, W, H - hz, '#cbb8b0');
        D.rect(0, hz, W, 1, '#a99189');
        for (let x = 4; x < W; x += 9) D.ellipse(x, H - 4 - (x % 3), 2, 1, '#ae9a92');
      },
      ambient(D, W, H, hz, world, t) {
        const y = Math.round(hz * 0.38);
        this.torches(W).forEach((x, i) => { // obor berkedip
          const f = Math.sin(t * 0.023 + i * 2.1) * 0.6 + Math.sin(t * 0.051 + i) * 0.4;
          D.alpha(0.18 + 0.06 * f); D.circle(x, y - 4, 15 + f * 2, '#ffd27a'); D.alpha(1);
          D.ellipse(x, y - 4 - f * 0.5, 3 + f * 0.6, 5 + f, '#ff9f3f');
          D.ellipse(x, y - 3, 1.5, 3 + f * 0.5, '#fff1a8');
        });
        for (let k = 0; k < 4; k++) { // tetes air dari stalaktit
          const per = 2300 + k * 700, s = (t + k * 900) % per;
          const x = 6 + ((k * 61) % Math.max(1, W - 12)), yy = 10 + (s / per) * (hz - 10) * 1.1;
          if (yy < hz - 1) D.dot(x, yy, 1, '#86c9e3');
        }
        const sp = Math.floor(t / 400) % 7; // kilau kristal
        D.alpha(0.9); D.dot(14 + rnd(sp % 3, 8) * (W - 28) + 1, hz - 7, 1, '#ffffff'); D.alpha(1);
      },
    },

    beach: {
      label: 'Beach',
      static(D, W, H, hz, world) {
        const sea = world ? hz - 14 : Math.round(H * 0.82);
        D.grad(0, 0, W, sea, [[0, '#f48fb1'], [0.55, '#ffb59a'], [1, '#ffd9a0']]);
        D.circle(W * 0.68, sea, 13, '#ffe58f'); // matahari terbenam
        D.circle(W * 0.68, sea, 10, '#fff2b8');
        D.grad(0, sea, W, (world ? hz : H) - sea, [[0, '#5aaad8'], [1, '#3a7fb7']]);
        if (!world) return;
        palm(D, 18, hz);
        if (W > 220) palm(D, W - 22, hz, true);
        D.rect(0, hz, W, H - hz, '#f6dcae');
        D.rect(0, hz, W, 1, '#e8c38a');
        for (let x = 7; x < W; x += 15) D.ellipse(x, H - 4 - (x % 2), 2, 1, '#f1b8a8'); // kerang
      },
      ambient(D, W, H, hz, world, t) {
        const sea = world ? hz - 14 : Math.round(H * 0.82), bottom = world ? hz : H;
        for (let row = 0; row < 3; row++) { // ombak bergulir
          const y = sea + 3 + row * ((bottom - sea - 4) / 3);
          const off = (t * (0.01 + row * 0.006)) % 14;
          for (let x = -14 + off; x < W; x += 14) { D.alpha(0.55); D.rect(x, y, 5, 1, '#e8f6ff'); }
        }
        D.alpha(1);
        for (let k = 0; k < 4; k++) { // kilau matahari di laut
          D.alpha(0.4 + 0.4 * Math.abs(Math.sin(t * 0.004 + k)));
          D.rect(W * 0.68 - 6 + ((k * 5) % 12), sea + 2 + k * 2, 4 - (k % 2), 1, '#fff2b8');
        }
        D.alpha(1);
        for (let g = 0; g < 2; g++) { // camar terbang
          const span = W + 40, x = ((t * (0.012 + g * 0.004) + g * 90) % span) - 20, y = 12 + g * 9 + Math.sin(t * 0.003 + g) * 2;
          const wing = Math.sin(t * 0.012 + g) > 0 ? 1 : 0;
          D.rect(x - 3, y - wing, 3, 1, '#6b4f63'); D.rect(x + 1, y - wing, 3, 1, '#6b4f63'); D.rect(x, y, 1, 1, '#6b4f63');
        }
      },
    },
  };

  function crystal(D, x, base, i) {
    const col = i % 2 ? '#e7a6d8' : '#9fe3e0', h = 10 + (i % 3) * 3;
    D.poly([[x - 3, base], [x, base - h], [x + 3, base]], col);
    D.poly([[x + 2, base], [x + 5, base - h * 0.6], [x + 7, base]], P.mix(col, '#ffffff', 0.25));
  }

  function palm(D, x, hz, flip) {
    const s = flip ? -1 : 1;
    for (let k = 0; k < 7; k++) D.rect(x + s * Math.round(k * k * 0.12), hz - 6 - k * 6, 3, 7, '#a0703f');
    const tx = x + s * 6, ty = hz - 46;
    for (const [dx, dy] of [[-12, 4], [12, 4], [-8, -5], [9, -5], [0, -8]]) D.poly([[tx, ty], [tx + dx * s, ty + dy], [tx + dx * 0.5 * s, ty + dy + 4]], '#5fae6a');
    D.circle(tx, ty + 2, 2, '#7a5230');
  }

  // ---------- keadaan + cache ----------

  const st = { name: 'day', pixel: true, clock: 0 };
  const cache = new Map();

  function layer(spec, W, H, k, pixel) {
    const key = [st.name, pixel, spec.kind, spec.GY, W, H, k].join('|');
    let cv = cache.get(key);
    if (cv) return cv;
    if (cache.size > 40) cache.clear();
    cv = document.createElement('canvas');
    cv.width = Math.max(1, Math.round(W * k)); cv.height = Math.max(1, Math.round(H * k));
    const c = cv.getContext('2d');
    const D = pixel ? pixelDraw(c) : smoothDraw(c, k);
    const th = THEMES[st.name] || THEMES.day, world = spec.kind === 'world';
    th.static(D, W, H, world ? spec.GY : H, world);
    cache.set(key, cv);
    return cv;
  }

  /** Dipanggil panggung setiap frame menggantikan w.drawImage(bg, 0, 0). */
  function drawBg(w, bg) {
    const spec = bg.__theme;
    if (!spec) { w.drawImage(bg, 0, 0); return; }
    const W = bg.width, H = bg.height, world = spec.kind === 'world';
    w.canvas.__bgSpec = spec;
    if (st.pixel) {
      w.drawImage(layer(spec, W, H, 1, true), 0, 0);
      const th = THEMES[st.name] || THEMES.day;
      w.save();
      th.ambient(pixelDraw(w), W, H, world ? spec.GY : H, world, st.clock);
      w.restore();
    } else {
      w.clearRect(0, 0, W, H); // latar halus digambar P.present di bawah dunia
    }
    w.drawImage(bg, 0, 0);
  }

  /** Mode halus: latar beresolusi penuh di canvas panggung sebelum dunia ditempel. */
  function underlay(ctx, world, W, H, k) {
    const spec = world.__bgSpec;
    if (st.pixel || !spec) return false;
    const th = THEMES[st.name] || THEMES.day, isWorld = spec.kind === 'world';
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.drawImage(layer(spec, W, H, k, false), 0, 0);
    ctx.save();
    th.ambient(smoothDraw(ctx, k), W, H, isWorld ? spec.GY : H, isWorld, st.clock);
    ctx.restore();
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.imageSmoothingEnabled = false;
    return true;
  }

  G.theme = {
    NAMES,
    label: name => (THEMES[name] || THEMES.day).label,
    get name() { return st.name; },
    get pixel() { return st.pixel; },
    set(name, pixel) {
      st.name = THEMES[name] ? name : 'day';
      st.pixel = pixel !== false;
    },
    update(dt) { st.clock += dt; },
    drawBg, underlay,
  };
})(window.GV = window.GV || {});
