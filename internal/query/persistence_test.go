package query

import (
	"context"
	"errors"
	"strings"
	"testing"

	"claude-code-go/internal/sessions"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

func TestSessionPersistsAndResumesContextAfterCompaction(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_HOME", t.TempDir())
	store, _ := sessions.NewStore(t.TempDir())
	saved := store.New("test-model", "openai")
	persist := func(messages, history []types.Message) error {
		saved.Messages = messages
		saved.History = history
		return store.Save(saved)
	}
	client := &scriptedStreamClient{calls: []func(api.MessageRequest, func(api.StreamEvent) error) error{
		func(req api.MessageRequest, emit func(api.StreamEvent) error) error {
			onDisk, err := store.Load(saved.ID)
			if err != nil || len(onDisk.Messages) != 1 {
				t.Fatalf("user prompt not saved before API: %v", err)
			}
			return emitTextResponse(emit, "remembered", "end_turn", api.Usage{})
		},
	}}
	engine := NewQueryEngine(QueryEngineConfig{SessionID: saved.ID, APIClient: client, PersistSession: persist})
	output, _ := engine.SubmitMessage(context.Background(), "remember this request")
	for event := range output {
		if result, ok := event.(ResultMessage); ok && result.IsError {
			t.Fatal(result.Result)
		}
	}
	loaded, err := store.Load(saved.ID)
	if err != nil || len(loaded.Messages) != 2 || len(loaded.History) != 2 {
		t.Fatalf("saved=%+v err=%v", loaded, err)
	}
	// Compact through the real command path; original history must survive.
	engine.config.APIClient = &staticMessageClient{response: &api.MessageResponse{Content: []api.ContentBlock{{Type: "text", Text: "saved summary"}}}}
	output, _ = engine.Compact(context.Background(), "")
	for range output {
	}
	loaded, err = store.Load(saved.ID)
	if err != nil || len(loaded.History) != 2 || !strings.Contains(string(loaded.Messages[0].Content), "saved summary") {
		t.Fatalf("compacted=%+v err=%v", loaded, err)
	}
	resumedClient := &scriptedStreamClient{calls: []func(api.MessageRequest, func(api.StreamEvent) error) error{
		func(req api.MessageRequest, emit func(api.StreamEvent) error) error {
			if !strings.Contains(string(mustMarshalJSON(req.Messages)), "saved summary") {
				t.Fatal("restored request lost context")
			}
			return emitTextResponse(emit, "continued", "end_turn", api.Usage{})
		},
	}}
	resumed := NewQueryEngine(QueryEngineConfig{SessionID: loaded.ID, InitialMessages: loaded.Messages, InitialHistory: loaded.History, APIClient: resumedClient, PersistSession: persist})
	output, _ = resumed.SubmitMessage(context.Background(), "continue")
	for range output {
	}
	loaded, err = store.Load(saved.ID)
	if err != nil || len(loaded.History) != 4 {
		t.Fatalf("resumed history=%+v err=%v", loaded, err)
	}
}

func TestSessionSaveFailureStopsBeforeAPI(t *testing.T) {
	client := &scriptedStreamClient{}
	engine := NewQueryEngine(QueryEngineConfig{APIClient: client, PersistSession: func([]types.Message, []types.Message) error { return errors.New("disk full") }})
	output, _ := engine.SubmitMessage(context.Background(), "hello")
	count := 0
	for event := range output {
		if r, ok := event.(ResultMessage); ok {
			count++
			if !r.IsError || r.Subtype != "session_save_error" {
				t.Fatalf("result=%+v", r)
			}
		}
	}
	if count != 1 || client.next != 0 {
		t.Fatalf("errors=%d API calls=%d", count, client.next)
	}
}
