package components

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"claude-code-go/internal/types"
)

func TestMessageListScrollsRenderedConversationLines(t *testing.T) {
	list := NewMessageList(60, 8)
	for i := 1; i <= 6; i++ {
		list.AddMessage(MessageModel{
			Role: "assistant",
			Content: []ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("message-%d", i),
			}},
		})
	}

	bottom := list.View()
	if !strings.Contains(bottom, "message-6") {
		t.Fatalf("bottom viewport does not show latest message:\n%s", bottom)
	}
	if strings.Contains(bottom, "message-1") {
		t.Fatalf("bottom viewport unexpectedly shows oldest message:\n%s", bottom)
	}

	for i := 0; i < 10; i++ {
		list.PageUp()
	}
	top := list.View()
	if !strings.Contains(top, "message-1") {
		t.Fatalf("scrolled viewport does not show oldest message:\n%s", top)
	}
	if !list.IsScrolled() {
		t.Fatal("message list should report that it is above the latest messages")
	}

	for i := 0; i < 10; i++ {
		list.PageDown()
	}
	if list.IsScrolled() {
		t.Fatalf("message list did not return to bottom: offset=%d", list.ScrollOffset)
	}
	if got := list.View(); !strings.Contains(got, "message-6") {
		t.Fatalf("restored viewport does not show latest message:\n%s", got)
	}
}

func TestChatHistoryKeyboardAndMouseScrolling(t *testing.T) {
	model := NewChatModel(60, 20)
	for i := 0; i < 8; i++ {
		model.AddAssistantMessage(fmt.Sprintf("history-%d", i))
	}

	model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if model.Messages.ScrollOffset == 0 {
		t.Fatal("PageUp did not scroll conversation history")
	}

	model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if model.Messages.ScrollOffset != 0 {
		t.Fatalf("PageDown did not restore latest messages: offset=%d", model.Messages.ScrollOffset)
	}

	model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if model.Messages.ScrollOffset == 0 {
		t.Fatal("mouse wheel did not scroll conversation history")
	}
	model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if model.Messages.ScrollOffset != 0 {
		t.Fatalf("mouse wheel did not return to latest messages: offset=%d", model.Messages.ScrollOffset)
	}
}

func TestAssistantStreamUpdatesSingleMessage(t *testing.T) {
	model := NewChatModel(60, 20)

	model.AppendAssistantDelta("hello")
	model.AppendAssistantDelta(" world")

	if len(model.Messages.Messages) != 1 {
		t.Fatalf("stream created %d messages, want 1", len(model.Messages.Messages))
	}
	message := model.Messages.Messages[0]
	if !message.IsStreaming {
		t.Fatal("assistant message should be marked as streaming")
	}
	if got := message.Content[0].Text; got != "hello world" {
		t.Fatalf("unexpected streamed content: %q", got)
	}
	if got := model.Messages.View(); !strings.Contains(got, "▌") {
		t.Fatalf("stream cursor is not rendered:\n%s", got)
	}

	model.FinalizeAssistantMessage("hello world")
	if len(model.Messages.Messages) != 1 {
		t.Fatalf("finalization duplicated the assistant message: %d", len(model.Messages.Messages))
	}
	if model.Messages.Messages[0].IsStreaming {
		t.Fatal("assistant message remained in streaming state")
	}
	if got := model.Messages.View(); strings.Contains(got, "▌") {
		t.Fatalf("stream cursor remained after finalization:\n%s", got)
	}
}

func TestToolProgressIsUpdatedInPlaceAndReplacedByResult(t *testing.T) {
	model := NewChatModel(60, 20)
	model.AddToolUse("Synthetic", "tool_1", []byte(`{"value":"input"}`))
	model.UpdateToolProgress("Synthetic", "tool_1", `{"percent":10}`)
	model.UpdateToolProgress("Synthetic", "tool_1", `{"percent":50}`)

	if len(model.Messages.Messages) != 1 {
		t.Fatalf("progress created %d rows, want 1", len(model.Messages.Messages))
	}
	progress := model.Messages.Messages[0]
	if progress.Role != "tool" || progress.Content[0].Content != `{"percent":50}` {
		t.Fatalf("unexpected progress row: %#v", progress)
	}

	model.AddToolResult("Synthetic", "tool_1", "done", false, false, 4, nil)
	if len(model.Messages.Messages) != 1 {
		t.Fatalf("final result left transient progress rows: %#v", model.Messages.Messages)
	}
	result := model.Messages.Messages[0]
	if result.Role != "tool" || result.Content[0].Content != "done" || result.Content[0].Status != "completed" {
		t.Fatalf("unexpected final tool row: %#v", result)
	}
	model.Messages.Mode = TranscriptVerbose
	view := model.Messages.View()
	if !strings.Contains(view, "done") || strings.Contains(view, "<nil>") {
		t.Fatalf("tool result rendered incorrectly:\n%s", view)
	}
}

func TestToolDetailsAreCollapsedAndExpandableWithDiff(t *testing.T) {
	model := NewChatModel(100, 40)
	model.AddToolUse("Edit", "edit_1", []byte(`{"file_path":"main.go","old_string":"old\nline","new_string":"new\nline"}`))
	model.AddToolResult("Edit", "edit_1", "Successfully edited main.go", false, false, 27, nil)

	collapsed := RenderMessage(model.Messages.Messages[0], 100)
	for _, want := range []string{"✓ ▸ Edit main.go (+2 -2)"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("collapsed tool card missing %q:\n%s", want, collapsed)
		}
	}
	if strings.Contains(collapsed, "old_string") || strings.Contains(collapsed, "-old") {
		t.Fatalf("collapsed tool card leaked details:\n%s", collapsed)
	}

	model.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	expanded := renderMessageWithMode(model.Messages.Messages[0], 100, model.Messages.Mode)
	for _, want := range []string{"✓ ▾ Edit", "Input", `"old_string"`, "Changes", "-old", "+new", "Output", "Successfully edited"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded tool card missing %q:\n%s", want, expanded)
		}
	}
}

func TestWriteToolCardShowsNewFileDiff(t *testing.T) {
	model := NewChatModel(100, 40)
	model.AddToolUse("Write", "write_1", []byte(`{"file_path":"result.txt","contents":"first\nsecond\n"}`))
	model.AddToolResult("Write", "write_1", "Successfully wrote to result.txt", false, false, 32, nil)
	model.Messages.Mode = TranscriptVerbose

	view := renderMessageWithMode(model.Messages.Messages[0], 100, model.Messages.Mode)
	for _, want := range []string{"✓ ▾ Write result.txt (+2 -0)", "--- /dev/null", "+++ result.txt", "+first", "+second"} {
		if !strings.Contains(view, want) {
			t.Fatalf("write diff missing %q:\n%s", want, view)
		}
	}
}

func TestCompletionSummaryListsChangedArtifacts(t *testing.T) {
	model := NewChatModel(100, 40)
	model.AddToolUse("Write", "write_1", []byte(`{"file_path":"result.txt","contents":"done"}`))
	model.AddToolResult("Write", "write_1", "written", false, false, 7, &types.ToolDisplay{
		Artifacts: []string{"result.txt"},
	})
	model.AddCompletionSummary("1.25s", "$0.001000", 2)

	summary := model.Messages.Messages[len(model.Messages.Messages)-1]
	view := RenderMessage(summary, 100)
	for _, want := range []string{"Result:", "Completed in 1.25s", "Changed files (1):", "result.txt"} {
		if !strings.Contains(view, want) {
			t.Fatalf("completion summary missing %q:\n%s", want, view)
		}
	}
}

func TestTranscriptModesCycleAndSummaryFiltersIntermediateWork(t *testing.T) {
	list := NewMessageList(100, 40)
	list.AddMessage(MessageModel{Role: "user", Content: []ContentBlock{{Type: "text", Text: "change it"}}})
	list.AddMessage(MessageModel{Role: "assistant", Content: []ContentBlock{{Type: "text", Text: "I will inspect first"}}})
	list.UpsertToolUse("Read", "read_1", []byte(`{"target_file":"main.go"}`))
	list.CompleteToolUse("Read", "read_1", "contents", false, false, 8, nil)
	list.UpsertToolUse("Edit", "edit_1", []byte(`{"file_path":"main.go","old_string":"old","new_string":"new"}`))
	list.CompleteToolUse("Edit", "edit_1", "edited", false, false, 6, &types.ToolDisplay{
		Summary: "Updated main.go", FilePath: "main.go", Diff: "--- main.go\n+++ main.go\n-old\n+new",
		Artifacts: []string{"main.go"},
	})
	list.AddMessage(MessageModel{Role: "assistant", Content: []ContentBlock{{Type: "text", Text: "Finished the change"}}})
	list.AddMessage(MessageModel{Role: "summary", Content: []ContentBlock{{Type: "text", Text: "Changed files (1)"}}})

	if list.Mode != TranscriptNormal || list.CycleTranscriptMode() != TranscriptVerbose ||
		list.CycleTranscriptMode() != TranscriptSummary {
		t.Fatalf("unexpected transcript mode cycle: %s", list.Mode)
	}
	view := strings.Join(list.renderedLines(), "\n")
	for _, want := range []string{"change it", "Updated main.go", "Finished the change", "Changed files (1)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("summary transcript missing %q:\n%s", want, view)
		}
	}
	for _, hidden := range []string{"I will inspect first", "Read main.go", "contents"} {
		if strings.Contains(view, hidden) {
			t.Fatalf("summary transcript leaked intermediate detail %q:\n%s", hidden, view)
		}
	}
	if list.CycleTranscriptMode() != TranscriptNormal {
		t.Fatalf("summary did not cycle back to normal: %s", list.Mode)
	}
}
