package main

import (
	"fmt"
	"sort"
	"strings"
)

// normalizeFileSectionComments moves merged file section markers like
// "// File: meituan\\basic.go" out of the import block when go/printer
// happens to attach them there.
func normalizeFileSectionComments(src []byte) []byte {
	text := string(src)
	lines := strings.Split(text, "\n")

	importStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "import (" {
			importStart = i
			break
		}
	}
	if importStart < 0 {
		return src
	}

	importEnd := -1
	for i := importStart + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == ")" {
			importEnd = i
			break
		}
	}
	if importEnd < 0 || importEnd <= importStart+1 {
		return src
	}

	markers := make([]string, 0)
	keptImportLines := make([]string, 0, importEnd-importStart-1)
	for i := importStart + 1; i < importEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "// File:") {
			markers = append(markers, trimmed)
			continue
		}
		keptImportLines = append(keptImportLines, lines[i])
	}

	if len(markers) == 0 {
		return src
	}

	rest := lines[importEnd+1:]
	for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
		rest = rest[1:]
	}

	rebuilt := make([]string, 0, len(lines)+len(markers)+1)
	rebuilt = append(rebuilt, lines[:importStart+1]...)
	rebuilt = append(rebuilt, keptImportLines...)
	rebuilt = append(rebuilt, lines[importEnd])
	rebuilt = append(rebuilt, "")
	rebuilt = append(rebuilt, markers...)
	rebuilt = append(rebuilt, rest...)

	return []byte(strings.Join(rebuilt, "\n"))
}

// annotateExternalTypeGroupComments injects one marker comment per external module,
// right before that module's first appended type declaration.
func annotateExternalTypeGroupComments(src []byte, requiredTypes map[string]map[string]struct{}) []byte {
	if len(requiredTypes) == 0 {
		return src
	}

	lines := strings.Split(string(src), "\n")
	if len(lines) == 0 {
		return src
	}

	typeLineByName := make(map[string]int)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "type ") {
			continue
		}
		rest := strings.TrimPrefix(trimmed, "type ")
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		typeLineByName[fields[0]] = i
	}

	type insertion struct {
		line    int
		comment string
	}
	insertions := make([]insertion, 0)
	for modulePath, names := range requiredTypes {
		firstLine := -1
		for typeName := range names {
			lineIdx, ok := typeLineByName[typeName]
			if !ok {
				continue
			}
			if firstLine < 0 || lineIdx < firstLine {
				firstLine = lineIdx
			}
		}
		if firstLine < 0 {
			continue
		}
		comment := fmt.Sprintf("// import %q", modulePath)
		insertions = append(insertions, insertion{line: firstLine, comment: comment})
	}

	if len(insertions) == 0 {
		return src
	}

	sort.Slice(insertions, func(i, j int) bool { return insertions[i].line < insertions[j].line })

	result := make([]string, 0, len(lines)+len(insertions)*2)
	insertIdx := 0
	for i := 0; i < len(lines); i++ {
		for insertIdx < len(insertions) && insertions[insertIdx].line == i {
			comment := insertions[insertIdx].comment
			already := false
			if len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == comment {
				already = true
			}
			if !already {
				result = append(result, comment)
				if len(result) == 1 || strings.TrimSpace(result[len(result)-2]) != "" {
					result = append(result, "")
				}
			}
			insertIdx++
		}
		result = append(result, lines[i])
	}

	return []byte(strings.Join(result, "\n"))
}

// compactBlankLinesInRenamedStructs removes empty lines inside generated external
// renamed struct declarations like `type Github_com__... struct { ... }`.
func compactBlankLinesInRenamedStructs(src []byte) []byte {
	lines := strings.Split(string(src), "\n")
	if len(lines) == 0 {
		return src
	}

	result := make([]string, 0, len(lines))
	inStruct := false
	compactCurrent := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inStruct && strings.HasPrefix(trimmed, "type ") && strings.HasSuffix(trimmed, " struct {") {
			parts := strings.Fields(trimmed)
			typeName := ""
			if len(parts) >= 2 {
				typeName = parts[1]
			}
			inStruct = true
			compactCurrent = strings.Contains(typeName, "__")
			result = append(result, line)
			continue
		}

		if inStruct {
			if compactCurrent && trimmed == "" {
				continue
			}
			result = append(result, line)
			if trimmed == "}" {
				inStruct = false
				compactCurrent = false
			}
			continue
		}

		result = append(result, line)
	}

	return []byte(strings.Join(result, "\n"))
}

// ensureAdjustGeneratedHelpersSeparator inserts a stable separator comment
// before the first adjust-generated helper function region.
func ensureAdjustGeneratedHelpersSeparator(src []byte) []byte {
	text := string(src)
	marker := "func __yanling_newValuePtrByTypeName("
	idx := strings.Index(text, marker)
	if idx < 0 {
		return src
	}

	separator := "// adjust-generated helpers"
	prefix := text[:idx]
	if strings.Contains(prefix, separator) {
		return src
	}

	insert := separator + "\n"
	if len(prefix) > 0 && !strings.HasSuffix(prefix, "\n\n") {
		insert = "\n" + insert
	}

	return []byte(prefix + insert + text[idx:])
}
