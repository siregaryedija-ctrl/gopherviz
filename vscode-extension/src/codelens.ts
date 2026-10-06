import * as vscode from 'vscode';
import type { Scene } from './server';

/** Tombol "▶ Visualisasikan loop" di atas setiap for. */
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
    const sig = scene.loops.map(l => `${l.id}:${l.line}`).join(',');
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
    return scene.loops
      .filter(l => l.line - 1 < doc.lineCount)
      .map(l => {
        const range = new vscode.Range(l.line - 1, 0, l.line - 1, 0);
        return new vscode.CodeLens(range, {
          title: `▶ Visualisasikan loop ${l.id}`,
          command: 'gopherviz.visualizeAtCursor',
          arguments: [doc.uri, l.line],
        });
      });
  }

  dispose(): void {
    this.emitter.dispose();
  }
}
