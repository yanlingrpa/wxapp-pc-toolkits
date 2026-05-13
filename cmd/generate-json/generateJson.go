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
	"cmd":       {},
	"doc":       {},
	"tests":     {},
	"symbols":   {},
	"schema":    {},
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

func main() {
	rootDir, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to locate project root: %v\n", err)
		os.Exit(1)
	}

	moduleName, err := parseModuleName(filepath.Join(rootDir, "go.mod"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse module name: %v\n", err)
		os.Exit(1)
	}

	packages, err := scanPackages(rootDir, moduleName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to scan packages: %v\n", err)
		os.Exit(1)
	}

	generatedAt := time.Now().UTC().Format(time.RFC3339)
	moduleDoc, symbolsDoc, packageDocs := buildOutputs(moduleName, packages, generatedAt)

	outputDir := filepath.Join(rootDir, ".yanling")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	if err := cleanupOutputDir(outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to cleanup output directory: %v\n", err)
		os.Exit(1)
	}

	if err := writeJSON(filepath.Join(outputDir, "module.json"), moduleDoc); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write module.json: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSON(filepath.Join(outputDir, "symbols.json"), symbolsDoc); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write symbols.json: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSON(filepath.Join(outputDir, "symbols.lite.json"), buildSymbolsLite(symbolsDoc)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write symbols.lite.json: %v\n", err)
		os.Exit(1)
	}

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

	fmt.Printf("generated %s\n", filepath.Join(outputDir, "module.json"))
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "symbols.json"))
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "symbols.lite.json"))
	fmt.Printf("generated %s\n", packagesDir)

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

	indexOutput := buildIndexOutput(moduleDoc, symbolsDoc, topicDocs)
	if err := writeJSON(filepath.Join(outputDir, "index.json"), indexOutput); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write index.json: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "index.json"))
}
