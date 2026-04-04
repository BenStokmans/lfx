import Editor from "@monaco-editor/react";
import {
  startTransition,
  useDeferredValue,
  useEffect,
  useRef,
  useState,
} from "react";
import "./App.css";
import {
  bootstrap,
  compilePreview,
  loadLayout,
  openWorkspace,
  readSourceFile,
  sampleCompiledFrame,
  saveSourceFile,
  selectLayoutFile,
} from "./lib/api";
import { drawLayout, EffectRenderer } from "./lib/webgpu";
import type {
  BootstrapData,
  CompileResponse,
  DiagnosticData,
  LayoutData,
  ParamData,
  PresetData,
  WorkspaceData,
} from "./types";

function App() {
  const rendererRef = useRef(new EffectRenderer());
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [workspace, setWorkspace] = useState<WorkspaceData | null>(null);
  const [layouts, setLayouts] = useState<LayoutData[]>([]);
  const [selectedLayoutId, setSelectedLayoutId] = useState("");
  const [selectedFilePath, setSelectedFilePath] = useState("");
  const [source, setSource] = useState("");
  const deferredSource = useDeferredValue(source);
  const [isDirty, setIsDirty] = useState(false);
  const [autoCompile, setAutoCompile] = useState(true);
  const [isBooting, setIsBooting] = useState(true);
  const [isCompiling, setIsCompiling] = useState(false);
  const [compileResult, setCompileResult] = useState<CompileResponse | null>(null);
  const [diagnostics, setDiagnostics] = useState<DiagnosticData[]>([]);
  const [paramOverrides, setParamOverrides] = useState<Record<string, unknown>>({});
  const [selectedPresetName, setSelectedPresetName] = useState("");
  const [phase, setPhase] = useState(0);
  const [speed, setSpeed] = useState(1);
  const [playing, setPlaying] = useState(false);
  const [previewMode, setPreviewMode] = useState<"gpu" | "cpu">("gpu");
  const [renderError, setRenderError] = useState("");
  const [gpuStatus, setGpuStatus] = useState("Checking WebGPU support…");
  const [parityStatus, setParityStatus] = useState("CPU/GPU parity pending");
  const [statusMessage, setStatusMessage] = useState("Loading workspace…");

  useEffect(() => {
    void (async () => {
      try {
        await rendererRef.current.ensureSupport();
        setGpuStatus("WebGPU ready");
      } catch (error) {
        setGpuStatus(formatError(error));
      }

      try {
        const initial = (await bootstrap()) as BootstrapData;
        setLayouts(initial.layouts);
        if (initial.layouts[0]) {
          setSelectedLayoutId(initial.layouts[0].id);
        }
        setWorkspace(initial.workspace);
        if (initial.workspace?.effects[0]) {
          await loadFile(initial.workspace.effects[0].path, initial.workspace.effects);
        }
        setStatusMessage("Preview shell ready");
      } catch (error) {
        setStatusMessage(formatError(error));
      } finally {
        setIsBooting(false);
      }
    })();
  }, []);

  useEffect(() => {
    if (!autoCompile || !selectedFilePath || isBooting) {
      return;
    }
    const timer = window.setTimeout(() => {
      void compileCurrent();
    }, 550);
    return () => window.clearTimeout(timer);
  }, [autoCompile, deferredSource, selectedFilePath, selectedLayoutId, isBooting]);

  useEffect(() => {
    if (!playing) {
      return;
    }

    let frame = 0;
    let last = performance.now();
    const preset = currentPreset(compileResult?.presets ?? [], selectedPresetName);

    const step = (now: number) => {
      const deltaSeconds = (now - last) / 1000;
      last = now;
      const baseRate = ((preset?.speed ?? 1200) / 1200) * 0.15 * speed;
      setPhase((value) => advancePhase(value, deltaSeconds * baseRate, preset));
      frame = requestAnimationFrame(step);
    };

    frame = requestAnimationFrame(step);
    return () => cancelAnimationFrame(frame);
  }, [playing, speed, selectedPresetName, compileResult]);

  useEffect(() => {
    const layout = selectedLayout(layouts, selectedLayoutId);
    if (!compileResult?.compilationId || !layout || !canvasRef.current) {
      return;
    }
    const activeCompile = compileResult as CompileResponse & {
      compilationId: string;
      wgsl?: string;
    };

    let cancelled = false;
    void (async () => {
      try {
        if (previewMode === "cpu") {
          const cpu = await sampleCompiledFrame({
            compilationId: activeCompile.compilationId,
            layout,
            phase,
            overrides: paramOverrides,
            limit: layout.points.length,
          });
          if (cancelled || !canvasRef.current) {
            return;
          }
          drawLayout(canvasRef.current, layout, Float32Array.from(cpu.points, (point) => point.value));
          setRenderError("");
          setParityStatus(`CPU preview mode across ${cpu.points.length} samples`);
          return;
        }

        if (!activeCompile.wgsl) {
          return;
        }

        const values = await rendererRef.current.render({
          wgsl: activeCompile.wgsl,
          layout,
          phase,
          params: activeCompile.params,
          boundParams: paramOverrides,
        });
        if (cancelled || !canvasRef.current) {
          return;
        }
        drawLayout(canvasRef.current, layout, values);
        setRenderError("");

        const sampleCount = Math.min(layout.points.length, 64);
        const cpu = await sampleCompiledFrame({
          compilationId: activeCompile.compilationId,
          layout,
          phase,
          overrides: paramOverrides,
          limit: sampleCount,
        });
        if (cancelled) {
          return;
        }

        let maxDelta = 0;
        for (let i = 0; i < cpu.points.length; i++) {
          maxDelta = Math.max(maxDelta, Math.abs(cpu.points[i].value - (values[i] ?? 0)));
        }
        setParityStatus(`CPU/GPU max delta ${maxDelta.toFixed(5)} across ${cpu.points.length} samples`);
      } catch (error) {
        if (!cancelled) {
          setRenderError(formatError(error));
          setParityStatus("CPU/GPU parity unavailable");
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [compileResult?.compilationId, compileResult?.wgsl, selectedLayoutId, phase, previewMode, JSON.stringify(paramOverrides)]);

  async function loadFile(path: string, effects = workspace?.effects ?? []): Promise<void> {
    if (!path) {
      return;
    }
    if (selectedFilePath && isDirty) {
      await persistCurrentSource();
    }

    const content = await readSourceFile(path);
    setSelectedFilePath(path);
    setSource(content);
    setIsDirty(false);
    setCompileResult(null);
    setDiagnostics([]);
    const file = effects.find((item) => item.path === path);
    setStatusMessage(file ? `Editing ${file.relativePath}` : `Editing ${path}`);
  }

  async function openWorkspacePicker(): Promise<void> {
    const nextWorkspace = await openWorkspace();
    if (!nextWorkspace) {
      return;
    }
    setWorkspace(nextWorkspace);
    if (nextWorkspace.effects[0]) {
      await loadFile(nextWorkspace.effects[0].path, nextWorkspace.effects);
    } else {
      setSelectedFilePath("");
      setSource("");
      setCompileResult(null);
      setDiagnostics([]);
    }
  }

  async function persistCurrentSource(): Promise<void> {
    if (!selectedFilePath) {
      return;
    }
    await saveSourceFile(selectedFilePath, source);
    setIsDirty(false);
  }

  async function compileCurrent(): Promise<void> {
    if (!selectedFilePath || !workspace) {
      return;
    }

    setIsCompiling(true);
    setStatusMessage("Compiling effect…");
    try {
      await persistCurrentSource();
      const response = await compilePreview({
        workspaceRoot: workspace.root,
        filePath: selectedFilePath,
        overrides: paramOverrides,
      });
      startTransition(() => {
        setCompileResult(response);
        setDiagnostics(response.diagnostics ?? []);
        if (response.compilationId) {
          setParamOverrides(response.boundParams ?? {});
          if (!currentPreset(response.presets, selectedPresetName)) {
            setSelectedPresetName(response.presets[0]?.name ?? "");
          }
          setStatusMessage(`Compiled ${response.modulePath ?? response.filePath}`);
        } else {
          setStatusMessage("Compile failed");
        }
      });
    } catch (error) {
      setStatusMessage(formatError(error));
    } finally {
      setIsCompiling(false);
    }
  }

  async function importLayout(): Promise<void> {
    const path = await selectLayoutFile();
    if (!path) {
      return;
    }
    const imported = await loadLayout(path);
    setLayouts((items) => [...items, imported]);
    setSelectedLayoutId(imported.id);
  }

  function updateParam(param: ParamData, rawValue: string | boolean): void {
    const next = { ...paramOverrides };
    switch (param.type) {
      case "int":
        next[param.name] = Number.parseInt(String(rawValue), 10);
        break;
      case "float":
        next[param.name] = Number.parseFloat(String(rawValue));
        break;
      case "bool":
        next[param.name] = Boolean(rawValue);
        break;
      case "enum":
        next[param.name] = String(rawValue);
        break;
      default:
        next[param.name] = rawValue;
    }
    setParamOverrides(next);
  }

  const layout = selectedLayout(layouts, selectedLayoutId);
  const preset = currentPreset(compileResult?.presets ?? [], selectedPresetName);

  return (
    <div className="app-shell">
      <aside className="left-rail">
        <div className="brand-block">
          <p className="eyebrow">LFX Desktop Preview</p>
          <h1>Render scalar effects against real point layouts.</h1>
          <p className="lede">
            Workspace-aware editing, WGSL preview, CPU parity sampling, and preset-window playback in one desktop tool.
          </p>
        </div>

        <section className="panel">
          <div className="panel-head">
            <h2>Workspace</h2>
            <button type="button" onClick={() => void openWorkspacePicker()}>
              Open
            </button>
          </div>
          <p className="meta-path">{workspace?.root ?? "No workspace loaded"}</p>
          <div className="effect-list">
            {(workspace?.effects ?? []).map((effect) => (
              <button
                key={effect.path}
                type="button"
                className={`effect-item ${effect.path === selectedFilePath ? "is-active" : ""}`}
                onClick={() => void loadFile(effect.path)}
              >
                <span>{effect.name}</span>
                <small>{effect.relativePath}</small>
              </button>
            ))}
          </div>
        </section>

        <section className="panel">
          <div className="panel-head">
            <h2>Layouts</h2>
            <button type="button" onClick={() => void importLayout()}>
              Import
            </button>
          </div>
          <div className="layout-list">
            {layouts.map((item) => (
              <button
                key={item.id}
                type="button"
                className={`layout-item ${item.id === selectedLayoutId ? "is-active" : ""}`}
                onClick={() => setSelectedLayoutId(item.id)}
              >
                <span>{item.name}</span>
                <small>
                  {item.points.length} pts · {item.width}×{item.height}
                </small>
              </button>
            ))}
          </div>
        </section>

        <section className="panel">
          <h2>Status</h2>
          <dl className="status-grid">
            <div>
              <dt>GPU</dt>
              <dd>{gpuStatus}</dd>
            </div>
            <div>
              <dt>Parity</dt>
              <dd>{parityStatus}</dd>
            </div>
            <div>
              <dt>Compile</dt>
              <dd>{isCompiling ? "Compiling…" : statusMessage}</dd>
            </div>
          </dl>
        </section>
      </aside>

      <main className="workspace-shell">
        <header className="toolbar">
          <div>
            <p className="eyebrow">Effect Source</p>
            <h2>{selectedFilePath ? shortName(selectedFilePath) : "No effect selected"}</h2>
          </div>
          <div className="toolbar-actions">
            <label className="toggle">
              <input
                type="checkbox"
                checked={autoCompile}
                onChange={(event) => setAutoCompile(event.target.checked)}
              />
              <span>Auto-compile</span>
            </label>
            <button type="button" onClick={() => void persistCurrentSource()} disabled={!selectedFilePath}>
              Save
            </button>
            <button type="button" className="primary" onClick={() => void compileCurrent()} disabled={!selectedFilePath}>
              Compile
            </button>
          </div>
        </header>

        <div className="editor-preview-grid">
          <section className="editor-panel">
            <Editor
              theme="vs-dark"
              path={selectedFilePath}
              language="lua"
              value={source}
              onChange={(value) => {
                setSource(value ?? "");
                setIsDirty(true);
              }}
              options={{
                minimap: { enabled: false },
                fontSize: 14,
                smoothScrolling: true,
                wordWrap: "on",
                automaticLayout: true,
                padding: { top: 16, bottom: 16 },
              }}
            />
          </section>

          <section className="preview-panel">
            <div className="preview-header">
              <div>
                <p className="eyebrow">Live Preview</p>
                <h2>{compileResult?.modulePath ?? "Compile to preview"}</h2>
              </div>
              <div className="preview-meta">
                <span>{layout ? `${layout.points.length} points` : "No layout"}</span>
                <span>Phase {phase.toFixed(3)}</span>
              </div>
            </div>
            <canvas ref={canvasRef} className="preview-canvas" />
            {renderError ? <div className="blocking-message">{renderError}</div> : null}

            <div className="controls-grid">
              <div className="control-group">
                <label>Phase</label>
                <input
                  type="range"
                  min={0}
                  max={1}
                  step={0.001}
                  value={phase}
                  onChange={(event) => setPhase(Number(event.target.value))}
                />
              </div>
              <div className="control-group">
                <label>Speed</label>
                <input
                  type="range"
                  min={0.2}
                  max={4}
                  step={0.1}
                  value={speed}
                  onChange={(event) => setSpeed(Number(event.target.value))}
                />
              </div>
              <div className="control-group">
                <label>Render Mode</label>
                <select value={previewMode} onChange={(event) => setPreviewMode(event.target.value as "gpu" | "cpu")}>
                  <option value="gpu">GPU</option>
                  <option value="cpu">CPU</option>
                </select>
              </div>
              <div className="playback-row">
                <button type="button" className="primary" onClick={() => setPlaying((value) => !value)}>
                  {playing ? "Pause" : "Play"}
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setPlaying(false);
                    setPhase(preset?.start ?? 0);
                  }}
                >
                  Reset
                </button>
                <select
                  value={selectedPresetName}
                  onChange={(event) => {
                    const nextPreset = currentPreset(compileResult?.presets ?? [], event.target.value);
                    setSelectedPresetName(event.target.value);
                    setPhase(nextPreset?.start ?? 0);
                  }}
                >
                  <option value="">No preset window</option>
                  {(compileResult?.presets ?? []).map((item) => (
                    <option key={item.name} value={item.name}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            {preset ? (
              <div className="preset-strip">
                <span>start {preset.start.toFixed(2)}</span>
                <span>loop {preset.loopStart.toFixed(2)} → {preset.loopEnd.toFixed(2)}</span>
                <span>finish {preset.finish.toFixed(2)}</span>
                <span>speed {preset.speed.toFixed(0)}</span>
              </div>
            ) : null}
          </section>
        </div>

        <div className="inspector-grid">
          <section className="panel">
            <h2>Parameters</h2>
            <div className="param-list">
              {(compileResult?.params ?? []).map((param) => (
                <div className="param-row" key={param.name}>
                  <div>
                    <strong>{param.name}</strong>
                    <small>{param.type}</small>
                  </div>
                  {param.type === "bool" ? (
                    <input
                      type="checkbox"
                      checked={Boolean(paramOverrides[param.name])}
                      onChange={(event) => updateParam(param, event.target.checked)}
                    />
                  ) : param.type === "enum" ? (
                    <select
                      value={String(paramOverrides[param.name] ?? param.defaultValue ?? "")}
                      onChange={(event) => updateParam(param, event.target.value)}
                    >
                      {(param.enumValues ?? []).map((value) => (
                        <option key={value} value={value}>
                          {value}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      type="number"
                      value={String(paramOverrides[param.name] ?? param.defaultValue ?? 0)}
                      min={param.min}
                      max={param.max}
                      step={param.type === "int" ? 1 : 0.01}
                      onChange={(event) => updateParam(param, event.target.value)}
                    />
                  )}
                </div>
              ))}
            </div>
          </section>

          <section className="panel">
            <h2>Diagnostics</h2>
            <div className="diagnostic-list">
              {diagnostics.length === 0 ? (
                <p className="empty-state">No compiler diagnostics.</p>
              ) : (
                diagnostics.map((item, index) => (
                  <article className="diagnostic-item" key={`${item.code}-${index}`}>
                    <header>
                      <strong>{item.code ?? item.severity.toUpperCase()}</strong>
                      <span>
                        {item.line ? `L${item.line}:${item.column ?? 1}` : "workspace"}
                      </span>
                    </header>
                    <p>{item.message}</p>
                    {item.filePath ? <small>{item.filePath}</small> : null}
                  </article>
                ))
              )}
            </div>
          </section>

          <section className="panel shader-panel">
            <h2>Generated WGSL</h2>
            <pre>{compileResult?.wgsl ?? "Compile an effect to inspect its generated compute shader."}</pre>
          </section>
        </div>
      </main>
    </div>
  );
}

function currentPreset(presets: PresetData[], name: string): PresetData | undefined {
  return presets.find((item) => item.name === name);
}

function selectedLayout(layouts: LayoutData[], id: string): LayoutData | undefined {
  return layouts.find((item) => item.id === id);
}

function formatError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

function shortName(path: string): string {
  const parts = path.split("/");
  return parts[parts.length - 1] ?? path;
}

function advancePhase(current: number, delta: number, preset?: PresetData): number {
  if (!preset) {
    return (current + delta) % 1;
  }

  if (current < preset.start || current > preset.finish) {
    return preset.start;
  }

  const next = current + delta;
  if (next <= preset.loopEnd) {
    return next;
  }

  const loopSpan = Math.max(0.001, preset.loopEnd - preset.loopStart);
  return preset.loopStart + ((next - preset.loopStart) % loopSpan);
}

export default App
