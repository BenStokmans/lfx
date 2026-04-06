import * as vscode from "vscode";
import { LfxCli, MissingBinaryError, type LfxPreviewResult } from "../cli";
import { getSettings } from "../config";
import { getPreviewHtml } from "./html";
import { builtInLayouts } from "./layouts";
import type { ExtensionToWebviewMessage, PreviewArtifact, PreviewLayout, WebviewToExtensionMessage } from "./protocol";

export class PreviewManager implements vscode.Disposable {
  private readonly context: vscode.ExtensionContext;
  private readonly cli: LfxCli;
  private readonly outputChannel: vscode.OutputChannel;
  private readonly layouts: PreviewLayout[];
  private readonly disposables: vscode.Disposable[] = [];
  private previewPanel: vscode.WebviewPanel | undefined;
  private probePanel: vscode.WebviewPanel | undefined;
  private previewDocument: vscode.TextDocument | undefined;
  private previewParams: Record<string, unknown> = {};
  private selectedLayoutId: string;
  private lastGoodArtifact: PreviewArtifact | undefined;

  public constructor(context: vscode.ExtensionContext, cli: LfxCli, outputChannel: vscode.OutputChannel) {
    this.context = context;
    this.cli = cli;
    this.outputChannel = outputChannel;
    this.layouts = builtInLayouts();
    this.selectedLayoutId = getSettings().previewPreferredLayout;

    this.disposables.push(
      vscode.workspace.onDidSaveTextDocument(async (document) => {
        if (!this.previewPanel || !this.previewDocument || document.uri.toString() !== this.previewDocument.uri.toString()) {
          return;
        }
        if (!getSettings(document.uri).previewEnableOnSave) {
          return;
        }
        await this.compileAndPost(document, false);
      }),
      vscode.window.onDidChangeActiveColorTheme(() => {
        this.postToPanels({
          type: "setTheme",
          themeKind: themeKindName(vscode.window.activeColorTheme.kind)
        });
      }),
      vscode.window.onDidChangeActiveTextEditor(async (editor) => {
        if (!editor || !this.previewPanel || !isLfxDocument(editor.document)) {
          return;
        }
        if (!getSettings(editor.document.uri).previewTrackActiveEditor) {
          return;
        }
        this.previewDocument = editor.document;
        this.previewPanel.title = `LFX Preview: ${editor.document.fileName.split(/[\\/]/).pop() ?? editor.document.fileName}`;
        await this.compileAndPost(editor.document, false);
      })
    );
  }

  public async openProbe(): Promise<void> {
    const panel = this.probePanel ?? this.createPanel("probe");
    this.probePanel = panel;
    panel.reveal(vscode.ViewColumn.Beside, true);
    this.postToWebview(panel, {
      type: "init",
      mode: "probe",
      layouts: [],
      preferredLayoutId: "",
      themeKind: themeKindName(vscode.window.activeColorTheme.kind)
    });
  }

  public async openLivePreview(): Promise<void> {
    const document = await requireLfxDocument();
    if (!document) {
      return;
    }

    this.previewDocument = document;
    const panel = this.previewPanel ?? this.createPanel("preview");
    this.previewPanel = panel;
    panel.title = `LFX Preview: ${document.fileName.split(/[\\/]/).pop() ?? document.fileName}`;
    panel.reveal(vscode.ViewColumn.Beside, true);
    this.postToWebview(panel, {
      type: "init",
      mode: "preview",
      layouts: this.layouts,
      preferredLayoutId: this.selectedLayoutId,
      filePath: document.uri.fsPath,
      compilerSource: this.cli.lastResolvedSourceLabel(),
      themeKind: themeKindName(vscode.window.activeColorTheme.kind)
    });
    await this.refreshLivePreview();
  }

  public async refreshLivePreview(): Promise<void> {
    if (!this.previewPanel) {
      await this.openLivePreview();
      return;
    }

    const document = this.previewDocument ?? (await requireLfxDocument());
    if (!document) {
      return;
    }
    if (document.isDirty) {
      const saved = await document.save();
      if (!saved) {
        return;
      }
    }
    this.previewDocument = document;
    await this.compileAndPost(document, false);
  }

  public dispose(): void {
    this.postToPanels({ type: "dispose" });
    vscode.Disposable.from(...this.disposables).dispose();
    this.previewPanel?.dispose();
    this.probePanel?.dispose();
  }

  private createPanel(mode: "preview" | "probe"): vscode.WebviewPanel {
    const panel = vscode.window.createWebviewPanel(
      mode === "preview" ? "lfxPreview" : "lfxWebgpuProbe",
      mode === "preview" ? "LFX Live Preview" : "LFX WebGPU Probe",
      { viewColumn: vscode.ViewColumn.Beside, preserveFocus: mode === "probe" },
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        localResourceRoots: [vscode.Uri.joinPath(this.context.extensionUri, "webview")]
      }
    );

    panel.webview.html = getPreviewHtml(panel.webview, this.context.extensionUri);
    panel.onDidDispose(() => {
      if (mode === "preview") {
        this.previewPanel = undefined;
      } else {
        this.probePanel = undefined;
      }
    });
    panel.webview.onDidReceiveMessage(async (message: WebviewToExtensionMessage) => {
      await this.handleMessage(mode, message);
    });

    return panel;
  }

  private async handleMessage(mode: "preview" | "probe", message: WebviewToExtensionMessage): Promise<void> {
    switch (message.type) {
      case "ready":
        if (mode === "probe" && this.probePanel) {
          this.postToWebview(this.probePanel, {
            type: "init",
            mode: "probe",
            layouts: [],
            preferredLayoutId: "",
            themeKind: themeKindName(vscode.window.activeColorTheme.kind)
          });
        }
        if (mode === "preview" && this.previewPanel) {
          this.postToWebview(this.previewPanel, {
            type: "init",
            mode: "preview",
            layouts: this.layouts,
            preferredLayoutId: this.selectedLayoutId,
            filePath: this.previewDocument?.uri.fsPath,
            compilerSource: this.cli.lastResolvedSourceLabel(),
            themeKind: themeKindName(vscode.window.activeColorTheme.kind)
          });
          if (this.lastGoodArtifact) {
            this.postToWebview(this.previewPanel, {
              type: "compiledEffect",
              artifact: this.lastGoodArtifact,
              compilerSource: this.cli.lastResolvedSourceLabel()
            });
          }
        }
        break;
      case "probeResult":
        if (getSettings().previewTrace) {
          this.outputChannel.appendLine(
            `[LFX preview] probe ${message.ok ? "ok" : "failed"}: ${JSON.stringify(message.details)}`
          );
        }
        break;
      case "setLayout":
        this.selectedLayoutId = message.layoutId;
        break;
      case "setParams":
        this.previewParams = message.params;
        break;
      case "requestRefresh":
        await this.refreshLivePreview();
        break;
      case "runtimeError":
        this.outputChannel.appendLine(`[LFX preview] runtime error: ${message.message}`);
        break;
      case "setPlayback":
      case "setPhase":
        break;
    }
  }

  private async compileAndPost(document: vscode.TextDocument, openingPreview: boolean): Promise<void> {
    try {
      const artifact = toPreviewArtifact(await this.cli.previewFile(document, this.previewParams));
      if (artifact.ok && artifact.wgsl) {
        this.lastGoodArtifact = artifact;
        this.postToWebview(this.previewPanel, {
          type: "compiledEffect",
          artifact,
          compilerSource: this.cli.lastResolvedSourceLabel()
        });
        return;
      }

      this.postToWebview(this.previewPanel, {
        type: "compileError",
        artifact,
        compilerSource: this.cli.lastResolvedSourceLabel(),
        reason: openingPreview && document.isDirty ? "Preview reflects the last saved file. Save to update." : undefined
      });
    } catch (error) {
      if (error instanceof MissingBinaryError) {
        await this.cli.showMissingBinaryWarning();
        return;
      }

      const message = error instanceof Error ? error.message : String(error);
      this.postToWebview(this.previewPanel, {
        type: "compileError",
        artifact: {
          ok: false,
          filePath: document.uri.fsPath,
          params: [],
          diagnostics: [{ severity: "error", message }]
        },
        compilerSource: this.cli.lastResolvedSourceLabel(),
        reason: message
      });
    }
  }

  private postToPanels(message: ExtensionToWebviewMessage): void {
    this.postToWebview(this.previewPanel, message);
    this.postToWebview(this.probePanel, message);
  }

  private postToWebview(panel: vscode.WebviewPanel | undefined, message: ExtensionToWebviewMessage): void {
    if (!panel) {
      return;
    }
    void panel.webview.postMessage(message);
  }
}

function toPreviewArtifact(result: LfxPreviewResult): PreviewArtifact {
  return {
    ok: result.ok,
    filePath: result.filePath,
    modulePath: result.modulePath,
    outputType: result.outputType,
    wgsl: result.wgsl,
    params: result.params,
    boundParams: result.boundParams,
    timeline: result.timeline,
    diagnostics: result.diagnostics
  };
}

async function requireLfxDocument(): Promise<vscode.TextDocument | undefined> {
  const editor = vscode.window.activeTextEditor;
  if (!editor || !isLfxDocument(editor.document)) {
    await vscode.window.showInformationMessage("Open an .lfx file to use this command.");
    return undefined;
  }
  return editor.document;
}

function isLfxDocument(document: vscode.TextDocument): boolean {
  return document.languageId === "lfx" || document.uri.fsPath.endsWith(".lfx");
}

function themeKindName(kind: vscode.ColorThemeKind): string {
  switch (kind) {
    case vscode.ColorThemeKind.Light:
      return "light";
    case vscode.ColorThemeKind.HighContrastLight:
      return "high-contrast-light";
    case vscode.ColorThemeKind.HighContrast:
      return "high-contrast";
    default:
      return "dark";
  }
}
