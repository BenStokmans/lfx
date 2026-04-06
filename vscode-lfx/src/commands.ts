import * as vscode from "vscode";
import { LfxCli, MissingBinaryError } from "./cli";
import { DiagnosticsManager } from "./diagnostics";
import { PreviewManager } from "./preview/panel";

interface ModuleGraphNode {
  path: string;
  is_library: boolean;
  imports: string[];
}

interface ModuleGraphPayload {
  entry: string;
  nodes: ModuleGraphNode[];
}

export class VirtualDocumentManager implements vscode.TextDocumentContentProvider, vscode.Disposable {
  private readonly emitter = new vscode.EventEmitter<vscode.Uri>();
  private readonly documents = new Map<string, string>();

  public readonly onDidChange = this.emitter.event;

  public provideTextDocumentContent(uri: vscode.Uri): string {
    return this.documents.get(uri.toString()) ?? "";
  }

  public async openVirtualDocument(kind: string, title: string, content: string, language = "plaintext"): Promise<void> {
    const uri = vscode.Uri.parse(`lfx-${kind}:${encodeURIComponent(title)}.${language}`);
    this.documents.set(uri.toString(), content);
    this.emitter.fire(uri);
    let document = await vscode.workspace.openTextDocument(uri);
    if (document.languageId !== language) {
      try {
        document = await vscode.languages.setTextDocumentLanguage(document, language);
      } catch {
        // Fall back to the provider's default language when the requested mode is unavailable.
      }
    }
    await vscode.window.showTextDocument(document, { preview: false, viewColumn: vscode.ViewColumn.Beside });
  }

  public dispose(): void {
    this.documents.clear();
    this.emitter.dispose();
  }
}

export function registerCommands(
  context: vscode.ExtensionContext,
  cli: LfxCli,
  diagnostics: DiagnosticsManager,
  virtualDocuments: VirtualDocumentManager,
  preview: PreviewManager
): void {
  context.subscriptions.push(
    vscode.commands.registerCommand("lfx.checkCurrentFile", async () => {
      const document = await requireLfxDocument();
      if (!document) {
        return;
      }
      const ok = await diagnostics.checkDocument(document, "command");
      if (ok) {
        void vscode.window.setStatusBarMessage("LFX check passed.", 2000);
      }
    }),
    vscode.commands.registerCommand("lfx.checkWorkspace", async () => {
      await diagnostics.checkWorkspace();
      void vscode.window.setStatusBarMessage("LFX workspace check finished.", 2500);
    }),
    vscode.commands.registerCommand("lfx.showGeneratedWgsl", async () => {
      const document = await requireLfxDocument();
      if (!document) {
        return;
      }
      try {
        const wgsl = await cli.runTextCommand(document, "emit-wgsl", []);
        await virtualDocuments.openVirtualDocument("wgsl", `${document.fileName} WGSL`, wgsl, "wgsl");
      } catch (error) {
        await handleCommandError(cli, error);
      }
    }),
    vscode.commands.registerCommand("lfx.openModuleGraph", async () => {
      const document = await requireLfxDocument();
      if (!document) {
        return;
      }
      try {
        const graph = await cli.runJsonCommand<ModuleGraphPayload>(document, "graph", []);
        const content = renderModuleGraph(graph);
        await virtualDocuments.openVirtualDocument("graph", `${document.fileName} Graph`, content, "plaintext");
      } catch (error) {
        await handleCommandError(cli, error);
      }
    }),
    vscode.commands.registerCommand("lfx.webgpuProbe", async () => {
      await preview.openProbe();
    }),
    vscode.commands.registerCommand("lfx.openLivePreview", async () => {
      await preview.openLivePreview();
    }),
    vscode.commands.registerCommand("lfx.refreshLivePreview", async () => {
      await preview.refreshLivePreview();
    })
  );
}

async function requireLfxDocument(): Promise<vscode.TextDocument | undefined> {
  const editor = vscode.window.activeTextEditor;
  if (!editor || (editor.document.languageId !== "lfx" && !editor.document.uri.fsPath.endsWith(".lfx"))) {
    await vscode.window.showInformationMessage("Open an .lfx file to use this command.");
    return undefined;
  }
  return editor.document;
}

async function handleCommandError(cli: LfxCli, error: unknown): Promise<void> {
  if (error instanceof MissingBinaryError) {
    await cli.showMissingBinaryWarning();
    return;
  }
  const message = error instanceof Error ? error.message : String(error);
  await vscode.window.showErrorMessage(`LFX command failed: ${message}`);
}

function renderModuleGraph(graph: ModuleGraphPayload): string {
  const byPath = new Map(graph.nodes.map((node) => [node.path, node]));
  const lines: string[] = [];
  const seen = new Set<string>();

  const visit = (modulePath: string, prefix: string, isLast: boolean): void => {
    const node = byPath.get(modulePath);
    if (!node) {
      return;
    }

    const marker = prefix.length === 0 ? "" : isLast ? "└─ " : "├─ ";
    const suffix = node.is_library ? " [library]" : " [effect]";
    lines.push(`${prefix}${marker}${modulePath}${suffix}`);

    if (seen.has(modulePath)) {
      return;
    }
    seen.add(modulePath);

    const childPrefix = prefix + (prefix.length === 0 ? "" : isLast ? "   " : "│  ");
    node.imports.forEach((child, index) => {
      visit(child, childPrefix, index === node.imports.length - 1);
    });
  };

  lines.push(`Entry: ${graph.entry}`);
  lines.push("");
  visit(graph.entry, "", true);

  if (graph.nodes.length > seen.size) {
    lines.push("");
    lines.push("Other nodes:");
    for (const node of graph.nodes) {
      if (!seen.has(node.path)) {
        lines.push(`- ${node.path}${node.is_library ? " [library]" : " [effect]"}`);
      }
    }
  }

  return lines.join("\n");
}
