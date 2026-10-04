package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"claude-code-go/internal/memory"
	"claude-code-go/internal/tools"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

type scriptedStreamClient struct {
	calls []func(api.MessageRequest, func(api.StreamEvent) error) error
	next  int
}

type staticMessageClient struct {
	request  api.MessageRequest
	response *api.MessageResponse
	err      error
}

func (c *staticMessageClient) CreateMessage(_ context.Context, request api.MessageRequest) (*api.MessageResponse, error) {
	c.request = request
	return c.response, c.err
}

func (c *staticMessageClient) CountTokens(context.Context, api.MessageRequest) (int, error) {
	return 0, errors.New("not implemented")
}

func (c *staticMessageClient) StreamMessage(context.Context, api.MessageRequest, func(api.StreamEvent) error) error {
	return errors.New("not implemented")
}

func (c *scriptedStreamClient) CreateMessage(context.Context, api.MessageRequest) (*api.MessageResponse, error) {
	return nil, errors.New("not implemented")
}

func (c *scriptedStreamClient) CountTokens(context.Context, api.MessageRequest) (int, error) {
	return 0, errors.New("not implemented")
}

func (c *scriptedStreamClient) StreamMessage(_ context.Context, req api.MessageRequest, emit func(api.StreamEvent) error) error {
	if c.next >= len(c.calls) {
		return errors.New("unexpected stream call")
	}
	call := c.calls[c.next]
	c.next++
	return call(req, emit)
}

func emitTextResponse(emit func(api.StreamEvent) error, text, stopReason string, usage api.Usage) error {
	if err := emit(api.StreamEvent{Type: "content_block_start", Index: 0, ContentBlock: &api.ContentBlock{Type: "text"}}); err != nil {
		return err
	}
	if err := emit(api.StreamEvent{Type: "content_block_delta", Index: 0, Delta: &api.EventDelta{Type: "text_delta", Text: text}}); err != nil {
		return err
	}
	return emit(api.StreamEvent{Type: "message_delta", Delta: &api.EventDelta{StopReason: stopReason}, Usage: &usage})
}

type gatedTool struct {
	*tools.BaseTool
	safe    bool
	started chan<- string
	release <-chan struct{}
}

type observableTool struct {
	*tools.BaseTool
	output string
	err    error
	limit  int
}

func (t *observableTool) Call(
	_ context.Context,
	_ json.RawMessage,
	toolCtx *types.ToolContext,
	_ types.CanUseToolFunc,
	_ *types.Message,
	onProgress func(interface{}),
) (*types.ToolResult, error) {
	if onProgress != nil {
		onProgress(map[string]interface{}{"status": "starting", "percent": 10})
		onProgress(map[string]interface{}{"status": "running", "percent": 50})
	}
	return &types.ToolResult{Output: t.output, Error: t.err, ToolUseID: toolCtx.ToolUseId}, nil
}

func (t *observableTool) MaxResultSizeChars() int { return t.limit }

func newGatedTool(name string, safe bool, started chan<- string, release <-chan struct{}) *gatedTool {
	return &gatedTool{
		BaseTool: tools.NewBaseTool(name, "test tool"),
		safe:     safe,
		started:  started,
		release:  release,
	}
}

func (t *gatedTool) IsConcurrencySafe(json.RawMessage) bool { return t.safe }

func (t *gatedTool) Call(
	ctx context.Context,
	_ json.RawMessage,
	toolCtx *types.ToolContext,
	_ types.CanUseToolFunc,
	_ *types.Message,
	_ func(interface{}),
) (*types.ToolResult, error) {
	select {
	case t.started <- t.Name():
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-t.release:
		return &types.ToolResult{Output: t.Name(), ToolUseID: toolCtx.ToolUseId}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

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

func TestExecuteToolsRunsSafeBatchConcurrentlyAndPreservesOrder(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	first := newGatedTool("First", true, started, release)
	second := newGatedTool("Second", true, started, release)
	engine := NewQueryEngine(QueryEngineConfig{
		Tools:              []types.Tool{first, second},
		UserSpecifiedModel: "test",
	})
	output := make(chan interface{}, 2)
	resultCh := make(chan []types.Message, 1)
	go func() {
		resultCh <- engine.executeTools(context.Background(), []api.ContentBlock{
			{Type: "tool_use", ID: "tool_1", Name: "First", Input: json.RawMessage(`{}`)},
			{Type: "tool_use", ID: "tool_2", Name: "Second", Input: json.RawMessage(`{}`)},
		}, output)
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case name := <-started:
			seen[name] = true
		case <-time.After(time.Second):
			t.Fatalf("safe tools did not start concurrently; started=%v", seen)
		}
	}
	close(release)
	results := <-resultCh

	if len(results) != 1 {
		t.Fatalf("got %d merged result messages, want 1", len(results))
	}
	var blocks []api.ContentBlock
	if err := json.Unmarshal(results[0].Content, &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].ToolUseID != "tool_1" || blocks[1].ToolUseID != "tool_2" {
		t.Fatalf("tool results lost model order: %#v", blocks)
	}
}

func TestExecuteToolsUsesUnsafeCallsAsSerialBarriers(t *testing.T) {
	started := make(chan string, 2)
	firstRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	first := newGatedTool("First", false, started, firstRelease)
	second := newGatedTool("Second", false, started, secondRelease)
	engine := NewQueryEngine(QueryEngineConfig{
		Tools:              []types.Tool{first, second},
		UserSpecifiedModel: "test",
	})
	output := make(chan interface{}, 2)
	done := make(chan struct{})
	go func() {
		engine.executeTools(context.Background(), []api.ContentBlock{
			{Type: "tool_use", ID: "tool_1", Name: "First", Input: json.RawMessage(`{}`)},
			{Type: "tool_use", ID: "tool_2", Name: "Second", Input: json.RawMessage(`{}`)},
		}, output)
		close(done)
	}()

	if name := <-started; name != "First" {
		t.Fatalf("first serial tool = %q, want First", name)
	}
	select {
	case name := <-started:
		t.Fatalf("unsafe tool %q started before the serial barrier was released", name)
	case <-time.After(50 * time.Millisecond):
	}
	close(firstRelease)
	select {
	case name := <-started:
		if name != "Second" {
			t.Fatalf("second serial tool = %q, want Second", name)
		}
	case <-time.After(time.Second):
		t.Fatal("second unsafe tool did not start after first completed")
	}
	close(secondRelease)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("serial tool execution did not finish")
	}
}

func TestValidateToolInput(t *testing.T) {
	schema := types.ToolInputJSONSchema{
		Type: "object",
		Properties: map[string]map[string]interface{}{
			"path":  {"type": "string", "minLength": 2, "maxLength": 20, "pattern": `^[a-z]`},
			"count": {"type": "integer", "minimum": 1, "maximum": 5},
			"force": {"type": "boolean"},
			"mode":  {"type": "string", "enum": []string{"read", "write"}},
			"tags":  {"type": "array", "minItems": 1, "items": map[string]interface{}{"type": "string"}},
			"options": {
				"type": "object",
				"properties": map[string]interface{}{
					"level": map[string]interface{}{"type": "integer", "minimum": 0},
				},
				"required":             []string{"level"},
				"additionalProperties": false,
			},
		},
		Required: []string{"path"},
	}

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid", input: `{"path":"go.mod","count":2,"force":false}`},
		{name: "missing required", input: `{"count":2}`, wantErr: `missing required field "path"`},
		{name: "wrong primitive type", input: `{"path":42}`, wantErr: `field "path" must be string`},
		{name: "fractional integer", input: `{"path":"go.mod","count":1.5}`, wantErr: `field "count" must be integer`},
		{name: "enum", input: `{"path":"go.mod","mode":"delete"}`, wantErr: `field "mode" must be one of`},
		{name: "string minimum", input: `{"path":"g"}`, wantErr: `field "path" must contain at least 2`},
		{name: "string pattern", input: `{"path":"Go.mod"}`, wantErr: `field "path" must match pattern`},
		{name: "number minimum", input: `{"path":"go.mod","count":0}`, wantErr: `field "count" must be >= 1`},
		{name: "number maximum", input: `{"path":"go.mod","count":6}`, wantErr: `field "count" must be <= 5`},
		{name: "array minimum", input: `{"path":"go.mod","tags":[]}`, wantErr: `field "tags" must contain at least 1 items`},
		{name: "array item", input: `{"path":"go.mod","tags":[1]}`, wantErr: `field "tags[0]" must be string`},
		{name: "nested required", input: `{"path":"go.mod","options":{}}`, wantErr: `field "options" missing required field "level"`},
		{name: "nested unknown", input: `{"path":"go.mod","options":{"level":1,"extra":true}}`, wantErr: `field "options.extra" is not allowed`},
		{name: "non object", input: `[]`, wantErr: "expected a JSON object"},
		{name: "malformed", input: `{`, wantErr: "expected a JSON object"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateToolInput(json.RawMessage(test.input), schema)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected validation error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validation error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestSemanticToolValidationRunsBeforePermission(t *testing.T) {
	permissionCalled := false
	engine := NewQueryEngine(QueryEngineConfig{
		Tools: []types.Tool{tools.NewWebSearchTool()},
		CanUseTool: func(context.Context, string, json.RawMessage) (*types.PermissionDecision, error) {
			permissionCalled = true
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		},
	})
	outcome := engine.executeTool(context.Background(), api.ContentBlock{
		Type: "tool_use", ID: "search", Name: "WebSearch",
		Input: json.RawMessage(`{"query":"golang","allowed_domains":["go.dev"],"blocked_domains":["example.com"]}`),
	}, nil)
	if permissionCalled {
		t.Fatal("permission was requested for semantically invalid input")
	}
	data := outcome.sdkMessage.Message.(map[string]interface{})
	if data["is_error"] != true || !strings.Contains(data["content"].(string), "cannot specify both allowed_domains and blocked_domains") {
		t.Fatalf("unexpected semantic validation outcome: %#v", data)
	}
}

func TestPathNormalizationRunsBeforePermissionAndExecution(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "input.txt"), []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	var approvedPath string
	engine := NewQueryEngine(QueryEngineConfig{
		Cwd:   workspace,
		Tools: []types.Tool{tools.NewFileReadTool()},
		CanUseTool: func(_ context.Context, _ string, input json.RawMessage) (*types.PermissionDecision, error) {
			var value map[string]string
			if err := json.Unmarshal(input, &value); err != nil {
				t.Fatal(err)
			}
			approvedPath = value["target_file"]
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		},
	})
	outcome := engine.executeTool(context.Background(), api.ContentBlock{
		Type: "tool_use", ID: "read", Name: "Read", Input: json.RawMessage(`{"target_file":"input.txt"}`),
	}, nil)
	canonicalWorkspace, err := tools.CanonicalizePath(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalWorkspace, "input.txt")
	if approvedPath != want {
		t.Fatalf("permission saw path %q, want canonical %q", approvedPath, want)
	}
	data := outcome.sdkMessage.Message.(map[string]interface{})
	if data["is_error"] == true || !strings.Contains(data["content"].(string), "safe") {
		t.Fatalf("normalized execution failed: %#v", data)
	}
}

func TestExecuteToolEmitsProgressAndLimitsFinalResult(t *testing.T) {
	tool := &observableTool{
		BaseTool: tools.NewBaseTool("Observable", "test progress and truncation"),
		output:   strings.Repeat("界", 200),
		limit:    80,
	}
	engine := NewQueryEngine(QueryEngineConfig{
		Tools:              []types.Tool{tool},
		UserSpecifiedModel: "test",
	})
	output := make(chan interface{}, 3)
	results := engine.executeTools(context.Background(), []api.ContentBlock{{
		Type: "tool_use", ID: "tool_observable", Name: "Observable", Input: json.RawMessage(`{}`),
	}}, output)

	first := (<-output).(SDKMessage)
	second := (<-output).(SDKMessage)
	final := (<-output).(SDKMessage)
	if first.Type != "tool_progress" || second.Type != "tool_progress" || final.Type != "tool_result" {
		t.Fatalf("unexpected SDK event order: %s, %s, %s", first.Type, second.Type, final.Type)
	}
	secondData := second.Message.(map[string]interface{})["data"].(map[string]interface{})
	if secondData["status"] != "running" {
		t.Fatalf("latest progress payload = %#v", secondData)
	}
	finalData := final.Message.(map[string]interface{})
	if finalData["truncated"] != true || finalData["original_chars"] != 200 {
		t.Fatalf("missing truncation metadata: %#v", finalData)
	}

	if len(results) != 1 {
		t.Fatalf("result message count = %d, want 1", len(results))
	}
	var blocks []api.ContentBlock
	if err := json.Unmarshal(results[0].Content, &blocks); err != nil {
		t.Fatal(err)
	}
	content, ok := blocks[0].Content.(string)
	if !ok {
		t.Fatalf("tool result content type = %T, want string", blocks[0].Content)
	}
	if len([]rune(content)) != 80 || !strings.Contains(content, "truncated") {
		t.Fatalf("limited result has %d chars and content %q", len([]rune(content)), content)
	}
}

func TestExecuteToolPreservesOutputAlongsideError(t *testing.T) {
	tool := &observableTool{
		BaseTool: tools.NewBaseTool("Failing", "test failed output"),
		output:   "diagnostic output",
		err:      errors.New("exit status 1"),
		limit:    1024,
	}
	engine := NewQueryEngine(QueryEngineConfig{Tools: []types.Tool{tool}})
	outcome := engine.executeTool(context.Background(), api.ContentBlock{
		Type: "tool_use", ID: "failed_1", Name: "Failing", Input: json.RawMessage(`{}`),
	}, nil)
	data := outcome.sdkMessage.Message.(map[string]interface{})
	content := data["content"].(string)
	if data["is_error"] != true || !strings.Contains(content, "diagnostic output") || !strings.Contains(content, "exit status 1") {
		t.Fatalf("failed tool output lost diagnostics: %#v", data)
	}
}

func TestExecuteToolEmitsDisplayMetadataOnlyToSDK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.txt")
	engine := NewQueryEngine(QueryEngineConfig{Tools: []types.Tool{tools.NewFileWriteTool()}})
	outcome := engine.executeTool(context.Background(), api.ContentBlock{
		Type: "tool_use", ID: "write_1", Name: "Write",
		Input: mustMarshalJSON(map[string]string{"file_path": path, "contents": "hello\n"}),
	}, nil)
	data := outcome.sdkMessage.Message.(map[string]interface{})
	display, ok := data["display"].(*types.ToolDisplay)
	if !ok || filepath.Base(display.FilePath) != filepath.Base(path) || !strings.Contains(display.Diff, "+hello") {
		t.Fatalf("SDK display metadata = %#v", data["display"])
	}
	if strings.Contains(string(outcome.message.Content), "artifacts") || strings.Contains(string(outcome.message.Content), "diff") {
		t.Fatalf("model-facing tool result leaked display metadata: %s", outcome.message.Content)
	}
}

func TestQueryEngineRecoversFromMaxOutputTokens(t *testing.T) {
	client := &scriptedStreamClient{calls: []func(api.MessageRequest, func(api.StreamEvent) error) error{
		func(req api.MessageRequest, emit func(api.StreamEvent) error) error {
			if req.MaxTokens != 1234 {
				t.Errorf("max tokens = %d, want configured 1234", req.MaxTokens)
			}
			return emitTextResponse(emit, "partial ", "max_tokens", api.Usage{InputTokens: 2, OutputTokens: 4})
		},
		func(req api.MessageRequest, emit func(api.StreamEvent) error) error {
			if len(req.Messages) != 3 {
				t.Errorf("recovery request has %d messages, want 3", len(req.Messages))
			}
			if got := req.Messages[1].Content[0].Text; got != "partial " {
				t.Errorf("preserved assistant text = %q", got)
			}
			return emitTextResponse(emit, "continued", "end_turn", api.Usage{InputTokens: 5, OutputTokens: 2})
		},
	}}
	engine := NewQueryEngine(QueryEngineConfig{
		APIClient: client, MaxTokens: 1234, MaxTurns: 4, UserSpecifiedModel: "test", CustomSystemPrompt: "test",
	})
	output, err := engine.SubmitMessage(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	var result ResultMessage
	var recoveries int
	for event := range output {
		switch event := event.(type) {
		case SDKMessage:
			if event.Type == "system" {
				if data, ok := event.Message.(map[string]interface{}); ok && data["subtype"] == "recovery" {
					recoveries++
				}
			}
		case ResultMessage:
			result = event
		}
	}
	if recoveries != 1 || result.IsError || result.Result != "continued" || result.NumTurns != 2 {
		t.Fatalf("unexpected recovery result: recoveries=%d result=%#v", recoveries, result)
	}
	if result.Usage.InputTokens != 7 || result.Usage.OutputTokens != 6 {
		t.Fatalf("recovery usage = %#v", result.Usage)
	}
}

func TestQueryEngineRecoversOnceFromInterruptedStream(t *testing.T) {
	client := &scriptedStreamClient{calls: []func(api.MessageRequest, func(api.StreamEvent) error) error{
		func(_ api.MessageRequest, emit func(api.StreamEvent) error) error {
			if err := emitTextResponse(emit, "cut off", "", api.Usage{InputTokens: 2, OutputTokens: 2}); err != nil {
				return err
			}
			return errors.New("connection reset")
		},
		func(req api.MessageRequest, emit func(api.StreamEvent) error) error {
			if len(req.Messages) != 3 || req.Messages[1].Content[0].Text != "cut off" {
				t.Errorf("interrupted response was not preserved safely: %#v", req.Messages)
			}
			return emitTextResponse(emit, "resumed", "end_turn", api.Usage{InputTokens: 4, OutputTokens: 1})
		},
	}}
	engine := NewQueryEngine(QueryEngineConfig{
		APIClient: client, MaxTurns: 4, UserSpecifiedModel: "test", CustomSystemPrompt: "test",
	})
	output, _ := engine.SubmitMessage(context.Background(), "go")
	var result ResultMessage
	for event := range output {
		if value, ok := event.(ResultMessage); ok {
			result = value
		}
	}
	if result.IsError || result.Result != "resumed" || client.next != 2 {
		t.Fatalf("unexpected interrupted stream recovery: calls=%d result=%#v", client.next, result)
	}
}

func TestToolResultsRespectAggregatePerTurnBudget(t *testing.T) {
	const resultSize = 45000
	toolList := make([]types.Tool, 5)
	blocks := make([]api.ContentBlock, 5)
	for i := range toolList {
		name := fmt.Sprintf("Large%d", i)
		toolList[i] = &observableTool{BaseTool: tools.NewBaseTool(name, "large output"), output: strings.Repeat("界", resultSize)}
		blocks[i] = api.ContentBlock{Type: "tool_use", ID: fmt.Sprintf("tool_%d", i), Name: name, Input: json.RawMessage(`{}`)}
	}
	engine := NewQueryEngine(QueryEngineConfig{Tools: toolList, UserSpecifiedModel: "test"})
	output := make(chan interface{}, 20)
	results := engine.executeTools(context.Background(), blocks, output)
	if len(results) != 1 {
		t.Fatalf("merged results = %d, want 1", len(results))
	}
	var resultBlocks []api.ContentBlock
	if err := json.Unmarshal(results[0].Content, &resultBlocks); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, block := range resultBlocks {
		total += len([]rune(block.Content.(string)))
	}
	if total > 200000 {
		t.Fatalf("aggregate tool result size = %d, want <= 200000", total)
	}
	truncated := 0
	for len(output) > 0 {
		event := (<-output).(SDKMessage)
		if event.Type == "tool_result" && event.Message.(map[string]interface{})["truncated"] == true {
			truncated++
		}
	}
	if truncated == 0 {
		t.Fatal("aggregate budget did not mark any SDK tool result as truncated")
	}
}

func TestBuildSystemPromptAppendsMemoryAfterCustomPrompt(t *testing.T) {
	engine := NewQueryEngine(QueryEngineConfig{
		CustomSystemPrompt: "custom prompt",
		AppendSystemPrompt: "caller appendix",
		MemoryPrompt:       "memory snapshot",
	})
	got := engine.buildSystemPrompt()
	if got != "custom prompt\n\ncaller appendix\n\nmemory snapshot" {
		t.Fatalf("system prompt = %q", got)
	}
}

func TestQueryEngineRecallsMemoryOnceAndInjectsItAsContext(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "editor.md")
	if err := os.WriteFile(path, []byte("---\ndescription: preferred editor\ntype: user\n---\nUse vim."), 0o600); err != nil {
		t.Fatal(err)
	}
	selectorCalls := 0
	engine := NewQueryEngine(QueryEngineConfig{
		MemoryDirectory: directory,
		MemorySelector: func(_ context.Context, query, manifest string) ([]string, error) {
			selectorCalls++
			if !strings.Contains(query, "preferred editor") || !strings.Contains(manifest, "editor.md") || strings.Contains(manifest, "Use vim") {
				t.Fatalf("query=%q manifest=%q", query, manifest)
			}
			return []string{"editor.md"}, nil
		},
	})

	recalled := engine.recallMemories(context.Background(), "use my preferred editor")
	if len(recalled) != 1 || selectorCalls != 1 {
		t.Fatalf("recalled=%d selectorCalls=%d", len(recalled), selectorCalls)
	}
	messages := engine.processUserInput("use my preferred editor", recalled)
	var blocks []api.ContentBlock
	if err := json.Unmarshal(messages[0].Content, &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || !strings.Contains(blocks[1].Text, "Use vim") || !strings.Contains(blocks[1].Text, "may be stale") {
		t.Fatalf("unexpected user context: %#v", blocks)
	}
	if again := engine.recallMemories(context.Background(), "use my preferred editor again"); len(again) != 0 || selectorCalls != 1 {
		t.Fatalf("duplicate recall=%d selectorCalls=%d", len(again), selectorCalls)
	}
}

func TestMemorySelectorUsesBoundedJSONSideQuery(t *testing.T) {
	client := &staticMessageClient{response: &api.MessageResponse{
		Content: []api.ContentBlock{{Type: "text", Text: "```json\n{\"selected_memories\":[\"editor.md\"]}\n```"}},
		Usage:   api.Usage{InputTokens: 7, OutputTokens: 3},
	}}
	engine := NewQueryEngine(QueryEngineConfig{APIClient: client, UserSpecifiedModel: "selector-model"})
	selected, err := engine.selectRelevantMemories(context.Background(), "use my editor", "- editor.md: preferred editor")
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0] != "editor.md" {
		t.Fatalf("selected = %#v", selected)
	}
	if client.request.Model != "selector-model" || client.request.MaxTokens != 256 || len(client.request.Messages) != 1 {
		t.Fatalf("selector request = %#v", client.request)
	}
	if !strings.Contains(client.request.Messages[0].Content[0].Text, "Available memories") {
		t.Fatalf("selector prompt = %q", client.request.Messages[0].Content[0].Text)
	}
	if engine.totalUsage.InputTokens != 7 || engine.totalUsage.OutputTokens != 3 {
		t.Fatalf("selector usage = %#v", engine.totalUsage)
	}
}

func TestMemoryRecallStopsAtSessionBudget(t *testing.T) {
	calls := 0
	engine := NewQueryEngine(QueryEngineConfig{
		MemoryDirectory: t.TempDir(),
		MemorySelector: func(context.Context, string, string) ([]string, error) {
			calls++
			return nil, nil
		},
	})
	engine.memoryRecallBytes = 60 * 1024
	if recalled := engine.recallMemories(context.Background(), "query with enough context"); len(recalled) != 0 || calls != 0 {
		t.Fatalf("recalled=%d selectorCalls=%d", len(recalled), calls)
	}
}

func TestQueryEngineUpdatesSessionMemoryAtNaturalBreakpoint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	store, err := memory.NewSessionMemory(t.TempDir(), "query-session", memory.SessionMemoryConfig{
		MinimumMessageTokensToInit: 1,
		MinimumTokensBetweenUpdate: 1,
		ToolCallsBetweenUpdates:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	summary := strings.Replace(
		memory.DefaultSessionMemoryTemplate(),
		"# Current State\n_What is actively being worked on right now? Pending tasks not yet completed. Immediate next steps._",
		"# Current State\n_What is actively being worked on right now? Pending tasks not yet completed. Immediate next steps._\n\nSession memory is implemented.",
		1,
	)
	client := &staticMessageClient{response: &api.MessageResponse{
		Content: []api.ContentBlock{{Type: "text", Text: summary}},
		Usage:   api.Usage{InputTokens: 20, OutputTokens: 10},
	}}
	engine := NewQueryEngine(QueryEngineConfig{
		APIClient:     client,
		SessionMemory: store,
	})
	engine.mutableMessages = []types.Message{{
		Role: "user",
		Content: mustMarshalJSON([]api.ContentBlock{
			{Type: "text", Text: "Implement session memory"},
			{Type: "text", Text: "<system-reminder>\nRelevant file-based memories should not be summarized</system-reminder>"},
		}),
	}}
	engine.startSessionMemoryUpdate(context.Background(), &api.MessageResponse{Usage: api.Usage{InputTokens: 10}})
	if !engine.WaitForSessionMemory(context.Background()) {
		t.Fatal("session memory update did not finish")
	}
	content, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Session memory is implemented") {
		t.Fatalf("session memory content: %s", content)
	}
	for _, message := range client.request.Messages {
		for _, block := range message.Content {
			if strings.Contains(block.Text, "Relevant file-based memories should not be summarized") {
				t.Fatal("recalled long-term memory leaked into session summary transcript")
			}
		}
	}
}

func TestCompactPrefersSessionMemoryAndKeepsRecentMessages(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_HOME", t.TempDir())
	store, err := memory.NewSessionMemory(t.TempDir(), "compact-session", memory.DefaultSessionMemoryConfig)
	if err != nil {
		t.Fatal(err)
	}
	summary := strings.Replace(
		memory.DefaultSessionMemoryTemplate(),
		"# Current State\n_What is actively being worked on right now? Pending tasks not yet completed. Immediate next steps._",
		"# Current State\n_What is actively being worked on right now? Pending tasks not yet completed. Immediate next steps._\n\nContinue compact integration.",
		1,
	)
	if err := store.Save(summary, 20_000, 6); err != nil {
		t.Fatal(err)
	}
	engine := NewQueryEngine(QueryEngineConfig{SessionMemory: store})
	for index := 0; index < 10; index++ {
		engine.mutableMessages = append(engine.mutableMessages, types.Message{
			Role:    map[bool]string{true: "user", false: "assistant"}[index%2 == 0],
			Content: mustMarshalJSON([]api.ContentBlock{{Type: "text", Text: fmt.Sprintf("message %d %s", index, strings.Repeat("x", 8_000))}}),
		})
	}
	output, err := engine.Compact(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var status string
	for event := range output {
		if sdk, ok := event.(SDKMessage); ok {
			status, _ = sdk.Message.(map[string]interface{})["content"].(string)
		}
	}
	if !strings.Contains(status, "session memory") {
		t.Fatalf("compact status = %q", status)
	}
	messages := engine.GetMessages()
	if len(messages) != 6 {
		t.Fatalf("post-compact messages = %d, want summary + 5 recent", len(messages))
	}
	if !strings.Contains(string(messages[0].Content), "Continue compact integration") {
		t.Fatalf("compact summary = %s", messages[0].Content)
	}
}

func TestSelectMessagesForCompactKeepsToolPair(t *testing.T) {
	messages := []types.Message{
		{Role: "assistant", Content: mustMarshalJSON([]api.ContentBlock{{Type: "tool_use", ID: "tool-1", Name: "Read"}})},
		{Role: "user", Content: mustMarshalJSON([]api.ContentBlock{{Type: "tool_result", ToolUseID: "tool-1", Content: strings.Repeat("x", 40_000)}})},
	}
	for index := 0; index < 5; index++ {
		messages = append(messages, types.Message{Role: "user", Content: mustMarshalJSON([]api.ContentBlock{{Type: "text", Text: fmt.Sprintf("text %d", index)}})})
	}
	kept := selectMessagesForCompact(messages, 1)
	if len(kept) != len(messages) || kept[0].Role != "assistant" {
		t.Fatalf("tool pair was split: %#v", kept)
	}
}

func TestCompactFallsBackToModelSummary(t *testing.T) {
	client := &staticMessageClient{response: &api.MessageResponse{
		Content: []api.ContentBlock{{Type: "text", Text: "<summary>Fallback compact summary.</summary>"}},
	}}
	engine := NewQueryEngine(QueryEngineConfig{APIClient: client, UserSpecifiedModel: "compact-model"})
	for index := 0; index < 6; index++ {
		engine.mutableMessages = append(engine.mutableMessages, types.Message{
			Role:    "user",
			Content: mustMarshalJSON([]api.ContentBlock{{Type: "text", Text: fmt.Sprintf("message %d", index)}}),
		})
	}
	output, err := engine.Compact(context.Background(), "focus on tests")
	if err != nil {
		t.Fatal(err)
	}
	for range output {
	}
	messages := engine.GetMessages()
	if !strings.Contains(string(messages[0].Content), "Fallback compact summary") {
		t.Fatalf("fallback compact summary = %s", messages[0].Content)
	}
	if client.request.Model != "compact-model" || !strings.Contains(client.request.Messages[len(client.request.Messages)-1].Content[0].Text, "focus on tests") {
		t.Fatalf("compact request = %#v", client.request)
	}
}
