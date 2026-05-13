package main

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
	Specifier    string           `json:"specifier"`
	Doc          string           `json:"doc,omitempty"`
	Direction    string           `json:"direction,omitempty"`
	GoStructName string           `json:"go_struct_name,omitempty"`
	GoImportPath string           `json:"go_import_path,omitempty"`
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

type IndexOutput struct {
	SchemaRef     string              `json:"$schema,omitempty"`
	SchemaVersion string              `json:"schema_version"`
	GeneratedAt   string              `json:"generated_at"`
	Modules       []IndexModuleEntry  `json:"modules"`
	Packages      []IndexPackageEntry `json:"packages"`
	Topics        []IndexTopicEntry   `json:"topics"`
	Symbols       []IndexSymbolEntry  `json:"symbols"`
}

// IndexModuleEntry holds per-module metadata and its local file references.
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
