package main

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
)

func main() {
	rootDir := "."
	if len(os.Args) > 2 {
		fmt.Fprintf(os.Stderr, "usage: %s [root_dir]\n", os.Args[0])
		os.Exit(1)
	}
	if len(os.Args) == 2 {
		rootDir = os.Args[1]
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve root dir: %v\n", err)
		os.Exit(1)
	}

	targetPackage, err := loadTargetPackageName(absRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load target package: %v\n", err)
		os.Exit(1)
	}

	fset := token.NewFileSet()
	files, structs, errs, scanErr := scanProject(absRoot, fset)
	if scanErr != nil {
		fmt.Fprintf(os.Stderr, "failed to scan project: %v\n", scanErr)
		os.Exit(1)
	}

	errList := validateAll(files, structs, targetPackage, fset)
	errs = append(errs, errList...)

	if len(errs) > 0 {
		printValidationErrors(errs)
		os.Exit(1)
	}

	fmt.Println("validation passed")
}
