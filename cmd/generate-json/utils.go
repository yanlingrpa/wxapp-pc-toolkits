package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}

	switch t := recv.List[0].Type.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	return nodeToString(token.NewFileSet(), expr)
}

func nodeToString(fset *token.FileSet, node any) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, node); err != nil {
		return ""
	}
	return buf.String()
}

func embeddedFieldName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.StarExpr:
		return embeddedFieldName(t.X)
	default:
		return ""
	}
}

func isExportedEmbedded(expr ast.Expr) bool {
	name := embeddedFieldName(expr)
	if name == "" {
		return false
	}
	return ast.IsExported(name)
}

func pickDoc(primary, fallback *ast.CommentGroup) string {
	if primary != nil {
		return primary.Text()
	}
	if fallback != nil {
		return fallback.Text()
	}
	return ""
}

func cleanComment(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "//"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if line != "" {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func sortMethods(methods []MethodDoc) {
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Name < methods[j].Name
	})
}

func sortFields(fields []FieldDoc) {
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Name == fields[j].Name {
			return fields[i].Type < fields[j].Type
		}
		return fields[i].Name < fields[j].Name
	})
}

func sortEmbeddedTypes(items []EmbeddedTypeDoc) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type == items[j].Type {
			return items[i].QualifiedName < items[j].QualifiedName
		}
		return items[i].Type < items[j].Type
	})
}

func dedupeTypeRefs(items []TypeRefDoc) []TypeRefDoc {
	unique := make(map[string]TypeRefDoc)
	for _, item := range items {
		unique[typeRefKey(item)] = item
	}
	result := make([]TypeRefDoc, 0, len(unique))
	for _, item := range unique {
		result = append(result, item)
	}
	return sortTypeRefs(result)
}

func sortTypeRefs(items []TypeRefDoc) []TypeRefDoc {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Expr == items[j].Expr {
			return items[i].QualifiedName < items[j].QualifiedName
		}
		return items[i].Expr < items[j].Expr
	})
	return items
}

func typeRefKey(item TypeRefDoc) string {
	return item.Expr + "|" + item.ImportPath + "|" + item.QualifiedName
}

func qualifiedName(importPath, name string) string {
	return importPath + "." + name
}

func symbolID(importPath, name string) string {
	return importPath + "#" + name
}

func sourceForNode(fset *token.FileSet, relPath string, pos token.Pos) SourceDoc {
	position := fset.Position(pos)
	return SourceDoc{
		File:   relPath,
		Line:   position.Line,
		Column: position.Column,
	}
}

func packageDirectory(relDir string) string {
	if relDir == "" {
		return "/"
	}
	return "/" + relDir
}

func packageFileName(importPath string) string {
	return strings.ReplaceAll(importPath, "/", "__") + ".json"
}

func cleanupOutputDir(outputDir string) error {
	paths := []string{
		filepath.Join(outputDir, "api"),
		filepath.Join(outputDir, "packages"),
		filepath.Join(outputDir, "api.json"),
		filepath.Join(outputDir, "struct.json"),
		filepath.Join(outputDir, "info.md"),
		filepath.Join(outputDir, "module.json"),
		filepath.Join(outputDir, "symbols.json"),
		filepath.Join(outputDir, "symbols.lite.json"),
		filepath.Join(outputDir, "topics.json"),
		filepath.Join(outputDir, "index.json"),
	}
	for _, item := range paths {
		if err := os.RemoveAll(item); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(path string, data any) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(path, content, 0644)
}

func oneLineDoc(text string) string {
	if text == "" {
		return ""
	}
	parts := strings.Split(text, "\n")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
