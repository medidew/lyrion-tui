// Package config loads lyrion-tui's per-user settings from a JSON file.
//
// Binaries built by install.sh read the per-user config
// (<os.UserConfigDir()>/lyrion-tui/config.json, e.g.
// ~/.config/lyrion-tui/config.json on Linux). Everything else - go run,
// go test, a plain go build - reads config.json from the repository root
// instead, so development never touches (or needs) an installed config.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// installed is set to "true" by install.sh via
// -ldflags "-X github.com/medidew/lyrion-tui/internal/config.installed=true".
var installed string

type Config struct {
	// ServerAddress is the LMS CLI endpoint as host:port (LMS's default CLI
	// port is 9090).
	ServerAddress string `json:"server_address"`
}

// Path returns where the config file is expected to live.
func Path() (string, error) {
	if installed == "true" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "lyrion-tui", "config.json"), nil
	}

	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.json"), nil
}

// repoRoot locates the repository from this source file's compile-time
// path, so it works the same for go run and go test whatever the working
// directory (go test runs in the package's own directory).
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("config: cannot locate the repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}

// Load reads and validates the config file at Path.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("no config file found at %s", path)
	}
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("invalid config file %s: %w", path, err)
	}
	if cfg.ServerAddress == "" {
		return Config{}, fmt.Errorf("config file %s is missing \"server_address\"", path)
	}
	return cfg, nil
}
