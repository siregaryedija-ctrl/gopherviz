import * as vscode from 'vscode';
import * as crypto from 'node:crypto';
import type { Trace, UpdateResult } from './server';

/** Pesan dari ekstensi ke webview. */
export type ToPanel =
  | {
      type: 'state';
      file?: string;
      result?: UpdateResult;
      focusBlock?: string;
      server: { ok: boolean; message?: string };
      /** Isi kotak Input untuk file ini (nilai dipisah koma). */
      input?: string;
    }
  | { type: 'trace'; blockId: string; trace: Trace }
  | { type: 'config'; speed: number; sound: boolean; music: boolean; details: boolean };

/** Pesan dari webview ke ekstensi. */
export type FromPanel =
  | { type: 'ready' }
  | { type: 'reveal'; line: number }
  | { type: 'setConfig'; key: 'speed' | 'sound' | 'music'; value: number | boolean }
  | { type: 'restart' }
  | { type: 'input'; text: string }
  | { type: 'reroll' }
  /** Baris (1-based) langkah trace yang sedang tampil; 0 = tidak ada (sorotan di editor dihapus). */
  | { type: 'step'; line: number };

const icon = (d: string) => `<svg viewBox="0 0 16 16" aria-hidden="true"><path d="${d}"/></svg>`;

/** Pemutar langkah trace: mundur, putar/jeda, maju, ulang, slider. Salinannya ada di dev/harness.html. */
export const TRACE_MARKUP = `<div id="tracebar" class="tracebar" hidden>
    <button id="tPrev" class="icon-btn" title="Langkah sebelumnya" aria-label="Langkah sebelumnya">${icon('M3 2h2v12H3zM14 2v12L6 8z')}</button>
    <button id="tPlay" class="icon-btn primary" title="Putar / jeda" aria-label="Putar atau jeda">${icon('M3 2h4v12H3zM9 2h4v12H9z')}</button>
    <button id="tNext" class="icon-btn" title="Langkah berikutnya" aria-label="Langkah berikutnya">${icon('M11 2h2v12h-2zM2 2v12l8-6z')}</button>
    <button id="tRestart" class="icon-btn" title="Ulang dari awal" aria-label="Ulang dari awal">${icon('M8 2a6 6 0 1 1-5.6 8.1l1.9-.7A4 4 0 1 0 8 4v2.5L4.5 3.2 8 0z')}</button>
    <input type="range" id="tScrub" min="0" max="0" value="0" aria-label="Posisi langkah">
    <span id="tLabel" class="tlabel"></span>
  </div>
  <div id="inputbar" class="inputbar" hidden>
    <label for="inputBox">Input</label>
    <input id="inputBox" type="text" placeholder="isi nilai input, pisahkan dengan koma (contoh: 25, Budi)" spellcheck="false" autocomplete="off">
    <button id="inReroll" class="icon-btn" title="Angka acak baru untuk input yang belum diisi" aria-label="Angka acak baru">${icon('M2 2h12v12H2zM5 4a1 1 0 1 0 0 2 1 1 0 0 0 0-2zm6 0a1 1 0 1 0 0 2 1 1 0 0 0 0-2zM8 7a1 1 0 1 0 0 2 1 1 0 0 0 0-2zm-3 3a1 1 0 1 0 0 2 1 1 0 0 0 0-2zm6 0a1 1 0 1 0 0 2 1 1 0 0 0 0-2z')}</button>
  </div>
  <p id="stepNote" class="step-note" hidden></p>`;

/** Panel webview GopherViz. Pesan terakhir disimpan dan dikirim ulang saat webview siap. */
export class LoopPanel implements vscode.Disposable {
  private panel?: vscode.WebviewPanel;
  private ready = false;
  private lastState?: ToPanel;
  private lastTrace?: ToPanel;
  private lastConfig?: ToPanel;
  private closedByUser = false;
  private readonly extensionUri: vscode.Uri;
  private readonly onMessage: (m: FromPanel) => void;
  private readonly onView: (visible: boolean) => void;

  constructor(extensionUri: vscode.Uri, onMessage: (m: FromPanel) => void, onView: (visible: boolean) => void = () => {}) {
    this.extensionUri = extensionUri;
    this.onMessage = onMessage;
    this.onView = onView;
  }

  /** true bila panel terbuka dan terlihat (animasi berjalan). */
  get isVisible(): boolean {
    return this.panel?.visible ?? false;
  }

  get isOpen(): boolean {
    return this.panel !== undefined;
  }

  /** true bila user pernah menutup panel di sesi ini (jangan buka otomatis lagi). */
  get wasClosedByUser(): boolean {
    return this.closedByUser;
  }

  show(preserveFocus: boolean): void {
    if (this.panel) {
      this.panel.reveal(vscode.ViewColumn.Beside, preserveFocus);
      return;
    }
    const media = vscode.Uri.joinPath(this.extensionUri, 'media');
    const panel = vscode.window.createWebviewPanel(
      'gopherviz.panel',
      'GopherViz',
      { viewColumn: vscode.ViewColumn.Beside, preserveFocus },
      { enableScripts: true, retainContextWhenHidden: true, localResourceRoots: [media] },
    );
    panel.iconPath = vscode.Uri.joinPath(media, 'icon.svg');
    panel.webview.html = this.html(panel.webview, media);
    panel.webview.onDidReceiveMessage((m: FromPanel) => {
      if (m.type === 'ready') {
        this.ready = true;
        for (const msg of [this.lastConfig, this.lastState, this.lastTrace]) {
          if (msg) {
            void panel.webview.postMessage(msg);
          }
        }
      }
      this.onMessage(m);
    });
    panel.onDidChangeViewState(e => this.onView(e.webviewPanel.visible));
    panel.onDidDispose(() => {
      this.panel = undefined;
      this.ready = false;
      this.closedByUser = true;
      this.onView(false);
    });
    this.panel = panel;
  }

  post(msg: ToPanel): void {
    if (msg.type === 'state') {
      this.lastState = msg;
    } else if (msg.type === 'trace') {
      this.lastTrace = msg;
    } else {
      this.lastConfig = msg;
    }
    if (this.panel && this.ready) {
      void this.panel.webview.postMessage(msg);
    }
  }

  dispose(): void {
    this.panel?.dispose();
  }

  private html(webview: vscode.Webview, media: vscode.Uri): string {
    const nonce = crypto.randomBytes(16).toString('base64');
    const src = (f: string) => webview.asWebviewUri(vscode.Uri.joinPath(media, f)).toString();
    const csp = [
      "default-src 'none'",
      `img-src ${webview.cspSource} data:`,
      `style-src ${webview.cspSource}`,
      `script-src 'nonce-${nonce}'`,
    ].join('; ');
    const scripts = ['pixel.js', 'sound.js', 'level.js', 'datastage.js', 'branchstage.js', 'funcstage.js', 'lanestage.js', 'memory.js', 'main.js']
      .map(f => `<script nonce="${nonce}" src="${src(f)}"></script>`)
      .join('\n');
    return `<!doctype html>
<html lang="id">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="stylesheet" href="${src('panel.css')}">
<title>GopherViz</title>
</head>
<body class="pic">
<main class="app">
  <header class="top">
    <div class="title">
      <span id="loopId" class="loop-id"></span>
      <span id="kind" class="kind"></span>
      <span id="stateChip" class="chip" hidden></span>
    </div>
    <code id="head" class="head"></code>
    <p id="explain" class="explain"></p>
  </header>

  <div class="stage">
    <canvas id="stage" width="768" height="384" aria-label="Panggung pixel: Gopher berjalan melewati setiap bagian loop"></canvas>
    <div id="overlay" class="overlay" hidden>
      <p id="overlayText"></p>
      <button id="btnRestart" class="btn" hidden>Mulai ulang server</button>
    </div>
  </div>
  ${TRACE_MARKUP}
  <p id="note" class="note"></p>

  <section id="partsSection">
    <h2 id="partsTitle">Isi loop</h2>
    <ol id="parts" class="parts"></ol>
  </section>
  <section id="varsSection" hidden>
    <h2>Variabel</h2>
    <div id="vars" class="vars"></div>
  </section>
  <section id="memSection" hidden>
    <h2>Memori</h2>
    <div id="mem" class="mem"></div>
    <p class="mem-legend">Peti cokelat = variabel, peti ungu = data di heap, tali = pointer (menunjuk ke data lain).</p>
  </section>
  <section id="outSection" hidden>
    <h2>Output</h2>
    <pre id="out" class="out"></pre>
  </section>

  <section id="navSection">
    <h2 id="navTitle">Blok di file ini</h2>
    <div id="nav" class="nav"></div>
  </section>

  <footer class="controls">
    <div class="speed" role="group" aria-label="Kecepatan">
      <button data-speed="0.5">0.5×</button><button data-speed="1">1×</button><button data-speed="2">2×</button><button data-speed="4">4×</button>
    </div>
    <button id="btnFx" class="btn tog" aria-pressed="true" title="Efek suara">Efek</button>
    <button id="btnMus" class="btn tog" aria-pressed="true" title="Musik latar">Musik</button>
    <span id="sndHint" class="hint" hidden>Klik panel untuk menyalakan suara.</span>
  </footer>
</main>
${scripts}
</body>
</html>`;
  }
}
