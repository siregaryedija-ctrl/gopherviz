// Menjalankan test/vscode/suite.ts di dalam VS Code terpisah yang diunduh ke .vscode-test/
// (tidak menyentuh VS Code yang sedang kamu pakai).
import * as esbuild from 'esbuild';
import { runTests } from '@vscode/test-electron';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import fs from 'node:fs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const suite = path.join(root, 'out', 'test', 'vscode', 'suite.js');

await esbuild.build({
  entryPoints: [path.join(root, 'test', 'vscode', 'suite.ts')],
  outfile: suite,
  bundle: true,
  platform: 'node',
  format: 'cjs',
  target: 'node20',
  external: ['vscode'],
  logLevel: 'warning',
});

const opts = {
  extensionDevelopmentPath: root,
  extensionTestsPath: suite,
  launchArgs: [path.join(root, '..', 'testdata', 'loops'), '--disable-extensions', '--skip-welcome', '--skip-release-notes'],
};

/** Versi VS Code terbaru yang sudah ada di .vscode-test (untuk dipakai saat tidak bisa mengunduh). */
function cachedVersion() {
  const dirs = fs.existsSync(path.join(root, '.vscode-test')) ? fs.readdirSync(path.join(root, '.vscode-test')) : [];
  const vers = dirs.map(d => /-(\d+\.\d+\.\d+)$/.exec(d)).filter(Boolean).map(m => m[1]);
  vers.sort((a, b) => a.localeCompare(b, undefined, { numeric: true }));
  return vers.pop();
}

try {
  try {
    await runTests(opts);
  } catch (e) {
    const cached = cachedVersion();
    if (!cached || !/download/i.test(String(e && e.message))) throw e;
    console.warn(`VS Code terbaru tidak bisa diunduh; memakai versi di cache (${cached}).`);
    await runTests({ ...opts, version: cached });
  }
} catch (e) {
  console.error('test VS Code gagal:', e instanceof Error ? e.message : e);
  process.exit(1);
}
