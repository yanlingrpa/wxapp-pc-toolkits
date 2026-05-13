package main

import (
	"bytes"
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

type ExportedStruct struct {
	Name    string
	Code    string
	Source  string // "method" or "publish" or "recursive"
	ModPath string
}

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

func extractMethodStructs(rootDir, targetPackage string) (map[string]string, error) {
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
		if !matchesTargetPackage(filePath, targetPackage) {
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

func extractPublishStructs(rootDir, targetPackage string) (map[string]string, error) {
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
		if !matchesTargetPackage(filePath, targetPackage) {
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
		if !matchesTargetPackage(filePath, targetPackage) {
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
	buf.WriteString("package main\n\n")

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
