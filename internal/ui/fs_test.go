package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindBeadsWorkdir_WalksUpToBeadsDir(t *testing.T) {
	root := t.TempDir()
	beadsDir := filepath.Join(root, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	cleanup := changeWorkingDir(t, nested)
	defer cleanup()

	workDir, foundBeadsDir, err := FindBeadsWorkdir()
	if err != nil {
		t.Fatalf("FindBeadsWorkdir: %v", err)
	}
	if normalizePath(t, workDir) != normalizePath(t, root) {
		t.Errorf("workDir = %q, want %q", workDir, root)
	}
	if normalizePath(t, foundBeadsDir) != normalizePath(t, beadsDir) {
		t.Errorf("beadsDir = %q, want %q", foundBeadsDir, beadsDir)
	}
}

func TestFindBeadsWorkdir_ReturnsErrorWhenMissing(t *testing.T) {
	root := t.TempDir()
	cleanup := changeWorkingDir(t, root)
	defer cleanup()

	_, _, err := FindBeadsWorkdir()
	if err == nil {
		t.Fatal("expected error when .beads directory is missing")
	}
}

func TestFindBeadsWorkdir_IgnoresFileNamedBeads(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".beads"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cleanup := changeWorkingDir(t, root)
	defer cleanup()

	_, _, err := FindBeadsWorkdir()
	if err == nil {
		t.Fatal("expected error when .beads is a file")
	}
}
