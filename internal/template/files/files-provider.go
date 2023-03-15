package files

import (
	"fmt"
	"os"
	"path/filepath"
)

const ReadFileFunc = "readF"

func ReadFile(args ...string) (string, error) {
	switch len(args) {
	case 1:
		return readFile(args[0], nil)
	case 2:
		return readFile(args[0], &args[1])
	default:
		return "", fmt.Errorf("wrong arguments count for function %q expected 1 or 2 aruments but got %d", ReadFileFunc, len(args))
	}
}

func readFile(relPath string, fallback *string) (string, error) {
	if !filepath.IsLocal(relPath) {
		return "", fmt.Errorf("given filepath %s is not relative (local) path", relPath)
	}
	content, err := os.ReadFile(relPath)
	if err != nil {
		if fallback != nil {
			return *fallback, nil
		} else {
			return "", fmt.Errorf("reading file %s failerd error: %s", relPath, err)
		}
	}
	return string(content), nil
}
