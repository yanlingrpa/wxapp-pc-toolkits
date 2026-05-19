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
	"strings"
)

func loadPackageFunctionSignatures(goModCache string, ii importInfo) (map[string]functionSignature, error) {
	pkgDir := modulePackageDir(goModCache, ii.ModulePath, ii.Version, ii.ImportPath)
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read package dir %s: %w", pkgDir, err)
	}

	fset := token.NewFileSet()
	sigs := make(map[string]functionSignature)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(pkgDir, name)
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Name == nil {
				continue
			}
			sig := parseFunctionSignature(fd)
			sigs[fd.Name.Name] = sig
		}
	}
	return sigs, nil
}

func parseFunctionSignature(fd *ast.FuncDecl) functionSignature {
	if fd.Type == nil || fd.Type.Results == nil || len(fd.Type.Results.List) == 0 {
		return functionSignature{FirstResultKind: resultUnknown}
	}
	first := fd.Type.Results.List[0].Type
	base, pointer := unwrapPointer(first)

	if id, ok := base.(*ast.Ident); ok {
		if isPrimitiveType(id.Name) {
			return functionSignature{FirstResultKind: resultPrimitive, FirstResultType: id.Name}
		}
		if id.Name != "" {
			t := id.Name
			if pointer {
				t = "*" + t
			}
			return functionSignature{FirstResultKind: resultStruct, FirstResultType: t, FirstResultStructName: id.Name}
		}
	}

	if se, ok := base.(*ast.SelectorExpr); ok && se.Sel != nil {
		t := se.Sel.Name
		if pointer {
			t = "*" + t
		}
		return functionSignature{FirstResultKind: resultStruct, FirstResultType: t, FirstResultStructName: se.Sel.Name}
	}

	var b strings.Builder
	_ = printer.Fprint(&b, token.NewFileSet(), first)
	return functionSignature{FirstResultKind: resultUnknown, FirstResultType: b.String()}
}

func unwrapPointer(expr ast.Expr) (ast.Expr, bool) {
	if se, ok := expr.(*ast.StarExpr); ok {
		return se.X, true
	}
	return expr, false
}

func isPrimitiveType(name string) bool {
	switch name {
	case "bool", "string":
		return true
	case "int", "int8", "int16", "int32", "int64":
		return true
	case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	case "byte", "rune", "float32", "float64", "complex64", "complex128":
		return true
	default:
		return false
	}
}

func loadExportTypes(goModCache, modulePath, version string) (map[string]exportTypeDecl, error) {
	exportPath := filepath.Join(moduleCacheModuleDir(goModCache, modulePath, version), ".yanling", "export.go")
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, exportPath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse export.go for %s@%s: %w", modulePath, version, err)
	}

	types := make(map[string]exportTypeDecl)
	for _, decl := range fileNode.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name == nil {
				continue
			}
			var b bytes.Buffer
			b.WriteString("type ")
			if err := printer.Fprint(&b, fset, ts); err != nil {
				continue
			}
			normalized := normalizeTypeCodeWhitespace(b.String())
			types[ts.Name.Name] = exportTypeDecl{Name: ts.Name.Name, Code: normalized}
		}
	}
	return types, nil
}

func normalizeTypeCodeWhitespace(typeCode string) string {
	lines := strings.Split(typeCode, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func moduleCacheModuleDir(goModCache, modulePath, version string) string {
	escaped := escapeModulePathForCache(modulePath)
	return filepath.Join(goModCache, escaped+"@"+version)
}

func modulePackageDir(goModCache, modulePath, version, importPath string) string {
	modDir := moduleCacheModuleDir(goModCache, modulePath, version)
	if importPath == modulePath {
		return modDir
	}
	suffix := strings.TrimPrefix(importPath, modulePath)
	suffix = strings.TrimPrefix(suffix, "/")
	if suffix == "" {
		return modDir
	}
	parts := strings.Split(suffix, "/")
	all := append([]string{modDir}, parts...)
	return filepath.Join(all...)
}

func escapeModulePathForCache(modulePath string) string {
	var b strings.Builder
	for _, r := range modulePath {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune('!')
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func collectImports(fileNode *ast.File, modInfo *goModInfo) (map[string]importInfo, error) {
	result := make(map[string]importInfo)
	for _, imp := range fileNode.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		alias := importAlias(imp)
		ii := importInfo{Alias: alias, ImportPath: path, Keep: isAllowedImport(path)}
		if !ii.Keep {
			modulePath, version, ok := resolveModuleFromImport(path, modInfo.Requires)
			if !ok {
				return nil, fmt.Errorf("cannot resolve module for import: %s", path)
			}
			ii.ModulePath = modulePath
			ii.Version = version
		}
		result[alias] = ii
	}
	return result, nil
}

func resolveModuleFromImport(importPath string, requires map[string]string) (string, string, bool) {
	best := ""
	version := ""
	for module, v := range requires {
		if importPath == module || strings.HasPrefix(importPath, module+"/") {
			if len(module) > len(best) {
				best = module
				version = v
			}
		}
	}
	if best == "" {
		return "", "", false
	}
	return best, version, true
}

func findImportByModule(imports map[string]importInfo, modulePath string) (importInfo, bool) {
	for _, ii := range imports {
		if ii.ModulePath == modulePath {
			return ii, true
		}
	}
	return importInfo{}, false
}

func importAlias(imp *ast.ImportSpec) string {
	if imp.Name != nil && imp.Name.Name != "" {
		return imp.Name.Name
	}
	path := strings.Trim(imp.Path.Value, `"`)
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func parseGoMod(goModPath string) (*goModInfo, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read go.mod: %w", err)
	}

	modInfo := &goModInfo{Requires: make(map[string]string)}
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
				modInfo.Requires[parts[0]] = parts[1]
			}
		}

		if strings.HasPrefix(trimmed, "require ") && !inRequireBlock {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 {
				modInfo.Requires[parts[1]] = parts[2]
			}
		}
	}

	return modInfo, nil
}

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

func isAllowedImport(importPath string) bool {
	if importPath == "yanlingrpa.com/yanling/protocol/script" {
		return true
	}
	first := importPath
	if idx := strings.Index(first, "/"); idx >= 0 {
		first = first[:idx]
	}
	return !strings.Contains(first, ".")
}

func renameStruct(moduleName, structName string) string {
	prefix := strings.ReplaceAll(moduleName, "/", "__")
	prefix = regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(prefix, "_")

	newName := prefix + "__" + structName
	if len(newName) > 0 && newName[0] >= 'a' && newName[0] <= 'z' {
		newName = strings.ToUpper(string(newName[0])) + newName[1:]
	}
	return newName
}
