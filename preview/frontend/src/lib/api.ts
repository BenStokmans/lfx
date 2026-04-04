import {
  Bootstrap,
  CompilePreview,
  LoadLayout,
  LoadWorkspace,
  OpenWorkspace,
  ReadSourceFile,
  SampleCompiledFrame,
  SaveSourceFile,
  SelectLayoutFile,
} from "../../wailsjs/go/main/App";
import { main as models } from "../../wailsjs/go/models";
import type {
  BootstrapData,
  CompileResponse,
  LayoutData,
  SampleResponse,
  WorkspaceData,
} from "../types";

export async function bootstrap(): Promise<BootstrapData> {
  return normalizeBootstrap(await Bootstrap());
}

export async function openWorkspace(): Promise<WorkspaceData | null> {
  const workspace = await OpenWorkspace();
  return workspace ? normalizeWorkspace(workspace) : null;
}

export async function loadWorkspace(root: string): Promise<WorkspaceData> {
  return normalizeWorkspace(await LoadWorkspace(root));
}

export async function readSourceFile(path: string): Promise<string> {
  return ReadSourceFile(path);
}

export async function saveSourceFile(path: string, content: string): Promise<void> {
  return SaveSourceFile(new models.SaveSourceRequest({ path, content }));
}

export async function selectLayoutFile(): Promise<string> {
  return SelectLayoutFile();
}

export async function loadLayout(path: string): Promise<LayoutData> {
  return normalizeLayout(await LoadLayout(path));
}

export async function compilePreview(req: {
  workspaceRoot: string;
  filePath: string;
  overrides: Record<string, unknown>;
}): Promise<CompileResponse> {
  return (await CompilePreview(new models.CompileRequest(req))) as unknown as CompileResponse;
}

export async function sampleCompiledFrame(req: {
  compilationId: string;
  layout: LayoutData;
  phase: number;
  overrides: Record<string, unknown>;
  limit: number;
}): Promise<SampleResponse> {
  return (await SampleCompiledFrame(
    new models.SampleRequest({
      ...req,
      layout: toModelLayout(req.layout),
    }),
  )) as unknown as SampleResponse;
}

function normalizeBootstrap(value: models.BootstrapData): BootstrapData {
  return {
    defaultWorkspace: value.defaultWorkspace,
    workspace: value.workspace ? normalizeWorkspace(value.workspace) : null,
    layouts: value.layouts.map(normalizeLayout),
  };
}

function normalizeWorkspace(value: models.WorkspaceData): WorkspaceData {
  return {
    root: value.root,
    effects: value.effects.map((effect) => ({
      name: effect.name,
      path: effect.path,
      relativePath: effect.relativePath,
      modulePath: effect.modulePath,
    })),
  };
}

function normalizeLayout(value: models.LayoutData): LayoutData {
  return {
    id: value.id,
    name: value.name,
    builtIn: value.builtIn,
    path: value.path,
    width: value.width,
    height: value.height,
    points: value.points.map((point) => ({
      index: point.Index,
      x: point.X,
      y: point.Y,
    })),
  };
}

function toModelLayout(value: LayoutData): models.LayoutData {
  return new models.LayoutData({
    id: value.id,
    name: value.name,
    builtIn: value.builtIn,
    path: value.path,
    width: value.width,
    height: value.height,
    points: value.points.map((point) => ({
      Index: point.index,
      X: point.x,
      Y: point.y,
    })),
  });
}
