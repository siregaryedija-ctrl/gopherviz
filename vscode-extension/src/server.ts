// Klien untuk `loopviz serve`. Tidak memakai API vscode supaya bisa dites
// langsung dengan Node.
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import * as readline from 'node:readline';

export const PROTOCOL_VERSION = 1;

// ---------- tipe kontrak (cermin dari internal/scene dan internal/protocol) ----------

export type LoopKind =
  | 'for3' | 'cond' | 'infinite'
  | 'range-int' | 'range-slice' | 'range-array' | 'range-string'
  | 'range-map' | 'range-chan' | 'range-func' | 'range-unknown';

export interface SceneNode {
  kind: 'stmt' | 'if' | 'loop' | 'break' | 'continue' | 'return' | 'goto' | 'block' | 'bad';
  line?: number;
  text?: string;
  label?: string;
  ref?: string;
  then?: SceneNode[];
  else?: SceneNode[];
  body?: SceneNode[];
}

export interface Loop {
  id: string;
  kind: LoopKind;
  line: number;
  endLine: number;
  label?: string;
  state: 'complete' | 'incomplete';
  header: {
    text: string;
    init?: string; cond?: string; post?: string;
    key?: string; value?: string; define?: boolean; range?: string;
  };
  vars?: string[];
  body: SceneNode[];
  parent?: string;
  children?: string[];
}

export interface Scene {
  version: number;
  file?: string;
  loops: Loop[];
}

export interface Diagnostic {
  line?: number;
  col?: number;
  message: string;
}

export interface Cursor {
  line: number;
  col: number;
}

export interface HelloResult {
  name: string;
  version: string;
  protocol: number;
  scene: number;
  trace: number;
}

export interface UpdateResult {
  scene: Scene;
  focusLoop?: string;
  diagnostics: Diagnostic[];
}

export interface FocusResult {
  focusLoop?: string;
}

// ---------- klien ----------

interface Pending {
  resolve: (v: unknown) => void;
  reject: (e: Error) => void;
  timer: NodeJS.Timeout;
}

export class ServerError extends Error {}

export class LoopvizClient {
  private proc?: ChildProcessWithoutNullStreams;
  private nextId = 1;
  private readonly pending = new Map<number, Pending>();
  private stderrTail = '';
  private readonly command: string;
  private readonly onExit?: (reason: string) => void;

  /**
   * @param command path binary loopviz
   * @param onExit dipanggil sekali bila proses berhenti tanpa diminta
   */
  constructor(command: string, onExit?: (reason: string) => void) {
    this.command = command;
    this.onExit = onExit;
  }

  /** Menjalankan `loopviz serve` dan memeriksa versi protokol. */
  async start(): Promise<HelloResult> {
    const proc = spawn(this.command, ['serve'], { stdio: 'pipe', windowsHide: true });
    this.proc = proc;
    proc.stderr.setEncoding('utf8');
    proc.stderr.on('data', (chunk: string) => {
      this.stderrTail = (this.stderrTail + chunk).slice(-4000);
    });
    readline.createInterface({ input: proc.stdout }).on('line', line => this.onLine(line));
    proc.on('error', err => this.stop(`gagal menjalankan ${this.command}: ${err.message}`));
    proc.on('exit', code => this.stop(`loopviz berhenti (kode ${code ?? '?'})${this.stderrTail ? ': ' + this.stderrTail.trim() : ''}`));

    const hello = await this.request<HelloResult>('hello', { client: 'vscode', protocol: PROTOCOL_VERSION });
    if (hello.protocol !== PROTOCOL_VERSION) {
      this.dispose();
      throw new ServerError(
        `versi protokol loopviz ${hello.protocol} tidak cocok dengan ekstensi (${PROTOCOL_VERSION}); perbarui binary atau ekstensinya`,
      );
    }
    return hello;
  }

  get running(): boolean {
    return this.proc !== undefined;
  }

  request<T>(method: string, params: unknown, timeoutMs = 5000): Promise<T> {
    const proc = this.proc;
    if (!proc) {
      return Promise.reject(new ServerError('server loopviz tidak berjalan'));
    }
    const id = this.nextId++;
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new ServerError(`loopviz tidak menjawab "${method}" dalam ${timeoutMs} ms`));
      }, timeoutMs);
      this.pending.set(id, { resolve: resolve as (v: unknown) => void, reject, timer });
      proc.stdin.write(JSON.stringify({ id, method, params }) + '\n');
    });
  }

  /** Menghentikan proses tanpa memanggil onExit. */
  dispose(): void {
    const proc = this.proc;
    this.proc = undefined;
    this.rejectAll('server loopviz dihentikan');
    proc?.kill();
  }

  private onLine(line: string): void {
    let msg: { id?: number; result?: unknown; error?: { message: string } };
    try {
      msg = JSON.parse(line);
    } catch {
      return; // baris yang bukan JSON diabaikan
    }
    const p = msg.id !== undefined ? this.pending.get(msg.id) : undefined;
    if (!p) {
      return;
    }
    this.pending.delete(msg.id!);
    clearTimeout(p.timer);
    if (msg.error) {
      p.reject(new ServerError(msg.error.message));
    } else {
      p.resolve(msg.result ?? {});
    }
  }

  private stop(reason: string): void {
    if (!this.proc) {
      return; // sudah dihentikan lewat dispose()
    }
    this.proc = undefined;
    this.rejectAll(reason);
    this.onExit?.(reason);
  }

  private rejectAll(reason: string): void {
    for (const p of this.pending.values()) {
      clearTimeout(p.timer);
      p.reject(new ServerError(reason));
    }
    this.pending.clear();
  }
}
