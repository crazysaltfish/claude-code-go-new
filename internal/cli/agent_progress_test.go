package cli

import (
	"context"
	"encoding/json"
	"testing"

	"claude-code-go/internal/query"
	"claude-code-go/internal/tasks"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

func TestAgentProgressRecordsActivityAndPermissions(t *testing.T) {
	m := tasks.NewManager(nil)
	task, err := m.SpawnLocalAgent(context.Background(), "work", "general-agent", "test", true)
	if err != nil {
		t.Fatal(err)
	}
	p := newAgentProgress(m, task.ID)
	p.record(query.SDKMessage{Type: "assistant", Message: &api.MessageResponse{Content: []api.ContentBlock{{Type: "tool_use", Name: "Read", Input: json.RawMessage(`{"file_path":"/tmp/test.go"}`)}}}})
	state := m.GetTask(task.ID).(*tasks.LocalAgentTaskState).Progress
	if state.ToolUseCount != 1 || state.LastActivity.ActivityDescription != "/tmp/test.go" {
		t.Fatalf("unexpected progress: %+v", state)
	}
	check := p.permissionCheck(func(context.Context, string, json.RawMessage) (*types.PermissionDecision, error) {
		if got := m.GetTask(task.ID).(*tasks.LocalAgentTaskState).Progress.Summary; got != "Checking permission: Read" {
			t.Errorf("permission state: %s", got)
		}
		return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
	})
	if _, err := check(context.Background(), "Read", nil); err != nil {
		t.Fatal(err)
	}
	if state.Summary != "Running: Read" {
		t.Fatal("snapshot changed")
	}
	p.record(query.SDKMessage{Type: "tool_result"})
	if got := m.GetTask(task.ID).(*tasks.LocalAgentTaskState).Progress.Summary; got != "Waiting for model" {
		t.Fatalf("unexpected summary: %s", got)
	}
}
