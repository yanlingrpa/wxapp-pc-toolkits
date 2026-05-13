package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

var excludedTopLevelDirs = map[string]struct{}{
	".git":      {},
	".vscode":   {},
	".protocol": {},
	".yanling":  {},
	"cmd":       {},
	"doc":       {},
	"tests":     {},
	"symbols":   {},
	"schema":    {},
}

func parseModuleName(goModPath string) (string, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "module ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}
	return "", errors.New("module line not found in go.mod")
}

type compileConfig struct {
	Module struct {
		Package string `json:"package"`
	} `json:"module"`
}

func loadTargetPackageName(rootDir string) (string, error) {
	configPath := filepath.Join(rootDir, ".yanling", "config.json")
	content, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("config file not found: %s", configPath)
		}
		return "", fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg compileConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	targetPackage := strings.TrimSpace(cfg.Module.Package)
	if targetPackage == "" {
		return "", fmt.Errorf("module.package is required in %s", configPath)
	}
	return targetPackage, nil
}

func matchesTargetPackage(filePath, targetPackage string) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.PackageClauseOnly)
	if err != nil {
		return false
	}
	return f.Name != nil && f.Name.Name == targetPackage
}

func collectGoFiles(rootDir, targetPackage string) ([]string, error) {
	var files []string
	matchedCount := 0
	err := filepath.WalkDir(rootDir, func(filePath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			relPath, err := filepath.Rel(rootDir, filePath)
			if err != nil {
				return nil
			}
			if relPath == "." {
				return nil
			}
			topLevelDir := strings.Split(relPath, string(os.PathSeparator))[0]
			if _, excluded := excludedTopLevelDirs[topLevelDir]; excluded {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		if !matchesTargetPackage(filePath, targetPackage) {
			return nil
		}
		relPath, _ := filepath.Rel(rootDir, filePath)
		files = append(files, relPath)
		matchedCount++
		return nil
	})
	if err != nil {
		return nil, err
	}
	if matchedCount == 0 {
		return nil, fmt.Errorf("no go files found for package %q", targetPackage)
	}
	return files, err
}
