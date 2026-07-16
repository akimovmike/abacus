package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// KeyTreeSort is the project-scoped tree sort order. Its value is one of the
// canonical sort tokens owned by internal/graph (e.g. "priority-asc").
const KeyTreeSort = "tree.sort"

// SaveSort persists the tree sort order to the project's .abacus/config.yaml.
// The order is data/tree-specific (like columns), so it is project-scoped, not
// a per-user preference. Errors are returned, not swallowed.
func SaveSort(value string) error {
	targetPath, err := localProjectConfigPath()
	if err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(targetPath)
	_ = v.ReadInConfig()
	v.Set(KeyTreeSort, value)

	dir := filepath.Dir(targetPath)
	//nolint:gosec // G301: Config directory needs standard permissions
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := v.WriteConfigAs(targetPath); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// LoadSort returns the project-scoped tree sort token, or "" if unset. Reads
// project config only (not the merged user/env layers).
func LoadSort() string {
	return GetProjectString(KeyTreeSort)
}
