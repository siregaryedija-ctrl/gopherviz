import * as vscode from 'vscode';
import * as crypto from 'node:crypto';
import type { RaceResult, Trace, UpdateResult } from './server';

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
  | { type: 'trace'; blockId: string; trace: Trace; /** Isi file (untuk potongan kode di rekaman reel). */ code?: { lines: string[] } }
  | {
      type: 'config'; speed: number; sound: boolean; music: boolean; details: boolean; size: number;
      /** Fase 13: tema latar (auto = ikut VS Code), latar pixel, tema yang punya file musik sendiri. */
      theme: string; pixelBg: boolean; musicFiles: Record<string, boolean>;
    }
  /** Isi file musik sendiri untuk satu tema (diminta panel lewat musicFile). */
  | { type: 'musicFile'; theme: string; data?: Uint8Array; mime?: string; error?: string }
  /** Hasil lomba algoritma (Fase 12c), atau pesan error. */
  | { type: 'race'; result?: RaceResult; error?: string };

/** Pesan dari webview ke ekstensi. */
export type FromPanel =
  | { type: 'ready' }
  | { type: 'reveal'; line: number }
  | { type: 'setConfig'; key: 'speed' | 'sound' | 'music' | 'size' | 'theme' | 'pixelBg'; value: number | boolean | string }
  /** Panel meminta isi file musik sendiri untuk tema ini. */
  | { type: 'musicFile'; theme: string }
  | { type: 'restart' }
  | { type: 'input'; text: string }
  | { type: 'reroll' }
  /** Lomba algoritma: fungsi-fungsi (ID blok) dengan list acak sepanjang n; reroll = input acak baru. */
  | { type: 'race'; blocks: string[]; n: number; reroll?: boolean }
  /** Hasil rekaman animasi (MP4 dikirim sebagai WebM lalu diubah ffmpeg). */
  | { type: 'export'; format: 'gif' | 'mp4' | 'webm'; name: string; data: Uint8Array | number[] | Record<string, number> }
  /** Baris (1-based) langkah trace yang sedang tampil; 0 = tidak ada (sorotan di editor dihapus). */
  | { type: 'step'; line: number; /** File langkah ini bila bukan file fokus (relatif). */ file?: string };

const icon = (d: string) => `<svg viewBox="0 0 16 16" aria-hidden="true"><path d="${d}"/></svg>`;

/** Pemutar langkah trace: mundur, putar/jeda, maju, ulang, slider. Salinannya ada di dev/harness.html. */
export const TRACE_MARKUP = `<div id="tracebar" class="tracebar" hidden>
    <button id="tPrev" class="icon-btn" title="Previous step" aria-label="Previous step">${icon('M3 2h2v12H3zM14 2v12L6 8z')}</button>
    <button id="tPlay" class="icon-btn primary" title="Play / pause" aria-label="Play or pause">${icon('M3 2h4v12H3zM9 2h4v12H9z')}</button>
    <button id="tNext" class="icon-btn" title="Next step" aria-label="Next step">${icon('M11 2h2v12h-2zM2 2v12l8-6z')}</button>
    <button id="tRestart" class="icon-btn" title="Restart from the beginning" aria-label="Restart from the beginning">${icon('M8 2a6 6 0 1 1-5.6 8.1l1.9-.7A4 4 0 1 0 8 4v2.5L4.5 3.2 8 0z')}</button>
    <button id="tRec" class="icon-btn" title="Record the animation (GIF / MP4 / WebM)" aria-label="Record the animation">${icon('M8 3a5 5 0 1 0 0 10A5 5 0 0 0 8 3z')}</button>
    <button id="tQuiz" class="icon-btn" aria-pressed="false" title="Guess mode: pause at key moments and guess what happens" aria-label="Guess mode">${icon('M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm0 2a5 5 0 1 1 0 10A5 5 0 0 1 8 3zm0 2a3 3 0 1 0 0 6 3 3 0 0 0 0-6zm0 2a1 1 0 1 1 0 2 1 1 0 0 1 0-2z')}</button>
    <button id="tRace" class="icon-btn" title="Algorithm race: two functions with the same input" aria-label="Algorithm race">${icon('M3 1h1v14H3zM5 2h8l-2 3 2 3H5z')}</button>
    <input type="range" id="tScrub" min="0" max="0" value="0" aria-label="Step position">
    <span id="tLabel" class="tlabel"></span>
  </div>
  <div id="quizCard" class="qcard" hidden></div>
  <div id="errCard" class="errcard" hidden></div>
  <div id="raceCard" class="racecard" hidden>
    <div class="race-head"><b id="raceTitle">RACE</b>
      <span class="race-btns"><button id="racePlay" class="btn">Pause</button><button id="raceAgain" class="btn">Again</button><button id="raceReroll" class="btn">New input</button><button id="raceExit" class="btn">Exit</button></span></div>
    <p id="raceInput" class="race-input"></p>
    <div id="raceResult" class="race-result"></div>
    <canvas id="raceChart" width="560" height="200" aria-label="Chart of operation count against input size"></canvas>
    <p class="race-note">Chart: simulator operation count when each function gets a random list of size n. A steeply rising line = slower for large inputs.</p>
  </div>
  <div id="inputbar" class="inputbar" hidden>
    <label for="inputBox">Input</label>
    <input id="inputBox" type="text" placeholder="input values, separated by commas (e.g. 25, Alice)" spellcheck="false" autocomplete="off">
    <button id="inReroll" class="icon-btn" title="New random numbers for empty inputs" aria-label="New random numbers">${icon('M2 2h12v12H2zM5 4a1 1 0 1 0 0 2 1 1 0 0 0 0-2zm6 0a1 1 0 1 0 0 2 1 1 0 0 0 0-2zM8 7a1 1 0 1 0 0 2 1 1 0 0 0 0-2zm-3 3a1 1 0 1 0 0 2 1 1 0 0 0 0-2zm6 0a1 1 0 1 0 0 2 1 1 0 0 0 0-2z')}</button>
  </div>
  <p id="stepNote" class="step-note" hidden></p>`;

/**
 * Tempat panel (setting gopherviz.location, keputusan user 2026-10-08: bisa dipindah sesuka user):
 * tab editor (bisa diseret ke samping/bawah editor), bagian bawah Explorer, atau panel bawah sebelah
 * Terminal. Dua yang terakhir adalah webview view; view itu juga bisa diseret user ke tempat lain.
 */
export type PanelLocation = 'editor' | 'sidebar' | 'panel';
export const VIEW_IDS = { sidebar: 'gopherviz.sideView', panel: 'gopherviz.bottomView' } as const;

/** Panel webview GopherViz. Pesan terakhir disimpan dan dikirim ulang saat webview siap. */
export class LoopPanel implements vscode.Disposable, vscode.WebviewViewProvider {
  private panel?: vscode.WebviewPanel;
  private view?: vscode.WebviewView;
  /** Webview yang sedang dipakai (tab editor atau view); pesan dari webview lain diabaikan. */
  private host?: vscode.Webview;
  /** Tab editor ditutup oleh kode (pindah tempat), bukan oleh user. */
  private moving = false;
  /** Panel pernah dibuka di sesi ini (dipakai saat pindah tempat: view lama bisa sudah ditutup VS Code). */
  private shown = false;
  private ready = false;
  private lastState?: ToPanel;
  private lastTrace?: ToPanel;
  private lastConfig?: ToPanel;
  private closedByUser = false;
  private readonly extensionUri: vscode.Uri;
  private readonly onMessage: (m: FromPanel) => void;
  private readonly onView: (visible: boolean) => void;
  private readonly location: () => PanelLocation;

  constructor(
    extensionUri: vscode.Uri,
    onMessage: (m: FromPanel) => void,
    onView: (visible: boolean) => void = () => {},
    location: () => PanelLocation = () => 'editor',
  ) {
    this.extensionUri = extensionUri;
    this.onMessage = onMessage;
    this.onView = onView;
    this.location = location;
  }

  /** true bila panel terbuka dan terlihat (animasi berjalan). */
  get isVisible(): boolean {
    return (this.panel?.visible ?? false) || (this.view?.visible ?? false);
  }

  /** Untuk test: tempat panel yang terbuka dan webview-nya sudah siap menerima pesan. */
  get place(): 'editor' | 'view' | undefined {
    if (!this.ready) {
      return undefined;
    }
    return this.panel && this.host === this.panel.webview ? 'editor' : this.view ? 'view' : undefined;
  }

  get isOpen(): boolean {
    return this.panel !== undefined || this.view !== undefined;
  }

  /** true bila user pernah menutup panel di sesi ini (jangan buka otomatis lagi). */
  get wasClosedByUser(): boolean {
    return this.closedByUser;
  }

  show(preserveFocus: boolean, tries = 0): void {
    this.shown = true;
    const loc = this.location();
    if (loc === 'editor') {
      this.showEditor(preserveFocus);
      return;
    }
    if (this.view && this.view.viewType === VIEW_IDS[loc]) {
      this.view.show(preserveFocus);
      return;
    }
    // view belum dibuka: perintah bawaan <id>.focus membuka tempatnya lalu memanggil resolveWebviewView.
    // Sesudah setting berubah, view baru bisa belum terdaftar (klausa when): coba lagi sebentar kemudian.
    void vscode.commands.executeCommand(`${VIEW_IDS[loc]}.focus`).then(() => {
      if (preserveFocus) {
        void vscode.commands.executeCommand('workbench.action.focusActiveEditorGroup');
      }
      setTimeout(() => {
        if (!this.view && this.location() === loc && tries < 8) {
          this.show(preserveFocus, tries + 1);
        }
      }, 250);
    });
  }

  /** Setting gopherviz.location berubah: tutup tempat lama, buka tempat baru bila panel sedang terbuka. */
  relocate(): void {
    const wasOpen = this.isOpen || (this.shown && !this.closedByUser);
    const loc = this.location();
    if (loc !== 'editor' && this.panel) {
      this.moving = true;
      this.panel.dispose();
      this.moving = false;
    }
    if (this.view && (loc === 'editor' || this.view.viewType !== VIEW_IDS[loc])) {
      this.detach(this.view.webview); // view lama disembunyikan VS Code (klausa when)
      this.view = undefined;
    }
    if (wasOpen) {
      // klausa when view baru diperbarui VS Code sesudah event setting; beri jeda sebelum membukanya
      setTimeout(() => this.show(false), loc === 'editor' ? 0 : 150);
    }
  }

  /** Tab editor; bila sudah ada, dimunculkan di kolomnya sekarang (tidak ditarik balik ke samping). */
  private showEditor(preserveFocus: boolean): void {
    if (this.panel) {
      this.panel.reveal(undefined, preserveFocus);
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
    const webview = panel.webview; // sesudah dispose, panel.webview melempar error
    this.attach(webview, media);
    panel.onDidChangeViewState(e => this.onView(e.webviewPanel.visible));
    panel.onDidDispose(() => {
      if (this.panel === panel) {
        this.panel = undefined;
      }
      this.detach(webview);
      if (!this.moving) {
        this.closedByUser = true;
      }
      this.onView(false);
    });
    this.panel = panel;
  }

  /** Dipanggil VS Code saat view di Explorer / panel bawah pertama kali tampil. */
  resolveWebviewView(view: vscode.WebviewView): void {
    const media = vscode.Uri.joinPath(this.extensionUri, 'media');
    view.webview.options = { enableScripts: true, localResourceRoots: [media] };
    this.view = view;
    const webview = view.webview; // sesudah dispose, view.webview melempar error
    this.attach(webview, media);
    view.onDidChangeVisibility(() => this.onView(view.visible));
    view.onDidDispose(() => {
      if (this.view === view) {
        this.view = undefined;
      }
      this.detach(webview);
      this.onView(false);
    });
    this.onView(view.visible);
  }

  /** Jadikan webview ini tujuan pesan; pesan terakhir dikirim ulang begitu webview siap. */
  private attach(webview: vscode.Webview, media: vscode.Uri): void {
    this.host = webview;
    this.ready = false;
    webview.html = this.html(webview, media);
    webview.onDidReceiveMessage((m: FromPanel) => {
      if (webview !== this.host) {
        return;
      }
      if (m.type === 'ready') {
        this.ready = true;
        for (const msg of [this.lastConfig, this.lastState, this.lastTrace]) {
          if (msg) {
            void webview.postMessage(msg);
          }
        }
      }
      this.onMessage(m);
    });
  }

  private detach(webview: vscode.Webview): void {
    if (this.host === webview) {
      this.host = undefined;
      this.ready = false;
    }
  }

  post(msg: ToPanel): void {
    if (msg.type === 'state') {
      this.lastState = msg;
    } else if (msg.type === 'trace') {
      this.lastTrace = msg;
    } else if (msg.type === 'config') {
      this.lastConfig = msg;
    } // race / musicFile: sekali kirim, tidak dikirim ulang saat panel dimuat ulang
    if (this.host && this.ready) {
      void this.host.postMessage(msg);
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
      'media-src blob:',
      `script-src 'nonce-${nonce}'`,
    ].join('; ');
    const scripts = ['pixel.js', 'theme.js', 'sound.js', 'level.js', 'datastage.js', 'branchstage.js', 'funcstage.js', 'lanestage.js', 'graphstage.js', 'classstage.js', 'islandstage.js', 'gridstage.js', 'treestage.js', 'sorts.js', 'mergestage.js', 'sqstage.js', 'evstage.js', 'race.js', 'recorder.js', 'errorfx.js', 'quiz.js', 'memory.js', 'main.js']
      .map(f => `<script nonce="${nonce}" src="${src(f)}"></script>`)
      .join('\n');
    return `<!doctype html>
<html lang="en">
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
    <canvas id="stage" width="768" height="384" aria-label="Pixel stage: the Gopher walks through every part of the code"></canvas>
    <div id="grip" class="grip" title="Drag to resize the picture (double-click: full size)" hidden></div>
    <div id="overlay" class="overlay" hidden>
      <p id="overlayText"></p>
      <button id="btnRestart" class="btn" hidden>Restart server</button>
    </div>
  </div>
  ${TRACE_MARKUP}
  <p id="note" class="note"></p>
  <div id="recDialog" class="rec-dialog" hidden>
    <div class="rec-box" role="dialog" aria-label="Record the animation">
      <h2>Record the animation</h2>
      <div class="rec-row"><span>Frame</span>
        <label><input type="radio" name="recFrame" value="stage" checked> Stage only</label>
        <label><input type="radio" name="recFrame" value="reel"> Vertical (9:16 reel)</label>
      </div>
      <div class="rec-row"><span>Format</span>
        <label><input type="radio" name="recFormat" value="gif" checked> GIF</label>
        <label><input type="radio" name="recFormat" value="mp4"> MP4</label>
        <label><input type="radio" name="recFormat" value="webm"> WebM</label>
      </div>
      <div class="rec-row" id="recTitleRow"><span>Title</span><input id="recTitle" type="text" maxlength="40" spellcheck="false"></div>
      <p id="recNote" class="rec-note"></p>
      <div id="recProgress" class="rec-progress" hidden><div id="recBar"></div></div>
      <div class="rec-actions">
        <button id="recGo" class="btn primary">Record</button>
        <button id="recCancel" class="btn">Cancel</button>
      </div>
    </div>
  </div>

  <div id="raceDialog" class="rec-dialog" hidden>
    <div class="rec-box" role="dialog" aria-label="Algorithm race">
      <h2>Algorithm race</h2>
      <p class="rec-note">Pick 2 functions. Both get the same random list and run side by side.</p>
      <div id="raceList" class="race-list"></div>
      <div class="rec-row"><span>List length</span>
        <label><input type="radio" name="raceN" value="6"> 6</label>
        <label><input type="radio" name="raceN" value="8" checked> 8</label>
        <label><input type="radio" name="raceN" value="10"> 10</label>
      </div>
      <p id="raceNote" class="rec-note"></p>
      <div class="rec-actions">
        <button id="raceGo" class="btn primary">Start race</button>
        <button id="raceCancel" class="btn">Cancel</button>
      </div>
    </div>
  </div>

  <section id="partsSection">
    <h2 id="partsTitle">Loop body</h2>
    <ol id="parts" class="parts"></ol>
  </section>
  <section id="varsSection" hidden>
    <h2>Variables</h2>
    <div id="vars" class="vars"></div>
  </section>
  <section id="memSection" hidden>
    <h2>Memory</h2>
    <div id="mem" class="mem"></div>
    <p class="mem-legend">Brown chest = variable, purple chest = data on the heap, rope = pointer (points to other data).</p>
  </section>
  <section id="outSection" hidden>
    <h2>Output</h2>
    <pre id="out" class="out"></pre>
  </section>

  <section id="navSection">
    <h2 id="navTitle">Blocks in this file</h2>
    <div id="nav" class="nav"></div>
  </section>

  <footer class="controls">
    <div class="speed" role="group" aria-label="Speed">
      <button data-speed="0.5">0.5×</button><button data-speed="1">1×</button><button data-speed="2">2×</button><button data-speed="4">4×</button>
    </div>
    <button id="btnFx" class="btn tog" aria-pressed="true" title="Sound effects">Sound</button>
    <button id="btnMus" class="btn tog" aria-pressed="true" title="Background music">Music</button>
    <label class="theme-pick" title="Background theme (the music changes with it)">🎨
      <select id="themeSel" aria-label="Background theme">
        <option value="auto">Match VS Code</option><option value="day">Day</option><option value="night">Night</option><option value="cave">Cave</option><option value="beach">Beach</option>
      </select></label>
    <label class="pixel-pick" title="Pixel-art background (turn off for a smooth background)"><input type="checkbox" id="pixelChk" checked> Pixel</label>
    <span id="sndHint" class="hint" hidden>Click the panel to turn on sound.</span>
  </footer>
</main>
${scripts}
</body>
</html>`;
  }
}
