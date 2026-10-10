// Penghubung webview: menerima state dari ekstensi, mengisi HTML, memilih renderer sesuai
// blok yang difokuskan (loop, if/switch, fungsi), dan memutar trace langkah demi langkah.
(function (G) {
  'use strict';
  /* global acquireVsCodeApi */
  const vscode = acquireVsCodeApi();
  const $ = id => document.getElementById(id);
  const esc = s => String(s == null ? '' : s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

  const KIND = {
    'for3': 'for klasik', 'cond': 'for kondisi', 'do-while': 'do-while', 'infinite': 'for tanpa batas',
    'range-int': 'range angka', 'range-slice': 'range slice', 'range-array': 'range array',
    'range-string': 'range string', 'range-map': 'range map', 'range-keys': 'range key', 'range-chan': 'range channel',
    'range-func': 'range iterator', 'range-unknown': 'range',
  };
  // Bahasa selain Go: nama jenis loop mengikuti istilah umum (while, for-each, for...in).
  const KIND_OTHER = {
    'cond': 'while', 'infinite': 'loop tanpa batas', 'range-int': 'for range()', 'range-slice': 'for-each',
    'range-array': 'for-each', 'range-string': 'per karakter', 'range-map': 'for-each key-value',
    'range-keys': 'for...in (key)', 'range-unknown': 'for-each',
  };
  const EXPLAIN_OTHER = {
    'cond': 'Isi diulang selama kondisinya true; kondisi dicek sebelum setiap putaran.',
    'range-int': 'Diulang untuk setiap angka dari range(); kalau tidak ditentukan, mulai dari 0.',
    'range-slice': 'Mengunjungi setiap elemen secara berurutan.',
    'range-array': 'Mengunjungi setiap elemen secara berurutan.',
    'range-string': 'Mengunjungi setiap karakter string secara berurutan.',
    'range-map': 'Mengunjungi setiap pasangan key dan value.',
    'range-unknown': 'Mengunjungi setiap elemen dari koleksi itu.',
  };
  const LANG_NAME = { go: 'Go', javascript: 'JavaScript', typescript: 'TypeScript', python: 'Python', php: 'PHP', cpp: 'C++', c: 'C', java: 'Java' };
  const sceneLang = () => (state && state.result && state.result.scene && state.result.scene.language) || 'go';
  const EXPLAIN = {
    'for3': 'Init jalan sekali. Lalu berulang: cek kondisi, jalankan isi, jalankan post.',
    'cond': 'Isi diulang selama kondisinya true, seperti while di bahasa lain.',
    'do-while': 'Isi dijalankan dulu sekali, baru kondisinya dicek; diulang selama kondisinya true.',
    'infinite': 'Isi diulang terus sampai bertemu break atau return.',
    'range-int': 'Diulang sebanyak angka itu; variabelnya mulai dari 0.',
    'range-slice': 'Mengunjungi setiap elemen secara berurutan: indeks dan nilainya.',
    'range-array': 'Mengunjungi setiap elemen array secara berurutan: indeks dan nilainya.',
    'range-string': 'Mengunjungi setiap karakter. Di Go, indeksnya dihitung dalam byte.',
    'range-map': 'Mengunjungi setiap pasangan key dan value. Urutannya tidak dijamin, bisa beda tiap dijalankan.',
    'range-keys': 'Mengunjungi setiap key (nama properti) dari objek.',
    'range-chan': 'Mengambil nilai dari channel satu per satu sampai channel ditutup.',
    'range-func': 'Mengambil nilai dari fungsi iterator sampai iteratornya selesai.',
    'range-unknown': 'Mengunjungi setiap elemen. Tipe datanya belum diketahui (berasal dari package lain).',
  };
  const BLOCK_EXPLAIN = {
    'if': 'Kondisi dicek satu per satu dari atas. Cabang pertama yang true dijalankan, sisanya dilewati.',
    'switch': 'Nilainya dicocokkan dengan setiap case dari atas. Case pertama yang cocok dijalankan.',
    'func': 'Setiap pemanggilan fungsi = Gopher masuk pipa ke lantai baru membawa argumen; return = naik lagi membawa koin berisi nilai. Rekursi = lantai bertumpuk ke bawah.',
    'main': 'Timeline seluruh program dari awal main. Setiap fungsi yang dipanggil turun ke lantai baru.',
    'class': 'Class = cetak biru (daftar field dan method). Setiap object = rumah yang dibangun dari cetak biru itu; kamarnya = field. Memanggil method = Gopher masuk ke rumah object itu (self/this), jadi terlihat object mana yang berubah.',
    'islands': 'Program ini terpecah ke beberapa file. Setiap file = pulau, fungsinya = pondok. Memanggil fungsi dari file lain = Gopher menyeberang jembatan ke pulau itu; return = koin hasilnya dibawa pulang.',
    'lanes': 'Setiap goroutine = satu Gopher di jalurnya sendiri. Channel = pipa: kirim melempar bola masuk, terima mengambilnya. Gopher yang menunggu berhenti sampai pasangannya datang. Urutan di sini mengikuti penjadwal Go dengan satu prosesor; di komputer kamu urutannya bisa berbeda.',
  };
  const NODE = {
    stmt: 'perintah', if: 'percabangan if', switch: 'switch', loop: 'loop', break: 'break: keluar dari loop',
    continue: 'continue: langsung ke putaran berikutnya', return: 'return: keluar dari fungsi',
    goto: 'goto', block: 'blok { }', bad: 'belum bisa dibaca (belum lengkap)',
  };
  const BRANCH = { 'if': 'if', 'else-if': 'else if', 'else': 'else', 'case': 'case', 'default': 'default' };

  let state = null;
  let config = { speed: 1, sound: true, music: true, details: false, size: 100 };
  let blocks = [], loops = [];
  let currentBlock = null, currentLoop = null, blockKey = '';
  let traceNote = ''; // peringatan dari trace mode struktur

  const snd = G.createSound(ok => { $('sndHint').hidden = ok || (!config.sound && !config.music); });
  const level = new G.Level($('stage'), { onPart: highlightPart, onPhase: highlightPhase, sound: name => snd.play(name) });
  const data = new G.DataStage($('stage'), {});
  const branch = new G.BranchStage($('stage'), {});
  const func = new G.FuncStage($('stage'), {});
  const lanes = new G.LaneStage($('stage'), {});
  const graph = new G.GraphStage($('stage'), {});
  const cls = new G.ClassStage($('stage'), {});
  const isles = new G.IslandStage($('stage'), {});
  const maze = new G.GridStage($('stage'), {});
  const pohon = new G.TreeStage($('stage'), {});
  const belah = new G.MergeStage($('stage'));

  // ---------- state dari ekstensi ----------

  window.addEventListener('message', e => {
    const m = e.data;
    if (m.type === 'config') applyConfig(m);
    else if (m.type === 'state') applyState(m);
    else if (m.type === 'trace') applyTrace(m);
    else if (m.type === 'race') applyRace(m);
  });

  function applyConfig(m) {
    config = { speed: m.speed, sound: m.sound, music: m.music, details: !!m.details, size: m.size || 100 };
    stageScale = P_clamp(config.size / 100, 0.25, 1);
    // Bawaan: panel hanya gambar (keputusan user 2026-10-07); teks penjelasan lewat setting gopherviz.details.
    document.body.classList.toggle('pic', !config.details);
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
    blocks = res ? res.scene.blocks || [] : [];
    loops = res ? res.scene.loops || [] : [];
    const block = blocks.find(b => b.id === m.focusBlock) || null;
    const loop = block && block.kind === 'loop' ? loops.find(l => l.id === block.id) || null : null;

    if (!m.server.ok) overlay(m.server.message || 'Server gopherviz tidak berjalan.', true);
    else if (!m.file) overlay('Buka file kode untuk mulai.');
    else if (!res) overlay('Menganalisis…');
    else if (!blocks.length) overlay('Belum ada fungsi atau loop di file ini.\nCoba ketik:  for i := 0; i < 3; i++ {}');
    else if (!block) overlay('Taruh kursor di dalam fungsi, if, switch, atau loop,\natau pilih bloknya di bawah.');
    else overlay(null);

    // Renderer hanya diperbarui bila isi blok berubah, supaya animasi tidak tersendat.
    const key = block ? JSON.stringify([block, loop]) : '';
    if (key !== blockKey) {
      blockKey = key;
      currentBlock = block;
      currentLoop = loop;
      stopTrace(); // kode berubah: trace lama tidak berlaku, tampilkan struktur sampai trace baru datang
      level.setLoop(loop);
      if (block && (block.kind === 'if' || block.kind === 'switch')) branch.setBlock(block);
      if (block && block.kind === 'func') func.setBlock(block, blocks);
      if (block && block.kind === 'class') cls.setBlock(block);
      renderParts();
    }
    renderHeader();
    renderNote(res ? res.diagnostics : []);
    renderNav(m.file);
    if (document.activeElement !== $('inputBox')) $('inputBox').value = m.input || '';
    syncInputBar();
  }

  function overlay(text, restart) {
    $('overlay').hidden = !text;
    $('overlayText').textContent = text || '';
    $('btnRestart').hidden = !restart;
  }

  // ---------- HTML ----------

  const span = (p, text) => `<span class="hp" data-p="${p}">${esc(text)}</span>`;

  /** Menyorot bagian-bagian (init/cond/post/range) di dalam teks kepala apa adanya. */
  function markParts(text, parts) {
    let out = '', pos = 0;
    for (const [p, sub] of parts) {
      if (!sub) continue;
      const i = text.indexOf(sub, pos);
      if (i < 0) continue;
      out += esc(text.slice(pos, i)) + span(p, sub);
      pos = i + sub.length;
    }
    return out + esc(text.slice(pos));
  }

  function loopHead(loop) {
    const h = loop.header;
    const label = loop.label ? esc(loop.label) + ': ' : '';
    if (loop.state === 'incomplete') return label + esc(h.text);
    if (sceneLang() !== 'go') {
      const lhs = [h.key, h.value].filter(Boolean);
      return label + markParts(h.text, [['init', h.init], ['cond', h.cond], ['post', h.post], ...lhs.map(x => ['init', x]), ['cond', h.range]]);
    }
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

  function kindLabel(b) {
    if (!b) return '';
    if (b.kind === 'loop') {
      const l = loops.find(x => x.id === b.id);
      if (!l) return 'loop';
      return (sceneLang() !== 'go' && KIND_OTHER[l.kind]) || KIND[l.kind] || l.kind;
    }
    if (b.kind === 'func') return `fungsi ${b.name || ''}`.trim();
    if (b.kind === 'class') return `class ${b.name || ''}`.trim();
    const n = (b.branches || []).length;
    return b.kind === 'if' ? `if · ${n} cabang` : `switch · ${n} case`;
  }

  function renderHeader() {
    const b = currentBlock;
    $('loopId').textContent = b ? b.id : 'GopherViz';
    const lg = sceneLang();
    $('kind').textContent = b ? kindLabel(b) + (lg !== 'go' ? ` · ${LANG_NAME[lg] || lg}` : '') : (state && state.file) || '';
    const open = lg === 'python' ? ':' : ' {';
    $('head').innerHTML = !b ? '' : currentLoop ? loopHead(currentLoop) + open : esc(b.header) + open;
    $('head').hidden = !b;
    let explain = '';
    if (currentLoop) explain = (sceneLang() !== 'go' && EXPLAIN_OTHER[currentLoop.kind]) || EXPLAIN[currentLoop.kind] || '';
    else if (b) explain = b.kind === 'func' && b.name === 'main' ? BLOCK_EXPLAIN.main : BLOCK_EXPLAIN[b.kind] || '';
    $('explain').textContent = explain;
    const chip = $('stateChip');
    chip.hidden = !b || b.state !== 'incomplete';
    chip.textContent = 'belum lengkap';
  }

  function renderNote(diags) {
    const note = $('note'), b = currentBlock;
    if (!b) { note.textContent = ''; return; }
    if (b.state === 'incomplete') {
      const d = diags.find(x => x.line >= b.line && x.line <= b.endLine + 1) || diags[0];
      note.textContent = 'Blok ini belum selesai diketik' + (d ? ` (baris ${d.line}: ${d.message})` : '') + '. Gopher menunggu sampai lengkap.';
      note.className = 'note warn';
      return;
    }
    if (tp.active) {
      const warn = tp.trace.warnings.length ? tp.trace.warnings.join(' ') : '';
      note.textContent = (tp.trace.truncated ? 'Trace dipotong. ' : '') + warn;
      note.className = warn ? 'note warn' : 'note';
      return;
    }
    const idle = currentLoop
      ? 'Mode struktur: Gopher menelusuri alur loop sebagai contoh 3 putaran. Nilai asli muncul setelah kamu berhenti mengetik sebentar.'
      : 'Mode struktur: nilai asli muncul setelah kamu berhenti mengetik sebentar.';
    note.textContent = traceNote ? 'Mode struktur: ' + traceNote : idle;
    note.className = traceNote ? 'note warn' : 'note';
  }

  function nodeText(n) {
    if (n.kind === 'loop') {
      const child = loops.find(l => l.id === n.ref);
      return child ? `${child.id}: ${child.header.text}` : n.ref || 'loop';
    }
    if (n.kind === 'block') return '{ … }';
    let text = n.text || '';
    if (n.kind === 'if') {
      const parts = (n.then || []).length + (n.else || []).length;
      if (parts) text += `  (${parts} bagian di dalamnya)`;
    }
    return text;
  }

  function partRow(i, ico, text, small, line) {
    const tag = line ? `button class="part" data-line="${line}"` : 'div class="part empty"';
    return `<li><${tag}><span class="num">${i}</span><span class="ico ico-${ico}"></span>` +
      `<span class="txt"><code>${esc(text)}</code><small>${esc(small)}</small></span></${tag.split(' ')[0]}></li>`;
  }

  function renderParts() {
    const ol = $('parts'), b = currentBlock;
    $('partsSection').hidden = !b;
    if (!b) { ol.innerHTML = ''; return; }
    if (b.kind === 'class') { // daftar method: anak blok (Go: fungsi dengan receiver tipe ini)
      $('partsTitle').textContent = 'Method';
      const goRecv = new RegExp('^func\\s*\\(\\s*\\w*\\s*\\*?' + b.name + '\\s*\\)');
      const ms = blocks.filter(x => x.kind === 'func' && (x.parent === b.id || goRecv.test(x.header || '')));
      ol.innerHTML = ms.length
        ? ms.map((m, i) => partRow(i + 1, 'stmt', m.header, `method · baris ${m.line}`, m.line)).join('')
        : partRow('', 'empty', 'Belum ada method. Field-nya tetap terlihat di cetak biru.', '', 0);
      return;
    }
    if (b.kind === 'if' || b.kind === 'switch') {
      $('partsTitle').textContent = b.kind === 'if' ? 'Cabang' : 'Case';
      const brs = b.branches || [];
      let rows = brs.map((br, i) => {
        let kw = BRANCH[br.kind];
        if (br.kind === 'else-if') kw = { python: 'elif', php: 'elseif' }[sceneLang()] || kw;
        if (br.kind === 'default' && sceneLang() === 'python') kw = 'case _';
        const text = `${kw} ${br.label}`.trim();
        return partRow(i + 1, br.kind === 'else' || br.kind === 'default' ? 'block' : 'if', text,
          `${br.body.length ? br.body.length + ' bagian' : 'kosong'} · baris ${br.line}`, br.line);
      }).join('');
      if (!brs.some(br => br.kind === 'else' || br.kind === 'default')) {
        rows += partRow(brs.length + 1, 'bad', b.kind === 'if' ? 'tidak ada else' : 'tidak ada default',
          'kalau tidak ada yang cocok, blok ini dilewati', 0);
      }
      ol.innerHTML = rows;
      return;
    }
    $('partsTitle').textContent = b.kind === 'func' ? 'Isi fungsi' : 'Isi loop';
    const body = currentLoop ? currentLoop.body : b.body || [];
    if (!body.length) {
      ol.innerHTML = partRow('', 'empty', 'Isinya masih kosong. Setiap baris yang kamu tulis di antara { } akan muncul sebagai blok.', '', 0);
      return;
    }
    ol.innerHTML = body.map((n, i) => partRow(i + 1, n.kind, nodeText(n), `${NODE[n.kind] || n.kind} · baris ${n.line || '?'}`, n.line || b.line)).join('');
  }

  function renderNav(file) {
    $('navSection').hidden = blocks.length === 0;
    $('navTitle').textContent = file ? `Blok di ${file}` : 'Blok di file ini';
    const depth = b => { let d = 0, p = b.parent; while (p) { d++; const x = blocks.find(y => y.id === p); p = x && x.parent; } return d; };
    $('nav').innerHTML = blocks.map(b => {
      const cur = currentBlock && b.id === currentBlock.id ? ' aria-current="true"' : '';
      const indent = '  '.repeat(Math.min(depth(b), 4)) + (b.parent ? '└ ' : '');
      return `<button class="loop-chip${b.state === 'incomplete' ? ' ghost' : ''}" data-line="${b.line}"${cur}>` +
        `${indent}<b>${esc(b.id)}</b> ${esc(kindLabel(b))} · baris ${b.line}</button>`;
    }).join('');
  }

  function highlightPart(i) {
    // Sengaja tanpa scrollIntoView: menggulir halaman akan membuat panggung keluar dari pandangan.
    document.querySelectorAll('#parts .part').forEach((el, k) => el.classList.toggle('on', k === i));
  }

  function highlightPhase(p) {
    document.querySelectorAll('#head .hp').forEach(el => el.classList.toggle('on', el.dataset.p === p));
  }

  // ---------- pemutar trace ----------

  const tp = { active: false, trace: null, view: [], boost: [], r: null, i: 0, el: 0, playing: true, once: false, cues: [], ci: 0, hold: 0 };

  /** Adapter supaya semua renderer diputar dengan cara yang sama. */
  const levelR = { prepare: tr => level.prepareTrace(tr), duration: i => level.traceDuration(i), cues: i => level.traceCues(i), render: (i, t) => level.renderTrace(i, t) };
  const dataR = {
    prepare: tr => data.prepare(tr, currentLoop || { vars: [], children: (currentBlock.children || []).filter(id => loops.some(l => l.id === id)) }, loops),
    duration: i => data.duration(i), cues: i => data.cues(i), render: (i, t) => data.render(i, t),
  };
  const branchR = { prepare: tr => branch.prepare(tr), duration: i => branch.duration(i), cues: i => branch.cues(i), render: (i, t) => branch.render(i, t) };
  const funcR = { prepare: tr => func.prepare(tr), duration: i => func.duration(i), cues: i => func.cues(i), render: (i, t) => func.render(i, t) };
  const graphR = { prepare: tr => graph.prepare(tr), duration: i => graph.duration(i), cues: i => graph.cues(i), render: (i, t) => graph.render(i, t) };
  const treeR = { prepare: tr => pohon.prepare(tr), duration: i => pohon.duration(i), cues: i => pohon.cues(i), render: (i, t) => pohon.render(i, t) };
  const gridR = { prepare: tr => maze.prepare(tr), duration: i => maze.duration(i), cues: i => maze.cues(i), render: (i, t) => maze.render(i, t) };
  const islandR = { prepare: tr => isles.prepare(tr), duration: i => isles.duration(i), cues: i => isles.cues(i), render: (i, t) => isles.render(i, t) };
  const classR = { prepare: tr => cls.prepare(tr), duration: i => cls.duration(i), cues: i => cls.cues(i), render: (i, t) => cls.render(i, t) };
  const laneR = { prepare: tr => lanes.prepare(tr), duration: i => lanes.duration(i), cues: i => lanes.cues(i), render: (i, t) => lanes.render(i, t) };
  const mergeR = { prepare: tr => belah.prepare(tr), duration: i => belah.duration(i), cues: i => belah.cues(i), render: (i, t) => belah.render(i, t) };

  /** Langkah call/return hanya berarti di mode fungsi; di renderer lain digabung ke langkah berikutnya. */
  function foldCalls(tr) {
    const steps = [];
    let carry = '';
    for (const s of tr.steps) {
      if (s.phase === 'call' || s.phase === 'return') { carry += s.output || ''; continue; }
      steps.push(carry ? Object.assign({}, s, { output: carry + (s.output || '') }) : s);
      carry = '';
    }
    if (carry && steps.length) steps[steps.length - 1] = Object.assign({}, steps[steps.length - 1], { output: (steps[steps.length - 1].output || '') + carry });
    return Object.assign({}, tr, { steps });
  }

  // Kotak Input muncul hanya bila program membaca input user (atau kotaknya sudah berisi).
  let readsInput = false;
  function syncInputBar() {
    $('inputbar').hidden = !readsInput && !$('inputBox').value;
  }

  function applyTrace(m) {
    if (!currentBlock || m.blockId !== currentBlock.id) return; // trace untuk blok lain
    tp.code = m.code || null; // isi file: potongan kode di rekaman reel
    let tr = m.trace;
    readsInput = !!tr.readsInput;
    syncInputBar();
    stopTrace();
    if (tr.mode !== 'simulated' || !tr.steps.length) {
      traceNote = tr.warnings[0] || '';
      renderNote((state && state.result && state.result.diagnostics) || []);
      return;
    }
    const kind = currentBlock.kind;
    if (kind !== 'func' && kind !== 'class') tr = foldCalls(tr);
    if (!tr.steps.length) return;
    // Mode data hanya untuk koleksi berisi nilai sederhana (angka, string pendek); slice bersarang memakai renderer lain.
    const scalar = v => v.length <= 12 && !/^(\[|map\[|\{|&)/.test(v);
    const coll = tr.collection;
    // if berisi pencarian bawaan (if 9 in arr, in_array, includes): tampilkan barisan yang dicek
    const probes = kind === 'if' && tr.steps.some(s => (s.ops || []).filter(o => o.op === 'compare' && o.b == null).length > 1);
    const dataMode = !!coll && (kind === 'loop' || kind === 'func' || probes) && (coll.kind === 'map'
      ? tr.steps.some(s => s.ops && s.ops.some(o => o.key != null && o.op !== 'read')) // map: loker
      : coll.items.every(scalar) && tr.steps.some(s => s.ops && s.ops.some(o => o.op !== 'read')));
    tp.stats = tr.steps.some(s => (s.ops || []).some(o => o.op === 'compare' || o.op === 'swap'));
    // Program dengan goroutine: jalur per goroutine + pipa channel, apa pun jenis blok fokusnya.
    const laneMode = tr.steps.some(s => s.gs) || (kind === 'func' && tr.steps.some(s => s.chans));
    if (laneMode) $('explain').textContent = BLOCK_EXPLAIN.lanes;
    // DFS/BFS: simulator mengenali graf (daftar ketetanggaan) yang ditelusuri → panggung graf
    // fungsi yang memanggil (atau dipanggil dari) file lain: pulau + jembatan
    const islands = kind === 'func' && tr.files && tr.files.length > 1;
    if (islands) $('explain').textContent = BLOCK_EXPLAIN.islands;
    // merge sort (atau quick sort) yang mengembalikan list baru di setiap rekursi: panggung belah-gabung
    const sortInfo = kind === 'func' && G.sorts ? G.sorts.detect(tr) : null;
    const mergeMode = !!(sortInfo && sortInfo.lists) && !laneMode && !islands;
    tp.r = kind === 'class' ? classR : laneMode ? laneR : tr.tree ? treeR : tr.grid ? gridR : tr.graph ? graphR : islands ? islandR : mergeMode ? mergeR : dataMode ? dataR : { loop: levelR, if: branchR, switch: branchR, func: funcR }[kind];
    tp.p = withErrFx(tp.r);
    level.mode = 'trace'; // hentikan animasi struktur (dan suaranya) di belakang layar
    tp.trace = tr;
    tp.view = tp.r.prepare(tr);
    tp.boost = boostOf(tp.view);
    tp.active = true;
    tp.playing = true;
    $('tracebar').hidden = false;
    $('stepNote').hidden = false;
    $('varsSection').hidden = false;
    $('tScrub').max = String(tp.view.length - 1);
    goto(0, false);
    syncPlay();
    renderNote([]);
    if (quiz.on) quizStart();
  }

  /** Error di langkah view ke-i (Step.err), bila ada. */
  const errOf = i => { const v = tp.view[i]; return v ? (v.group || [v.step]).map(s => s.err).find(Boolean) : null; };

  /**
   * Pemutar memakai adapter ini: sama dengan renderer panggung, tetapi langkah error diperpanjang
   * dengan adegan penjelasan (errorfx.js) yang digambar di atas panggung setelah animasi langkahnya.
   */
  function withErrFx(base) {
    const FX = G.errorfx, cv = $('stage'), ctx = cv.getContext('2d');
    return {
      duration: i => base.duration(i) + (errOf(i) ? FX.DUR : 0),
      cues: i => {
        const e = errOf(i);
        if (!e) return base.cues(i);
        const d = base.duration(i), T = d + FX.DUR;
        return base.cues(i).map(c => Object.assign({}, c, { t: (c.t * d) / T }))
          .concat(FX.cues(e).map(c => Object.assign({}, c, { t: (d + c.t * FX.DUR) / T })));
      },
      render: (i, t) => {
        const e = errOf(i);
        if (!e) return base.render(i, t);
        const d = base.duration(i), el = t * (d + FX.DUR);
        base.render(i, Math.min(1, el / d));
        if (el > d) FX.draw(ctx, cv, e, (el - d) / FX.DUR);
      },
    };
  }

  /**
   * Percepatan otomatis untuk trace panjang (mis. mencari 67 di antara 100 balok): langkah yang
   * jauh dari kejadian penting diputar makin cepat (sampai 12x), lalu melambat lagi menjelang
   * kejadian berikutnya. Kejadian penting: beberapa langkah pertama/terakhir, ketemu (compare =
   * dengan ==), break/keluar/return/panic.
   */
  function boostOf(view) {
    const n = view.length, out = new Array(n).fill(1);
    if (n <= 40) return out;
    const max = Math.min(12, n / 25);
    const key = view.map((v, i) => {
      const s = v.step || {};
      return i < 6 || i >= n - 6 || !!s.reason || s.phase === 'break' || s.phase === 'return' ||
        (s.ops || []).some(o => o.op === 'compare' && o.result === '=' && (!o.cmp || o.cmp === '=='));
    });
    let d = Infinity;
    const dist = new Array(n);
    for (let i = 0; i < n; i++) { d = key[i] ? 0 : d + 1; dist[i] = d; }
    d = Infinity;
    for (let i = n - 1; i >= 0; i--) { d = key[i] ? 0 : d + 1; dist[i] = Math.min(dist[i], d); }
    for (let i = 0; i < n; i++) out[i] = 1 + (max - 1) * Math.min(1, Math.max(0, (dist[i] - 2) / 6));
    return out;
  }

  function stopTrace() {
    if (tp.active || level.mode === 'trace') level.stopTrace();
    tp.active = false;
    tp.trace = null;
    traceNote = '';
    for (const id of ['tracebar', 'stepNote', 'varsSection', 'memSection', 'outSection', 'errCard', 'quizCard', 'raceCard']) $(id).hidden = true;
    quiz.open = null;
    raceView.stop(); // kode berubah: hasil lomba lama tidak berlaku
    tp.outLen = 0;
    vscode.postMessage({ type: 'step', line: 0 }); // hapus sorotan baris di editor
    highlightPart(null);
    highlightPhase(null);
  }

  function goto(i, silent) {
    tp.i = Math.max(0, Math.min(i, tp.view.length - 1));
    tp.el = 0;
    tp.hold = 0;
    tp.cues = tp.p.cues(tp.i).sort((a, b) => a.t - b.t);
    tp.ci = silent ? tp.cues.length : 0;
    stepUI();
  }

  function fireCues(t) {
    while (tp.ci < tp.cues.length && tp.cues[tp.ci].t <= t) {
      const c = tp.cues[tp.ci++];
      if (c.sound && (tp.boost[tp.i] || 1) < 2.5) snd.play(c.sound); // langkah yang dipercepat: tanpa suara
      if (c.fx) c.fx();
    }
  }

  /** Baris kode → indeks baris di daftar "Isi"/"Cabang". */
  function partOf(line, step) {
    const b = currentBlock;
    if (!b || !line) return -1;
    const nodeAt = G.pixel.nodeAt;
    if (currentLoop) return nodeAt(currentLoop.body, line);
    if (b.kind === 'func') return step && step.depth ? -1 : nodeAt(b.body, line); // lantai dalam = fungsi lain/rekursi
    const brs = b.branches || [];
    if (step && step.phase === 'branch' && step.branch != null) return Math.min(step.branch, brs.length);
    const k = brs.findIndex(br => br.line === line);
    return k >= 0 ? k : brs.findIndex(br => nodeAt(br.body, line) >= 0);
  }

  function stepUI() {
    const v = tp.view[tp.i], s = v.step;
    const group = v.group && v.group.length > 1 ? `${s.loop} menjalankan ${v.group.length} langkah · ` : '';
    const floor = tp.r === laneR ? (s.g && !(s.note || '').startsWith(`G${s.g} `) ? `G${s.g} · ` : '') : s.depth ? `lantai ${s.depth} · ` : '';
    $('stepNote').textContent = (s.file ? `${s.file}:${s.line || '?'} · ` : '') + floor + group + (s.note || '');
    $('stepNote').className = 'step-note' + (s.reason === 'panic' ? ' warn' : '');
    renderErrCard(errOf(tp.i), s);

    // variabel: diagram memori bila program memakai struct/pointer, selain itu chip biasa.
    // Perubahan dibanding langkah sebelumnya disorot.
    const before = tp.i > 0 ? tp.view[tp.i - 1].step : null;
    $('memSection').hidden = !s.mem;
    $('varsSection').hidden = !!s.mem;
    if (s.mem) {
      $('mem').innerHTML = G.memory.render(s.mem, before && before.mem);
    } else {
      const prev = (before && before.vars) || {};
      const vars = s.vars || {};
      $('vars').innerHTML = Object.keys(vars).sort().map(k => {
        const changed = tp.i > 0 && prev[k] !== vars[k];
        return `<span class="var${changed ? ' changed' : ''}"><b>${esc(k)}</b> = ${esc(vars[k])}</span>`;
      }).join('') || '<span class="muted">Belum ada variabel.</span>';
    }

    // output kumulatif sampai langkah ini; kotak selalu tampil selama trace, garis hijau saat output bertambah
    let out = '';
    for (let k = 0; k <= tp.i; k++) {
      const vk = tp.view[k];
      for (const st of vk.group || [vk.step]) out += st.output || '';
    }
    $('outSection').hidden = false;
    $('out').textContent = out || '—';
    $('out').classList.toggle('empty', !out);
    $('out').scrollTop = $('out').scrollHeight;
    if (out.length > (tp.outLen || 0)) {
      const sec = $('outSection');
      sec.classList.remove('fresh');
      void sec.offsetWidth; // mulai ulang animasi
      sec.classList.add('fresh');
    }
    tp.outLen = out.length;

    // baris yang sedang dijalankan → disorot di editor VS Code
    if (s.line) vscode.postMessage({ type: 'step', line: s.line, file: s.file || '' });

    highlightPart(partOf(s.line, s));
    highlightPhase(currentLoop && s.loop === currentLoop.id ? { init: 'init', cond: 'cond', post: 'post' }[s.phase] || null : null);

    let label = `Langkah ${tp.i + 1}/${tp.view.length}`;
    if (v.cmpCount != null && tp.stats) label += ` · banding ${v.cmpCount} · tukar ${v.swpCount}`;
    $('tLabel').textContent = label;
    $('tScrub').value = String(tp.i);
  }

  function syncPlay() {
    const pause = 'M3 2h4v12H3zM9 2h4v12H9z', play = 'M3 1v14l11-7z';
    $('tPlay').innerHTML = `<svg viewBox="0 0 16 16" aria-hidden="true"><path d="${tp.playing ? pause : play}"/></svg>`;
    $('tPlay').title = tp.playing ? 'Jeda' : 'Putar';
  }

  function tickTrace(dt) {
    const d = tp.p.duration(tp.i);
    if ((tp.playing || tp.once) && !quiz.open) {
      if (tp.el < d) {
        tp.el = Math.min(d, tp.el + dt * config.speed * (tp.boost[tp.i] || 1));
        fireCues(tp.el / d);
      } else if (tp.i < tp.view.length - 1) {
        if (tp.once) { tp.once = false; } else if (!quizAsk(tp.i + 1)) goto(tp.i + 1, false);
      } else if (tp.playing) {
        if (quiz.on && quiz.qs.length) quizSummary(); // mode tebak: berhenti di akhir dan tampilkan skor
        else {
          tp.hold += dt; // jeda sebentar di akhir, lalu putar ulang
          if (tp.hold > 2500) goto(0, false);
        }
      } else tp.once = false;
    }
    tp.p.render(tp.i, Math.min(1, tp.el / d));
  }

  $('tPlay').addEventListener('click', () => { tp.playing = !tp.playing; tp.once = false; syncPlay(); });
  $('tPrev').addEventListener('click', () => {
    tp.playing = false; tp.once = false; syncPlay();
    goto(tp.i - 1, true);
    tp.el = tp.p.duration(tp.i);
    quizSeek();
  });
  $('tNext').addEventListener('click', () => {
    if (quiz.open) return;
    tp.playing = false; syncPlay();
    if (tp.i < tp.view.length - 1 && !quizAsk(tp.i + 1)) { goto(tp.i + 1, false); tp.once = true; }
  });
  $('tRestart').addEventListener('click', () => { goto(0, false); tp.playing = true; syncPlay(); if (quiz.on) quizStart(); });
  $('tScrub').addEventListener('input', () => {
    tp.playing = false; tp.once = false; syncPlay();
    goto(+$('tScrub').value, true);
    tp.el = tp.p.duration(tp.i);
    quizSeek();
  });

  // ---------- interaksi ----------

  document.addEventListener('click', e => {
    const t = e.target.closest('[data-line]');
    if (t && +t.dataset.line) vscode.postMessage({ type: 'reveal', line: +t.dataset.line });
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
  // Input: dikirim setelah user berhenti mengetik sebentar (atau langsung saat Enter).
  let inputTimer = 0;
  const sendInput = () => { clearTimeout(inputTimer); vscode.postMessage({ type: 'input', text: $('inputBox').value }); };
  $('inputBox').addEventListener('input', () => { clearTimeout(inputTimer); inputTimer = setTimeout(sendInput, 600); });
  $('inputBox').addEventListener('keydown', e => { if (e.key === 'Enter') sendInput(); });
  $('inReroll').addEventListener('click', () => vscode.postMessage({ type: 'reroll' }));
  ['pointerdown', 'keydown'].forEach(ev => document.addEventListener(ev, () => snd.unlock()));

  // ---------- ukuran & tata letak gambar ----------

  const P_clamp = (v, a, b) => Math.max(a, Math.min(b, v));
  let stageScale = 1, fitKey = '';
  /** Lebar tampil canvas bila memenuhi ruang (skala 1): selebar kolomnya, setinggi sisa layar. */
  function fullWidth() {
    const cv = $('stage'), stage = cv.parentElement, aspect = cv.width / cv.height;
    const wide = document.body.classList.contains('wide');
    const availW = Math.max(60, stage.clientWidth - 4);
    const availH = Math.max(80, window.innerHeight - (wide ? 24 : 150)) - 4;
    return Math.min(availW, availH * aspect);
  }
  /** Ukuran tampil canvas + posisi pegangan; dipanggil tiap frame (murah: hanya berubah bila perlu). */
  function fitStage() {
    const cv = $('stage');
    const W = window.innerWidth, H = window.innerHeight;
    document.body.classList.toggle('wide', W >= 560 && W > H * 1.7);
    const key = [W, H, cv.width, cv.height, stageScale, cv.parentElement.clientWidth, document.body.className].join();
    if (key === fitKey) return;
    fitKey = key;
    const w = Math.round(fullWidth() * stageScale), h = Math.round(w * cv.height / cv.width);
    cv.style.width = w + 'px';
    cv.style.height = h + 'px';
    const grip = $('grip');
    if (grip) {
      grip.hidden = false;
      grip.style.left = (cv.offsetLeft + cv.offsetWidth - 16) + 'px';
      grip.style.top = (cv.offsetTop + cv.offsetHeight - 16) + 'px';
    }
  }
  // seret pojok kanan bawah gambar: perkecil/perbesar (25%..100% dari ruang yang ada)
  if ($('grip')) {
    const grip = $('grip');
    let drag = null;
    grip.addEventListener('pointerdown', e => {
      const cv = $('stage');
      drag = { x: e.clientX, y: e.clientY, w: cv.offsetWidth - 4, full: fullWidth(), aspect: cv.width / cv.height };
      grip.setPointerCapture(e.pointerId);
      grip.classList.add('drag');
      e.preventDefault();
    });
    grip.addEventListener('pointermove', e => {
      if (!drag) return;
      const grow = Math.max(e.clientX - drag.x, (e.clientY - drag.y) * drag.aspect);
      stageScale = P_clamp((drag.w + grow) / drag.full, 0.25, 1);
      fitStage();
    });
    const end = () => {
      if (!drag) return;
      drag = null;
      grip.classList.remove('drag');
      config.size = Math.round(stageScale * 100);
      vscode.postMessage({ type: 'setConfig', key: 'size', value: config.size });
    };
    grip.addEventListener('pointerup', end);
    grip.addEventListener('pointercancel', end);
    grip.addEventListener('dblclick', () => {
      stageScale = 1;
      config.size = 100;
      fitStage();
      vscode.postMessage({ type: 'setConfig', key: 'size', value: 100 });
    });
  }

  // ---------- penjelasan error + mode tebak (Fase 12b) ----------

  /** Kartu penjelasan di bawah panggung saat langkahnya adalah error yang menghentikan program. */
  function renderErrCard(e, s) {
    const card = $('errCard');
    card.hidden = !e;
    if (!e) return;
    const src = [s.line ? `baris ${s.line}` : '', e.name && e.msg ? `${e.name}: ${e.msg}` : e.msg || e.name || ''].filter(Boolean).join(' · ');
    card.innerHTML = `<h3>⚠ ${esc(e.title)}</h3><p>${esc(e.text)}</p>` +
      (e.hint ? `<p class="hint"><b>Saran:</b> ${esc(e.hint)}</p>` : '') +
      (src ? `<p class="src">${esc(src)}</p>` : '');
  }

  const quiz = { on: false, qs: [], k: 0, score: 0, open: null, done: false };

  function quizStart() {
    quiz.qs = tp.active ? G.quiz.build(tp.view, { lang: sceneLang(), block: currentBlock, code: tp.code }) : [];
    quiz.k = 0; quiz.score = 0; quiz.open = null; quiz.done = false;
    $('quizCard').hidden = true;
    if (!tp.active) return;
    if (!quiz.qs.length) {
      quizCard(`<p class="q-prompt">Belum ada titik yang bisa ditebak di trace ini (tidak ada kondisi, output, return, atau error).</p>` +
        `<div class="q-actions"><button class="btn" data-qact="off">Tutup</button></div>`);
      return;
    }
    goto(0, false);
    tp.playing = true;
    syncPlay();
  }

  function quizCard(html) {
    $('quizCard').innerHTML = html;
    $('quizCard').hidden = false;
  }

  /** Bila langkah i adalah titik tebakan berikutnya: hentikan pemutar dan tampilkan pertanyaannya. */
  function quizAsk(i) {
    const q = quiz.on && !quiz.open && quiz.qs[quiz.k];
    if (!q || q.at !== i) return false;
    quiz.open = q;
    q.picked = null;
    const vars = (q.vars || []).map(([k, v]) => `<span class="var"><b>${esc(k)}</b> = ${esc(v)}</span>`).join('');
    quizCard(`<div class="q-head"><b>TEBAK ${quiz.k + 1}/${quiz.qs.length}</b><span>★ ${quiz.score}</span></div>` +
      `<p class="q-prompt">${esc(q.prompt)}</p>` +
      (q.code ? `<code class="q-code">${esc(q.code)}</code>` : '') +
      (vars ? `<div class="vars q-vars">${vars}</div>` : '') +
      `<div class="q-opts">${q.options.map((o, j) => `<button class="q-opt" data-qopt="${j}"><b>${G.quiz.LETTERS[j]}</b><span>${esc(o)}</span></button>`).join('')}</div>` +
      `<p class="q-fb" hidden></p><div class="q-actions" hidden><button class="btn primary" data-qact="next">Lanjut ▶</button></div>`);
    snd.play('think');
    return true;
  }

  function quizAnswer(j) {
    const q = quiz.open;
    if (!q || q.picked != null || !q.options[j]) return;
    q.picked = j;
    const ok = j === q.answer;
    if (ok) quiz.score++;
    snd.play(ok ? 'yes' : 'no');
    const card = $('quizCard');
    card.querySelectorAll('.q-opt').forEach((b, k) => {
      b.disabled = true;
      b.classList.toggle('right', k === q.answer);
      b.classList.toggle('wrong', k === j && !ok);
    });
    const fb = card.querySelector('.q-fb');
    fb.hidden = false;
    fb.className = 'q-fb ' + (ok ? 'ok' : 'no');
    fb.innerHTML = (ok ? '✓ Benar!' : `✗ Belum tepat. Jawabannya: <b>${esc(q.options[q.answer])}</b>`) + (q.explain ? `<br><span>${esc(q.explain)}</span>` : '');
    card.querySelector('.q-head span').textContent = `★ ${quiz.score}`;
    card.querySelector('.q-actions').hidden = false;
  }

  /** Lanjut setelah menjawab: langkah yang ditanyakan diputar. */
  function quizNext() {
    const q = quiz.open;
    if (!q || q.picked == null) return;
    quiz.open = null;
    quiz.k++;
    $('quizCard').hidden = true;
    goto(q.at, false);
    tp.playing = true;
    syncPlay();
  }

  /** Setelah menggeser slider/mundur: pertanyaan yang sudah dilewati tidak ditanyakan lagi. */
  function quizSeek() {
    if (!quiz.on) return;
    quiz.open = null;
    quiz.done = false;
    $('quizCard').hidden = true;
    quiz.k = quiz.qs.findIndex(q => q.at > tp.i);
    if (quiz.k < 0) quiz.k = quiz.qs.length;
  }

  function quizSummary() {
    if (quiz.done) return;
    quiz.done = true;
    const n = quiz.qs.length, s = quiz.score;
    const stars = '★'.repeat(s) + '☆'.repeat(Math.max(0, n - s));
    const msg = s === n ? 'Sempurna! Kamu paham alurnya.' : s * 2 >= n ? 'Bagus! Coba ulangi untuk skor penuh.' : 'Tidak apa-apa, tonton lagi alurnya lalu coba lagi.';
    quizCard(`<div class="q-head"><b>SELESAI</b><span>${s}/${n}</span></div><p class="q-stars">${stars}</p><p class="q-prompt">${msg}</p>` +
      `<div class="q-actions"><button class="btn primary" data-qact="again">Ulangi</button><button class="btn" data-qact="off">Selesai</button></div>`);
    snd.play(s === n ? 'jingle' : 'coin');
  }

  function setQuiz(on) {
    quiz.on = on;
    $('tQuiz').setAttribute('aria-pressed', String(on));
    if (on) quizStart();
    else { quiz.open = null; $('quizCard').hidden = true; }
  }

  $('tQuiz').addEventListener('click', () => setQuiz(!quiz.on));
  $('quizCard').addEventListener('click', e => {
    const opt = e.target.closest('[data-qopt]');
    if (opt) { quizAnswer(+opt.dataset.qopt); return; }
    const act = e.target.closest('[data-qact]');
    if (!act) return;
    if (act.dataset.qact === 'next') quizNext();
    else if (act.dataset.qact === 'again') quizStart();
    else setQuiz(false);
  });
  document.addEventListener('keydown', e => {
    if (!quiz.open || e.target.tagName === 'INPUT') return;
    const k = '1234'.indexOf(e.key) >= 0 ? '1234'.indexOf(e.key) : 'abcd'.indexOf(e.key.toLowerCase());
    if (k >= 0 && quiz.open.picked == null) { quizAnswer(k); e.preventDefault(); }
    else if (e.key === 'Enter' && quiz.open.picked != null) { quizNext(); e.preventDefault(); }
  });

  // ---------- lomba algoritma (Fase 12c) ----------

  const raceView = new G.race.RaceView($('stage'));
  let raceSel = [], raceN = 8, raceShown = false;
  const funcBlocks = () => blocks.filter(b => b.kind === 'func' && b.name && b.name !== 'main' && b.state !== 'incomplete');

  $('tRace').addEventListener('click', () => {
    const fs = funcBlocks();
    const cur = currentBlock && currentBlock.kind === 'func' && currentBlock.name !== 'main' ? currentBlock.id : null;
    const sorty = b => /sort|urut|search|cari/i.test(b.name || '');
    const first = cur || (fs.find(sorty) || fs[0] || {}).id;
    const second = (fs.find(b => b.id !== first && sorty(b)) || fs.find(b => b.id !== first) || {}).id;
    raceSel = [first, second].filter(Boolean);
    $('raceList').innerHTML = fs.length
      ? fs.map(b => `<label><input type="checkbox" value="${esc(b.id)}"${raceSel.includes(b.id) ? ' checked' : ''}> ${esc(b.header || b.name)} <small>· baris ${b.line}</small></label>`).join('')
      : '<p>Belum ada fungsi di file ini.</p>';
    $('raceGo').disabled = false;
    syncRaceDialog();
    $('raceDialog').hidden = false;
  });
  $('raceList').addEventListener('change', e => {
    const id = e.target.value;
    if (e.target.checked) { raceSel.push(id); if (raceSel.length > 2) raceSel.shift(); }
    else raceSel = raceSel.filter(x => x !== id);
    $('raceList').querySelectorAll('input').forEach(el => { el.checked = raceSel.includes(el.value); });
    syncRaceDialog();
  });
  function syncRaceDialog() {
    $('raceGo').disabled = raceSel.length !== 2;
    $('raceNote').textContent = raceSel.length === 2 ? '' : 'Pilih tepat 2 fungsi.';
  }
  $('raceCancel').addEventListener('click', () => { $('raceDialog').hidden = true; });
  $('raceGo').addEventListener('click', () => {
    if (raceSel.length !== 2) return;
    raceN = +(radio('raceN') || 8);
    $('raceGo').disabled = true;
    $('raceNote').textContent = 'Menyiapkan lomba… (setiap fungsi juga diukur untuk n = 5, 10, 20, 40)';
    vscode.postMessage({ type: 'race', blocks: raceSel, n: raceN });
  });

  /** Hasil lomba dari server: mulai lomba di panggung, isi kartu grafik. */
  function applyRace(m) {
    $('raceGo').disabled = false;
    if (m.error || !m.result) { $('raceNote').textContent = `Lomba gagal: ${m.error || 'tidak ada hasil'}`; return; }
    const res = m.result;
    $('raceDialog').hidden = true;
    if (quiz.on) setQuiz(false);
    raceView.start(res, blocks, loops);
    raceShown = false;
    for (const id of ['tracebar', 'errCard', 'quizCard', 'stepNote']) $(id).hidden = true;
    const names = raceView.lanes.map(l => l.name);
    $('raceTitle').textContent = `LOMBA · ${names.join(' vs ')}`;
    $('raceInput').textContent = `Input sama untuk keduanya (n = ${res.input.length}): [${res.input.join(', ')}]`;
    $('racePlay').textContent = 'Jeda';
    $('raceResult').innerHTML = '';
    $('raceCard').hidden = false;
    G.race.drawChart($('raceChart'), res.sizes, res.runs, names);
  }

  /** Setelah kedua lajur finis: langkah, pemenang, dan tebakan Big-O tiap fungsi. */
  function raceSummary() {
    const res = raceView.res, win = raceView.winner();
    $('raceResult').innerHTML = raceView.lanes.map((l, k) => {
      const w = res.runs[k].work, o = G.race.bigO(res.sizes, w);
      const ops = w.map((x, j) => `n=${res.sizes[j]}: ${x > 0 ? x : '—'}`).join(' · ');
      return `<div class="race-lane${win === k ? ' win' : ''}"><b>${esc(l.name)}${win === k ? ' 🏆' : ''}</b>` +
        (l.st ? `${l.steps} langkah · kira-kira O(${esc(o)})<br><small>${esc(ops)}</small>` : esc('gagal: ' + l.failed)) + '</div>';
    }).join('') + raceLesson(res, win);
    snd.play(win != null && win >= 0 ? 'jingle' : 'coin');
  }

  /** Kalimat pelajaran: pemenang di n kecil belum tentu paling hemat untuk input besar. */
  function raceLesson(res, win) {
    if (win == null || win < 0 || raceView.lanes.length !== 2) return '';
    const RANK = { '1': 0, 'n': 1, 'n log n': 2, 'n²': 3, 'n³': 4 };
    const lose = 1 - win, o = res.runs.map(r => G.race.bigO(res.sizes, r.work));
    const nw = raceView.lanes[win].name, nl = raceView.lanes[lose].name;
    if (!(o[win] in RANK) || !(o[lose] in RANK)) return '';
    let msg;
    if (RANK[o[win]] > RANK[o[lose]]) msg = `${nw} menang karena list-nya masih pendek (n = ${res.input.length}). Tapi lihat grafik: untuk input besar, ${nl} (O(${o[lose]})) jauh lebih hemat daripada ${nw} (O(${o[win]})).`;
    else if (RANK[o[win]] < RANK[o[lose]]) msg = `${nw} menang, dan grafiknya juga tumbuh paling lambat (O(${o[win]})): makin besar input, makin jauh selisihnya.`;
    else msg = `Keduanya tumbuh dengan laju yang mirip (O(${o[win]})); bedanya hanya di jumlah langkah per elemen.`;
    return `<p class="race-note race-lesson">${esc(msg)}</p>`;
  }

  function exitRace() {
    raceView.stop();
    $('raceCard').hidden = true;
    if (tp.active) { $('tracebar').hidden = false; $('stepNote').hidden = false; goto(tp.i, true); tp.el = tp.p.duration(tp.i); }
  }
  $('racePlay').addEventListener('click', () => { raceView.playing = !raceView.playing; $('racePlay').textContent = raceView.playing ? 'Jeda' : 'Putar'; });
  $('raceAgain').addEventListener('click', () => { raceView.restart(); raceShown = false; $('raceResult').innerHTML = ''; $('racePlay').textContent = 'Jeda'; });
  $('raceReroll').addEventListener('click', () => vscode.postMessage({ type: 'race', blocks: raceView.res.runs.map(r => r.block), n: raceN, reroll: true }));
  $('raceExit').addEventListener('click', exitRace);

  // ---------- rekam animasi (Fase 12a) ----------

  const REC = G.recorder;
  let recording = false, recCancel = false;
  const allStages = () => [level, data, branch, func, lanes, graph, cls, isles, maze, pohon, belah];
  const sleep = ms => new Promise(r => setTimeout(r, ms));
  const radio = name => (document.querySelector(`input[name="${name}"]:checked`) || {}).value;

  const titleCase = s => s.toLowerCase().replace(/(^|\s)\S/g, m => m.toUpperCase());

  /** Judul otomatis: jenis panggung + nama blok. */
  function autoTitle() {
    const tr = tp.trace || {};
    if (tr.grid) return `${tr.grid.kind === 'dfs' ? 'DFS' : 'BFS'} Labirin`;
    if (tr.tree) return 'DFS Pohon';
    if (tr.graph) return `${(tr.graph.kind || 'DFS').toUpperCase()} Graf`;
    if (tr.class) return `Class ${tr.class.name}`;
    if (tp.r === dataR && data.elim) return 'Pencarian';
    if (tp.r === mergeR) return titleCase(belah.title);
    if (tp.r === dataR && data.sort) return titleCase(data.sort.title);
    const b = currentBlock;
    if (!b) return 'GopherViz';
    if (b.kind === 'func') return `Fungsi ${b.name || ''}`.trim();
    return kindLabel(b).replace(/ · .*$/, '');
  }

  /** Perkiraan lama rekaman (detik) dengan percepatan otomatis, maks 60 detik. */
  function recPlan() {
    const durs = tp.view.map((_, i) => tp.p.duration(i));
    const real = durs.reduce((a, d, i) => a + d / (tp.boost[i] || 1), 0) + 1500;
    return { durs, total: real, speed: Math.max(1, real / 60000) };
  }

  $('tRec').addEventListener('click', () => {
    if (!tp.active || recording) return;
    $('recTitle').value = autoTitle();
    $('recProgress').hidden = true;
    $('recGo').disabled = false;
    const sec = Math.round(Math.min(60000, recPlan().total) / 1000);
    $('recNote').textContent = `Seluruh langkah direkam (±${sec} detik, bagian berulang dipercepat). MP4 memakai ffmpeg di komputermu (atur di setting "GopherViz: Ffmpeg Path").`;
    syncRecDialog();
    $('recDialog').hidden = false;
  });
  const syncRecDialog = () => { $('recTitleRow').hidden = radio('recFrame') !== 'reel'; };
  document.querySelectorAll('input[name="recFrame"]').forEach(el => el.addEventListener('change', syncRecDialog));
  $('recCancel').addEventListener('click', () => {
    if (recording) recCancel = true;
    else $('recDialog').hidden = true;
  });
  $('recGo').addEventListener('click', () => {
    if (recording) return;
    void recordTrace(radio('recFrame') || 'stage', radio('recFormat') || 'gif', $('recTitle').value.trim() || autoTitle());
  });

  /**
   * Memutar trace dari awal sampai akhir di luar layar dan merekam setiap frame. GIF dirender
   * secepat mungkin; video (WebM/MP4) direkam waktu-nyata lewat MediaRecorder.
   */
  async function recordTrace(frame, format, title) {
    recording = true;
    recCancel = false;
    $('recGo').disabled = true;
    $('recProgress').hidden = false;
    const wasPlaying = tp.playing, wasI = tp.i;
    tp.playing = false;
    const progress = (f, label) => { $('recBar').style.width = `${Math.round(f * 100)}%`; $('recNote').textContent = label; };
    try {
      const fps = format === 'gif' ? 12 : 30, frameMs = 1000 / fps;
      const { durs, total, speed } = recPlan();
      const nFrames = Math.ceil(total / speed / frameMs);
      const stageCv = $('stage');
      tp.p.render(0, 0);
      const { w, h } = REC.frameSize(frame, format, stageCv.width, stageCv.height);
      const out = document.createElement('canvas');
      out.width = w; out.height = h;
      const octx = out.getContext('2d', { willReadFrequently: true });
      const lang = sceneLang();
      const sub = `${LANG_NAME[lang] || lang || ''}${currentBlock ? ' · ' + currentBlock.id : ''}`;
      let gif = null, rec = null;
      const chunks = [];
      if (format === 'gif') gif = new REC.GifWriter(w, h);
      else {
        const mime = ['video/webm;codecs=vp9', 'video/webm;codecs=vp8', 'video/webm'].find(m => window.MediaRecorder && MediaRecorder.isTypeSupported(m));
        if (!mime) throw new Error('perekam video tidak tersedia di VS Code ini; pilih GIF');
        rec = new MediaRecorder(out.captureStream(fps), { mimeType: mime, videoBitsPerSecond: 5_000_000 });
        rec.ondataavailable = e => { if (e.data && e.data.size) chunks.push(e.data); };
        rec.start(250);
      }
      let i = 0, el = 0, cues = tp.p.cues(0).sort((a, b) => a.t - b.t), ci = 0;
      let output = tp.view[0].step.output || '';
      const fire = t => { while (ci < cues.length && cues[ci].t <= t) { const c = cues[ci++]; if (c.fx) c.fx(); } };
      const t0 = performance.now();
      for (let f = 0; f < nFrames && !recCancel; f++) {
        const adv = frameMs * speed;
        allStages().forEach(s => s.update(adv));
        let left = adv;
        while (left > 0) { // maju di trace (langkah dipercepat seperti saat diputar biasa)
          const d = durs[i], b = tp.boost[i] || 1, need = (d - el) / b;
          if (left < need) { el += left * b; left = 0; fire(el / d); break; }
          el = d; left -= need; fire(1);
          if (i < tp.view.length - 1) {
            i++; el = 0; ci = 0;
            cues = tp.p.cues(i).sort((a, b) => a.t - b.t);
            output += tp.view[i].step.output || '';
          } else left = 0;
        }
        tp.p.render(i, Math.min(1, el / durs[i]));
        const st = tp.view[i].step;
        REC.compose(octx, w, h, frame, stageCv, { title, sub, code: tp.code, line: st.line, file: st.file, output, startLine: currentBlock ? currentBlock.line : 1 });
        if (gif) {
          gif.addFrame(octx.getImageData(0, 0, w, h), frameMs);
          if (f % 3 === 0) { progress((f + 1) / nFrames, `Membuat GIF… ${Math.round(((f + 1) / nFrames) * 100)}%`); await sleep(0); }
        } else {
          progress((f + 1) / nFrames, `Merekam video… ${Math.round((f + 1) / fps)} / ${Math.round(nFrames / fps)} detik`);
          const wait = t0 + (f + 1) * frameMs - performance.now();
          if (wait > 0) await sleep(wait);
        }
      }
      if (recCancel) throw new Error('dibatalkan');
      let bytes;
      if (gif) bytes = gif.finish();
      else {
        await new Promise(res => { rec.onstop = res; rec.stop(); });
        bytes = new Uint8Array(await new Blob(chunks, { type: 'video/webm' }).arrayBuffer());
      }
      const name = title.replace(/[^\w\- ]+/g, '').trim().replace(/\s+/g, '-').toLowerCase() || 'gopherviz';
      progress(1, 'Menyimpan…');
      vscode.postMessage({ type: 'export', format, name, data: bytes });
      $('recDialog').hidden = true;
    } catch (err) {
      $('recNote').textContent = `Rekaman gagal: ${err && err.message ? err.message : err}`;
      $('recProgress').hidden = true;
      $('recGo').disabled = false;
    } finally {
      recording = false;
      goto(wasI, true);
      tp.el = tp.p.duration(tp.i);
      tp.playing = wasPlaying;
    }
  }

  // ---------- loop animasi ----------

  let last = performance.now();
  function frame(now) {
    const dt = Math.min(64, now - last);
    last = now;
    level.update(dt);
    data.update(dt);
    branch.update(dt);
    func.update(dt);
    lanes.update(dt);
    graph.update(dt);
    cls.update(dt);
    isles.update(dt);
    maze.update(dt);
    pohon.update(dt);
    belah.update(dt);
    if (recording) { requestAnimationFrame(frame); return; } // perekam yang menggambar panggung
    if (raceView.active) { // lomba menggantikan panggung biasa sampai user keluar
      raceView.update(dt, config.speed);
      raceView.render();
      if (!raceShown && raceView.finished()) { raceShown = true; raceSummary(); }
      snd.music(config.music && raceView.playing && document.visibilityState === 'visible');
      fitStage();
      requestAnimationFrame(frame);
      return;
    }
    if (tp.active) tickTrace(dt);
    else if (currentBlock && (currentBlock.kind === 'if' || currentBlock.kind === 'switch')) branch.renderIdle();
    else if (currentBlock && currentBlock.kind === 'func') func.renderIdle();
    else if (currentBlock && currentBlock.kind === 'class') cls.renderIdle();
    else level.render();
    const playing = !!currentBlock && currentBlock.state === 'complete' && document.visibilityState === 'visible' &&
      (!tp.active || tp.playing);
    snd.music(config.music && playing);
    fitStage();
    requestAnimationFrame(frame);
  }

  G.app = { level, data, branch, func, tp, tickTrace, goto, quiz, setQuiz, quizAnswer, quizNext, raceView, applyRace }; // untuk dev/harness.html
  snd.unlock(); // di VS Code suara kadang boleh langsung; kalau tidak, menunggu klik
  applyConfig(config);
  overlay('Menyiapkan…');
  requestAnimationFrame(frame);
  vscode.postMessage({ type: 'ready' });
})(window.GV = window.GV || {});
