package main

import "go/ast"

var excludedTopLevelDirs = map[string]struct{}{
	".git":      {},
	".vscode":   {},
	".protocol": {},
	".yanling":  {},
	"assets":    {},
	"bin":       {},
	"build":     {},
	"cmd":       {},
	"debug":     {},
	"dist":      {},
	"doc":       {},
	"docs":      {},
	"examples":  {},
	"internal":  {},
	"scripts":   {},
	"symbol":    {},
	"symbols":   {},
	"schema":    {},
	"schemas":   {},
	"testdata":  {},
	"test":      {},
	"tests":     {},
	"vendor":    {},
}

const scriptImportPath = "yanlingrpa.com/yanling/protocol/script"

type validationError struct {
	File string
	Line int
	Msg  string
}

type structDef struct {
	Name string
	File string
	Line int
	Node *ast.StructType
}

type fileInfo struct {
	AbsPath string
	RelPath string
	Ast     *ast.File
	Imports map[string]string
}

type paramInfo struct {
	Name string
	Type ast.Expr
}
