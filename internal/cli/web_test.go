package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"claude-code-go/internal/query"
	"claude-code-go/internal/sessions"
	"claude-code-go/internal/tasks"
	"claude-code-go/internal/tools"
	"claude-code-go/internal/ui"
	"claude-code-go/pkg/api"
)

type webScriptClient struct {
	started chan struct{}
	release chan struct{}
}

func (c *webScriptClient) CreateMessage(context.Context, api.MessageRequest) (*api.MessageResponse, error) {
	return nil, errors.New("unused")
}
func (c *webScriptClient) CountTokens(context.Context, api.MessageRequest) (int, error) {
	return 0, errors.New("unused")
}
func (c *webScriptClient) StreamMessage(ctx context.Context, req api.MessageRequest, emit func(api.StreamEvent) error) error {
	if c.started != nil {
		c.started <- struct{}{}
	}
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := emit(api.StreamEvent{Type: "content_block_start", Index: 0, ContentBlock: &api.ContentBlock{Type: "text"}}); err != nil {
		return err
	}
	if err := emit(api.StreamEvent{Type: "content_block_delta", Index: 0, Delta: &api.EventDelta{Type: "text_delta", Text: "Hello from Go"}}); err != nil {
		return err
	}
	return emit(api.StreamEvent{Type: "message_delta", Delta: &api.EventDelta{StopReason: "end_turn"}})
}
func webTestServer(t *testing.T, client api.MessageClient) (*webServer, *webChat, http.Handler) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_HOME", t.TempDir())
	app := NewApp(&Config{Cwd: t.TempDir(), Model: "test-model", Provider: "openai"}, "test")
	store, err := sessions.NewStore(app.config.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	app.sessionStore = store
	app.session = store.New("test-model", "openai")
	app.toolRegistry = tools.NewToolRegistry()
	app.queryEngine = query.NewQueryEngine(query.QueryEngineConfig{SessionID: app.session.ID, Cwd: app.config.Cwd, APIClient: client, PersistSession: app.persistSession})
	s := &webServer{root: app, chats: make(map[string]*webChat)}
	chat, err := s.add(app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.cancel)
	return s, chat, s.handler("127.0.0.1:8080")
}
func webRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
func awaitWeb(t *testing.T, chat *webChat, predicate func() bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		chat.mu.Lock()
		done := predicate()
		changed := chat.changed
		chat.mu.Unlock()
		if done {
			return
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatal("timed out waiting for web runtime")
		}
	}
}
func TestWebStreamsPersistsAndSerializesTurns(t *testing.T) {
	client := &webScriptClient{started: make(chan struct{}, 1), release: make(chan struct{})}
	s, chat, handler := webTestServer(t, client)
	path := "/api/chats/" + chat.id
	if response := webRequest(handler, "POST", path+"/messages", `{"message":"hello"}`); response.Code != 202 {
		t.Fatal(response.Body.String())
	}
	select {
	case <-client.started:
	case <-time.After(3 * time.Second):
		t.Fatal("query did not start")
	}
	if response := webRequest(handler, "POST", path+"/messages", `{"message":"overlap"}`); response.Code != 409 {
		t.Fatalf("accepted overlapping query: %d", response.Code)
	}
	close(client.release)
	awaitWeb(t, chat, func() bool { return !chat.busy })
	chat.mu.Lock()
	encoded, _ := json.Marshal(chat.events)
	last := len(chat.events)
	chat.mu.Unlock()
	if !strings.Contains(string(encoded), "Hello from Go") {
		t.Fatal("missing streamed text")
	}
	saved, err := s.root.sessionStore.Load(chat.id)
	if err != nil || len(saved.History) != 2 {
		t.Fatalf("history not persisted: %v, %+v", err, saved)
	}
	// A reconnect starts after its last seen event, without replaying old text.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest("GET", "http://127.0.0.1:8080"+path+"/events", nil).WithContext(ctx)
	request.Header.Set("Last-Event-ID", jsonNumber(last-1))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), `"kind":"busy","data":false`) || strings.Contains(recorder.Body.String(), "Hello from Go") {
		t.Fatal("incorrect SSE replay", recorder.Body.String())
	}
}
func jsonNumber(value int) string { data, _ := json.Marshal(value); return string(data) }

func TestWebOriginGuardsAndEmbeddedAssets(t *testing.T) {
	_, _, handler := webTestServer(t, &webScriptClient{})
	for _, target := range []string{"/", "/app.js", "/style.css"} {
		res := webRequest(handler, "GET", target, "")
		if res.Code != 200 || res.Body.Len() == 0 {
			t.Fatalf("asset %s unavailable: %d", target, res.Code)
		}
		if res.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
	}
	for _, header := range []string{"Origin", "Host", "Sec-Fetch-Site"} {
		req := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/config", nil)
		switch header {
		case "Origin":
			req.Header.Set(header, "https://untrusted.example")
		case "Host":
			req.Host = "untrusted.example:8080"
		case "Sec-Fetch-Site":
			req.Header.Set(header, "cross-site")
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != 403 {
			t.Fatalf("%s guard failed: %d", header, res.Code)
		}
	}
}
func TestWebApprovalAndStop(t *testing.T) {
	_, chat, handler := webTestServer(t, &webScriptClient{})
	response := make(chan ui.PermissionResponse, 1)
	go func() {
		chat.app.permissionRequests <- ui.PermissionRequest{ToolName: "Write", Input: json.RawMessage(`{"file_path":"/tmp/example"}`), Response: response}
	}()
	awaitWeb(t, chat, func() bool { return len(chat.approvals) == 1 })
	chat.mu.Lock()
	var id string
	for key := range chat.approvals {
		id = key
	}
	chat.mu.Unlock()
	res := webRequest(handler, "POST", "/api/chats/"+chat.id+"/approve", `{"id":"`+id+`","decision":"allow_once"}`)
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	select {
	case got := <-response:
		if got != ui.PermissionAllowOnce {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("approval not delivered")
	}
	res = webRequest(handler, "POST", "/api/chats/"+chat.id+"/approve", `{"id":"`+id+`","decision":"allow_once"}`)
	if res.Code != 404 {
		t.Fatal("duplicate approval accepted")
	}
	// Pending approvals are rejected when stopping the conversation.
	chat.mu.Lock()
	chat.approvals["pending"] = ui.PermissionRequest{Response: response}
	chat.mu.Unlock()
	res = webRequest(handler, "POST", "/api/chats/"+chat.id+"/stop", "{}")
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	select {
	case got := <-response:
		if got != ui.PermissionDeny {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("stop left approval blocked")
	}
}

func TestWebBackgroundAgentTriggersSameSessionFollowup(t *testing.T) {
	_, chat, _ := webTestServer(t, &webScriptClient{})
	manager := chat.app.toolRegistry.TaskManager()
	task, err := manager.SpawnLocalAgentForSession(chat.app.ctx, chat.id, "work", "general-agent", "background check", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartExecution(chat.app.ctx, task.ID, func(context.Context, *tasks.LocalAgentTaskState) (interface{}, error) { return "finished result", nil }); err != nil {
		t.Fatal(err)
	}
	awaitWeb(t, chat, func() bool {
		for _, event := range chat.events {
			if event.Kind == "result" && !chat.busy {
				return true
			}
		}
		return false
	})
	chat.mu.Lock()
	defer chat.mu.Unlock()
	notices := 0
	for _, event := range chat.events {
		if event.Kind == "notice" {
			notices++
		}
		if event.Kind == "result" {
			result := event.Data.(query.ResultMessage)
			if result.IsError || result.SessionID != chat.id {
				t.Fatalf("incorrect follow-up: %+v", result)
			}
		}
	}
	if notices != 1 {
		t.Fatalf("expected one automatic turn, got %d", notices)
	}
}

func TestWebAgentDetailShowsExecutionAndScopesItToTheChat(t *testing.T) {
	_, chat, handler := webTestServer(t, &webScriptClient{})
	manager := chat.app.toolRegistry.TaskManager()
	task, err := manager.SpawnLocalAgentForSession(chat.app.ctx, chat.id, "Inspect the README", "general-agent", "Read docs", true)
	if err != nil {
		t.Fatal(err)
	}
	log := &webAgentLog{}
	log.record(query.SDKMessage{Type: "assistant", Message: &api.MessageResponse{Content: []api.ContentBlock{
		{Type: "thinking", Thinking: "private reasoning"},
		{Type: "text", Text: "Reading the documentation"},
		{Type: "tool_use", Name: "Read", Input: json.RawMessage(`{"file_path":"README.MD"}`)},
	}}})
	log.record(query.SDKMessage{Type: "tool_result", Message: map[string]interface{}{
		"tool_name": "Read", "content": "documentation output",
	}})
	chat.app.webAgentLogs.Store(task.ID, log)
	path := "/api/chats/" + chat.id + "/agent?task="
	response := webRequest(handler, "GET", path+task.ID, "")
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var body struct {
		Task    tasks.LocalAgentTaskState
		Entries []webAgentEntry
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Task.Prompt != "Inspect the README" || len(body.Entries) != 3 ||
		body.Entries[1].Kind != "tool_call" || body.Entries[2].Text != "documentation output" {
		t.Fatalf("unexpected detail: %+v", body)
	}
	if strings.Contains(response.Body.String(), "private reasoning") {
		t.Fatal("detail exposed internal reasoning")
	}

	foreign, err := manager.SpawnLocalAgentForSession(chat.app.ctx, "other-chat", "private prompt", "general-agent", "Other chat", true)
	if err != nil {
		t.Fatal(err)
	}
	if response := webRequest(handler, "GET", path+foreign.ID, ""); response.Code != http.StatusNotFound {
		t.Fatalf("foreign task status: %d", response.Code)
	}
}
