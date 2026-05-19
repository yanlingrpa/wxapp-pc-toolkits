package main

import (
	"errors"
	"os"
	"path/filepath"
)

func findProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	current := wd
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("go.mod not found in current directory or parent directories")
		}
		current = parent
	}
}
