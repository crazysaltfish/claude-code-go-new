package query

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"claude-code-go/internal/tools"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

func TestQueryEngineToolRoundTrip(t *testing.T) {
	target := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(target, []byte("tool output"), 0600); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request api.MessageRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !request.Stream {
			t.Error("query engine did not request an Anthropic stream")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			arguments, err := json.Marshal(map[string]string{"target_file": target})
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintln(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"test","usage":{"input_tokens":2,"output_tokens":0}}}`)
			fmt.Fprintln(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"Read","input":{}}}`)
			fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":%q}}\n\n", string(arguments))
			fmt.Fprintln(w, `data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`)
			fmt.Fprintln(w, `data: {"type":"message_stop"}`)
			return
		}
		if len(request.Messages) < 3 {
			t.Errorf("second request has %d messages", len(request.Messages))
		} else {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "user" || len(last.Content) != 1 || last.Content[0].Type != "tool_result" {
				t.Errorf("unexpected tool result message: %#v", last)
			}
		}
		fmt.Fprintln(w, `data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"test","usage":{"input_tokens":4,"output_tokens":0}}}`)
		fmt.Fprintln(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"final "}}`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"answer"}}`)
		fmt.Fprintln(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))
	defer server.Close()

	engine := NewQueryEngine(QueryEngineConfig{
		Cwd:       t.TempDir(),
		Tools:     []types.Tool{tools.NewFileReadTool()},
		MaxTurns:  3,
		APIClient: api.NewClient(api.Config{BaseURL: server.URL, APIKey: "test", MaxRetries: 1}),
		CanUseTool: func(context.Context, string, json.RawMessage) (*types.PermissionDecision, error) {
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		},
		GetAppState:        func() *types.AppState { return &types.AppState{} },
		UserSpecifiedModel: "test",
		CustomSystemPrompt: "test",
	})

	output, err := engine.SubmitMessage(context.Background(), "read the file")
	if err != nil {
		t.Fatal(err)
	}
	var result ResultMessage
	var sawToolResult bool
	for event := range output {
		switch event := event.(type) {
		case SDKMessage:
			sawToolResult = sawToolResult || event.Type == "tool_result"
		case ResultMessage:
			result = event
		}
	}

	if !sawToolResult {
		t.Fatal("tool result event was not emitted")
	}
	if result.IsError || result.Result != "final answer" || result.NumTurns != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Usage.InputTokens != 6 || result.Usage.OutputTokens != 8 {
		t.Fatalf("unexpected usage: %#v", result.Usage)
	}
}

func TestQueryEngineOpenAIToolRoundTrip(t *testing.T) {
	target := filepath.Join(t.TempDir(), "openai-input.txt")
	if err := os.WriteFile(target, []byte("openai tool output"), 0600); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var request struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role       string      `json:"role"`
				Content    interface{} `json:"content"`
				ToolCallID string      `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !request.Stream {
			t.Error("query engine did not request an OpenAI stream")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			arguments, err := json.Marshal(map[string]string{"target_file": target})
			if err != nil {
				t.Error(err)
				http.Error(w, "failed to encode arguments", http.StatusInternalServerError)
				return
			}
			fmt.Fprintln(w, `data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":""}]}`)
			fmt.Fprintf(w, "data: {\"id\":\"chatcmpl_1\",\"object\":\"chat.completion.chunk\",\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"Read\",\"arguments\":%q}}]},\"finish_reason\":\"\"}]}\n\n", string(arguments[:len(arguments)/2]))
			fmt.Fprintf(w, "data: {\"id\":\"chatcmpl_1\",\"object\":\"chat.completion.chunk\",\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":%q}}]},\"finish_reason\":\"\"}]}\n\n", string(arguments[len(arguments)/2:]))
			fmt.Fprintln(w, `data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
			fmt.Fprintln(w, `data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3}}`)
			fmt.Fprintln(w, `data: [DONE]`)
			return
		}

		if len(request.Messages) < 4 {
			t.Errorf("second OpenAI request has %d messages", len(request.Messages))
		} else {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" ||
				last.ToolCallID != "call_1" ||
				!strings.Contains(fmt.Sprint(last.Content), "openai tool output") {
				t.Errorf("unexpected OpenAI tool result message: %#v", last)
			}
		}
		fmt.Fprintln(w, `data: {"id":"chatcmpl_2","object":"chat.completion.chunk","model":"test","choices":[{"index":0,"delta":{"content":"OpenAI "},"finish_reason":""}]}`)
		fmt.Fprintln(w, `data: {"id":"chatcmpl_2","object":"chat.completion.chunk","model":"test","choices":[{"index":0,"delta":{"content":"final answer"},"finish_reason":"stop"}]}`)
		fmt.Fprintln(w, `data: {"id":"chatcmpl_2","object":"chat.completion.chunk","model":"test","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":5}}`)
		fmt.Fprintln(w, `data: [DONE]`)
	}))
	defer server.Close()

	engine := NewQueryEngine(QueryEngineConfig{
		Cwd:       t.TempDir(),
		Tools:     []types.Tool{tools.NewFileReadTool()},
		MaxTurns:  3,
		APIClient: api.NewOpenAIClient(api.Config{BaseURL: server.URL, APIKey: "test", MaxRetries: 1}),
		CanUseTool: func(context.Context, string, json.RawMessage) (*types.PermissionDecision, error) {
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		},
		GetAppState:        func() *types.AppState { return &types.AppState{} },
		UserSpecifiedModel: "test",
		CustomSystemPrompt: "test",
	})

	output, err := engine.SubmitMessage(context.Background(), "read the file")
	if err != nil {
		t.Fatal(err)
	}
	var result ResultMessage
	var sawToolResult bool
	var streamedText strings.Builder
	for event := range output {
		switch event := event.(type) {
		case SDKMessage:
			sawToolResult = sawToolResult || event.Type == "tool_result"
			if event.Type == "assistant_delta" {
				streamedText.WriteString(event.Message.(AssistantDelta).Text)
			}
		case ResultMessage:
			result = event
		}
	}

	if !sawToolResult {
		t.Fatal("OpenAI tool result event was not emitted")
	}
	if result.IsError || result.Result != "OpenAI final answer" || result.NumTurns != 2 {
		t.Fatalf("unexpected OpenAI result: %#v", result)
	}
	if result.Usage.InputTokens != 6 || result.Usage.OutputTokens != 8 {
		t.Fatalf("unexpected OpenAI usage: %#v", result.Usage)
	}
	if streamedText.String() != "OpenAI final answer" {
		t.Fatalf("unexpected streamed text: %q", streamedText.String())
	}
}
