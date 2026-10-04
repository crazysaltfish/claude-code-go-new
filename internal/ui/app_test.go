package ui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"claude-code-go/internal/query"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

func TestFormatPermissionRequestShowsCompleteInput(t *testing.T) {
	longTail := strings.Repeat("x", 256) + "END-OF-COMMAND"
	input, err := json.Marshal(map[string]interface{}{
		"command": "printf " + longTail,
		"paths":   []string{"/tmp/first", "/tmp/second"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := formatPermissionRequest(PermissionRequest{
		ToolName:    "Bash",
		Input:       input,
		ReadOnly:    false,
		Destructive: true,
		RiskReason:  "Command contains command substitution",
	})

	for _, want := range []string{
		"Tool: Bash",
		"Impact: potentially destructive",
		"Read-only: false",
		"Destructive: true",
		`"command": "printf `,
		`"paths": [`,
		"END-OF-COMMAND",
		"Security note: Command contains command substitution",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted approval does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "truncated") || strings.Contains(got, "...") {
		t.Fatalf("formatted approval unexpectedly truncates input:\n%s", got)
	}
}

func TestPermissionPromptCanAllowSession(t *testing.T) {
	model := NewAppModelWithSubmit(nil, 80, 24)
	response := make(chan PermissionResponse, 1)
	model.pendingPermission = &PermissionRequest{ToolName: "Bash", Response: response}
	model.chat.SetApproval("Permission required")

	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if got := <-response; got != PermissionAllowSession {
		t.Fatalf("permission response = %s", got)
	}
	if model.permissionMode != types.PermissionModeSession || model.pendingPermission != nil {
		t.Fatalf("session approval did not update UI state: mode=%s pending=%#v", model.permissionMode, model.pendingPermission)
	}
}

func TestShiftTabCyclesCommonPermissionModes(t *testing.T) {
	model := NewAppModelWithSubmit(nil, 80, 24)
	var selected []types.PermissionMode
	model.SetPermissionMode(types.PermissionModeDefault, func(mode types.PermissionMode) error {
		selected = append(selected, mode)
		return nil
	})

	for _, want := range []types.PermissionMode{
		types.PermissionModeAcceptEdits,
		types.PermissionModePlan,
		types.PermissionModeDefault,
	} {
		model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if model.permissionMode != want || model.chat.PermissionMode != string(want) {
			t.Fatalf("Shift+Tab mode = %s, want %s", model.permissionMode, want)
		}
	}
	if len(selected) != 3 {
		t.Fatalf("mode setter called %d times, want 3", len(selected))
	}
}

func TestDiffCommandOpensViewerWithoutSubmittingQuery(t *testing.T) {
	called := false
	model := NewAppModelWithSubmit(func(context.Context, string) (<-chan interface{}, error) {
		called = true
		return nil, nil
	}, 80, 24)

	if cmd := model.startQuery("/diff"); cmd != nil {
		t.Fatal("/diff unexpectedly started an asynchronous query")
	}
	if called {
		t.Fatal("/diff was sent to the query engine")
	}
	if model.chat.DiffView == nil {
		t.Fatal("/diff did not open the dedicated viewer")
	}
}

func TestAssistantToolUseAndResultUpdateOneExpandableCard(t *testing.T) {
	model := NewAppModelWithSubmit(nil, 100, 40)
	model.handleSDKMessage(query.SDKMessage{
		Type: "assistant",
		Message: &api.MessageResponse{Content: []api.ContentBlock{{
			Type: "tool_use", ID: "edit_1", Name: "Edit",
			Input: json.RawMessage(`{"file_path":"main.go","old_string":"old","new_string":"new"}`),
		}}},
	})
	model.handleSDKMessage(query.SDKMessage{
		Type: "tool_result",
		Message: map[string]interface{}{
			"tool_name": "Edit", "tool_use_id": "edit_1", "content": "done",
			"display": &types.ToolDisplay{
				Summary: "Updated main.go", FilePath: "main.go",
				Diff:      "--- main.go\n+++ main.go\n@@ replacement @@\n-old\n+new",
				Artifacts: []string{"main.go"},
			},
		},
	})

	if len(model.chat.Messages.Messages) != 1 {
		t.Fatalf("tool call rendered as %d messages, want one", len(model.chat.Messages.Messages))
	}
	block := model.chat.Messages.Messages[0].Content[0]
	if block.Status != "completed" || block.Summary != "Updated main.go" || !strings.Contains(string(block.Input), "old_string") {
		t.Fatalf("tool card lost execution details: %#v", block)
	}
	if got := model.chat.Messages.ArtifactPaths(); len(got) != 1 || got[0] != "main.go" {
		t.Fatalf("artifact paths = %#v", got)
	}
}

func TestFormatPermissionRequestPreservesInvalidJSON(t *testing.T) {
	const raw = `{"command": "unterminated"`
	got := formatPermissionRequest(PermissionRequest{
		ToolName: "Bash",
		Input:    json.RawMessage(raw),
	})
	if !strings.Contains(got, raw) {
		t.Fatalf("raw input missing from formatted approval:\n%s", got)
	}
}
