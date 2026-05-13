package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

type ExportedStruct struct {
	Name    string
	Code    string
	Source  string // "method" or "publish"
	ModPath string
}

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
								var structBuf bytes.Buffer
								fmt.Fprintf(&structBuf, "type ")
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

func collectGoFiles(rootDir string) ([]string, error) {
	var files []string
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
		relPath, _ := filepath.Rel(rootDir, filePath)
		files = append(files, relPath)
		return nil
	})
	return files, err
}

func mergeGoFiles(rootDir string, goFiles []string, moduleName string) (string, error) {
	fset := token.NewFileSet()
	var buf bytes.Buffer

	buf.WriteString("package yscript\n\n")

	// Collect all imports
	allImports := make(map[string]bool)

	// First pass: collect imports
	for _, file := range goFiles {
		filePath := filepath.Join(rootDir, file)
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			allImports[importPath] = true
		}
	}

	// Write imports
	if len(allImports) > 0 {
		buf.WriteString("import (\n")
		var importList []string
		for imp := range allImports {
			importList = append(importList, imp)
		}
		sort.Strings(importList)
		for _, imp := range importList {
			fmt.Fprintf(&buf, "\t%q\n", imp)
		}
		buf.WriteString(")\n\n")
	}

	// Second pass: merge declarations
	for _, file := range goFiles {
		filePath := filepath.Join(rootDir, file)
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			continue
		}

		buf.WriteString("// File: " + file + "\n")
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok == token.IMPORT {
					continue
				}
			}
			var declBuf bytes.Buffer
			printer.Fprint(&declBuf, fset, decl)
			buf.WriteString(declBuf.String())
			buf.WriteString("\n\n")
		}
	}

	return buf.String(), nil
}

// collectStructDependencies 递归收集 struct 依赖
func collectStructDependencies(
	structName string,
	structDefs map[string]string,
	collected map[string]string,
	astStructs map[string]*ast.StructType,
) {
	if _, exists := collected[structName]; exists {
		return
	}
	code, ok := structDefs[structName]
	if !ok {
		return
	}
	collected[structName] = code

	st, ok := astStructs[structName]
	if !ok {
		return
	}
	for _, field := range st.Fields.List {
		typeName := extractStructName(field.Type)
		if typeName != "" && isExportedType(typeName) {
			collectStructDependencies(typeName, structDefs, collected, astStructs)
		}
	}
}

// extractMethodStructs 递归收集所有依赖 struct
func extractMethodStructs(rootDir string) (map[string]string, error) {
	fset := token.NewFileSet()
	structDefs := make(map[string]string)
	astStructs := make(map[string]*ast.StructType)
	usedStructs := make(map[string]struct{})

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

		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		// Collect struct definitions in this file
		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							if ts.Name.IsExported() {
								var structBuf bytes.Buffer
								fmt.Fprintf(&structBuf, "type ")
								printer.Fprint(&structBuf, fset, ts)
								structDefs[ts.Name.Name] = structBuf.String()
								astStructs[ts.Name.Name] = st
							}
						}
					}
				}
			}
		}

		// Find methods with signature func(rt script.ModuleRuntime, param StructType)
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok {
				if !fd.Name.IsExported() {
					continue
				}
				if fd.Type.Params == nil || len(fd.Type.Params.List) != 2 {
					continue
				}
				firstParam := fd.Type.Params.List[0]
				if !isModuleRuntimeType(firstParam.Type) {
					continue
				}
				secondParam := fd.Type.Params.List[1]
				if len(secondParam.Names) == 0 {
					continue
				}
				structName := extractStructName(secondParam.Type)
				if structName != "" && isExportedType(structName) {
					usedStructs[structName] = struct{}{}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 递归收集所有依赖 struct
	collected := make(map[string]string)
	for name := range usedStructs {
		collectStructDependencies(name, structDefs, collected, astStructs)
	}
	return collected, nil
}

// extractPublishStructs 递归收集所有依赖 struct
func extractPublishStructs(rootDir string) (map[string]string, error) {
	fset := token.NewFileSet()
	structDefs := make(map[string]string)
	astStructs := make(map[string]*ast.StructType)
	usedStructs := make(map[string]struct{})

	// First pass: collect all struct definitions
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
								var structBuf bytes.Buffer
								fmt.Fprintf(&structBuf, "type ")
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
	if err != nil {
		return nil, err
	}

	// Second pass: find rt.Publish calls inside functions where rt is script.ModuleRuntime
	err = filepath.WalkDir(rootDir, func(filePath string, d os.DirEntry, walkErr error) error {
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
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			// Find name of the script.ModuleRuntime parameter
			rtName := ""
			if fd.Type.Params != nil {
				for _, param := range fd.Type.Params.List {
					if isModuleRuntimeType(param.Type) && len(param.Names) > 0 {
						rtName = param.Names[0].Name
						break
					}
				}
			}
			if rtName == "" {
				continue
			}
			// Inspect function body for rtName.Publish(...) calls
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Publish" {
					return true
				}
				recv, ok := sel.X.(*ast.Ident)
				if !ok || recv.Name != rtName {
					return true
				}
				if len(call.Args) >= 2 {
					payloadArg := call.Args[1]
					if comp, ok := payloadArg.(*ast.CompositeLit); ok {
						structName := extractStructName(comp.Type)
						if structName != "" && isExportedType(structName) {
							usedStructs[structName] = struct{}{}
						}
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 递归收集所有依赖 struct
	collected := make(map[string]string)
	for name := range usedStructs {
		collectStructDependencies(name, structDefs, collected, astStructs)
	}
	return collected, nil
}

func generateExportGo(moduleName string, exportedStructs map[string]*ExportedStruct) string {
	var buf bytes.Buffer
	buf.WriteString("package yscript\n\n")

	// Sort struct names for consistent output
	var names []string
	for name := range exportedStructs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		s := exportedStructs[name]
		newName := renameStruct(moduleName, name)

		// Replace struct name in code with regex to avoid partial matches
		code := s.Code
		// Pattern: type OLDNAME followed by space or { or newline
		pattern := regexp.MustCompile(`\btype\s+` + regexp.QuoteMeta(name) + `\b`)
		code = pattern.ReplaceAllString(code, "type "+newName)

		fmt.Fprintf(&buf, "// %s (%s)\n", name, s.Source)
		buf.WriteString(code)
		buf.WriteString("\n\n")
	}

	return buf.String()
}

func renameStruct(moduleName, structName string) string {
	// Replace / with __, other special chars with _
	prefix := strings.ReplaceAll(moduleName, "/", "__")
	prefix = regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(prefix, "_")

	newName := prefix + "__" + structName
	// Ensure first letter is uppercase
	if len(newName) > 0 {
		if newName[0] >= 'a' && newName[0] <= 'z' {
			newName = strings.ToUpper(string(newName[0])) + newName[1:]
		}
	}
	return newName
}

func isModuleRuntimeType(expr ast.Expr) bool {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if ident, ok := sel.X.(*ast.Ident); ok {
			return ident.Name == "script" && sel.Sel.Name == "ModuleRuntime"
		}
	}
	return false
}

func extractStructName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return extractStructName(t.X)
	case *ast.CompositeLit:
		return extractStructName(t.Type)
	}
	return ""
}

func isExportedType(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}
