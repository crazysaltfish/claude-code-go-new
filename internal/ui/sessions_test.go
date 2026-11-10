package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"claude-code-go/internal/query"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

func TestRestoreSessionReplacesVisibleHistoryAndReplaysTools(t *testing.T) {
	model := NewAppModelWithSubmit(nil, 100, 30)
	model.chat.AddUserMessage("old unrelated session")
	encode := func(blocks ...api.ContentBlock) json.RawMessage { data, _ := json.Marshal(blocks); return data }
	history := []types.Message{
		{Role: "user", Content: encode(api.ContentBlock{Type: "text", Text: "previous request"})},
		{Role: "assistant", Content: encode(api.ContentBlock{Type: "text", Text: "previous reply"}, api.ContentBlock{Type: "tool_use", ID: "tool-1", Name: "Read", Input: json.RawMessage(`{"file_path":"main.go"}`)})},
		{Role: "user", Content: encode(api.ContentBlock{Type: "tool_result", ToolUseID: "tool-1", Content: "file contents"})},
	}
	model.handleSDKMessage(query.SDKMessage{Type: "session_restored", Message: history, SessionID: "restored-id"})
	visible := model.View()
	for _, want := range []string{"previous request", "previous reply", "Read", "restored-id"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("missing %q in %s", want, visible)
		}
	}
	if strings.Contains(visible, "old unrelated session") {
		t.Fatal("previous session still visible")
	}
	if model.chat.PermissionMode != string(types.PermissionModeDefault) {
		t.Fatal("permission mode was lost")
	}
}
