// Test integrasi di dalam VS Code sungguhan (dijalankan oleh scripts/test-vscode.mjs).
import * as vscode from 'vscode';
import * as path from 'node:path';
import assert from 'node:assert/strict';
import type { TestApi } from '../../src/extension';

async function waitFor<T>(what: string, fn: () => T | undefined | false, timeoutMs = 10_000): Promise<T> {
  const end = Date.now() + timeoutMs;
  for (;;) {
    const v = fn();
    if (v) return v;
    if (Date.now() > end) throw new Error(`waktu habis menunggu: ${what}`);
    await new Promise(r => setTimeout(r, 50));
  }
}

const tests: [string, (api: TestApi) => Promise<void>][] = [];
const test = (name: string, fn: (api: TestApi) => Promise<void>) => tests.push([name, fn]);

// __dirname = vscode-extension/out/test/vscode
const file = path.resolve(__dirname, '..', '..', '..', '..', 'testdata', 'loops', '11_nested_bubble.go');
const file0 = file;

test('ekstensi aktif saat file Go dibuka dan scene terkirim', async api => {
  const doc = await vscode.workspace.openTextDocument(file);
  await vscode.window.showTextDocument(doc);
  const res = await waitFor('hasil update', () => api.result(doc.uri.toString()));
  assert.deepEqual(res.scene.loops.map(l => l.id), ['L1', 'L2']);
  assert.deepEqual(res.scene.blocks.map(b => b.id), ['F1', 'L1', 'L2', 'I1', 'I2']);
});

test('kursor di loop dalam: fokus L2 dan panel terbuka otomatis', async api => {
  const ed = vscode.window.activeTextEditor!;
  const pos = new vscode.Position(9, 4); // baris 10: for j := 0; ...
  ed.selection = new vscode.Selection(pos, pos);
  await waitFor('fokus L2', () => api.focus(ed.document.uri.toString()) === 'L2');
  await waitFor('panel terbuka', () => api.panelOpen());
});

test('kursor di if: fokus ke blok if terdalam', async api => {
  const ed = vscode.window.activeTextEditor!;
  const pos = new vscode.Position(11, 6); // baris 12: a[j], a[j+1] = ... di dalam if
  ed.selection = new vscode.Selection(pos, pos);
  await waitFor('fokus I1', () => api.focus(ed.document.uri.toString()) === 'I1');
  ed.selection = new vscode.Selection(new vscode.Position(9, 4), new vscode.Position(9, 4));
  await waitFor('kembali ke L2', () => api.focus(ed.document.uri.toString()) === 'L2');
});

test('trace nilai asli dikirim ke panel setelah berhenti mengetik', async api => {
  const ed = vscode.window.activeTextEditor!;
  const uri = ed.document.uri.toString();
  await waitFor('trace L2', () => api.traceKey().startsWith(uri) && api.traceKey().split('|')[2] === 'L2');
});

test('panel mengirim baris yang sedang jalan untuk disorot di editor', async api => {
  const ed = vscode.window.activeTextEditor!;
  const l2 = api.result(ed.document.uri.toString())!.scene.blocks.find(b => b.id === 'L2')!;
  await waitFor('baris langkah di dalam L2', () => api.stepLine() >= l2.line && api.stepLine() <= l2.endLine);
});

test('mengetik loop baru tanpa menyimpan langsung terlihat (realtime)', async api => {
  const ed = vscode.window.activeTextEditor!;
  await ed.edit(e => e.insert(new vscode.Position(6, 0), '\tfor k := range 3 {\n\t}\n'));
  const uri = ed.document.uri.toString();
  const res = await waitFor('3 loop', () => {
    const r = api.result(uri);
    return r && r.scene.loops.length === 3 ? r : undefined;
  });
  assert.equal(res.scene.loops[0].kind, 'range-int');
  assert.deepEqual(res.scene.loops[0].body, []);
  assert.equal(ed.document.isDirty, true);
});

test('CodeLens muncul di atas setiap blok', async () => {
  const ed = vscode.window.activeTextEditor!;
  const lenses = await vscode.commands.executeCommand<vscode.CodeLens[]>('vscode.executeCodeLensProvider', ed.document.uri);
  const titles = lenses.map(l => l.command?.title ?? '');
  for (const want of ['▶ Visualisasikan loop L1', '▶ Visualisasikan fungsi F1', '▶ Visualisasikan if I1']) {
    assert.ok(titles.includes(want), titles.join(', '));
  }
  assert.equal(titles.filter(t => t.startsWith('▶')).length, 6); // main, 3 loop, 2 if
});

test('goroutine dan channel: trace membawa jalur goroutine dan isi channel', async api => {
  const conc = path.resolve(path.dirname(file), '..', 'conc', '03_workers.go');
  const ed = await vscode.workspace.openTextDocument(conc).then(d => vscode.window.showTextDocument(d));
  ed.selection = new vscode.Selection(12, 1, 12, 1); // baris 13: di dalam main, di luar loop
  const uri = ed.document.uri.toString();
  await waitFor('trace F2', () => api.traceKey().startsWith(uri) && api.traceKey().split('|')[2] === 'F2');
  const tr = api.lastTrace()!;
  assert.equal(tr.mode, 'simulated');
  assert.ok(tr.steps.some(s => s.gs?.length === 3), 'tidak ada potret 3 goroutine');
  assert.ok(tr.steps.some(s => s.ops?.some(o => o.op === 'send' && o.key === 'ch1')), 'tidak ada kirim ke jobs');
});

test('bahasa lain: JavaScript dan Python dianalisis dengan parsernya sendiri', async api => {
  for (const [dir, file, lang, loops] of [['javascript', '01_basics.js', 'javascript', 5], ['python', '01_basics.py', 'python', 4]] as const) {
    const doc = await vscode.workspace.openTextDocument(path.resolve(path.dirname(file0), '..', dir, file));
    await vscode.window.showTextDocument(doc);
    const res = await waitFor(`scene ${lang}`, () => api.result(doc.uri.toString()));
    assert.equal(res.scene.language, lang);
    assert.equal(res.scene.loops.length, loops);
    assert.deepEqual(res.diagnostics, []);
    const lenses = await vscode.commands.executeCommand<vscode.CodeLens[]>('vscode.executeCodeLensProvider', doc.uri);
    assert.ok(lenses.some(l => l.command?.title === '▶ Visualisasikan loop L1'), 'lens L1 tidak ada');
  }
});

test('kotak Input: nilai dari user dipakai simulasi', async api => {
  const doc = await vscode.workspace.openTextDocument(path.resolve(path.dirname(file0), '..', 'python', '04_input.noracle.py'));
  const ed = await vscode.window.showTextDocument(doc);
  const pos = new vscode.Position(3, 6); // baris 4: if 0 <= usia <= 5:
  ed.selection = new vscode.Selection(pos, pos);
  const tr = await waitFor('trace I1 yang membaca input', () => {
    const t = api.lastTrace();
    return t && t.block.id === 'I1' && t.readsInput ? t : undefined;
  });
  assert.ok(tr.warnings.some(w => w.includes('angka acak')), tr.warnings.join(' | '));
  api.panelMessage({ type: 'input', text: '15, 1, 2, 3' }); // program membaca 4 input
  const t2 = await waitFor('trace dengan usia = 15', () => {
    const t = api.lastTrace();
    return t && t.steps.some(s => s.vars && s.vars.usia === '15') ? t : undefined;
  });
  assert.ok(!t2.warnings.some(w => w.includes('angka acak')), t2.warnings.join(' | '));
  api.panelMessage({ type: 'input', text: '' });
});

test('class: kursor di baris class menampilkan cetak biru dan object-objectnya', async api => {
  const doc = await vscode.workspace.openTextDocument(path.resolve(path.dirname(file0), '..', 'python', '02_lanjut.py'));
  const ed = await vscode.window.showTextDocument(doc);
  const uri = doc.uri.toString();
  const res = await waitFor('scene python', () => api.result(uri));
  const cls = res.scene.blocks.find(b => b.kind === 'class' && b.name === 'Vector');
  assert.ok(cls, 'blok class Vector tidak ada');
  const pos = new vscode.Position(cls.line - 1, 2);
  ed.selection = new vscode.Selection(pos, pos);
  const tr = await waitFor('trace class Vector', () => {
    const t = api.lastTrace();
    return t && t.block.id === cls.id && t.mode === 'simulated' ? t : undefined;
  }, 20_000);
  assert.equal(tr.class?.name, 'Vector');
  assert.ok(tr.steps.some(s => (s.objs || []).length > 1), 'object tidak terekam');
  assert.ok(tr.steps.some(s => s.phase === 'call' && s.self), 'langkah method tanpa self');
});

test('multi-file: fungsi di modul dijalankan dari file yang meng-import-nya', async api => {
  const util = path.resolve(__dirname, '..', '..', '..', 'dev', 'fixtures', 'modpy', 'utils.py');
  const doc = await vscode.workspace.openTextDocument(util);
  const ed = await vscode.window.showTextDocument(doc);
  const uri = doc.uri.toString();
  const res = await waitFor('scene utils.py', () => api.result(uri));
  const fn = res.scene.blocks.find(b => b.kind === 'func' && b.name === 'total');
  assert.ok(fn, 'fungsi total tidak ada');
  const pos = new vscode.Position(fn.line, 4);
  ed.selection = new vscode.Selection(pos, pos);
  const tr = await waitFor('trace total dari main.py', () => {
    const t = api.lastTrace();
    return t && t.block.id === fn.id && t.mode === 'simulated' ? t : undefined;
  }, 20_000);
  assert.equal(tr.entry, 'main.py');
  assert.deepEqual(tr.files, ['utils.py', 'main.py']);
  assert.equal(tr.steps[0].file, 'main.py');
});

test('panel bisa dipindah: panel bawah, sidebar, lalu kembali ke tab editor', async api => {
  const doc = vscode.window.activeTextEditor!.document;
  const cfg = () => vscode.workspace.getConfiguration('gopherviz');
  try {
    await cfg().update('location', 'panel', vscode.ConfigurationTarget.Global);
    await waitFor('view panel bawah siap', () => api.panelPlace() === 'view');
    await cfg().update('location', 'sidebar', vscode.ConfigurationTarget.Global);
    await waitFor('view sidebar siap', () => api.panelPlace() === 'view');
  } finally {
    await cfg().update('location', undefined, vscode.ConfigurationTarget.Global);
  }
  await waitFor('tab editor siap', () => api.panelPlace() === 'editor');
  await vscode.window.showTextDocument(doc);
});

export async function run(): Promise<void> {
  const ext = vscode.extensions.getExtension<TestApi>('gopherviz.gopherviz');
  assert.ok(ext, 'ekstensi gopherviz tidak ditemukan');
  await vscode.workspace.openTextDocument(file).then(d => vscode.window.showTextDocument(d));
  const api = await ext.activate();
  let failed = 0;
  for (const [name, fn] of tests) {
    try {
      await fn(api);
      console.log(`  ✔ ${name}`);
    } catch (e) {
      failed++;
      console.log(`  ✖ ${name}\n    ${e instanceof Error ? e.message : e}`);
    }
  }
  await vscode.commands.executeCommand('workbench.action.revertAndCloseActiveEditor');
  if (failed) throw new Error(`${failed} test gagal`);
}
