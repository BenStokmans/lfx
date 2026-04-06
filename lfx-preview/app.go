package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"

	"github.com/BenStokmans/lfx/compiler"
	"github.com/BenStokmans/lfx/ir"
	"github.com/BenStokmans/lfx/modules"
	lfxruntime "github.com/BenStokmans/lfx/runtime"
	"github.com/BenStokmans/lfx/stdlib"
	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type compilationSession struct {
	artifact *compiler.PreviewArtifact
}

type App struct {
	ctx          context.Context
	mu           sync.RWMutex
	compilations map[string]*compilationSession
	workspaces   map[string]struct{}
}

func NewApp() *App {
	return &App{
		compilations: make(map[string]*compilationSession),
		workspaces:   make(map[string]struct{}),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) Bootstrap() (*BootstrapData, error) {
	root := defaultWorkspaceRoot()
	workspace, err := a.LoadWorkspace(root)
	if err != nil {
		return nil, err
	}
	return &BootstrapData{
		DefaultWorkspace: root,
		Workspace:        workspace,
		Layouts:          builtInLayouts(),
	}, nil
}

func (a *App) OpenWorkspace() (*WorkspaceData, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:            "Open LFX workspace",
		DefaultDirectory: defaultWorkspaceRoot(),
	})
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, nil
	}
	return a.LoadWorkspace(dir)
}

func (a *App) LoadWorkspace(root string) (*WorkspaceData, error) {
	root, err := canonicalPath(root, false)
	if err != nil {
		return nil, err
	}
	effects := make([]EffectFileData, 0, 16)

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build", "stdlib":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".lfx" {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		item := EffectFileData{
			Name:         strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
			Path:         path,
			RelativePath: filepath.ToSlash(rel),
		}

		if parsed, _, err := compiler.ParseFile(path); err == nil {
			item.ModulePath = parsed.ModPath
		}

		effects = append(effects, item)
		return nil
	})
	if err != nil {
		return nil, err
	}

	a.rememberWorkspace(root)

	sort.Slice(effects, func(i, j int) bool {
		return effects[i].RelativePath < effects[j].RelativePath
	})

	return &WorkspaceData{
		Root:    root,
		Effects: effects,
	}, nil
}

func (a *App) ReadSourceFile(path string) (string, error) {
	if err := a.ensurePathInKnownWorkspace(path, false); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func (a *App) SaveSourceFile(req SaveSourceRequest) error {
	if req.Path == "" {
		return errors.New("save path is required")
	}
	if err := a.ensurePathInKnownWorkspace(req.Path, true); err != nil {
		return err
	}
	if err := os.WriteFile(req.Path, []byte(req.Content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", req.Path, err)
	}
	return nil
}

func (a *App) SelectLayoutFile() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:            "Import layout JSON",
		DefaultDirectory: defaultWorkspaceRoot(),
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "LFX Layout JSON",
			Pattern:     "*.json",
		}},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) LoadLayout(path string) (*LayoutData, error) {
	if path == "" {
		return nil, errors.New("layout path is required")
	}
	layout, err := loadLayoutFile(path)
	if err != nil {
		return nil, err
	}
	return &layout, nil
}

func (a *App) CompilePreview(req CompileRequest) (*CompileResponse, error) {
	if req.FilePath == "" {
		return nil, errors.New("file path is required")
	}

	workspaceRoot := req.WorkspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = compiler.DetectBaseDir(req.FilePath)
	}
	if err := a.ensurePathInWorkspace(workspaceRoot, req.FilePath, false); err != nil {
		return nil, err
	}
	a.rememberWorkspace(workspaceRoot)

	artifact, err := compiler.CompileForPreview(req.FilePath, req.Overrides, compiler.Options{
		BaseDir:  workspaceRoot,
		Resolver: stdlib.NewResolver(modules.NewFileResolver(modules.DefaultRoots(workspaceRoot)...)),
	})
	if err != nil {
		var previewErr *compiler.PreviewError
		if errors.As(err, &previewErr) {
			return &CompileResponse{
				WorkspaceRoot: workspaceRoot,
				FilePath:      req.FilePath,
				Diagnostics:   mapDiagnostics(previewErr.Diagnostics),
			}, nil
		}
		return nil, err
	}

	compilationID := uuid.NewString()

	a.mu.Lock()
	a.compilations[compilationID] = &compilationSession{artifact: artifact}
	a.mu.Unlock()

	return &CompileResponse{
		CompilationID: compilationID,
		WorkspaceRoot: workspaceRoot,
		FilePath:      req.FilePath,
		ModulePath:    artifact.ModulePath,
		OutputType:    artifact.OutputType,
		WGSL:          artifact.WGSL,
		Params:        mapParams(artifact.Params),
		BoundParams:   artifact.BoundParams,
		Timeline:      mapTimeline(artifact.Timeline),
		Diagnostics:   mapDiagnostics(artifact.Diagnostics),
	}, nil
}

func (a *App) SampleCompiledFrame(req SampleRequest) (*SampleResponse, error) {
	if req.CompilationID == "" {
		return nil, errors.New("compilation id is required")
	}

	a.mu.RLock()
	session := a.compilations[req.CompilationID]
	a.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("unknown compilation %q", req.CompilationID)
	}

	layout := toRuntimeLayout(req.Layout)
	if err := lfxruntime.ValidateLayout(layout); err != nil {
		return nil, err
	}

	bound, err := lfxruntime.Bind(session.artifact.Result.IR.Params, req.Overrides)
	if err != nil {
		return nil, err
	}

	limit := len(layout.Points)
	if req.Limit > 0 && req.Limit < limit {
		limit = req.Limit
	}

	points := make([]SamplePointData, 0, limit)
	for i := 0; i < limit; i++ {
		pt := layout.Points[i]
		values, err := session.artifact.Sampler.SamplePoint(layout, i, req.Phase, bound)
		if err != nil {
			return nil, err
		}
		points = append(points, SamplePointData{
			Index:  pt.Index,
			X:      pt.X,
			Y:      pt.Y,
			Values: values,
		})
	}

	return &SampleResponse{Points: points}, nil
}

func defaultWorkspaceRoot() string {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func (a *App) rememberWorkspace(root string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.workspaces[root] = struct{}{}
}

func (a *App) ensurePathInKnownWorkspace(target string, allowMissingLeaf bool) error {
	a.mu.RLock()
	roots := make([]string, 0, len(a.workspaces))
	for root := range a.workspaces {
		roots = append(roots, root)
	}
	a.mu.RUnlock()

	for _, root := range roots {
		if err := ensurePathInRoot(root, target, allowMissingLeaf); err == nil {
			return nil
		}
	}

	return fmt.Errorf("path %q is outside the loaded workspaces", target)
}

func (a *App) ensurePathInWorkspace(root, target string, allowMissingLeaf bool) error {
	canonicalRoot, err := canonicalPath(root, false)
	if err != nil {
		return err
	}
	if err := ensurePathInRoot(canonicalRoot, target, allowMissingLeaf); err != nil {
		return err
	}
	return nil
}

func ensurePathInRoot(root, target string, allowMissingLeaf bool) error {
	canonicalRoot, err := canonicalPath(root, false)
	if err != nil {
		return err
	}
	canonicalTarget, err := canonicalPath(target, allowMissingLeaf)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(canonicalRoot, canonicalTarget)
	if err != nil {
		return fmt.Errorf("resolve %q relative to %q: %w", canonicalTarget, canonicalRoot, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q is outside workspace root %q", target, canonicalRoot)
	}
	return nil
}

func canonicalPath(value string, allowMissingLeaf bool) (string, error) {
	if value == "" {
		return "", errors.New("path is required")
	}

	abs, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", fmt.Errorf("absolute path for %q: %w", value, err)
	}

	if !allowMissingLeaf {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("resolve path %q: %w", value, err)
		}
		return resolved, nil
	}

	parent := filepath.Dir(abs)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("resolve parent for %q: %w", value, err)
	}
	return filepath.Join(resolvedParent, filepath.Base(abs)), nil
}

func mapParams(params []ir.ParamSpec) []ParamData {
	out := make([]ParamData, 0, len(params))
	for _, param := range params {
		item := ParamData{
			Name:       param.Name,
			Type:       paramTypeName(param.Type),
			Min:        param.Min,
			Max:        param.Max,
			EnumValues: append([]string(nil), param.EnumValues...),
		}
		switch param.Type {
		case ir.ParamInt:
			item.DefaultValue = param.IntDefault
		case ir.ParamFloat:
			item.DefaultValue = param.FloatDefault
		case ir.ParamBool:
			item.DefaultValue = param.BoolDefault
		case ir.ParamEnum:
			item.DefaultValue = param.EnumDefault
		}
		out = append(out, item)
	}
	return out
}

func mapTimeline(tl *ir.TimelineSpec) *TimelineData {
	if tl == nil {
		return nil
	}
	return &TimelineData{
		LoopStart: tl.LoopStart,
		LoopEnd:   tl.LoopEnd,
	}
}

func mapDiagnostics(items []compiler.PreviewDiagnostic) []DiagnosticData {
	out := make([]DiagnosticData, 0, len(items))
	for _, item := range items {
		out = append(out, DiagnosticData{
			Severity: item.Severity,
			Code:     item.Code,
			Message:  item.Message,
			FilePath: item.FilePath,
			Line:     item.Line,
			Column:   item.Column,
		})
	}
	return out
}

func paramTypeName(kind ir.ParamType) string {
	switch kind {
	case ir.ParamInt:
		return "int"
	case ir.ParamFloat:
		return "float"
	case ir.ParamBool:
		return "bool"
	case ir.ParamEnum:
		return "enum"
	default:
		return "unknown"
	}
}
