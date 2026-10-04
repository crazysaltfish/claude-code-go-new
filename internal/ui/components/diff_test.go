package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"claude-code-go/internal/types"
)

func TestDiffViewNavigatesChangesScrollsAndCloses(t *testing.T) {
	chat := NewChatModel(80, 14)
	chat.AddToolUse("Edit", "edit_1", []byte(`{"file_path":"first.go"}`))
	chat.AddToolResult("Edit", "edit_1", "done", false, false, 4, &types.ToolDisplay{
		Summary: "Updated first.go", FilePath: "first.go",
		Diff: "--- first.go\n+++ first.go\n@@ change @@\n-old\n+new\n context-1\n context-2\n context-3\n context-4",
	})
	chat.AddToolUse("Write", "write_1", []byte(`{"file_path":"second.go"}`))
	chat.AddToolResult("Write", "write_1", "done", false, false, 4, &types.ToolDisplay{
		Summary: "Created second.go", FilePath: "second.go",
		Diff: "--- /dev/null\n+++ second.go\n+package second",
	})

	chat.OpenDiffView()
	if chat.DiffView == nil {
		t.Fatal("diff view was not opened")
	}
	if view := chat.View(); !strings.Contains(view, "Claude Code · Diff") || !strings.Contains(view, "[1/2] first.go") {
		t.Fatalf("unexpected initial diff view:\n%s", view)
	}
	chat.Update(tea.KeyMsg{Type: tea.KeyDown})
	if chat.DiffView.Offset == 0 {
		t.Fatal("down did not scroll diff")
	}
	chat.Update(tea.KeyMsg{Type: tea.KeyRight})
	if view := chat.View(); !strings.Contains(view, "[2/2] second.go") || !strings.Contains(view, "+package second") {
		t.Fatalf("right did not select next diff:\n%s", view)
	}
	chat.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if chat.DiffView != nil {
		t.Fatal("escape did not close diff view")
	}
}

func TestEmptyDiffViewHasUsefulMessage(t *testing.T) {
	chat := NewChatModel(80, 20)
	chat.OpenDiffView()
	if view := chat.View(); !strings.Contains(view, "No file changes") || !strings.Contains(view, "Esc/q: back") {
		t.Fatalf("empty diff view is not actionable:\n%s", view)
	}
}
