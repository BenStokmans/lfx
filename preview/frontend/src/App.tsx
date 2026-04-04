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
  TimelineData,
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
  const [phase, setPhase] = useState(0);
  const [speed, setSpeed] = useState(1);
  const [playing, setPlaying] = useState(false);
  const [previewMode, setPreviewMode] = useState<"gpu" | "cpu">("gpu");
  const [renderError, setRenderError] = useState("");
  const [gpuStatus, setGpuStatus] = useState("Checking WebGPU support…");
  const [parityStatus, setParityStatus] = useState("CPU/GPU parity pending");
  const [perfStats, setPerfStats] = useState<{cpu?: number; gpu?: number; faster?: string; maxDelta?: number; error?: string} | null>(null);
  const [isComparingPerformance, setIsComparingPerformance] = useState(false);
  const [statusMessage, setStatusMessage] = useState("Loading workspace…");
  const [activeTab, setActiveTab] = useState<"diagnostics" | "wgsl">("diagnostics");

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
    const timeline = compileResult?.timeline ?? null;

    const step = (now: number) => {
      const deltaSeconds = (now - last) / 1000;
      last = now;
      setPhase((value) => advancePhase(value, deltaSeconds * 0.15 * speed, timeline));
      frame = requestAnimationFrame(step);
    };

    frame = requestAnimationFrame(step);
    return () => cancelAnimationFrame(frame);
  }, [playing, speed, compileResult]);

  useEffect(() => {
    const layout = selectedLayout(layouts, selectedLayoutId);
    if (!compileResult?.compilationId || !layout || !canvasRef.current) {
      return;
    }
    const activeCompile = compileResult as CompileResponse & {
      compilationId: string;
      wgsl?: string;
    };
    const boundParams = buildBoundParams(activeCompile.params, paramOverrides);

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
          drawLayout(
            canvasRef.current,
            layout,
            Float32Array.from(cpu.points.flatMap((point) => point.values)),
            activeCompile.outputType ?? "scalar",
          );
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
          outputType: activeCompile.outputType ?? "scalar",
          params: activeCompile.params,
          boundParams,
        });
        if (cancelled || !canvasRef.current) {
          return;
        }
        drawLayout(canvasRef.current, layout, values, activeCompile.outputType ?? "scalar");
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
        const channels = activeCompile.outputType === "rgb" ? 3 : activeCompile.outputType === "rgbw" ? 4 : 1;
        for (let i = 0; i < cpu.points.length; i++) {
          for (let channel = 0; channel < channels; channel++) {
            maxDelta = Math.max(
              maxDelta,
              Math.abs((cpu.points[i].values[channel] ?? 0) - (values[i * channels + channel] ?? 0)),
            );
          }
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

  async function comparePerformance(): Promise<void> {
    const layout = selectedLayout(layouts, selectedLayoutId);
    const activeCompile = compileResult as (CompileResponse & {
      compilationId: string;
      wgsl?: string;
    }) | null;
    if (!layout || !activeCompile?.compilationId || !activeCompile.wgsl) {
      return;
    }

    const boundParams = buildBoundParams(activeCompile.params, paramOverrides);
    setIsComparingPerformance(true);
    setPerfStats(null);

    try {
      await rendererRef.current.ensureSupport();

      // Warm both paths once to avoid comparing cold-start setup costs.
      await sampleCompiledFrame({
        compilationId: activeCompile.compilationId,
        layout,
        phase,
        overrides: paramOverrides,
        limit: layout.points.length,
      });
      await rendererRef.current.render({
        wgsl: activeCompile.wgsl,
        layout,
        phase,
        outputType: activeCompile.outputType ?? "scalar",
        params: activeCompile.params,
        boundParams,
      });

      const rounds = 3
      let cpuTotal = 0
      let gpuTotal = 0
      let maxDelta = 0
      const channels = activeCompile.outputType === "rgb" ? 3 : activeCompile.outputType === "rgbw" ? 4 : 1

      for (let round = 0; round < rounds; round++) {
        const cpuStart = performance.now()
        const cpu = await sampleCompiledFrame({
          compilationId: activeCompile.compilationId,
          layout,
          phase,
          overrides: paramOverrides,
          limit: layout.points.length,
        })
        cpuTotal += performance.now() - cpuStart

        const gpuStart = performance.now()
        const gpu = await rendererRef.current.render({
          wgsl: activeCompile.wgsl,
          layout,
          phase,
          outputType: activeCompile.outputType ?? "scalar",
          params: activeCompile.params,
          boundParams,
        })
        gpuTotal += performance.now() - gpuStart

        for (let i = 0; i < cpu.points.length; i++) {
          for (let channel = 0; channel < channels; channel++) {
            maxDelta = Math.max(
              maxDelta,
              Math.abs((cpu.points[i].values[channel] ?? 0) - (gpu[i * channels + channel] ?? 0)),
            )
          }
        }
      }

      const cpuAvg = cpuTotal / rounds
      const gpuAvg = gpuTotal / rounds
      const faster =
        gpuAvg > 0
          ? cpuAvg > gpuAvg
            ? `GPU ${(cpuAvg / gpuAvg).toFixed(2)}x`
            : `CPU ${(gpuAvg / cpuAvg).toFixed(2)}x`
          : "N/A"

      setPerfStats({ cpu: cpuAvg, gpu: gpuAvg, faster: faster, maxDelta: maxDelta })
    } catch (error) {
      setPerfStats({ error: formatError(error) })
    } finally {
      setIsComparingPerformance(false)
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

  function updateParam(param: ParamData, rawValue: string | boolean | number): void {
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
  const timeline = compileResult?.timeline ?? null;

  return (
    <div className="ide-shell">
      {/* Header */}
      <header className="ide-header">
        <div className="ide-brand">
          <div className="ide-logo">LFX</div>
          <span>Desktop Preview</span>
        </div>
        <div className="ide-title">
          {selectedFilePath ? shortName(selectedFilePath) : "No effect selected"}
        </div>
        <div className="ide-actions">
          <label className="ide-toggle">
            <input
              type="checkbox"
              checked={autoCompile}
              onChange={(event) => setAutoCompile(event.target.checked)}
            />
            <span className="toggle-slider"></span>
            <span className="toggle-label">Auto-compile</span>
          </label>
          <button
            type="button"
            className="ide-button"
            onClick={() => void persistCurrentSource()}
            disabled={!selectedFilePath}
          >
            Save
          </button>
          <button
            type="button"
            className="ide-button primary"
            onClick={() => void compileCurrent()}
            disabled={!selectedFilePath}
          >
            Compile
          </button>
        </div>
      </header>

      {/* Body Layout */}
      <div className="ide-body">
        {/* Left Sidebar */}
        <aside className="ide-sidebar-left">
          <div className="ide-panel">
            <div className="ide-panel-header">
              <h3>WORKSPACE</h3>
              <button
                type="button"
                className="ide-icon-button"
                onClick={() => void openWorkspacePicker()}
              >
                Open
              </button>
            </div>
            <p className="ide-meta-text">{workspace?.root ?? "No workspace loaded"}</p>
            <div className="ide-list">
              {(workspace?.effects ?? []).map((effect) => (
                <button
                  key={effect.path}
                  type="button"
                  className={`ide-list-item ${effect.path === selectedFilePath ? "active" : ""}`}
                  onClick={() => void loadFile(effect.path)}
                >
                  <span className="title">{effect.name}</span>
                  <span className="subtitle">{effect.relativePath}</span>
                </button>
              ))}
            </div>
          </div>

          <div className="ide-panel">
            <div className="ide-panel-header">
              <h3>LAYOUTS</h3>
              <button
                type="button"
                className="ide-icon-button"
                onClick={() => void importLayout()}
              >
                Import
              </button>
            </div>
            <div className="ide-list">
              {layouts.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  className={`ide-list-item ${item.id === selectedLayoutId ? "active" : ""}`}
                  onClick={() => setSelectedLayoutId(item.id)}
                >
                  <span className="title">{item.name}</span>
                  <span className="subtitle">
                    {item.points.length} pts · {item.width}×{item.height}
                  </span>
                </button>
              ))}
            </div>
          </div>
        </aside>

        {/* Center Main Area */}
        <main className="ide-main">
          <div className="ide-editor-container">
            <Editor
              height="100%"
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
                scrollBeyondLastLine: false,
              }}
            />
          </div>

          {/* Bottom Terminal Tab */}
          <div className="ide-bottom-panel">
            <div className="ide-tabs">
              <button
                type="button"
                className={`ide-tab ${activeTab === "diagnostics" ? "active" : ""}`}
                onClick={() => setActiveTab("diagnostics")}
              >
                Diagnostics ({diagnostics.length})
              </button>
              <button
                type="button"
                className={`ide-tab ${activeTab === "wgsl" ? "active" : ""}`}
                onClick={() => setActiveTab("wgsl")}
              >
                Generated WGSL
              </button>
            </div>
            <div className="ide-tab-content">
              {activeTab === "diagnostics" && (
                <div className="ide-diagnostic-list">
                  {diagnostics.length === 0 ? (
                    <p style={{ margin: 0 }}>No compiler diagnostics.</p>
                  ) : (
                    diagnostics.map((item, index) => (
                      <div
                        className={`ide-diagnostic-item ${item.severity === "error" ? "error" : "warning"}`}
                        key={`${item.code}-${index}`}
                      >
                        <div className="ide-diagnostic-header">
                          <strong>{item.code ?? item.severity?.toUpperCase()}</strong>
                          <span>
                            {item.line ? `L${item.line}:${item.column ?? 1}` : "workspace"}
                          </span>
                        </div>
                        <p className="ide-diagnostic-message">{item.message}</p>
                        {item.filePath ? <small>{shortName(item.filePath)}</small> : null}
                      </div>
                    ))
                  )}
                </div>
              )}

              {activeTab === "wgsl" && (
                <pre>
                  {compileResult?.wgsl ?? "Compile an effect to inspect its generated compute shader."}
                </pre>
              )}
            </div>
          </div>
        </main>

        {/* Parameters Sidebar */}
        <aside className="ide-sidebar-params">
          <div className="ide-parameters">
            <h3>PARAMETERS</h3>
            <div className="ide-param-list">
              {(compileResult?.params ?? []).map((param) => (
                <div className="ide-param-row" key={param.name}>
                  <div className="ide-param-header">
                    <span className="ide-param-name">{param.name}</span>
                    <span className="ide-param-type">{param.type}</span>
                  </div>
                  
                  {param.type === "bool" ? (
                    <label className="ide-toggle">
                      <input
                        type="checkbox"
                        checked={readBooleanParamValue(param, paramOverrides)}
                        onChange={(event) => updateParam(param, event.target.checked)}
                      />
                      <span className="toggle-slider"></span>
                      <span className="toggle-label">
                        {readBooleanParamValue(param, paramOverrides) ? "Enabled" : "Disabled"}
                      </span>
                    </label>
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
                  ) : hasNumericBounds(param) ? (
                    <div className="ide-param-control stack">
                      <input
                        type="range"
                        min={param.min}
                        max={param.max}
                        step={paramStep(param)}
                        value={readNumericParamValue(param, paramOverrides)}
                        onChange={(event) => updateParam(param, event.target.value)}
                      />
                      <div className="ide-param-control">
                        <input
                          type="number"
                          value={String(readNumericParamValue(param, paramOverrides))}
                          min={param.min}
                          max={param.max}
                          step={paramStep(param)}
                          onChange={(event) => updateParam(param, event.target.value)}
                        />
                        <span>Max {formatParamNumber(param.max)}</span>
                      </div>
                    </div>
                  ) : (
                    <div className="ide-param-control">
                      <input
                        type="number"
                        value={String(readNumericParamValue(param, paramOverrides))}
                        min={param.min}
                        max={param.max}
                        step={paramStep(param)}
                        onChange={(event) => updateParam(param, event.target.value)}
                      />
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        </aside>

        {/* Right Sidebar (Preview) */}
        <aside className="ide-sidebar-right">
          <div className="ide-preview-container">
            <div className="ide-preview-header">
              <span>{layout ? `${layout.points.length} points` : "No layout"}</span>
              <span>Phase {phase.toFixed(3)}</span>
            </div>
            <canvas ref={canvasRef} className="ide-canvas" />
            {renderError ? <div className="ide-error-banner">{renderError}</div> : null}
          </div>

          <div className="ide-transport">
            <div className="ide-transport-controls">
              <button
                type="button"
                className={`ide-play-button ${playing ? "active" : ""}`}
                onClick={() => setPlaying((value) => !value)}
                title={playing ? "Pause" : "Play"}
              >
                {playing ? "⏸" : "▶"}
              </button>
              <button
                type="button"
                className="ide-stop-button"
                onClick={() => {
                  setPlaying(false);
                  setPhase(0);
                }}
                title="Reset"
              >
                ⏹
              </button>
              <div className="ide-transport-slider">
                <span>Phase</span>
                <input
                  type="range"
                  min={0}
                  max={1}
                  step={0.001}
                  value={phase}
                  onChange={(event) => setPhase(Number(event.target.value))}
                />
              </div>
            </div>
            <div className="ide-settings-row">
              <div className="ide-setting-group">
                <label>Speed (x{speed})</label>
                <input
                  type="range"
                  min={0.2}
                  max={4}
                  step={0.1}
                  value={speed}
                  onChange={(event) => setSpeed(Number(event.target.value))}
                />
              </div>
              <div className="ide-setting-group">
                <label>Mode</label>
                <select
                  value={previewMode}
                  onChange={(event) => setPreviewMode(event.target.value as "gpu" | "cpu")}
                >
                  <option value="gpu">GPU Shader Component</option>
                  <option value="cpu">CPU Parity Model</option>
                </select>
              </div>
            </div>
            <button
              type="button"
              className="ide-benchmark-btn"
              onClick={() => void comparePerformance()}
              disabled={!compileResult?.compilationId || !layout || isComparingPerformance}
            >
              {isComparingPerformance ? "Benchmarking…" : "Benchmark CPU vs GPU"}
            </button>
            
            {perfStats && (
              <div className="ide-perf-stats">
                {perfStats.error ? (
                  <div className="ide-perf-box error">
                    <small>Benchmark Error</small>
                    <strong>{perfStats.error}</strong>
                  </div>
                ) : (
                  <>
                    <div className="ide-perf-box">
                      <small>CPU Avg</small>
                      <strong>{perfStats.cpu?.toFixed(2)} ms</strong>
                    </div>
                    <div className="ide-perf-box">
                      <small>GPU Avg</small>
                      <strong>{perfStats.gpu?.toFixed(2)} ms</strong>
                    </div>
                    <div className="ide-perf-box wide">
                      <div style={{display: 'flex', flexDirection: 'column'}}>
                        <small>Advantage</small>
                        <strong>{perfStats.faster}</strong>
                      </div>
                      <div style={{display: 'flex', flexDirection: 'column', alignItems: 'flex-end'}}>
                        <small>Max Delta</small>
                        <strong>{perfStats.maxDelta?.toFixed(5)}</strong>
                      </div>
                    </div>
                  </>
                )}
              </div>
            )}
          </div>

          <div className="ide-status-badges">
            <div className="status-badge" title={gpuStatus}>
              GPU: {gpuStatus === "WebGPU ready" ? "Ready" : "Error"}
            </div>
            <div className="status-badge" title={parityStatus}>
              Parity: {parityStatus.includes("pending") ? "Pending" : "Evaluated"}
            </div>
            <div className="status-badge wide">
              {isCompiling ? "Compiling…" : statusMessage}
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}

function buildBoundParams(params: ParamData[], overrides: Record<string, unknown>): Record<string, unknown> {
  const bound: Record<string, unknown> = {}
  for (const param of params) {
    bound[param.name] = overrides[param.name] ?? param.defaultValue
  }
  return bound
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

function advancePhase(current: number, delta: number, timeline: TimelineData | null): number {
  const loopStart = timeline?.loopStart ?? 0;
  const loopEnd = timeline?.loopEnd ?? 1;
  const loopSpan = Math.max(0.001, loopEnd - loopStart);

  const next = current + delta;
  if (next <= loopEnd) {
    return next;
  }
  return loopStart + ((next - loopStart) % loopSpan);
}

function hasNumericBounds(param: ParamData): param is ParamData & { min: number; max: number } {
  return (param.type === "int" || param.type === "float") && typeof param.min === "number" && typeof param.max === "number";
}

function readNumericParamValue(param: ParamData, overrides: Record<string, unknown>): number {
  const candidate = overrides[param.name] ?? param.defaultValue ?? 0;
  const value = typeof candidate === "number" ? candidate : Number(candidate);
  if (!Number.isFinite(value)) {
    return 0;
  }
  return param.type === "int" ? Math.round(value) : value;
}

function readBooleanParamValue(param: ParamData, overrides: Record<string, unknown>): boolean {
  return Boolean(overrides[param.name] ?? param.defaultValue ?? false);
}

function paramStep(param: ParamData): number {
  if (param.type === "int") {
    return 1;
  }
  if (hasNumericBounds(param)) {
    const span = Math.abs(param.max - param.min);
    if (span <= 1) {
      return 0.01;
    }
    if (span <= 10) {
      return 0.05;
    }
    return 0.1;
  }
  return 0.01;
}

function formatParamNumber(value: number | undefined): string {
  if (typeof value !== "number") {
    return "";
  }
  if (Number.isInteger(value)) {
    return String(value);
  }
  return value.toFixed(2).replace(/\.?0+$/, "");
}

export default App
