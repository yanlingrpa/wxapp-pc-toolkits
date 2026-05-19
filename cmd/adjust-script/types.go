package main

type goModInfo struct {
	Requires map[string]string
}

type importInfo struct {
	Alias      string
	ImportPath string
	ModulePath string
	Version    string
	Keep       bool
}

type firstResultKind int

const (
	resultUnknown firstResultKind = iota
	resultPrimitive
	resultStruct
)

type functionSignature struct {
	FirstResultKind       firstResultKind
	FirstResultType       string
	FirstResultStructName string
}

type exportTypeDecl struct {
	Name string
	Code string
}

type moduleCache struct {
	goModCache       string
	exportTypesByMod map[string]map[string]exportTypeDecl
	funcSigByMod     map[string]map[string]functionSignature
}
