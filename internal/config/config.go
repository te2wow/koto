// Package config loads koto runtime configuration from ~/.koto/config.yaml,
// applying sensible defaults. Secrets are never read from CLI flags.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds runtime settings for koto.
type Config struct {
	Provider string `yaml:"provider"` // claude, codex, aider, gemini, copilot, mock
	Model    string `yaml:"model"`    // passed through to the provider when supported
	Language string `yaml:"language"` // en or ja (used in some prompts/messages)
}

// Default returns the built-in default configuration.
func Default() Config {
	return Config{Provider: "claude", Model: "", Language: "en"}
}

// Path returns the location of the user config file (~/.koto/config.yaml).
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".koto", "config.yaml")
}

// Load reads the user config, falling back to defaults for any missing field.
// A missing file is not an error; defaults are returned.
func Load() (Config, error) {
	cfg := Default()
	p := Path()
	if p == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", p, err)
	}
	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", p, err)
	}
	if fileCfg.Provider != "" {
		cfg.Provider = fileCfg.Provider
	}
	if fileCfg.Model != "" {
		cfg.Model = fileCfg.Model
	}
	if fileCfg.Language != "" {
		cfg.Language = fileCfg.Language
	}
	return cfg, nil
}
