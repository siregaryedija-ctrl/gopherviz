// Ekspor animasi (Fase 12a, keputusan user 2026-10-09): GIF (encoder sendiri, tanpa pustaka luar),
// WebM (MediaRecorder), dan MP4 (WebM diubah ffmpeg oleh ekstensi). Bingkai "panggung saja" atau
// "reel" vertikal 9:16: judul + panggung + potongan kode dengan baris aktif menyala + output.
(function (G) {
  'use strict';
  const P = G.pixel;

  // ---------- encoder GIF ----------

  /**
   * GifWriter: setiap frame memakai tabel warna lokal (palet bertambah sepanjang rekaman, maks 255
   * warna + 1 transparan). Piksel yang sama dengan frame sebelumnya ditulis transparan (disposal 1)
   * sehingga file kecil untuk animasi pixel art.
   */
  class GifWriter {
    constructor(w, h) {
      this.w = w; this.h = h;
      this.out = [];
      this.pal = [];           // warna RGB (angka 0xRRGGBB), indeks 0..254
      this.idx = new Map();    // warna → indeks
      this.near = new Map();   // warna di luar palet → indeks terdekat
      this.prev = null;        // indeks frame sebelumnya (untuk transparansi)
      this.bytes('GIF89a');
      this.u16(w); this.u16(h);
      this.out.push(0x00, 0, 0); // tanpa tabel warna global
      // ulang terus (NETSCAPE2.0)
      this.out.push(0x21, 0xff, 0x0b);
      this.bytes('NETSCAPE2.0');
      this.out.push(0x03, 0x01, 0x00, 0x00, 0x00);
    }

    bytes(s) { for (let i = 0; i < s.length; i++) this.out.push(s.charCodeAt(i)); }
    u16(n) { this.out.push(n & 255, (n >> 8) & 255); }

    colorIndex(c) {
      let k = this.idx.get(c);
      if (k != null) return k;
      if (this.pal.length < 255) {
        k = this.pal.length;
        this.pal.push(c);
        this.idx.set(c, k);
        return k;
      }
      k = this.near.get(c);
      if (k != null) return k;
      const r = c >> 16, g = (c >> 8) & 255, b = c & 255;
      let best = 0, bd = Infinity;
      for (let i = 0; i < this.pal.length; i++) {
        const p = this.pal[i];
        const dr = r - (p >> 16), dg = g - ((p >> 8) & 255), db = b - (p & 255);
        const d = dr * dr * 3 + dg * dg * 4 + db * db * 2;
        if (d < bd) { bd = d; best = i; }
      }
      this.near.set(c, best);
      return best;
    }

    /**
     * Menambah satu frame dari ImageData; delay dalam milidetik. Hanya kotak yang berubah dari frame
     * sebelumnya yang ditulis (sisanya tetap), dan di dalam kotak itu piksel yang sama = transparan.
     */
    addFrame(img, delayMs) {
      const W = this.w, n = W * this.h, d = img.data, ind = new Uint8Array(n);
      for (let i = 0, j = 0; i < n; i++, j += 4) ind[i] = this.colorIndex((d[j] << 16) | (d[j + 1] << 8) | d[j + 2]);
      let x0 = 0, y0 = 0, x1 = W - 1, y1 = this.h - 1;
      if (this.prev) {
        x0 = W; y0 = this.h; x1 = -1; y1 = -1;
        for (let i = 0; i < n; i++) {
          if (this.prev[i] === ind[i]) continue;
          const x = i % W, y = (i - x) / W;
          if (x < x0) x0 = x;
          if (x > x1) x1 = x;
          if (y < y0) y0 = y;
          if (y > y1) y1 = y;
        }
        if (x1 < 0) { x0 = y0 = x1 = y1 = 0; } // tidak ada yang berubah: 1 piksel transparan
      }
      const rw = x1 - x0 + 1, rh = y1 - y0 + 1, out = new Uint8Array(rw * rh);
      for (let y = 0; y < rh; y++) {
        for (let x = 0; x < rw; x++) {
          const i = (y0 + y) * W + x0 + x;
          out[y * rw + x] = this.prev && this.prev[i] === ind[i] ? 255 : ind[i];
        }
      }
      this.prev = ind;
      // graphic control: disposal 1 (biarkan), transparan = 255
      this.out.push(0x21, 0xf9, 0x04, (1 << 2) | 1);
      this.u16(Math.max(2, Math.round(delayMs / 10)));
      this.out.push(255, 0);
      // image descriptor (kotak yang berubah) + tabel warna lokal 256
      this.out.push(0x2c);
      this.u16(x0); this.u16(y0); this.u16(rw); this.u16(rh);
      this.out.push(0x80 | 7);
      for (let i = 0; i < 256; i++) {
        const c = this.pal[i] || 0;
        this.out.push(c >> 16, (c >> 8) & 255, c & 255);
      }
      this.lzw(out, 8);
    }

    lzw(px, minSize) {
      const out = this.out;
      out.push(minSize);
      const clear = 1 << minSize, eoi = clear + 1;
      let size = minSize + 1, next = eoi + 1;
      let dict = new Map();
      let cur = 0, bits = 0;
      const block = [];
      const emit = code => {
        cur |= code << bits;
        bits += size;
        while (bits >= 8) {
          block.push(cur & 255);
          if (block.length === 255) { out.push(255, ...block); block.length = 0; }
          cur >>= 8;
          bits -= 8;
        }
      };
      emit(clear);
      let prefix = px[0];
      for (let i = 1; i < px.length; i++) {
        const k = px[i];
        const key = prefix * 4096 + k;
        const hit = dict.get(key);
        if (hit != null) { prefix = hit; continue; }
        emit(prefix);
        if (next === 4096) { // tabel penuh: mulai ulang
          emit(clear);
          dict = new Map();
          size = minSize + 1;
          next = eoi + 1;
        } else {
          if (next >= (1 << size)) size++;
          dict.set(key, next++);
        }
        prefix = k;
      }
      emit(prefix);
      emit(eoi);
      if (bits > 0) block.push(cur & 255);
      for (let i = 0; i < block.length; i += 255) {
        const part = block.slice(i, i + 255);
        out.push(part.length, ...part);
      }
      out.push(0);
    }

    finish() {
      this.out.push(0x3b);
      return new Uint8Array(this.out);
    }
  }

  // ---------- penyusun bingkai ----------

  /** Ukuran keluaran: panggung saja (lebar tetap, tinggi mengikuti panggung) atau reel 9:16. */
  function frameSize(frame, format, stageW, stageH) {
    const big = format !== 'gif';
    if (frame === 'reel') return big ? { w: 720, h: 1280 } : { w: 360, h: 640 };
    const w = big ? 960 : 480;
    let h = Math.round(w * stageH / stageW);
    if (big && h % 2) h++; // video butuh ukuran genap
    return { w, h };
  }

  /** Teks pixel (font GopherViz) diperbesar k kali. */
  function bigText(ctx, s, x, y, k, col) {
    const c = document.createElement('canvas');
    c.width = Math.max(1, P.textW(s)); c.height = 5;
    P.text(c.getContext('2d'), s, 0, 0, col);
    ctx.imageSmoothingEnabled = false;
    ctx.drawImage(c, Math.round(x), Math.round(y), c.width * k, c.height * k);
  }

  /**
   * Menggambar satu bingkai ke ctx (w×h). info: { title, sub, code: {lines}, line, file, output }.
   */
  function compose(ctx, w, h, frame, stage, info) {
    ctx.imageSmoothingEnabled = false;
    if (frame !== 'reel') {
      ctx.fillStyle = '#fde3eb';
      ctx.fillRect(0, 0, w, h);
      ctx.drawImage(stage, 0, 0, stage.width, stage.height, 0, 0, w, h);
      return;
    }
    const u = w / 360; // satuan skala (360 = GIF, 720 = video)
    ctx.fillStyle = '#121a2e';
    ctx.fillRect(0, 0, w, h);
    // judul
    const title = P.fit(info.title || 'GopherViz', 22);
    const k = Math.max(2, Math.floor(Math.min(4 * u, (w - 24 * u) / Math.max(1, P.textW(title)))));
    bigText(ctx, title, (w - P.textW(title) * k) / 2, 18 * u, k, '#f3c75a');
    if (info.sub) {
      const sub = P.fit(info.sub, 40);
      bigText(ctx, sub, (w - P.textW(sub) * 2 * u) / 2, 18 * u + 5 * k + 8 * u, Math.max(1, Math.round(2 * u)), '#9fb4d8');
    }
    // panggung
    const top = 18 * u + 5 * k + 26 * u;
    const sw = w - 24 * u;
    let sh = sw * stage.height / stage.width;
    const maxSh = h * 0.42;
    let sx = 12 * u, sww = sw;
    if (sh > maxSh) { sww = maxSh * stage.width / stage.height; sh = maxSh; sx = (w - sww) / 2; }
    ctx.fillStyle = '#4a3550';
    ctx.fillRect(sx - 2 * u, top - 2 * u, sww + 4 * u, sh + 4 * u);
    ctx.drawImage(stage, 0, 0, stage.width, stage.height, sx, top, sww, sh);
    // kode dengan baris aktif
    const codeTop = top + sh + 14 * u;
    const outH = 64 * u;
    const codeH = h - codeTop - outH - 22 * u;
    const lh = 13 * u, fs = Math.round(10.5 * u);
    ctx.fillStyle = '#0b1220';
    ctx.fillRect(12 * u, codeTop, w - 24 * u, codeH);
    ctx.strokeStyle = '#2a3550';
    ctx.lineWidth = Math.max(1, u);
    ctx.strokeRect(12 * u, codeTop, w - 24 * u, codeH);
    const lines = (info.code && info.code.lines) || [];
    const rows = Math.max(1, Math.floor((codeH - 10 * u) / lh));
    const cur = info.file ? 0 : info.line || 0;
    let first = Math.max(1, Math.min(cur - Math.floor(rows / 2), lines.length - rows + 1));
    if (!cur) first = Math.max(1, info.startLine || 1);
    ctx.font = `${fs}px Consolas, "Courier New", monospace`;
    ctx.textBaseline = 'top';
    for (let r = 0; r < rows; r++) {
      const n = first + r;
      if (n > lines.length) break;
      const y = codeTop + 5 * u + r * lh;
      if (n === cur) {
        ctx.fillStyle = '#2b3350';
        ctx.fillRect(13 * u, y - 1 * u, w - 26 * u, lh);
        ctx.fillStyle = '#f3c75a';
        ctx.fillRect(13 * u, y - 1 * u, 3 * u, lh);
      }
      ctx.fillStyle = n === cur ? '#f3c75a' : '#4f5d80';
      ctx.fillText(String(n).padStart(3, ' '), 18 * u, y);
      ctx.fillStyle = n === cur ? '#ffffff' : '#b9c3dc';
      const txt = lines[n - 1].replace(/\t/g, '    ');
      ctx.fillText(txt.length > 46 ? txt.slice(0, 45) + '…' : txt, 44 * u, y);
    }
    if (info.file) { // langkah di file lain (modul)
      ctx.fillStyle = '#f7b779';
      ctx.fillText(`→ ${info.file}:${info.line || '?'}`, 20 * u, codeTop + codeH - lh);
    }
    // output
    const oy = h - outH - 14 * u;
    ctx.fillStyle = '#0b1220';
    ctx.fillRect(12 * u, oy, w - 24 * u, outH);
    ctx.fillStyle = '#4f5d80';
    ctx.fillText('OUTPUT', 18 * u, oy + 5 * u);
    ctx.fillStyle = '#9fd88f';
    const outLines = (info.output || '').split('\n').filter((l, i, a) => l || i < a.length - 1).slice(-3);
    outLines.forEach((l, i) => ctx.fillText(l.length > 46 ? l.slice(0, 45) + '…' : l, 18 * u, oy + 19 * u + i * lh));
    ctx.fillStyle = '#4f5d80';
    ctx.fillText('GopherViz', w - 76 * u, h - 12 * u);
  }

  G.recorder = { GifWriter, frameSize, compose };
})(window.GV = window.GV || {});
