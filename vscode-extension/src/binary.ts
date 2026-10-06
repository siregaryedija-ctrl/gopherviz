import * as fs from 'node:fs';
import * as path from 'node:path';

export interface ResolvedBinary {
  path: string;
  source: 'setting' | 'bundled' | 'path';
}

/**
 * Menentukan binary loopviz: setting user → binary bawaan ekstensi
 * (bin/<platform>-<arch>/) → `loopviz` di PATH.
 */
export function resolveBinary(
  extensionPath: string,
  configured: string,
  platform: string = process.platform,
  arch: string = process.arch,
): ResolvedBinary {
  if (configured.trim() !== '') {
    return { path: configured.trim(), source: 'setting' };
  }
  const exe = platform === 'win32' ? 'loopviz.exe' : 'loopviz';
  const bundled = path.join(extensionPath, 'bin', `${platform}-${arch}`, exe);
  if (fs.existsSync(bundled)) {
    return { path: bundled, source: 'bundled' };
  }
  return { path: exe, source: 'path' };
}
