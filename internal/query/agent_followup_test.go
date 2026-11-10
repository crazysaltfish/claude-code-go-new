package query

import (
	"context"
	"strings"
	"testing"

	"claude-code-go/internal/tools"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

func TestAgentFollowupUsesSessionHistoryWithoutTools(t *testing.T) {
	client := &scriptedStreamClient{calls: []func(api.MessageRequest, func(api.StreamEvent) error) error{
		func(req api.MessageRequest, emit func(api.StreamEvent) error) error {
			if len(req.Tools) == 0 {
				t.Error("ordinary turn did not advertise its tools")
			}
			return emitTextResponse(emit, "I started the agent", "end_turn", api.Usage{})
		},
		func(req api.MessageRequest, emit func(api.StreamEvent) error) error {
			if len(req.Tools) != 0 {
				t.Error("automatic turn advertised tools")
			}
			if len(req.Messages) < 3 {
				t.Errorf("automatic turn lost the conversation: %+v", req.Messages)
			}
			return emitTextResponse(emit, "The task is complete", "end_turn", api.Usage{})
		},
	}}
	engine := NewQueryEngine(QueryEngineConfig{
		SessionID: "same-session", APIClient: client, Tools: []types.Tool{tools.NewAgentTool()},
	})
	ctx := context.Background()
	stream, err := engine.SubmitMessage(ctx, "Run an agent")
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	stream, err = engine.SubmitAgentFollowup(ctx, "Automatic sub-agent update: done")
	if err != nil {
		t.Fatal(err)
	}
	var final ResultMessage
	for event := range stream {
		if result, ok := event.(ResultMessage); ok {
			final = result
		}
	}
	if final.IsError || final.SessionID != "same-session" || !strings.Contains(final.Result, "task is complete") {
		t.Fatalf("unexpected follow-up result: %+v", final)
	}
}
