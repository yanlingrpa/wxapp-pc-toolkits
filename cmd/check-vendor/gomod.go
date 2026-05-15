package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// goModInfo 表示 go.mod 中的 go 版本信息
type goModInfo struct {
	GoVersion string
	Requires  map[string]string // module -> version
}

// parseGoMod 解析 go.mod 文件
func parseGoMod(goModPath string) (*goModInfo, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read go.mod: %w", err)
	}

	modInfo := &goModInfo{
		Requires: make(map[string]string),
	}

	lines := strings.Split(string(content), "\n")
	inRequireBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 解析 go 版本
		if strings.HasPrefix(trimmed, "go ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				modInfo.GoVersion = parts[1]
			}
			continue
		}

		// 解析 require 块
		if trimmed == "require (" {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && trimmed == ")" {
			inRequireBlock = false
			continue
		}

		// 在 require 块内解析模块
		if inRequireBlock && trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				module := parts[0]
				version := parts[1]
				modInfo.Requires[module] = version
			}
		}

		// 解析单行 require
		if strings.HasPrefix(trimmed, "require ") && !inRequireBlock {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 {
				module := parts[1]
				version := parts[2]
				modInfo.Requires[module] = version
			}
		}
	}

	if modInfo.GoVersion == "" {
		return nil, errors.New("go version not found in go.mod")
	}

	return modInfo, nil
}

// validateGoVersion 验证 go 版本是否为 1.22 及其小版本
func validateGoVersion(version string) error {
	// 匹配版本格式，如 1.22, 1.22.0, 1.22.1 等
	pattern := `^1\.22(\.\d+)?$`
	matched, err := regexp.MatchString(pattern, version)
	if err != nil {
		return err
	}

	if !matched {
		return fmt.Errorf("go version must be 1.22 or 1.22.x, got %s", version)
	}

	return nil
}

// getGoModCache 获取 GOMODCACHE 环境变量或使用默认路径
func getGoModCache() string {
	if goModCache := os.Getenv("GOMODCACHE"); goModCache != "" {
		return goModCache
	}

	// 优先使用 GOPATH
	if goPath := os.Getenv("GOPATH"); goPath != "" {
		return filepath.Join(goPath, "pkg", "mod")
	}

	// 如果 GOPATH 也不存在，使用 ${HOME}/go/pkg/mod
	home, err := os.UserHomeDir()
	if err != nil {
		// 最后的默认值（如果连 home 都获取不了）
		return filepath.Join("go", "pkg", "mod")
	}

	return filepath.Join(home, "go", "pkg", "mod")
}

// getAllDependencies 使用 go list -json ./... 获取所有直接和间接依赖
func getAllDependencies(goModPath string) (map[string]string, error) {
	// 这里使用简单的 go.mod 解析方式
	// 对于获取间接依赖，我们需要解析 go.sum 或运行 go mod graph

	// 首先从 go.mod 中获取所有 require
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}

	deps := make(map[string]string)
	lines := strings.Split(string(content), "\n")
	inRequireBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "require (" {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && trimmed == ")" {
			inRequireBlock = false
			continue
		}

		if inRequireBlock && trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				module := parts[0]
				version := parts[1]
				deps[module] = version
			}
		}

		if strings.HasPrefix(trimmed, "require ") && !inRequireBlock {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 {
				module := parts[1]
				version := parts[2]
				deps[module] = version
			}
		}
	}

	// 尝试从 go.sum 中获取所有间接依赖
	goSumPath := filepath.Join(filepath.Dir(goModPath), "go.sum")
	if sumContent, err := os.ReadFile(goSumPath); err == nil {
		for _, line := range strings.Split(string(sumContent), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "//") {
				continue
			}

			// go.sum 格式: module version hash
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				module := parts[0]
				version := parts[1]
				// 避免重复
				if _, exists := deps[module]; !exists {
					deps[module] = version
				}
			}
		}
	}

	return deps, nil
}
