package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func findProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}

		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("go.mod not found")
		}
		wd = parent
	}
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

func scanPackages(rootDir, moduleName string) ([]*packageAggregate, error) {
	fset := token.NewFileSet()
	packages := make(map[string]*packageAggregate)

	err := filepath.WalkDir(rootDir, func(filePath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if shouldSkipDir(rootDir, filePath, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(rootDir, filePath)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		fileAst, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relPath, err)
		}

		relDir := filepath.ToSlash(filepath.Dir(relPath))
		if relDir == "." {
			relDir = ""
		}
		importPath := moduleName
		if relDir != "" {
			importPath = moduleName + "/" + relDir
		}

		pkg := packages[importPath]
		if pkg == nil {
			pkg = &packageAggregate{
				Name:           fileAst.Name.Name,
				ImportPath:     importPath,
				RelDir:         relDir,
				PackageImports: make(map[string]map[string]struct{}),
				Symbols:        make(map[string]*SymbolDoc),
				PendingMethods: make(map[string][]MethodDoc),
			}
			packages[importPath] = pkg
		}
		pkg.Files = append(pkg.Files, relPath)
		if pkg.Doc == "" {
			pkg.Doc = cleanComment(pickDoc(fileAst.Doc, nil))
		}

		ctx := fileContext{
			RelPath: relPath,
			Imports: buildImportMap(fileAst),
		}
		mergePackageImports(pkg, ctx.Imports)
		extractDeclarations(pkg, fileAst, ctx, fset)
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]*packageAggregate, 0, len(packages))
	for _, pkg := range packages {
		finalizePackage(pkg)
		result = append(result, pkg)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ImportPath < result[j].ImportPath
	})
	return result, nil
}

func shouldSkipDir(rootDir, dirPath, name string) bool {
	if dirPath == rootDir {
		return false
	}

	if name == ".git" || strings.HasPrefix(name, ".") {
		return true
	}

	relPath, err := filepath.Rel(rootDir, dirPath)
	if err != nil {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	if relPath == "." {
		return false
	}

	topLevelDir := strings.Split(relPath, "/")[0]
	_, excluded := excludedTopLevelDirs[topLevelDir]
	return excluded
}

func buildImportMap(fileAst *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, spec := range fileAst.Imports {
		importPath := strings.Trim(spec.Path.Value, "\"")
		alias := path.Base(importPath)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		imports[alias] = importPath
	}
	return imports
}

func mergePackageImports(pkg *packageAggregate, imports map[string]string) {
	for alias, importPath := range imports {
		aliases := pkg.PackageImports[importPath]
		if aliases == nil {
			aliases = make(map[string]struct{})
			pkg.PackageImports[importPath] = aliases
		}
		aliases[alias] = struct{}{}
	}
}
