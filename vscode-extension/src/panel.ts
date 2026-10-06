import * as vscode from 'vscode';
import * as crypto from 'node:crypto';
import type { UpdateResult } from './server';

/** Pesan dari ekstensi ke webview. */
export type ToPanel =
  | {
      type: 'state';
      file?: string;
      result?: UpdateResult;
      focusLoop?: string;
      server: { ok: boolean; message?: string };
    }
  | { type: 'config'; speed: number; sound: boolean; music: boolean };

/** Pesan dari webview ke ekstensi. */
export type FromPanel =
  | { type: 'ready' }
  | { type: 'reveal'; line: number }
  | { type: 'setConfig'; key: 'speed' | 'sound' | 'music'; value: number | boolean }
  | { type: 'restart' };

/** Panel webview GoLoopViz. Pesan terakhir disimpan dan dikirim ulang saat webview siap. */
export class LoopPanel implements vscode.Disposable {
  private panel?: vscode.WebviewPanel;
  private ready = false;
  private lastState?: ToPanel;
  private lastConfig?: ToPanel;
  private closedByUser = false;
  private readonly extensionUri: vscode.Uri;
  private readonly onMessage: (m: FromPanel) => void;

  constructor(extensionUri: vscode.Uri, onMessage: (m: FromPanel) => void) {
    this.extensionUri = extensionUri;
    this.onMessage = onMessage;
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
      'goloopviz.panel',
      'GoLoopViz',
      { viewColumn: vscode.ViewColumn.Beside, preserveFocus },
      { enableScripts: true, retainContextWhenHidden: true, localResourceRoots: [media] },
    );
    panel.iconPath = vscode.Uri.joinPath(media, 'icon.svg');
    panel.webview.html = this.html(panel.webview, media);
    panel.webview.onDidReceiveMessage((m: FromPanel) => {
      if (m.type === 'ready') {
        this.ready = true;
        for (const msg of [this.lastConfig, this.lastState]) {
          if (msg) {
            void panel.webview.postMessage(msg);
          }
        }
      }
      this.onMessage(m);
    });
    panel.onDidDispose(() => {
      this.panel = undefined;
      this.ready = false;
      this.closedByUser = true;
    });
    this.panel = panel;
  }

  post(msg: ToPanel): void {
    if (msg.type === 'state') {
      this.lastState = msg;
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
    const scripts = ['pixel.js', 'sound.js', 'level.js', 'main.js']
      .map(f => `<script nonce="${nonce}" src="${src(f)}"></script>`)
      .join('\n');
    return `<!doctype html>
<html lang="id">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="stylesheet" href="${src('panel.css')}">
<title>GoLoopViz</title>
</head>
<body>
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
  <p id="note" class="note"></p>

  <section id="partsSection">
    <h2>Isi loop</h2>
    <ol id="parts" class="parts"></ol>
  </section>

  <section id="navSection">
    <h2 id="navTitle">Loop di file ini</h2>
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
