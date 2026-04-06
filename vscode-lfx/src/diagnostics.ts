import * as vscode from "vscode";
import { LfxCli, MissingBinaryError } from "./cli";
import { getSettings } from "./config";

export class DiagnosticsManager implements vscode.Disposable {
  private readonly cli: LfxCli;
  private readonly collection: vscode.DiagnosticCollection;
  private readonly disposables: vscode.Disposable[] = [];
  private readonly pending = new Map<string, NodeJS.Timeout>();

  public constructor(cli: LfxCli) {
    this.cli = cli;
    this.collection = vscode.languages.createDiagnosticCollection("lfx");
    this.disposables.push(this.collection);

    this.disposables.push(
      vscode.workspace.onDidOpenTextDocument((document) => {
        if (isLfxDocument(document)) {
          void this.checkDocument(document, "open");
        }
      }),
      vscode.workspace.onDidSaveTextDocument((document) => {
        if (isLfxDocument(document) && getSettings(document.uri).checkOnSave) {
          void this.checkDocument(document, "save");
        }
      }),
      vscode.workspace.onDidChangeTextDocument((event) => {
        const { document } = event;
        if (!isLfxDocument(document) || !getSettings(document.uri).checkOnType) {
          return;
        }
        this.scheduleCheck(document);
      }),
      vscode.workspace.onDidCloseTextDocument((document) => {
        this.clearDocument(document);
      })
    );
  }

  public async activate(): Promise<void> {
    for (const document of vscode.workspace.textDocuments) {
      if (isLfxDocument(document)) {
        await this.checkDocument(document, "activate");
      }
    }
  }

  public async checkDocument(document: vscode.TextDocument, reason: string): Promise<boolean> {
    if (!isLfxDocument(document)) {
      return false;
    }

    this.clearPending(document);
    try {
      const result = await this.cli.checkFile(document);
      const diagnostics = result.diagnostics.map((item) => toVscodeDiagnostic(document, item));
      this.collection.set(document.uri, diagnostics);
      return result.ok;
    } catch (error) {
      this.collection.delete(document.uri);
      if (error instanceof MissingBinaryError) {
        await this.cli.showMissingBinaryWarning();
        return false;
      }

      const message = error instanceof Error ? error.message : String(error);
      void vscode.window.showErrorMessage(`LFX check failed on ${reason}: ${message}`);
      return false;
    }
  }

  public async checkWorkspace(): Promise<void> {
    const uris = await vscode.workspace.findFiles("**/*.lfx", "**/{node_modules,.git}/**");
    for (const uri of uris) {
      const document = await vscode.workspace.openTextDocument(uri);
      await this.checkDocument(document, "workspace");
    }
  }

  public clearDocument(document: vscode.TextDocument): void {
    this.clearPending(document);
    this.collection.delete(document.uri);
  }

  public dispose(): void {
    for (const timeout of this.pending.values()) {
      clearTimeout(timeout);
    }
    this.pending.clear();
    vscode.Disposable.from(...this.disposables).dispose();
  }

  private scheduleCheck(document: vscode.TextDocument): void {
    this.clearPending(document);
    const timeout = setTimeout(() => {
      void this.checkDocument(document, "type");
    }, getSettings(document.uri).debounceMs);
    this.pending.set(document.uri.toString(), timeout);
  }

  private clearPending(document: vscode.TextDocument): void {
    const key = document.uri.toString();
    const timeout = this.pending.get(key);
    if (!timeout) {
      return;
    }
    clearTimeout(timeout);
    this.pending.delete(key);
  }
}

function isLfxDocument(document: vscode.TextDocument): boolean {
  return document.languageId === "lfx" || document.uri.fsPath.endsWith(".lfx");
}

function toVscodeDiagnostic(
  document: vscode.TextDocument,
  item: {
    severity: string;
    code?: string;
    message: string;
    line?: number;
    column?: number;
    length?: number;
  }
): vscode.Diagnostic {
  const line = Math.max((item.line ?? 1) - 1, 0);
  const column = Math.max((item.column ?? 1) - 1, 0);
  const pos = new vscode.Position(line, column);
  let range: vscode.Range;
  if (item.length && item.length > 0) {
    range = new vscode.Range(pos, pos.translate(0, item.length));
  } else {
    const wordRange = document.getWordRangeAtPosition(pos);
    range = wordRange || new vscode.Range(pos, pos.translate(0, 1));
  }
  const diagnostic = new vscode.Diagnostic(range, item.message, toSeverity(item.severity));
  diagnostic.source = "lfx";
  if (item.code) {
    diagnostic.code = item.code;
  }
  return diagnostic;
}

function toSeverity(severity: string): vscode.DiagnosticSeverity {
  switch (severity) {
    case "warning":
      return vscode.DiagnosticSeverity.Warning;
    case "information":
      return vscode.DiagnosticSeverity.Information;
    case "hint":
      return vscode.DiagnosticSeverity.Hint;
    default:
      return vscode.DiagnosticSeverity.Error;
  }
}
