package memory

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	EntrypointName           = "MEMORY.md"
	MaxEntrypointLines       = 200
	MaxEntrypointBytes       = 25_000
	MaxInstructionCharacters = 40_000
)

var unsafePathCharacter = regexp.MustCompile(`[^a-zA-Z0-9]`)

// Context is the stable memory snapshot injected when a query engine starts.
type Context struct {
	Prompt           string
	Directory        string
	InstructionFiles []string
}

// Load creates the project-scoped memory directory and loads bounded,
// file-based instructions and the memory index.
func Load(cwd string) (Context, error) {
	if envTruthy(os.Getenv("CLAUDE_CODE_SIMPLE")) {
		return Context{}, nil
	}

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return Context{}, fmt.Errorf("resolve working directory: %w", err)
	}
	absCwd = filepath.Clean(absCwd)

	instructions, files := loadInstructionFiles(absCwd)
	sections := make([]string, 0, len(instructions)+1)
	sections = append(sections, instructions...)
	ctx := Context{
		Prompt:           strings.Join(sections, "\n\n"),
		InstructionFiles: files,
	}
	if !Enabled() {
		return ctx, nil
	}

	memoryDir, err := AutoMemoryDir(absCwd)
	if err != nil {
		return ctx, err
	}
	if err := os.MkdirAll(memoryDir, 0o700); err != nil {
		return ctx, fmt.Errorf("create auto memory directory: %w", err)
	}
	ctx.Directory = memoryDir
	sections = append(sections, buildMemoryPrompt(memoryDir))
	ctx.Prompt = strings.Join(sections, "\n\n")
	return ctx, nil
}

// Enabled follows Claude Code's highest-priority environment gates.
func Enabled() bool {
	return !envTruthy(os.Getenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY")) &&
		!envTruthy(os.Getenv("CLAUDE_CODE_SIMPLE"))
}

// AutoMemoryDir returns a stable directory shared by worktrees under the same
// discovered repository root.
func AutoMemoryDir(cwd string) (string, error) {
	if override := os.Getenv("CLAUDE_COWORK_MEMORY_PATH_OVERRIDE"); override != "" {
		return validateOverride(override)
	}

	base := os.Getenv("CLAUDE_CODE_REMOTE_MEMORY_DIR")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		base = filepath.Join(home, ".claude")
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("memory base directory must be absolute: %s", base)
	}

	projectRoot := findProjectRoot(cwd)
	return filepath.Join(filepath.Clean(base), "projects", sanitizePath(projectRoot), "memory"), nil
}

func loadInstructionFiles(cwd string) ([]string, []string) {
	candidates := make([]string, 0, 4)
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".claude", "CLAUDE.md"))
	}

	root := findProjectRoot(cwd)
	for _, dir := range ancestors(root, cwd) {
		candidates = append(candidates, filepath.Join(dir, "CLAUDE.md"))
	}
	candidates = append(candidates, filepath.Join(cwd, "CLAUDE.local.md"))

	sections := make([]string, 0, len(candidates))
	loaded := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, path := range candidates {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		content, ok := readRegularFileNoFollow(path, MaxInstructionCharacters*utf8.UTFMax+1)
		if !ok || strings.TrimSpace(content) == "" {
			continue
		}
		content, truncated := truncateRunes(strings.TrimSpace(content), MaxInstructionCharacters)
		if truncated {
			content += fmt.Sprintf("\n\n> WARNING: instruction file truncated at %d characters.", MaxInstructionCharacters)
		}
		sections = append(sections, fmt.Sprintf("# Instructions from %s\n\n%s", path, content))
		loaded = append(loaded, path)
	}
	return sections, loaded
}

func buildMemoryPrompt(memoryDir string) string {
	entrypoint := filepath.Join(memoryDir, EntrypointName)
	content, ok := readRegularFileNoFollow(entrypoint, MaxEntrypointBytes+1)
	if ok {
		content = truncateEntrypoint(content)
	}
	if strings.TrimSpace(content) == "" {
		content = "Your MEMORY.md is currently empty. When you save new memories, they will appear here."
	}

	return fmt.Sprintf(`# Auto memory

You have a persistent, file-based memory system at %s. The directory already exists.

- Save only durable user preferences, corrections, and project context that cannot be derived from the repository.
- Do not save secrets, transient task state, plans, or facts already present in code or documentation.
- Store each topic in its own Markdown file. Keep MEMORY.md as a concise index only.
- Before writing, check for an existing topic to update. Remove or correct stale memories.
- If the user explicitly asks you to remember or forget something, update these files immediately.
- MEMORY.md is always loaded. Topic files are loaded only when selected as relevant or read explicitly.

## How to save memories

1. Write or update a topic file in the memory directory.
2. Add or update one index line in MEMORY.md using: - [Title](file.md) — one-line description.

Keep MEMORY.md within %d lines and %d bytes.

## MEMORY.md

%s`, "`"+memoryDir+"`", MaxEntrypointLines, MaxEntrypointBytes, content)
}

func truncateEntrypoint(raw string) string {
	trimmed := strings.TrimSpace(raw)
	originalLines := strings.Split(trimmed, "\n")
	lineTruncated := len(originalLines) > MaxEntrypointLines
	byteTruncated := len([]byte(trimmed)) > MaxEntrypointBytes
	if !lineTruncated && !byteTruncated {
		return trimmed
	}

	lines := originalLines
	if lineTruncated {
		lines = lines[:MaxEntrypointLines]
	}
	content := strings.Join(lines, "\n")
	if len(content) > MaxEntrypointBytes {
		content = truncateUTF8Bytes(content, MaxEntrypointBytes)
		if cut := strings.LastIndexByte(content, '\n'); cut > 0 {
			content = content[:cut]
		}
	}
	reasons := make([]string, 0, 2)
	if lineTruncated {
		reasons = append(reasons, fmt.Sprintf("more than %d lines", MaxEntrypointLines))
	}
	if byteTruncated {
		reasons = append(reasons, fmt.Sprintf("more than %d bytes", MaxEntrypointBytes))
	}
	return content + "\n\n> WARNING: MEMORY.md was partially loaded because it contains " + strings.Join(reasons, " and ") + ". Move details into topic files."
}

func readRegularFileNoFollow(path string, maxBytes int) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func findProjectRoot(cwd string) string {
	dir := filepath.Clean(cwd)
	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Clean(cwd)
		}
		dir = parent
	}
}

func ancestors(root, cwd string) []string {
	rel, err := filepath.Rel(root, cwd)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return []string{cwd}
	}
	dirs := []string{root}
	if rel == "." {
		return dirs
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		dirs = append(dirs, current)
	}
	return dirs
}

func sanitizePath(value string) string {
	sanitized := unsafePathCharacter.ReplaceAllString(value, "-")
	if len(sanitized) <= 200 {
		return sanitized
	}
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%s-%x", sanitized[:183], hash[:8])
}

func validateOverride(raw string) (string, error) {
	if strings.ContainsRune(raw, 0) || !filepath.IsAbs(raw) {
		return "", fmt.Errorf("auto memory override must be an absolute path")
	}
	clean := filepath.Clean(raw)
	volume := filepath.VolumeName(clean)
	if clean == string(filepath.Separator) || clean == volume+string(filepath.Separator) {
		return "", fmt.Errorf("auto memory override cannot be a filesystem root")
	}
	return clean, nil
}

func envTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func truncateRunes(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}

func truncateUTF8Bytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
