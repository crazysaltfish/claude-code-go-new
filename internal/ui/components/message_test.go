package components

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
