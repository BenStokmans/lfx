(function () {
  const vscode = acquireVsCodeApi();

  const state = {
    mode: "preview",
    renderMode: "points",
    layouts: [],
    selectedLayoutId: "",
    artifact: null,
    lastGoodArtifact: null,
    params: {},
    phase: 0,
    speed: 1,
    playing: false,
    renderer: null,
    frameHandle: 0,
    lastFrameAt: 0,
    renderToken: 0,
  };

  const elements = {
    subtitle: document.getElementById("subtitle"),
    refreshButton: document.getElementById("refreshButton"),
    playButton: document.getElementById("playButton"),
    phaseInput: document.getElementById("phaseInput"),
    phaseValue: document.getElementById("phaseValue"),
    speedInput: document.getElementById("speedInput"),
    speedValue: document.getElementById("speedValue"),
    layoutSelect: document.getElementById("layoutSelect"),
    renderModeSelect: document.getElementById("renderModeSelect"),
    paramsPanel: document.getElementById("paramsPanel"),
    compilerSource: document.getElementById("compilerSource"),
    probeStatus: document.getElementById("probeStatus"),
    canvas: document.getElementById("canvas"),
    overlay: document.getElementById("overlay"),
    diagnosticsOutput: document.getElementById("diagnosticsOutput"),
  };

  bindUi();
  window.addEventListener("message", (event) => {
    void handleMessage(event.data);
  });
  window.addEventListener("resize", () => {
    if (!state.renderer) {
      return;
    }
    state.renderer.resize();
    void renderCurrentFrame();
  });
  vscode.postMessage({ type: "ready" });

  function bindUi() {
    elements.refreshButton.addEventListener("click", () => {
      vscode.postMessage({ type: "requestRefresh" });
    });
    elements.playButton.addEventListener("click", () => {
      state.playing = !state.playing;
      syncPlayback();
    });
    elements.phaseInput.addEventListener("input", () => {
      state.phase = Number(elements.phaseInput.value);
      syncPhaseUi();
      vscode.postMessage({ type: "setPhase", phase: state.phase });
      void renderCurrentFrame();
    });
    elements.speedInput.addEventListener("input", () => {
      state.speed = Number(elements.speedInput.value);
      syncSpeedUi();
      syncPlayback();
    });
    elements.layoutSelect.addEventListener("change", () => {
      state.selectedLayoutId = elements.layoutSelect.value;
      vscode.postMessage({ type: "setLayout", layoutId: state.selectedLayoutId });
      void renderCurrentFrame();
    });
    if (elements.renderModeSelect) {
      elements.renderModeSelect.addEventListener("change", () => {
        state.renderMode = elements.renderModeSelect.value;
        elements.layoutSelect.disabled = state.renderMode === "solid";
        void renderCurrentFrame();
      });
      elements.layoutSelect.disabled = state.renderMode === "solid";
    }
  }

  async function handleMessage(message) {
    if (!message || typeof message.type !== "string") {
      return;
    }

    switch (message.type) {
      case "init":
        state.mode = message.mode;
        state.layouts = Array.isArray(message.layouts) ? message.layouts : [];
        state.selectedLayoutId = message.preferredLayoutId || state.layouts[0]?.id || "";
        renderLayoutOptions();
        elements.subtitle.textContent = message.filePath || (state.mode === "probe" ? "Probe runtime capability" : "Waiting for source…");
        elements.compilerSource.textContent = `Compiler: ${message.compilerSource || "pending"}`;
        if (state.mode === "probe") {
          elements.refreshButton.hidden = true;
          elements.playButton.hidden = true;
        }
        await ensureProbe();
        break;
      case "compiledEffect":
        state.artifact = message.artifact;
        state.lastGoodArtifact = message.artifact;
        elements.subtitle.textContent = message.artifact.modulePath || message.artifact.filePath;
        elements.compilerSource.textContent = `Compiler: ${message.compilerSource || "pending"}`;
        hydrateParams(message.artifact);
        renderDiagnostics(message.artifact.diagnostics || []);
        hideOverlay();
        await renderCurrentFrame();
        break;
      case "compileError":
        state.artifact = message.artifact;
        elements.compilerSource.textContent = `Compiler: ${message.compilerSource || "pending"}`;
        renderDiagnostics(message.artifact.diagnostics || []);
        showOverlay(formatCompileError(message.artifact, message.reason));
        if (state.lastGoodArtifact) {
          await renderCurrentFrame();
        }
        break;
      case "dispose":
        if (state.frameHandle) {
          cancelAnimationFrame(state.frameHandle);
        }
        break;
    }
  }

  async function ensureProbe() {
    const details = {
      hasNavigatorGpu: false,
      adapterAcquired: false,
      canvasContextAvailable: false,
      error: "",
      adapterInfo: "",
      userAgent: navigator.userAgent,
    };

    try {
      details.hasNavigatorGpu = Boolean(navigator.gpu);
      if (!navigator.gpu) {
        throw new Error("navigator.gpu is unavailable in this VS Code runtime.");
      }
      if (!state.renderer) {
        state.renderer = new PreviewRenderer(elements.canvas);
      }
      await state.renderer.ensureSupport();
      details.adapterAcquired = Boolean(state.renderer.adapter);
      details.canvasContextAvailable = Boolean(state.renderer.context);
      details.adapterInfo = await state.renderer.describeAdapter();
      elements.probeStatus.textContent = "WebGPU available";
      hideOverlay();
      vscode.postMessage({ type: "probeResult", ok: true, details });
    } catch (error) {
      details.error = formatError(error);
      elements.probeStatus.textContent = `WebGPU unavailable: ${details.error}`;
      showOverlay(`WebGPU unsupported\n\n${details.error}`);
      vscode.postMessage({ type: "probeResult", ok: false, details });
      if (state.mode === "probe") {
        renderDiagnostics([{ severity: "error", message: details.error }]);
      }
    }
  }

  function renderLayoutOptions() {
    elements.layoutSelect.innerHTML = "";
    for (const layout of state.layouts) {
      const option = document.createElement("option");
      option.value = layout.id;
      option.textContent = layout.name;
      option.selected = layout.id === state.selectedLayoutId;
      elements.layoutSelect.appendChild(option);
    }
  }

  function hydrateParams(artifact) {
    state.params = {};
    for (const param of artifact.params || []) {
      state.params[param.name] = artifact.boundParams?.[param.name] ?? param.defaultValue;
    }
    renderParams(artifact.params || []);
  }

  function renderParams(params) {
    elements.paramsPanel.innerHTML = "";
    for (const param of params) {
      const label = document.createElement("label");
      const title = document.createElement("span");
      title.textContent = param.name;
      label.appendChild(title);

      if (param.type === "bool") {
        const input = document.createElement("input");
        input.type = "checkbox";
        input.checked = Boolean(state.params[param.name]);
        input.addEventListener("change", () => {
          state.params[param.name] = input.checked;
          syncParams();
        });
        label.appendChild(input);
      } else if (param.type === "enum") {
        const select = document.createElement("select");
        for (const optionValue of param.enumValues || []) {
          const option = document.createElement("option");
          option.value = optionValue;
          option.textContent = optionValue;
          option.selected = optionValue === state.params[param.name];
          select.appendChild(option);
        }
        select.addEventListener("change", () => {
          state.params[param.name] = select.value;
          syncParams();
        });
        label.appendChild(select);
      } else {
        const input = document.createElement("input");
        const output = document.createElement("output");
        input.type = "range";
        input.min = String(param.min ?? 0);
        input.max = String(param.max ?? 1);
        input.step = param.type === "int" ? "1" : "0.01";
        input.value = String(state.params[param.name] ?? 0);
        output.textContent = input.value;
        input.addEventListener("input", () => {
          state.params[param.name] = param.type === "int" ? Number.parseInt(input.value, 10) : Number(input.value);
          output.textContent = input.value;
          syncParams();
        });
        label.appendChild(input);
        label.appendChild(output);
      }

      elements.paramsPanel.appendChild(label);
    }
  }

  function syncParams() {
    vscode.postMessage({ type: "setParams", params: state.params });
    void renderCurrentFrame();
  }

  function syncPhaseUi() {
    elements.phaseValue.textContent = state.phase.toFixed(3);
    elements.phaseInput.value = String(state.phase);
  }

  function syncSpeedUi() {
    elements.speedValue.textContent = `${state.speed.toFixed(1)}x`;
    elements.speedInput.value = String(state.speed);
  }

  function syncPlayback() {
    elements.playButton.textContent = state.playing ? "Pause" : "Play";
    vscode.postMessage({ type: "setPlayback", playing: state.playing, speed: state.speed });
    if (!state.playing) {
      if (state.frameHandle) {
        cancelAnimationFrame(state.frameHandle);
        state.frameHandle = 0;
      }
      return;
    }

    state.lastFrameAt = performance.now();
    const tick = (now) => {
      const delta = (now - state.lastFrameAt) / 1000;
      state.lastFrameAt = now;
      state.phase = advancePhase(state.phase, delta * 0.15 * state.speed, state.artifact?.timeline);
      syncPhaseUi();
      void renderCurrentFrame();
      state.frameHandle = requestAnimationFrame(tick);
    };
    state.frameHandle = requestAnimationFrame(tick);
  }

  async function renderCurrentFrame() {
    const token = ++state.renderToken;
    syncPhaseUi();
    syncSpeedUi();
    if (state.mode === "probe" || !state.renderer || !state.lastGoodArtifact || !state.lastGoodArtifact.wgsl || !elements.canvas) {
      return;
    }
    
    const baseLayout = state.layouts.find((candidate) => candidate.id === state.selectedLayoutId) || state.layouts[0];
    if (!baseLayout) {
      return;
    }
    let layout = baseLayout;

    if (state.renderMode === "solid") {
      const extents = getLayoutExtents(baseLayout);
      const spanX = Math.max(extents.spanX || 1, 0.001);
      const spanY = Math.max(extents.spanY || 1, 0.001);

      const padding = 28;
      const usableWidth = Math.max(1, (elements.canvas.clientWidth || 800) - padding * 2);
      const usableHeight = Math.max(1, (elements.canvas.clientHeight || 600) - padding * 2);
      const fitScale = Math.min(usableWidth / spanX, usableHeight / spanY);
      
      let gridW = Math.max(1, Math.floor(spanX * fitScale));
      let gridH = Math.max(1, Math.floor(spanY * fitScale));

      // Limit resolution to avoid WebGPU out-of-memory or timeouts on huge windows
      const MAX_POINTS = 512 * 512;
      if (gridW * gridH > MAX_POINTS) {
        const scale = Math.sqrt(MAX_POINTS / (gridW * gridH));
        gridW = Math.max(1, Math.floor(gridW * scale));
        gridH = Math.max(1, Math.floor(gridH * scale));
      }
      
      if (state._fullResLayout && state._fullResLayout.baseLayoutId === baseLayout.id && state._fullResLayout.id === `full-res-${gridW}x${gridH}`) {
        layout = state._fullResLayout;
      } else {
        const points = new Array(gridW * gridH);
        for (let y = 0; y < gridH; y++) {
          for (let x = 0; x < gridW; x++) {
            points[y * gridW + x] = {
              index: y * gridW + x,
              x: extents.minX + spanX * (gridW > 1 ? x / (gridW - 1) : 0),
              y: extents.minY + spanY * (gridH > 1 ? y / (gridH - 1) : 0),
            };
          }
        }
        layout = {
          id: `full-res-${gridW}x${gridH}`,
          name: "Full Resolution",
          baseLayoutId: baseLayout.id,
          width: baseLayout.width,
          height: baseLayout.height,
          points: points
        };
        state._fullResLayout = layout;
      }
    }
    try {
      await state.renderer.render({
        wgsl: state.lastGoodArtifact.wgsl,
        outputType: state.lastGoodArtifact.outputType || "scalar",
        layout,
        renderMode: state.renderMode,
        phase: state.phase,
        params: state.lastGoodArtifact.params || [],
        overrides: state.params,
      });
      if (token !== state.renderToken) {
        return;
      }
      hideOverlay();
    } catch (error) {
      if (token !== state.renderToken) {
        return;
      }
      const message = formatError(error);
      showOverlay(message);
      vscode.postMessage({ type: "runtimeError", message });
    }
  }

  function renderDiagnostics(diagnostics) {
    if (!diagnostics || diagnostics.length === 0) {
      elements.diagnosticsOutput.textContent = "No diagnostics.";
      return;
    }
    elements.diagnosticsOutput.textContent = diagnostics
      .map((diagnostic) => {
        const code = diagnostic.code ? `[${diagnostic.code}] ` : "";
        const location = diagnostic.line ? ` (${diagnostic.line}:${diagnostic.column || 1})` : "";
        return `${diagnostic.severity.toUpperCase()} ${code}${diagnostic.message}${location}`;
      })
      .join("\n");
  }

  function formatCompileError(artifact, reason) {
    const lines = [];
    if (reason) {
      lines.push(reason);
      lines.push("");
    }
    lines.push("Preview kept the last valid render.");
    if (artifact?.diagnostics?.length) {
      lines.push("");
      for (const diagnostic of artifact.diagnostics) {
        lines.push(`- ${diagnostic.message}`);
      }
    }
    return lines.join("\n");
  }

  function showOverlay(message) {
    elements.overlay.textContent = message;
    elements.overlay.classList.remove("hidden");
  }

  function hideOverlay() {
    elements.overlay.textContent = "";
    elements.overlay.classList.add("hidden");
  }

  function advancePhase(current, delta, timeline) {
    let next = current + delta;
    if (timeline && Number.isFinite(timeline.loopStart) && Number.isFinite(timeline.loopEnd) && timeline.loopEnd > timeline.loopStart) {
      const span = timeline.loopEnd - timeline.loopStart;
      if (next > timeline.loopEnd) {
        next = timeline.loopStart + ((next - timeline.loopStart) % span);
      }
      return next;
    }
    if (next > 1) {
      next %= 1;
    }
    return next;
  }

  function formatError(error) {
    return error instanceof Error ? error.message : String(error);
  }

  class PreviewRenderer {
    constructor(canvas) {
      this.canvas = canvas;
      this.adapter = null;
      this.device = null;
      this.context = null;
      this.supportPromise = null;
      this.presentationFormat = null;
      this.computePipeline = null;
      this.renderPipeline = null;
      this.cachedWGSL = "";
      this.layoutId = "";
      this.pointCount = 0;
      this.outputType = "scalar";
      this.pipelineDevice = null;
      this.pointBuffer = null;
      this.valueBuffer = null;
      this.uniformBuffer = null;
      this.renderUniformBuffer = null;
      this.bindGroup = null;
      this.renderBindGroup = null;
    }

    async ensureSupport() {
      if (this.device && this.context && this.presentationFormat) {
        this.resize();
        return;
      }
      if (!this.supportPromise) {
        this.supportPromise = this.initializeSupport().finally(() => {
          this.supportPromise = null;
        });
      }
      await this.supportPromise;
      this.resize();
    }

    async initializeSupport() {
      if (!navigator.gpu) {
        throw new Error("This runtime does not expose navigator.gpu.");
      }
      if (!this.adapter) {
        this.adapter = await navigator.gpu.requestAdapter();
      }
      if (!this.adapter) {
        throw new Error("requestAdapter() returned null.");
      }
      if (!this.device) {
        const adapter = this.adapter;
        const device = await adapter.requestDevice();
        this.device = device;
        device.lost.then(() => {
          if (this.device !== device) {
            return;
          }
          this.supportPromise = null;
          this.adapter = null;
          this.device = null;
          this.context = null;
          this.presentationFormat = null;
          this.pipelineDevice = null;
          this.resetGpuResources();
        });
      }
      if (!this.context) {
        this.context = this.canvas.getContext("webgpu");
      }
      if (!this.context) {
        throw new Error("canvas.getContext('webgpu') failed.");
      }
      if (!this.presentationFormat) {
        this.presentationFormat = navigator.gpu.getPreferredCanvasFormat();
      }
    }

    async describeAdapter() {
      if (!this.adapter) {
        return "";
      }
      const info = await this.adapter.requestAdapterInfo?.();
      if (!info) {
        return "";
      }
      return [info.vendor, info.architecture, info.description].filter(Boolean).join(" / ");
    }

    resize() {
      if (!this.context || !this.device || !this.presentationFormat) {
        return;
      }
      const width = Math.max(1, Math.floor(this.canvas.clientWidth * (window.devicePixelRatio || 1)));
      const height = Math.max(1, Math.floor(this.canvas.clientHeight * (window.devicePixelRatio || 1)));
      this.canvas.width = width;
      this.canvas.height = height;
      this.context.configure({
        device: this.device,
        format: this.presentationFormat,
        alphaMode: "premultiplied",
      });
    }

    async render(request) {
      await this.ensureSupport();
      const device = this.device;
      const context = this.context;
      if (!device || !context) {
        throw new Error("WebGPU device is not ready.");
      }

      await this.ensurePipelines(request.wgsl, device);
      if (device !== this.device || context !== this.context) {
        throw new Error("Preview render invalidated by a GPU device reset.");
      }
      this.ensureLayoutBuffers(request.layout, request.outputType);
      this.writeUniforms(request);

      const encoder = device.createCommandEncoder();
      const computePass = encoder.beginComputePass();
      computePass.setPipeline(this.computePipeline);
      computePass.setBindGroup(0, this.bindGroup);
      computePass.dispatchWorkgroups(Math.max(1, Math.ceil(this.pointCount / 64)));
      computePass.end();

      const renderPass = encoder.beginRenderPass({
        colorAttachments: [
          {
            view: context.getCurrentTexture().createView(),
            clearValue: { r: 0, g: 0, b: 0, a: 0 },
            loadOp: "clear",
            storeOp: "store",
          },
        ],
      });
      renderPass.setPipeline(this.renderPipeline);
      renderPass.setBindGroup(0, this.renderBindGroup);
      renderPass.draw(6, this.pointCount, 0, 0);
      renderPass.end();

      device.queue.submit([encoder.finish()]);
    }

    async ensurePipelines(wgsl, device) {
      if (this.pipelineDevice !== this.device) {
        this.resetGpuResources();
        this.pipelineDevice = this.device;
      }
      if (this.cachedWGSL === wgsl && this.computePipeline && this.renderPipeline) {
        return;
      }
      const effectModule = device.createShaderModule({ code: wgsl });
      const effectInfo = await effectModule.getCompilationInfo?.();
      if (device !== this.device) {
        throw new Error("Preview pipeline invalidated by a GPU device reset.");
      }
      const effectError = effectInfo?.messages?.find((message) => message.type === "error");
      if (effectError) {
        throw new Error(effectError.message || "WGSL compilation failed.");
      }
      this.computePipeline = await device.createComputePipelineAsync({
        layout: "auto",
        compute: { module: effectModule, entryPoint: "main" },
      });
      if (device !== this.device) {
        throw new Error("Preview pipeline invalidated by a GPU device reset.");
      }

      const renderModule = device.createShaderModule({ code: renderShaderWGSL });
      const renderInfo = await renderModule.getCompilationInfo?.();
      if (device !== this.device) {
        throw new Error("Preview pipeline invalidated by a GPU device reset.");
      }
      const renderError = renderInfo?.messages?.find((message) => message.type === "error");
      if (renderError) {
        throw new Error(renderError.message || "Preview render shader failed.");
      }
      this.renderPipeline = await device.createRenderPipelineAsync({
        layout: "auto",
        vertex: { module: renderModule, entryPoint: "vs_main" },
        fragment: {
          module: renderModule,
          entryPoint: "fs_main",
          targets: [{
            format: this.presentationFormat,
            blend: {
              color: { srcFactor: "src-alpha", dstFactor: "one-minus-src-alpha" },
              alpha: { srcFactor: "one", dstFactor: "one-minus-src-alpha" }
            }
          }],
        },
        primitive: { topology: "triangle-list" },
      });
      if (device !== this.device) {
        throw new Error("Preview pipeline invalidated by a GPU device reset.");
      }
      this.cachedWGSL = wgsl;
    }

    ensureLayoutBuffers(layout, outputType) {
      if (this.pipelineDevice !== this.device) {
        this.resetGpuResources();
        this.pipelineDevice = this.device;
      }
      const layoutChanged = this.layoutId !== layout.id || this.outputType !== outputType;
      if (!layoutChanged && this.bindGroup && this.renderBindGroup) {
        return;
      }

      const channels = outputChannels(outputType);
      this.layoutId = layout.id;
      this.outputType = outputType;
      this.pointCount = layout.points.length;

      const pointsBytes = createPointsBytes(layout);
      this.pointBuffer = this.device.createBuffer({
        size: pointsBytes.byteLength,
        usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_DST,
      });
      this.device.queue.writeBuffer(this.pointBuffer, 0, pointsBytes);

      this.uniformBuffer = this.device.createBuffer({
        size: 256,
        usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
      });
      this.valueBuffer = this.device.createBuffer({
        size: Math.max(16, layout.points.length * channels * 4),
        usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_DST,
      });
      this.renderUniformBuffer = this.device.createBuffer({
        size: 64, // 16 floats * 4 bytes
        usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
      });

      this.bindGroup = this.device.createBindGroup({
        layout: this.computePipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: { buffer: this.pointBuffer } },
          { binding: 1, resource: { buffer: this.uniformBuffer } },
          { binding: 2, resource: { buffer: this.valueBuffer } },
        ],
      });

      this.renderBindGroup = this.device.createBindGroup({
        layout: this.renderPipeline.getBindGroupLayout(0),
        entries: [
          { binding: 0, resource: { buffer: this.pointBuffer } },
          { binding: 1, resource: { buffer: this.renderUniformBuffer } },
          { binding: 2, resource: { buffer: this.valueBuffer } },
        ],
      });
    }

    writeUniforms(request) {
      const numericParams = (request.params || []).filter((param) => param.type !== "enum");
      const uniformFloats = new Float32Array(64);
      uniformFloats[0] = request.layout.width;
      uniformFloats[1] = request.layout.height;
      uniformFloats[2] = request.phase;
      uniformFloats[3] = request.layout.points.length;
      for (let index = 0; index < numericParams.length; index += 1) {
        uniformFloats[4 + index] = coerceUniformValue(request.overrides[numericParams[index].name]);
      }
      this.device.queue.writeBuffer(this.uniformBuffer, 0, uniformFloats);

      const extents = getLayoutExtents(request.layout);
      const padding = 28;
      const usableWidth = Math.max(1, this.canvas.width - padding * 2);
      const usableHeight = Math.max(1, this.canvas.height - padding * 2);
      const spanX = Math.max(extents.spanX, 1);
      const spanY = Math.max(extents.spanY, 1);
      const fitScale = Math.min(usableWidth / spanX, usableHeight / spanY);
      
      // Estimate grid density to size the pixels
      const gridW = Math.max(1, Math.sqrt(request.layout.points.length * (spanX / spanY)));
      const logicalDistance = Math.max(0.1, spanX / gridW);
      const pixelDistance = logicalDistance * fitScale;
      let radius = Math.max(1.5, pixelDistance * 0.45); // 90% of the spacing

      if (request.renderMode === "solid") {
        radius = Math.max(0.5, pixelDistance * 0.52); // Overlap slightly to prevent seams
      }

      const renderFloats = new Float32Array([
        this.canvas.width,
        this.canvas.height,
        extents.minX,
        extents.minY,
        spanX,
        spanY,
        radius,
        fitScale,
        0,
        0,
        outputChannels(request.outputType),
        request.layout.points.length,
        request.renderMode === "solid" ? 1 : 0,
        0,
        0,
        0,
      ]);
      this.device.queue.writeBuffer(this.renderUniformBuffer, 0, renderFloats);
    }

    resetGpuResources() {
      this.computePipeline = null;
      this.renderPipeline = null;
      this.cachedWGSL = "";
      this.layoutId = "";
      this.pointCount = 0;
      this.outputType = "scalar";
      this.pointBuffer = null;
      this.valueBuffer = null;
      this.uniformBuffer = null;
      this.renderUniformBuffer = null;
      this.bindGroup = null;
      this.renderBindGroup = null;
    }
  }

  function coerceUniformValue(value) {
    if (typeof value === "number") {
      return value;
    }
    if (typeof value === "boolean") {
      return value ? 1 : 0;
    }
    return 0;
  }

  function outputChannels(outputType) {
    switch (outputType) {
      case "rgb":
        return 3;
      case "rgbw":
        return 4;
      default:
        return 1;
    }
  }

  function createPointsBytes(layout) {
    const buffer = new ArrayBuffer(layout.points.length * 16);
    const view = new DataView(buffer);
    for (let index = 0; index < layout.points.length; index += 1) {
      const point = layout.points[index];
      const offset = index * 16;
      view.setUint32(offset, point.index, true);
      view.setFloat32(offset + 4, point.x, true);
      view.setFloat32(offset + 8, point.y, true);
      view.setFloat32(offset + 12, 0, true);
    }
    return new Uint8Array(buffer);
  }

  function getLayoutExtents(layout) {
    let minX = Number.POSITIVE_INFINITY;
    let maxX = Number.NEGATIVE_INFINITY;
    let minY = Number.POSITIVE_INFINITY;
    let maxY = Number.NEGATIVE_INFINITY;
    for (const point of layout.points) {
      minX = Math.min(minX, point.x);
      maxX = Math.max(maxX, point.x);
      minY = Math.min(minY, point.y);
      maxY = Math.max(maxY, point.y);
    }
    return {
      minX,
      minY,
      spanX: maxX - minX,
      spanY: maxY - minY,
    };
  }

  const renderShaderWGSL = `
struct Point {
  index: u32,
  x: f32,
  y: f32,
  pad: f32,
}

struct RenderUniforms {
  canvasWidth: f32,
  canvasHeight: f32,
  minX: f32,
  minY: f32,
  spanX: f32,
  spanY: f32,
  radius: f32,
  fitScale: f32,
  offsetX: f32,
  offsetY: f32,
  channels: f32,
  pointCount: f32,
  renderMode: f32,
  pad: f32,
  pad2: f32,
  pad3: f32,
}

@group(0) @binding(0) var<storage, read> points: array<Point>;
@group(0) @binding(1) var<uniform> uniforms: RenderUniforms;
@group(0) @binding(2) var<storage, read> values: array<f32>;

struct VertexOut {
  @builtin(position) position: vec4<f32>,
  @location(0) local: vec2<f32>,
  @interpolate(flat)
  @location(1) pointIndex: u32,
}

@vertex
fn vs_main(@builtin(vertex_index) vertexIndex: u32, @builtin(instance_index) instanceIndex: u32) -> VertexOut {
  let corners = array<vec2<f32>, 6>(
    vec2<f32>(-1.0, -1.0),
    vec2<f32>( 1.0, -1.0),
    vec2<f32>( 1.0,  1.0),
    vec2<f32>(-1.0, -1.0),
    vec2<f32>( 1.0,  1.0),
    vec2<f32>(-1.0,  1.0)
  );
  let point = points[instanceIndex];
  let pixel_centered = vec2<f32>(
    ((point.x - uniforms.minX) - uniforms.spanX * 0.5) * uniforms.fitScale + uniforms.offsetX,
    ((point.y - uniforms.minY) - uniforms.spanY * 0.5) * uniforms.fitScale + uniforms.offsetY
  );
  let ndc_centered = vec2<f32>(
    pixel_centered.x / (uniforms.canvasWidth * 0.5),
    pixel_centered.y / (uniforms.canvasHeight * 0.5)
  );
  let local = corners[vertexIndex];
  let radius = vec2<f32>(
    (uniforms.radius / uniforms.canvasWidth) * 2.0,
    (uniforms.radius / uniforms.canvasHeight) * 2.0
  );
  var out: VertexOut;
  out.position = vec4<f32>(ndc_centered + local * radius, 0.0, 1.0);
  out.local = local;
  out.pointIndex = instanceIndex;
  return out;
}

@fragment
fn fs_main(input: VertexOut) -> @location(0) vec4<f32> {
  let dist = length(input.local);
  
  if (uniforms.renderMode == 0.0) {
    if (dist > 1.0) {
      discard;
    }
  } else {
    // Solid rectangle block mapping (square bounds)
    if (abs(input.local.x) > 1.0 || abs(input.local.y) > 1.0) {
      discard;
    }
  }

  let channels = u32(uniforms.channels);
  let offset = input.pointIndex * channels;
  var color = vec3<f32>(0.0, 0.0, 0.0);
  if (channels == 1u) {
    let scalar = clamp(values[offset], 0.0, 1.0);
    color = vec3<f32>(scalar, scalar, scalar);
  } else if (channels == 3u) {
    color = vec3<f32>(
      clamp(values[offset], 0.0, 1.0),
      clamp(values[offset + 1u], 0.0, 1.0),
      clamp(values[offset + 2u], 0.0, 1.0)
    );
  } else {
    let white = clamp(values[offset + 3u], 0.0, 1.0);
    color = min(
      vec3<f32>(1.0, 1.0, 1.0),
      vec3<f32>(
        clamp(values[offset], 0.0, 1.0),
        clamp(values[offset + 1u], 0.0, 1.0),
        clamp(values[offset + 2u], 0.0, 1.0)
      ) + vec3<f32>(white, white, white)
    );
  }

  if (uniforms.renderMode == 0.0) {
    // Solid circular dot without soft edge
    return vec4<f32>(color, 1.0);
  } else {
    // Solid full rectangle component
    return vec4<f32>(color, 1.0);
  }
}
`;
})();
