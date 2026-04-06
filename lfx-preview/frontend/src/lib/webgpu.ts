import type { LayoutData, ParamData } from "../types";

type RenderRequest = {
  wgsl: string;
  layout: LayoutData;
  phase: number;
  outputType: "scalar" | "rgb" | "rgbw";
  params: ParamData[];
  boundParams: Record<string, unknown>;
};

export class EffectRenderer {
  private device: any | null = null;
  private adapter: any | null = null;
  private cachedWGSL: string | null = null;
  private cachedShaderModule: any | null = null;
  private cachedPipeline: any | null = null;

  async ensureSupport(): Promise<void> {
    const gpu = (navigator as any).gpu;
    if (!gpu) {
      throw new Error("WebGPU is not available in this runtime.");
    }
    if (!this.adapter) {
      this.adapter = await gpu.requestAdapter();
    }
    if (!this.adapter) {
      throw new Error("No compatible GPU adapter was found.");
    }
    if (!this.device) {
      this.device = await this.adapter.requestDevice();
    }
  }

  async render(request: RenderRequest): Promise<Float32Array> {
    await this.ensureSupport();
    const device = this.device;
    if (!device) {
      throw new Error("GPU device was not initialised.");
    }

    const pipeline = await this.getPipeline(request.wgsl);

    const pointsBytes = createPointsBuffer(request.layout);
    const uniformBytes = createUniformBuffer(request);
    const pointCount = request.layout.points.length;
    const outputSize = pointCount * channelsForOutput(request.outputType) * 4;

    const pointsBuffer = device.createBuffer({
      size: pointsBytes.byteLength,
      usage: (globalThis as any).GPUBufferUsage.STORAGE | (globalThis as any).GPUBufferUsage.COPY_DST,
    });
    device.queue.writeBuffer(pointsBuffer, 0, pointsBytes);

    const uniformsBuffer = device.createBuffer({
      size: uniformBytes.byteLength,
      usage: (globalThis as any).GPUBufferUsage.UNIFORM | (globalThis as any).GPUBufferUsage.COPY_DST,
    });
    device.queue.writeBuffer(uniformsBuffer, 0, uniformBytes);

    const outputBuffer = device.createBuffer({
      size: outputSize,
      usage:
        (globalThis as any).GPUBufferUsage.STORAGE |
        (globalThis as any).GPUBufferUsage.COPY_SRC,
    });

    const readbackBuffer = device.createBuffer({
      size: outputSize,
      usage:
        (globalThis as any).GPUBufferUsage.COPY_DST |
        (globalThis as any).GPUBufferUsage.MAP_READ,
    });

    const bindGroup = device.createBindGroup({
      layout: pipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: { buffer: pointsBuffer } },
        { binding: 1, resource: { buffer: uniformsBuffer } },
        { binding: 2, resource: { buffer: outputBuffer } },
      ],
    });

    const encoder = device.createCommandEncoder();
    const pass = encoder.beginComputePass();
    pass.setPipeline(pipeline);
    pass.setBindGroup(0, bindGroup);
    pass.dispatchWorkgroups(Math.ceil(pointCount / 64));
    pass.end();
    encoder.copyBufferToBuffer(outputBuffer, 0, readbackBuffer, 0, outputSize);
    device.queue.submit([encoder.finish()]);

    await readbackBuffer.mapAsync((globalThis as any).GPUMapMode.READ);
    const mapped = readbackBuffer.getMappedRange();
    const values = new Float32Array(mapped.slice(0));
    readbackBuffer.unmap();

    return values;
  }

  private async getPipeline(wgsl: string): Promise<any> {
    if (this.cachedWGSL === wgsl && this.cachedPipeline) {
      return this.cachedPipeline;
    }

    const device = this.device;
    if (!device) {
      throw new Error("GPU device was not initialised.");
    }

    const shaderModule = device.createShaderModule({ code: wgsl });
    const getCompilationInfo = shaderModule.getCompilationInfo?.bind(shaderModule);
    if (getCompilationInfo) {
      const info = await getCompilationInfo();
      const errors = (info?.messages ?? []).filter((message: any) => message.type === "error");
      if (errors.length > 0) {
        const first = errors[0];
        throw new Error(first.message ?? "WGSL compilation failed.");
      }
    }

    const pipeline = await device.createComputePipelineAsync({
      layout: "auto",
      compute: {
        module: shaderModule,
        entryPoint: "main",
      },
    });

    this.cachedWGSL = wgsl;
    this.cachedShaderModule = shaderModule;
    this.cachedPipeline = pipeline;
    return pipeline;
  }
}

export function drawLayout(
  canvas: HTMLCanvasElement,
  layout: LayoutData,
  values: Float32Array,
  outputType: "scalar" | "rgb" | "rgbw",
): void {
  const context = canvas.getContext("2d");
  if (!context) {
    return;
  }

  const dpr = window.devicePixelRatio || 1;
  const width = Math.max(1, canvas.clientWidth);
  const height = Math.max(1, canvas.clientHeight);
  canvas.width = Math.floor(width * dpr);
  canvas.height = Math.floor(height * dpr);
  context.setTransform(dpr, 0, 0, dpr, 0, 0);

  const gradient = context.createLinearGradient(0, 0, width, height);
  gradient.addColorStop(0, "#09111d");
  gradient.addColorStop(1, "#18283e");
  context.fillStyle = gradient;
  context.fillRect(0, 0, width, height);

  const extents = getExtents(layout);
  const padding = 28;
  const usableWidth = Math.max(1, width - padding * 2);
  const usableHeight = Math.max(1, height - padding * 2);
  const scaleX = extents.spanX === 0 ? 0 : usableWidth / extents.spanX;
  const scaleY = extents.spanY === 0 ? 0 : usableHeight / extents.spanY;
  const scale = Math.min(scaleX || usableHeight, scaleY || usableWidth);

  context.strokeStyle = "rgba(255,255,255,0.06)";
  context.lineWidth = 1;
  context.strokeRect(18, 18, width - 36, height - 36);

  const radius = Math.max(5, Math.min(14, 180 / Math.sqrt(layout.points.length || 1)));
  const channels = channelsForOutput(outputType);

  for (let i = 0; i < layout.points.length; i++) {
    const point = layout.points[i];
    const offset = i * channels;
    const scalar = Math.max(0, Math.min(1, values[offset] ?? 0));
    let red = scalar
    let green = scalar
    let blue = scalar
    let glow = scalar
    if (outputType === "rgb") {
      red = Math.max(0, Math.min(1, values[offset] ?? 0));
      green = Math.max(0, Math.min(1, values[offset + 1] ?? 0));
      blue = Math.max(0, Math.min(1, values[offset + 2] ?? 0));
      glow = Math.max(red, green, blue);
    } else if (outputType === "rgbw") {
      const white = Math.max(0, Math.min(1, values[offset + 3] ?? 0));
      red = Math.min(1, Math.max(0, Math.min(1, values[offset] ?? 0)) + white);
      green = Math.min(1, Math.max(0, Math.min(1, values[offset + 1] ?? 0)) + white);
      blue = Math.min(1, Math.max(0, Math.min(1, values[offset + 2] ?? 0)) + white);
      glow = Math.max(red, green, blue);
    }
    const x =
      extents.spanX === 0
        ? width / 2
        : padding + (point.x - extents.minX) * scale + (usableWidth - extents.spanX * scale) / 2;
    const y =
      extents.spanY === 0
        ? height / 2
        : padding + (point.y - extents.minY) * scale + (usableHeight - extents.spanY * scale) / 2;

    context.beginPath();
    context.fillStyle = `rgba(${Math.round(red * 255)},${Math.round(green * 255)},${Math.round(blue * 255)},1)`;
    context.shadowColor = `rgba(${Math.round(red * 255)},${Math.round(green * 255)},${Math.round(blue * 255)},${0.15 + glow * 0.55})`;
    context.shadowBlur = 10 + glow * 24;
    context.arc(x, y, radius + glow * radius * 0.25, 0, Math.PI * 2);
    context.fill();
  }

  context.shadowBlur = 0;
}

function createPointsBuffer(layout: LayoutData): Uint8Array {
  const buffer = new ArrayBuffer(layout.points.length * 16);
  const view = new DataView(buffer);

  layout.points.forEach((point, index) => {
    const offset = index * 16;
    view.setUint32(offset, point.index, true);
    view.setFloat32(offset + 4, point.x, true);
    view.setFloat32(offset + 8, point.y, true);
    view.setFloat32(offset + 12, 0, true);
  });

  return new Uint8Array(buffer);
}

function createUniformBuffer(request: RenderRequest): Uint8Array {
  const numericParams = request.params.filter((param) => param.type !== "enum");
  const scalarCount = 4 + numericParams.length;
  const byteLength = alignTo16(scalarCount * 4);
  const buffer = new ArrayBuffer(byteLength);
  const view = new DataView(buffer);

  view.setFloat32(0, request.layout.width, true);
  view.setFloat32(4, request.layout.height, true);
  view.setFloat32(8, request.phase, true);
  view.setUint32(12, request.layout.points.length, true);

  numericParams.forEach((param, index) => {
    view.setFloat32(16 + index * 4, coerceUniformValue(request.boundParams[param.name]), true);
  });

  return new Uint8Array(buffer);
}

function coerceUniformValue(value: unknown): number {
  if (typeof value === "number") {
    return value;
  }
  if (typeof value === "boolean") {
    return value ? 1 : 0;
  }
  return 0;
}

function alignTo16(value: number): number {
  return Math.ceil(value / 16) * 16;
}

function channelsForOutput(outputType: "scalar" | "rgb" | "rgbw"): number {
  switch (outputType) {
    case "rgb":
      return 3;
    case "rgbw":
      return 4;
    default:
      return 1;
  }
}

function getExtents(layout: LayoutData): {
  minX: number;
  maxX: number;
  minY: number;
  maxY: number;
  spanX: number;
  spanY: number;
} {
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
    maxX,
    minY,
    maxY,
    spanX: maxX - minX,
    spanY: maxY - minY,
  };
}
