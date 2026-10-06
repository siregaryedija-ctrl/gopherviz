// Test integrasi: menjalankan binary gopherviz sungguhan lewat GophervizClient.
// Jalankan dengan `npm test` (binary dibangun dulu oleh build:bin).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as path from 'node:path';
import { GophervizClient, ServerError, type UpdateResult } from '../src/server';
import { resolveBinary } from '../src/binary';

const extRoot = path.resolve(__dirname, '..', '..');
const bin = resolveBinary(extRoot, process.env.GOPHERVIZ_BIN ?? '');

const code = (body: string) => `package main\n\nfunc main() {\n${body}\n}\n`;

test('binary bawaan ditemukan', () => {
  assert.notEqual(bin.source, 'path', `binary tidak ada di ${extRoot}/bin; jalankan npm run build:bin`);
});

test('hello, update, dan focus', async () => {
  const client = new GophervizClient(bin.path);
  try {
    const hello = await client.start();
    assert.equal(hello.protocol, 1);

    const src = code('\tfor i := 0; i < 3; i++ {\n\t\tfor j := 0; j < i; j++ {\n\t\t}\n\t}');
    const res = await client.request<UpdateResult>('update', { file: 'a.go', text: src, cursor: { line: 5, col: 1 } });
    assert.equal(res.scene.loops.length, 2);
    assert.equal(res.focusLoop, 'L2');
    assert.equal(res.scene.loops[0].header.cond, 'i < 3');
    assert.deepEqual(res.scene.loops[1].body, []);

    const focus = await client.request<{ focusLoop?: string }>('focus', { file: 'a.go', cursor: { line: 4, col: 1 } });
    assert.equal(focus.focusLoop, 'L1');
  } finally {
    client.dispose();
  }
});

test('banyak request sekaligus dijawab ke pemanggil yang benar', async () => {
  const client = new GophervizClient(bin.path);
  try {
    await client.start();
    const jobs = Array.from({ length: 25 }, (_, n) => {
      const body = Array.from({ length: n % 5 }, () => '\tfor {\n\t\tbreak\n\t}').join('\n');
      return client
        .request<UpdateResult>('update', { file: `f${n}.go`, text: code(body), cursor: { line: 1, col: 1 } })
        .then(r => [n % 5, r.scene.loops.length]);
    });
    for (const [want, got] of await Promise.all(jobs)) {
      assert.equal(got, want);
    }
  } finally {
    client.dispose();
  }
});

test('error dari server menjadi ServerError', async () => {
  const client = new GophervizClient(bin.path);
  try {
    await client.start();
    await assert.rejects(client.request('trace', { file: 'belum-dikirim.go', loopId: 'L1' }), (e: unknown) => {
      return e instanceof ServerError && /belum pernah dikirim/.test(e.message);
    });
  } finally {
    client.dispose();
  }
});

test('binary tidak ada: start gagal dengan pesan jelas', async () => {
  const client = new GophervizClient(path.join(extRoot, 'tidak-ada', 'gopherviz.exe'));
  await assert.rejects(client.start(), /gagal menjalankan/);
});

test('server mati di tengah jalan: onExit dipanggil dan request ditolak', async () => {
  let reason = '';
  const client = new GophervizClient(bin.path, r => {
    reason = r;
  });
  await client.start();
  const pending = client.request('update', { file: 'a.go', text: code(''), cursor: { line: 1, col: 1 } });
  (client as unknown as { proc: { kill(): void } }).proc.kill();
  await assert.rejects(pending, ServerError).catch(() => undefined); // bisa saja sudah terjawab sebelum mati
  await new Promise(r => setTimeout(r, 300));
  assert.match(reason, /berhenti/);
  assert.equal(client.running, false);
  await assert.rejects(client.request('hello', {}), /tidak berjalan/);
});
