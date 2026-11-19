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

func TestAgentFollowupRetriesTextEncodedToolCall(t *testing.T) {
	const markup = `<｜｜DSML｜｜ calls> <｜｜DSML｜｜ invoke name="Bash">ignored\</｜｜DSML｜｜ invoke> \</｜｜DSML｜｜ calls>\`
	client := &scriptedStreamClient{calls: []func(api.MessageRequest, func(api.StreamEvent) error) error{
		func(_ api.MessageRequest, emit func(api.StreamEvent) error) error {
			for _, part := range []string{markup[:1], markup[1:]} {
				if err := emit(api.StreamEvent{Type: "content_block_delta", Delta: &api.EventDelta{Type: "text_delta", Text: part}}); err != nil {
					return err
				}
			}
			return emit(api.StreamEvent{Type: "message_delta", Delta: &api.EventDelta{StopReason: "end_turn"}})
		},
		func(req api.MessageRequest, emit func(api.StreamEvent) error) error {
			system, _ := req.System.(string)
			if len(req.Tools) != 0 || !strings.Contains(system, "previous attempt returned a tool call") {
				t.Error("retry must ask for prose without enabling tools")
			}
			return emitTextResponse(emit, "Task complete.", "end_turn", api.Usage{})
		},
	}}
	engine := NewQueryEngine(QueryEngineConfig{APIClient: client, MaxTurns: 1})
	stream, err := engine.SubmitAgentFollowup(context.Background(), "Automatic sub-agent update: done")
	if err != nil {
		t.Fatal(err)
	}
	var final ResultMessage
	for event := range stream {
		switch m := event.(type) {
		case SDKMessage:
			if m.Type == "assistant_delta" {
				t.Error("unverified text was streamed")
			}
		case ResultMessage:
			final = m
		}
	}
	if final.IsError || final.Result != "Task complete." || client.next != 2 {
		t.Fatalf("unexpected result: %+v, API calls: %d", final, client.next)
	}
	for _, message := range engine.GetMessages() {
		if strings.Contains(string(message.Content), "DSML") {
			t.Fatal("tool markup leaked into session history")
		}
	}
}

func TestStripDSMLKeepsSurroundingSummary(t *testing.T) {
	input := "已生成 TOOLS.md。\n" +
		`<｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="Bash">ignored\</｜｜DSML｜｜ invoke>\</｜｜DSML｜｜ calls>\` +
		"\n结果来自子 Agent。"
	got, changed := stripDSMLCalls(input)
	if !changed || got != "已生成 TOOLS.md。\n\n结果来自子 Agent。" {
		t.Fatalf("unexpected cleaned text: %q", got)
	}
}
