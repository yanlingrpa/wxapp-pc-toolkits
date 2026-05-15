package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func scanProject(rootDir string, fset *token.FileSet) ([]*fileInfo, map[string]*structDef, []validationError, error) {
	files := make([]*fileInfo, 0)
	structs := make(map[string]*structDef)
	errs := make([]validationError, 0)

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
			errs = append(errs, validationError{
				File: relPath,
				Msg:  fmt.Sprintf("failed to parse file: %v", err),
			})
			return nil
		}

		fi := &fileInfo{
			AbsPath: filePath,
			RelPath: relPath,
			Ast:     fileAst,
			Imports: buildImportMap(fileAst),
		}
		files = append(files, fi)

		for _, decl := range fileAst.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				if _, exists := structs[ts.Name.Name]; exists {
					pos := fset.Position(ts.Pos())
					errs = append(errs, validationError{
						File: relPath,
						Line: pos.Line,
						Msg:  fmt.Sprintf("duplicate struct name %q", ts.Name.Name),
					})
					continue
				}
				pos := fset.Position(ts.Pos())
				structs[ts.Name.Name] = &structDef{
					Name: ts.Name.Name,
					File: relPath,
					Line: pos.Line,
					Node: st,
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, nil, nil, err
	}

	if len(files) == 0 {
		errs = append(errs, validationError{Msg: "no Go files found after directory exclusions"})
	}

	return files, structs, errs, nil
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
		alias := filepath.Base(importPath)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		imports[alias] = importPath
	}
	return imports
}
