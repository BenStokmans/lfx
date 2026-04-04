package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/BenStokmans/lfx/modules"
	"github.com/BenStokmans/lfx/stdlib"
)

const CompilerVersion = "0.1"

// Key identifies a cacheable compilation artifact.
type Key struct {
	Backend         string `json:"backend"`
	CompilerVersion string `json:"compiler_version"`
	StdlibVersion   string `json:"stdlib_version"`
	SourceHash      string `json:"source_hash"`
	GraphHash       string `json:"graph_hash"`
}

func (k Key) String() string {
	return fmt.Sprintf("%s-%s-%s-%s-%s", k.Backend, k.CompilerVersion, k.StdlibVersion, k.SourceHash, k.GraphHash)
}

// NewKey creates a deterministic cache key for a compiler artifact.
func NewKey(source []byte, graph *modules.ModuleGraph, backend string) (Key, error) {
	graphHash, err := GraphHash(graph)
	if err != nil {
		return Key{}, err
	}
	return Key{
		Backend:         backend,
		CompilerVersion: CompilerVersion,
		StdlibVersion:   stdlib.Version,
		SourceHash:      HashBytes(source),
		GraphHash:       graphHash,
	}, nil
}

// HashBytes returns a stable SHA-256 hex digest.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// GraphHash hashes the resolved import graph and source payloads.
func GraphHash(graph *modules.ModuleGraph) (string, error) {
	if graph == nil {
		return "", fmt.Errorf("nil module graph")
	}

	type nodeEntry struct {
		Path       string   `json:"path"`
		IsLib      bool     `json:"is_lib"`
		SourceHash string   `json:"source_hash"`
		Edges      []string `json:"edges"`
	}

	paths := make([]string, 0, len(graph.Nodes))
	for path := range graph.Nodes {
		paths = append(paths, path)
	}
	slices.Sort(paths)

	nodes := make([]nodeEntry, 0, len(paths))
	for _, path := range paths {
		node := graph.Nodes[path]
		edges := append([]string(nil), graph.Edges[path]...)
		slices.Sort(edges)
		nodes = append(nodes, nodeEntry{
			Path:       path,
			IsLib:      node.IsLib,
			SourceHash: HashBytes(node.Source),
			Edges:      edges,
		})
	}

	payload := struct {
		Entry string      `json:"entry"`
		Nodes []nodeEntry `json:"nodes"`
	}{
		Entry: graph.Entry,
		Nodes: nodes,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return HashBytes(encoded), nil
}
