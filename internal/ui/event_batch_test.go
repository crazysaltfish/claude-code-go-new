package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"claude-code-go/internal/query"
	tea "github.com/charmbracelet/bubbletea"
)

func TestProgressBatchCoalescesAndFlushesOnFinal(t *testing.T) {
	stream := make(chan interface{}, 128)
	for i := 0; i < 100; i++ {
		stream <- query.SDKMessage{Type: "tool_progress", Message: map[string]interface{}{"tool_use_id": "call", "data": i}}
	}
	stream <- query.SDKMessage{Type: "tool_result", Message: map[string]interface{}{"tool_use_id": "call", "content": "done"}}
	close(stream)
	batch, ok := readQueryBatch(context.Background(), stream).(queryBatchMsg)
	if !ok || len(batch.events) != 2 {
		t.Fatalf("unexpected batch: %+v", batch)
	}
	progress := batch.events[0].(query.SDKMessage).Message.(map[string]interface{})
	if progress["data"] != 99 || batch.events[1].(query.SDKMessage).Type != "tool_result" {
		t.Fatal("lost final progress or reordered result")
	}
	if _, ok := readQueryBatch(context.Background(), stream).(queryCompleteMsg); !ok {
		t.Fatal("stream did not close cleanly")
	}
}

func TestAssistantBatchPreservesTextAndResponseBoundaries(t *testing.T) {
	stream := make(chan interface{}, 10)
	for _, text := range []string{"hello", " ", "world"} {
		stream <- query.SDKMessage{Type: "assistant_delta", UUID: "response", Message: query.AssistantDelta{Text: text}}
	}
	stream <- query.SDKMessage{Type: "assistant", Message: "final"}
	close(stream)
	batch := readQueryBatch(context.Background(), stream).(queryBatchMsg)
	if len(batch.events) != 2 {
		t.Fatal("deltas not merged")
	}
	delta := batch.events[0].(query.SDKMessage).Message.(query.AssistantDelta)
	if delta.Text != "hello world" {
		t.Fatal("stream text changed")
	}
}

func TestProgressBatchFlushesWithoutMoreEventsAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := make(chan interface{}, 1)
	stream <- query.SDKMessage{Type: "tool_progress"}
	done := make(chan tea.Msg, 1)
	go func() { done <- readQueryBatch(ctx, stream) }()
	select {
	case msg := <-done:
		if _, ok := msg.(queryBatchMsg); !ok {
			t.Fatal("missing timed batch")
		}
	case <-time.After(time.Second):
		t.Fatal("batch failed to flush")
	}
	cancel()
	if _, ok := readQueryBatch(ctx, stream).(queryCompleteMsg); !ok {
		t.Fatal("read did not cancel")
	}
}

func TestToolViewerRoutesKeysAndYieldsToApproval(t *testing.T) {
	model := NewAppModelWithSubmit(nil, 80, 24)
	model.chat.AddToolUse("Bash", "one", nil)
	model.chat.AddToolUse("Read", "two", nil)
	if cmd := model.startQuery("/tools"); cmd != nil {
		t.Fatal("tools command submitted query")
	}
	model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if model.chat.ToolDetail.Index != 0 || model.permissionMode != "default" {
		t.Fatal("viewer key changed permission mode")
	}
	model.Update(permissionRequestMsg{Request: PermissionRequest{ToolName: "Bash", Response: make(chan PermissionResponse, 1)}})
	if model.chat.ToolDetail != nil || !strings.Contains(model.View(), "Permission required") {
		t.Fatal("viewer hid permission request")
	}
}
