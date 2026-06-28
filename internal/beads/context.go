package beads

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// StoreKind classifies the beads storage backend discovered on disk.
type StoreKind string

const (
	StoreKindUnknown StoreKind = ""
	StoreKindSQLite  StoreKind = "sqlite"
	StoreKindDolt    StoreKind = "dolt"
)

// BackendContext captures the output of <bin> context --json plus the
// store kind derived from the .beads directory. It is the single source
// of truth for routing decisions in NewClientForBackend.
type BackendContext struct {
	Backend       string    `json:"backend"`
	DoltMode      string    `json:"dolt_mode"`
	SchemaVersion int       `json:"schema_version"`
	Database      string    `json:"database"`
	BeadsDir      string    `json:"beads_dir"`
	RepoRoot      string    `json:"repo_root"`
	Kind          StoreKind `json:"-"` // derived from disk, not JSON
}

// ProbeContext runs <bin> context --json in workDir. If the binary lacks
// the context command or returns non-JSON output, it falls back to
// detectStoreKind(workDir/.beads) and returns a context with Backend set
// to bin and Kind set from disk.
func ProbeContext(ctx context.Context, bin, workDir string) (BackendContext, error) {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return BackendContext{}, fmt.Errorf("binary name is required")
	}

	out, err := runContextCommand(ctx, bin, workDir)
	if err == nil {
		jsonBytes := extractJSON(out)
		if jsonBytes != nil {
			var parsed BackendContext
			if unmarshalErr := json.Unmarshal(jsonBytes, &parsed); unmarshalErr == nil {
				if parsed.Kind == StoreKindUnknown {
					parsed.Kind = detectStoreKind(filepath.Join(workDir, ".beads"))
				}
				return parsed, nil
			}
		}
	}

	// Fallback: old binary or no context command; infer from disk.
	kind := detectStoreKind(filepath.Join(workDir, ".beads"))
	backend := bin
	switch kind {
	case StoreKindDolt:
		backend = "dolt"
	case StoreKindSQLite:
		backend = "sqlite"
	}
	return BackendContext{
		Backend:       backend,
		SchemaVersion: 0,
		Kind:          kind,
	}, nil
}

func runContextCommand(ctx context.Context, bin, workDir string) ([]byte, error) {
	//nolint:gosec // G204: caller controls the binary path.
	cmd := exec.CommandContext(ctx, bin, "context", "--json")
	if workDir != "" {
		cmd.Dir = workDir
	}
	return cmd.CombinedOutput()
}

// detectStoreKind inspects .beads directory contents to choose the reader.
// Dolt directories take precedence over legacy SQLite files.
func detectStoreKind(beadsDir string) StoreKind {
	if _, err := os.Stat(filepath.Join(beadsDir, "embeddeddolt")); err == nil {
		return StoreKindDolt
	}
	if _, err := os.Stat(filepath.Join(beadsDir, "dolt")); err == nil {
		return StoreKindDolt
	}
	if info, err := os.Stat(filepath.Join(beadsDir, "beads.db")); err == nil && !info.IsDir() {
		return StoreKindSQLite
	}
	return StoreKindUnknown
}
