// Package config loads baton's layered configuration: built-in defaults, then a
// global file (~/.config/baton/config.toml), then a per-project .baton.toml
// found by walking up from the working directory. Later layers override earlier
// ones key-by-key, so a project can retune just its execute model.
package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the whole configuration surface.
type Config struct {
	Port        int                      `toml:"port"`
	LogLevel    string                   `toml:"log_level"`
	OnExecError string                   `toml:"on_exec_error"` // "fallback-claude" | "error"
	DefaultRole string                   `toml:"default_role"`  // role for unmatched tiers
	Roles       map[string]RoleConfig    `toml:"roles"`
	Tiers       map[string]string        `toml:"tiers"` // tier name -> role name
	Backends    map[string]BackendConfig `toml:"backends"`
	Prices      map[string]Price         `toml:"prices"`
}

// RoleConfig binds a role (plan/execute) to a backend and, optionally, a fixed
// upstream model. An empty Model means "keep whatever model the client sent"
// (used by the plan/passthrough role so a tier is never forced).
type RoleConfig struct {
	Backend string `toml:"backend"`
	Model   string `toml:"model"`
}

// BackendConfig describes one upstream provider.
type BackendConfig struct {
	Type       string `toml:"type"`       // wire: "anthropic" | "openai"
	BaseURL    string `toml:"base_url"`   // prefix before /v1/...
	Auth       string `toml:"auth"`       // "passthrough" | "bearer" | "x-api-key"
	Credential string `toml:"credential"` // keyring service name (for key auth)
	ModelsURL  string `toml:"models_url"` // optional: catalog for `baton models`
}

// Price is a per-model cost, USD per 1M tokens, for usage estimates.
type Price struct {
	In  float64 `toml:"in"`
	Out float64 `toml:"out"`
}

// Defaults returns the built-in configuration: Claude lane via subscription
// passthrough, OpenCode lane over its OpenAI-compatible Zen endpoint, tiers wired
// so opus/sonnet plan and haiku executes.
func Defaults() *Config {
	return &Config{
		Port:        8787,
		LogLevel:    "info",
		OnExecError: "fallback-claude",
		DefaultRole: "plan",
		Roles: map[string]RoleConfig{
			"plan":    {Backend: "anthropic", Model: ""},
			"execute": {Backend: "opencode", Model: ""},
		},
		Tiers: map[string]string{
			"opus":   "plan",
			"sonnet": "plan",
			"haiku":  "execute",
		},
		Backends: map[string]BackendConfig{
			"anthropic": {
				Type:    "anthropic",
				BaseURL: "https://api.anthropic.com",
				Auth:    "passthrough",
			},
			"opencode": {
				Type:       "openai",
				BaseURL:    "https://opencode.ai/zen",
				Auth:       "bearer",
				Credential: "opencode",
				ModelsURL:  "https://opencode.ai/zen/v1/models",
			},
		},
		Prices: map[string]Price{},
	}
}

// Load builds the effective config: defaults, then the global file, then the
// nearest project file. Missing files are skipped silently.
func Load() (*Config, error) {
	cfg := Defaults()
	if gp := GlobalPath(); gp != "" {
		if err := mergeFile(cfg, gp); err != nil {
			return nil, err
		}
	}
	if pp := ProjectPath(); pp != "" {
		if err := mergeFile(cfg, pp); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// mergeFile decodes a TOML file over cfg. BurntSushi decodes into the existing
// value, adding/overwriting map keys while leaving untouched ones intact — which
// is exactly the override-by-key behaviour we want.
func mergeFile(cfg *Config, path string) error {
	if _, err := os.Stat(path); err != nil {
		return nil // absent layer: fine
	}
	_, err := toml.DecodeFile(path, cfg)
	return err
}

// GlobalPath is ~/.config/baton/config.toml (honoring XDG_CONFIG_HOME).
func GlobalPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "baton", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "baton", "config.toml")
}

// ProjectPath walks up from the working directory looking for .baton.toml.
func ProjectPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ".baton.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Save writes cfg to path as TOML, creating parent directories.
func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
