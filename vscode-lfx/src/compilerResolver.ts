import * as path from "node:path";

export interface CompilerInvocation {
  command: string;
  args: string[];
  cwd: string | undefined;
  source: "bundled" | "configured" | "path" | "go-run";
}

export interface CompilerResolutionOptions {
  configuredPath: string;
  args: string[];
  workspaceFolderPath?: string;
}

export function bundledBinaryRelativePath(platform = process.platform, arch = process.arch): string | undefined {
  const executable = platform === "win32" ? "lfx.exe" : "lfx";
  switch (`${platform}-${arch}`) {
    case "darwin-arm64":
    case "darwin-x64":
    case "linux-arm64":
    case "linux-x64":
    case "win32-x64":
      return path.join("bin", `${platform}-${arch}`, executable);
    default:
      return undefined;
  }
}

export function bundledBinaryPath(extensionRoot: string, platform = process.platform, arch = process.arch): string | undefined {
  const relative = bundledBinaryRelativePath(platform, arch);
  if (!relative) {
    return undefined;
  }
  return path.join(extensionRoot, relative);
}

export function buildCompilerInvocations(
  options: CompilerResolutionOptions,
  bundledPath?: string
): CompilerInvocation[] {
  const invocations: CompilerInvocation[] = [];
  const configured = options.configuredPath.trim();

  if (bundledPath) {
    invocations.push({
      command: bundledPath,
      args: options.args,
      cwd: undefined,
      source: "bundled"
    });
  }

  if (configured.length > 0) {
    invocations.push({
      command: configured,
      args: options.args,
      cwd: undefined,
      source: configured === "lfx" ? "path" : "configured"
    });
  }

  if (configured !== "lfx") {
    invocations.push({
      command: "lfx",
      args: options.args,
      cwd: undefined,
      source: "path"
    });
  }

  if (options.workspaceFolderPath) {
    invocations.push({
      command: "go",
      args: ["run", "./cmd/lfx", ...options.args],
      cwd: options.workspaceFolderPath,
      source: "go-run"
    });
  }

  return dedupeInvocations(invocations);
}

function dedupeInvocations(invocations: CompilerInvocation[]): CompilerInvocation[] {
  const seen = new Set<string>();
  const unique: CompilerInvocation[] = [];
  for (const invocation of invocations) {
    const key = `${invocation.command}\n${invocation.cwd ?? ""}\n${invocation.args.join("\u0000")}`;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    unique.push(invocation);
  }
  return unique;
}
