// Test logika webview yang tidak butuh canvas (media/quiz.js, nanti juga logika lomba).
// File media dimuat apa adanya ke sandbox vm dengan objek window palsu, memakai contoh trace
// dari dev/samples.js (dibuat oleh scripts/dev-samples.mjs).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as vm from 'node:vm';

const extRoot = path.resolve(__dirname, '..', '..');

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Any = any;

function load(...files: string[]): Any {
  const window: Any = {};
  const ctx = vm.createContext({ window, globalThis: {} });
  for (const f of files) {
    vm.runInContext(fs.readFileSync(path.join(extRoot, f), 'utf8'), ctx, { filename: f });
  }
  return window;
}

const w = load('dev/samples.js', 'media/quiz.js');
const S = w.SAMPLES;
const Q = w.GV.quiz;

/** Tampilan sederhana: satu langkah trace = satu entri view (cukup untuk memilih pertanyaan). */
function quizOf(name: string) {
  const s = S[name];
  const block = s.scene.scene.blocks.find((b: Any) => b.id === s.focusBlock);
  const view = s.trace.steps.map((step: Any) => ({ step }));
  return { qs: Q.build(view, { lang: s.scene.scene.language, block, code: s.code }), view };
}

test('kuis: maks 5 pertanyaan, urut, jawaban sah', () => {
  for (const name of Object.keys(S)) {
    const { qs, view } = quizOf(name);
    assert.ok(qs.length <= Q.MAX, `${name}: ${qs.length} pertanyaan`);
    let last = 0;
    for (const q of qs) {
      assert.ok(q.at > 0 && q.at < view.length, `${name}: at ${q.at}`);
      assert.ok(q.at > last || last === 0, `${name}: urutan`);
      last = q.at;
      assert.ok(q.options.length >= 2 && q.options.length <= 4, `${name}: ${q.options.length} pilihan`);
      assert.equal(new Set(q.options).size, q.options.length, `${name}: pilihan kembar ${q.options}`);
      assert.ok(q.answer >= 0 && q.answer < q.options.length, `${name}: jawaban ${q.answer}`);
    }
  }
});

test('kuis: jenis pertanyaan mengikuti isi trace', () => {
  const types = (name: string) => new Set(quizOf(name).qs.map((q: Any) => q.type));
  assert.ok(types('bubble').has('cond'));
  assert.ok(types('switch').has('branch'));
  assert.ok(types('fib').has('return'));
  const err = types('err_idx');
  assert.ok(err.has('error') && err.has('assign'), [...err].join(','));
});

test('kuis: jawaban benar sesuai trace', () => {
  // switch: cabang yang dipilih = langkah branch berikutnya
  const { qs, view } = quizOf('switch');
  const brs = S.switch.scene.scene.blocks.find((b: Any) => b.id === S.switch.focusBlock).branches;
  for (const q of qs.filter((x: Any) => x.type === 'branch')) {
    const br = view.slice(q.at).find((v: Any) => v.step.phase === 'branch').step.branch;
    assert.ok(q.options[q.answer].endsWith(brs[br].label), `${q.options[q.answer]} vs ${brs[br].label}`);
  }
  // error: judul dari Step.err
  const e = quizOf('err_idx').qs.find((x: Any) => x.type === 'error');
  assert.equal(e.options[e.answer], 'Indeks di luar batas');
});

test('parseCond', () => {
  assert.deepEqual({ ...Q.parseCond('if a[j] > a[j+1] → 3 > 1 → true') }, { expr: 'a[j] > a[j+1]', subst: '3 > 1', result: true, kw: 'if' });
  assert.equal(Q.parseCond('elif x > 2 → 3 > 2 → True').result, true);
  assert.equal(Q.parseCond('jumlah += 1 → jumlah = 2'), null);
});
