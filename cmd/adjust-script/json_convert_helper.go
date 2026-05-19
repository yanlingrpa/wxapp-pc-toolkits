package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

func ensureJSONConvertHelper(fileNode *ast.File) error {
	// Remove both old names (backward compat) and new names (idempotency)
	removeFuncDeclByName(fileNode, "newStructPtrByTypeName")
	removeFuncDeclByName(fileNode, "ConvertJSONToStruct")
	removeFuncDeclByName(fileNode, "__yanling_newValuePtrByTypeName")
	removeFuncDeclByName(fileNode, "__yanling_convertJSONToStruct")
	removeFuncDeclByName(fileNode, "__yanling_convertJSONToValue")
	removeFuncDeclByName(fileNode, "Yanling_ConvertJSONToStruct")

	typeNames := collectStructTypeNames(fileNode)

	ensureImportPath(fileNode, "encoding/json")
	ensureImportPath(fileNode, "reflect")
	ensureImportPath(fileNode, "fmt")

	newPtrDecl, err := parseFuncDecl(buildNewValuePtrByTypeNameSource(typeNames))
	if err != nil {
		return err
	}
	convertDecl, err := parseFuncDecl(buildConvertJSONToValueSource())
	if err != nil {
		return err
	}

	fileNode.Decls = append(fileNode.Decls, newPtrDecl, convertDecl)
	return nil
}

func removeFuncDeclByName(fileNode *ast.File, name string) {
	newDecls := make([]ast.Decl, 0, len(fileNode.Decls))
	for _, decl := range fileNode.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if ok && fd.Name != nil && fd.Name.Name == name {
			continue
		}
		newDecls = append(newDecls, decl)
	}
	fileNode.Decls = newDecls
}

func collectStructTypeNames(fileNode *ast.File) []string {
	names := make([]string, 0)
	seen := make(map[string]struct{})
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
			if !ast.IsExported(ts.Name.Name) {
				continue
			}
			if _, ok := ts.Type.(*ast.StructType); !ok {
				continue
			}
			if _, exists := seen[ts.Name.Name]; exists {
				continue
			}
			seen[ts.Name.Name] = struct{}{}
			names = append(names, ts.Name.Name)
		}
	}
	sort.Strings(names)
	return names
}

func ensureImportPath(fileNode *ast.File, importPath string) {
	for _, imp := range fileNode.Imports {
		if strings.Trim(imp.Path.Value, "\"") == importPath {
			return
		}
	}

	newSpec := &ast.ImportSpec{
		Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", importPath)},
	}

	for _, decl := range fileNode.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if ok && gd.Tok == token.IMPORT {
			gd.Specs = append(gd.Specs, newSpec)
			fileNode.Imports = append(fileNode.Imports, newSpec)
			return
		}
	}

	importDecl := &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{newSpec}}
	fileNode.Decls = append([]ast.Decl{importDecl}, fileNode.Decls...)
	fileNode.Imports = append(fileNode.Imports, newSpec)
}

func buildNewValuePtrByTypeNameSource(typeNames []string) string {
	var b strings.Builder
	b.WriteString("func __yanling_newValuePtrByTypeName(typeName string) (any, error) {\n")
	b.WriteString("\tswitch typeName {\n")
	for _, primitiveName := range []string{"bool", "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte", "rune", "float32", "float64"} {
		b.WriteString("\tcase \"")
		b.WriteString(primitiveName)
		b.WriteString("\":\n")
		b.WriteString("\t\treturn new(")
		b.WriteString(primitiveName)
		b.WriteString("), nil\n")
	}
	for _, name := range typeNames {
		b.WriteString("\tcase \"")
		b.WriteString(name)
		b.WriteString("\":\n")
		b.WriteString("\t\treturn &")
		b.WriteString(name)
		b.WriteString("{}, nil\n")
	}
	b.WriteString("\tdefault:\n")
	b.WriteString("\t\treturn nil, fmt.Errorf(\"unsupported targetType: %s\", typeName)\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

func buildConvertJSONToValueSource() string {
	return `func __yanling_convertJSONToValue(jsonStr string, targetType string, ref bool) (result any, err error) {
	ptr, err := __yanling_newValuePtrByTypeName(targetType)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(jsonStr), ptr); err != nil {
		return nil, fmt.Errorf("unmarshal %s failed: %w", targetType, err)
	}

	if ref {
		return ptr, nil
	}

	v := reflect.ValueOf(ptr)
	if v.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("internal error: target is not pointer for %s", targetType)
	}
	return v.Elem().Interface(), nil
}
`
}

func parseFuncDecl(funcSrc string) (*ast.FuncDecl, error) {
	src := "package main\n\n" + funcSrc
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, "inline_func.go", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated function: %w", err)
	}
	if len(fileNode.Decls) == 0 {
		return nil, fmt.Errorf("generated function declaration is empty")
	}
	fd, ok := fileNode.Decls[0].(*ast.FuncDecl)
	if !ok {
		return nil, fmt.Errorf("generated declaration is not a function")
	}
	return fd, nil
}

func regenerateHelperFunctionsInSource(src []byte, typeNames []string, methods []yanlingMethodInfo, subscriptions []topicSubscriptionInfo) ([]byte, error) {
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, "script.go", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated script for helper regeneration: %w", err)
	}

	tf := fset.File(fileNode.Pos())
	if tf == nil {
		return nil, fmt.Errorf("failed to resolve token file for generated script")
	}

	type span struct {
		start int
		end   int
	}

	// Build set of names to strip and regenerate
	stripNames := map[string]struct{}{
		"__yanling_newValuePtrByTypeName": {},
		"__yanling_convertJSONToStruct":   {},
		"__yanling_convertJSONToValue":    {},
		"Yanling_ConvertJSONToStruct":     {},
		"Yanling_onTopicTrigger":          {},
	}
	for _, m := range methods {
		stripNames["Yanling_"+m.Name] = struct{}{}
	}

	spans := make([]span, 0)
	for _, decl := range fileNode.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name == nil {
			continue
		}
		if _, found := stripNames[fd.Name.Name]; !found {
			continue
		}
		start := tf.Offset(fd.Pos())
		end := tf.Offset(fd.End())
		if start >= 0 && end > start && end <= len(src) {
			spans = append(spans, span{start: start, end: end})
		}
	}

	if len(spans) > 0 {
		sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
		for _, sp := range spans {
			src = append(src[:sp.start], src[sp.end:]...)
		}
	}

	src = bytes.TrimRight(src, " \t\r\n")
	if len(typeNames) > 0 {
		helper := "\n\n" + buildNewValuePtrByTypeNameSource(typeNames) + "\n" + buildConvertJSONToValueSource() + "\n"
		src = append(src, []byte(helper)...)
	}
	for _, m := range methods {
		src = append(src, []byte("\n"+buildYanlingWrapperSource(m)+"\n")...)
	}
	src = append(src, []byte("\n"+buildYanlingOnTopicTriggerSource(subscriptions)+"\n")...)

	formatted, err := format.Source(src)
	if err != nil {
		return nil, fmt.Errorf("failed to format regenerated helper source: %w", err)
	}
	return formatted, nil
}
