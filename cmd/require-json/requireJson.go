package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// main 是 require 索引生成命令入口。
//
// 执行流程：
// 1. 定位项目根目录并解析 go.mod 的 require 列表。
// 2. 从 GOMODCACHE 读取每个依赖模块的 .yanling/index.json。
// 3. 将多个依赖索引按模块/包/topic/symbol 去重合并。
// 4. 输出统一的 .yanling/require.json，供 AI 进行跨模块能力检索。
func main() {
	// Step 1: 定位项目根目录（向上查找 go.mod）。
	rootDir, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to locate project root: %v\n", err)
		os.Exit(1)
	}

	// Step 2: 解析 go.mod 中的直接依赖(require)。
	// 这里得到的是 module + version 列表，后续用于拼接 GOMODCACHE 路径。
	goModPath := filepath.Join(rootDir, "go.mod")
	requires, err := parseGoModRequires(goModPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse go.mod requires: %v\n", err)
		os.Exit(1)
	}

	// Step 3: 初始化合并输出对象。
	// generated_at 统一使用 UTC RFC3339，便于后续比较与审计。
	output := IndexOutput{
		SchemaVersion: schemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Modules:       make([]IndexModuleEntry, 0),
		Packages:      make([]IndexPackageEntry, 0),
		Topics:        make([]IndexTopicEntry, 0),
		Symbols:       make([]IndexSymbolEntry, 0),
	}

	// Step 4: 获取模块缓存目录，并准备合并过程的统计与去重集合。
	goModCache := getGoModCache()
	missingCount := 0

	// 去重键用于避免多个依赖索引中出现重复条目。
	seenModules := make(map[string]struct{})
	seenPackages := make(map[string]struct{})
	seenTopics := make(map[string]struct{})
	seenSymbols := make(map[string]struct{})

	// Step 5: 逐个依赖读取其 .yanling/index.json 并合并。
	for _, req := range requires {
		// 缓存目录命名遵循 Go module cache escaped path 规则。
		indexPath := filepath.Join(
			goModCache,
			moduleCachePath(req.Module, req.Version),
			".yanling",
			"index.json",
		)

		indexDoc, err := readIndexFile(indexPath)
		if err != nil {
			// 单个依赖缺失索引不终止整体流程，只记 warning 并继续。
			// 这样即使部分依赖未提供 machine-first 产物，也能生成可用 require.json。
			if errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "warning: missing yanling index for %s@%s (%s)\n", req.Module, req.Version, indexPath)
				missingCount++
				continue
			}
			// 读取失败（例如 JSON 非法）也跳过，避免中断整个聚合任务。
			fmt.Fprintf(os.Stderr, "warning: failed to read index for %s@%s: %v\n", req.Module, req.Version, err)
			continue
		}

		// 合并模块级索引内容（模块、包、topics、symbols）。
		mergeIndex(&output, indexDoc, seenModules, seenPackages, seenTopics, seenSymbols)
	}

	// Step 6: 确保输出目录 .yanling 存在。
	outputDir := filepath.Join(rootDir, ".yanling")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Step 7: 写入最终 require 聚合索引。
	outputPath := filepath.Join(outputDir, "require.json")
	if err := writeJSON(outputPath, output); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write require.json: %v\n", err)
		os.Exit(1)
	}

	// Step 8: 输出执行结果摘要。
	fmt.Printf("generated %s\n", outputPath)
	if missingCount > 0 {
		fmt.Printf("completed with %d missing module indexes\n", missingCount)
	}
}
