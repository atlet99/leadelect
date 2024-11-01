package node

import (
	"fmt"
	"path/filepath"
)

// absPath converts a relative path to an absolute path and returns an error if conversion fails.
func absPath(rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return rel, nil
	}

	abs, err := filepath.Abs(rel)
	if err != nil {
		return "", fmt.Errorf("failed to convert %s to absolute path: %w", rel, err)
	}
	return abs, nil
}
