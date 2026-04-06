import * as vscode from "vscode";
import { LfxCli } from "./cli";
import { registerCommands, VirtualDocumentManager } from "./commands";
import { DiagnosticsManager } from "./diagnostics";
import { PreviewManager } from "./preview/panel";

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  const outputChannel = vscode.window.createOutputChannel("LFX Language Support");
  const cli = new LfxCli(outputChannel, context.extensionUri);
  const diagnostics = new DiagnosticsManager(cli);
  const virtualDocuments = new VirtualDocumentManager();
  const preview = new PreviewManager(context, cli, outputChannel);

  context.subscriptions.push(
    outputChannel,
    diagnostics,
    virtualDocuments,
    preview,
    vscode.workspace.registerTextDocumentContentProvider("lfx-wgsl", virtualDocuments),
    vscode.workspace.registerTextDocumentContentProvider("lfx-graph", virtualDocuments)
  );

  registerCommands(context, cli, diagnostics, virtualDocuments, preview);
  await diagnostics.activate();
}

export function deactivate(): void {
  // VS Code disposes extension subscriptions automatically.
}
