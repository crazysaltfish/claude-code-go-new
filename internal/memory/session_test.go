package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionMemoryThresholdsAndSecurePersistence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	store, err := NewSessionMemory(t.TempDir(), "session-123", SessionMemoryConfig{
		MinimumMessageTokensToInit: 100,
		MinimumTokensBetweenUpdate: 50,
		ToolCallsBetweenUpdates:    3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.ShouldExtract(99, false) {
		t.Fatal("session memory initialized below threshold")
	}
	if store.ShouldExtract(100, true) {
		t.Fatal("tool-chain extraction ran without enough tool calls")
	}
	store.RecordToolCalls(3)
	if !store.ShouldExtract(100, true) {
		t.Fatal("session memory did not trigger after token and tool thresholds")
	}

	current, err := store.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if current != DefaultSessionMemoryTemplate() {
		t.Fatal("new session memory did not use the default template")
	}
	summary := withSessionSectionContent(DefaultSessionMemoryTemplate(), "# Current State", "Implementing session memory.")
	if err := store.Save(summary, 100); err != nil {
		t.Fatal(err)
	}
	if store.ShouldExtract(149, false) {
		t.Fatal("session memory updated before token growth threshold")
	}
	if !store.ShouldExtract(150, false) {
		t.Fatal("session memory did not update at a natural breakpoint")
	}

	directoryInfo, err := os.Stat(filepath.Dir(store.Path()))
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("session memory modes directory=%o file=%o", directoryInfo.Mode().Perm(), fileInfo.Mode().Perm())
	}
}

func TestNormalizeSessionSummaryPreservesTemplate(t *testing.T) {
	raw := "```markdown\n" + withSessionSectionContent(DefaultSessionMemoryTemplate(), "# Session Title", strings.Repeat("x", MaxSessionMemorySectionCharacters+100)) + "```"
	normalized, err := NormalizeSessionSummary(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(normalized, "# ") != len(sessionSections) || !strings.Contains(normalized, strings.Repeat("x", MaxSessionMemorySectionCharacters)) {
		t.Fatalf("unexpected normalized summary: %s", normalized)
	}
	if strings.Contains(normalized, strings.Repeat("x", MaxSessionMemorySectionCharacters+1)) {
		t.Fatal("oversized section was not truncated")
	}

	changedDescription := strings.Replace(DefaultSessionMemoryTemplate(), sessionSections[0].Description, "_changed_", 1)
	if _, err := NormalizeSessionSummary(changedDescription); err == nil {
		t.Fatal("changed template description was accepted")
	}
	extraSection := DefaultSessionMemoryTemplate() + "\n# Unexpected\ncontent"
	if _, err := NormalizeSessionSummary(extraSection); err == nil {
		t.Fatal("unexpected section was accepted")
	}
}

func TestSessionMemoryPathUsesConfiguredHome(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_HOME", configHome)
	store, err := NewSessionMemory("/work/project", "abc/../123", DefaultSessionMemoryConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(store.Path(), filepath.Join(configHome, "projects")) || strings.Contains(store.Path(), "..") {
		t.Fatalf("unsafe session memory path: %s", store.Path())
	}
}

func TestSessionMemoryRejectsSymlinkedSummary(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_HOME", t.TempDir())
	store, err := NewSessionMemory(t.TempDir(), "symlink-session", DefaultSessionMemoryConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(external, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, store.Path()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadOrCreate(); err == nil {
		t.Fatal("symlinked session summary was accepted")
	}
	content, err := os.ReadFile(external)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "external" {
		t.Fatalf("external target was modified: %q", content)
	}
}

func TestSessionMemoryAllowsOnlyOneExtractionAtATime(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_HOME", t.TempDir())
	store, err := NewSessionMemory(t.TempDir(), "single-extraction", SessionMemoryConfig{
		MinimumMessageTokensToInit: 1,
		MinimumTokensBetweenUpdate: 1,
		ToolCallsBetweenUpdates:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.TryBeginExtraction(1, false) {
		t.Fatal("first extraction did not start")
	}
	if store.TryBeginExtraction(2, false) {
		t.Fatal("concurrent extraction was allowed")
	}
	store.FinishExtraction(false)
	if !store.TryBeginExtraction(2, false) {
		t.Fatal("failed extraction could not be retried")
	}
}

func withSessionSectionContent(template, header, content string) string {
	for _, section := range sessionSections {
		if section.Header == header {
			return strings.Replace(template, header+"\n"+section.Description, header+"\n"+section.Description+"\n\n"+content, 1)
		}
	}
	return template
}
