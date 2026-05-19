package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

func rewriteBlockStatements(
	stmts []ast.Stmt,
	rtNames map[string]struct{},
	imports map[string]importInfo,
	cache *moduleCache,
	requiredTypes map[string]map[string]struct{},
) []ast.Stmt {
	if len(stmts) == 0 {
		return stmts
	}

	result := make([]ast.Stmt, 0, len(stmts))
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			s.List = rewriteBlockStatements(s.List, rtNames, imports, cache, requiredTypes)
			result = append(result, s)
		case *ast.IfStmt:
			s.Body.List = rewriteBlockStatements(s.Body.List, rtNames, imports, cache, requiredTypes)
			if s.Else != nil {
				s.Else = rewriteNestedStmt(s.Else, rtNames, imports, cache, requiredTypes)
			}
			result = append(result, s)
		case *ast.ForStmt:
			s.Body.List = rewriteBlockStatements(s.Body.List, rtNames, imports, cache, requiredTypes)
			result = append(result, s)
		case *ast.RangeStmt:
			s.Body.List = rewriteBlockStatements(s.Body.List, rtNames, imports, cache, requiredTypes)
			result = append(result, s)
		case *ast.SwitchStmt:
			rewriteCaseClauses(s.Body, rtNames, imports, cache, requiredTypes)
			result = append(result, s)
		case *ast.TypeSwitchStmt:
			rewriteCaseClauses(s.Body, rtNames, imports, cache, requiredTypes)
			result = append(result, s)
		case *ast.SelectStmt:
			rewriteCommClauses(s.Body, rtNames, imports, cache, requiredTypes)
			result = append(result, s)
		default:
			if rewritten, injected := tryRewriteAssignStmt(stmt, rtNames, imports, cache, requiredTypes); injected {
				result = append(result, rewritten...)
			} else {
				result = append(result, stmt)
			}
		}
	}

	return result
}

func rewriteNestedStmt(
	stmt ast.Stmt,
	rtNames map[string]struct{},
	imports map[string]importInfo,
	cache *moduleCache,
	requiredTypes map[string]map[string]struct{},
) ast.Stmt {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		s.List = rewriteBlockStatements(s.List, rtNames, imports, cache, requiredTypes)
		return s
	case *ast.IfStmt:
		s.Body.List = rewriteBlockStatements(s.Body.List, rtNames, imports, cache, requiredTypes)
		if s.Else != nil {
			s.Else = rewriteNestedStmt(s.Else, rtNames, imports, cache, requiredTypes)
		}
		return s
	default:
		return s
	}
}

func rewriteCaseClauses(
	body *ast.BlockStmt,
	rtNames map[string]struct{},
	imports map[string]importInfo,
	cache *moduleCache,
	requiredTypes map[string]map[string]struct{},
) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		cc.Body = rewriteBlockStatements(cc.Body, rtNames, imports, cache, requiredTypes)
	}
}

func rewriteCommClauses(
	body *ast.BlockStmt,
	rtNames map[string]struct{},
	imports map[string]importInfo,
	cache *moduleCache,
	requiredTypes map[string]map[string]struct{},
) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		cc, ok := stmt.(*ast.CommClause)
		if !ok {
			continue
		}
		cc.Body = rewriteBlockStatements(cc.Body, rtNames, imports, cache, requiredTypes)
	}
}

func tryRewriteAssignStmt(
	stmt ast.Stmt,
	rtNames map[string]struct{},
	imports map[string]importInfo,
	cache *moduleCache,
	requiredTypes map[string]map[string]struct{},
) ([]ast.Stmt, bool) {
	as, ok := stmt.(*ast.AssignStmt)
	if !ok {
		return nil, false
	}
	if len(as.Lhs) != 2 || len(as.Rhs) != 1 {
		return nil, false
	}
	call, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	recvIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, false
	}
	ii, ok := imports[recvIdent.Name]
	if !ok || ii.Keep {
		return nil, false
	}
	if len(call.Args) == 0 || len(call.Args) > 2 {
		return nil, false
	}
	if !isRuntimeArg(call.Args[0], rtNames) {
		return nil, false
	}

	sig, err := cache.lookupFunctionSignature(ii, sel.Sel.Name)
	if err != nil {
		return nil, false
	}

	invokeCall := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   call.Args[0],
			Sel: ast.NewIdent("InvokeWorker"),
		},
		Args: []ast.Expr{
			&ast.BasicLit{Kind: token.STRING, Value: quote(ii.ModulePath)},
			&ast.BasicLit{Kind: token.STRING, Value: quote(sel.Sel.Name)},
		},
	}
	if len(call.Args) == 2 {
		invokeCall.Args = append(invokeCall.Args, call.Args[1])
	}

	lhs0Name := extractIdentName(as.Lhs[0])
	respVar := uniqueRespName(sel.Sel.Name)
	invokeFirstLhs := ast.Expr(ast.NewIdent(respVar))
	if lhs0Name == "_" {
		if as.Tok == token.ASSIGN {
			invokeFirstLhs = ast.NewIdent("_")
		}
	}

	rewritten := make([]ast.Stmt, 0, 3)
	if lhs0Name != "_" && as.Tok == token.DEFINE {
		varTypeExpr, ok := buildResultTypeExpr(sig, ii.ModulePath, requiredTypes)
		if !ok {
			return nil, false
		}
		rewritten = append(rewritten, &ast.DeclStmt{
			Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(lhs0Name)},
					Type:  varTypeExpr,
				}},
			},
		})
	}

	invokeAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{
			invokeFirstLhs,
			as.Lhs[1],
		},
		Tok: as.Tok,
		Rhs: []ast.Expr{invokeCall},
	}
	rewritten = append(rewritten, invokeAssign)

	if lhs0Name == "_" {
		return rewritten, true
	}

	assertTypeExpr, ok := buildResultTypeExpr(sig, ii.ModulePath, requiredTypes)
	if !ok {
		return nil, false
	}
	targetTypeName, ok := buildResultTargetTypeName(sig, ii.ModulePath)
	if !ok {
		return nil, false
	}
	convertVar := uniqueAnyRespName(sel.Sel.Name)
	convertCall := &ast.CallExpr{
		Fun: ast.NewIdent("__yanling_convertJSONToValue"),
		Args: []ast.Expr{
			ast.NewIdent(respVar),
			&ast.BasicLit{Kind: token.STRING, Value: quote(targetTypeName)},
			ast.NewIdent(boolString(buildResultRefFlag(sig))),
		},
	}
	convertAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(convertVar), as.Lhs[1]},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{convertCall},
	}

	assignFromResp := &ast.AssignStmt{
		Lhs: []ast.Expr{as.Lhs[0]},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{
			&ast.TypeAssertExpr{
				X:    ast.NewIdent(convertVar),
				Type: assertTypeExpr,
			},
		},
	}

	if errName := extractIdentName(as.Lhs[1]); errName != "" && errName != "_" {
		rewritten = append(rewritten, convertAssign, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  ast.NewIdent(errName),
				Op: token.EQL,
				Y:  ast.NewIdent("nil"),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{assignFromResp}},
		})
	} else {
		rewritten = append(rewritten, convertAssign, assignFromResp)
	}

	return rewritten, true
}

func buildResultTypeExpr(
	sig functionSignature,
	modulePath string,
	requiredTypes map[string]map[string]struct{},
) (ast.Expr, bool) {
	switch sig.FirstResultKind {
	case resultPrimitive:
		expr, err := parser.ParseExpr(sig.FirstResultType)
		if err != nil {
			return ast.NewIdent("interface{}"), false
		}
		return expr, true
	case resultStruct:
		renamed := renameStruct(modulePath, sig.FirstResultStructName)
		requiredTypes[modulePath][renamed] = struct{}{}
		if strings.HasPrefix(strings.TrimSpace(sig.FirstResultType), "*") {
			return &ast.StarExpr{X: ast.NewIdent(renamed)}, true
		}
		return ast.NewIdent(renamed), true
	default:
		return ast.NewIdent("interface{}"), false
	}
}

func buildResultTargetTypeName(sig functionSignature, modulePath string) (string, bool) {
	switch sig.FirstResultKind {
	case resultPrimitive:
		return sig.FirstResultType, true
	case resultStruct:
		return renameStruct(modulePath, sig.FirstResultStructName), true
	default:
		return "", false
	}
}

func buildResultRefFlag(sig functionSignature) bool {
	if sig.FirstResultKind != resultStruct {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(sig.FirstResultType), "*")
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func collectModuleRuntimeParamNames(params *ast.FieldList) map[string]struct{} {
	result := make(map[string]struct{})
	if params == nil {
		return result
	}
	for _, p := range params.List {
		if !isModuleRuntimeType(p.Type) {
			continue
		}
		for _, n := range p.Names {
			result[n.Name] = struct{}{}
		}
	}
	return result
}

func isModuleRuntimeType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return id.Name == "script" && sel.Sel.Name == "ModuleRuntime"
}

func isRuntimeArg(expr ast.Expr, rtNames map[string]struct{}) bool {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	_, exists := rtNames[id.Name]
	return exists
}

func extractIdentName(expr ast.Expr) string {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

func uniqueRespName(funcName string) string {
	clean := regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(funcName, "")
	if clean == "" {
		clean = "InvokeWorker"
	}
	return "_" + clean + "_Json"
}

func uniqueAnyRespName(funcName string) string {
	clean := regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(funcName, "")
	if clean == "" {
		clean = "InvokeWorker"
	}
	return "_" + clean + "_Value"
}

func quote(s string) string {
	return fmt.Sprintf("%q", s)
}
