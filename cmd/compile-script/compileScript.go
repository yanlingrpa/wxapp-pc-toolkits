package main

import (
	"fmt"
	"os"
	"strings"
)

var excludedTopLevelDirs = map[string]struct{}{
	".git":      {},
	".vscode":   {},
	".protocol": {},
	".yanling":  {},
	"cmd":       {},
	"doc":       {},
	"tests":     {},
	"symbols":   {},
	"schema":    {},
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <script.go>\n", os.Args[0])
		os.Exit(1)
	}
	scriptPath := os.Args[1]
	if !strings.HasSuffix(scriptPath, ".go") {
		fmt.Fprintf(os.Stderr, "input file must be a .go file\n")
		os.Exit(1)
	}
}
