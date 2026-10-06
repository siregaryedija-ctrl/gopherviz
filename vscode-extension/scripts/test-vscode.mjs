// Menjalankan test/vscode/suite.ts di dalam VS Code terpisah yang diunduh ke .vscode-test/
// (tidak menyentuh VS Code yang sedang kamu pakai).
import * as esbuild from 'esbuild';
import { runTests } from '@vscode/test-electron';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

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

try {
  await runTests({
    extensionDevelopmentPath: root,
    extensionTestsPath: suite,
    launchArgs: [path.join(root, '..', 'testdata', 'loops'), '--disable-extensions', '--skip-welcome', '--skip-release-notes'],
  });
} catch (e) {
  console.error('test VS Code gagal:', e instanceof Error ? e.message : e);
  process.exit(1);
}
