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

test('ekstensi aktif saat file Go dibuka dan scene terkirim', async api => {
  const doc = await vscode.workspace.openTextDocument(file);
  await vscode.window.showTextDocument(doc);
  const res = await waitFor('hasil update', () => api.result(doc.uri.toString()));
  assert.deepEqual(res.scene.loops.map(l => l.id), ['L1', 'L2']);
});

test('kursor di loop dalam: fokus L2 dan panel terbuka otomatis', async api => {
  const ed = vscode.window.activeTextEditor!;
  const pos = new vscode.Position(10, 4); // baris 11: if a[j] > a[j+1]
  ed.selection = new vscode.Selection(pos, pos);
  await waitFor('fokus L2', () => api.focus(ed.document.uri.toString()) === 'L2');
  await waitFor('panel terbuka', () => api.panelOpen());
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

test('CodeLens muncul di atas setiap for', async () => {
  const ed = vscode.window.activeTextEditor!;
  const lenses = await vscode.commands.executeCommand<vscode.CodeLens[]>('vscode.executeCodeLensProvider', ed.document.uri);
  const titles = lenses.map(l => l.command?.title ?? '');
  assert.ok(titles.includes('▶ Visualisasikan loop L1'), titles.join(', '));
  assert.equal(titles.filter(t => t.startsWith('▶')).length, 3);
});

export async function run(): Promise<void> {
  const ext = vscode.extensions.getExtension<TestApi>('goloopviz.goloopviz');
  assert.ok(ext, 'ekstensi goloopviz tidak ditemukan');
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
