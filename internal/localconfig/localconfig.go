// Package localconfig reads the consumer-local .buttress.local.toml override file.
package localconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// LocalConfig holds per-project overrides that are not committed to source control.
type LocalConfig struct {
	PackageManager string
	Overrides      map[string]PackageOverride
}

// PackageOverride holds per-package generation settings.
type PackageOverride struct {
	Output   string // output file path relative to project root
	Language string // language override for this package
}

// Load reads .buttress.local.toml from projectDir.
// Returns an empty config (no error) if the file does not exist.
func Load(projectDir string) (*LocalConfig, error) {
	path := filepath.Join(projectDir, ".buttress.local.toml")

	var raw map[string]any
	_, err := toml.DecodeFile(path, &raw)
	if os.IsNotExist(err) {
		return &LocalConfig{Overrides: make(map[string]PackageOverride)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading .buttress.local.toml: %w", err)
	}

	cfg := &LocalConfig{Overrides: make(map[string]PackageOverride)}
	for k, v := range raw {
		if k == "package_manager" {
			if s, ok := v.(string); ok {
				cfg.PackageManager = s
			}
			continue
		}
		if m, ok := v.(map[string]interface{}); ok {
			override := PackageOverride{}
			if s, ok := m["output"].(string); ok {
				override.Output = s
			}
			if s, ok := m["language"].(string); ok {
				override.Language = s
			}
			cfg.Overrides[k] = override
		}
	}
	return cfg, nil
}
