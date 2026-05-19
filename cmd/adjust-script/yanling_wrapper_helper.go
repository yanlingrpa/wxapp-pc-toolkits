package main

import (
	"fmt"
	"go/ast"
	"strings"
)

// yanlingMethodInfo describes a public script method eligible for a Yanling_ wrapper.
type yanlingMethodInfo struct {
	Name         string // original method name (e.g. "CollectMedicine")
	RtParamName  string // name of ModuleRuntime param (e.g. "rt")
	DtoParamName string // name of second param if present (e.g. "dto")
	DtoTypeName  string // unqualified type name of second param (e.g. "SearchProductDto")
	DtoIsPtr     bool   // whether second param is a pointer type
}

// collectYanlingMethods returns all exported top-level functions (excluding Yanling_* ones)
// whose first parameter is script.ModuleRuntime and that have 1 or 2 parameters total.
func collectYanlingMethods(fileNode *ast.File) []yanlingMethodInfo {
	var result []yanlingMethodInfo
	for _, decl := range fileNode.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil || fd.Name == nil || fd.Type == nil || fd.Type.Params == nil {
			continue
		}
		if !ast.IsExported(fd.Name.Name) {
			continue
		}
		if strings.HasPrefix(fd.Name.Name, "Yanling_") {
			continue
		}
		params := fd.Type.Params.List
		// Count total parameters by expanding named groups.
		total := 0
		for _, f := range params {
			if len(f.Names) == 0 {
				total++
			} else {
				total += len(f.Names)
			}
		}
		if total < 1 || total > 2 {
			continue
		}
		// First field must be script.ModuleRuntime.
		if len(params) == 0 || !isModuleRuntimeType(params[0].Type) {
			continue
		}
		if len(params[0].Names) == 0 {
			continue
		}
		rtName := params[0].Names[0].Name
		info := yanlingMethodInfo{
			Name:        fd.Name.Name,
			RtParamName: rtName,
		}
		if total == 2 {
			// The second param must be in its own field (different type from ModuleRuntime).
			if len(params) < 2 {
				continue
			}
			secondField := params[1]
			if len(secondField.Names) == 0 {
				continue
			}
			typeName, isPtr := extractTypeNameFromExpr(secondField.Type)
			if typeName == "" {
				continue
			}
			info.DtoParamName = secondField.Names[0].Name
			info.DtoTypeName = typeName
			info.DtoIsPtr = isPtr
		}
		result = append(result, info)
	}
	return result
}

func extractTypeNameFromExpr(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.StarExpr:
		name, _ := extractTypeNameFromExpr(e.X)
		return name, true
	case *ast.Ident:
		return e.Name, false
	}
	return "", false
}

// buildYanlingWrapperSource generates the source text for a Yanling_${Name} wrapper function.
func buildYanlingWrapperSource(info yanlingMethodInfo) string {
	var b strings.Builder
	b.WriteString("func Yanling_")
	b.WriteString(info.Name)
	b.WriteString("(")
	b.WriteString(info.RtParamName)
	b.WriteString(" script.ModuleRuntime")
	if info.DtoParamName != "" {
		b.WriteString(", ")
		b.WriteString(info.DtoParamName)
		b.WriteString("JsonStr string")
	}
	b.WriteString(") (string, error) {\n")

	if info.DtoParamName != "" {
		b.WriteString("\t")
		b.WriteString(info.DtoParamName)
		b.WriteString("Any, err := __yanling_convertJSONToValue(")
		b.WriteString(info.DtoParamName)
		b.WriteString("JsonStr, \"")
		b.WriteString(info.DtoTypeName)
		b.WriteString("\", ")
		if info.DtoIsPtr {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString(")\n")
		b.WriteString("\tif err != nil {\n\t\treturn \"\", err\n\t}\n")
		b.WriteString("\t")
		b.WriteString(info.DtoParamName)
		b.WriteString(" := ")
		b.WriteString(info.DtoParamName)
		b.WriteString("Any.(")
		if info.DtoIsPtr {
			b.WriteString("*")
		}
		b.WriteString(info.DtoTypeName)
		b.WriteString(")\n")
		b.WriteString("\tresult, err := ")
		b.WriteString(info.Name)
		b.WriteString("(")
		b.WriteString(info.RtParamName)
		b.WriteString(", ")
		b.WriteString(info.DtoParamName)
		b.WriteString(")\n")
	} else {
		b.WriteString("\tresult, err := ")
		b.WriteString(info.Name)
		b.WriteString("(")
		b.WriteString(info.RtParamName)
		b.WriteString(")\n")
	}

	b.WriteString("\tif err != nil {\n\t\treturn \"\", err\n\t}\n")
	b.WriteString("\tresultBytes, err := json.Marshal(result)\n")
	b.WriteString("\tif err != nil {\n\t\treturn \"\", fmt.Errorf(\"failed to marshal result: %w\", err)\n\t}\n")
	b.WriteString("\treturn string(resultBytes), nil\n")
	b.WriteString("}\n")
	return b.String()
}

// ensureYanlingWrappers removes any existing Yanling_ wrapper functions for the given methods
// and appends freshly generated ones to the file.
func ensureYanlingWrappers(fileNode *ast.File, methods []yanlingMethodInfo) error {
	for _, m := range methods {
		removeFuncDeclByName(fileNode, "Yanling_"+m.Name)
	}
	if len(methods) == 0 {
		return nil
	}
	ensureImportPath(fileNode, "encoding/json")
	ensureImportPath(fileNode, "fmt")
	for _, m := range methods {
		src := buildYanlingWrapperSource(m)
		decl, err := parseFuncDecl(src)
		if err != nil {
			return fmt.Errorf("failed to parse Yanling_%s wrapper: %w", m.Name, err)
		}
		fileNode.Decls = append(fileNode.Decls, decl)
	}
	return nil
}
