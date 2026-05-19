package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	// 1) 以当前工作目录作为项目根目录。该命令通常在仓库根目录执行。
	rootDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get current directory: %v\n", err)
		os.Exit(1)
	}

	// 2) 读取 go.mod，用于把 import 路径解析到具体 module/version，
	// 后续才能去 module cache 里读取导出类型和函数签名。
	goModPath := filepath.Join(rootDir, "go.mod")
	modInfo, err := parseGoMod(goModPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", err)
		os.Exit(1)
	}

	// 3) 解析 merge.go 的 AST（保留注释），
	// 所有改写都在 AST 上进行，最后统一格式化回写。
	mergePath := filepath.Join(rootDir, ".yanling", "merge.go")
	scriptPath := filepath.Join(rootDir, ".yanling", "script.go")
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, mergePath, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", fmt.Errorf("failed to parse merge.go: %w", err))
		os.Exit(1)
	}

	// 4) 收集 import 信息并标记哪些是允许保留的标准依赖，
	// 哪些属于外部模块（需要改写并最终移除 import）。
	imports, err := collectImports(fileNode, modInfo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", err)
		os.Exit(1)
	}

	// 5) 初始化模块缓存：
	// - exportTypesByMod: 外部模块导出类型缓存
	// - funcSigByMod: 外部函数签名缓存
	// 避免重复 IO 和重复解析。
	cache := &moduleCache{
		goModCache:       getGoModCache(),
		exportTypesByMod: make(map[string]map[string]exportTypeDecl),
		funcSigByMod:     make(map[string]map[string]functionSignature),
	}

	// 6) 为每个外部模块准备 requiredTypes 集合。
	// 改写过程中一旦发现用到了外部 struct，就登记到对应模块集合里，
	// 后续统一把这些类型定义注入到 script.go。
	requiredTypes := make(map[string]map[string]struct{})
	for _, ii := range imports {
		if ii.Keep {
			continue
		}
		requiredTypes[ii.ModulePath] = make(map[string]struct{})
	}

	// 7) 遍历所有函数，做两类改写：
	// - 把外部 worker 调用改写为 rt.InvokeWorker + __yanling_convertJSONToValue
	// - 把代码块中外部类型选择器（pkg.Type）改为本地注入类型名
	for _, decl := range fileNode.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		// 只有包含 script.ModuleRuntime 形参的函数才进行 invoke 改写。
		if fd.Type != nil && fd.Type.Params != nil {
			rtNames := collectModuleRuntimeParamNames(fd.Type.Params)
			if len(rtNames) > 0 {
				fd.Body.List = rewriteBlockStatements(fd.Body.List, rtNames, imports, cache, requiredTypes)
			}
		}
		rewriteExternalTypeSelectorsInBlock(fd.Body, imports, requiredTypes)
	}

	// 8) 将 requiredTypes 对应的类型定义注入 script.go（位于 import 后）。
	if err := appendRequiredTypes(fileNode, requiredTypes, cache, imports); err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", err)
		os.Exit(1)
	}

	// 9) 收集当前脚本中的公开 struct 名称，供 helper 生成工厂 switch 使用。
	helperTypeNames := collectStructTypeNames(fileNode)

	// 9.5) 收集需要生成 Yanling_ wrapper 的 public 方法（在 helper 注入前收集，避免干扰）。
	yanlingMethods := collectYanlingMethods(fileNode)

	// 9.6) 收集 topic 订阅信息，用于生成 host 触发入口：Yanling_onTopicTrigger。
	topicSubscriptions := collectTopicSubscriptions(fileNode)

	// 10) 注入 JSON->struct 通用 helper：
	// - __yanling_newValuePtrByTypeName
	// - __yanling_convertJSONToValue
	if err := ensureJSONConvertHelper(fileNode); err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", err)
		os.Exit(1)
	}

	// 10.5) 为 public 方法注入 Yanling_ wrapper，统一以 (jsonStr, error) 为接口。
	if err := ensureYanlingWrappers(fileNode, yanlingMethods); err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", err)
		os.Exit(1)
	}

	// 10.6) 生成 host 主动触发 topic 的统一入口。
	if err := ensureYanlingTopicTriggerWrapper(fileNode, topicSubscriptions); err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", err)
		os.Exit(1)
	}

	// 11) helper 注入可能新增 import，重新收集并执行非法 import 清理。
	imports, err = collectImports(fileNode, modInfo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", err)
		os.Exit(1)
	}

	pruneIllegalImports(fileNode, imports)

	// 12) 将 AST 格式化为源码。
	var out bytes.Buffer
	if err := format.Node(&out, fset, fileNode); err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", fmt.Errorf("failed to format script.go: %w", err))
		os.Exit(1)
	}

	// 13) 再执行一次 canonical gofmt，减少由节点位置信息导致的异常换行。
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", fmt.Errorf("failed to canonicalize script.go: %w", err))
		os.Exit(1)
	}

	// 14) 以源码方式重建 helper 函数，进一步保证输出稳定性。
	formatted, err = regenerateHelperFunctionsInSource(formatted, helperTypeNames, yanlingMethods, topicSubscriptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", err)
		os.Exit(1)
	}

	// 14.5) 修正 merge 段落注释位置：避免 `// File: ...` 误落在 import 组内部。
	formatted = normalizeFileSectionComments(formatted)

	// 14.6) 为每个外部 module 的类型分组补充 import 注释，便于阅读。
	formatted = annotateExternalTypeGroupComments(formatted, requiredTypes)

	// 14.7) 压缩外部重命名 struct 内部的空行，避免字段间出现多余留白。
	formatted = compactBlankLinesInRenamedStructs(formatted)

	// 14.8) 在 adjust 生成 helper 区域前添加分隔注释，提升可读性。
	formatted = ensureAdjustGeneratedHelpersSeparator(formatted)

	// 15) 回写最终脚本。
	if err := os.WriteFile(scriptPath, formatted, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", fmt.Errorf("failed to write script.go: %w", err))
		os.Exit(1)
	}

	// 16) 最后再执行一次 gofmt，保证输出完全符合 Go 标准格式。
	if out, err := exec.Command("gofmt", "-w", scriptPath).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "adjust script failed: %v\n", fmt.Errorf("failed to gofmt script.go: %w, output: %s", err, string(out)))
		os.Exit(1)
	}

	fmt.Println("adjusted .yanling/script.go")
}
