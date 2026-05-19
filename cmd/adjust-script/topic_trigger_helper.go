package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strings"
)

type topicSubscriptionInfo struct {
	Specifier   string
	Topic       string
	HandlerName string
	PayloadType string
}

func collectTopicSubscriptions(fileNode *ast.File) []topicSubscriptionInfo {
	handlerPayloads := make(map[string]string)
	for _, decl := range fileNode.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil || fd.Name == nil || fd.Body == nil {
			continue
		}
		payloadType := extractEventPayloadType(fd)
		if payloadType == "" {
			continue
		}
		handlerPayloads[fd.Name.Name] = payloadType
	}

	type subscriptionKey struct {
		specifier string
		topic     string
		handler   string
	}
	seen := make(map[subscriptionKey]struct{})
	result := make([]topicSubscriptionInfo, 0)

	for _, decl := range fileNode.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Subscribe" {
				return true
			}
			if len(call.Args) != 3 {
				return true
			}

			specifierLit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || specifierLit.Kind != token.STRING {
				return true
			}
			topicLit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || topicLit.Kind != token.STRING {
				return true
			}
			handlerIdent, ok := call.Args[2].(*ast.Ident)
			if !ok || handlerIdent.Name == "" {
				return true
			}

			specifier := strings.Trim(specifierLit.Value, "\"")
			topic := strings.Trim(topicLit.Value, "\"")
			handler := handlerIdent.Name
			if specifier == "" || topic == "" {
				return true
			}

			key := subscriptionKey{specifier: specifier, topic: topic, handler: handler}
			if _, exists := seen[key]; exists {
				return true
			}
			seen[key] = struct{}{}

			result = append(result, topicSubscriptionInfo{
				Specifier:   specifier,
				Topic:       topic,
				HandlerName: handler,
				PayloadType: handlerPayloads[handler],
			})
			return true
		})
	}

	sort.Slice(result, func(i, j int) bool {
		ki := result[i].Specifier + "." + result[i].Topic + "|" + result[i].HandlerName
		kj := result[j].Specifier + "." + result[j].Topic + "|" + result[j].HandlerName
		return ki < kj
	})

	return result
}

func extractEventPayloadType(fd *ast.FuncDecl) string {
	if fd.Body == nil {
		return ""
	}
	var payloadType string
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if payloadType != "" {
			return false
		}
		tae, ok := n.(*ast.TypeAssertExpr)
		if !ok || tae.Type == nil {
			return true
		}
		sel, ok := tae.X.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "Data" {
			return true
		}
		payloadType, _ = extractTypeNameFromExpr(tae.Type)
		return true
	})
	return payloadType
}

func buildYanlingOnTopicTriggerSource(subscriptions []topicSubscriptionInfo) string {
	if len(subscriptions) == 0 {
		return `func Yanling_onTopicTrigger(specifierTopic string, payloadJson string, occuredAt time.Time) error {
	return nil
}
`
	}

	var b strings.Builder
	b.WriteString("func Yanling_onTopicTrigger(specifierTopic string, payloadJson string, occuredAt time.Time) error {\n")
	b.WriteString("\tswitch specifierTopic {\n")
	for i := 0; i < len(subscriptions); {
		key := subscriptions[i].Specifier + "." + subscriptions[i].Topic
		j := i
		for j < len(subscriptions) {
			candidate := subscriptions[j].Specifier + "." + subscriptions[j].Topic
			if candidate != key {
				break
			}
			j++
		}

		b.WriteString("\tcase ")
		b.WriteString(quote(key))
		b.WriteString(":\n")
		for k := i; k < j; k++ {
			sub := subscriptions[k]
			if sub.PayloadType != "" {
				b.WriteString("\t\thandlerData, err := __yanling_convertJSONToValue(payloadJson, ")
				b.WriteString(quote(sub.PayloadType))
				b.WriteString(", false)\n")
				b.WriteString("\t\tif err != nil {\n")
				b.WriteString("\t\t\treturn fmt.Errorf(\"failed to convert payload for topic %s (handler %s): %w\", specifierTopic, ")
				b.WriteString(quote(sub.HandlerName))
				b.WriteString(", err)\n")
				b.WriteString("\t\t}\n")
				b.WriteString("\t\t")
				b.WriteString(sub.HandlerName)
				b.WriteString("(script.Event{\n")
				b.WriteString("\t\t\tTopic:     ")
				b.WriteString(quote(sub.Topic))
				b.WriteString(",\n")
				b.WriteString("\t\t\tData:      handlerData,\n")
				b.WriteString("\t\t\tOccuredAt: occuredAt,\n")
				b.WriteString("\t\t})\n")
			} else {
				b.WriteString("\t\t")
				b.WriteString(sub.HandlerName)
				b.WriteString("(script.Event{\n")
				b.WriteString("\t\t\tTopic:     ")
				b.WriteString(quote(sub.Topic))
				b.WriteString(",\n")
				b.WriteString("\t\t\tData:      payloadJson,\n")
				b.WriteString("\t\t\tOccuredAt: occuredAt,\n")
				b.WriteString("\t\t})\n")
			}
		}
		b.WriteString("\t\treturn nil\n")

		i = j
	}
	b.WriteString("\tdefault:\n")
	b.WriteString("\t\treturn fmt.Errorf(\"unsupported topic trigger: %s\", specifierTopic)\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn nil\n")
	b.WriteString("}\n")
	return b.String()
}

func ensureYanlingTopicTriggerWrapper(fileNode *ast.File, subscriptions []topicSubscriptionInfo) error {
	removeFuncDeclByName(fileNode, "Yanling_onTopicTrigger")
	if len(subscriptions) > 0 {
		ensureImportPath(fileNode, "fmt")
	}
	ensureImportPath(fileNode, "time")
	src := buildYanlingOnTopicTriggerSource(subscriptions)
	decl, err := parseFuncDecl(src)
	if err != nil {
		return fmt.Errorf("failed to parse Yanling_onTopicTrigger wrapper: %w", err)
	}
	fileNode.Decls = append(fileNode.Decls, decl)
	return nil
}
