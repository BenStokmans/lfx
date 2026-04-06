import * as path from "node:path";
import * as vscode from "vscode";

const SECTION = "lfx";

export interface LfxSettings {
  path: string;
  checkOnSave: boolean;
  checkOnType: boolean;
  debounceMs: number;
  moduleRoots: string[];
  traceDiagnostics: boolean;
  previewEnableOnSave: boolean;
  previewTrackActiveEditor: boolean;
  previewPreferredLayout: string;
  previewTrace: boolean;
}

export function getSettings(scope?: vscode.ConfigurationScope): LfxSettings {
  const config = vscode.workspace.getConfiguration(SECTION, scope);
  return {
    path: config.get<string>("path", "lfx"),
    checkOnSave: config.get<boolean>("check.onSave", true),
    checkOnType: config.get<boolean>("check.onType", false),
    debounceMs: config.get<number>("check.debounceMs", 400),
    moduleRoots: config.get<string[]>("moduleRoots", []),
    traceDiagnostics: config.get<boolean>("trace.diagnostics", false),
    previewEnableOnSave: config.get<boolean>("preview.enableOnSave", true),
    previewTrackActiveEditor: config.get<boolean>("preview.trackActiveEditor", true),
    previewPreferredLayout: config.get<string>("preview.preferredLayout", "grid-32x32"),
    previewTrace: config.get<boolean>("preview.trace", false)
  };
}

export function resolveModuleRoots(
  settings: LfxSettings,
  workspaceFolder: vscode.WorkspaceFolder | undefined
): string[] {
  return settings.moduleRoots
    .map((root) => root.trim())
    .filter((root) => root.length > 0)
    .map((root) => {
      if (path.isAbsolute(root) || !workspaceFolder) {
        return root;
      }
      return path.join(workspaceFolder.uri.fsPath, root);
    });
}
