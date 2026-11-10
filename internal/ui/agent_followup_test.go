package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"claude-code-go/internal/query"
	"claude-code-go/internal/tasks"
)

func TestAgentFollowupWaitsForStreamCloseAndCoalesces(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	manager := tasks.NewManager(nil)
	parent := make(chan interface{})
	model := NewAppModelWithContext(ctx, func(context.Context, string) (<-chan interface{}, error) {
		return parent, nil
	}, 100, 30)
	model.SetTaskManager(manager)
	model.RestoreSession("current", nil)
	var prompts []string
	var followupStream chan interface{}
	model.SetAgentFollowupSubmitter(func(_ context.Context, prompt string) (<-chan interface{}, error) {
		prompts = append(prompts, prompt)
		followupStream = make(chan interface{})
		return followupStream, nil
	})
	start := func(session, result string) func() {
		t.Helper()
		task, err := manager.SpawnLocalAgentForSession(ctx, session, "work", "general-agent", result, true)
		if err != nil {
			t.Fatal(err)
		}
		release := make(chan struct{})
		if err := manager.StartExecution(ctx, task.ID, func(ctx context.Context, _ *tasks.LocalAgentTaskState) (interface{}, error) {
			select {
			case <-release:
				return result, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}); err != nil {
			t.Fatal(err)
		}
		return func() {
			t.Helper()
			close(release)
			msg := model.waitForAgentFinished()()
			if _, ok := msg.(agentFinishedMsg); !ok {
				t.Fatal("missing completion notification")
			}
			model.Update(msg)
		}
	}
	foreign := start("other", "foreign result")
	first := start("current", "first result")
	second := start("current", "second result")
	third := start("current", "third result")

	parentRead := model.startQuery("Run several agents")
	foreign()
	first()
	// A result can arrive before EOF; the parent still owns the stream.
	model.Update(QueryEngineMsg{Data: query.ResultMessage{Result: "Agents started"}})
	second()
	if len(prompts) != 0 || model.messageChan != parent {
		t.Fatal("completion replaced a live parent stream")
	}
	close(parent)
	model.Update(parentRead())
	if len(prompts) != 1 {
		t.Fatalf("expected a single coalesced follow-up, got %d", len(prompts))
	}
	for _, expected := range []string{"brief current status", "first result", "second result", `"status":"running"`} {
		if !strings.Contains(prompts[0], expected) {
			t.Fatalf("missing %q in %s", expected, prompts[0])
		}
	}
	if strings.Contains(prompts[0], "foreign result") {
		t.Fatal("follow-up leaked a different session's result")
	}

	third()
	if len(prompts) != 1 {
		t.Fatal("completion interrupted a follow-up turn")
	}
	close(followupStream)
	model.Update(model.waitForMessages()())
	if len(prompts) != 2 {
		t.Fatalf("expected final follow-up, got %d turns", len(prompts))
	}
	for _, expected := range []string{"final response", "first result", "second result", "third result"} {
		if !strings.Contains(prompts[1], expected) {
			t.Fatalf("missing %q in %s", expected, prompts[1])
		}
	}
	close(followupStream)
	model.Update(model.waitForMessages()())
	model.Update(agentFinishedMsg{})
	if len(prompts) != 2 {
		t.Fatal("a duplicate signal repeated an acknowledged follow-up")
	}
}
