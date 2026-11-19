package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"claude-code-go/internal/query"
	"claude-code-go/internal/tasks"
	"claude-code-go/internal/types"
	"claude-code-go/internal/ui"
	"claude-code-go/internal/webassets"
)

// Each browser conversation owns an App and serializes query/follow-up turns.
// The HTTP transport only handles snapshots, events and approval responses.
type webChat struct {
	app          *App
	id           string
	history      []types.Message
	mu           sync.Mutex
	busy         bool
	stopped      bool
	cancel       context.CancelFunc
	events       []webEvent
	changed      chan struct{}
	messages     chan string
	approvals    map[string]ui.PermissionRequest
	nextApproval int
}
type webEvent struct {
	ID   int         `json:"id"`
	Kind string      `json:"kind"`
	Data interface{} `json:"data"`
}
type webServer struct {
	root  *App
	mu    sync.Mutex
	chats map[string]*webChat
}

func (a *App) runWebMode() error {
	host, _, err := net.SplitHostPort(a.config.WebAddr)
	if err != nil {
		return fmt.Errorf("invalid --web-addr: %w", err)
	}
	if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return fmt.Errorf("--web-addr must bind to a loopback address")
	}
	listener, err := net.Listen("tcp", a.config.WebAddr)
	if err != nil {
		return err
	}
	s := &webServer{root: a, chats: make(map[string]*webChat)}
	if _, err := s.add(a); err != nil {
		listener.Close()
		return err
	}
	server := &http.Server{Handler: s.handler(listener.Addr().String()), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-a.ctx.Done()
		s.mu.Lock()
		for _, chat := range s.chats {
			chat.app.cancel()
			for id, task := range chat.app.toolRegistry.TaskManager().GetAllTasks() {
				if status := task.GetBase().Status; status == tasks.TaskStatusRunning || status == tasks.TaskStatusPending {
					_ = chat.app.toolRegistry.TaskManager().KillTask(id)
				}
			}
		}
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	fmt.Printf("Web chat: http://%s\nProject: %s\n", listener.Addr(), a.config.Cwd)
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *webServer) add(app *App) (*webChat, error) {
	app.permissionUI = true
	if err := app.sessionStore.Save(app.session); err != nil {
		return nil, err
	}
	if app.session.Title == "Untitled session" {
		app.session.Title = ""
	}
	chat := &webChat{app: app, id: app.queryEngine.GetSessionID(), history: append([]types.Message(nil), app.session.History...), changed: make(chan struct{}), messages: make(chan string, 1), approvals: make(map[string]ui.PermissionRequest)}
	s.chats[chat.id] = chat
	go chat.run()
	go chat.permissions()
	return chat, nil
}
func (c *webChat) emit(kind string, data interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emitLocked(kind, data)
}

func (c *webChat) emitLocked(kind string, data interface{}) {
	c.events = append(c.events, webEvent{ID: len(c.events) + 1, Kind: kind, Data: data})
	close(c.changed)
	c.changed = make(chan struct{})
}
func (c *webChat) run() {
	var followups tasks.AgentFollowups
	manager := c.app.toolRegistry.TaskManager()
	for {
		var prompt string
		auto := false
		select {
		case <-c.app.ctx.Done():
			return
		case prompt = <-c.messages:
		case <-manager.AgentFinished():
			c.mu.Lock()
			// A user submission may already own the next turn.
			if c.busy {
				c.mu.Unlock()
				continue
			}
			batch := followups.Next(manager, c.id)
			if batch.Prompt == "" {
				c.mu.Unlock()
				continue
			}
			c.busy = true
			c.mu.Unlock()
			prompt, auto = batch.Prompt, true
			followups.Acknowledge(batch)
		}
		for {
			ctx, cancel := context.WithCancel(c.app.ctx)
			c.mu.Lock()
			c.cancel = cancel
			if c.stopped {
				cancel()
			}
			c.mu.Unlock()
			c.emit("busy", true)
			var stream <-chan interface{}
			var err error
			if auto {
				c.emit("notice", "子 Agent 已结束，正在汇总结果")
				stream, err = c.app.queryEngine.SubmitAgentFollowup(ctx, prompt)
			} else {
				stream, err = c.app.queryEngine.SubmitMessage(ctx, prompt)
			}
			if err != nil {
				c.emit("error", err.Error())
			} else {
				for event := range stream {
					switch message := event.(type) {
					case query.SDKMessage:
						c.emit("sdk", message)
					case query.ResultMessage:
						c.emit("result", message)
					}
				}
			}
			stopped := ctx.Err() != nil
			cancel()
			c.mu.Lock()
			c.cancel = nil
			batch := followups.Next(manager, c.id)
			if stopped || batch.Prompt == "" {
				c.busy = false
				c.emitLocked("busy", false)
				c.mu.Unlock()
				break
			}
			c.mu.Unlock()
			followups.Acknowledge(batch)
			prompt, auto = batch.Prompt, true
		}
	}
}
func (c *webChat) permissions() {
	for {
		select {
		case <-c.app.ctx.Done():
			return
		case request := <-c.app.permissionRequests:
			c.mu.Lock()
			if c.stopped {
				c.mu.Unlock()
				request.Response <- ui.PermissionDeny
				continue
			}
			c.nextApproval++
			id := strconv.Itoa(c.nextApproval)
			c.approvals[id] = request
			c.mu.Unlock()
			c.emit("approval", map[string]interface{}{"id": id, "tool": request.ToolName, "input": request.Input, "risk": request.RiskReason})
		}
	}
}
func writeWebJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func webError(w http.ResponseWriter, status int, message string) {
	writeWebJSON(w, status, map[string]string{"error": message})
}
func decodeWebJSON(w http.ResponseWriter, r *http.Request, target interface{}) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("Content-Type must be application/json")
	}
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(target)
}
func (s *webServer) handler(address string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		writeWebJSON(w, 200, map[string]string{"project": s.root.config.Cwd, "model": s.root.config.Model, "provider": s.root.config.Provider, "initial_chat": s.root.queryEngine.GetSessionID()})
	})
	mux.HandleFunc("GET /api/chats", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.root.sessionStore.List()
		if err != nil {
			webError(w, 500, err.Error())
			return
		}
		// Only metadata crosses the wire here, never full session model context.
		var items []map[string]interface{}
		for _, session := range list {
			items = append(items, map[string]interface{}{"id": session.ID, "title": session.Title, "updated_at": session.UpdatedAt, "model": session.Model})
		}
		writeWebJSON(w, 200, items)
	})
	mux.HandleFunc("POST /api/chats", func(w http.ResponseWriter, r *http.Request) {
		var input struct{}
		if err := decodeWebJSON(w, r, &input); err != nil {
			webError(w, 400, err.Error())
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		config := s.root.webConfig()
		config.Resume, config.Continue = "", false
		app := NewApp(&config, s.root.version)
		if err := app.Initialize(); err != nil {
			app.cancel()
			webError(w, 500, err.Error())
			return
		}
		chat, err := s.add(app)
		if err != nil {
			app.cancel()
			webError(w, 500, err.Error())
			return
		}
		writeWebJSON(w, 201, map[string]string{"id": chat.id})
	})
	mux.HandleFunc("GET /api/chats/{id}/{action}", s.chatAction)
	mux.HandleFunc("POST /api/chats/{id}/{action}", s.chatAction)
	mux.Handle("GET /", http.FileServer(http.FS(webassets.Files)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This local agent can execute tools. Reject cross-site requests and
		// unexpected Host values, including DNS-rebinding requests.
		host, port, _ := net.SplitHostPort(r.Host)
		_, expectedPort, _ := net.SplitHostPort(address)
		if (host != "localhost" && host != "127.0.0.1" && host != "::1") || port != expectedPort {
			webError(w, 403, "unexpected host")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || u.Scheme != "http" {
				webError(w, 403, "cross-origin request rejected")
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			webError(w, 403, "cross-site request rejected")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}
func (s *webServer) chatAction(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	chat := s.chats[r.PathValue("id")]
	if chat == nil {
		config := s.root.webConfig()
		config.Resume, config.Continue = r.PathValue("id"), false
		app := NewApp(&config, s.root.version)
		if err := app.Initialize(); err != nil {
			app.cancel()
			s.mu.Unlock()
			webError(w, 404, "session not found")
			return
		}
		var err error
		chat, err = s.add(app)
		if err != nil {
			app.cancel()
			s.mu.Unlock()
			webError(w, 500, err.Error())
			return
		}
	}
	s.mu.Unlock()
	action := r.PathValue("action")
	if r.Method == "GET" {
		switch action {
		case "state":
			chat.mu.Lock()
			pending := make([]map[string]interface{}, 0)
			for id, approval := range chat.approvals {
				pending = append(pending, map[string]interface{}{"id": id, "tool": approval.ToolName, "input": approval.Input, "risk": approval.RiskReason})
			}
			data := map[string]interface{}{"history": chat.history, "busy": chat.busy, "approvals": pending}
			chat.mu.Unlock()
			data["tasks"] = chat.app.toolRegistry.TaskManager().GetAllTasks()
			writeWebJSON(w, 200, data)
		case "events":
			chat.stream(w, r)
		case "agent":
			chat.agentDetail(w, r.URL.Query().Get("task"))
		default:
			webError(w, 404, "unknown action")
		}
		return
	}
	if r.Method != "POST" {
		webError(w, 405, "method not allowed")
		return
	}
	var input struct {
		Message  string                `json:"message"`
		ID       string                `json:"id"`
		Decision ui.PermissionResponse `json:"decision"`
	}
	if err := decodeWebJSON(w, r, &input); err != nil {
		webError(w, 400, err.Error())
		return
	}
	switch action {
	case "messages":
		prompt := strings.TrimSpace(input.Message)
		if prompt == "" {
			webError(w, 400, "message is empty")
			return
		}
		chat.mu.Lock()
		if chat.busy {
			chat.mu.Unlock()
			webError(w, 409, "conversation is already running")
			return
		}
		chat.busy = true
		chat.stopped = false
		chat.messages <- prompt
		chat.mu.Unlock()
		writeWebJSON(w, 202, map[string]bool{"ok": true})
	case "stop":
		chat.mu.Lock()
		chat.stopped = true
		if chat.cancel != nil {
			chat.cancel()
		}
		for id, approval := range chat.approvals {
			approval.Response <- ui.PermissionDeny
			delete(chat.approvals, id)
		}
		chat.mu.Unlock()
		for id, task := range chat.app.toolRegistry.TaskManager().GetAllTasks() {
			if status := task.GetBase().Status; status == tasks.TaskStatusRunning || status == tasks.TaskStatusPending {
				_ = chat.app.toolRegistry.TaskManager().KillTask(id)
			}
		}
		writeWebJSON(w, 200, map[string]bool{"ok": true})
	case "approve":
		if input.Decision != ui.PermissionAllowOnce && input.Decision != ui.PermissionAllowSession && input.Decision != ui.PermissionDeny {
			webError(w, 400, "invalid approval")
			return
		}
		chat.mu.Lock()
		approval, ok := chat.approvals[input.ID]
		if ok {
			delete(chat.approvals, input.ID)
			approval.Response <- input.Decision
		}
		chat.mu.Unlock()
		if !ok {
			webError(w, 404, "approval no longer pending")
			return
		}
		chat.emit("approved", input.ID)
		writeWebJSON(w, 200, map[string]bool{"ok": true})
	default:
		webError(w, 404, "unknown action")
	}
}
func (c *webChat) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		webError(w, 500, "streaming unavailable")
		return
	}
	cursor, _ := strconv.Atoi(r.Header.Get("Last-Event-ID"))
	if cursor < 0 {
		cursor = 0
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		c.mu.Lock()
		if cursor > len(c.events) {
			cursor = 0
		}
		batch := append([]webEvent(nil), c.events[cursor:]...)
		changed := c.changed
		c.mu.Unlock()
		for _, event := range batch {
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.ID, data); err != nil {
				return
			}
			cursor = event.ID
		}
		if len(batch) == 0 {
			if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
				return
			}
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-c.app.ctx.Done():
			return
		case <-changed:
		case <-heartbeat.C:
		}
	}
}

func (a *App) webConfig() Config {
	a.permissionMu.RLock()
	defer a.permissionMu.RUnlock()
	return *a.config
}
