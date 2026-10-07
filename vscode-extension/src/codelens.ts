import * as vscode from 'vscode';
import type { Scene } from './server';

/** Tombol "▶ Visualisasikan" di atas setiap blok (fungsi, loop, if, switch). */
export class LoopCodeLens implements vscode.CodeLensProvider {
  private readonly emitter = new vscode.EventEmitter<void>();
  readonly onDidChangeCodeLenses = this.emitter.event;
  private readonly signatures = new Map<string, string>();
  private readonly getScene: (uri: string) => Scene | undefined;

  constructor(getScene: (uri: string) => Scene | undefined) {
    this.getScene = getScene;
  }

  /** Refresh hanya bila letak loop berubah, supaya CodeLens tidak berkedip setiap ketikan. */
  sceneChanged(uri: string, scene: Scene): void {
    const sig = scene.blocks.map(b => `${b.id}:${b.line}`).join(',');
    if (this.signatures.get(uri) !== sig) {
      this.signatures.set(uri, sig);
      this.emitter.fire();
    }
  }

  refresh(): void {
    this.emitter.fire();
  }

  forget(uri: string): void {
    this.signatures.delete(uri);
  }

  provideCodeLenses(doc: vscode.TextDocument): vscode.CodeLens[] {
    if (!vscode.workspace.getConfiguration('gopherviz').get<boolean>('codeLens', true)) {
      return [];
    }
    const scene = this.getScene(doc.uri.toString());
    if (!scene) {
      return [];
    }
    const what = { func: 'fungsi', if: 'if', switch: 'switch', loop: 'loop' } as const;
    // Satu tombol per baris: blok terluar yang mulai di baris itu (mis. fungsi satu baris berisi loop).
    const seen = new Set<number>();
    return scene.blocks
      .filter(b => b.line - 1 < doc.lineCount && !seen.has(b.line) && seen.add(b.line))
      .map(b => {
        const range = new vscode.Range(b.line - 1, 0, b.line - 1, 0);
        return new vscode.CodeLens(range, {
          title: `▶ Visualisasikan ${what[b.kind]} ${b.id}`,
          command: 'gopherviz.visualizeAtCursor',
          arguments: [doc.uri, b.line],
        });
      });
  }

  dispose(): void {
    this.emitter.dispose();
  }
}
