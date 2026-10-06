package tool

import (
	"agent/config"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runRead(input map[string]any) (string, error) {
	// path string, limit int
	path, err := safePath(input)
	if err != nil {
		return "", err
	}
	limit := 0

	if v, exists := input["limit"]; exists {
		switch n := v.(type) {
		case float64:
			limit = int(n)
		case int:
			limit = n
		default:
			return "", fmt.Errorf("limit must be a number")
		}
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(file)
	if limit > 0 && len(file) > limit {
		text = string(text)[:limit]
	}
	return text, nil
}

func runWrite(input map[string]any) (string, error) {
	// path string, content string
	path, err := safePath(input)
	if err != nil {
		return "", err
	}

	content, err := requireString(input, "content", "content is required")
	if err != nil {
		return "", err
	}
	err = os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %v bytes to %v\n", len([]byte(content)), path), nil
}

func runEdit(input map[string]any) (string, error) {
	// path string, old_text string, new_text string
	path, err := safePath(input)
	if err != nil {
		return "", err
	}
	oldText, err := requireString(input, "old_text", "old_text is required")
	if err != nil {
		return "", err
	}
	newText, err := requireString(input, "new_text", "new_text is required")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(data)
	if !strings.Contains(text, oldText) {
		return "", fmt.Errorf("text not found")
	}
	text = strings.Replace(text, oldText, newText, 1)
	err = os.WriteFile(path, []byte(text), 0644)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Edited %v\n", path), nil
}

func runGlob(input map[string]any) (string, error) {
	// pattern string
	pattern, err := requireString(input, "pattern", "pattern is required")
	if err != nil {
		return "", err
	}
	path := filepath.Join(config.WORKDIR, pattern)
	matches, err := filepath.Glob(path)
	if err != nil {
		return "", err
	}
	for i, m := range matches {
		rel, err := filepath.Rel(config.WORKDIR, m)
		if err == nil {
			matches[i] = rel
		}
	}

	return strings.Join(matches, "\n"), nil
}

func safePath(input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(config.WORKDIR, path)
	}
	path = filepath.Clean(path)

	rel, err := filepath.Rel(config.WORKDIR, path)
	if err != nil {
		return "", err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}

	return path, nil
}
