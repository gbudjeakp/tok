// Package config persists small user preferences (currently just skill mode).
// No config file is required — defaults apply if absent, matching tok's
// zero-config install philosophy. TOK_MODE env var overrides the file.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Mode string `json:"mode"` // "lite", "full" (default), "ultra", or "off"
}

func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "tok")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load returns the current config. Missing file or keys fall back to defaults.
func Load() Config {
	c := Config{Mode: "full"}
	if envMode := os.Getenv("TOK_MODE"); envMode != "" {
		c.Mode = envMode
		return c
	}
	p, err := path()
	if err != nil {
		return c
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return c
	}
	var stored Config
	if json.Unmarshal(data, &stored) == nil && stored.Mode != "" {
		c.Mode = stored.Mode
	}
	return c
}

// Save writes the config to disk.
func Save(c Config) error {
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}
