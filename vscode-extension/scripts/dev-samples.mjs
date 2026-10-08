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
  unbuffered: ['conc/01_unbuffered.go', 14],
  waitgroup: ['conc/02_waitgroup.go', 13],
  workers: ['conc/03_workers.go', 13],
  mutex: ['conc/04_mutex.go', 21],
  select: ['conc/05_select.go', 9],
  pingpong: ['conc/06_pingpong.go', 20],
  deadlock: ['conc/07_deadlock.go', 6],
  js_for: ['javascript/01_basics.js', 12],
  js_bubble: ['javascript/03_algorithms.js', 6],
  js_class: ['javascript/03_algorithms.js', 67],
  ts_shapes: ['typescript/02_classes.ts', 50],
  js_if: ['javascript/01_basics.js', 18],
  py_for: ['python/01_basics.py', 16],
  py_if_input: ['python/04_input.noracle.py', 4],
  php_foreach: ['php/01_basics.php', 12],
  java_switch: ['java/01_basics.java', 37],
  java_bubble: ['java/03_koleksi.java', 45],
  cpp_while: ['cpp/01_basics.cpp', 43],
  // contoh user (tembok putaran): '@/' = relatif terhadap vscode-extension/dev/fixtures
  py_range100: ['@/loops.py', 2],
  py_range12: ['@/loops.py', 5],
  py_linear: ['@/search.py', 6],
  py_binary: ['@/search.py', 15],
  py_jump: ['@/search.py', 30],
  py_interp: ['@/search.py', 44],
  py_notfound: ['@/search.py', 55],
  py_in: ['@/builtin.py', 4],
  py_index: ['@/builtin.py', 8],
  py_bisect: ['@/builtin.py', 13],
  js_find: ['@/builtin.js', 3],
  py_bfs: ['@/graph.py', 14],
  py_dfs: ['@/graph.py', 28],
  py_dfs_stack: ['@/graph.py', 37],
  go_bfs: ['@/graph.go', 6],
  py_balok: ['@/balok.py', 4],
};

const run = (...args) => JSON.parse(execFileSync(exe, args, { encoding: 'utf8' }));
const out = {};
for (const [name, [file, line]] of Object.entries(samples)) {
  const f = file.startsWith('@/') ? path.join(root, 'dev', 'fixtures', file.slice(2)) : path.join(testdata, file);
  const scene = run('list', '--json', f);
  const trace = run('trace', '--json', `${f}:${line}`);
  out[name] = { file: path.basename(file), scene, focusBlock: trace.block.id, trace };
}
writeFileSync(path.join(root, 'dev', 'samples.js'), `// Dibuat oleh scripts/dev-samples.mjs, jangan diedit manual.\nwindow.SAMPLES = ${JSON.stringify(out)};\n`);
console.log(`dev/samples.js: ${Object.keys(out).join(', ')}`);
