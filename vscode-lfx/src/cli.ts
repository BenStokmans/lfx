import * as nodeFs from "node:fs";
import * as fs from "node:fs/promises";
import * as path from "node:path";
import { execFile } from "node:child_process";
import * as vscode from "vscode";
import { buildCompilerInvocations, bundledBinaryPath, type CompilerInvocation } from "./compilerResolver";
import { getSettings, resolveModuleRoots } from "./config";

export interface LfxCliDiagnostic {
  severity: string;
  code?: string;
  message: string;
  filePath?: string;
  modulePath?: string;
  line?: number;
  column?: number;
  length?: number;
}

export interface LfxCheckResult {
  ok: boolean;
  filePath: string;
  modulePath?: string;
  diagnostics: LfxCliDiagnostic[];
}

export interface LfxPreviewParam {
  name: string;
  type: "int" | "float" | "bool" | "enum" | string;
  defaultValue: unknown;
  min?: number;
  max?: number;
  enumValues?: string[];
}

export interface LfxPreviewTimeline {
  loopStart?: number;
  loopEnd?: number;
}

export interface LfxPreviewResult {
  ok: boolean;
  filePath: string;
  modulePath?: string;
  outputType?: "scalar" | "rgb" | "rgbw";
  wgsl?: string;
  params: LfxPreviewParam[];
  boundParams?: Record<string, unknown>;
  timeline?: LfxPreviewTimeline;
  diagnostics: LfxCliDiagnostic[];
}

interface CommandExecution {
  stdout: string;
  stderr: string;
}

interface Invocation {
  command: string;
  args: string[];
  cwd: string | undefined;
  source: "bundled" | "configured" | "path" | "go-run";
}

export class MissingBinaryError extends Error {
  public constructor(message: string) {
    super(message);
    this.name = "MissingBinaryError";
  }
}

export class LfxCli {
  private readonly outputChannel: vscode.OutputChannel;
  private readonly extensionRoot: string;
  private missingBinaryWarningShown = false;
  private resolvedSourceLabel = "pending";

  public constructor(outputChannel: vscode.OutputChannel, extensionUri?: vscode.Uri) {
    this.outputChannel = outputChannel;
    this.extensionRoot = extensionUri?.fsPath ?? "";
  }

  public async checkFile(document: vscode.TextDocument): Promise<LfxCheckResult> {
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
    const args = this.withModuleRoots(["check", "--json", document.uri.fsPath], workspaceFolder);
    const execution = await this.runWithFallback(args, workspaceFolder);
    this.trace(`check stdout for ${document.uri.fsPath}:\n${execution.stdout}`);
    if (execution.stderr.trim().length > 0) {
      this.trace(`check stderr for ${document.uri.fsPath}:\n${execution.stderr}`);
    }

    let payload: LfxCheckResult;
    try {
      payload = JSON.parse(execution.stdout) as LfxCheckResult;
    } catch (error) {
      throw new Error(
        `Failed to parse JSON output from 'lfx check --json': ${error instanceof Error ? error.message : String(error)}`
      );
    }
    payload.filePath ||= document.uri.fsPath;
    payload.diagnostics ??= [];
    return payload;
  }

  public async previewFile(document: vscode.TextDocument, overrides: Record<string, unknown>): Promise<LfxPreviewResult> {
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
    const args = this.withModuleRoots(
      ["preview", "--json", ...paramsToArgs(overrides), document.uri.fsPath],
      workspaceFolder
    );
    const execution = await this.runWithFallback(args, workspaceFolder);
    if (execution.stderr.trim().length > 0) {
      this.trace(`preview stderr for ${document.uri.fsPath}:\n${execution.stderr}`);
    }

    try {
      const payload = JSON.parse(execution.stdout) as LfxPreviewResult;
      payload.filePath ||= document.uri.fsPath;
      payload.params ??= [];
      payload.diagnostics ??= [];
      return payload;
    } catch (error) {
      throw new Error(
        `Failed to parse JSON output from 'lfx preview --json': ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  public async runTextCommand(
    document: vscode.TextDocument,
    subcommand: string,
    extraArgs: string[]
  ): Promise<string> {
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
    const args = this.withModuleRoots([subcommand, ...extraArgs, document.uri.fsPath], workspaceFolder);
    const execution = await this.runWithFallback(args, workspaceFolder);
    if (execution.stderr.trim().length > 0) {
      this.trace(`${subcommand} stderr for ${document.uri.fsPath}:\n${execution.stderr}`);
    }
    return execution.stdout;
  }

  public async runJsonCommand<T>(
    document: vscode.TextDocument,
    subcommand: string,
    extraArgs: string[]
  ): Promise<T> {
    const stdout = await this.runTextCommand(document, subcommand, extraArgs);
    try {
      return JSON.parse(stdout) as T;
    } catch (error) {
      throw new Error(
        `Failed to parse JSON output from 'lfx ${subcommand}': ${error instanceof Error ? error.message : String(error)}`
      );
    }
  }

  public async showMissingBinaryWarning(): Promise<void> {
    if (this.missingBinaryWarningShown) {
      return;
    }
    this.missingBinaryWarningShown = true;
    const selection = await vscode.window.showWarningMessage(
      "LFX executable not found. Install 'lfx', package a bundled compiler, or set 'lfx.path' in your VS Code settings.",
      "Open Settings"
    );
    if (selection === "Open Settings") {
      await vscode.commands.executeCommand("workbench.action.openSettings", "lfx.path");
    }
  }

  public lastResolvedSourceLabel(): string {
    return this.resolvedSourceLabel;
  }

  private async runWithFallback(args: string[], workspaceFolder: vscode.WorkspaceFolder | undefined): Promise<CommandExecution> {
    const settings = getSettings(workspaceFolder);
    const bundled = await this.resolveBundledInvocation(args);
    const repoFallback = await this.resolveGoRunInvocation(args, workspaceFolder);
    const attempts = buildCompilerInvocations(
      {
        configuredPath: settings.path,
        args,
        workspaceFolderPath: repoFallback?.cwd
      },
      bundled?.command
    ).map((invocation) => () => this.exec(invocation));

    let lastError: unknown;
    for (const attempt of attempts) {
      try {
        return await attempt();
      } catch (error) {
        lastError = error;
        if (!isMissingExecutable(error)) {
          throw error;
        }
      }
    }

    throw new MissingBinaryError(
      lastError instanceof Error ? lastError.message : "Unable to locate the LFX executable."
    );
  }

  private withModuleRoots(args: string[], workspaceFolder: vscode.WorkspaceFolder | undefined): string[] {
    const settings = getSettings(workspaceFolder);
    const roots = resolveModuleRoots(settings, workspaceFolder);
    if (roots.length === 0) {
      return args;
    }

    const prefix = roots.flatMap((root) => ["--module-root", root]);
    return [...args.slice(0, 1), ...prefix, ...args.slice(1)];
  }

  private async resolveBundledInvocation(args: string[]): Promise<Invocation | undefined> {
    if (!this.extensionRoot) {
      return undefined;
    }

    const candidate = bundledBinaryPath(this.extensionRoot);
    if (!candidate) {
      return undefined;
    }

    try {
      await fs.access(candidate, nodeFs.constants.X_OK);
      return {
        command: candidate,
        args,
        cwd: undefined,
        source: "bundled"
      };
    } catch {
      return undefined;
    }
  }

  private async resolveGoRunInvocation(
    args: string[],
    workspaceFolder: vscode.WorkspaceFolder | undefined
  ): Promise<Invocation | undefined> {
    if (!workspaceFolder) {
      return undefined;
    }

    const repoRoot = workspaceFolder.uri.fsPath;
    const markers = [
      path.join(repoRoot, "go.mod"),
      path.join(repoRoot, "cmd", "lfx", "main.go"),
      path.join(repoRoot, "docs", "LANGUAGE.md")
    ];
    try {
      await Promise.all(markers.map((marker) => vscode.workspace.fs.stat(vscode.Uri.file(marker))));
    } catch {
      return undefined;
    }

    return {
      command: "go",
      args: ["run", "./cmd/lfx", ...args],
      cwd: repoRoot,
      source: "go-run"
    };
  }

  private exec(invocation: CompilerInvocation): Promise<CommandExecution> {
    this.resolvedSourceLabel = compilerSourceLabel(invocation);
    this.outputChannel.appendLine(`[LFX compiler] using ${this.resolvedSourceLabel}`);
    this.trace(`Running ${invocation.source}: ${invocation.command} ${invocation.args.join(" ")}`);
    return new Promise<CommandExecution>((resolve, reject) => {
      execFile(invocation.command, invocation.args, { cwd: invocation.cwd }, (error, stdout, stderr) => {
        if (error) {
          const execError = error as NodeJS.ErrnoException;
          if (isMissingExecutable(execError)) {
            reject(execError);
            return;
          }

          const detail = stderr.trim() || execError.message;
          reject(new Error(detail));
          return;
        }

        resolve({ stdout, stderr });
      });
    });
  }

  private trace(message: string): void {
    if (!getSettings().traceDiagnostics && !getSettings().previewTrace) {
      return;
    }
    this.outputChannel.appendLine(message);
  }
}

function isMissingExecutable(error: unknown): boolean {
  if (!(error instanceof Error)) {
    return false;
  }
  const errnoError = error as NodeJS.ErrnoException;
  return errnoError.code === "ENOENT";
}

function paramsToArgs(overrides: Record<string, unknown>): string[] {
  const args: string[] = [];
  const entries = Object.entries(overrides).sort(([left], [right]) => left.localeCompare(right));
  for (const [name, value] of entries) {
    if (typeof value === "number" || typeof value === "boolean" || typeof value === "string") {
      args.push("--param", `${name}=${String(value)}`);
    }
  }
  return args;
}

function compilerSourceLabel(invocation: CompilerInvocation): string {
  switch (invocation.source) {
    case "bundled":
      return "bundled binary";
    case "configured":
      return `configured path (${invocation.command})`;
    case "go-run":
      return "go run ./cmd/lfx";
    default:
      return "PATH lookup (lfx)";
  }
}
