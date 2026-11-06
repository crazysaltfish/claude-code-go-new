package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"claude-code-go/internal/tasks"
)

func TestTaskOutputDoesNotReportUnfinishedAgentAsCompleted(t *testing.T) {
	manager := tasks.NewManager(nil)
	task, err := manager.SpawnLocalAgent(context.Background(), "work", "general-agent", "output test", true)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	defer close(release)
	if err := manager.StartExecution(context.Background(), task.ID, func(context.Context, *tasks.LocalAgentTaskState) (interface{}, error) {
		<-release
		return "done", nil
	}); err != nil {
		t.Fatal(err)
	}
	tool := NewTaskOutputTool(manager)
	for _, blocking := range []bool{false, true} {
		input, _ := json.Marshal(map[string]interface{}{"task_id": task.ID, "block": blocking, "timeout": 5})
		result, err := tool.Call(context.Background(), input, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		output := result.Output.(string)
		if !strings.Contains(output, "running") || strings.Contains(output, "completed") {
			t.Fatalf("incorrect output for block=%v: %s", blocking, output)
		}
		if state := manager.GetTask(task.ID).GetBase().Status; state != tasks.TaskStatusRunning {
			t.Fatalf("unexpected task state: %s", state)
		}
	}
	// Omitting block must honor the schema default and wait, while still
	// letting cancellation interrupt that wait immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input, _ := json.Marshal(map[string]interface{}{"task_id": task.ID})
	_, err = tool.Call(ctx, input, nil, nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled wait, got %v", err)
	}
}

func TestTaskOutputWaitsForAgentResult(t *testing.T) {
	manager := tasks.NewManager(nil)
	task, err := manager.SpawnLocalAgent(context.Background(), "work", "general-agent", "output test", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartExecution(context.Background(), task.ID, func(context.Context, *tasks.LocalAgentTaskState) (interface{}, error) {
		return "agent result", nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	input, _ := json.Marshal(map[string]interface{}{"task_id": task.ID})
	result, err := NewTaskOutputTool(manager).Call(ctx, input, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "agent result" {
		t.Fatalf("unexpected output: %v", result.Output)
	}
}
