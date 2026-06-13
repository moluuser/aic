// Package config loads aic settings from a JSON file and applies
// sensible defaults. Command-line flags (handled in package app) take
// precedence over anything loaded here.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// OllamaConfig holds Ollama-specific settings.
type OllamaConfig struct {
	Endpoint string `json:"endpoint,omitempty"`
}

// Config is the full, resolved configuration.
type Config struct {
	// Provider selects the backend, e.g. "ollama".
	Provider string `json:"provider,omitempty"`
	// Model is the model name passed to the provider.
	Model string `json:"model,omitempty"`
	// History is how many recent commit subjects to feed the model for style.
	History int `json:"history,omitempty"`
	// MaxDiffBytes caps how much of the staged diff is sent to the model.
	// Larger diffs are truncated with a marker. 0 means use the default.
	MaxDiffBytes int `json:"max_diff_bytes,omitempty"`
	// Language is the natural language for the commit message, e.g. "en".
	Language string `json:"language,omitempty"`
	// ExtraInstructions is appended to the system prompt verbatim, letting a
	// project encode its own commit conventions.
	ExtraInstructions string `json:"extra_instructions,omitempty"`

	Ollama OllamaConfig `json:"ollama,omitempty"`
}

// Defaults returns the built-in configuration used when nothing else is set.
func Defaults() Config {
	return Config{
		Provider:     "ollama",
		Model:        "llama3:latest",
		History:      20,
		MaxDiffBytes: 12000,
		Language:     "en",
		Ollama:       OllamaConfig{Endpoint: "http://localhost:11434"},
	}
}

// DefaultPath returns the path of the user-level config file, honouring
// XDG_CONFIG_HOME and falling back to ~/.config/aic/config.json.
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "aic", "config.json")
}

// Load reads config from path and overlays it onto the defaults. A missing
// file is not an error: the defaults are returned. When path is empty the
// default user-level path is used.
func Load(path string) (Config, error) {
	cfg := Defaults()
	if path == "" {
		path = DefaultPath()
	}
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	// Decode into the already-populated defaults so omitted keys keep their
	// default values.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to path as indented JSON, creating parent directories.
func Save(cfg Config, path string) error {
	if path == "" {
		path = DefaultPath()
	}
	if path == "" {
		return fmt.Errorf("could not determine config path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
