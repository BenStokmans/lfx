import * as crypto from "node:crypto";
import * as vscode from "vscode";

export function getPreviewHtml(webview: vscode.Webview, extensionUri: vscode.Uri): string {
  const nonce = crypto.randomBytes(16).toString("base64");
  const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, "webview", "preview.js"));
  const styleUri = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, "webview", "preview.css"));

  return `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src ${webview.cspSource} https: data:; style-src ${webview.cspSource}; script-src 'nonce-${nonce}';" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <link rel="stylesheet" href="${styleUri}" />
    <title>LFX Live Preview</title>
  </head>
  <body>
    <div class="shell">
      <header class="toolbar">
        <div class="toolbar__title">
          <strong id="title">LFX Live Preview</strong>
          <span id="subtitle">Waiting for source…</span>
        </div>
        <div class="toolbar__actions">
          <button id="refreshButton" type="button">Refresh</button>
          <button id="playButton" type="button">Play</button>
        </div>
      </header>
      <section class="controls">
        <label>
          <span>Phase</span>
          <input id="phaseInput" type="range" min="0" max="1" step="0.001" value="0" />
          <output id="phaseValue">0.000</output>
        </label>
        <label>
          <span>Speed</span>
          <input id="speedInput" type="range" min="0.1" max="3" step="0.1" value="1" />
          <output id="speedValue">1.0x</output>
        </label>
        <label>
          <span>Layout</span>
          <select id="layoutSelect"></select>
        </label>
        <label>
          <span>Render Mode</span>
          <select id="renderModeSelect">
            <option value="points" selected>Discrete Points</option>
            <option value="solid">Solid Canvas</option>
          </select>
        </label>
      </section>
      <section id="paramsPanel" class="params"></section>
      <section class="status">
        <span id="compilerSource">Compiler: pending</span>
        <span id="probeStatus">WebGPU probe pending</span>
      </section>
      <section class="viewport">
        <canvas id="canvas"></canvas>
        <div id="overlay" class="overlay hidden"></div>
      </section>
      <section class="diagnostics">
        <h2>Diagnostics</h2>
        <pre id="diagnosticsOutput">No diagnostics.</pre>
      </section>
    </div>
    <script nonce="${nonce}" src="${scriptUri}"></script>
  </body>
</html>`;
}
