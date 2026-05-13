package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// buildSymbolIndex creates a qualified-name -> *SymbolDoc map for fast type lookup.
func buildSymbolIndex(packages []*packageAggregate) map[string]*SymbolDoc {
	index := make(map[string]*SymbolDoc)
	for _, pkg := range packages {
		for _, symbol := range pkg.Symbols {
			index[symbol.QualifiedName] = symbol
		}
	}
	return index
}

// scanTopics walks the source tree looking for .Publish(topic, payload) calls,
// deduplicates by topic name, extracts preceding doc comments and payload struct
// types, and returns a sorted slice of TopicDoc entries.
func scanTopics(rootDir, moduleName string, symbolIndex map[string]*SymbolDoc) ([]TopicDoc, error) {
	fset := token.NewFileSet()

	type rawTopic struct {
		doc               string
		payloadTypeName   string
		payloadImportPath string
	}
	seen := make(map[string]*rawTopic)

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

		fileAst, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return nil // skip files with parse errors
		}

		imports := buildImportMap(fileAst)

		relPath, _ := filepath.Rel(rootDir, filePath)
		relPath = filepath.ToSlash(relPath)
		relDir := filepath.ToSlash(filepath.Dir(relPath))
		if relDir == "." {
			relDir = ""
		}
		pkgImportPath := moduleName
		if relDir != "" {
			pkgImportPath = moduleName + "/" + relDir
		}

		// Map: comment-group end line -> cleaned comment text.
		// Used to locate the doc comment immediately preceding a Publish call.
		commentByEndLine := make(map[int]string)
		for _, cg := range fileAst.Comments {
			endLine := fset.Position(cg.End()).Line
			commentByEndLine[endLine] = cleanComment(cg.Text())
		}

		ast.Inspect(fileAst, func(n ast.Node) bool {
			callExpr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := callExpr.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Publish" {
				return true
			}
			if len(callExpr.Args) != 2 {
				return true
			}
			topicLit, ok := callExpr.Args[0].(*ast.BasicLit)
			if !ok || topicLit.Kind != token.STRING {
				return true
			}
			topic := strings.Trim(topicLit.Value, `"`)
			if topic == "" {
				return true
			}

			callLine := fset.Position(callExpr.Pos()).Line
			// Search for a comment ending 1-3 lines before the call.
			doc := ""
			for delta := 1; delta <= 3; delta++ {
				if c, found := commentByEndLine[callLine-delta]; found {
					doc = c
					break
				}
			}

			typeName, typeImportPath := resolvePayloadType(callExpr.Args[1], imports, pkgImportPath)

			if existing, exists := seen[topic]; !exists {
				seen[topic] = &rawTopic{
					doc:               doc,
					payloadTypeName:   typeName,
					payloadImportPath: typeImportPath,
				}
			} else {
				if existing.doc == "" && doc != "" {
					existing.doc = doc
				}
				if existing.payloadTypeName == "" && typeName != "" {
					existing.payloadTypeName = typeName
					existing.payloadImportPath = typeImportPath
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	topics := make([]TopicDoc, 0, len(seen))
	for topicName, raw := range seen {
		payload := buildTopicPayload(raw.payloadTypeName, raw.payloadImportPath, symbolIndex)
		entry := TopicDoc{
			Name:      topicName,
			Specifier: moduleName,
			Doc:       raw.doc,
			Direction: "publish",
			Payload:   payload,
		}
		if raw.payloadTypeName != "" && ast.IsExported(raw.payloadTypeName) {
			entry.GoStructName = raw.payloadTypeName
			entry.GoImportPath = raw.payloadImportPath
		}
		topics = append(topics, entry)
	}
	sort.Slice(topics, func(i, j int) bool {
		return topics[i].Name < topics[j].Name
	})
	return topics, nil
}

// resolvePayloadType extracts the struct type name and its import path from a
// Publish payload expression (typically a composite literal).
func resolvePayloadType(expr ast.Expr, imports map[string]string, localImportPath string) (string, string) {
	switch t := expr.(type) {
	case *ast.CompositeLit:
		if t.Type != nil {
			return resolvePayloadType(t.Type, imports, localImportPath)
		}
	case *ast.Ident:
		return t.Name, localImportPath
	case *ast.SelectorExpr:
		if pkgIdent, ok := t.X.(*ast.Ident); ok {
			if importPath, found := imports[pkgIdent.Name]; found {
				return t.Sel.Name, importPath
			}
		}
	case *ast.StarExpr:
		return resolvePayloadType(t.X, imports, localImportPath)
	case *ast.UnaryExpr:
		return resolvePayloadType(t.X, imports, localImportPath)
	}
	return "", ""
}
