import * as vscode from 'vscode';
import * as path from 'node:path';
import * as os from 'node:os';
import { promises as fs } from 'node:fs';
import { execFile } from 'node:child_process';

/** Hasil rekaman dari panel (Fase 12a). MP4 datang sebagai WebM lalu diubah ffmpeg. */
export interface ExportMessage {
  format: 'gif' | 'mp4' | 'webm';
  name: string;
  data: Uint8Array | number[] | Record<string, number>;
}

/** Data dari webview bisa tiba sebagai Uint8Array, array angka, atau objek berindeks. */
function toBytes(d: ExportMessage['data']): Uint8Array {
  if (d instanceof Uint8Array) {
    return d;
  }
  if (Array.isArray(d)) {
    return Uint8Array.from(d);
  }
  const n = Object.keys(d).length;
  const out = new Uint8Array(n);
  for (let i = 0; i < n; i++) {
    out[i] = d[i] ?? 0;
  }
  return out;
}

const FILTERS: Record<ExportMessage['format'], Record<string, string[]>> = {
  gif: { 'Animasi GIF': ['gif'] },
  mp4: { 'Video MP4': ['mp4'] },
  webm: { 'Video WebM': ['webm'] },
};

function ffmpegPath(): string {
  return vscode.workspace.getConfiguration('gopherviz').get<string>('ffmpegPath', '').trim() || 'ffmpeg';
}

function runFfmpeg(input: string, output: string): Promise<void> {
  // H.264 + yuv420p supaya bisa diputar di ponsel/Instagram/WhatsApp; ukuran dibulatkan genap
  const args = ['-y', '-i', input, '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-movflags', '+faststart',
    '-vf', 'scale=trunc(iw/2)*2:trunc(ih/2)*2', output];
  return new Promise((resolve, reject) => {
    execFile(ffmpegPath(), args, { windowsHide: true, timeout: 180_000 }, (err, _out, stderr) => {
      if (err) {
        reject(new Error((err as NodeJS.ErrnoException).code === 'ENOENT' ? 'ffmpeg tidak ditemukan' : stderr.split('\n').slice(-3).join(' ').trim() || err.message));
        return;
      }
      resolve();
    });
  });
}

/** Menyimpan rekaman: dialog Save di folder file yang sedang divisualkan. */
export async function saveExport(m: ExportMessage, baseDir: string | undefined): Promise<void> {
  const bytes = toBytes(m.data);
  const dir = baseDir ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? os.homedir();
  const ask = (format: ExportMessage['format']) =>
    vscode.window.showSaveDialog({
      defaultUri: vscode.Uri.file(path.join(dir, `${m.name || 'gopherviz'}.${format}`)),
      filters: FILTERS[format],
      saveLabel: 'Simpan rekaman',
    });
  if (m.format !== 'mp4') {
    const target = await ask(m.format);
    if (target) {
      await vscode.workspace.fs.writeFile(target, bytes);
      void showSaved(target);
    }
    return;
  }
  const target = await ask('mp4');
  if (!target) {
    return;
  }
  const tmp = path.join(os.tmpdir(), `gopherviz-${Date.now()}.webm`);
  await fs.writeFile(tmp, bytes);
  try {
    await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: 'GopherViz: mengubah rekaman ke MP4 (ffmpeg)…' },
      () => runFfmpeg(tmp, target.fsPath),
    );
    void showSaved(target);
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    const pick = await vscode.window.showErrorMessage(
      `GopherViz: gagal membuat MP4 (${msg}). Atur lokasi ffmpeg.exe di setting "GopherViz: Ffmpeg Path", atau simpan sebagai WebM.`,
      'Atur ffmpeg',
      'Simpan WebM',
    );
    if (pick === 'Atur ffmpeg') {
      void vscode.commands.executeCommand('workbench.action.openSettings', 'gopherviz.ffmpegPath');
    } else if (pick === 'Simpan WebM') {
      const webm = await ask('webm');
      if (webm) {
        await vscode.workspace.fs.writeFile(webm, bytes);
        void showSaved(webm);
      }
    }
  } finally {
    void fs.unlink(tmp).catch(() => undefined);
  }
}

async function showSaved(target: vscode.Uri): Promise<void> {
  const pick = await vscode.window.showInformationMessage(`GopherViz: rekaman disimpan ke ${path.basename(target.fsPath)}`, 'Buka folder');
  if (pick === 'Buka folder') {
    void vscode.commands.executeCommand('revealFileInOS', target);
  }
}
