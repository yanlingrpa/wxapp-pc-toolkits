package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func readIndexFile(path string) (IndexOutput, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return IndexOutput{}, err
	}

	var output IndexOutput
	if err := json.Unmarshal(content, &output); err != nil {
		return IndexOutput{}, fmt.Errorf("invalid json in %s: %w", path, err)
	}

	if output.SchemaVersion == "" {
		return IndexOutput{}, fmt.Errorf("schema_version is empty in %s", path)
	}

	return output, nil
}

func mergeIndex(
	dst *IndexOutput,
	src IndexOutput,
	seenModules map[string]struct{},
	seenPackages map[string]struct{},
	seenTopics map[string]struct{},
	seenSymbols map[string]struct{},
) {
	for _, item := range src.Modules {
		key := item.Module
		if _, ok := seenModules[key]; ok {
			continue
		}
		seenModules[key] = struct{}{}
		dst.Modules = append(dst.Modules, item)
	}

	for _, item := range src.Packages {
		key := item.Module + "|" + item.ImportPath + "|" + item.Name
		if _, ok := seenPackages[key]; ok {
			continue
		}
		seenPackages[key] = struct{}{}
		dst.Packages = append(dst.Packages, item)
	}

	for _, item := range src.Topics {
		key := item.Module + "|" + item.Specifier + "|" + item.Name
		if _, ok := seenTopics[key]; ok {
			continue
		}
		seenTopics[key] = struct{}{}
		dst.Topics = append(dst.Topics, item)
	}

	for _, item := range src.Symbols {
		key := item.Module + "|" + item.ImportPath + "|" + item.Name + "|" + item.Kind
		if _, ok := seenSymbols[key]; ok {
			continue
		}
		seenSymbols[key] = struct{}{}
		dst.Symbols = append(dst.Symbols, item)
	}
}
