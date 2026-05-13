package main

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
)

func extractDeclarations(pkg *packageAggregate, fileAst *ast.File, ctx fileContext, fset *token.FileSet) {
	for _, decl := range fileAst.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE:
				extractTypeDecls(pkg, d, ctx, fset)
			case token.CONST:
				extractConstDecls(pkg, d, ctx, fset)
			}
		case *ast.FuncDecl:
			if d.Name == nil || !d.Name.IsExported() {
				continue
			}
			if d.Recv == nil {
				addSymbol(pkg, buildFunctionSymbol(pkg, d, ctx, fset))
				continue
			}

			receiverType := receiverTypeName(d.Recv)
			if receiverType == "" {
				continue
			}
			attachMethod(pkg, receiverType, buildMethodDoc(d, ctx, pkg.ImportPath))
		}
	}
}

func extractTypeDecls(pkg *packageAggregate, decl *ast.GenDecl, ctx fileContext, fset *token.FileSet) {
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok || !typeSpec.Name.IsExported() {
			continue
		}

		doc := cleanComment(pickDoc(typeSpec.Doc, decl.Doc))
		source := sourceForNode(fset, ctx.RelPath, typeSpec.Pos())
		if typeSpec.Assign.IsValid() {
			addSymbol(pkg, buildAliasSymbol(pkg, typeSpec, doc, source, ctx))
			continue
		}

		var symbol *SymbolDoc
		switch t := typeSpec.Type.(type) {
		case *ast.InterfaceType:
			symbol = buildInterfaceSymbol(pkg, typeSpec.Name.Name, t, doc, source, ctx)
		case *ast.StructType:
			symbol = buildStructSymbol(pkg, typeSpec.Name.Name, t, doc, source, ctx)
		case *ast.FuncType:
			symbol = buildFuncTypeSymbol(pkg, typeSpec.Name.Name, t, doc, source, ctx)
		default:
			symbol = buildNamedTypeSymbol(pkg, typeSpec.Name.Name, typeSpec.Type, doc, source, ctx)
		}
		addSymbol(pkg, symbol)
	}
}

func extractConstDecls(pkg *packageAggregate, decl *ast.GenDecl, ctx fileContext, fset *token.FileSet) {
	lastType := ""
	var lastValues []string

	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		typeText := lastType
		if valueSpec.Type != nil {
			typeText = exprToString(valueSpec.Type)
			lastType = typeText
		}

		values := lastValues
		if len(valueSpec.Values) > 0 {
			values = make([]string, 0, len(valueSpec.Values))
			for _, value := range valueSpec.Values {
				values = append(values, exprToString(value))
			}
			lastValues = values
		}

		doc := cleanComment(pickDoc(valueSpec.Doc, decl.Doc))
		typeRefs := collectTypeRefs(valueSpec.Type, ctx.Imports, pkg.ImportPath)
		for idx, name := range valueSpec.Names {
			if !name.IsExported() {
				continue
			}

			valueText := ""
			switch {
			case len(values) == len(valueSpec.Names):
				valueText = values[idx]
			case len(values) == 1:
				valueText = values[0]
			case idx < len(values):
				valueText = values[idx]
			}

			signature := "const " + name.Name
			if typeText != "" {
				signature += " " + typeText
			}
			if valueText != "" {
				signature += " = " + valueText
			}

			addSymbol(pkg, &SymbolDoc{
				ID:            symbolID(pkg.ImportPath, name.Name),
				Name:          name.Name,
				QualifiedName: qualifiedName(pkg.ImportPath, name.Name),
				Kind:          "const",
				Package:       pkg.Name,
				ImportPath:    pkg.ImportPath,
				Doc:           doc,
				Signature:     signature,
				ConstType:     typeText,
				ConstValue:    valueText,
				Source:        sourceForNode(fset, ctx.RelPath, name.Pos()),
				TypeRefs:      append([]TypeRefDoc{}, typeRefs...),
			})
		}
	}
}

func buildFunctionSymbol(pkg *packageAggregate, fn *ast.FuncDecl, ctx fileContext, fset *token.FileSet) *SymbolDoc {
	params := extractFieldList(fn.Type.Params)
	results := extractFieldList(fn.Type.Results)
	name := fn.Name.Name
	return &SymbolDoc{
		ID:            symbolID(pkg.ImportPath, name),
		Name:          name,
		QualifiedName: qualifiedName(pkg.ImportPath, name),
		Kind:          "function",
		Package:       pkg.Name,
		ImportPath:    pkg.ImportPath,
		Doc:           cleanComment(pickDoc(fn.Doc, nil)),
		Signature:     buildSignature(name, params, results),
		Source:        sourceForNode(fset, ctx.RelPath, fn.Pos()),
		Params:        params,
		Results:       results,
		TypeRefs:      collectFuncTypeRefs(fn.Type, ctx.Imports, pkg.ImportPath),
	}
}

func buildMethodDoc(fn *ast.FuncDecl, ctx fileContext, localImportPath string) MethodDoc {
	params := extractFieldList(fn.Type.Params)
	results := extractFieldList(fn.Type.Results)
	return MethodDoc{
		Name:      fn.Name.Name,
		Doc:       cleanComment(pickDoc(fn.Doc, nil)),
		Params:    params,
		Results:   results,
		Signature: buildSignature(fn.Name.Name, params, results),
		TypeRefs:  collectFuncTypeRefs(fn.Type, ctx.Imports, localImportPath),
	}
}

func buildInterfaceSymbol(pkg *packageAggregate, name string, iface *ast.InterfaceType, doc string, source SourceDoc, ctx fileContext) *SymbolDoc {
	methods := make([]MethodDoc, 0)
	embeds := make([]EmbeddedTypeDoc, 0)
	typeRefs := make([]TypeRefDoc, 0)
	if iface.Methods != nil {
		for _, field := range iface.Methods.List {
			if len(field.Names) == 0 {
				typeText := exprToString(field.Type)
				embeds = append(embeds, buildEmbeddedTypeDoc(field.Type, typeText, ctx.Imports, pkg.ImportPath))
				typeRefs = append(typeRefs, collectTypeRefs(field.Type, ctx.Imports, pkg.ImportPath)...)
				continue
			}
			funcType, ok := field.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			params := extractFieldList(funcType.Params)
			results := extractFieldList(funcType.Results)
			methodRefs := collectFuncTypeRefs(funcType, ctx.Imports, pkg.ImportPath)
			typeRefs = append(typeRefs, methodRefs...)
			methods = append(methods, MethodDoc{
				Name:      field.Names[0].Name,
				Doc:       cleanComment(pickDoc(field.Doc, field.Comment)),
				Params:    params,
				Results:   results,
				Signature: buildSignature(field.Names[0].Name, params, results),
				TypeRefs:  methodRefs,
			})
		}
	}
	sortMethods(methods)
	sortEmbeddedTypes(embeds)
	return &SymbolDoc{
		ID:            symbolID(pkg.ImportPath, name),
		Name:          name,
		QualifiedName: qualifiedName(pkg.ImportPath, name),
		Kind:          "interface",
		Package:       pkg.Name,
		ImportPath:    pkg.ImportPath,
		Doc:           doc,
		Signature:     "type " + name + " interface",
		Source:        source,
		Methods:       methods,
		Embeds:        embeds,
		TypeRefs:      dedupeTypeRefs(typeRefs),
	}
}

func buildStructSymbol(pkg *packageAggregate, name string, st *ast.StructType, doc string, source SourceDoc, ctx fileContext) *SymbolDoc {
	fields := extractStructFields(st, ctx.Imports, pkg.ImportPath)
	return &SymbolDoc{
		ID:            symbolID(pkg.ImportPath, name),
		Name:          name,
		QualifiedName: qualifiedName(pkg.ImportPath, name),
		Kind:          "struct",
		Package:       pkg.Name,
		ImportPath:    pkg.ImportPath,
		Doc:           doc,
		Signature:     "type " + name + " struct",
		Source:        source,
		Fields:        fields,
		Methods:       []MethodDoc{},
		TypeRefs:      fieldTypeRefs(fields),
	}
}

func buildFuncTypeSymbol(pkg *packageAggregate, name string, fnType *ast.FuncType, doc string, source SourceDoc, ctx fileContext) *SymbolDoc {
	params := extractFieldList(fnType.Params)
	results := extractFieldList(fnType.Results)
	underlying := exprToString(fnType)
	return &SymbolDoc{
		ID:             symbolID(pkg.ImportPath, name),
		Name:           name,
		QualifiedName:  qualifiedName(pkg.ImportPath, name),
		Kind:           "func_type",
		Package:        pkg.Name,
		ImportPath:     pkg.ImportPath,
		Doc:            doc,
		Signature:      buildTypeSignature(name, false, underlying),
		UnderlyingType: underlying,
		Source:         source,
		Params:         params,
		Results:        results,
		Methods:        []MethodDoc{},
		TypeRefs:       collectFuncTypeRefs(fnType, ctx.Imports, pkg.ImportPath),
	}
}

func buildNamedTypeSymbol(pkg *packageAggregate, name string, expr ast.Expr, doc string, source SourceDoc, ctx fileContext) *SymbolDoc {
	underlying := exprToString(expr)
	return &SymbolDoc{
		ID:             symbolID(pkg.ImportPath, name),
		Name:           name,
		QualifiedName:  qualifiedName(pkg.ImportPath, name),
		Kind:           "named_type",
		Package:        pkg.Name,
		ImportPath:     pkg.ImportPath,
		Doc:            doc,
		Signature:      buildTypeSignature(name, false, underlying),
		UnderlyingType: underlying,
		Source:         source,
		Methods:        []MethodDoc{},
		TypeRefs:       collectTypeRefs(expr, ctx.Imports, pkg.ImportPath),
	}
}

func buildAliasSymbol(pkg *packageAggregate, typeSpec *ast.TypeSpec, doc string, source SourceDoc, ctx fileContext) *SymbolDoc {
	underlying := exprToString(typeSpec.Type)
	return &SymbolDoc{
		ID:             symbolID(pkg.ImportPath, typeSpec.Name.Name),
		Name:           typeSpec.Name.Name,
		QualifiedName:  qualifiedName(pkg.ImportPath, typeSpec.Name.Name),
		Kind:           "type_alias",
		Package:        pkg.Name,
		ImportPath:     pkg.ImportPath,
		Doc:            doc,
		Signature:      buildTypeSignature(typeSpec.Name.Name, true, underlying),
		UnderlyingType: underlying,
		Alias:          true,
		Source:         source,
		Methods:        []MethodDoc{},
		TypeRefs:       collectTypeRefs(typeSpec.Type, ctx.Imports, pkg.ImportPath),
	}
}

func extractStructFields(st *ast.StructType, imports map[string]string, localImportPath string) []FieldDoc {
	if st.Fields == nil {
		return []FieldDoc{}
	}

	fields := make([]FieldDoc, 0, len(st.Fields.List))
	for _, field := range st.Fields.List {
		typeText := exprToString(field.Type)
		doc := cleanComment(pickDoc(field.Doc, field.Comment))
		tag := ""
		if field.Tag != nil {
			tag = strings.Trim(field.Tag.Value, "`")
		}
		typeRefs := collectTypeRefs(field.Type, imports, localImportPath)

		if len(field.Names) == 0 {
			if !isExportedEmbedded(field.Type) {
				continue
			}
			fields = append(fields, FieldDoc{
				Name:     embeddedFieldName(field.Type),
				Type:     typeText,
				Tag:      tag,
				Doc:      doc,
				Embedded: true,
				TypeRefs: typeRefs,
			})
			continue
		}

		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			fields = append(fields, FieldDoc{
				Name:     name.Name,
				Type:     typeText,
				Tag:      tag,
				Doc:      doc,
				TypeRefs: typeRefs,
			})
		}
	}
	sortFields(fields)
	return fields
}

func extractFieldList(list *ast.FieldList) []ParamDoc {
	if list == nil || len(list.List) == 0 {
		return []ParamDoc{}
	}

	items := make([]ParamDoc, 0)
	for _, field := range list.List {
		typeText := exprToString(field.Type)
		if len(field.Names) == 0 {
			items = append(items, ParamDoc{Type: typeText})
			continue
		}
		for _, name := range field.Names {
			items = append(items, ParamDoc{Name: name.Name, Type: typeText})
		}
	}
	return items
}

func addSymbol(pkg *packageAggregate, symbol *SymbolDoc) {
	if symbol.Methods == nil && (symbol.Kind == "struct" || symbol.Kind == "named_type" || symbol.Kind == "func_type" || symbol.Kind == "type_alias") {
		symbol.Methods = []MethodDoc{}
	}
	pkg.Symbols[symbol.Name] = symbol
	if methods, ok := pkg.PendingMethods[symbol.Name]; ok {
		symbol.Methods = append(symbol.Methods, methods...)
		sortMethods(symbol.Methods)
		symbol.TypeRefs = dedupeTypeRefs(append(symbol.TypeRefs, typeRefsFromMethods(symbol.Methods)...))
		delete(pkg.PendingMethods, symbol.Name)
	}
}

func attachMethod(pkg *packageAggregate, receiver string, method MethodDoc) {
	if symbol, ok := pkg.Symbols[receiver]; ok {
		symbol.Methods = append(symbol.Methods, method)
		sortMethods(symbol.Methods)
		symbol.TypeRefs = dedupeTypeRefs(append(symbol.TypeRefs, method.TypeRefs...))
		return
	}
	pkg.PendingMethods[receiver] = append(pkg.PendingMethods[receiver], method)
}

func finalizePackage(pkg *packageAggregate) {
	sort.Strings(pkg.Files)
	for _, methods := range pkg.PendingMethods {
		sortMethods(methods)
	}
	for _, symbol := range pkg.Symbols {
		sortMethods(symbol.Methods)
		symbol.TypeRefs = dedupeTypeRefs(symbol.TypeRefs)
		sortFields(symbol.Fields)
		sortEmbeddedTypes(symbol.Embeds)
	}
}
