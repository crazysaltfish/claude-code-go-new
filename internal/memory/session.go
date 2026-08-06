package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	DefaultSessionMemoryInitTokens    = 10_000
	DefaultSessionMemoryUpdateTokens  = 5_000
	DefaultSessionMemoryToolCalls     = 3
	MaxSessionMemorySectionCharacters = 8_000
	MaxSessionMemoryCharacters        = 48_000
)

type SessionMemoryConfig struct {
	MinimumMessageTokensToInit int
	MinimumTokensBetweenUpdate int
	ToolCallsBetweenUpdates    int
}

var DefaultSessionMemoryConfig = SessionMemoryConfig{
	MinimumMessageTokensToInit: DefaultSessionMemoryInitTokens,
	MinimumTokensBetweenUpdate: DefaultSessionMemoryUpdateTokens,
	ToolCallsBetweenUpdates:    DefaultSessionMemoryToolCalls,
}

type sessionSection struct {
	Header      string
	Description string
}

var sessionSections = []sessionSection{
	{"# Session Title", "_A short and distinctive 5-10 word descriptive title for the session. Super info dense, no filler_"},
	{"# Current State", "_What is actively being worked on right now? Pending tasks not yet completed. Immediate next steps._"},
	{"# Task specification", "_What did the user ask to build? Any design decisions or other explanatory context_"},
	{"# Files and Functions", "_What are the important files? In short, what do they contain and why are they relevant?_"},
	{"# Workflow", "_What bash commands are usually run and in what order? How to interpret their output if not obvious?_"},
	{"# Errors & Corrections", "_Errors encountered and how they were fixed. What did the user correct? What approaches failed and should not be tried again?_"},
	{"# Codebase and System Documentation", "_What are the important system components? How do they work/fit together?_"},
	{"# Learnings", "_What has worked well? What has not? What to avoid? Do not duplicate items from other sections_"},
	{"# Key results", "_If the user asked a specific output such as an answer to a question, a table, or other document, repeat the exact result here_"},
	{"# Worklog", "_Step by step, what was attempted, done? Very terse summary for each step_"},
}

// SessionMemory tracks extraction thresholds and owns one sensitive summary file.
type SessionMemory struct {
	mu               sync.Mutex
	config           SessionMemoryConfig
	path             string
	initialized      bool
	tokensAtLastSave int
	toolCalls        int
	extracting       bool
	summarizedCount  int
}

func NewSessionMemory(cwd, sessionID string, config SessionMemoryConfig) (*SessionMemory, error) {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve session working directory: %w", err)
	}
	config = normalizeSessionMemoryConfig(config)
	base, err := sessionMemoryBaseDir()
	if err != nil {
		return nil, err
	}
	safeSessionID := strings.Trim(sanitizePath(sessionID), "-")
	if safeSessionID == "" {
		return nil, errors.New("session ID must contain at least one alphanumeric character")
	}
	path := filepath.Join(base, "projects", sanitizePath(filepath.Clean(absCwd)), safeSessionID, "session-memory", "summary.md")
	return &SessionMemory{config: config, path: path}, nil
}

func SessionMemoryEnabled() bool {
	return !envTruthy(os.Getenv("CLAUDE_CODE_DISABLE_SESSION_MEMORY")) &&
		!envTruthy(os.Getenv("CLAUDE_CODE_SIMPLE")) &&
		!envTruthy(os.Getenv("CLAUDE_CODE_REMOTE"))
}

func (s *SessionMemory) Path() string {
	return s.path
}

func (s *SessionMemory) RecordToolCalls(count int) {
	if count <= 0 {
		return
	}
	s.mu.Lock()
	s.toolCalls += count
	s.mu.Unlock()
}

func (s *SessionMemory) ShouldExtract(currentTokens int, lastAssistantHasToolCalls bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		if currentTokens < s.config.MinimumMessageTokensToInit {
			return false
		}
		s.initialized = true
	}
	if currentTokens-s.tokensAtLastSave < s.config.MinimumTokensBetweenUpdate {
		return false
	}
	return s.toolCalls >= s.config.ToolCallsBetweenUpdates || !lastAssistantHasToolCalls
}

func (s *SessionMemory) TryBeginExtraction(currentTokens int, lastAssistantHasToolCalls bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.extracting {
		return false
	}
	if !s.initialized {
		if currentTokens < s.config.MinimumMessageTokensToInit {
			return false
		}
		s.initialized = true
	}
	if currentTokens-s.tokensAtLastSave < s.config.MinimumTokensBetweenUpdate {
		return false
	}
	if s.toolCalls < s.config.ToolCallsBetweenUpdates && lastAssistantHasToolCalls {
		return false
	}
	s.extracting = true
	return true
}

func (s *SessionMemory) FinishExtraction(success bool) {
	s.mu.Lock()
	s.extracting = false
	s.mu.Unlock()
}

func (s *SessionMemory) LoadOrCreate() (string, error) {
	directory := filepath.Dir(s.path)
	if err := ensureSessionMemoryDirectory(directory); err != nil {
		return "", err
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		if _, writeErr := file.WriteString(DefaultSessionMemoryTemplate()); writeErr != nil {
			file.Close()
			os.Remove(s.path)
			return "", fmt.Errorf("initialize session memory: %w", writeErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return "", fmt.Errorf("close session memory: %w", closeErr)
		}
	} else if !os.IsExist(err) {
		return "", fmt.Errorf("create session memory file: %w", err)
	}
	info, err := os.Lstat(s.path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("session memory path must be a regular file")
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return "", fmt.Errorf("secure session memory file: %w", err)
	}
	content, err := os.ReadFile(s.path)
	if err != nil {
		return "", fmt.Errorf("read session memory: %w", err)
	}
	return string(content), nil
}

func (s *SessionMemory) Save(raw string, currentTokens int, summarizedCount ...int) error {
	normalized, err := NormalizeSessionSummary(raw)
	if err != nil {
		return err
	}
	directory := filepath.Dir(s.path)
	if err := ensureSessionMemoryDirectory(directory); err != nil {
		return err
	}
	if info, err := os.Lstat(s.path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("session memory path must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect session memory path: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".summary-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary session memory: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary session memory: %w", err)
	}
	if _, err := temporary.WriteString(normalized); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary session memory: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary session memory: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary session memory: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace session memory: %w", err)
	}

	s.mu.Lock()
	s.tokensAtLastSave = currentTokens
	s.toolCalls = 0
	s.extracting = false
	if len(summarizedCount) > 0 && summarizedCount[0] >= 0 {
		s.summarizedCount = summarizedCount[0]
	}
	s.mu.Unlock()
	return nil
}

func (s *SessionMemory) LastSummarizedMessageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.summarizedCount
}

func (s *SessionMemory) LoadExisting() (string, error) {
	directory := filepath.Dir(s.path)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		if os.IsNotExist(err) {
			return "", os.ErrNotExist
		}
		return "", fmt.Errorf("session memory directory must be a regular directory")
	}
	info, err = os.Lstat(s.path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("session memory path must be a regular file")
	}
	content, err := os.ReadFile(s.path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func IsSessionMemoryEmpty(content string) bool {
	return strings.TrimSpace(content) == strings.TrimSpace(DefaultSessionMemoryTemplate())
}

func DefaultSessionMemoryTemplate() string {
	parts := make([]string, 0, len(sessionSections))
	for _, section := range sessionSections {
		parts = append(parts, section.Header+"\n"+section.Description)
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// NormalizeSessionSummary preserves the fixed template and bounds model output.
func NormalizeSessionSummary(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		if newline := strings.IndexByte(raw, '\n'); newline >= 0 {
			raw = raw[newline+1:]
		}
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	contents := make(map[string]string, len(sessionSections))
	position := 0
	for index, section := range sessionSections {
		for position < len(lines) && strings.TrimSpace(lines[position]) == "" {
			position++
		}
		if position >= len(lines) || strings.TrimSpace(lines[position]) != section.Header {
			return "", fmt.Errorf("session summary is missing or reordering section %q", section.Header)
		}
		position++
		if position >= len(lines) || strings.TrimSpace(lines[position]) != section.Description {
			return "", fmt.Errorf("session summary changed template description for %q", section.Header)
		}
		position++
		bodyStart := position
		for position < len(lines) {
			trimmed := strings.TrimSpace(lines[position])
			if index+1 < len(sessionSections) && trimmed == sessionSections[index+1].Header {
				break
			}
			if strings.HasPrefix(trimmed, "# ") {
				return "", fmt.Errorf("session summary contains unexpected section %q", trimmed)
			}
			position++
		}
		sectionBody := strings.TrimSpace(strings.Join(lines[bodyStart:position], "\n"))
		sectionBody, _ = truncateRunes(sectionBody, MaxSessionMemorySectionCharacters)
		contents[section.Header] = sectionBody
	}

	parts := make([]string, 0, len(sessionSections))
	for _, section := range sessionSections {
		part := section.Header + "\n" + section.Description
		if content := contents[section.Header]; content != "" {
			part += "\n\n" + content
		}
		parts = append(parts, part)
	}
	result := strings.Join(parts, "\n\n") + "\n"
	if len([]rune(result)) > MaxSessionMemoryCharacters {
		return "", fmt.Errorf("session summary exceeds %d characters", MaxSessionMemoryCharacters)
	}
	return result, nil
}

func normalizeSessionMemoryConfig(config SessionMemoryConfig) SessionMemoryConfig {
	if config.MinimumMessageTokensToInit <= 0 {
		config.MinimumMessageTokensToInit = DefaultSessionMemoryConfig.MinimumMessageTokensToInit
	}
	if config.MinimumTokensBetweenUpdate <= 0 {
		config.MinimumTokensBetweenUpdate = DefaultSessionMemoryConfig.MinimumTokensBetweenUpdate
	}
	if config.ToolCallsBetweenUpdates <= 0 {
		config.ToolCallsBetweenUpdates = DefaultSessionMemoryConfig.ToolCallsBetweenUpdates
	}
	return config
}

func sessionMemoryBaseDir() (string, error) {
	if configured := os.Getenv("CLAUDE_CONFIG_HOME"); configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("CLAUDE_CONFIG_HOME must be absolute")
		}
		return filepath.Clean(configured), nil
	}
	if configured := os.Getenv("XDG_CONFIG_HOME"); configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("XDG_CONFIG_HOME must be absolute")
		}
		return filepath.Join(filepath.Clean(configured), "claude"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

func ensureSessionMemoryDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create session memory directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("session memory directory must not be a symbolic link")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("secure session memory directory: %w", err)
	}
	return nil
}
