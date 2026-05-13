// mainImpl 拆分后的主流程
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <root_dir>\n", os.Args[0])
		os.Exit(1)
	}

	rootDir := os.Args[1]

	// Step 1: Get module name
	moduleName, err := parseModuleName(filepath.Join(rootDir, "go.mod"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse module name: %v\n", err)
		os.Exit(1)
	}

	// Step 2: Collect all go files (excluding excludedTopLevelDirs)
	goFiles, err := collectGoFiles(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to collect go files: %v\n", err)
		os.Exit(1)
	}

	// Step 3: Merge go files into script.go
	scriptContent, err := mergeGoFiles(rootDir, goFiles, moduleName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to merge go files: %v\n", err)
		os.Exit(1)
	}

	// Step 4: Extract exported structs (collect all, mark来源)
	exportedStructs := make(map[string]*ExportedStruct)

	methodStructs, err := extractMethodStructs(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract method structs: %v\n", err)
		os.Exit(1)
	}
	for name, s := range methodStructs {
		exportedStructs[name] = &ExportedStruct{
			Name:    name,
			Code:    s,
			Source:  "method",
			ModPath: moduleName,
		}
	}

	publishStructs, err := extractPublishStructs(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract publish structs: %v\n", err)
		os.Exit(1)
	}
	for name, s := range publishStructs {
		if _, exists := exportedStructs[name]; !exists {
			exportedStructs[name] = &ExportedStruct{
				Name:    name,
				Code:    s,
				Source:  "publish",
				ModPath: moduleName,
			}
		}
	}

	// Step 5: 递归收集所有 struct（包括子 struct），并全部重命名输出
	// 1. 合并所有 struct 名和代码
	allStructs := make(map[string]*ExportedStruct)
	for name, s := range exportedStructs {
		allStructs[name] = s
	}
	// 2. 递归收集所有依赖 struct
	// 2.1 收集所有 struct 定义和 ast
	fset := token.NewFileSet()
	structDefs := make(map[string]string)
	astStructs := make(map[string]*ast.StructType)
	_ = filepath.WalkDir(rootDir, func(filePath string, d os.DirEntry, walkErr error) error {
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
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return nil
		}
		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							if ts.Name.IsExported() {
								var structBuf strings.Builder
								fmt.Fprintf(&structBuf, "type ")
								// Use printer.Fprint for ast.TypeSpec
								printer.Fprint(&structBuf, fset, ts)
								structDefs[ts.Name.Name] = structBuf.String()
								astStructs[ts.Name.Name] = st
							}
						}
					}
				}
			}
		}
		return nil
	})
	// 2.2 递归收集所有依赖 struct
	collected := make(map[string]string)
	for name := range allStructs {
		collectStructDependencies(name, structDefs, collected, astStructs)
	}
	// 2.3 生成新的 allStructs，全部来源标记为 "recursive"
	for name, code := range collected {
		if _, exists := allStructs[name]; !exists {
			allStructs[name] = &ExportedStruct{
				Name:    name,
				Code:    code,
				Source:  "recursive",
				ModPath: moduleName,
			}
		}
	}

	// Step 6: Generate export.go with renamed structs (全部 struct 都重命名)
	exportContent := generateExportGo(moduleName, allStructs)

	// Write files
	outputDir := filepath.Join(rootDir, ".yanling")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "script.go"), []byte(scriptContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write script.go: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "export.go"), []byte(exportContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write export.go: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("generated %s\n", filepath.Join(outputDir, "script.go"))
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "export.go"))
}
