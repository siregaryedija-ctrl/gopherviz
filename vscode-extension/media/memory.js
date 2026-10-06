// Diagram memori: variabel = peti, struct = peti dengan ransel berisi field, pointer = tali
// ke data yang ditunjuk (variabel lain atau objek di heap). Digambar sebagai SVG di HTML.
(function (G) {
  'use strict';
  const esc = s => String(s == null ? '' : s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

  const W = 440;        // lebar viewBox (ada ruang di kanan untuk tali antar objek heap)
  const COL_A = 6;      // kolom variabel
  const COL_B = 236;    // kolom heap
  const BOX_W = 150;
  const HEAD = 18;      // tinggi kepala peti (nama)
  const ROW = 16;       // tinggi satu baris isi
  const GAP = 10;

  function rowsOf(item) { return item.fields && item.fields.length ? item.fields.length : 1; }

  /** Membangun SVG untuk satu potret memori. prev = potret langkah sebelumnya (untuk menyorot perubahan). */
  function render(mem, prev) {
    if (!mem) return '';
    const prevVals = {};
    if (prev) {
      for (const v of prev.vars || []) prevVals['v:' + v.name] = JSON.stringify(v);
      for (const o of prev.heap || []) prevVals['o:' + o.id] = JSON.stringify(o);
    }
    const boxes = {}; // label → { x, y, h, rows: { field: y } }
    const parts = [];
    const ropes = [];

    const place = (item, x, y, label, kind) => {
      const h = HEAD + rowsOf(item) * ROW + 4;
      const changed = prev && prevVals[kind + label] !== JSON.stringify(item);
      boxes[label] = { x, y, h, rows: {} };
      const isObj = kind === 'o:';
      const fill = isObj ? '#e3d7f7' : '#f6dcb4';
      const stroke = isObj ? '#5a43a0' : '#8b5a2b';
      parts.push(`<g class="mem-box${changed ? ' changed' : ''}">`);
      parts.push(`<rect x="${x}" y="${y}" width="${BOX_W}" height="${h}" rx="3" fill="${fill}" stroke="${stroke}" stroke-width="1.5"/>`);
      parts.push(`<rect x="${x}" y="${y}" width="${BOX_W}" height="${HEAD}" rx="3" fill="${stroke}"/>`);
      const title = isObj ? `${item.id}${item.type ? ' · ' + item.type : ''}` : `${item.name}${item.type ? ' · ' + item.type : ''}`;
      parts.push(`<text x="${x + 6}" y="${y + 13}" class="mem-title">${esc(title)}</text>`);
      if (item.fields && item.fields.length) {
        item.fields.forEach((f, i) => {
          const ry = y + HEAD + i * ROW;
          boxes[label].rows[f.name] = ry + ROW / 2 + 1;
          if (i > 0) parts.push(`<line x1="${x + 4}" y1="${ry + 1}" x2="${x + BOX_W - 4}" y2="${ry + 1}" stroke="${stroke}" stroke-opacity="0.25"/>`);
          parts.push(`<text x="${x + 6}" y="${ry + 12}" class="mem-field">${esc(f.name)}</text>`);
          if (f.points) {
            ropes.push({ from: label, row: f.name, to: f.points });
            if (f.points === 'nil') parts.push(`<text x="${x + BOX_W - 8}" y="${ry + 12}" class="mem-nil" text-anchor="end">nil</text>`);
          } else {
            parts.push(`<text x="${x + BOX_W - 8}" y="${ry + 12}" class="mem-val" text-anchor="end">${esc(clip(f.value, 14))}</text>`);
          }
        });
      } else if (item.kind === 'pointer') {
        boxes[label].rows[''] = y + HEAD + ROW / 2 + 1;
        ropes.push({ from: label, row: '', to: item.points });
        parts.push(`<text x="${x + 6}" y="${y + HEAD + 12}" class="mem-field">pointer</text>`);
        if (item.points === 'nil') parts.push(`<text x="${x + BOX_W - 8}" y="${y + HEAD + 12}" class="mem-nil" text-anchor="end">nil</text>`);
      } else {
        parts.push(`<text x="${x + 6}" y="${y + HEAD + 12}" class="mem-val">${esc(clip(item.value, 22))}</text>`);
      }
      parts.push('</g>');
      return h;
    };

    let ya = 6, yb = 6;
    for (const v of mem.vars || []) ya += place(v, COL_A, ya, v.name, 'v:') + GAP;
    for (const o of mem.heap || []) yb += place(o, COL_B, yb, o.id, 'o:') + GAP;
    const H = Math.max(ya, yb, 40);

    // tali pointer: dari tepi kanan baris sumber ke kepala peti tujuan
    const ropeSvg = ropes.filter(r => r.to && r.to !== 'nil').map(r => {
      const src = boxes[r.from];
      const dstKey = boxes[r.to] ? r.to : r.to.split('.')[0];
      const dst = boxes[dstKey];
      if (!src || !dst) return '';
      const x1 = src.x + BOX_W, y1 = src.rows[r.row] != null ? src.rows[r.row] : src.y + HEAD / 2;
      let x2, y2, d;
      const fieldRow = r.to.includes('.') && dst.rows[r.to.split('.').slice(1).join('.')];
      y2 = fieldRow || dst.y + HEAD / 2;
      if (dst.x > src.x) { // ke kolom heap
        x2 = dst.x;
        d = `M${x1},${y1} C${x1 + 40},${y1} ${x2 - 40},${y2} ${x2},${y2}`;
      } else { // ke variabel lain di kolom yang sama: melengkung lewat kanan
        x2 = dst.x + BOX_W;
        const bulge = Math.max(x1, x2) + 34 + Math.abs(y2 - y1) * 0.15;
        d = `M${x1},${y1} C${bulge},${y1} ${bulge},${y2} ${x2},${y2}`;
      }
      return `<path d="${d}" class="mem-rope" marker-end="url(#gv-arrow)"/>` +
        `<circle cx="${x1}" cy="${y1}" r="2.5" class="mem-knot"/>`;
    }).join('');

    return `<svg viewBox="0 0 ${W} ${H}" width="100%" role="img" aria-label="Diagram memori: variabel dan pointer">` +
      `<defs><marker id="gv-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto">` +
      `<path d="M0,0 L8,4 L0,8 z" class="mem-arrow"/></marker></defs>` +
      parts.join('') + ropeSvg + '</svg>';
  }

  function clip(s, n) {
    s = String(s == null ? '' : s);
    return s.length > n ? s.slice(0, n - 1) + '…' : s;
  }

  G.memory = { render };
})(window.GV = window.GV || {});
