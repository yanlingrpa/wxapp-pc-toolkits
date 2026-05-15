package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type validateConfig struct {
	Module struct {
		Package string `json:"package"`
	} `json:"module"`
}

func loadTargetPackageName(rootDir string) (string, error) {
	configPath := filepath.Join(rootDir, ".yanling", "config.json")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", configPath, err)
	}

	var cfg validateConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		return "", fmt.Errorf("parse %s: %w", configPath, err)
	}

	pkg := strings.TrimSpace(cfg.Module.Package)
	if pkg == "" {
		return "", fmt.Errorf("module.package is required in %s", configPath)
	}
	return pkg, nil
}
