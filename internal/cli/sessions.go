package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"claude-code-go/internal/memory"
	"claude-code-go/internal/query"
	"claude-code-go/internal/sessions"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

// ListSessions is usable without initializing API clients or application state.
func ListSessions(cwd string) (string, error) {
	store, err := sessions.NewStore(cwd)
	if err != nil {
		return "", err
	}
	return formatSessions(store)
}

func formatSessions(store *sessions.Store) (string, error) {
	list, err := store.List()
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "No saved sessions for this project.", nil
	}
	var b strings.Builder
	b.WriteString("Saved sessions (newest first):\n")
	for _, s := range list {
		fmt.Fprintf(&b, "%s  %s  %d messages  %s\n", s.ID, s.UpdatedAt.Local().Format("2006-01-02 15:04"), len(s.History), s.Title)
	}
	b.WriteString("Resume with --resume <id> or /resume <id>; --continue resumes the latest session.")
	return b.String(), nil
}

func (a *App) initializeSession() error {
	store, err := sessions.NewStore(a.config.Cwd)
	if err != nil {
		return err
	}
	a.sessionStore = store
	if a.config.Resume != "" && a.config.Continue {
		return fmt.Errorf("--resume and --continue cannot be combined")
	}
	id := a.config.Resume
	if a.config.Continue {
		list, err := store.List()
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return fmt.Errorf("no saved sessions for this project")
		}
		id = list[0].ID
	}
	if id != "" {
		a.session, err = store.Load(id)
		if err != nil {
			return err
		}
		prepareSession(a.session)
	} else {
		a.session = store.New(a.config.Model, a.config.Provider)
	}
	return nil
}

func (a *App) persistSession(messages, history []types.Message) error {
	a.session.Messages = messages
	a.session.History = history
	a.session.Model = a.config.Model
	a.session.Provider = a.config.Provider
	return a.sessionStore.Save(a.session)
}

// Repair interrupted tool calls without re-executing their potentially destructive effects.
func repairPendingTools(messages []types.Message) []types.Message {
	pending := make(map[string]bool)
	var order []string
	for _, m := range messages {
		var blocks []api.ContentBlock
		if json.Unmarshal(m.Content, &blocks) != nil {
			continue
		}
		for _, block := range blocks {
			if block.Type == "tool_use" {
				pending[block.ID] = true
				order = append(order, block.ID)
			}
			if block.Type == "tool_result" {
				delete(pending, block.ToolUseID)
			}
		}
	}
	var results []api.ContentBlock
	for _, id := range order {
		if !pending[id] {
			continue
		}
		results = append(results, api.ContentBlock{Type: "tool_result", ToolUseID: id, IsError: true, Content: "Session interrupted before the tool result was saved. Execution status is unknown; inspect current state before retrying."})
	}
	if len(results) > 0 {
		data, _ := json.Marshal(results)
		messages = append(append([]types.Message(nil), messages...), types.Message{Role: "user", Content: data})
	}
	return messages
}

func (a *App) sessionMemory(id string) *memory.SessionMemory {
	if !memory.SessionMemoryEnabled() || a.config.PrintMode {
		return nil
	}
	m, err := memory.NewSessionMemory(a.config.Cwd, id, memory.DefaultSessionMemoryConfig)
	if err != nil && a.config.Debug {
		fmt.Printf("Warning: failed to initialize session memory: %v\n", err)
	}
	return m
}

func (a *App) executeSessionCommand(ctx context.Context, name, args string) (<-chan interface{}, error) {
	if name == "sessions" || strings.TrimSpace(args) == "" {
		text, err := formatSessions(a.sessionStore)
		if err != nil {
			return nil, err
		}
		return sessionOutput(text), nil
	}
	saved, err := a.sessionStore.Load(strings.TrimSpace(args))
	if err != nil {
		return nil, err
	}
	// Wait before switching so a summary update cannot target the newly selected session.
	a.queryEngine.WaitForSessionMemory(ctx)
	config := a.queryConfig
	config.SessionID = saved.ID
	config.UserSpecifiedModel = a.config.Model
	prepareSession(saved)
	config.InitialMessages = saved.Messages
	config.InitialHistory = saved.History
	config.SessionMemory = a.sessionMemory(saved.ID)
	a.session = saved
	a.queryEngine = query.NewQueryEngine(config)
	output := make(chan interface{}, 1)
	output <- query.SDKMessage{Type: "session_restored", Message: saved.History, SessionID: saved.ID}
	close(output)
	return output, nil
}

func sessionOutput(text string) <-chan interface{} {
	output := make(chan interface{}, 1)
	output <- query.SDKMessage{Type: "system", Message: map[string]interface{}{"subtype": "message", "content": text}}
	close(output)
	return output
}

func prepareSession(saved *sessions.Session) {
	messages := repairPendingTools(saved.Messages)
	if len(messages) > len(saved.Messages) {
		saved.History = append(saved.History, messages[len(saved.Messages):]...)
		saved.Messages = messages
	}
}
