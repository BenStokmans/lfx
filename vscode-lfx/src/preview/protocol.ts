export interface PreviewLayoutPoint {
  index: number;
  x: number;
  y: number;
}

export interface PreviewLayout {
  id: string;
  name: string;
  width: number;
  height: number;
  points: PreviewLayoutPoint[];
}

export interface PreviewParam {
  name: string;
  type: "int" | "float" | "bool" | "enum" | string;
  defaultValue: unknown;
  min?: number;
  max?: number;
  enumValues?: string[];
}

export interface PreviewTimeline {
  loopStart?: number;
  loopEnd?: number;
}

export interface PreviewDiagnostic {
  severity: string;
  code?: string;
  message: string;
  filePath?: string;
  line?: number;
  column?: number;
}

export interface PreviewArtifact {
  ok: boolean;
  filePath: string;
  modulePath?: string;
  outputType?: "scalar" | "rgb" | "rgbw";
  wgsl?: string;
  params: PreviewParam[];
  boundParams?: Record<string, unknown>;
  timeline?: PreviewTimeline;
  diagnostics: PreviewDiagnostic[];
}

export type ExtensionToWebviewMessage =
  | {
      type: "init";
      mode: "preview" | "probe";
      layouts: PreviewLayout[];
      preferredLayoutId: string;
      filePath?: string;
      compilerSource?: string;
      themeKind: string;
    }
  | {
      type: "compiledEffect";
      artifact: PreviewArtifact;
      compilerSource?: string;
    }
  | {
      type: "compileError";
      artifact: PreviewArtifact;
      compilerSource?: string;
      reason?: string;
    }
  | {
      type: "setTheme";
      themeKind: string;
    }
  | {
      type: "dispose";
    };

export type WebviewToExtensionMessage =
  | { type: "ready" }
  | {
      type: "probeResult";
      ok: boolean;
      details: {
        hasNavigatorGpu: boolean;
        adapterAcquired: boolean;
        canvasContextAvailable: boolean;
        error?: string;
        adapterInfo?: string;
        userAgent?: string;
      };
    }
  | { type: "setPlayback"; playing: boolean; speed: number }
  | { type: "setPhase"; phase: number }
  | { type: "setParams"; params: Record<string, unknown> }
  | { type: "setLayout"; layoutId: string }
  | { type: "requestRefresh" }
  | { type: "runtimeError"; message: string };
