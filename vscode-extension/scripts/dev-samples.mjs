// Membuat dev/samples.js: scene + trace asli dari binary gopherviz untuk dev/harness.html.
//   node scripts/dev-samples.mjs   (jalankan npm run build:bin dulu)
import { execFileSync } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const exe = path.join(root, 'bin', `${process.platform}-${process.arch}`, process.platform === 'win32' ? 'gopherviz.exe' : 'gopherviz');
const loops = path.join(root, '..', 'testdata', 'loops');

// nama sampel → [file, baris loop fokus]
const samples = {
  for3: ['01_for3.go', 6],
  bubble: ['11_nested_bubble.go', 8],
  inner: ['11_nested_bubble.go', 10],
  range: ['05_range_slice.go', 8],
  labeled: ['12_labeled.go', 8],
  infinite: ['03_infinite_break.go', 7],
};

const run = (...args) => JSON.parse(execFileSync(exe, args, { encoding: 'utf8' }));
const out = {};
for (const [name, [file, line]] of Object.entries(samples)) {
  const f = path.join(loops, file);
  const scene = run('list', '--json', f);
  const trace = run('trace', '--json', `${f}:${line}`);
  out[name] = { file, scene, focusLoop: trace.loop.id, trace };
}
writeFileSync(path.join(root, 'dev', 'samples.js'), `// Dibuat oleh scripts/dev-samples.mjs, jangan diedit manual.\nwindow.SAMPLES = ${JSON.stringify(out)};\n`);
console.log(`dev/samples.js: ${Object.keys(out).join(', ')}`);
