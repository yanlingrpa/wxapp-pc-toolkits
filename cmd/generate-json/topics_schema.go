package main

import (
	"sort"
	"strings"
)

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

	// Slice: []T -> array with items schema.
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

	// Map: map[K]V -> generic object with go_type_hint.
	if strings.HasPrefix(goType, "map[") {
		return TopicFieldSchema{Type: "object", GoTypeHint: goType, Description: doc}
	}

	// Named types: look up via typeRefs -> symbolIndex.
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
