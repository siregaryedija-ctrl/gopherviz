import * as vscode from 'vscode';
import * as path from 'node:path';
import { GophervizClient, type BlockKind, type FocusResult, type LoopKind, type Trace, type TraceResult, type UpdateResult } from './server';
import { resolveBinary } from './binary';
import { LoopPanel, type FromPanel } from './panel';
import { LoopCodeLens } from './codelens';

const UPDATE_DELAY_MS = 75; // jeda setelah ketikan terakhir sebelum scene dihitung ulang
const FOCUS_DELAY_MS = 40;
const TRACE_DELAY_MS = 300; // trace (nilai asli) menyusul setelah user berhenti mengetik
const CRASH_LIMIT = 3; // berhenti mencoba bila server mati sebanyak ini dalam satu menit

const KIND_NAMES: Record<LoopKind, string> = {
  'for3': 'for klasik',
  'cond': 'for kondisi',
  'do-while': 'do-while',
  'infinite': 'for tanpa batas',
  'range-int': 'range angka',
  'range-slice': 'range slice',
  'range-array': 'range array',
  'range-string': 'range string',
  'range-map': 'range map',
  'range-keys': 'range key',
  'range-chan': 'range channel',
  'range-func': 'range iterator',
  'range-unknown': 'range',
};

const BLOCK_NAMES: Record<BlockKind, string> = { func: 'fungsi', if: 'if', switch: 'switch', loop: 'loop' };

/** languageId editor yang didukung GopherViz (Go disimulasikan; bahasa lain mode struktur dulu). */
export const LANGUAGES = ['go', 'javascript', 'javascriptreact', 'typescript', 'typescriptreact', 'python', 'php', 'cpp', 'c', 'java'];
const isSupported = (doc: vscode.TextDocument) => LANGUAGES.includes(doc.languageId) && doc.uri.scheme !== 'output';
const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));
const config = () => vscode.workspace.getConfiguration('gopherviz');

class Controller implements vscode.Disposable {
  private client?: GophervizClient;
  private starting?: Promise<GophervizClient | undefined>;
  private serverError?: string;
  private crashes: number[] = [];

  private readonly results = new Map<string, UpdateResult>();
  private readonly focus = new Map<string, string | undefined>();
  private readonly seq = new Map<string, number>();
  private readonly timers = new Map<string, NodeJS.Timeout>();
  private focusTimer?: NodeJS.Timeout;
  private traceTimer?: NodeJS.Timeout;
  private traceKey = ''; // dokumen|versi|loop dari trace terakhir yang dikirim ke panel
  private lastTrace?: Trace;
  private editor?: vscode.TextEditor; // editor Go terakhir yang aktif

  private readonly panel: LoopPanel;
  private readonly lens: LoopCodeLens;
  private readonly status: vscode.StatusBarItem;
  private readonly output: vscode.OutputChannel;
  private readonly disposables: vscode.Disposable[] = [];
  private readonly ctx: vscode.ExtensionContext;

  constructor(ctx: vscode.ExtensionContext) {
    this.ctx = ctx;
    this.output = vscode.window.createOutputChannel('GopherViz');
    this.panel = new LoopPanel(ctx.extensionUri, m => this.onPanelMessage(m));
    this.lens = new LoopCodeLens(uri => this.results.get(uri)?.scene);
    this.status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
    this.status.command = 'gopherviz.showPanel';

    this.disposables.push(
      this.output,
      this.panel,
      this.lens,
      this.status,
      vscode.languages.registerCodeLensProvider(LANGUAGES.map(language => ({ language })), this.lens),
      vscode.commands.registerCommand('gopherviz.showPanel', () => this.panel.show(true)),
      vscode.commands.registerCommand('gopherviz.visualizeAtCursor', (uri?: vscode.Uri, line?: number) =>
        this.visualize(uri, line),
      ),
      vscode.commands.registerCommand('gopherviz.restartServer', () => this.restart()),
      vscode.window.onDidChangeActiveTextEditor(e => {
        if (e && isSupported(e.document)) {
          this.editor = e;
          this.schedule(e.document, 0);
        }
        this.updateStatus();
      }),
      vscode.workspace.onDidChangeTextDocument(e => {
        if (isSupported(e.document) && e.contentChanges.length > 0 && config().get('realtime', true)) {
          this.schedule(e.document, UPDATE_DELAY_MS);
        }
      }),
      vscode.workspace.onDidSaveTextDocument(doc => {
        if (isSupported(doc) && !config().get('realtime', true)) {
          this.schedule(doc, 0);
        }
      }),
      vscode.window.onDidChangeTextEditorSelection(e => {
        if (isSupported(e.textEditor.document)) {
          this.editor = e.textEditor;
          clearTimeout(this.focusTimer);
          this.focusTimer = setTimeout(() => void this.sendFocus(e.textEditor), FOCUS_DELAY_MS);
        }
      }),
      vscode.workspace.onDidCloseTextDocument(doc => this.forget(doc)),
      vscode.workspace.onDidChangeConfiguration(e => {
        if (!e.affectsConfiguration('gopherviz')) {
          return;
        }
        if (e.affectsConfiguration('gopherviz.binaryPath')) {
          void this.restart();
        }
        this.postConfig();
        this.lens.refresh();
      }),
    );

    this.postConfig();
    const active = vscode.window.activeTextEditor;
    if (active && isSupported(active.document)) {
      this.editor = active;
      this.schedule(active.document, 0);
    }
    this.updateStatus();
  }

  // ---------- server ----------

  private ensureClient(): Promise<GophervizClient | undefined> {
    if (this.client?.running) {
      return Promise.resolve(this.client);
    }
    if (this.serverError) {
      return Promise.resolve(undefined); // tunggu user menekan "Mulai ulang"
    }
    this.starting ??= (async () => {
      const bin = resolveBinary(this.ctx.extensionPath, config().get('binaryPath', ''));
      const client = new GophervizClient(bin.path, reason => this.onServerExit(reason));
      try {
        const hello = await client.start();
        this.client = client;
        this.output.appendLine(`gopherviz ${hello.version} siap (${bin.source}: ${bin.path})`);
        return client;
      } catch (e) {
        client.dispose();
        this.setServerError(`server gopherviz tidak bisa dijalankan: ${errMsg(e)}`);
        return undefined;
      } finally {
        this.starting = undefined;
      }
    })();
    return this.starting;
  }

  private onServerExit(reason: string): void {
    this.client = undefined;
    this.output.appendLine(reason);
    const now = Date.now();
    this.crashes = [...this.crashes.filter(t => now - t < 60_000), now];
    if (this.crashes.length >= CRASH_LIMIT) {
      this.setServerError(`${reason} (berhenti ${CRASH_LIMIT} kali dalam satu menit)`);
    } else if (this.editor) {
      this.schedule(this.editor.document, 200);
    }
  }

  private setServerError(message: string): void {
    this.serverError = message;
    this.output.appendLine(message);
    this.pushState();
    void vscode.window.showErrorMessage(`GopherViz: ${message}`, 'Mulai ulang', 'Lihat log').then(choice => {
      if (choice === 'Mulai ulang') {
        void this.restart();
      } else if (choice === 'Lihat log') {
        this.output.show();
      }
    });
  }

  private async restart(): Promise<void> {
    this.client?.dispose();
    this.client = undefined;
    this.serverError = undefined;
    this.crashes = [];
    this.traceKey = '';
    this.results.clear();
    this.pushState();
    if (this.editor) {
      this.schedule(this.editor.document, 0);
    } else {
      await this.ensureClient();
    }
  }

  // ---------- sinkronisasi dokumen ----------

  private schedule(doc: vscode.TextDocument, delay: number): void {
    const uri = doc.uri.toString();
    clearTimeout(this.timers.get(uri));
    this.timers.set(
      uri,
      setTimeout(() => {
        this.timers.delete(uri);
        void this.sendUpdate(doc);
      }, delay),
    );
  }

  private cursorFor(doc: vscode.TextDocument) {
    const ed = this.editor?.document === doc ? this.editor : vscode.window.visibleTextEditors.find(e => e.document === doc);
    const pos = ed?.selection.active;
    return pos ? { line: pos.line + 1, col: pos.character + 1 } : { line: 0, col: 0 };
  }

  private async sendUpdate(doc: vscode.TextDocument): Promise<void> {
    const uri = doc.uri.toString();
    const n = (this.seq.get(uri) ?? 0) + 1;
    this.seq.set(uri, n);
    const client = await this.ensureClient();
    if (!client || doc.isClosed) {
      return;
    }
    try {
      const res = await client.request<UpdateResult>('update', {
        file: doc.fileName,
        text: doc.getText(),
        cursor: this.cursorFor(doc),
        language: doc.languageId,
      });
      if (this.seq.get(uri) !== n) {
        return; // sudah ada update yang lebih baru
      }
      this.results.set(uri, res);
      this.lens.sceneChanged(uri, res.scene);
      this.setFocus(uri, res.focusBlock);
    } catch (e) {
      this.output.appendLine(`update gagal: ${errMsg(e)}`);
    }
  }

  private async sendFocus(editor: vscode.TextEditor): Promise<void> {
    const uri = editor.document.uri.toString();
    if (this.timers.has(uri)) {
      return; // update yang tertunda akan menghitung fokus sekaligus
    }
    if (!this.client?.running || !this.results.has(uri)) {
      this.schedule(editor.document, 0);
      return;
    }
    const pos = editor.selection.active;
    try {
      const res = await this.client.request<FocusResult>('focus', {
        file: editor.document.fileName,
        cursor: { line: pos.line + 1, col: pos.character + 1 },
      });
      this.setFocus(uri, res.focusBlock);
    } catch (e) {
      this.output.appendLine(`focus gagal: ${errMsg(e)}`);
    }
  }

  private setFocus(uri: string, blockId: string | undefined): void {
    this.focus.set(uri, blockId);
    // Buka otomatis hanya saat kursor masuk loop/if/switch; berada di dalam fungsi saja terlalu sering.
    const kind = this.results.get(uri)?.scene.blocks.find(b => b.id === blockId)?.kind;
    if (kind && kind !== 'func' && config().get('autoOpen', true) && !this.panel.isOpen && !this.panel.wasClosedByUser) {
      this.panel.show(true);
    }
    this.pushState();
    clearTimeout(this.traceTimer);
    this.traceTimer = setTimeout(() => void this.sendTrace(uri), TRACE_DELAY_MS);
  }

  private async sendTrace(uri: string): Promise<void> {
    const doc = this.editor?.document;
    const blockId = this.focus.get(uri);
    if (!doc || doc.uri.toString() !== uri || !blockId || !this.client?.running) {
      return;
    }
    const key = `${uri}|${this.seq.get(uri)}|${blockId}`;
    if (key === this.traceKey) {
      return; // kode dan blok fokus tidak berubah sejak trace terakhir
    }
    try {
      const res = await this.client.request<TraceResult>('trace', { file: doc.fileName, blockId });
      if (this.focus.get(uri) !== blockId || key !== `${uri}|${this.seq.get(uri)}|${blockId}`) {
        return; // user sudah mengetik atau pindah blok lagi
      }
      this.traceKey = key;
      this.lastTrace = res.trace;
      this.panel.post({ type: 'trace', blockId, trace: res.trace });
    } catch (e) {
      this.output.appendLine(`trace gagal: ${errMsg(e)}`);
    }
  }

  private forget(doc: vscode.TextDocument): void {
    const uri = doc.uri.toString();
    if (!this.results.delete(uri)) {
      return;
    }
    this.focus.delete(uri);
    this.lens.forget(uri);
    clearTimeout(this.timers.get(uri));
    this.timers.delete(uri);
    void this.client?.request('close', { file: doc.fileName }).catch(() => undefined);
  }

  // ---------- panel & status ----------

  private pushState(): void {
    const doc = this.editor?.document;
    const uri = doc?.uri.toString();
    this.panel.post({
      type: 'state',
      file: doc ? path.basename(doc.fileName) : undefined,
      result: uri ? this.results.get(uri) : undefined,
      focusBlock: uri ? this.focus.get(uri) : undefined,
      server: { ok: !this.serverError, message: this.serverError },
    });
    this.updateStatus();
  }

  private postConfig(): void {
    const c = config();
    this.panel.post({
      type: 'config',
      speed: c.get('speed', 1),
      sound: c.get('sound', true),
      music: c.get('music', true),
    });
  }

  private updateStatus(): void {
    const ed = vscode.window.activeTextEditor;
    if (!ed || !isSupported(ed.document)) {
      this.status.hide();
      return;
    }
    const uri = ed.document.uri.toString();
    const scene = this.results.get(uri)?.scene;
    const block = scene?.blocks.find(b => b.id === this.focus.get(uri));
    if (this.serverError) {
      this.status.text = '$(warning) GopherViz';
      this.status.tooltip = this.serverError;
    } else if (block) {
      const loop = block.kind === 'loop' ? scene?.loops.find(l => l.id === block.id) : undefined;
      const name = loop ? KIND_NAMES[loop.kind] : `${BLOCK_NAMES[block.kind]}${block.name && block.kind === 'func' ? ' ' + block.name : ''}`;
      this.status.text = `$(sync) ${block.id} · ${name}`;
      this.status.tooltip = 'Buka panel GopherViz';
    } else {
      this.status.text = '$(sync) GopherViz';
      this.status.tooltip = 'Taruh kursor di dalam fungsi, if, switch, atau loop, lalu klik untuk membuka panel';
    }
    this.status.show();
  }

  private onPanelMessage(m: FromPanel): void {
    switch (m.type) {
      case 'ready':
        this.postConfig();
        this.pushState();
        break;
      case 'reveal':
        void this.reveal(m.line);
        break;
      case 'setConfig':
        void config().update(m.key, m.value, vscode.ConfigurationTarget.Global);
        break;
      case 'restart':
        void this.restart();
        break;
    }
  }

  private async reveal(line: number): Promise<void> {
    const ed = this.editor;
    if (!ed) {
      return;
    }
    const shown = await vscode.window.showTextDocument(ed.document, { viewColumn: ed.viewColumn, preserveFocus: false });
    const idx = Math.max(0, Math.min(line - 1, shown.document.lineCount - 1));
    const col = shown.document.lineAt(idx).firstNonWhitespaceCharacterIndex;
    const pos = new vscode.Position(idx, col);
    shown.selection = new vscode.Selection(pos, pos);
    shown.revealRange(new vscode.Range(pos, pos), vscode.TextEditorRevealType.InCenterIfOutsideViewport);
  }

  private async visualize(uri?: vscode.Uri, line?: number): Promise<void> {
    if (uri && line) {
      const doc = await vscode.workspace.openTextDocument(uri);
      const ed = await vscode.window.showTextDocument(doc, { preserveFocus: false });
      this.editor = ed;
      const pos = new vscode.Position(line - 1, doc.lineAt(line - 1).firstNonWhitespaceCharacterIndex);
      ed.selection = new vscode.Selection(pos, pos);
    }
    this.panel.show(true);
  }

  /** Dipakai test integrasi (test/vscode) untuk memeriksa keadaan ekstensi. */
  get testApi() {
    return {
      result: (uri: string) => this.results.get(uri),
      focus: (uri: string) => this.focus.get(uri),
      traceKey: () => this.traceKey,
      lastTrace: () => this.lastTrace,
      panelOpen: () => this.panel.isOpen,
    };
  }

  dispose(): void {
    for (const t of this.timers.values()) {
      clearTimeout(t);
    }
    clearTimeout(this.focusTimer);
    clearTimeout(this.traceTimer);
    this.client?.dispose();
    for (const d of this.disposables) {
      d.dispose();
    }
  }
}

export function activate(ctx: vscode.ExtensionContext) {
  const controller = new Controller(ctx);
  ctx.subscriptions.push(controller);
  return controller.testApi;
}

export type TestApi = ReturnType<typeof activate>;

export function deactivate(): void {}
