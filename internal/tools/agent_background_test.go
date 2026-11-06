package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"claude-code-go/internal/tasks"
	"claude-code-go/internal/types"
)

func TestBackgroundAgentOutlivesParentProgressStream(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failure=%t", fail), func(t *testing.T) {
			manager := tasks.NewManager(nil)
			tool := NewAgentTool()
			tool.SetTaskManager(manager)
			parentOutput := make(chan interface{})
			release := make(chan struct{})
			defer close(release)
			checkedContext := make(chan error, 1)
			tool.SetRunner(func(ctx context.Context, task *tasks.LocalAgentTaskState, _ *types.ToolContext, progress func(interface{})) (result interface{}, err error) {
				// Turn a late callback panic into a task failure so a regression
				// fails this test instead of crashing the entire test process.
				defer func() {
					if p := recover(); p != nil {
						err = fmt.Errorf("late progress panic: %v", p)
					}
				}()
				<-release
				checkedContext <- ctx.Err()
				if progress != nil {
					progress("late progress")
				}
				if fail {
					return nil, fmt.Errorf("runner failed")
				}
				return "done", nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result, err := tool.Call(ctx, json.RawMessage(`{"description":"background test","prompt":"work","run_in_background":true}`), nil, nil, nil, func(p interface{}) {
				parentOutput <- p
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(fmt.Sprint(result.Output), "started in background") {
				t.Fatalf("unexpected launch result: %v", result.Output)
			}
			var taskID string
			for id := range manager.GetAllTasks() {
				taskID = id
			}
			close(parentOutput)
			cancel()
			release <- struct{}{}
			select {
			case err := <-checkedContext:
				if err != nil {
					t.Fatalf("background context canceled with parent: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("runner did not resume")
			}
			deadline := time.After(2 * time.Second)
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				state := manager.GetTask(taskID).(*tasks.LocalAgentTaskState)
				if state.Status == tasks.TaskStatusCompleted || state.Status == tasks.TaskStatusFailed {
					if fail {
						if state.Status != tasks.TaskStatusFailed || state.Error != "runner failed" {
							t.Fatalf("unexpected failure: %+v", state)
						}
					} else if state.Status != tasks.TaskStatusCompleted || state.Result != "done" {
						t.Fatalf("unexpected result: %+v", state)
					}
					break
				}
				select {
				case <-deadline:
					t.Fatal("background task did not finish")
				case <-ticker.C:
				}
			}
		})
	}
}

func TestForegroundAgentForwardsProgress(t *testing.T) {
	tool := NewAgentTool()
	tool.SetTaskManager(tasks.NewManager(nil))
	tool.SetRunner(func(_ context.Context, _ *tasks.LocalAgentTaskState, _ *types.ToolContext, progress func(interface{})) (interface{}, error) {
		progress("working")
		return "done", nil
	})
	updates := make(chan interface{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := tool.Call(ctx, json.RawMessage(`{"description":"foreground test","prompt":"work"}`), nil, nil, nil, func(p interface{}) { updates <- p })
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "done" {
		t.Fatalf("unexpected result: %v", result.Output)
	}
	select {
	case update := <-updates:
		if update != "working" {
			t.Fatalf("unexpected progress: %v", update)
		}
	default:
		t.Fatal("foreground progress was not forwarded")
	}
}
