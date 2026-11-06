package cli

import (
	"context"
	"encoding/json"
	"testing"

	"claude-code-go/internal/query"
	"claude-code-go/internal/sessions"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

func TestSessionSelectionAndFreshIDs(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_HOME", t.TempDir())
	cwd := t.TempDir()
	app := NewApp(&Config{Cwd: cwd}, "test")
	if err := app.initializeSession(); err != nil {
		t.Fatal(err)
	}
	first := app.session.ID
	if err := app.persistSession([]types.Message{{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"first session"}]`)}}, nil); err != nil {
		t.Fatal(err)
	}
	fresh := NewApp(&Config{Cwd: cwd}, "test")
	if err := fresh.initializeSession(); err != nil {
		t.Fatal(err)
	}
	if first == fresh.session.ID {
		t.Fatal("new invocation reused session")
	}
	for _, config := range []*Config{{Cwd: cwd, Resume: first}, {Cwd: cwd, Continue: true}} {
		resumed := NewApp(config, "test")
		if err := resumed.initializeSession(); err != nil {
			t.Fatal(err)
		}
		if resumed.session.ID != first || len(resumed.session.Messages) != 1 {
			t.Fatal("session was not restored")
		}
	}
	app.queryEngine = query.NewQueryEngine(query.QueryEngineConfig{})
	app.queryConfig = query.QueryEngineConfig{PersistSession: app.persistSession}
	output, err := app.executeSessionCommand(context.Background(), "resume", first)
	if err != nil {
		t.Fatal(err)
	}
	for range output {
	}
	if app.queryEngine.GetSessionID() != first || len(app.queryEngine.GetMessages()) != 1 {
		t.Fatal("slash resume did not switch engine")
	}
}

func TestInterruptedToolCallsAreNotReexecuted(t *testing.T) {
	blocks := []api.ContentBlock{{Type: "tool_use", ID: "call-1", Name: "Bash", Input: json.RawMessage(`{"command":"do something"}`)}}
	data, _ := json.Marshal(blocks)
	saved := &sessions.Session{Messages: []types.Message{{Role: "assistant", Content: data}}, History: []types.Message{{Role: "assistant", Content: data}}}
	prepareSession(saved)
	if len(saved.Messages) != 2 || len(saved.History) != 2 {
		t.Fatal("missing interrupted tool result")
	}
	var results []api.ContentBlock
	if err := json.Unmarshal(saved.Messages[1].Content, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].IsError || results[0].ToolUseID != "call-1" {
		t.Fatalf("results=%+v", results)
	}
	prepareSession(saved)
	if len(saved.Messages) != 2 {
		t.Fatal("repair was not idempotent")
	}
}
