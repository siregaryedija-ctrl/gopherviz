// Mesin pixel bersama: sprite Gopher, font 3x5, dan bentuk dasar.
// Diambil dari prototype/bubble-sort.html. Semua fungsi menggambar dalam satuan piksel dunia.
(function (G) {
  'use strict';

  const INK = '#4a3550';
  const VCOL = { 1: '#f5a3bf', 2: '#8ec4f0', 3: '#f1d064', 4: '#9fd88f', 5: '#f7b779', 6: '#b8a2ee', 7: '#86d6cb', 8: '#f39a8a', 9: '#c9b3d8' };

  const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
  const lerp = (a, b, t) => a + (b - a) * t;
  const ease = t => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2);
  function bounce(t) {
    const n = 7.5625, d = 2.75;
    if (t < 1 / d) return n * t * t;
    if (t < 2 / d) return n * (t -= 1.5 / d) * t + 0.75;
    if (t < 2.5 / d) return n * (t -= 2.25 / d) * t + 0.9375;
    return n * (t -= 2.625 / d) * t + 0.984375;
  }
  function mix(a, b, f) {
    const x = parseInt(a.slice(1), 16), y = parseInt(b.slice(1), 16);
    const c = [16, 8, 0].map(s => Math.round(((x >> s) & 255) + ((((y >> s) & 255) - ((x >> s) & 255)) * f)));
    return '#' + c.map(v => v.toString(16).padStart(2, '0')).join('');
  }

  // ---------- sprite gopher ----------
  const HEAD = [
    '.oo..oooooo..oo.',
    'oeeoobbbbbbooeeo',
    'oeebbbbbbbbbbeeo',
    '.obwwwwbbwwwwbo.',
    '.obwwkwbbwkwwbo.',
    '.obwwkwbbwkwwbo.',
    '.obbwwbbbbwwbbo.',
    '.obpbbnnnnbbpbo.',
    '.obbbnnkknnbbbo.',
    '.obbbbnnnnbbbbo.',
    '.obbbbbttbbbbbo.',
  ];
  const FACES = {
    happy: ['.obbbbbbbbbbbbo.', '.obbkkbbbbkkbbo.', '.obkbbkbbkbbkbo.', '.obbbbbbbbbbbbo.'],
    blink: ['.obbbbbbbbbbbbo.', '.obbbbbbbbbbbbo.', '.obkkkkbbkkkkbo.', '.obbbbbbbbbbbbo.'],
    shock: ['.obwwwwbbwwwwbo.', '.obwwwwbbwwwwbo.', '.obwwwwbbwwwwbo.', '.obbwwbbbbwwbbo.'],
    think: ['.obwwkwbbwkwwbo.', '.obwwwwbbwwwwbo.', '.obwwwwbbwwwwbo.', '.obbwwbbbbwwbbo.'],
  };
  const BODY = '.olbbbbbbbbbbbo.';
  const ARMS = 'onlbbbbbbbbbbbno';
  const FEET = [
    ['..olbbbbbbbbbo..', '..onnno..onnno..', '...ooo....ooo...'],
    ['..olbbbbbbbbbo..', '...onnnoonnno...', '....ooo..ooo....'], // langkah kaki
  ];
  const cache = new Map();

  /** Sprite gopher 16 piksel lebar; tinggi bertambah 2 piksel per nilai v. */
  function gopher(v, face, step) {
    const key = v + face + (step || 0);
    if (cache.has(key)) return cache.get(key);
    const head = HEAD.slice();
    if (FACES[face]) FACES[face].forEach((r, k) => { head[3 + k] = r; });
    const body = [];
    for (let r = 0; r < 2 + v * 2; r++) body.push(r === 1 ? ARMS : BODY);
    const rows = [...head, ...body, ...FEET[step ? 1 : 0]];
    const base = VCOL[v] || VCOL[2];
    const pal = { o: INK, b: base, l: mix(base, '#ffffff', 0.45), e: mix(base, INK, 0.25), n: '#f7ddb9', k: '#2a1f30', w: '#ffffff', t: '#ffffff', p: '#ff8fae' };
    const c = document.createElement('canvas');
    c.width = 16; c.height = rows.length;
    const x = c.getContext('2d');
    rows.forEach((row, y) => {
      for (let i = 0; i < 16; i++) {
        const ch = row[i];
        if (!ch || ch === '.') continue;
        x.fillStyle = pal[ch]; x.fillRect(i, y, 1, 1);
      }
    });
    cache.set(key, c);
    return c;
  }

  // ---------- font 3x5 ----------
  const GLYPHS = {
    'A': '.#.|#.#|###|#.#|#.#', 'B': '##.|#.#|##.|#.#|##.', 'C': '.##|#..|#..|#..|.##', 'D': '##.|#.#|#.#|#.#|##.',
    'E': '###|#..|##.|#..|###', 'F': '###|#..|##.|#..|#..', 'G': '.##|#..|#.#|#.#|.##', 'H': '#.#|#.#|###|#.#|#.#',
    'I': '###|.#.|.#.|.#.|###', 'J': '..#|..#|..#|#.#|.#.', 'K': '#.#|#.#|##.|#.#|#.#', 'L': '#..|#..|#..|#..|###',
    'M': '#.#|###|###|#.#|#.#', 'N': '##.|#.#|#.#|#.#|#.#', 'O': '.#.|#.#|#.#|#.#|.#.', 'P': '##.|#.#|##.|#..|#..',
    'Q': '.#.|#.#|#.#|##.|.##', 'R': '##.|#.#|##.|#.#|#.#', 'S': '.##|#..|.#.|..#|##.', 'T': '###|.#.|.#.|.#.|.#.',
    'U': '#.#|#.#|#.#|#.#|###', 'V': '#.#|#.#|#.#|#.#|.#.', 'W': '#.#|#.#|###|###|#.#', 'X': '#.#|#.#|.#.|#.#|#.#',
    'Y': '#.#|#.#|.#.|.#.|.#.', 'Z': '###|..#|.#.|#..|###',
    '0': '###|#.#|#.#|#.#|###', '1': '.#.|##.|.#.|.#.|###', '2': '###|..#|###|#..|###', '3': '###|..#|.##|..#|###',
    '4': '#.#|#.#|###|..#|..#', '5': '###|#..|###|..#|###', '6': '###|#..|###|#.#|###', '7': '###|..#|..#|.#.|.#.',
    '8': '###|#.#|###|#.#|###', '9': '###|#.#|###|..#|###',
    '!': '.#.|.#.|.#.|...|.#.', '?': '##.|..#|.#.|...|.#.', '<': '..#|.#.|#..|.#.|..#', '>': '#..|.#.|..#|.#.|#..',
    '=': '...|###|...|###|...', '+': '...|.#.|###|.#.|...', '-': '...|...|###|...|...', '.': '...|...|...|...|.#.',
    ':': '...|.#.|...|.#.|...', '/': '..#|..#|.#.|#..|#..', '(': '.#.|#..|#..|#..|.#.', ')': '.#.|..#|..#|..#|.#.',
    '[': '##.|#..|#..|#..|##.', ']': '.##|..#|..#|..#|.##', ' ': '...|...|...|...|...',
    '✓': '...|..#|#.#|.#.|...', '✗': '#.#|.#.|#.#|...|...', ',': '...|...|...|.#.|#..',
    '"': '#.#|#.#|...|...|...', "'": '.#.|.#.|...|...|...', '%': '#.#|..#|.#.|#..|#.#', '*': '...|#.#|.#.|#.#|...',
    '_': '...|...|...|...|###', '#': '#.#|###|#.#|###|#.#', '&': '.#.|#.#|.#.|#.#|.##',
    '{': '.##|.#.|##.|.#.|.##', '}': '##.|.#.|.##|.#.|##.', ';': '...|.#.|...|.#.|#..', '|': '.#.|.#.|.#.|.#.|.#.',
    '$': '.##|##.|.#.|.##|##.', '^': '.#.|#.#|...|...|...', '@': '###|#.#|###|#..|.##',
  };
  const FONT = {};
  for (const k in GLYPHS) FONT[k] = GLYPHS[k].split('|');

  const textW = s => Math.max(0, s.length * 4 - 1);
  function text(c, s, x, y, col) {
    if (defer(c, s, x, y, col, 1)) return;
    pixelText(c, s, x, y, col);
  }
  function pixelText(c, s, x, y, col) {
    c.fillStyle = col;
    s = s.toUpperCase();
    for (let i = 0; i < s.length; i++) {
      const g = FONT[s[i]];
      if (!g) continue;
      for (let r = 0; r < 5; r++) for (let q = 0; q < 3; q++) if (g[r][q] === '#') c.fillRect(x + i * 4 + q, y + r, 1, 1);
    }
  }
  const textCenter = (c, s, cx, y, col) => text(c, s, Math.round(cx - textW(s) / 2), y, col);
  /** Teks satu baris (huruf besar/kecil dipertahankan), dipotong ".." bila lebih dari max karakter. */
  function fit(s, max) {
    s = [...String(s)].map(ch => (FONT[ch.toUpperCase()] || ch > ' ' ? ch : ch === '\t' ? ' ' : '')).join('').replace(/\s+/g, ' ').trim();
    return max != null && s.length > max ? s.slice(0, Math.max(1, max - 2)) + '..' : s;
  }

  // ---------- bentuk ----------
  function disc(c, cx, cy, r, col) {
    c.fillStyle = col;
    for (let dy = -r; dy <= r; dy++) {
      const w = Math.floor(Math.sqrt(Math.max(0, r * r + r * 0.6 - dy * dy)));
      c.fillRect(cx - w, cy + dy, w * 2 + 1, 1);
    }
  }
  function ell(c, cx, cy, rx, ry, col) {
    c.fillStyle = col;
    for (let dy = -ry; dy <= ry; dy++) {
      const w = Math.round(rx * Math.sqrt(Math.max(0, 1 - (dy * dy) / ((ry + 0.5) * (ry + 0.5)))));
      c.fillRect(cx - w, cy + dy, w * 2 + 1, 1);
    }
  }
  function rect(c, x, y, w, h, col) { c.fillStyle = col; c.fillRect(x, y, w, h); }
  /** Kotak berbingkai 1 piksel dengan sudut terpotong. */
  function box(c, x, y, w, h, fill, line) {
    rect(c, x + 1, y, w - 2, h, line); rect(c, x, y + 1, w, h - 2, line);
    rect(c, x + 1, y + 1, w - 2, h - 2, fill);
  }
  /** Bingkai putus-putus untuk blok "hantu" (kode belum lengkap). */
  function dashedBox(c, x, y, w, h, col) {
    c.fillStyle = col;
    for (let i = 0; i < w; i += 2) { c.fillRect(x + i, y, 1, 1); c.fillRect(x + i, y + h - 1, 1, 1); }
    for (let j = 0; j < h; j += 2) { c.fillRect(x, y + j, 1, 1); c.fillRect(x + w - 1, y + j, 1, 1); }
  }

  /** Kedalaman sisi atas/kanan kubus untuk sisi depan selebar s piksel (0 = kotak datar). */
  function cubeDepth(s) { return s < 4 ? 0 : s < 6 ? 1 : s < 12 ? 2 : clamp(Math.round(s / 4), 3, 8); }

  /**
   * Kubus pixel art (proyeksi miring, keputusan user 2026-10-08: balok = kubus, bukan batang
   * berdiri). Sisi depan w x h berpojok kiri atas (x, y) berwarna col; sisi atas terang setebal d
   * di atasnya dan sisi kanan gelap di kanannya. ring = warna bingkai sorotan (opsional).
   */
  function cube(c, x, y, w, h, col, d, ring) {
    x = Math.round(x); y = Math.round(y); w = Math.max(1, Math.round(w)); h = Math.max(1, Math.round(h));
    d = d == null ? cubeDepth(Math.min(w, h)) : Math.max(0, Math.round(d));
    if (ring) { // siluet satu piksel lebih besar di belakang kubus
      rect(c, x - 1, y - 1, w + 2, h + 2, ring);
      for (let k = 1; k <= d; k++) { rect(c, x - 1 + k, y - 1 - k, w + 2, 1, ring); rect(c, x + w + k, y - 1 - k, 1, h + 2, ring); }
    }
    const top = mix(col, '#ffffff', 0.45), side = mix(col, INK, 0.3);
    for (let k = 1; k <= d; k++) {
      rect(c, x + w - 1 + k, y - k, 1, h, side);
      rect(c, x + k, y - k, w, 1, top);
    }
    rect(c, x, y, w, h, col);
    if (w >= 5 && h >= 5) {
      rect(c, x, y + h - 1, w, 1, mix(col, INK, 0.45));
      rect(c, x, y, 1, h - 1, mix(col, '#ffffff', 0.25));
    }
  }

  function dashedLine(c, x0, x1, y, col) {
    c.fillStyle = col;
    for (let x = x0; x <= x1; x += 3) c.fillRect(x, y, 2, 1);
  }

  /** Balon ucapan 11x11 berisi satu karakter, ekornya menunjuk ke bawah di (x, y+13). */
  function bubble(c, x, y, ch, col) {
    const w = 11, h = 11, L = Math.round(x - w / 2), T = Math.round(y), cx = Math.round(x);
    rect(c, L + 1, T, w - 2, h, INK); rect(c, L, T + 1, w, h - 2, INK);
    rect(c, L + 1, T + 1, w - 2, h - 2, '#ffffff');
    rect(c, cx - 1, T + h, 1, 1, INK); rect(c, cx + 1, T + h, 1, 1, INK); rect(c, cx, T + h + 1, 1, 1, INK);
    rect(c, cx, T + h - 1, 1, 2, '#ffffff');
    text(c, ch, cx - 1, T + 3, col);
  }

  const stamps = {};
  /** Stempel teks berbingkai (BREAK!, SELESAI!), digambar dengan skala bilangan bulat. */
  function stamp(label, col) {
    const key = label + col;
    if (stamps[key]) return stamps[key];
    const w = textW(label) + 8, h = 13, c = document.createElement('canvas');
    c.width = w; c.height = h;
    const x = c.getContext('2d');
    rect(x, 0, 0, w, h, col); rect(x, 1, 1, w - 2, h - 2, '#fff8fb');
    rect(x, 2, 2, w - 4, 1, col); rect(x, 2, h - 3, w - 4, 1, col);
    text(x, label, 4, 4, col);
    c.__stamp = { label, col }; // drawImage ke canvas yang mendukung teks halus: label ditulis ulang halus
    return (stamps[key] = c);
  }

  /**
   * Latar dunia (langit + tanah di garis GY) dan latar langit saja. Gambarnya milik tema aktif
   * (theme.js, Fase 13): di sini canvas latar panggung hanya ditandai jenisnya dan dikosongkan,
   * panggung boleh menggambar isinya sendiri di atasnya (lantai, kotak). P.drawBg menggabungkan
   * tema + isi itu setiap frame.
   */
  function paintWorld(c, W, H, GY) {
    c.clearRect(0, 0, W, H);
    c.canvas.__theme = { kind: 'world', GY };
    c.canvas.__texts = [];
  }

  /** Gambar latar panggung setiap frame: tema (bergerak) di bawah isi canvas latar. */
  function drawBg(w, bg) {
    const dst = w.canvas.__texts;
    if (dst) dst.length = 0; // latar = awal frame: teks frame sebelumnya yang belum dipresent dibuang
    if (G.theme) G.theme.drawBg(w, bg);
    else w.drawImage(bg, 0, 0);
    if (dst && bg.__texts) for (const t of bg.__texts) dst.push(t); // teks latar (sudah di koordinat dunia)
  }

  /** Posisi pada jalur berbelok (daftar titik) di t = 0..1, menurut panjang lintasan. */
  function along(points, t) {
    if (points.length === 1) return { x: points[0].x, y: points[0].y };
    const segs = [];
    let total = 0;
    for (let i = 1; i < points.length; i++) {
      const d = Math.hypot(points[i].x - points[i - 1].x, points[i].y - points[i - 1].y);
      segs.push(d);
      total += d;
    }
    let dist = clamp(t, 0, 1) * total;
    for (let i = 0; i < segs.length; i++) {
      if (dist <= segs[i] || i === segs.length - 1) {
        const u = segs[i] ? Math.min(1, dist / segs[i]) : 1;
        return { x: lerp(points[i].x, points[i + 1].x, u), y: lerp(points[i].y, points[i + 1].y, u) };
      }
      dist -= segs[i];
    }
    return points[points.length - 1];
  }

  /**
   * Menampilkan SELURUH dunia (worldW x worldH piksel dunia) di canvas, tanpa kamera yang menggeser
   * (keputusan user 2026-10-07: seluruh struktur harus terlihat). Ukuran canvas mengikuti bentuk
   * dunia dengan skala bulat k supaya piksel tetap tajam; CSS (.stage canvas) lalu memperkecilnya
   * agar muat selebar panel dan setinggi layar. canvas.__k = piksel canvas per piksel dunia.
   */
  function present(ctx, canvas, world, worldW, worldH) {
    let k = Math.max(2, Math.ceil(1536 / worldW));
    k = Math.max(1, Math.min(k, Math.floor(Math.sqrt(8e6 / (worldW * worldH)))));
    const cw = worldW * k, ch = worldH * k;
    if (canvas.width !== cw || canvas.height !== ch) {
      canvas.width = cw;
      canvas.height = ch;
    }
    canvas.__k = k;
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.imageSmoothingEnabled = false;
    // tema mode halus: latar beresolusi penuh di bawah dunia (dunia transparan di bagian latar)
    if (!(G.theme && G.theme.underlay(ctx, world, worldW, worldH, k))) {
      ctx.fillStyle = '#fde3eb';
      ctx.fillRect(0, 0, cw, ch);
    }
    ctx.drawImage(world, 0, 0, worldW, worldH, 0, 0, cw, ch);
    flushTexts(ctx, world, k, 0, 0, 1);
    return k;
  }

  // ---------- teks halus ----------
  // Keputusan user 2026-10-12: tulisan di panggung jangan terlalu pixel supaya mudah dibaca. Kubus, Gopher, dan
  // latar tetap pixel art; teks yang ditulis ke canvas dunia dicatat (posisi, warna, alpha, transform) lalu
  // digambar dengan font monospace beresolusi penuh oleh present(), sesudah dunia diperbesar. Lebar satu huruf
  // monospace (~0.55em) hampir sama dengan lebar huruf pixel (4 piksel dunia), jadi tata letak tidak berubah.
  // Canvas lain (cache sprite kecil) tetap memakai font pixel.
  const MONO = 'Consolas, "Cascadia Mono", "SF Mono", Menlo, "DejaVu Sans Mono", monospace';

  /** Catat teks untuk lapisan halus bila canvas c mendukungnya (dunia panggung atau latarnya). */
  function defer(c, s, x, y, col, size) {
    const list = c.canvas && c.canvas.__texts;
    if (!list) return false;
    const m = c.getTransform();
    list.push({ s: String(s), x, y, col, size, a: c.globalAlpha, m, box: devBox(m, x, y, textW(String(s)) * size * 1.06, 5 * size) });
    return true;
  }

  /** Kotak (x, y, w, h) di transform m (tanpa rotasi) dalam koordinat canvas: [x0, y0, x1, y1]. */
  function devBox(m, x, y, w, h) {
    const x0 = m.a * x + m.c * y + m.e, y0 = m.b * x + m.d * y + m.f, x1 = x0 + m.a * w, y1 = y0 + m.d * h;
    return [Math.min(x0, x1), Math.min(y0, y1), Math.max(x0, x1), Math.max(y0, y1)];
  }

  // Urutan gambar: teks halus digambar paling akhir, jadi kotak atau sprite yang digambar SESUDAH sebuah teks
  // dan menimpanya (tiket yang lewat, Gopher, lapisan peredup) dicatat sebagai penutup teks itu. Saat teks
  // ditulis, bagian yang tertutup dihapus (sprite: sesuai bentuk pikselnya), seolah teks ada di bawahnya.
  function occlude(c, x, y, w, h, img, src) {
    const list = c.canvas.__texts;
    if (!list || !list.length || !(Math.abs(w) >= 1 && Math.abs(h) >= 1)) return;
    const b = devBox(c.getTransform(), x, y, w, h);
    for (const t of list) {
      const q = t.box;
      if (!q || q[0] >= b[2] || b[0] >= q[2] || q[1] >= b[3] || b[1] >= q[3]) continue;
      (t.occ || (t.occ = [])).push({ b, img, src, a: c.globalAlpha });
    }
  }
  const rawFillRect = CanvasRenderingContext2D.prototype.fillRect;
  CanvasRenderingContext2D.prototype.fillRect = function (x, y, w, h) {
    if (this.canvas.__texts) occlude(this, x, y, w, h);
    return rawFillRect.call(this, x, y, w, h);
  };

  /** Teks yang sebagian tertutup: tulis di canvas sementara, hapus bagian penutupnya, lalu tempel. */
  let tmpCv = null;
  function smoothOccluded(ctx, t, k, ox, oy, alpha) {
    const q = t.box, M = 3, x0 = Math.floor(q[0] - M), y0 = Math.floor(q[1] - M);
    const w = Math.ceil((q[2] - q[0] + 2 * M) * k), h = Math.ceil((q[3] - q[1] + 2 * M) * k);
    if (!tmpCv) tmpCv = document.createElement('canvas');
    if (tmpCv.width < w) tmpCv.width = w;
    if (tmpCv.height < h) tmpCv.height = h;
    const x = tmpCv.getContext('2d');
    x.setTransform(1, 0, 0, 1, 0, 0);
    x.globalCompositeOperation = 'source-over';
    x.clearRect(0, 0, w, h);
    smooth(x, t, k, -x0 * k, -y0 * k, 1);
    x.globalCompositeOperation = 'destination-out';
    x.imageSmoothingEnabled = false;
    x.setTransform(k, 0, 0, k, -x0 * k, -y0 * k);
    for (const o of t.occ) {
      x.globalAlpha = o.a;
      const [bx0, by0, bx1, by1] = o.b;
      if (o.img) rawDrawImage.call(x, o.img, ...o.src, bx0, by0, bx1 - bx0, by1 - by0);
      else x.fillRect(bx0, by0, bx1 - bx0, by1 - by0);
    }
    x.globalCompositeOperation = 'source-over';
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.globalAlpha = alpha;
    rawDrawImage.call(ctx, tmpCv, 0, 0, w, h, ox + x0 * k, oy + y0 * k, w, h);
  }

  /** Gambar satu teks halus: posisi dunia (x, y = atas huruf) di transform m, diperbesar k, digeser (ox, oy). */
  function smooth(ctx, t, k, ox, oy, alpha) {
    const m = t.m;
    ctx.setTransform(m.a * k, m.b * k, m.c * k, m.d * k, ox + m.e * k, oy + m.f * k);
    ctx.globalAlpha = t.a * alpha;
    ctx.fillStyle = t.col;
    ctx.font = `bold ${7.4 * t.size}px ${MONO}`;
    const want = textW(t.s) * t.size * 1.06, got = ctx.measureText(t.s).width;
    ctx.translate(t.x, t.y + 5 * t.size);
    if (got > want && got > 0) ctx.scale(want / got, 1); // teks yang lebih lebar dari kotaknya dipadatkan
    ctx.fillText(t.s, 0, 0);
  }

  /** Teks halus t di transform ctx sekarang (untuk perekam). */
  function drawText(ctx, t) {
    ctx.save();
    smooth(ctx, Object.assign({}, t, { m: ctx.getTransform() }), 1, 0, 0, 1);
    ctx.restore();
  }

  /** Tulis semua teks halus milik canvas dunia src ke ctx (dunia digambar dengan skala k di (ox, oy)). */
  function flushTexts(ctx, src, k, ox, oy, alpha) {
    const list = src.__texts;
    src.__texts = [];
    if (!list || !list.length) return;
    ctx.save();
    for (const t of list) {
      if (t.occ) smoothOccluded(ctx, t, k, ox, oy, alpha);
      else smooth(ctx, t, k, ox, oy, alpha);
    }
    ctx.restore();
  }

  // Stempel (DONE!, PANIC!) digambar dengan drawImage: di canvas yang mendukung teks halus, labelnya ditulis
  // ulang halus di atas stempel (stempel memakai bingkai pixel yang sama).
  const rawDrawImage = CanvasRenderingContext2D.prototype.drawImage;
  CanvasRenderingContext2D.prototype.drawImage = function (img, ...a) {
    if (this.canvas.__texts && a.length >= 2) {
      const n = a.length, src = n === 8 ? a.slice(0, 4) : [0, 0, img.width, img.height];
      occlude(this, n === 8 ? a[4] : a[0], n === 8 ? a[5] : a[1], n === 8 ? a[6] : n === 4 ? a[2] : img.width, n === 8 ? a[7] : n === 4 ? a[3] : img.height, img, src);
    }
    const st = img && img.__stamp;
    const hiRes = st && !this.canvas.__texts && this.canvas.__k; // canvas panggung (sudah diperbesar)
    if (!st || (!this.canvas.__texts && !hiRes)) return rawDrawImage.call(this, img, ...a);
    const dx = a.length === 8 ? a[4] : a[0], dy = a.length === 8 ? a[5] : a[1];
    const dw = a.length === 8 ? a[6] : a.length === 4 ? a[2] : img.width;
    rawDrawImage.call(this, plainStamp(img), ...a);
    const sc = dw / img.width;
    const t = { s: st.label, x: dx + 4 * sc, y: dy + 4 * sc, col: st.col, size: sc, a: this.globalAlpha, m: this.getTransform() };
    if (this.canvas.__texts) this.canvas.__texts.push(t);
    else { this.save(); smooth(this, Object.assign(t, { m: new DOMMatrix() }), 1, 0, 0, 1); this.restore(); }
  };
  /** Stempel tanpa tulisan (bingkai saja) untuk ditimpa teks halus. */
  function plainStamp(img) {
    if (img.__plain) return img.__plain;
    const c = document.createElement('canvas');
    c.width = img.width; c.height = img.height;
    const x = c.getContext('2d'), col = img.__stamp.col, w = img.width, h = img.height;
    rect(x, 0, 0, w, h, col); rect(x, 1, 1, w - 2, h - 2, '#fff8fb');
    rect(x, 2, 2, w - 4, 1, col); rect(x, 2, h - 3, w - 4, 1, col);
    return (img.__plain = c);
  }

  /** Aktifkan teks halus untuk canvas (dunia panggung yang dipresent, adegan error). */
  const smoothText = cv => { if (!cv.__texts) cv.__texts = []; return cv; };

  /** Gopher biru berdiri dengan kaki di groundY. Mengembalikan y kepala. */
  function drawGopher(c, x, groundY, face, step, alpha) {
    const spr = gopher(2, face || 'idle', step || 0);
    c.globalAlpha = clamp(alpha == null ? 1 : alpha, 0, 1);
    c.drawImage(spr, Math.round(x - 8), Math.round(groundY + 2 - spr.height));
    c.globalAlpha = 1;
    return Math.round(groundY + 2 - spr.height);
  }

  /** Langit saja (dunia yang tingginya bervariasi menggambar lantainya sendiri). */
  function paintSky(c, W, H) {
    c.clearRect(0, 0, W, H);
    c.canvas.__theme = { kind: 'sky' };
    c.canvas.__texts = [];
  }

  // Warna ubin per jenis bagian body, sama dengan ikon di daftar "Isi" panel.
  const NODE_COL = {
    stmt: '#f3c75a', if: '#86d6cb', switch: '#86d6cb', loop: '#b8a2ee', break: '#7cc68b', continue: '#f7b779',
    return: '#f39a8a', goto: '#c9b3d8', block: '#e9a28d', bad: '#f6eef2',
  };

  /** Ubin bagian body (12x9) bernomor; bump = 0..1 untuk animasi disundul. */
  function tile(c, x, groundY, kind, label, bump, dim) {
    const col = NODE_COL[kind] || NODE_COL.stmt;
    const y = groundY - 9 - Math.round(Math.sin(Math.PI * clamp(bump || 0, 0, 1)) * 3);
    c.globalAlpha = dim ? 0.35 : 1;
    box(c, Math.round(x - 6), y, 12, 9, col, INK);
    rect(c, Math.round(x - 5), y + 1, 10, 1, mix(col, '#ffffff', 0.5));
    if (label) textCenter(c, label, x, y + 2, INK);
    c.globalAlpha = 1;
  }

  const KEYWORD = { 'break': 'BREAK', 'continue': 'CONTINUE', 'return': 'RETURN', 'goto': 'GOTO' };

  /**
   * Label ubin isi blok (cabang if, fungsi) = nama perintahnya (keputusan user 2026-10-08: nomor 1, 2 tidak jelas):
   * kata kunci (BREAK, RETURN), variabel yang diubah (X=, COUNT+=), atau fungsi yang dipanggil (PRINT).
   */
  function stmtLabel(n) {
    if (KEYWORD[n.kind]) return KEYWORD[n.kind];
    const t = (n.text || '').trim();
    // self.nilai += n / this.saldo = x / $this->saldo -= 1: label = field yang diubah (NILAI+=)
    let m = t.match(/^(?:(?:let|const|var|auto|int|long|float|double|string|bool|char)\s+)?\$?([A-Za-z_]\w*(?:(?:\.|->)\w+)*)(\[[^\]]*\])?\s*(\+\+|--|:=|[-+*/%]?=)(?!=)/);
    if (m) return fit(m[1].split(/\.|->/).pop() + (m[2] ? '[]' : '') + m[3], 9);
    m = t.match(/^(?:await\s+)?\$?([A-Za-z_][\w.]*)\s*\(/);
    if (m) return fit(m[1].split('.').pop(), 9);
    m = t.match(/^[A-Za-z_]\w*/);
    return fit(m ? m[0] : n.kind, 9) || '...';
  }

  /** Ubin berlabel selebar labelnya, dijejer mulai x0: [{ x, w, cx, label }]. */
  function tileRow(body, x0) {
    let x = x0;
    return (body || []).map(n => {
      const label = stmtLabel(n), w = Math.max(12, textW(label) + 6);
      const t = { x, w, cx: x + w / 2, label };
      x += w + 3;
      return t;
    });
  }

  /** Menggambar satu ubin berlabel (hasil tileRow) berdiri di groundY; bump = 0..1 disundul. */
  function labelTile(c, t, groundY, kind, bump, dim) {
    const col = NODE_COL[kind] || NODE_COL.stmt;
    const y = groundY - 9 - Math.round(Math.sin(Math.PI * clamp(bump || 0, 0, 1)) * 3);
    c.globalAlpha = dim ? 0.35 : 1;
    box(c, t.x, y, t.w, 9, col, INK);
    rect(c, t.x + 1, y + 1, t.w - 2, 1, mix(col, '#ffffff', 0.5));
    textCenter(c, t.label, t.cx, y + 2, INK);
    c.globalAlpha = 1;
  }

  // ---------- tembok penghitung ----------

  /** Label pendek untuk balok tembok (nilai variabel), atau null bila terlalu panjang / tidak bisa digambar. */
  function wallLabel(v) {
    if (v == null) return null;
    const s = String(v).trim().replace(/^(["'`])(.*)\1$/, '$2').toUpperCase();
    return s && s.length <= 3 && [...s].every(ch => FONT[ch]) ? s : null;
  }

  /**
   * Tata letak tembok penghitung: satu kubus per putaran loop / per panggilan rekursif, dijejer lalu
   * pindah baris saat penuh. Supaya semuanya tetap terlihat, kubus mengecil bila jumlahnya banyak:
   * berlabel (nilai variabel) bila sedikit, lalu kubus polos 7px, lalu 4px.
   * n = jumlah kubus terbanyak; labels = label terlebar yang mungkin muncul; more = trace terpotong.
   */
  function wallLayout(n, labels, W, more) {
    const slots = n + (more ? 1 : 0), usable = W - 16;
    const lw = Math.max(7, ...labels.map(textW));
    const sizes = [
      { cw: Math.max(9, lw + 4), label: true, maxRows: 3 },
      { cw: 7, label: false, maxRows: 4 },
      { cw: 4, label: false, maxRows: Infinity },
    ];
    for (const s of sizes) {
      const d = cubeDepth(s.cw), pitch = s.cw + d + (s.cw >= 7 ? 1 : 0);
      const perRow = Math.max(1, Math.floor((usable - d) / pitch));
      const rows = Math.max(1, Math.ceil(slots / perRow));
      if (rows <= s.maxRows) return Object.assign(s, { ch: s.cw, d, pitch, perRow, rows, x0: 8, h: rows * pitch + 7 });
    }
  }

  /**
   * Menggambar count balok pertama dari tembok di y0. Balok terakhir jatuh masuk bila pop (0..1) diberikan;
   * ghost = gambar balok hantu "+" sesudahnya (trace dipotong, putaran berikutnya tidak disimulasikan).
   */
  function drawWall(c, L, y0, labels, count, pop, ghost) {
    const at = k => ({ x: L.x0 + (k % L.perRow) * L.pitch, y: y0 + 4 + L.d + Math.floor(k / L.perRow) * L.pitch });
    for (let k = 0; k < count; k++) {
      let { x, y } = at(k);
      if (k === count - 1 && pop != null && pop < 1) {
        y -= Math.round((1 - bounce(clamp(pop, 0, 1))) * 10);
        c.globalAlpha = clamp(pop * 3, 0, 1);
      }
      // setiap kubus ke-10 berwarna jingga supaya mudah dihitung
      const fill = k % 10 === 9 ? '#f7b779' : '#f3c75a';
      cube(c, x, y, L.cw, L.ch, fill, L.d);
      if (L.label) textCenter(c, labels[k] || String(k + 1), x + L.cw / 2, y + Math.floor((L.ch - 5) / 2), INK);
      c.globalAlpha = 1;
    }
    if (ghost) {
      const { x, y } = at(count);
      dashedBox(c, x, y, L.cw, L.ch, '#b07a92');
      if (L.ch >= 7) textCenter(c, '+', x + L.cw / 2, y + Math.floor((L.ch - 5) / 2), '#b07a92');
    }
  }

  /** Bagian body mana (indeks) yang memuat baris line; -1 bila tidak ada. */
  function nodeAt(body, line) {
    let best = -1;
    (body || []).forEach((n, i) => { if (line >= n.line && line <= (n.endLine || n.line)) best = i; });
    return best;
  }

  G.pixel = {
    INK, VCOL, clamp, lerp, ease, bounce, mix, gopher, text, textW, textCenter, fit, smoothText, flushTexts, drawText,
    disc, ell, rect, box, dashedBox, dashedLine, bubble, stamp, paintWorld,
    along, present, drawGopher, paintSky, drawBg, tile, nodeAt, NODE_COL, wallLabel, wallLayout, drawWall, cube, cubeDepth, stmtLabel, tileRow, labelTile,
  };
})(window.GV = window.GV || {});
