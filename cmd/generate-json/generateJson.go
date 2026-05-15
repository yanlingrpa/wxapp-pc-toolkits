package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const schemaVersion = "yanling.machine-first/v1"

const (
	moduleSchemaRef  = "./schema/yanling.machine-first.v1/module.schema.json"
	symbolsSchemaRef = "./schema/yanling.machine-first.v1/symbols.schema.json"
	packageSchemaRef = "./../schema/yanling.machine-first.v1/package.schema.json"
	topicsSchemaRef  = "./schema/yanling.machine-first.v1/topics.schema.json"
	indexSchemaRef   = "./schema/yanling.machine-first.v1/index.schema.json"
)

var excludedTopLevelDirs = map[string]struct{}{
	".git":      {},
	".vscode":   {},
	".protocol": {},
	".yanling":  {},
	"assets":    {},
	"bin":       {},
	"build":     {},
	"cmd":       {},
	"debug":     {},
	"dist":      {},
	"doc":       {},
	"docs":      {},
	"examples":  {},
	"internal":  {},
	"scripts":   {},
	"symbol":    {},
	"symbols":   {},
	"schema":    {},
	"schemas":   {},
	"testdata":  {},
	"test":      {},
	"tests":     {},
	"vendor":    {},
}

var builtinTypes = map[string]struct{}{
	"any":         {},
	"bool":        {},
	"byte":        {},
	"comparable":  {},
	"complex64":   {},
	"complex128":  {},
	"error":       {},
	"float32":     {},
	"float64":     {},
	"int":         {},
	"int8":        {},
	"int16":       {},
	"int32":       {},
	"int64":       {},
	"interface{}": {},
	"rune":        {},
	"string":      {},
	"uint":        {},
	"uint8":       {},
	"uint16":      {},
	"uint32":      {},
	"uint64":      {},
	"uintptr":     {},
}

// main 是 machine-first 索引生成工具的主入口，负责 orchestrate 整个生成流程。
// 步骤详解：
// 1. 定位项目根目录（findProjectRoot），以便后续所有操作基于统一根路径。
// 2. 解析 go.mod 获取当前 module 名称。
// 3. 扫描所有包，收集包信息、符号信息。
// 4. 生成 module、symbols、package 等 machine-first 规范的 JSON 文件。
// 5. 清理 .yanling 目录下旧的输出文件，准备写入新文件。
// 6. 写入 module.json、symbols.json、symbols.lite.json。
// 7. 为每个包写入独立的 package 详情文件。
// 8. 构建符号索引，扫描所有 topics，生成 topics.json。
// 9. 汇总所有信息，生成总索引 index.json。
// 10. 每步出错均会输出错误信息并退出。
func main() {
	// 1. 定位项目根目录（支持多种启动路径）
	rootDir, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to locate project root: %v\n", err)
		os.Exit(1)
	}

	// 2. 解析 go.mod 获取 module 名称
	moduleName, err := parseModuleName(filepath.Join(rootDir, "go.mod"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse module name: %v\n", err)
		os.Exit(1)
	}

	// 3. 扫描所有包，收集包、符号等信息
	packages, err := scanPackages(rootDir, moduleName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to scan packages: %v\n", err)
		os.Exit(1)
	}

	// 4. 生成 module、symbols、package 等 machine-first 规范的 JSON 文档
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	moduleDoc, symbolsDoc, packageDocs := buildOutputs(moduleName, packages, generatedAt)

	// 5. 创建 .yanling 输出目录
	outputDir := filepath.Join(rootDir, ".yanling")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// 6. 清理 .yanling 目录下旧的输出文件（避免残留）
	if err := cleanupOutputDir(outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to cleanup output directory: %v\n", err)
		os.Exit(1)
	}

	// 7. 写入 module.json
	if err := writeJSON(filepath.Join(outputDir, "module.json"), moduleDoc); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write module.json: %v\n", err)
		os.Exit(1)
	}
	// 写入 symbols.json
	if err := writeJSON(filepath.Join(outputDir, "symbols.json"), symbolsDoc); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write symbols.json: %v\n", err)
		os.Exit(1)
	}
	// 写入 symbols.lite.json（精简符号索引）
	if err := writeJSON(filepath.Join(outputDir, "symbols.lite.json"), buildSymbolsLite(symbolsDoc)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write symbols.lite.json: %v\n", err)
		os.Exit(1)
	}

	// 8. 为每个包写入独立的 package 详情文件
	packagesDir := filepath.Join(outputDir, "packages")
	if err := os.MkdirAll(packagesDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create packages output directory: %v\n", err)
		os.Exit(1)
	}
	for _, pkg := range packageDocs {
		if err := writeJSON(filepath.Join(packagesDir, packageFileName(pkg.Package.ImportPath)), pkg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write package file for %s: %v\n", pkg.Package.ImportPath, err)
			os.Exit(1)
		}
	}

	// 9. 打印生成的主要文件路径
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "module.json"))
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "symbols.json"))
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "symbols.lite.json"))
	fmt.Printf("generated %s\n", packagesDir)

	// 10. 构建符号索引，扫描所有 topics，生成 topics.json
	symbolIndex := buildSymbolIndex(packages)
	topicDocs, err := scanTopics(rootDir, moduleName, symbolIndex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to scan topics: %v\n", err)
		os.Exit(1)
	}
	topicsOutput := TopicsOutput{
		SchemaRef:     topicsSchemaRef,
		SchemaVersion: schemaVersion,
		GeneratedAt:   generatedAt,
		Module:        moduleName,
		Topics:        topicDocs,
	}
	if err := writeJSON(filepath.Join(outputDir, "topics.json"), topicsOutput); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write topics.json: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "topics.json"))

	// 11. 汇总所有信息，生成总索引 index.json
	indexOutput := buildIndexOutput(moduleDoc, symbolsDoc, topicDocs)
	if err := writeJSON(filepath.Join(outputDir, "index.json"), indexOutput); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write index.json: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "index.json"))
}
