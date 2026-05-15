package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// yanlingModuleFiles 定义 yanling script module 所需的文件
var yanlingModuleFiles = []string{
	"config.json",
	"index.json",
	"module.json",
	"symbols.json",
	"script.go",
}

// checkYanlingModule 检查指定的模块是否符合 yanling script module 标准
func checkYanlingModule(goModCache, module, version string) error {
	// 构建 .yanling 目录路径
	yanlingDir := filepath.Join(goModCache, fmt.Sprintf("%s@%s", module, version), ".yanling")

	// 检查 .yanling 目录是否存在
	info, err := os.Stat(yanlingDir)
	if err != nil {
		return fmt.Errorf(".yanling directory not found: %s", yanlingDir)
	}
	if !info.IsDir() {
		return fmt.Errorf(".yanling is not a directory: %s", yanlingDir)
	}

	// 根据 module 类型确定需要检查的文件
	requiredFiles := yanlingModuleFiles

	// yanlingrpa.com/yanling/protocol 是协议表述 module，不需要 config.json 和 script.go
	if module == "yanlingrpa.com/yanling/protocol" {
		requiredFiles = []string{
			"index.json",
			"module.json",
			"symbols.json",
		}
	}

	// 检查所有必需的文件
	missingFiles := []string{}
	for _, fileName := range requiredFiles {
		filePath := filepath.Join(yanlingDir, fileName)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			missingFiles = append(missingFiles, fileName)
		}
	}

	if len(missingFiles) > 0 {
		return fmt.Errorf("missing required files: %s", strings.Join(missingFiles, ", "))
	}

	return nil
}
