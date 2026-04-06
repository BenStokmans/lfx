import test from "node:test";
import assert from "node:assert/strict";
import { buildCompilerInvocations, bundledBinaryRelativePath } from "../compilerResolver";

test("bundledBinaryRelativePath returns supported targets", () => {
  assert.equal(bundledBinaryRelativePath("darwin", "arm64"), "bin/darwin-arm64/lfx");
  assert.equal(bundledBinaryRelativePath("linux", "x64"), "bin/linux-x64/lfx");
  assert.equal(bundledBinaryRelativePath("win32", "x64"), "bin/win32-x64/lfx.exe");
  assert.equal(bundledBinaryRelativePath("win32", "arm64"), undefined);
});

test("buildCompilerInvocations prefers bundled then configured then path then go-run", () => {
  const invocations = buildCompilerInvocations(
    {
      configuredPath: "/custom/lfx",
      args: ["preview", "--json", "/tmp/example.lfx"],
      workspaceFolderPath: "/workspace/repo"
    },
    "/extension/bin/darwin-arm64/lfx"
  );

  assert.deepEqual(
    invocations.map((invocation) => ({ command: invocation.command, source: invocation.source, cwd: invocation.cwd })),
    [
      { command: "/extension/bin/darwin-arm64/lfx", source: "bundled", cwd: undefined },
      { command: "/custom/lfx", source: "configured", cwd: undefined },
      { command: "lfx", source: "path", cwd: undefined },
      { command: "go", source: "go-run", cwd: "/workspace/repo" }
    ]
  );
});

test("buildCompilerInvocations deduplicates the default path compiler", () => {
  const invocations = buildCompilerInvocations(
    {
      configuredPath: "lfx",
      args: ["check", "--json", "/tmp/example.lfx"]
    },
    undefined
  );

  assert.deepEqual(invocations.map((invocation) => invocation.source), ["path"]);
});
