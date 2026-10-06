// Membuat dev/samples.js: scene + trace asli dari binary gopherviz untuk dev/harness.html.
//   node scripts/dev-samples.mjs   (jalankan npm run build:bin dulu)
import { execFileSync } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const exe = path.join(root, 'bin', `${process.platform}-${process.arch}`, process.platform === 'win32' ? 'gopherviz.exe' : 'gopherviz');
const testdata = path.join(root, '..', 'testdata');

// nama sampel → [file relatif terhadap testdata, baris di dalam blok fokus]
const samples = {
  for3: ['loops/01_for3.go', 6],
  bubble: ['loops/11_nested_bubble.go', 8],
  inner: ['loops/11_nested_bubble.go', 10],
  range: ['loops/05_range_slice.go', 8],
  labeled: ['loops/12_labeled.go', 8],
  infinite: ['loops/03_infinite_break.go', 7],
  ifchain: ['blocks/01_if_chain.go', 6],
  ifskip: ['blocks/01_if_chain.go', 22],
  switch: ['blocks/02_switch.go', 6],
  fib: ['blocks/03_recursion.go', 12],
  fact: ['blocks/03_recursion.go', 5],
  program: ['blocks/04_program.go', 19],
  structs: ['data/01_struct_method.go', 18],
  pointers: ['data/02_pointer.go', 14],
  linked: ['data/03_linked_list.go', 22],
  wordcount: ['data/04_map_count.go', 8],
  defer: ['data/05_defer_recover.go', 26],
  errors: ['data/06_errors.go', 34],
};

const run = (...args) => JSON.parse(execFileSync(exe, args, { encoding: 'utf8' }));
const out = {};
for (const [name, [file, line]] of Object.entries(samples)) {
  const f = path.join(testdata, file);
  const scene = run('list', '--json', f);
  const trace = run('trace', '--json', `${f}:${line}`);
  out[name] = { file: path.basename(file), scene, focusBlock: trace.block.id, trace };
}
writeFileSync(path.join(root, 'dev', 'samples.js'), `// Dibuat oleh scripts/dev-samples.mjs, jangan diedit manual.\nwindow.SAMPLES = ${JSON.stringify(out)};\n`);
console.log(`dev/samples.js: ${Object.keys(out).join(', ')}`);
