package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

func rewriteExternalTypeSelectorsInBlock(
	body *ast.BlockStmt,
	imports map[string]importInfo,
	requiredTypes map[string]map[string]struct{},
) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		rewriteExternalTypeSelectorsInStmt(stmt, imports, requiredTypes)
	}
}

func rewriteExternalTypeSelectorsInStmt(
	stmt ast.Stmt,
	imports map[string]importInfo,
	requiredTypes map[string]map[string]struct{},
) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			rewriteExternalTypeSelectorsInExpr(rhs, imports, requiredTypes)
		}
		for _, lhs := range s.Lhs {
			rewriteExternalTypeSelectorsInExpr(lhs, imports, requiredTypes)
		}
	case *ast.ExprStmt:
		rewriteExternalTypeSelectorsInExpr(s.X, imports, requiredTypes)
	case *ast.ReturnStmt:
		for _, expr := range s.Results {
			rewriteExternalTypeSelectorsInExpr(expr, imports, requiredTypes)
		}
	case *ast.IfStmt:
		if s.Init != nil {
			rewriteExternalTypeSelectorsInStmt(s.Init, imports, requiredTypes)
		}
		rewriteExternalTypeSelectorsInExpr(s.Cond, imports, requiredTypes)
		rewriteExternalTypeSelectorsInBlock(s.Body, imports, requiredTypes)
		if s.Else != nil {
			rewriteExternalTypeSelectorsInStmt(s.Else, imports, requiredTypes)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			rewriteExternalTypeSelectorsInStmt(s.Init, imports, requiredTypes)
		}
		if s.Cond != nil {
			rewriteExternalTypeSelectorsInExpr(s.Cond, imports, requiredTypes)
		}
		if s.Post != nil {
			rewriteExternalTypeSelectorsInStmt(s.Post, imports, requiredTypes)
		}
		rewriteExternalTypeSelectorsInBlock(s.Body, imports, requiredTypes)
	case *ast.RangeStmt:
		rewriteExternalTypeSelectorsInExpr(s.X, imports, requiredTypes)
		rewriteExternalTypeSelectorsInBlock(s.Body, imports, requiredTypes)
	case *ast.BlockStmt:
		rewriteExternalTypeSelectorsInBlock(s, imports, requiredTypes)
	case *ast.SwitchStmt:
		if s.Init != nil {
			rewriteExternalTypeSelectorsInStmt(s.Init, imports, requiredTypes)
		}
		if s.Tag != nil {
			rewriteExternalTypeSelectorsInExpr(s.Tag, imports, requiredTypes)
		}
		for _, cs := range s.Body.List {
			cc, ok := cs.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range cc.List {
				rewriteExternalTypeSelectorsInExpr(expr, imports, requiredTypes)
			}
			for _, bstmt := range cc.Body {
				rewriteExternalTypeSelectorsInStmt(bstmt, imports, requiredTypes)
			}
		}
	}
}

func rewriteExternalTypeSelectorsInExpr(
	expr ast.Expr,
	imports map[string]importInfo,
	requiredTypes map[string]map[string]struct{},
) {
	switch e := expr.(type) {
	case *ast.CallExpr:
		rewriteExternalTypeSelectorsInExpr(e.Fun, imports, requiredTypes)
		for _, a := range e.Args {
			rewriteExternalTypeSelectorsInExpr(a, imports, requiredTypes)
		}
	case *ast.CompositeLit:
		if sel, ok := e.Type.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ii, ok := imports[ident.Name]; ok && !ii.Keep {
					renamed := renameStruct(ii.ModulePath, sel.Sel.Name)
					requiredTypes[ii.ModulePath][renamed] = struct{}{}
					e.Type = ast.NewIdent(renamed)
				}
			}
		}
		for _, elt := range e.Elts {
			rewriteExternalTypeSelectorsInExpr(elt, imports, requiredTypes)
		}
	case *ast.TypeAssertExpr:
		rewriteExternalTypeSelectorsInExpr(e.X, imports, requiredTypes)
		if sel, ok := e.Type.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ii, ok := imports[ident.Name]; ok && !ii.Keep {
					renamed := renameStruct(ii.ModulePath, sel.Sel.Name)
					requiredTypes[ii.ModulePath][renamed] = struct{}{}
					e.Type = ast.NewIdent(renamed)
				}
			}
		}
	case *ast.SelectorExpr:
		rewriteExternalTypeSelectorsInExpr(e.X, imports, requiredTypes)
	case *ast.UnaryExpr:
		rewriteExternalTypeSelectorsInExpr(e.X, imports, requiredTypes)
	case *ast.BinaryExpr:
		rewriteExternalTypeSelectorsInExpr(e.X, imports, requiredTypes)
		rewriteExternalTypeSelectorsInExpr(e.Y, imports, requiredTypes)
	case *ast.ParenExpr:
		rewriteExternalTypeSelectorsInExpr(e.X, imports, requiredTypes)
	case *ast.IndexExpr:
		rewriteExternalTypeSelectorsInExpr(e.X, imports, requiredTypes)
		rewriteExternalTypeSelectorsInExpr(e.Index, imports, requiredTypes)
	case *ast.KeyValueExpr:
		rewriteExternalTypeSelectorsInExpr(e.Key, imports, requiredTypes)
		rewriteExternalTypeSelectorsInExpr(e.Value, imports, requiredTypes)
	case *ast.SliceExpr:
		rewriteExternalTypeSelectorsInExpr(e.X, imports, requiredTypes)
		if e.Low != nil {
			rewriteExternalTypeSelectorsInExpr(e.Low, imports, requiredTypes)
		}
		if e.High != nil {
			rewriteExternalTypeSelectorsInExpr(e.High, imports, requiredTypes)
		}
		if e.Max != nil {
			rewriteExternalTypeSelectorsInExpr(e.Max, imports, requiredTypes)
		}
	case *ast.StarExpr:
		rewriteExternalTypeSelectorsInExpr(e.X, imports, requiredTypes)
	}
}

func appendRequiredTypes(
	fileNode *ast.File,
	requiredTypes map[string]map[string]struct{},
	cache *moduleCache,
	imports map[string]importInfo,
) error {
	moduleTypeNames := make(map[string][]string)
	for modulePath, names := range requiredTypes {
		if len(names) == 0 {
			continue
		}
		orderedNames := make([]string, 0, len(names))
		for name := range names {
			orderedNames = append(orderedNames, name)
		}
		sort.Strings(orderedNames)
		moduleTypeNames[modulePath] = orderedNames
	}
	if len(moduleTypeNames) == 0 {
		return nil
	}

	moduleOrdered := make([]string, 0, len(moduleTypeNames))
	for modulePath := range moduleTypeNames {
		moduleOrdered = append(moduleOrdered, modulePath)
	}
	sort.Strings(moduleOrdered)

	appendDecls := make([]ast.Decl, 0)
	for _, modulePath := range moduleOrdered {
		ii, ok := findImportByModule(imports, modulePath)
		if !ok {
			continue
		}
		exports, err := cache.loadModuleExportTypes(ii)
		if err != nil {
			return err
		}

		typeNames := moduleTypeNames[modulePath]
		for idx, typeName := range typeNames {
			decl, ok := exports[typeName]
			if !ok {
				return fmt.Errorf("type %s not found in module export: %s", typeName, modulePath)
			}
			var typeDecl ast.Decl
			var err error
			if idx == 0 {
				typeDecl, err = parseTypeDeclWithImportComment(modulePath, decl.Code)
			} else {
				typeDecl, err = parseTypeDecl(decl.Code)
			}
			if err != nil {
				return err
			}
			appendDecls = append(appendDecls, typeDecl)
		}
	}

	if len(appendDecls) == 0 {
		return nil
	}

	fileNode.Decls = append(fileNode.Decls, appendDecls...)

	return nil
}

func firstNonImportDeclIndex(fileNode *ast.File) int {
	for i, decl := range fileNode.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			return i
		}
	}
	return len(fileNode.Decls)
}

func parseTypeDecl(typeCode string) (ast.Decl, error) {
	typeCode = normalizeTypeCodeWhitespace(typeCode)
	src := "package main\n\n" + typeCode + "\n"
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, "inline_type.go", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse inline type: %w", err)
	}
	if len(fileNode.Decls) == 0 {
		return nil, fmt.Errorf("inline type declaration is empty")
	}
	return fileNode.Decls[0], nil
}

func parseTypeDeclWithImportComment(modulePath, typeCode string) (ast.Decl, error) {
	typeCode = normalizeTypeCodeWhitespace(typeCode)
	src := fmt.Sprintf("package main\n\n// import %q\n%s\n", modulePath, typeCode)
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, "inline_type_comment.go", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse inline type with import comment: %w", err)
	}
	if len(fileNode.Decls) == 0 {
		return nil, fmt.Errorf("inline type declaration with import comment is empty")
	}
	return fileNode.Decls[0], nil
}

func pruneIllegalImports(fileNode *ast.File, imports map[string]importInfo) {
	newDecls := make([]ast.Decl, 0, len(fileNode.Decls))
	for _, decl := range fileNode.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			newDecls = append(newDecls, decl)
			continue
		}
		kept := make([]ast.Spec, 0, len(gd.Specs))
		for _, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok {
				continue
			}
			path := strings.Trim(is.Path.Value, `"`)
			alias := importAlias(is)
			ii, ok := imports[alias]
			if !ok {
				if isAllowedImport(path) {
					kept = append(kept, spec)
				}
				continue
			}
			if ii.Keep {
				kept = append(kept, spec)
			}
		}
		if len(kept) == 0 {
			continue
		}
		gd.Specs = kept
		newDecls = append(newDecls, gd)
	}
	fileNode.Decls = newDecls
}
