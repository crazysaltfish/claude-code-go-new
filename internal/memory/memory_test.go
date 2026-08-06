package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLoadBuildsProjectMemoryContext(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "services", "api")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), "user instruction")
	writeTestFile(t, filepath.Join(workspace, "CLAUDE.md"), "project instruction")
	writeTestFile(t, filepath.Join(workspace, "services", "CLAUDE.md"), "service instruction")
	writeTestFile(t, filepath.Join(nested, "CLAUDE.local.md"), "local instruction")

	memoryDir := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(memoryDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(memoryDir, EntrypointName), "- [Preference](preference.md) — use Go")

	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_COWORK_MEMORY_PATH_OVERRIDE", memoryDir)
	t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", "")
	t.Setenv("CLAUDE_CODE_SIMPLE", "")

	ctx, err := Load(nested)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"user instruction", "project instruction", "service instruction", "local instruction",
		"[Preference](preference.md)", memoryDir,
	} {
		if !strings.Contains(ctx.Prompt, want) {
			t.Fatalf("prompt does not contain %q:\n%s", want, ctx.Prompt)
		}
	}
	if len(ctx.InstructionFiles) != 4 {
		t.Fatalf("loaded instruction files = %d, want 4", len(ctx.InstructionFiles))
	}
	if ctx.Directory != memoryDir {
		t.Fatalf("memory directory = %q, want %q", ctx.Directory, memoryDir)
	}
}

func TestLoadRejectsSymlinkedInstructions(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.md")
	writeTestFile(t, target, "do not load me")
	if err := os.Symlink(target, filepath.Join(workspace, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_COWORK_MEMORY_PATH_OVERRIDE", filepath.Join(t.TempDir(), "memory"))
	ctx, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ctx.Prompt, "do not load me") || len(ctx.InstructionFiles) != 0 {
		t.Fatalf("symlinked instruction was loaded: %#v", ctx.InstructionFiles)
	}
}

func TestEntrypointTruncationPreservesUTF8AndCapsLines(t *testing.T) {
	lines := make([]string, MaxEntrypointLines+20)
	for i := range lines {
		lines[i] = strings.Repeat("界", 100)
	}
	got := truncateEntrypoint(strings.Join(lines, "\n"))
	if !utf8.ValidString(got) {
		t.Fatal("truncated entrypoint is not valid UTF-8")
	}
	if !strings.Contains(got, "partially loaded") || !strings.Contains(got, "200 lines") || !strings.Contains(got, "25000 bytes") {
		t.Fatalf("missing truncation warning: %s", got[len(got)-200:])
	}
	content := strings.Split(got, "\n\n> WARNING:")[0]
	if len(strings.Split(content, "\n")) > MaxEntrypointLines {
		t.Fatalf("loaded %d index lines", len(strings.Split(content, "\n")))
	}
	if len(content) > MaxEntrypointBytes {
		t.Fatalf("loaded %d index bytes", len(content))
	}
}

func TestLoadCanBeDisabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", "true")
	ctx, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Prompt != "" || ctx.Directory != "" || len(ctx.InstructionFiles) != 0 {
		t.Fatalf("disabled memory context = %#v", ctx)
	}
}

func TestDisablingAutoMemoryKeepsClaudeInstructions(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(workspace, "CLAUDE.md"), "keep project instructions")
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", "true")
	t.Setenv("CLAUDE_CODE_SIMPLE", "")

	ctx, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.Prompt, "keep project instructions") || ctx.Directory != "" {
		t.Fatalf("context = %#v", ctx)
	}
}

func TestAutoMemoryDirUsesRepositoryRoot(t *testing.T) {
	workspace := t.TempDir()
	nested := filepath.Join(workspace, "a", "b")
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	t.Setenv("CLAUDE_COWORK_MEMORY_PATH_OVERRIDE", "")
	t.Setenv("CLAUDE_CODE_REMOTE_MEMORY_DIR", base)
	dir, err := AutoMemoryDir(nested)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "projects", sanitizePath(workspace), "memory")
	if dir != want {
		t.Fatalf("memory directory = %q, want %q", dir, want)
	}
}

func TestScanAndRecallUseManifestAndBoundedBodies(t *testing.T) {
	directory := t.TempDir()
	preferences := filepath.Join(directory, "preferences.md")
	writeTestFile(t, preferences, `---
description: Preferred editor and formatting choices
type: user
---
Use vim.
`+strings.Repeat("detail\n", MaxRecalledFileLines+20))
	writeTestFile(t, filepath.Join(directory, "unrelated.md"), `---
description: Deployment incident history
type: project
---
Do not surface this body.`)
	writeTestFile(t, filepath.Join(directory, EntrypointName), "index")

	headers := Scan(directory)
	if len(headers) != 2 {
		t.Fatalf("headers = %d, want 2", len(headers))
	}
	manifest := FormatManifest(headers)
	if !strings.Contains(manifest, "[user] preferences.md") || strings.Contains(manifest, "Use vim") || strings.Contains(manifest, EntrypointName) {
		t.Fatalf("unexpected manifest:\n%s", manifest)
	}

	selectorCalls := 0
	recalled := Recall(context.Background(), "Please use my preferred editor", directory, map[string]struct{}{}, func(_ context.Context, query, gotManifest string) ([]string, error) {
		selectorCalls++
		if query == "" || gotManifest != manifest {
			t.Fatalf("selector query=%q manifest=%q", query, gotManifest)
		}
		return []string{"missing.md", "preferences.md", "preferences.md", "unrelated.md", "missing-again.md"}, nil
	})
	if selectorCalls != 1 || len(recalled) != 2 {
		t.Fatalf("selector calls=%d recalled=%d", selectorCalls, len(recalled))
	}
	if recalled[0].Path != preferences || !recalled[0].Truncated || !strings.Contains(recalled[0].Content, "This memory was truncated") {
		t.Fatalf("unexpected recalled preference: %#v", recalled[0])
	}
	if len([]byte(strings.Split(recalled[0].Content, "\n\n> This memory")[0])) > MaxRecalledFileBytes {
		t.Fatalf("recalled body exceeded %d bytes", MaxRecalledFileBytes)
	}
}

func TestRecallSkipsSurfacedFilesAndShortQueries(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "preference.md")
	writeTestFile(t, path, "---\ndescription: editor\ntype: user\n---\nvim")
	calls := 0
	selector := func(context.Context, string, string) ([]string, error) {
		calls++
		return []string{"preference.md"}, nil
	}
	if got := Recall(context.Background(), "short", directory, nil, selector); len(got) != 0 {
		t.Fatalf("short query recalled %d files", len(got))
	}
	if got := Recall(context.Background(), "请使用我之前喜欢的编辑器", directory, nil, selector); len(got) != 1 {
		t.Fatalf("Chinese query recalled %d files, want 1", len(got))
	}
	if got := Recall(context.Background(), "请使用我之前喜欢的编辑器", directory, map[string]struct{}{path: {}}, selector); len(got) != 0 {
		t.Fatalf("surfaced file recalled again: %#v", got)
	}
	if calls != 1 {
		t.Fatalf("selector called %d times", calls)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
