package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const schemaVersion = "yanling.machine-first/v1"

const (
	moduleSchemaRef  = "./schema/yanling.machine-first.v1/module.schema.json"
	symbolsSchemaRef = "./schema/yanling.machine-first.v1/symbols.schema.json"
	packageSchemaRef = "./../schema/yanling.machine-first.v1/package.schema.json"
	topicsSchemaRef  = "./schema/yanling.machine-first.v1/topics.schema.json"
)

var excludedTopLevelDirs = map[string]struct{}{
	".git":      {},
	".vscode":   {},
	".protocol": {},
	".yanling":  {},
	"cmd":       {},
	"doc":       {},
	"tests":     {},
	"symbols":   {},
	"schema":    {},
}

var builtinTypes = map[string]struct{}{
	"any":         {},
	"bool":        {},
	"byte":        {},
	"comparable":  {},
	"complex64":   {},
	"complex128":  {},
	"error":       {},
	"float32":     {},
	"float64":     {},
	"int":         {},
	"int8":        {},
	"int16":       {},
	"int32":       {},
	"int64":       {},
	"interface{}": {},
	"rune":        {},
	"string":      {},
	"uint":        {},
	"uint8":       {},
	"uint16":      {},
	"uint32":      {},
	"uint64":      {},
	"uintptr":     {},
}

type packageAggregate struct {
	Name           string
	ImportPath     string
	RelDir         string
	Doc            string
	Files          []string
	PackageImports map[string]map[string]struct{}
	Symbols        map[string]*SymbolDoc
	PendingMethods map[string][]MethodDoc
}

type fileContext struct {
	RelPath string
	Imports map[string]string
}

type SourceDoc struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type CountsDoc struct {
	Packages    int `json:"packages,omitempty"`
	Symbols     int `json:"symbols,omitempty"`
	Interfaces  int `json:"interfaces,omitempty"`
	Structs     int `json:"structs,omitempty"`
	Functions   int `json:"functions,omitempty"`
	Methods     int `json:"methods,omitempty"`
	NamedTypes  int `json:"named_types,omitempty"`
	TypeAliases int `json:"type_aliases,omitempty"`
	FuncTypes   int `json:"func_types,omitempty"`
	Consts      int `json:"consts,omitempty"`
}

type ModuleOutput struct {
	SchemaRef     string               `json:"$schema,omitempty"`
	SchemaVersion string               `json:"schema_version"`
	GeneratedAt   string               `json:"generated_at"`
	Module        string               `json:"module"`
	Counts        CountsDoc            `json:"counts"`
	Files         ModuleFilesDoc       `json:"files"`
	Packages      []ModulePackageEntry `json:"packages"`
}

type ModuleFilesDoc struct {
	SymbolIndex     string `json:"symbol_index"`
	SymbolIndexLite string `json:"symbol_index_lite,omitempty"`
	PackageDir      string `json:"package_dir"`
}

type ModulePackageEntry struct {
	Name         string    `json:"name"`
	ImportPath   string    `json:"import_path"`
	Directory    string    `json:"directory"`
	Doc          string    `json:"doc,omitempty"`
	Counts       CountsDoc `json:"counts"`
	PackageFile  string    `json:"package_file"`
	Dependencies []string  `json:"dependencies,omitempty"`
}

type SymbolsOutput struct {
	SchemaRef     string             `json:"$schema,omitempty"`
	SchemaVersion string             `json:"schema_version"`
	GeneratedAt   string             `json:"generated_at"`
	Module        string             `json:"module"`
	Counts        CountsDoc          `json:"counts"`
	Symbols       []SymbolIndexEntry `json:"symbols"`
}

type SymbolsLiteOutput struct {
	SchemaVersion string                 `json:"schema_version"`
	GeneratedAt   string                 `json:"generated_at"`
	Module        string                 `json:"module"`
	Counts        CountsDoc              `json:"counts"`
	Symbols       []SymbolLiteIndexEntry `json:"symbols"`
}

type SymbolLiteIndexEntry struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	QualifiedName string    `json:"qualified_name"`
	Kind          string    `json:"kind"`
	Package       string    `json:"package"`
	ImportPath    string    `json:"import_path"`
	Source        SourceDoc `json:"source"`
	PackageFile   string    `json:"package_file"`
}

type SymbolIndexEntry struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	QualifiedName string       `json:"qualified_name"`
	Kind          string       `json:"kind"`
	Package       string       `json:"package"`
	ImportPath    string       `json:"import_path"`
	Doc           string       `json:"doc,omitempty"`
	Signature     string       `json:"signature,omitempty"`
	Source        SourceDoc    `json:"source"`
	PackageFile   string       `json:"package_file"`
	TypeRefs      []TypeRefDoc `json:"type_refs,omitempty"`
	MethodCount   int          `json:"method_count,omitempty"`
	FieldCount    int          `json:"field_count,omitempty"`
}

type PackageOutput struct {
	SchemaRef     string     `json:"$schema,omitempty"`
	SchemaVersion string     `json:"schema_version"`
	GeneratedAt   string     `json:"generated_at"`
	Module        string     `json:"module"`
	Package       PackageDoc `json:"package"`
}

type PackageDoc struct {
	Name         string          `json:"name"`
	ImportPath   string          `json:"import_path"`
	Directory    string          `json:"directory"`
	Doc          string          `json:"doc,omitempty"`
	Files        []string        `json:"files"`
	Imports      []PackageImport `json:"imports,omitempty"`
	Dependencies []string        `json:"dependencies,omitempty"`
	Counts       CountsDoc       `json:"counts"`
	Symbols      []SymbolDoc     `json:"symbols"`
}

type PackageImport struct {
	Path    string   `json:"path"`
	Aliases []string `json:"aliases,omitempty"`
}

type SymbolDoc struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	QualifiedName  string            `json:"qualified_name"`
	Kind           string            `json:"kind"`
	Package        string            `json:"package"`
	ImportPath     string            `json:"import_path"`
	Doc            string            `json:"doc,omitempty"`
	Signature      string            `json:"signature,omitempty"`
	UnderlyingType string            `json:"underlying_type,omitempty"`
	Alias          bool              `json:"alias,omitempty"`
	Source         SourceDoc         `json:"source"`
	Params         []ParamDoc        `json:"params,omitempty"`
	Results        []ParamDoc        `json:"results,omitempty"`
	Fields         []FieldDoc        `json:"fields,omitempty"`
	Methods        []MethodDoc       `json:"methods,omitempty"`
	Embeds         []EmbeddedTypeDoc `json:"embeds,omitempty"`
	ConstType      string            `json:"const_type,omitempty"`
	ConstValue     string            `json:"const_value,omitempty"`
	TypeRefs       []TypeRefDoc      `json:"type_refs,omitempty"`
}

type MethodDoc struct {
	Name      string       `json:"name"`
	Doc       string       `json:"doc,omitempty"`
	Params    []ParamDoc   `json:"params"`
	Results   []ParamDoc   `json:"results"`
	Signature string       `json:"signature"`
	TypeRefs  []TypeRefDoc `json:"type_refs,omitempty"`
}

type ParamDoc struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
}

type FieldDoc struct {
	Name     string       `json:"name,omitempty"`
	Type     string       `json:"type"`
	Tag      string       `json:"tag,omitempty"`
	Doc      string       `json:"doc,omitempty"`
	Embedded bool         `json:"embedded,omitempty"`
	TypeRefs []TypeRefDoc `json:"type_refs,omitempty"`
}

type EmbeddedTypeDoc struct {
	Type          string `json:"type"`
	ImportPath    string `json:"import_path,omitempty"`
	QualifiedName string `json:"qualified_name,omitempty"`
}

type TypeRefDoc struct {
	Expr          string `json:"expr"`
	ImportPath    string `json:"import_path,omitempty"`
	QualifiedName string `json:"qualified_name,omitempty"`
	Local         bool   `json:"local,omitempty"`
	Builtin       bool   `json:"builtin,omitempty"`
}

// TopicFieldSchema describes a topic payload field (or the root payload object) in JSON Schema style.
type TopicFieldSchema struct {
	Type                 string                       `json:"type,omitempty"`
	Description          string                       `json:"description,omitempty"`
	Nullable             bool                         `json:"nullable,omitempty"`
	GoTypeHint           string                       `json:"go_type_hint,omitempty"`
	GoFieldName          string                       `json:"go_field_name,omitempty"`
	Format               string                       `json:"format,omitempty"`
	Properties           map[string]*TopicFieldSchema `json:"properties,omitempty"`
	Items                *TopicFieldSchema            `json:"items,omitempty"`
	Required             []string                     `json:"required,omitempty"`
	AdditionalProperties *bool                        `json:"additionalProperties,omitempty"`
}

// TopicDoc describes a single event topic published by this module.
type TopicDoc struct {
	Name         string           `json:"name"`
	Doc          string           `json:"doc,omitempty"`
	Direction    string           `json:"direction,omitempty"`
	GoStructName string           `json:"go_struct_name,omitempty"`
	Payload      TopicFieldSchema `json:"payload"`
}

// TopicsOutput is the top-level structure for topics.json.
type TopicsOutput struct {
	SchemaRef     string     `json:"$schema,omitempty"`
	SchemaVersion string     `json:"schema_version"`
	GeneratedAt   string     `json:"generated_at"`
	Module        string     `json:"module"`
	Topics        []TopicDoc `json:"topics"`
}

func main() {
	rootDir, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to locate project root: %v\n", err)
		os.Exit(1)
	}

	moduleName, err := parseModuleName(filepath.Join(rootDir, "go.mod"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse module name: %v\n", err)
		os.Exit(1)
	}

	packages, err := scanPackages(rootDir, moduleName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to scan packages: %v\n", err)
		os.Exit(1)
	}

	generatedAt := time.Now().UTC().Format(time.RFC3339)
	moduleDoc, symbolsDoc, packageDocs := buildOutputs(moduleName, packages, generatedAt)

	outputDir := filepath.Join(rootDir, ".yanling")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	if err := cleanupOutputDir(outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to cleanup output directory: %v\n", err)
		os.Exit(1)
	}

	if err := writeJSON(filepath.Join(outputDir, "module.json"), moduleDoc); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write module.json: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSON(filepath.Join(outputDir, "symbols.json"), symbolsDoc); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write symbols.json: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSON(filepath.Join(outputDir, "symbols.lite.json"), buildSymbolsLite(symbolsDoc)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write symbols.lite.json: %v\n", err)
		os.Exit(1)
	}

	packagesDir := filepath.Join(outputDir, "packages")
	if err := os.MkdirAll(packagesDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create packages output directory: %v\n", err)
		os.Exit(1)
	}
	for _, pkg := range packageDocs {
		if err := writeJSON(filepath.Join(packagesDir, packageFileName(pkg.Package.ImportPath)), pkg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write package file for %s: %v\n", pkg.Package.ImportPath, err)
			os.Exit(1)
		}
	}

	fmt.Printf("generated %s\n", filepath.Join(outputDir, "module.json"))
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "symbols.json"))
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "symbols.lite.json"))
	fmt.Printf("generated %s\n", packagesDir)

	symbolIndex := buildSymbolIndex(packages)
	topicDocs, err := scanTopics(rootDir, moduleName, symbolIndex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to scan topics: %v\n", err)
		os.Exit(1)
	}
	topicsOutput := TopicsOutput{
		SchemaRef:     topicsSchemaRef,
		SchemaVersion: schemaVersion,
		GeneratedAt:   generatedAt,
		Module:        moduleName,
		Topics:        topicDocs,
	}
	if err := writeJSON(filepath.Join(outputDir, "topics.json"), topicsOutput); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write topics.json: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "topics.json"))
}

func findProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}

		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("go.mod not found")
		}
		wd = parent
	}
}

func parseModuleName(goModPath string) (string, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "module ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}

	return "", errors.New("module line not found in go.mod")
}

func scanPackages(rootDir, moduleName string) ([]*packageAggregate, error) {
	fset := token.NewFileSet()
	packages := make(map[string]*packageAggregate)

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

		relPath, err := filepath.Rel(rootDir, filePath)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		fileAst, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relPath, err)
		}

		relDir := filepath.ToSlash(filepath.Dir(relPath))
		if relDir == "." {
			relDir = ""
		}
		importPath := moduleName
		if relDir != "" {
			importPath = moduleName + "/" + relDir
		}

		pkg := packages[importPath]
		if pkg == nil {
			pkg = &packageAggregate{
				Name:           fileAst.Name.Name,
				ImportPath:     importPath,
				RelDir:         relDir,
				PackageImports: make(map[string]map[string]struct{}),
				Symbols:        make(map[string]*SymbolDoc),
				PendingMethods: make(map[string][]MethodDoc),
			}
			packages[importPath] = pkg
		}
		pkg.Files = append(pkg.Files, relPath)
		if pkg.Doc == "" {
			pkg.Doc = cleanComment(pickDoc(fileAst.Doc, nil))
		}

		ctx := fileContext{
			RelPath: relPath,
			Imports: buildImportMap(fileAst),
		}
		mergePackageImports(pkg, ctx.Imports)
		extractDeclarations(pkg, fileAst, ctx, fset)
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]*packageAggregate, 0, len(packages))
	for _, pkg := range packages {
		finalizePackage(pkg)
		result = append(result, pkg)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ImportPath < result[j].ImportPath
	})
	return result, nil
}

func shouldSkipDir(rootDir, dirPath, name string) bool {
	if dirPath == rootDir {
		return false
	}

	if name == ".git" || strings.HasPrefix(name, ".") {
		return true
	}

	relPath, err := filepath.Rel(rootDir, dirPath)
	if err != nil {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	if relPath == "." {
		return false
	}

	topLevelDir := strings.Split(relPath, "/")[0]
	_, excluded := excludedTopLevelDirs[topLevelDir]
	return excluded
}

func buildImportMap(fileAst *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, spec := range fileAst.Imports {
		importPath := strings.Trim(spec.Path.Value, "\"")
		alias := path.Base(importPath)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		imports[alias] = importPath
	}
	return imports
}

func mergePackageImports(pkg *packageAggregate, imports map[string]string) {
	for alias, importPath := range imports {
		aliases := pkg.PackageImports[importPath]
		if aliases == nil {
			aliases = make(map[string]struct{})
			pkg.PackageImports[importPath] = aliases
		}
		aliases[alias] = struct{}{}
	}
}

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

func buildOutputs(moduleName string, packages []*packageAggregate, generatedAt string) (ModuleOutput, SymbolsOutput, []PackageOutput) {
	moduleDoc := ModuleOutput{
		SchemaRef:     moduleSchemaRef,
		SchemaVersion: schemaVersion,
		GeneratedAt:   generatedAt,
		Module:        moduleName,
		Files: ModuleFilesDoc{
			SymbolIndex:     "symbols.json",
			SymbolIndexLite: "symbols.lite.json",
			PackageDir:      "packages",
		},
		Packages: make([]ModulePackageEntry, 0, len(packages)),
	}

	symbolsDoc := SymbolsOutput{
		SchemaRef:     symbolsSchemaRef,
		SchemaVersion: schemaVersion,
		GeneratedAt:   generatedAt,
		Module:        moduleName,
		Symbols:       make([]SymbolIndexEntry, 0),
	}

	packageDocs := make([]PackageOutput, 0, len(packages))
	allCounts := CountsDoc{Packages: len(packages)}

	for _, pkg := range packages {
		symbols := sortedSymbols(pkg.Symbols)
		pkgCounts := countSymbols(symbols)
		dependencies := packageDependencies(symbols, pkg.ImportPath)
		packageFile := filepath.ToSlash(filepath.Join("packages", packageFileName(pkg.ImportPath)))

		moduleDoc.Packages = append(moduleDoc.Packages, ModulePackageEntry{
			Name:         pkg.Name,
			ImportPath:   pkg.ImportPath,
			Directory:    packageDirectory(pkg.RelDir),
			Doc:          oneLineDoc(pkg.Doc),
			Counts:       pkgCounts,
			PackageFile:  packageFile,
			Dependencies: dependencies,
		})

		packageDocs = append(packageDocs, PackageOutput{
			SchemaRef:     packageSchemaRef,
			SchemaVersion: schemaVersion,
			GeneratedAt:   generatedAt,
			Module:        moduleName,
			Package: PackageDoc{
				Name:         pkg.Name,
				ImportPath:   pkg.ImportPath,
				Directory:    packageDirectory(pkg.RelDir),
				Doc:          pkg.Doc,
				Files:        append([]string{}, pkg.Files...),
				Imports:      packageImports(pkg.PackageImports),
				Dependencies: dependencies,
				Counts:       pkgCounts,
				Symbols:      cloneSymbols(symbols),
			},
		})

		for _, symbol := range symbols {
			symbolsDoc.Symbols = append(symbolsDoc.Symbols, SymbolIndexEntry{
				ID:            symbol.ID,
				Name:          symbol.Name,
				QualifiedName: symbol.QualifiedName,
				Kind:          symbol.Kind,
				Package:       symbol.Package,
				ImportPath:    symbol.ImportPath,
				Doc:           oneLineDoc(symbol.Doc),
				Signature:     symbol.Signature,
				Source:        symbol.Source,
				PackageFile:   packageFile,
				TypeRefs:      append([]TypeRefDoc{}, symbol.TypeRefs...),
				MethodCount:   len(symbol.Methods),
				FieldCount:    len(symbol.Fields),
			})
		}

		accumulateCounts(&allCounts, pkgCounts)
	}

	moduleDoc.Counts = allCounts
	symbolsDoc.Counts = allCounts
	return moduleDoc, symbolsDoc, packageDocs
}

func buildSymbolsLite(full SymbolsOutput) SymbolsLiteOutput {
	lite := SymbolsLiteOutput{
		SchemaVersion: full.SchemaVersion,
		GeneratedAt:   full.GeneratedAt,
		Module:        full.Module,
		Counts:        full.Counts,
		Symbols:       make([]SymbolLiteIndexEntry, 0, len(full.Symbols)),
	}

	for _, symbol := range full.Symbols {
		lite.Symbols = append(lite.Symbols, SymbolLiteIndexEntry{
			ID:            symbol.ID,
			Name:          symbol.Name,
			QualifiedName: symbol.QualifiedName,
			Kind:          symbol.Kind,
			Package:       symbol.Package,
			ImportPath:    symbol.ImportPath,
			Source:        symbol.Source,
			PackageFile:   symbol.PackageFile,
		})
	}

	return lite
}

func sortedSymbols(items map[string]*SymbolDoc) []*SymbolDoc {
	symbols := make([]*SymbolDoc, 0, len(items))
	for _, symbol := range items {
		symbols = append(symbols, symbol)
	}
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Name == symbols[j].Name {
			return symbols[i].Kind < symbols[j].Kind
		}
		return symbols[i].Name < symbols[j].Name
	})
	return symbols
}

func cloneSymbols(symbols []*SymbolDoc) []SymbolDoc {
	result := make([]SymbolDoc, 0, len(symbols))
	for _, symbol := range symbols {
		cloned := *symbol
		cloned.Params = append([]ParamDoc{}, symbol.Params...)
		cloned.Results = append([]ParamDoc{}, symbol.Results...)
		cloned.Fields = append([]FieldDoc{}, symbol.Fields...)
		cloned.Methods = append([]MethodDoc{}, symbol.Methods...)
		cloned.Embeds = append([]EmbeddedTypeDoc{}, symbol.Embeds...)
		cloned.TypeRefs = append([]TypeRefDoc{}, symbol.TypeRefs...)
		result = append(result, cloned)
	}
	return result
}

func countSymbols(symbols []*SymbolDoc) CountsDoc {
	counts := CountsDoc{Symbols: len(symbols)}
	for _, symbol := range symbols {
		switch symbol.Kind {
		case "interface":
			counts.Interfaces++
		case "struct":
			counts.Structs++
		case "function":
			counts.Functions++
		case "named_type":
			counts.NamedTypes++
		case "type_alias":
			counts.TypeAliases++
		case "func_type":
			counts.FuncTypes++
		case "const":
			counts.Consts++
		}
		counts.Methods += len(symbol.Methods)
	}
	return counts
}

func accumulateCounts(total *CountsDoc, current CountsDoc) {
	total.Symbols += current.Symbols
	total.Interfaces += current.Interfaces
	total.Structs += current.Structs
	total.Functions += current.Functions
	total.Methods += current.Methods
	total.NamedTypes += current.NamedTypes
	total.TypeAliases += current.TypeAliases
	total.FuncTypes += current.FuncTypes
	total.Consts += current.Consts
}

func packageDependencies(symbols []*SymbolDoc, localImportPath string) []string {
	deps := make(map[string]struct{})
	for _, symbol := range symbols {
		for _, ref := range symbol.TypeRefs {
			if ref.ImportPath == "" || ref.ImportPath == localImportPath {
				continue
			}
			deps[ref.ImportPath] = struct{}{}
		}
	}
	items := make([]string, 0, len(deps))
	for dep := range deps {
		items = append(items, dep)
	}
	sort.Strings(items)
	return items
}

func packageImports(imports map[string]map[string]struct{}) []PackageImport {
	items := make([]PackageImport, 0, len(imports))
	for importPath, aliasesMap := range imports {
		aliases := make([]string, 0, len(aliasesMap))
		for alias := range aliasesMap {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		items = append(items, PackageImport{Path: importPath, Aliases: aliases})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})
	return items
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

func buildSignature(name string, params, results []ParamDoc) string {
	paramParts := make([]string, 0, len(params))
	for _, p := range params {
		if p.Name == "" {
			paramParts = append(paramParts, p.Type)
		} else {
			paramParts = append(paramParts, p.Name+" "+p.Type)
		}
	}

	resultParts := make([]string, 0, len(results))
	for _, r := range results {
		if r.Name == "" {
			resultParts = append(resultParts, r.Type)
		} else {
			resultParts = append(resultParts, r.Name+" "+r.Type)
		}
	}

	sig := fmt.Sprintf("%s(%s)", name, strings.Join(paramParts, ", "))
	switch len(resultParts) {
	case 0:
		return sig
	case 1:
		return sig + " " + resultParts[0]
	default:
		return sig + " (" + strings.Join(resultParts, ", ") + ")"
	}
}

func buildTypeSignature(name string, alias bool, underlying string) string {
	if alias {
		return "type " + name + " = " + underlying
	}
	return "type " + name + " " + underlying
}

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

// buildSymbolIndex creates a qualified-name → *SymbolDoc map for fast type lookup.
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

		// Map: comment-group end line → cleaned comment text.
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
			Doc:       raw.doc,
			Direction: "publish",
			Payload:   payload,
		}
		if raw.payloadTypeName != "" && ast.IsExported(raw.payloadTypeName) {
			entry.GoStructName = raw.payloadTypeName
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

// buildTopicPayload constructs the root TopicFieldSchema for a named struct type.
func buildTopicPayload(typeName, importPath string, symbolIndex map[string]*SymbolDoc) TopicFieldSchema {
	if typeName == "" || importPath == "" {
		return TopicFieldSchema{Type: "object", Properties: map[string]*TopicFieldSchema{}}
	}
	qname := qualifiedName(importPath, typeName)
	symbol, ok := symbolIndex[qname]
	if !ok || symbol.Kind != "struct" {
		return TopicFieldSchema{Type: "object", GoTypeHint: typeName, Properties: map[string]*TopicFieldSchema{}}
	}
	return buildStructPayload(symbol.Fields, symbolIndex, 0)
}

// buildStructPayload converts a slice of FieldDoc into a TopicFieldSchema of type "object".
func buildStructPayload(fields []FieldDoc, symbolIndex map[string]*SymbolDoc, depth int) TopicFieldSchema {
	props := make(map[string]*TopicFieldSchema)
	required := make([]string, 0)

	for _, field := range fields {
		if field.Embedded {
			continue
		}
		jsonKey := extractJSONFieldName(field)
		if jsonKey == "" || jsonKey == "-" {
			continue
		}
		isOmitempty := strings.Contains(field.Tag, "omitempty")
		fieldSchema := goTypeToFieldSchema(field.Type, field.TypeRefs, field.Doc, symbolIndex, depth)
		props[jsonKey] = &fieldSchema
		if !isOmitempty {
			required = append(required, jsonKey)
		}
	}
	sort.Strings(required)

	falseVal := false
	return TopicFieldSchema{
		Type:                 "object",
		Properties:           props,
		Required:             required,
		AdditionalProperties: &falseVal,
	}
}

// extractJSONFieldName returns the JSON key for a struct field by reading its json tag,
// falling back to the field name when no tag is present.
func extractJSONFieldName(field FieldDoc) string {
	if field.Tag != "" {
		idx := strings.Index(field.Tag, `json:"`)
		if idx >= 0 {
			rest := field.Tag[idx+6:]
			end := strings.Index(rest, `"`)
			if end > 0 {
				parts := strings.SplitN(rest[:end], ",", 2)
				if parts[0] != "" {
					return parts[0]
				}
			}
		}
	}
	return field.Name
}

// goTypeToFieldSchema maps a Go type string to a TopicFieldSchema.
// Named types are resolved recursively via symbolIndex up to depth 6.
func goTypeToFieldSchema(goType string, typeRefs []TypeRefDoc, doc string, symbolIndex map[string]*SymbolDoc, depth int) TopicFieldSchema {
	if depth > 6 {
		return TopicFieldSchema{Type: "object", Description: doc}
	}

	// Pointer: strip * and mark the inner schema as nullable.
	if strings.HasPrefix(goType, "*") {
		inner := goTypeToFieldSchema(goType[1:], typeRefs, doc, symbolIndex, depth)
		inner.Nullable = true
		return inner
	}

	// Primitive types.
	switch goType {
	case "string":
		return TopicFieldSchema{Type: "string", Description: doc}
	case "bool":
		return TopicFieldSchema{Type: "boolean", Description: doc}
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "byte", "rune":
		return TopicFieldSchema{Type: "integer", Description: doc}
	case "float32", "float64":
		return TopicFieldSchema{Type: "number", Description: doc}
	case "any", "interface{}":
		return TopicFieldSchema{Description: doc}
	case "time.Time":
		return TopicFieldSchema{Type: "string", Format: "date-time", GoTypeHint: "time.Time", Description: doc}
	}

	// Slice: []T → array with items schema.
	if strings.HasPrefix(goType, "[]") {
		innerType := goType[2:]
		var innerRefs []TypeRefDoc
		for _, ref := range typeRefs {
			if !ref.Builtin {
				innerRefs = append(innerRefs, ref)
			}
		}
		innerSchema := goTypeToFieldSchema(innerType, innerRefs, "", symbolIndex, depth)
		return TopicFieldSchema{Type: "array", Items: &innerSchema, Description: doc}
	}

	// Map: map[K]V → generic object with go_type_hint.
	if strings.HasPrefix(goType, "map[") {
		return TopicFieldSchema{Type: "object", GoTypeHint: goType, Description: doc}
	}

	// Named types: look up via typeRefs → symbolIndex.
	for _, ref := range typeRefs {
		if ref.Builtin || ref.QualifiedName == "" {
			continue
		}
		// Special case: time.Time from the standard library.
		if ref.ImportPath == "time" && ref.QualifiedName == "time.Time" {
			return TopicFieldSchema{Type: "string", Format: "date-time", GoTypeHint: "time.Time", Description: doc}
		}
		symbol, ok := symbolIndex[ref.QualifiedName]
		if !ok {
			continue
		}
		switch symbol.Kind {
		case "struct":
			result := buildStructPayload(symbol.Fields, symbolIndex, depth+1)
			result.Description = doc
			return result
		case "named_type":
			return goTypeToFieldSchema(symbol.UnderlyingType, symbol.TypeRefs, doc, symbolIndex, depth)
		case "type_alias":
			return goTypeToFieldSchema(symbol.UnderlyingType, symbol.TypeRefs, doc, symbolIndex, depth)
		}
	}

	// Unknown type: preserve as go_type_hint for downstream code generators.
	return TopicFieldSchema{GoTypeHint: goType, Description: doc}
}
