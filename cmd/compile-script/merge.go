package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

func mergeGoFiles(rootDir string, goFiles []string, moduleName string) (string, error) {
	fset := token.NewFileSet()
	var buf bytes.Buffer

	buf.WriteString("package main\n\n")

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
			buf.WriteString("\t\"" + imp + "\"\n")
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
