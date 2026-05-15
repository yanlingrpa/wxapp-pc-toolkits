package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	// 获取项目根目录（使用当前工作目录）
	rootDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get current working directory: %v\n", err)
		os.Exit(1)
	}

	// Step 1: 读取并解析 go.mod
	goModPath := filepath.Join(rootDir, "go.mod")
	modInfo, err := parseGoMod(goModPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse go.mod: %v\n", err)
		os.Exit(1)
	}

	// Step 2: 验证 go 版本
	if err := validateGoVersion(modInfo.GoVersion); err != nil {
		fmt.Fprintf(os.Stderr, "invalid go version: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Go version: %s (valid)\n", modInfo.GoVersion)

	// Step 3: 获取 GOMODCACHE 路径
	goModCache := getGoModCache()
	fmt.Printf("✓ GOMODCACHE: %s\n", goModCache)

	// Step 4: 获取所有依赖（直接和间接）
	allDeps, err := getAllDependencies(goModPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get dependencies: %v\n", err)
		os.Exit(1)
	}

	// Step 5: 检验每个依赖是否符合 yanling script module 标准
	if len(allDeps) == 0 {
		fmt.Println("No dependencies found.")
		os.Exit(0)
	}

	fmt.Printf("\nChecking %d dependencies for yanling script module compliance...\n\n", len(allDeps))

	invalidCount := 0
	for module, version := range allDeps {
		if err := checkYanlingModule(goModCache, module, version); err != nil {
			fmt.Printf("✗ %s@%s: %v\n", module, version, err)
			invalidCount++
		} else {
			fmt.Printf("✓ %s@%s\n", module, version)
		}
	}

	if invalidCount > 0 {
		fmt.Printf("\n%d invalid module(s) found.\n", invalidCount)
		os.Exit(1)
	}
	fmt.Println("\nAll dependencies are valid yanling script modules.")
	os.Exit(0)
}
