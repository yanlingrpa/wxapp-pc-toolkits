package main

import (
	"os"
	"path/filepath"
	"strings"
)

func moduleCachePath(moduleName, version string) string {
	return filepath.Join(escapeForModuleCache(moduleName) + "@" + escapeForModuleCache(version))
}

func escapeForModuleCache(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	for _, r := range input {
		switch {
		case r == '!':
			b.WriteString("!!")
		case r >= 'A' && r <= 'Z':
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// getGoModCache 获取 GOMODCACHE 环境变量或使用默认路径
func getGoModCache() string {
	if goModCache := os.Getenv("GOMODCACHE"); goModCache != "" {
		return goModCache
	}

	if goPath := os.Getenv("GOPATH"); goPath != "" {
		return filepath.Join(goPath, "pkg", "mod")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("go", "pkg", "mod")
	}

	return filepath.Join(home, "go", "pkg", "mod")
}
