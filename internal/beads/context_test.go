package beads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectStoreKind_RecognizesDolt(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if got := detectStoreKind(beadsDir); got != StoreKindDolt {
		t.Fatalf("detectStoreKind() = %q, want %q", got, StoreKindDolt)
	}
}

func TestDetectStoreKind_RecognizesSQLite(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.db"), []byte("db"), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	if got := detectStoreKind(beadsDir); got != StoreKindSQLite {
		t.Fatalf("detectStoreKind() = %q, want %q", got, StoreKindSQLite)
	}
}

func TestDetectStoreKind_PrefersDoltOverSQLite(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDir, "dolt"), 0o755); err != nil {
		t.Fatalf("mkdir dolt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.db"), []byte("db"), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	if got := detectStoreKind(beadsDir); got != StoreKindDolt {
		t.Fatalf("detectStoreKind() = %q, want %q", got, StoreKindDolt)
	}
}

func TestDetectStoreKind_Unknown(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if got := detectStoreKind(beadsDir); got != StoreKindUnknown {
		t.Fatalf("detectStoreKind() = %q, want %q", got, StoreKindUnknown)
	}
}

func TestProbeContext_ParsesJSON(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebd.sh")
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "context" ] && [ "$2" = "--json" ]; then
  echo '{"backend":"dolt","dolt_mode":"embedded","schema_version":1,"database":"mydb","beads_dir":"/x/.beads","repo_root":"/x"}'
fi
exit 0
`)

	ctx, err := ProbeContext(t.Context(), script, dir)
	if err != nil {
		t.Fatalf("ProbeContext: %v", err)
	}
	if ctx.Backend != "dolt" {
		t.Errorf("Backend = %q, want dolt", ctx.Backend)
	}
	if ctx.DoltMode != "embedded" {
		t.Errorf("DoltMode = %q, want embedded", ctx.DoltMode)
	}
	if ctx.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", ctx.SchemaVersion)
	}
}

func TestProbeContext_SkipsLeadingWarnings(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakebd.sh")
	writeTestScript(t, script, `#!/bin/sh
if [ "$1" = "context" ] && [ "$2" = "--json" ]; then
  echo 'warning: permission thing'
  echo '{"backend":"dolt","dolt_mode":"embedded","schema_version":1}'
fi
exit 0
`)

	ctx, err := ProbeContext(t.Context(), script, dir)
	if err != nil {
		t.Fatalf("ProbeContext: %v", err)
	}
	if ctx.Backend != "dolt" {
		t.Errorf("Backend = %q, want dolt", ctx.Backend)
	}
}

func TestProbeContext_FallsBackToDetectStoreKind(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	script := filepath.Join(dir, "fakebd.sh")
	writeTestScript(t, script, `#!/bin/sh
echo 'unknown command'
exit 1
`)

	ctx, err := ProbeContext(t.Context(), script, dir)
	if err != nil {
		t.Fatalf("ProbeContext: %v", err)
	}
	if ctx.Backend != "dolt" {
		t.Errorf("Backend = %q, want dolt", ctx.Backend)
	}
	if ctx.Kind != StoreKindDolt {
		t.Errorf("Kind = %q, want %q", ctx.Kind, StoreKindDolt)
	}
}
