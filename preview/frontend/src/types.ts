export interface BootstrapData {
  defaultWorkspace: string;
  workspace: WorkspaceData | null;
  layouts: LayoutData[];
}

export interface WorkspaceData {
  root: string;
  effects: EffectFileData[];
}

export interface EffectFileData {
  name: string;
  path: string;
  relativePath: string;
  modulePath?: string;
}

export interface LayoutPoint {
  index: number;
  x: number;
  y: number;
}

export interface LayoutData {
  id: string;
  name: string;
  builtIn: boolean;
  path?: string;
  width: number;
  height: number;
  points: LayoutPoint[];
}

export interface ParamData {
  name: string;
  type: "int" | "float" | "bool" | "enum" | "unknown";
  defaultValue: unknown;
  min?: number;
  max?: number;
  enumValues?: string[];
}

export interface TimelineData {
  loopStart?: number;
  loopEnd?: number;
}

export interface DiagnosticData {
  severity: string;
  code?: string;
  message: string;
  filePath?: string;
  line?: number;
  column?: number;
}

export interface CompileResponse {
  compilationId?: string;
  workspaceRoot: string;
  filePath: string;
  modulePath?: string;
  wgsl?: string;
  params: ParamData[];
  boundParams?: Record<string, unknown>;
  timeline?: TimelineData;
  diagnostics: DiagnosticData[];
}

export interface SamplePointData {
  index: number;
  x: number;
  y: number;
  value: number;
}

export interface SampleResponse {
  points: SamplePointData[];
}
