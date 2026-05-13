package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

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

func buildIndexOutput(moduleDoc ModuleOutput, symbolsDoc SymbolsOutput, topicDocs []TopicDoc) IndexOutput {
	files := IndexFilesDoc{
		SymbolIndex:     moduleDoc.Files.SymbolIndex,
		SymbolIndexLite: moduleDoc.Files.SymbolIndexLite,
		PackageDir:      moduleDoc.Files.PackageDir,
		Topics:          "topics.json",
	}
	if files.SymbolIndex == "" {
		files.SymbolIndex = "symbols.json"
	}
	if files.SymbolIndexLite == "" {
		files.SymbolIndexLite = "symbols.lite.json"
	}
	if files.PackageDir == "" {
		files.PackageDir = "packages"
	}

	index := IndexOutput{
		SchemaRef:     indexSchemaRef,
		SchemaVersion: moduleDoc.SchemaVersion,
		GeneratedAt:   moduleDoc.GeneratedAt,
		Modules: []IndexModuleEntry{
			{Module: moduleDoc.Module, Files: files},
		},
		Packages: make([]IndexPackageEntry, 0, len(moduleDoc.Packages)),
		Topics:   make([]IndexTopicEntry, 0, len(topicDocs)),
		Symbols:  make([]IndexSymbolEntry, 0, len(symbolsDoc.Symbols)),
	}

	for _, pkg := range moduleDoc.Packages {
		index.Packages = append(index.Packages, IndexPackageEntry{
			Module:      moduleDoc.Module,
			Name:        pkg.Name,
			ImportPath:  pkg.ImportPath,
			Directory:   pkg.Directory,
			Doc:         pkg.Doc,
			PackageFile: pkg.PackageFile,
		})
	}

	for _, topic := range topicDocs {
		index.Topics = append(index.Topics, IndexTopicEntry{
			Module:       moduleDoc.Module,
			Name:         topic.Name,
			Specifier:    topic.Specifier,
			GoStructName: topic.GoStructName,
			GoImportPath: topic.GoImportPath,
			Doc:          oneLineDoc(topic.Doc),
		})
	}

	for _, symbol := range symbolsDoc.Symbols {
		index.Symbols = append(index.Symbols, IndexSymbolEntry{
			Module:      moduleDoc.Module,
			Name:        symbol.Name,
			Kind:        symbol.Kind,
			ImportPath:  symbol.ImportPath,
			Package:     symbol.Package,
			Doc:         symbol.Doc,
			PackageFile: symbol.PackageFile,
		})
	}

	return index
}
