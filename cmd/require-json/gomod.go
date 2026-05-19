package main

import (
	"os"
	"sort"
	"strings"
)

func parseGoModRequires(goModPath string) ([]ModuleRequire, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}

	requiresMap := make(map[string]string)
	lines := strings.Split(string(content), "\n")
	inRequireBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		if trimmed == "require (" {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && trimmed == ")" {
			inRequireBlock = false
			continue
		}

		if inRequireBlock {
			module, version, ok := parseRequireLine(trimmed)
			if ok {
				requiresMap[module] = version
			}
			continue
		}

		if strings.HasPrefix(trimmed, "require ") {
			module, version, ok := parseRequireLine(strings.TrimSpace(strings.TrimPrefix(trimmed, "require ")))
			if ok {
				requiresMap[module] = version
			}
		}
	}

	requires := make([]ModuleRequire, 0, len(requiresMap))
	for module, version := range requiresMap {
		requires = append(requires, ModuleRequire{Module: module, Version: version})
	}
	sort.Slice(requires, func(i, j int) bool {
		return requires[i].Module < requires[j].Module
	})

	return requires, nil
}

func parseRequireLine(line string) (string, string, bool) {
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
