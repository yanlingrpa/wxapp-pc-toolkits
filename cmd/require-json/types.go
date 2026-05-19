package main

const schemaVersion = "yanling.machine-first/v1"

type ModuleRequire struct {
	Module  string
	Version string
}

type IndexOutput struct {
	SchemaVersion string              `json:"schema_version"`
	GeneratedAt   string              `json:"generated_at"`
	Modules       []IndexModuleEntry  `json:"modules"`
	Packages      []IndexPackageEntry `json:"packages"`
	Topics        []IndexTopicEntry   `json:"topics"`
	Symbols       []IndexSymbolEntry  `json:"symbols"`
}

type IndexModuleEntry struct {
	Module string        `json:"module"`
	Files  IndexFilesDoc `json:"files"`
}

type IndexFilesDoc struct {
	SymbolIndex     string `json:"symbol_index"`
	SymbolIndexLite string `json:"symbol_index_lite,omitempty"`
	PackageDir      string `json:"package_dir"`
	Topics          string `json:"topics"`
}

type IndexPackageEntry struct {
	Module      string `json:"module"`
	Name        string `json:"name"`
	ImportPath  string `json:"import_path"`
	Directory   string `json:"directory"`
	Doc         string `json:"doc,omitempty"`
	PackageFile string `json:"package_file"`
}

type IndexTopicEntry struct {
	Module       string `json:"module"`
	Name         string `json:"name"`
	Specifier    string `json:"specifier"`
	GoStructName string `json:"go_struct_name,omitempty"`
	GoImportPath string `json:"go_import_path,omitempty"`
	Doc          string `json:"doc,omitempty"`
}

type IndexSymbolEntry struct {
	Module      string `json:"module"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	ImportPath  string `json:"import_path"`
	Package     string `json:"package"`
	Doc         string `json:"doc,omitempty"`
	PackageFile string `json:"package_file"`
}
