package main

import (
	"encoding/json"
	"os"
)

func writeJSON(path string, data any) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(path, content, 0644)
}
