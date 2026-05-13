package main

import "go/ast"

func collectFuncTypeRefs(fnType *ast.FuncType, imports map[string]string, localImportPath string) []TypeRefDoc {
	if fnType == nil {
		return nil
	}
	refs := make([]TypeRefDoc, 0)
	refs = append(refs, collectFieldListTypeRefs(fnType.Params, imports, localImportPath)...)
	refs = append(refs, collectFieldListTypeRefs(fnType.Results, imports, localImportPath)...)
	return dedupeTypeRefs(refs)
}

func collectFieldListTypeRefs(list *ast.FieldList, imports map[string]string, localImportPath string) []TypeRefDoc {
	if list == nil {
		return nil
	}
	refs := make([]TypeRefDoc, 0)
	for _, field := range list.List {
		refs = append(refs, collectTypeRefs(field.Type, imports, localImportPath)...)
	}
	return refs
}

func collectTypeRefs(expr ast.Expr, imports map[string]string, localImportPath string) []TypeRefDoc {
	if expr == nil {
		return nil
	}

	refs := make(map[string]TypeRefDoc)
	var visit func(ast.Expr)
	visit = func(current ast.Expr) {
		switch t := current.(type) {
		case *ast.Ident:
			if t.Name == "" {
				return
			}
			if _, ok := builtinTypes[t.Name]; ok {
				refs["builtin:"+t.Name] = TypeRefDoc{Expr: t.Name, Builtin: true}
				return
			}
			refs["local:"+t.Name] = TypeRefDoc{
				Expr:          t.Name,
				ImportPath:    localImportPath,
				QualifiedName: qualifiedName(localImportPath, t.Name),
				Local:         true,
			}
		case *ast.SelectorExpr:
			if pkgIdent, ok := t.X.(*ast.Ident); ok {
				if importPath, found := imports[pkgIdent.Name]; found {
					exprText := exprToString(t)
					refs["import:"+exprText] = TypeRefDoc{
						Expr:          exprText,
						ImportPath:    importPath,
						QualifiedName: qualifiedName(importPath, t.Sel.Name),
					}
					return
				}
			}
			visit(t.X)
		case *ast.StarExpr:
			visit(t.X)
		case *ast.ArrayType:
			visit(t.Elt)
		case *ast.MapType:
			visit(t.Key)
			visit(t.Value)
		case *ast.ChanType:
			visit(t.Value)
		case *ast.Ellipsis:
			visit(t.Elt)
		case *ast.FuncType:
			for _, ref := range collectFuncTypeRefs(t, imports, localImportPath) {
				refs[typeRefKey(ref)] = ref
			}
		case *ast.InterfaceType:
			if t.Methods != nil {
				for _, field := range t.Methods.List {
					visit(field.Type)
				}
			}
		case *ast.StructType:
			if t.Fields != nil {
				for _, field := range t.Fields.List {
					visit(field.Type)
				}
			}
		case *ast.IndexExpr:
			visit(t.X)
			visit(t.Index)
		case *ast.IndexListExpr:
			visit(t.X)
			for _, index := range t.Indices {
				visit(index)
			}
		case *ast.ParenExpr:
			visit(t.X)
		}
	}

	visit(expr)
	items := make([]TypeRefDoc, 0, len(refs))
	for _, ref := range refs {
		items = append(items, ref)
	}
	return sortTypeRefs(items)
}

func buildEmbeddedTypeDoc(expr ast.Expr, typeText string, imports map[string]string, localImportPath string) EmbeddedTypeDoc {
	refs := collectTypeRefs(expr, imports, localImportPath)
	item := EmbeddedTypeDoc{Type: typeText}
	if len(refs) > 0 {
		item.ImportPath = refs[0].ImportPath
		item.QualifiedName = refs[0].QualifiedName
	}
	return item
}

func fieldTypeRefs(fields []FieldDoc) []TypeRefDoc {
	refs := make([]TypeRefDoc, 0)
	for _, field := range fields {
		refs = append(refs, field.TypeRefs...)
	}
	return dedupeTypeRefs(refs)
}

func typeRefsFromMethods(methods []MethodDoc) []TypeRefDoc {
	refs := make([]TypeRefDoc, 0)
	for _, method := range methods {
		refs = append(refs, method.TypeRefs...)
	}
	return dedupeTypeRefs(refs)
}
