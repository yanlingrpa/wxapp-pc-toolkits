package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
)

func validateAll(files []*fileInfo, structs map[string]*structDef, targetPackage string, fset *token.FileSet) []validationError {
	errs := make([]validationError, 0)

	errs = append(errs, validatePackageConstraint(files, targetPackage)...)

	methodStructs := make(map[string]struct{})
	publishStructs := make(map[string]struct{})
	publicFnCount := 0

	for _, fi := range files {
		for _, decl := range fi.Ast.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			if fd.Recv == nil && fd.Name != nil && fd.Name.IsExported() {
				publicFnCount++
				errList, methodStructName, resultStructName := validatePublicMethod(fd, fi.Imports, fi.RelPath, fset, structs)
				errs = append(errs, errList...)
				if methodStructName != "" {
					methodStructs[methodStructName] = struct{}{}
				}
				if resultStructName != "" {
					methodStructs[resultStructName] = struct{}{}
				}
			}

			if fd.Body == nil {
				continue
			}
			errList, published := validatePublishCalls(fd, fi.Imports, fi.RelPath, fset, structs)
			errs = append(errs, errList...)
			for _, name := range published {
				publishStructs[name] = struct{}{}
			}
		}
	}

	if publicFnCount < 1 {
		errs = append(errs, validationError{
			Msg: fmt.Sprintf("must define at least one exported top-level function in package %q, got %d", targetPackage, publicFnCount),
		})
	}

	recorded := make(map[string]struct{})
	for name := range methodStructs {
		recorded[name] = struct{}{}
	}
	for name := range publishStructs {
		recorded[name] = struct{}{}
	}

	errs = append(errs, validateRecordedStructs(recorded, structs, fset)...)
	return errs
}

func printValidationErrors(errs []validationError) {
	sort.Slice(errs, func(i, j int) bool {
		if errs[i].File == errs[j].File {
			if errs[i].Line == errs[j].Line {
				return errs[i].Msg < errs[j].Msg
			}
			return errs[i].Line < errs[j].Line
		}
		return errs[i].File < errs[j].File
	})

	fmt.Fprintf(os.Stderr, "validation failed (%d errors):\n", len(errs))
	for _, e := range errs {
		if e.File != "" {
			if e.Line > 0 {
				fmt.Fprintf(os.Stderr, "- %s:%d: %s\n", e.File, e.Line, e.Msg)
			} else {
				fmt.Fprintf(os.Stderr, "- %s: %s\n", e.File, e.Msg)
			}
		} else {
			fmt.Fprintf(os.Stderr, "- %s\n", e.Msg)
		}
	}
}

func validatePackageConstraint(files []*fileInfo, targetPackage string) []validationError {
	errs := make([]validationError, 0)
	packageSet := make(map[string]struct{})

	for _, fi := range files {
		pkg := fi.Ast.Name.Name
		packageSet[pkg] = struct{}{}
		if pkg != targetPackage {
			errs = append(errs, validationError{
				File: fi.RelPath,
				Line: 1,
				Msg:  fmt.Sprintf("package must be %q, got %q", targetPackage, pkg),
			})
		}
	}

	if len(packageSet) != 1 {
		pkgs := make([]string, 0, len(packageSet))
		for pkg := range packageSet {
			pkgs = append(pkgs, pkg)
		}
		sort.Strings(pkgs)
		errs = append(errs, validationError{
			Msg: fmt.Sprintf("project must contain exactly one package %q, found: %s", targetPackage, strings.Join(pkgs, ", ")),
		})
	}

	return errs
}

func validatePublicMethod(fd *ast.FuncDecl, imports map[string]string, relPath string, fset *token.FileSet, structs map[string]*structDef) ([]validationError, string, string) {
	errs := make([]validationError, 0)
	pos := fset.Position(fd.Pos())
	methodStructName := ""
	resultStructName := ""

	params := expandParams(fd.Type.Params)
	if len(params) == 0 {
		errs = append(errs, validationError{
			File: relPath,
			Line: pos.Line,
			Msg:  fmt.Sprintf("exported function %s must have at least one parameter", fd.Name.Name),
		})
	}

	if !isModuleRuntimeType(params[0].Type, imports) {
		errs = append(errs, validationError{
			File: relPath,
			Line: fset.Position(params[0].Type.Pos()).Line,
			Msg:  fmt.Sprintf("the first parameter of %s must be script.ModuleRuntime", fd.Name.Name),
		})
	}

	if len(params) > 2 {
		errs = append(errs, validationError{
			File: relPath,
			Line: pos.Line,
			Msg:  fmt.Sprintf("exported function %s must have exactly one/two parameters", fd.Name.Name),
		})
	}

	results := expandParams(fd.Type.Results)
	if len(results) != 2 {
		errs = append(errs, validationError{
			File: relPath,
			Line: pos.Line,
			Msg:  fmt.Sprintf("exported function %s must return exactly two values", fd.Name.Name),
		})
	} else if !isErrorType(results[1].Type) {
		errs = append(errs, validationError{
			File: relPath,
			Line: fset.Position(results[1].Type.Pos()).Line,
			Msg:  fmt.Sprintf("the second return value of %s must be error", fd.Name.Name),
		})
	}

	if len(results) > 0 {
		firstResultType := results[0].Type
		if isPrimitiveType(firstResultType) {
			// primitive first return is allowed
		} else if structName := structTypeNameFromResultExpr(firstResultType, structs); structName != "" {
			resultStructName = structName
		} else {
			errs = append(errs, validationError{
				File: relPath,
				Line: fset.Position(firstResultType.Pos()).Line,
				Msg:  fmt.Sprintf("the first return value of %s must be a struct or primitive type", fd.Name.Name),
			})
		}
	}

	if len(params) == 2 {
		if isPrimitiveType(params[1].Type) {
			return errs, methodStructName, resultStructName
		}

		if structName := structTypeNameFromTypeExpr(params[1].Type, structs); structName != "" {
			methodStructName = structName
		} else {
			errs = append(errs, validationError{
				File: relPath,
				Line: fset.Position(params[1].Type.Pos()).Line,
				Msg:  fmt.Sprintf("the second parameter of %s must be a DTO struct or primitive type", fd.Name.Name),
			})
		}
	}

	return errs, methodStructName, resultStructName
}

func validatePublishCalls(fd *ast.FuncDecl, imports map[string]string, relPath string, fset *token.FileSet, structs map[string]*structDef) ([]validationError, []string) {
	errs := make([]validationError, 0)
	publishedStructs := make([]string, 0)
	localTypes := buildLocalStructTypeMap(fd, structs)
	runtimeParamNames := buildRuntimeParamNameSet(fd, imports)

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			updateLocalTypesFromAssign(node, localTypes, structs)
		case *ast.DeclStmt:
			updateLocalTypesFromDecl(node, localTypes, structs)
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Publish" {
				return true
			}
			if len(node.Args) != 2 {
				errs = append(errs, validationError{
					File: relPath,
					Line: fset.Position(node.Pos()).Line,
					Msg:  "Publish must have exactly two arguments: topic and struct payload",
				})
				return true
			}

			if !isModuleRuntimeReceiver(sel.X, runtimeParamNames, localTypes) {
				return true
			}

			payloadType := structTypeNameFromExpr(node.Args[1], localTypes, structs)
			if payloadType == "" {
				errs = append(errs, validationError{
					File: relPath,
					Line: fset.Position(node.Args[1].Pos()).Line,
					Msg:  "Publish payload must be a struct value or pointer to struct",
				})
				return true
			}

			publishedStructs = append(publishedStructs, payloadType)
		}
		return true
	})

	return errs, publishedStructs
}

func validateRecordedStructs(recorded map[string]struct{}, structs map[string]*structDef, fset *token.FileSet) []validationError {
	errs := make([]validationError, 0)

	for structName := range recorded {
		sd, ok := structs[structName]
		if !ok {
			errs = append(errs, validationError{Msg: fmt.Sprintf("recorded struct %q not found in project source", structName)})
			continue
		}

		if !ast.IsExported(sd.Name) {
			errs = append(errs, validationError{
				File: sd.File,
				Line: sd.Line,
				Msg:  fmt.Sprintf("struct %s must be exported", sd.Name),
			})
		}

		for _, field := range sd.Node.Fields.List {
			tagLine := sd.Line
			if field.Tag != nil {
				tagLine = fset.Position(field.Tag.Pos()).Line
			}

			if len(field.Names) == 0 {
				embeddedName := embeddedFieldName(field.Type)
				if embeddedName == "" || !ast.IsExported(embeddedName) {
					errs = append(errs, validationError{
						File: sd.File,
						Line: tagLine,
						Msg:  fmt.Sprintf("embedded field in struct %s must be exported", sd.Name),
					})
				}
				if !hasJSONTag(field) {
					errs = append(errs, validationError{
						File: sd.File,
						Line: tagLine,
						Msg:  fmt.Sprintf("embedded field in struct %s must have json tag", sd.Name),
					})
				}
				continue
			}

			for _, name := range field.Names {
				if !name.IsExported() {
					errs = append(errs, validationError{
						File: sd.File,
						Line: fset.Position(name.Pos()).Line,
						Msg:  fmt.Sprintf("field %s in struct %s must be exported", name.Name, sd.Name),
					})
				}
			}

			if !hasJSONTag(field) {
				errs = append(errs, validationError{
					File: sd.File,
					Line: tagLine,
					Msg:  fmt.Sprintf("all fields in struct %s must have json tag", sd.Name),
				})
			}
		}
	}

	return errs
}

func expandParams(list *ast.FieldList) []paramInfo {
	params := make([]paramInfo, 0)
	if list == nil {
		return params
	}

	for _, field := range list.List {
		if len(field.Names) == 0 {
			params = append(params, paramInfo{Type: field.Type})
			continue
		}
		for _, name := range field.Names {
			params = append(params, paramInfo{Name: name.Name, Type: field.Type})
		}
	}
	return params
}

func isModuleRuntimeType(expr ast.Expr, imports map[string]string) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		ident, ok := t.X.(*ast.Ident)
		if !ok {
			return false
		}
		return t.Sel.Name == "ModuleRuntime" && imports[ident.Name] == scriptImportPath
	case *ast.Ident:
		if t.Name != "ModuleRuntime" {
			return false
		}
		return imports["."] == scriptImportPath
	case *ast.StarExpr:
		return isModuleRuntimeType(t.X, imports)
	default:
		return false
	}
}

func isPrimitiveType(expr ast.Expr) bool {
	builtinTypes := map[string]struct{}{
		"bool": {}, "string": {},
		"int": {}, "int8": {}, "int16": {}, "int32": {}, "int64": {},
		"uint": {}, "uint8": {}, "uint16": {}, "uint32": {}, "uint64": {}, "uintptr": {},
		"float32": {}, "float64": {},
		"complex64": {}, "complex128": {},
		"byte": {}, "rune": {},
	}

	switch t := expr.(type) {
	case *ast.Ident:
		_, ok := builtinTypes[t.Name]
		return ok
	case *ast.StarExpr:
		return isPrimitiveType(t.X)
	default:
		return false
	}
}

func isErrorType(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "error"
}

func structTypeNameFromTypeExpr(expr ast.Expr, structs map[string]*structDef) string {
	switch t := expr.(type) {
	case *ast.Ident:
		if _, ok := structs[t.Name]; ok {
			return t.Name
		}
	case *ast.StarExpr:
		return structTypeNameFromTypeExpr(t.X, structs)
	}
	return ""
}

func structTypeNameFromResultExpr(expr ast.Expr, structs map[string]*structDef) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return structTypeNameFromResultExpr(t.X, structs)
	default:
		return structTypeNameFromTypeExpr(expr, structs)
	}
}

func buildLocalStructTypeMap(fd *ast.FuncDecl, structs map[string]*structDef) map[string]string {
	locals := make(map[string]string)

	for _, param := range expandParams(fd.Type.Params) {
		if param.Name == "" {
			continue
		}
		if typeName := structTypeNameFromTypeExpr(param.Type, structs); typeName != "" {
			locals[param.Name] = typeName
		}
	}

	return locals
}

func updateLocalTypesFromAssign(assign *ast.AssignStmt, locals map[string]string, structs map[string]*structDef) {
	for i, lhs := range assign.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name == "_" {
			continue
		}

		if i >= len(assign.Rhs) {
			continue
		}

		typeName := structTypeNameFromExpr(assign.Rhs[i], locals, structs)
		if typeName != "" {
			locals[ident.Name] = typeName
		}
	}
}

func updateLocalTypesFromDecl(declStmt *ast.DeclStmt, locals map[string]string, structs map[string]*structDef) {
	gen, ok := declStmt.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return
	}

	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		typeFromDecl := ""
		if vs.Type != nil {
			typeFromDecl = structTypeNameFromTypeExpr(vs.Type, structs)
		}

		for i, name := range vs.Names {
			if name.Name == "_" {
				continue
			}

			typeName := typeFromDecl
			if i < len(vs.Values) {
				if inferred := structTypeNameFromExpr(vs.Values[i], locals, structs); inferred != "" {
					typeName = inferred
				}
			}

			if typeName != "" {
				locals[name.Name] = typeName
			}
		}
	}
}

func structTypeNameFromExpr(expr ast.Expr, locals map[string]string, structs map[string]*structDef) string {
	switch t := expr.(type) {
	case *ast.CompositeLit:
		return structTypeNameFromTypeExpr(t.Type, structs)
	case *ast.UnaryExpr:
		if t.Op == token.AND {
			return structTypeNameFromExpr(t.X, locals, structs)
		}
		return ""
	case *ast.ParenExpr:
		return structTypeNameFromExpr(t.X, locals, structs)
	case *ast.Ident:
		if typeName, ok := locals[t.Name]; ok {
			return typeName
		}
		return ""
	default:
		return ""
	}
}

func buildRuntimeParamNameSet(fd *ast.FuncDecl, imports map[string]string) map[string]struct{} {
	runtimeNames := make(map[string]struct{})
	for _, p := range expandParams(fd.Type.Params) {
		if p.Name == "" {
			continue
		}
		if isModuleRuntimeType(p.Type, imports) {
			runtimeNames[p.Name] = struct{}{}
		}
	}
	return runtimeNames
}

func isModuleRuntimeReceiver(expr ast.Expr, runtimeNames map[string]struct{}, locals map[string]string) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}

	if _, isStructVar := locals[ident.Name]; isStructVar {
		return false
	}

	_, ok = runtimeNames[ident.Name]
	return ok
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

func hasJSONTag(field *ast.Field) bool {
	if field.Tag == nil {
		return false
	}
	tagValue := strings.Trim(field.Tag.Value, "`")
	jsonTag, ok := reflect.StructTag(tagValue).Lookup("json")
	if !ok {
		return false
	}
	return strings.TrimSpace(jsonTag) != ""
}
