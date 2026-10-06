// Penghubung webview: menerima state dari ekstensi, mengisi HTML, dan menjalankan Level.
(function (G) {
  'use strict';
  /* global acquireVsCodeApi */
  const vscode = acquireVsCodeApi();
  const $ = id => document.getElementById(id);
  const esc = s => String(s == null ? '' : s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

  const KIND = {
    'for3': 'for klasik', 'cond': 'for kondisi', 'infinite': 'for tanpa batas',
    'range-int': 'range angka', 'range-slice': 'range slice', 'range-array': 'range array',
    'range-string': 'range string', 'range-map': 'range map', 'range-chan': 'range channel',
    'range-func': 'range iterator', 'range-unknown': 'range',
  };
  const EXPLAIN = {
    'for3': 'Init jalan sekali. Lalu berulang: cek kondisi, jalankan isi, jalankan post.',
    'cond': 'Isi diulang selama kondisinya true, seperti while di bahasa lain.',
    'infinite': 'Isi diulang terus sampai bertemu break atau return.',
    'range-int': 'Diulang sebanyak angka itu; variabelnya mulai dari 0.',
    'range-slice': 'Mengunjungi setiap elemen slice secara berurutan: indeks dan nilainya.',
    'range-array': 'Mengunjungi setiap elemen array secara berurutan: indeks dan nilainya.',
    'range-string': 'Mengunjungi setiap karakter (rune). Indeksnya dihitung dalam byte.',
    'range-map': 'Mengunjungi setiap pasangan key dan value. Urutannya tidak dijamin, bisa beda tiap dijalankan.',
    'range-chan': 'Mengambil nilai dari channel satu per satu sampai channel ditutup.',
    'range-func': 'Mengambil nilai dari fungsi iterator sampai iteratornya selesai.',
    'range-unknown': 'Mengunjungi setiap elemen. Tipe datanya belum diketahui (berasal dari package lain).',
  };
  const NODE = {
    stmt: 'perintah', if: 'percabangan if', loop: 'loop di dalam loop', break: 'break: keluar dari loop',
    continue: 'continue: langsung ke putaran berikutnya', return: 'return: keluar dari fungsi',
    goto: 'goto', block: 'blok { }', bad: 'belum bisa dibaca (belum lengkap)',
  };

  let state = null;
  let config = { speed: 1, sound: true, music: true };
  let currentLoop = null;

  const snd = G.createSound(ok => { $('sndHint').hidden = ok || (!config.sound && !config.music); });
  const level = new G.Level($('stage'), {
    onPart: highlightPart,
    onPhase: highlightPhase,
    sound: name => snd.play(name),
  });

  // ---------- state dari ekstensi ----------

  window.addEventListener('message', e => {
    const m = e.data;
    if (m.type === 'config') applyConfig(m);
    else if (m.type === 'state') applyState(m);
  });

  function applyConfig(m) {
    config = { speed: m.speed, sound: m.sound, music: m.music };
    level.speed = config.speed;
    snd.setFx(config.sound);
    snd.setMus(config.music);
    document.querySelectorAll('[data-speed]').forEach(b => b.setAttribute('aria-pressed', String(+b.dataset.speed === config.speed)));
    $('btnFx').setAttribute('aria-pressed', String(config.sound));
    $('btnMus').setAttribute('aria-pressed', String(config.music));
    $('sndHint').hidden = snd.ready() || (!config.sound && !config.music);
  }

  function applyState(m) {
    state = m;
    const res = m.result;
    const loops = res ? res.scene.loops : [];
    const loop = loops.find(l => l.id === m.focusLoop) || null;

    if (!m.server.ok) overlay(m.server.message || 'Server loopviz tidak berjalan.', true);
    else if (!m.file) overlay('Buka file Go untuk mulai.');
    else if (!res) overlay('Menganalisis…');
    else if (!loops.length) overlay('Belum ada loop di file ini.\nCoba ketik:  for i := 0; i < 3; i++ {}');
    else if (!loop) overlay('Taruh kursor di dalam sebuah loop,\natau pilih loopnya di bawah.');
    else overlay(null);

    // Hanya kirim ke level bila isinya berubah, supaya animasi tidak tersendat.
    const key = loop ? JSON.stringify(loop) : '';
    if (key !== (currentLoop ? JSON.stringify(currentLoop) : '')) {
      currentLoop = loop;
      level.setLoop(loop);
      renderParts(loop, loops);
    }
    renderHeader(loop);
    renderNote(loop, res ? res.diagnostics : []);
    renderNav(loops, loop, m.file);
  }

  function overlay(text, restart) {
    $('overlay').hidden = !text;
    $('overlayText').textContent = text || '';
    $('btnRestart').hidden = !restart;
  }

  // ---------- HTML ----------

  const span = (p, text) => `<span class="hp" data-p="${p}">${esc(text)}</span>`;

  function headHTML(loop) {
    const h = loop.header;
    const label = loop.label ? esc(loop.label) + ': ' : '';
    if (loop.state === 'incomplete') return label + esc(h.text);
    switch (loop.kind) {
      case 'for3':
        return `${label}for ${span('init', h.init || '')}; ${span('cond', h.cond || '')}; ${span('post', h.post || '')}`;
      case 'cond':
        return `${label}for ${span('cond', h.cond)}`;
      case 'infinite':
        return `${label}for`;
      default: {
        const lhs = [h.key, h.value].filter(Boolean).join(', ');
        const assign = lhs ? esc(lhs) + (h.define ? ' := ' : ' = ') : '';
        return `${label}for ${span('init', assign)}${span('cond', 'range ' + (h.range || ''))}`;
      }
    }
  }

  function renderHeader(loop) {
    $('loopId').textContent = loop ? loop.id : 'GoLoopViz';
    $('kind').textContent = loop ? KIND[loop.kind] || loop.kind : (state && state.file) || '';
    $('head').innerHTML = loop ? headHTML(loop) + ' {' : '';
    $('head').hidden = !loop;
    $('explain').textContent = loop ? EXPLAIN[loop.kind] || '' : '';
    const chip = $('stateChip');
    chip.hidden = !loop || loop.state !== 'incomplete';
    chip.textContent = 'belum lengkap';
  }

  function renderNote(loop, diags) {
    const note = $('note');
    if (!loop) { note.textContent = ''; return; }
    if (loop.state === 'incomplete') {
      const d = diags.find(x => x.line >= loop.line && x.line <= loop.endLine + 1) || diags[0];
      note.textContent = 'Loop ini belum selesai diketik' + (d ? ` (baris ${d.line}: ${d.message})` : '') + '. Gopher menunggu sampai lengkap.';
      note.className = 'note warn';
      return;
    }
    note.textContent = 'Mode struktur: Gopher menelusuri alur loop sebagai contoh 3 putaran. Nilai variabel asli tiap putaran menyusul.';
    note.className = 'note';
  }

  function renderParts(loop, loops) {
    const ol = $('parts');
    $('partsSection').hidden = !loop;
    if (!loop) { ol.innerHTML = ''; return; }
    if (!loop.body.length) {
      ol.innerHTML = `<li><div class="part empty"><span class="ico ico-empty"></span><span class="txt">Isi loop masih kosong. Setiap baris yang kamu tulis di antara { } akan muncul sebagai blok.</span></div></li>`;
      return;
    }
    ol.innerHTML = loop.body.map((n, i) => {
      let text = n.text || '';
      if (n.kind === 'loop') {
        const child = loops.find(l => l.id === n.ref);
        text = child ? `${child.id}: ${child.header.text}` : n.ref || 'loop';
      } else if (n.kind === 'if') {
        const parts = (n.then || []).length + (n.else || []).length;
        if (parts) text += `  (${parts} bagian di dalamnya)`;
      } else if (n.kind === 'block') {
        text = '{ … }';
      }
      return `<li><button class="part" data-line="${n.line || loop.line}">` +
        `<span class="num">${i + 1}</span><span class="ico ico-${n.kind}"></span>` +
        `<span class="txt"><code>${esc(text)}</code><small>${esc(NODE[n.kind] || n.kind)} · baris ${n.line || '?'}</small></span>` +
        `</button></li>`;
    }).join('');
  }

  function renderNav(loops, focus, file) {
    $('navSection').hidden = loops.length === 0;
    $('navTitle').textContent = file ? `Loop di ${file}` : 'Loop di file ini';
    $('nav').innerHTML = loops.map(l => {
      const depth = l.parent ? '└ ' : '';
      const cur = focus && l.id === focus.id ? ' aria-current="true"' : '';
      return `<button class="loop-chip${l.state === 'incomplete' ? ' ghost' : ''}" data-line="${l.line}"${cur}>` +
        `${depth}<b>${esc(l.id)}</b> ${esc(KIND[l.kind] || l.kind)} · baris ${l.line}</button>`;
    }).join('');
  }

  function highlightPart(i) {
    // Sengaja tanpa scrollIntoView: menggulir halaman akan membuat panggung keluar dari pandangan.
    document.querySelectorAll('#parts .part').forEach((el, k) => el.classList.toggle('on', k === i));
  }

  function highlightPhase(p) {
    document.querySelectorAll('#head .hp').forEach(el => el.classList.toggle('on', el.dataset.p === p));
  }

  // ---------- interaksi ----------

  document.addEventListener('click', e => {
    const t = e.target.closest('[data-line]');
    if (t) vscode.postMessage({ type: 'reveal', line: +t.dataset.line });
  });
  document.querySelectorAll('[data-speed]').forEach(b => b.addEventListener('click', () => {
    applyConfig({ ...config, speed: +b.dataset.speed });
    vscode.postMessage({ type: 'setConfig', key: 'speed', value: +b.dataset.speed });
  }));
  $('btnFx').addEventListener('click', () => {
    applyConfig({ ...config, sound: !config.sound });
    vscode.postMessage({ type: 'setConfig', key: 'sound', value: config.sound });
  });
  $('btnMus').addEventListener('click', () => {
    applyConfig({ ...config, music: !config.music });
    vscode.postMessage({ type: 'setConfig', key: 'music', value: config.music });
  });
  $('btnRestart').addEventListener('click', () => vscode.postMessage({ type: 'restart' }));
  ['pointerdown', 'keydown'].forEach(ev => document.addEventListener(ev, () => snd.unlock()));

  // ---------- loop animasi ----------

  let last = performance.now();
  function frame(now) {
    const dt = Math.min(64, now - last);
    last = now;
    level.update(dt);
    level.render();
    const playing = !!currentLoop && currentLoop.state === 'complete' && document.visibilityState === 'visible';
    snd.music(config.music && playing);
    requestAnimationFrame(frame);
  }

  G.app = { level }; // untuk dev/harness.html
  snd.unlock(); // di VS Code suara kadang boleh langsung; kalau tidak, menunggu klik
  applyConfig(config);
  overlay('Menyiapkan…');
  requestAnimationFrame(frame);
  vscode.postMessage({ type: 'ready' });
})(window.GLV = window.GLV || {});
