package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveSortRoundTripProjectScoped(t *testing.T) {
	reset()
	t.Cleanup(reset)

	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "repo")
	mustMkdir(t, filepath.Join(projectDir, ".beads"))
	mustMkdir(t, filepath.Join(projectDir, ".abacus"))
	projectCfg := filepath.Join(projectDir, ".abacus", "config.yaml")
	writeFile(t, projectCfg, "theme: tokyonight\n")

	oldWd, _ := os.Getwd()
	_ = os.Chdir(projectDir)
	defer func() { _ = os.Chdir(oldWd) }()

	userCfg := filepath.Join(tmp, "user.yaml")
	if err := Initialize(WithWorkingDir(projectDir), WithUserConfig(userCfg)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if got := LoadSort(); got != "" {
		t.Fatalf("LoadSort before save = %q, want empty", got)
	}

	if err := SaveSort("priority-desc"); err != nil {
		t.Fatalf("SaveSort: %v", err)
	}

	if got := LoadSort(); got != "priority-desc" {
		t.Fatalf("LoadSort = %q, want priority-desc", got)
	}

	data, err := os.ReadFile(projectCfg)
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	if !contains(string(data), "theme: tokyonight") {
		t.Fatalf("expected theme preserved, got:\n%s", data)
	}
}
