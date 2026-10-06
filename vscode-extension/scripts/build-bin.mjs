// Build binary loopviz dan taruh di bin/<platform>-<arch>/ supaya ikut dibundel di .vsix.
//   node scripts/build-bin.mjs        hanya untuk platform komputer ini
//   node scripts/build-bin.mjs --all  semua platform yang didukung
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const extRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = path.resolve(extRoot, '..');

// [GOOS, GOARCH, nama folder sesuai process.platform-process.arch milik Node]
const targets = [
  ['windows', 'amd64', 'win32-x64'],
  ['windows', 'arm64', 'win32-arm64'],
  ['linux', 'amd64', 'linux-x64'],
  ['linux', 'arm64', 'linux-arm64'],
  ['darwin', 'amd64', 'darwin-x64'],
  ['darwin', 'arm64', 'darwin-arm64'],
];

const current = `${process.platform}-${process.arch}`;
const selected = process.argv.includes('--all') ? targets : targets.filter(t => t[2] === current);
if (selected.length === 0) {
  console.error(`platform ${current} belum didukung`);
  process.exit(1);
}

for (const [goos, goarch, dir] of selected) {
  const out = path.join(extRoot, 'bin', dir, goos === 'windows' ? 'loopviz.exe' : 'loopviz');
  console.log(`go build ${goos}/${goarch} -> ${path.relative(extRoot, out)}`);
  execFileSync('go', ['build', '-trimpath', '-ldflags=-s -w', '-o', out, './cmd/loopviz'], {
    cwd: repoRoot,
    stdio: 'inherit',
    env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: '0' },
  });
}
