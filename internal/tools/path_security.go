package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CanonicalizePath resolves a model-supplied path against cwd and resolves all
// existing symlink components. For a new target, the deepest existing parent
// is resolved before the non-existing suffix is appended.
func CanonicalizePath(path, cwd string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		if cwd == "" {
			var err error
			cwd, err = os.Getwd()
			if err != nil {
				return "", fmt.Errorf("resolve working directory: %w", err)
			}
		}
		path = filepath.Join(cwd, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	candidate := filepath.Clean(absolute)
	existing := candidate
	var suffix []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect path %q: %w", existing, err)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		suffix = append(suffix, filepath.Base(existing))
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks for %q: %w", existing, err)
	}
	for index := len(suffix) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, suffix[index])
	}
	return filepath.Clean(resolved), nil
}

// IsPathWithin reports whether target resolves to root or one of its children.
func IsPathWithin(root, target string) bool {
	canonicalRoot, err := CanonicalizePath(root, "")
	if err != nil {
		return false
	}
	canonicalTarget, err := CanonicalizePath(target, canonicalRoot)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(canonicalRoot, canonicalTarget)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func normalizePathFields(input json.RawMessage, cwd string, fields map[string]bool) (json.RawMessage, error) {
	var object map[string]interface{}
	if err := json.Unmarshal(input, &object); err != nil {
		return nil, err
	}
	for field, defaultToCwd := range fields {
		value, exists := object[field]
		if !exists || value == nil || value == "" {
			if !defaultToCwd {
				continue
			}
			value = cwd
		}
		path, ok := value.(string)
		if !ok {
			continue // JSON Schema reports the more precise type error.
		}
		canonical, err := CanonicalizePath(path, cwd)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", field, err)
		}
		object[field] = canonical
	}
	return json.Marshal(object)
}

func inputPathFields(input json.RawMessage, fields ...string) []string {
	var object map[string]interface{}
	if err := json.Unmarshal(input, &object); err != nil {
		return nil
	}
	paths := make([]string, 0, len(fields))
	for _, field := range fields {
		if path, ok := object[field].(string); ok && path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func (t *FileReadTool) NormalizeInput(input json.RawMessage, cwd string) (json.RawMessage, error) {
	return normalizePathFields(input, cwd, map[string]bool{"target_file": false})
}
func (t *FileReadTool) InputPaths(input json.RawMessage) []string {
	return inputPathFields(input, "target_file")
}

func (t *FileWriteTool) NormalizeInput(input json.RawMessage, cwd string) (json.RawMessage, error) {
	return normalizePathFields(input, cwd, map[string]bool{"file_path": false})
}
func (t *FileWriteTool) InputPaths(input json.RawMessage) []string {
	return inputPathFields(input, "file_path")
}

func (t *FileEditTool) NormalizeInput(input json.RawMessage, cwd string) (json.RawMessage, error) {
	return normalizePathFields(input, cwd, map[string]bool{"file_path": false})
}
func (t *FileEditTool) InputPaths(input json.RawMessage) []string {
	return inputPathFields(input, "file_path")
}

func (t *MultiEditTool) NormalizeInput(input json.RawMessage, cwd string) (json.RawMessage, error) {
	return normalizePathFields(input, cwd, map[string]bool{"file_path": false})
}
func (t *MultiEditTool) InputPaths(input json.RawMessage) []string {
	return inputPathFields(input, "file_path")
}

func (t *GlobTool) NormalizeInput(input json.RawMessage, cwd string) (json.RawMessage, error) {
	return normalizePathFields(input, cwd, map[string]bool{"target_directory": true})
}
func (t *GlobTool) InputPaths(input json.RawMessage) []string {
	return inputPathFields(input, "target_directory")
}

func (t *GrepTool) NormalizeInput(input json.RawMessage, cwd string) (json.RawMessage, error) {
	return normalizePathFields(input, cwd, map[string]bool{"path": true})
}
func (t *GrepTool) InputPaths(input json.RawMessage) []string {
	return inputPathFields(input, "path")
}
