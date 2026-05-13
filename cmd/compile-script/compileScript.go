// mainImpl 拆分后的主流程
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// main 是 compile-script 命令的入口函数。
//
// 该命令的目标：
// 1) 扫描项目 Go 源码并合并为 .yanling/script.go（统一执行脚本内容）。
// 2) 提取对外暴露且被方法参数/Publish payload 使用到的 struct。
// 3) 递归补齐这些 struct 的依赖 struct，并统一重命名后输出到 .yanling/export.go。
//
// 失败策略：任一步骤失败都立即输出错误并退出（exit code = 1），避免产生不完整产物。
func main() {
	// 参数约定：仅接受一个参数 root_dir，表示要扫描的项目根目录。
	// 示例：go run ./cmd/compile-script .
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <root_dir>\n", os.Args[0])
		os.Exit(1)
	}

	// rootDir 是后续所有扫描、解析、输出的基准目录。
	rootDir := os.Args[1]

	// Step 1: 读取 go.mod 中的 module 名称。
	// moduleName 会用于后续 struct 重命名，确保生成名具备全局唯一前缀。
	moduleName, err := parseModuleName(filepath.Join(rootDir, "go.mod"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse module name: %v\n", err)
		os.Exit(1)
	}

	// Step 1.1: 从 .yanling/config.json 读取目标 package 名。
	// 只有该 package 的 Go 文件会参与合并和导出提取。
	targetPackage, err := loadTargetPackageName(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load target package from config: %v\n", err)
		os.Exit(1)
	}

	// Step 2: 收集项目内参与编译脚本的 Go 文件。
	// 会跳过 excludedTopLevelDirs 中声明的顶层目录，以及 *_test.go。
	goFiles, err := collectGoFiles(rootDir, targetPackage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to collect go files: %v\n", err)
		os.Exit(1)
	}

	// Step 3: 将收集到的文件合并为单一脚本内容 scriptContent。
	// 合并逻辑会统一包名、汇总 imports，并按文件顺序拼接声明。
	scriptContent, err := mergeGoFiles(rootDir, goFiles, moduleName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to merge go files: %v\n", err)
		os.Exit(1)
	}

	// Step 4: 提取需要导出的 struct，并记录来源（method / publish）。
	// exportedStructs 是初始集合：
	// - method: 来自公开方法参数中的 struct
	// - publish: 来自 rt.Publish(...) payload 中的 struct
	exportedStructs := make(map[string]*ExportedStruct)

	// 4.1 提取 method 来源 struct（含其递归依赖）
	methodStructs, err := extractMethodStructs(rootDir, targetPackage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract method structs: %v\n", err)
		os.Exit(1)
	}
	for name, s := range methodStructs {
		exportedStructs[name] = &ExportedStruct{
			Name:    name,
			Code:    s,
			Source:  "method",
			ModPath: moduleName,
		}
	}

	// 4.2 提取 publish 来源 struct（含其递归依赖）
	// 若与 method 重复，保留已有来源，不重复覆盖。
	publishStructs, err := extractPublishStructs(rootDir, targetPackage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract publish structs: %v\n", err)
		os.Exit(1)
	}
	for name, s := range publishStructs {
		if _, exists := exportedStructs[name]; !exists {
			exportedStructs[name] = &ExportedStruct{
				Name:    name,
				Code:    s,
				Source:  "publish",
				ModPath: moduleName,
			}
		}
	}

	// Step 5: 二次递归补齐 struct 依赖集合。
	// 目的：确保输出 export.go 时，不仅有入口 struct，还有字段引用到的子 struct。
	//
	// 5.1 拷贝初始导出集合。
	allStructs := make(map[string]*ExportedStruct)
	for name, s := range exportedStructs {
		allStructs[name] = s
	}
	// 5.2 扫描全项目，建立 struct 定义与 AST 索引：
	// - structDefs: struct 名 -> 源码片段
	// - astStructs: struct 名 -> AST 结构（用于继续递归字段类型）
	fset := token.NewFileSet()
	structDefs := make(map[string]string)
	astStructs := make(map[string]*ast.StructType)
	_ = filepath.WalkDir(rootDir, func(filePath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			relPath, err := filepath.Rel(rootDir, filePath)
			if err != nil {
				return nil
			}
			if relPath == "." {
				return nil
			}
			topLevelDir := strings.Split(relPath, string(os.PathSeparator))[0]
			if _, excluded := excludedTopLevelDirs[topLevelDir]; excluded {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		if !matchesTargetPackage(filePath, targetPackage) {
			return nil
		}
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return nil
		}
		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							if ts.Name.IsExported() {
								var structBuf strings.Builder
								fmt.Fprintf(&structBuf, "type ")
								// Use printer.Fprint for ast.TypeSpec
								printer.Fprint(&structBuf, fset, ts)
								structDefs[ts.Name.Name] = structBuf.String()
								astStructs[ts.Name.Name] = st
							}
						}
					}
				}
			}
		}
		return nil
	})
	// 5.3 以当前 allStructs 为起点，递归收集依赖 struct。
	collected := make(map[string]string)
	for name := range allStructs {
		collectStructDependencies(name, structDefs, collected, astStructs)
	}
	// 5.4 将新增依赖并入 allStructs，来源标记为 recursive。
	for name, code := range collected {
		if _, exists := allStructs[name]; !exists {
			allStructs[name] = &ExportedStruct{
				Name:    name,
				Code:    code,
				Source:  "recursive",
				ModPath: moduleName,
			}
		}
	}

	// Step 6: 生成 export.go 内容。
	// 会对 allStructs 中所有 struct 进行统一重命名（含 module 前缀），避免跨模块名称冲突。
	exportContent := generateExportGo(moduleName, allStructs)

	// Step 7: 落盘输出到 .yanling 目录。
	// - script.go: 合并后的脚本源码
	// - export.go: 重命名后的导出 struct 定义
	outputDir := filepath.Join(rootDir, ".yanling")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "script.go"), []byte(scriptContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write script.go: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "export.go"), []byte(exportContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write export.go: %v\n", err)
		os.Exit(1)
	}

	// Step 8: 打印输出路径，便于调用方确认产物位置。
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "script.go"))
	fmt.Printf("generated %s\n", filepath.Join(outputDir, "export.go"))
}
