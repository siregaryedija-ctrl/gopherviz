// Klien untuk `gopherviz serve`. Tidak memakai API vscode supaya bisa dites
// langsung dengan Node.
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import * as readline from 'node:readline';

export const PROTOCOL_VERSION = 2;

// ---------- tipe kontrak (cermin dari internal/scene dan internal/protocol) ----------

export type LoopKind =
  | 'for3' | 'cond' | 'do-while' | 'infinite'
  | 'range-int' | 'range-slice' | 'range-array' | 'range-string'
  | 'range-map' | 'range-keys' | 'range-chan' | 'range-func' | 'range-unknown';

export type BlockKind = 'func' | 'if' | 'switch' | 'loop' | 'class';

export interface SceneNode {
  kind: 'stmt' | 'if' | 'switch' | 'loop' | 'break' | 'continue' | 'return' | 'goto' | 'block' | 'bad';
  line?: number;
  endLine?: number;
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

export interface Branch {
  kind: 'if' | 'else-if' | 'else' | 'case' | 'default';
  label: string;
  line: number;
  body: SceneNode[];
}

export interface Block {
  id: string;
  kind: BlockKind;
  line: number;
  endLine: number;
  state: 'complete' | 'incomplete';
  name?: string;
  header: string;
  params?: string[];
  branches?: Branch[];
  body?: SceneNode[];
  parent?: string;
  children?: string[];
}

export interface Scene {
  version: number;
  language: string;
  file?: string;
  blocks: Block[];
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
  focusBlock?: string;
  diagnostics: Diagnostic[];
}

export interface FocusResult {
  focusLoop?: string;
  focusBlock?: string;
}

// ---------- Trace (cermin dari internal/trace) ----------

export interface TraceOp {
  op: 'read' | 'compare' | 'swap' | 'set' | 'append' | 'delete' | 'settle' | 'send' | 'recv';
  /** Indeks elemen; untuk send/recv = slot di pipa channel. */
  a: number;
  b?: number;
  result?: string;
  /** Operator compare dengan elemen a di kiri: "==", "!=", "<", "<=", ">", ">=". */
  cmp?: string;
  value?: string;
  /** Key map (set/read/delete pada map), atau ID channel (send/recv). */
  key?: string;
}

/** Potret memori untuk diagram peti (variabel) dan tali (pointer). */
export interface Memory {
  vars: { name: string; kind: string; type?: string; value?: string; fields?: MemField[]; points?: string }[];
  heap?: { id: string; kind: string; type?: string; value?: string; fields?: MemField[] }[];
}

export interface MemField {
  name: string;
  value?: string;
  points?: string;
}

/** Keadaan satu goroutine (1 = main). */
export interface GState {
  id: number;
  name: string;
  state: 'jalan' | 'siap' | 'menunggu' | 'tidur' | 'selesai';
  why?: string;
}

/** Isi satu channel: buffer, lalu nilai yang dibawa pengirim yang menunggu. */
export interface ChanState {
  id: string;
  name?: string;
  cap: number;
  closed?: boolean;
  items: string[];
  senders?: number;
  receivers?: number;
}

export interface TraceStep {
  n: number;
  loop: string;
  block?: string;
  run?: number;
  depth?: number;
  phase: 'init' | 'cond' | 'body' | 'post' | 'continue' | 'break' | 'enter-child' | 'exit' | 'branch' | 'call' | 'return';
  line?: number;
  iteration?: number;
  branch?: number;
  func?: string;
  args?: string;
  return?: string;
  vars?: Record<string, string>;
  result?: boolean;
  note?: string;
  output?: string;
  reason?: 'cond-false' | 'range-done' | 'break' | 'continue' | 'return' | 'goto' | 'step-limit' | 'panic' | 'done';
  ops?: TraceOp[];
  items?: string[];
  keys?: string[];
  mem?: Memory;
  child?: string;
  g?: number;
  gs?: GState[];
  chans?: ChanState[];
  /** Potret object class fokus (trace class) atau object self (trace method). */
  objs?: ObjState[];
  /** ID object yang menjadi this/self di langkah ini. */
  self?: string;
  /** Go: method value receiver bekerja pada salinan object self. */
  selfCopy?: boolean;
  /** File langkah ini bila bukan file fokus (relatif, mis. "utils.py"). */
  file?: string;
  /** Simpul pohon yang dipegang parameter; null = anak kosong (None/null). */
  node?: { cur?: string; null?: boolean };
  /** Keadaan penelusuran grid/labirin: sel [baris, kolom]. */
  cells?: { cur?: [number, number]; visited?: [number, number][]; frontier?: [number, number][] };
  /** Penjelasan error yang menghentikan program (langkah exit reason panic). */
  err?: ErrInfo;
}

/** Penjelasan error (cermin dari trace.ErrInfo). */
export interface ErrInfo {
  kind: 'index' | 'empty' | 'zero' | 'null' | 'key' | 'depth' | 'type' | 'value' | 'deadlock' | 'other';
  name?: string;
  msg?: string;
  title: string;
  text: string;
  hint?: string;
  expr?: string;
  value?: string;
  len?: number;
  items?: string[];
}

/** Satu object (rumah) pada satu langkah. */
export interface ObjState {
  id: string;
  class: string;
  name?: string;
  fields: { name: string; value: string; points?: string }[];
}

/** Cetak biru class: field dan method yang dideklarasikan. */
export interface ClassInfo {
  name: string;
  super?: string;
  subs?: string[];
  fields?: string[];
  methods?: string[];
}

export interface Trace {
  version: number;
  language: string;
  file?: string;
  mode: 'simulated' | 'structural' | 'instrumented';
  block: { id: string; kind: BlockKind; line: number; endLine: number; name?: string; /** Nama yang dipakai di dalam blok. */ names?: string[] };
  loop: { id: string; kind: LoopKind; line: number; endLine: number; label?: string; vars?: string[]; children?: string[] };
  steps: TraceStep[];
  collection?: { kind: string; items: string[]; keys?: string[] };
  /** Graf yang ditelusuri DFS/BFS (panggung graf). */
  graph?: { var: string; kind?: 'bfs' | 'dfs' | ''; nodes: string[]; edges: [string, string][]; directed?: boolean; frontier?: string; visited?: string };
  /** Cetak biru class (blok class, atau class pemilik method fokus). */
  class?: ClassInfo;
  /** Pohon yang ditelusuri rekursi node.left/node.right (panggung pohon). */
  tree?: { var: string; root: string; nodes: { id: string; label: string; left?: string; right?: string; kids?: string[] }[] };
  /** Grid labirin yang ditelusuri BFS/DFS (panggung labirin). */
  grid?: { var: string; rows: string[][]; start?: [number, number]; goal?: [number, number]; kind?: 'bfs' | 'dfs'; frontier?: string; visited?: string };
  /** File program yang ikut berjalan (file fokus dulu), bila lebih dari satu. */
  files?: string[];
  /** File tempat program dimulai bila bukan file fokus (file yang meng-import-nya). */
  entry?: string;
  truncated: boolean;
  warnings: string[];
  /** Program membaca input user: panel menampilkan kotak Input. */
  readsInput?: boolean;
  /** Lomba: jumlah operasi simulator selama fungsi fokus berjalan. */
  work?: number;
}

export interface TraceResult {
  trace: Trace;
}

/** Hasil method race (Fase 12c): setiap fungsi dengan list acak yang sama. */
export interface RaceResult {
  input: number[];
  sizes: number[];
  runs: { block: string; trace: Trace; /** Operasi untuk sizes[k]; -1 = gagal. */ work: number[] }[];
}

// ---------- klien ----------

interface Pending {
  resolve: (v: unknown) => void;
  reject: (e: Error) => void;
  timer: NodeJS.Timeout;
}

export class ServerError extends Error {}

export class GophervizClient {
  private proc?: ChildProcessWithoutNullStreams;
  private nextId = 1;
  private readonly pending = new Map<number, Pending>();
  private stderrTail = '';
  private readonly command: string;
  private readonly onExit?: (reason: string) => void;

  /**
   * @param command path binary gopherviz
   * @param onExit dipanggil sekali bila proses berhenti tanpa diminta
   */
  constructor(command: string, onExit?: (reason: string) => void) {
    this.command = command;
    this.onExit = onExit;
  }

  /** Menjalankan `gopherviz serve` dan memeriksa versi protokol. */
  async start(): Promise<HelloResult> {
    const proc = spawn(this.command, ['serve'], { stdio: 'pipe', windowsHide: true });
    this.proc = proc;
    proc.stderr.setEncoding('utf8');
    proc.stderr.on('data', (chunk: string) => {
      this.stderrTail = (this.stderrTail + chunk).slice(-4000);
    });
    readline.createInterface({ input: proc.stdout }).on('line', line => this.onLine(line));
    proc.on('error', err => this.stop(`failed to start ${this.command}: ${err.message}`));
    proc.on('exit', code => this.stop(`gopherviz stopped (code ${code ?? '?'})${this.stderrTail ? ': ' + this.stderrTail.trim() : ''}`));

    const hello = await this.request<HelloResult>('hello', { client: 'vscode', protocol: PROTOCOL_VERSION });
    if (hello.protocol !== PROTOCOL_VERSION) {
      this.dispose();
      throw new ServerError(
        `gopherviz protocol version ${hello.protocol} does not match the extension (${PROTOCOL_VERSION}); update the binary or the extension`,
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
      return Promise.reject(new ServerError('the gopherviz server is not running'));
    }
    const id = this.nextId++;
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new ServerError(`gopherviz did not answer "${method}" within ${timeoutMs} ms`));
      }, timeoutMs);
      this.pending.set(id, { resolve: resolve as (v: unknown) => void, reject, timer });
      proc.stdin.write(JSON.stringify({ id, method, params }) + '\n');
    });
  }

  /** Menghentikan proses tanpa memanggil onExit. */
  dispose(): void {
    const proc = this.proc;
    this.proc = undefined;
    this.rejectAll('the gopherviz server was stopped');
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
