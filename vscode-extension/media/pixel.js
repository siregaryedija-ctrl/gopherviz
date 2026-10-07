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
  };
  const FONT = {};
  for (const k in GLYPHS) FONT[k] = GLYPHS[k].split('|');

  const textW = s => Math.max(0, s.length * 4 - 1);
  function text(c, s, x, y, col) {
    c.fillStyle = col;
    s = s.toUpperCase();
    for (let i = 0; i < s.length; i++) {
      const g = FONT[s[i]];
      if (!g) continue;
      for (let r = 0; r < 5; r++) for (let q = 0; q < 3; q++) if (g[r][q] === '#') c.fillRect(x + i * 4 + q, y + r, 1, 1);
    }
  }
  const textCenter = (c, s, cx, y, col) => text(c, s, Math.round(cx - textW(s) / 2), y, col);

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
    return (stamps[key] = c);
  }

  /** Latar dunia pastel: langit bergradasi berdither, awan, pohon lolipop, tanah berkerikil. */
  function paintWorld(c, W, H, GY) {
    const bands = ['#fcd3e1', '#fddbe6', '#fde3eb', '#feeaf0', '#fff1f4'];
    const bh = Math.ceil(GY / bands.length);
    bands.forEach((col, k) => rect(c, 0, k * bh, W, bh + 1, col));
    for (let k = 1; k < bands.length; k++) {
      c.fillStyle = bands[k - 1];
      for (let x = 0; x < W; x += 2) c.fillRect(x, k * bh, 1, 1);
      for (let x = 1; x < W; x += 4) c.fillRect(x, k * bh + 1, 1, 1);
    }
    for (let x = 20; x < W; x += 64) {
      const y = 8 + ((x * 7) % 14);
      ell(c, x, y, 8, 3, '#fff8fb'); ell(c, x - 6, y + 1, 5, 2, '#fff8fb'); ell(c, x + 7, y + 1, 6, 2, '#fff8fb');
    }
    for (let x = 30, k = 0; x < W; x += 58, k++) {
      const col = k % 2 ? '#dcefb8' : '#fbe7a6', r = 10 + (k % 3) * 2, cy = GY - 38 + (k % 2) * 8;
      rect(c, x - 1, cy + r - 2, 3, GY - (cy + r - 2), '#efcab9');
      disc(c, x, cy, r, mix(col, '#fde3eb', 0.35));
    }
    rect(c, 0, GY, W, H - GY, '#fbe3e8');
    rect(c, 0, GY, W, 1, '#f2c4d2');
    rect(c, 0, GY + 1, W, 1, '#f7d3dd');
    for (let x = 3; x < W; x += 8) ell(c, x, H - 4, 2, 1, '#f5b3c7');
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
    ctx.fillStyle = '#fde3eb';
    ctx.fillRect(0, 0, cw, ch);
    ctx.drawImage(world, 0, 0, worldW, worldH, 0, 0, cw, ch);
    return k;
  }

  /** Gopher biru berdiri dengan kaki di groundY. Mengembalikan y kepala. */
  function drawGopher(c, x, groundY, face, step, alpha) {
    const spr = gopher(2, face || 'idle', step || 0);
    c.globalAlpha = clamp(alpha == null ? 1 : alpha, 0, 1);
    c.drawImage(spr, Math.round(x - 8), Math.round(groundY + 2 - spr.height));
    c.globalAlpha = 1;
    return Math.round(groundY + 2 - spr.height);
  }

  /** Langit + awan saja (dunia yang tingginya bervariasi menggambar lantainya sendiri). */
  function paintSky(c, W, H) {
    const bands = ['#fcd3e1', '#fddbe6', '#fde3eb', '#feeaf0', '#fff1f4'];
    const bh = Math.ceil(H / bands.length);
    bands.forEach((col, k) => rect(c, 0, k * bh, W, bh + 1, col));
    for (let k = 1; k < bands.length; k++) {
      c.fillStyle = bands[k - 1];
      for (let x = 0; x < W; x += 2) c.fillRect(x, k * bh, 1, 1);
    }
    for (let x = 20; x < W; x += 64) {
      const y = 6 + ((x * 7) % 10);
      ell(c, x, y, 8, 3, '#fff8fb'); ell(c, x - 6, y + 1, 5, 2, '#fff8fb'); ell(c, x + 7, y + 1, 6, 2, '#fff8fb');
    }
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

  /** Bagian body mana (indeks) yang memuat baris line; -1 bila tidak ada. */
  function nodeAt(body, line) {
    let best = -1;
    (body || []).forEach((n, i) => { if (line >= n.line && line <= (n.endLine || n.line)) best = i; });
    return best;
  }

  G.pixel = {
    INK, VCOL, clamp, lerp, ease, bounce, mix, gopher, text, textW, textCenter,
    disc, ell, rect, box, dashedBox, dashedLine, bubble, stamp, paintWorld,
    along, present, drawGopher, paintSky, tile, nodeAt, NODE_COL,
  };
})(window.GV = window.GV || {});
