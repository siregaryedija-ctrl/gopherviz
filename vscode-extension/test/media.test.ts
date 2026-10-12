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
  assert.equal(e.options[e.answer], 'Index out of range');
});

test('parseCond', () => {
  assert.deepEqual({ ...Q.parseCond('if a[j] > a[j+1] → 3 > 1 → true') }, { expr: 'a[j] > a[j+1]', subst: '3 > 1', result: true, kw: 'if' });
  assert.equal(Q.parseCond('elif x > 2 → 3 > 2 → True').result, true);
  assert.equal(Q.parseCond('jumlah += 1 → jumlah = 2'), null);
});

const ws = load('dev/samples.js', 'media/sorts.js', 'media/race.js');

test('sorts: jenis sort dan variabel perannya dikenali', () => {
  const D = (name: string) => ws.GV.sorts.detect(ws.SAMPLES[name].trace);
  const sel = D('sort_sel');
  assert.equal(sel.kind, 'selection');
  assert.equal(sel.min, 'm');
  const ins = D('sort_ins');
  assert.equal(ins.kind, 'insertion');
  assert.equal(ins.key, 'key');
  const q = D('sort_quick');
  assert.equal(q.kind, 'quick');
  assert.deepEqual([q.pivot, q.lo, q.hi], ['pivot', 'lo', 'hi']);
  const m = D('sort_merge');
  assert.equal(m.kind, 'merge');
  assert.equal(m.lists, true);
  assert.equal(D('bubble').kind, 'bubble');
  assert.equal(D('py_linear'), null); // pencarian bukan sorting
  assert.deepEqual([...ws.GV.sorts.parseList('[5, 2, 8]')], ['5', '2', '8']);
  assert.deepEqual([...ws.GV.sorts.parseList('[5 2 8]')], ['5', '2', '8']);
  assert.equal(ws.GV.sorts.parseList('5'), null);
});

test('race: tebakan Big-O dari jumlah operasi', () => {
  const O = ws.GV.race.bigO;
  const sizes = [5, 10, 20, 40];
  assert.equal(O(sizes, sizes.map(n => 3 * n + 4)), 'n');
  assert.equal(O(sizes, sizes.map(n => n * Math.log2(n) * 5)), 'n log n');
  assert.equal(O(sizes, sizes.map(n => n * n)), 'n²');
  assert.equal(O(sizes, [-1, -1, -1, -1]), '?');
  // sampel lomba: selection O(n²), merge O(n log n)
  const r = ws.SAMPLES.race_sel_merge.race;
  assert.deepEqual([...r.runs.map((x: Any) => O(r.sizes, x.work))], ['n²', 'n log n']);
});

const wq = load('dev/samples.js', 'media/sqstage.js');

test('sq: tumpukan dan antrian dikenali dari perubahan isi koleksi', () => {
  const D = (name: string) => wq.GV.sq.detect(wq.SAMPLES[name].trace);
  const want: Record<string, [string, string]> = {
    sq_stack_py: ['stack', 'back'], sq_queue_py: ['queue', 'back'], sq_queue_js: ['queue', 'back'],
    sq_stack_java: ['stack', 'front'], sq_queue_cpp: ['queue', 'back'], sq_stack_go: ['stack', 'back'],
  };
  for (const [name, [kind, end]] of Object.entries(want)) {
    const r = D(name);
    assert.ok(r, `${name}: tidak dikenali`);
    assert.deepEqual([r.kind, r.pushEnd], [kind, end], name);
    assert.equal(r.lists.length, wq.SAMPLES[name].trace.steps.length);
  }
  for (const name of ['bubble', 'sort_sel', 'py_bfs', 'js_for', 'ev_await']) assert.equal(D(name), null, name);
});

test('event loop: setiap langkah sampel async membawa potret evloop', () => {
  const steps = wq.SAMPLES.ev_await.trace.steps;
  assert.ok(steps.every((s: Any) => s.evloop), 'langkah tanpa evloop');
  assert.ok(steps.some((s: Any) => s.evloop.micro.length && s.evloop.macro.length));
  let out = '';
  for (const s of steps) out += s.output || '';
  assert.equal(out, 'mulai A\nsinkron selesai\nlanjut A\npromise\npanjang 1\ntimer\n');
});

test('kuis: pilihan masuk akal (boolean hanya lawannya, tanpa FALSE / FalseFalse)', () => {
  let bools = 0;
  for (const name of Object.keys(S)) {
    for (const q of quizOf(name).qs) {
      const ans = q.options[q.answer];
      if (/^(true|false)$/i.test(ans)) {
        bools++;
        assert.equal(q.options.length, 2, `${name}: ${q.options}`);
        assert.ok(q.options.every((o: string) => /^(true|false)$/i.test(o)), `${name}: ${q.options}`);
      }
      for (const o of q.options) {
        for (const p of q.options) {
          if (o === p) continue;
          assert.notEqual(o.toLowerCase(), p.toLowerCase(), `${name}: ${q.options}`);
          assert.notEqual(o, p + p, `${name}: ${q.options}`);
        }
      }
    }
  }
  assert.ok(bools > 0, 'tidak ada pertanyaan boolean di sampel');
});
