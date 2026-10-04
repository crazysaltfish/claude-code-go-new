package components

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCtrlOCyclesTranscriptModesAndUpdatesHeader(t *testing.T) {
	model := NewChatModel(80, 24)
	for _, want := range []TranscriptMode{TranscriptVerbose, TranscriptSummary, TranscriptNormal} {
		model.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
		if model.Messages.Mode != want {
			t.Fatalf("Ctrl+O mode = %s, want %s", model.Messages.Mode, want)
		}
		if view := model.View(); !strings.Contains(view, "Claude Code · "+want.String()) {
			t.Fatalf("header does not show %s mode:\n%s", want, view)
		}
	}
}

func TestApprovalViewScrollsThroughCompleteToolCall(t *testing.T) {
	model := NewChatModel(80, 30)
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("input-line-%02d", i))
	}

	model.SetApproval(strings.Join(lines, "\n"))
	first := model.approvalView()
	if !strings.Contains(first, "input-line-01") {
		t.Fatalf("first approval page does not show the beginning:\n%s", first)
	}
	if !strings.Contains(first, "[1-10 of 20 lines]") {
		t.Fatalf("first approval page does not show its position:\n%s", first)
	}

	for i := 0; i < 30; i++ {
		model.ScrollApprovalDown()
	}
	last := model.approvalView()
	if !strings.Contains(last, "input-line-20") {
		t.Fatalf("last approval page does not show the complete input tail:\n%s", last)
	}
	if !strings.Contains(last, "[11-20 of 20 lines]") {
		t.Fatalf("last approval page does not show its position:\n%s", last)
	}
	if !strings.Contains(last, "[y] Allow once") || !strings.Contains(last, "[n/Esc] Deny") {
		t.Fatalf("approval controls must remain visible:\n%s", last)
	}
}

func TestApprovalPanelReservesAndRestoresMessageViewport(t *testing.T) {
	model := NewChatModel(80, 30)
	original := model.Messages.MaxVisible

	model.SetApproval(strings.Repeat("tool input\n", 20))
	if model.Messages.MaxVisible >= original {
		t.Fatalf("approval should reserve screen space: before=%d after=%d", original, model.Messages.MaxVisible)
	}

	model.ClearApproval()
	if model.Messages.MaxVisible != original {
		t.Fatalf("message viewport was not restored: got=%d want=%d", model.Messages.MaxVisible, original)
	}
	if model.ApprovalOffset != 0 {
		t.Fatalf("approval offset was not reset: %d", model.ApprovalOffset)
	}
}
