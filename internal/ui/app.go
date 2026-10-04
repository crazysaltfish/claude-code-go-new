package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"claude-code-go/internal/query"
	"claude-code-go/internal/types"
	"claude-code-go/internal/ui/components"
	"claude-code-go/pkg/api"
)

// SubmitFunc submits a prompt and returns the query event stream.
type SubmitFunc func(context.Context, string) (<-chan interface{}, error)

// PermissionRequest asks the UI to approve a tool call.
type PermissionRequest struct {
	ToolName    string
	Input       json.RawMessage
	ReadOnly    bool
	Destructive bool
	RiskReason  string
	Response    chan PermissionResponse
}

// PermissionResponse is the scope selected in an interactive approval.
type PermissionResponse string

const (
	PermissionAllowOnce    PermissionResponse = "allow_once"
	PermissionAllowSession PermissionResponse = "allow_session"
	PermissionDeny         PermissionResponse = "deny"
)

// AppModel integrates prompt dispatch and QueryEngine events with Bubble Tea.
type AppModel struct {
	chat               *components.ChatModel
	ctx                context.Context
	cancel             context.CancelFunc
	messageChan        <-chan interface{}
	submit             SubmitFunc
	initialPrompt      string
	permissionRequests <-chan PermissionRequest
	pendingPermission  *PermissionRequest
	permissionMode     types.PermissionMode
	setPermissionMode  func(types.PermissionMode) error
}

// NewAppModel creates a UI model backed directly by a QueryEngine.
func NewAppModel(engine *query.QueryEngine, width, height int) *AppModel {
	return NewAppModelWithSubmit(engine.SubmitMessage, width, height)
}

// NewAppModelWithSubmit creates a UI model with a custom prompt dispatcher.
func NewAppModelWithSubmit(submit SubmitFunc, width, height int) *AppModel {
	return NewAppModelWithContext(context.Background(), submit, width, height)
}

// NewAppModelWithContext creates a UI model whose work is cancelled with parent.
func NewAppModelWithContext(parent context.Context, submit SubmitFunc, width, height int) *AppModel {
	ctx, cancel := context.WithCancel(parent)
	return &AppModel{
		chat:           components.NewChatModel(width, height),
		ctx:            ctx,
		cancel:         cancel,
		submit:         submit,
		permissionMode: types.PermissionModeDefault,
	}
}

// SetPermissionMode connects the UI indicator and Shift+Tab switcher to the
// runtime permission policy.
func (m *AppModel) SetPermissionMode(mode types.PermissionMode, setter func(types.PermissionMode) error) {
	if mode == "" {
		mode = types.PermissionModeDefault
	}
	m.permissionMode = mode
	m.setPermissionMode = setter
	m.chat.PermissionMode = string(mode)
}

// SetInitialPrompt schedules a prompt after the UI starts.
func (m *AppModel) SetInitialPrompt(prompt string) {
	m.initialPrompt = prompt
}

// SetPermissionRequests connects interactive tool approval requests.
func (m *AppModel) SetPermissionRequests(requests <-chan PermissionRequest) {
	m.permissionRequests = requests
}

// Init initializes the app.
func (m *AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.chat.Init()}
	if m.initialPrompt != "" {
		prompt := m.initialPrompt
		cmds = append(cmds, func() tea.Msg { return submitPromptMsg{Prompt: prompt} })
	}
	if m.permissionRequests != nil {
		cmds = append(cmds, m.waitForPermissionRequest())
	}
	return tea.Batch(cmds...)
}

type submitPromptMsg struct{ Prompt string }
type queryCompleteMsg struct{}
type permissionRequestMsg struct{ Request PermissionRequest }

// Update handles app updates.
func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.chat.Width = msg.Width
		m.chat.Height = msg.Height

	case tea.KeyMsg:
		if m.pendingPermission != nil {
			if msg.Type == tea.KeyCtrlC {
				m.pendingPermission.Response <- PermissionDeny
				m.chat.ClearApproval()
				m.cancel()
				return m, tea.Quit
			}
			if msg.Type == tea.KeyUp {
				m.chat.ScrollApprovalUp()
				return m, nil
			}
			if msg.Type == tea.KeyDown {
				m.chat.ScrollApprovalDown()
				return m, nil
			}
			switch strings.ToLower(msg.String()) {
			case "y":
				m.pendingPermission.Response <- PermissionAllowOnce
				m.chat.AddSystemMessage(fmt.Sprintf("Allowed tool: %s", m.pendingPermission.ToolName))
				m.chat.ClearApproval()
				m.pendingPermission = nil
				cmds = append(cmds, m.waitForPermissionRequest())
			case "a":
				m.pendingPermission.Response <- PermissionAllowSession
				m.permissionMode = types.PermissionModeSession
				m.chat.PermissionMode = string(types.PermissionModeSession)
				m.chat.AddSystemMessage("All tools allowed for this session")
				m.chat.ClearApproval()
				m.pendingPermission = nil
				cmds = append(cmds, m.waitForPermissionRequest())
			case "n", "esc":
				m.pendingPermission.Response <- PermissionDeny
				m.chat.AddSystemMessage(fmt.Sprintf("Denied tool: %s", m.pendingPermission.ToolName))
				m.chat.ClearApproval()
				m.pendingPermission = nil
				cmds = append(cmds, m.waitForPermissionRequest())
			}
			return m, tea.Batch(cmds...)
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			m.cancel()
			return m, tea.Quit
		case tea.KeyShiftTab:
			next := nextPermissionMode(m.permissionMode)
			if m.setPermissionMode != nil {
				if err := m.setPermissionMode(next); err != nil {
					m.chat.SetError(fmt.Errorf("change permission mode: %w", err))
					return m, nil
				}
			}
			m.permissionMode = next
			m.chat.PermissionMode = string(next)
			m.chat.AddSystemMessage("Permission mode: " + string(next))
			return m, nil
		case tea.KeyEnter:
			if m.chat.State == components.ChatStateIdle && m.chat.Input.Value != "" {
				prompt := m.chat.Input.Value
				m.chat.Input.Clear()
				if cmd := m.startQuery(prompt); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

	case submitPromptMsg:
		if cmd := m.startQuery(msg.Prompt); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case QueryEngineMsg:
		switch data := msg.Data.(type) {
		case query.SDKMessage:
			m.handleSDKMessage(data)
		case query.ResultMessage:
			m.handleResultMessage(data)
		}
		cmds = append(cmds, m.waitForMessages())

	case queryCompleteMsg:
		m.chat.AbortAssistantMessage()
		if m.chat.State == components.ChatStateProcessing {
			m.chat.State = components.ChatStateIdle
		}

	case permissionRequestMsg:
		m.pendingPermission = &msg.Request
		m.chat.SetApproval(formatPermissionRequest(msg.Request))
	}

	newChat, cmd := m.chat.Update(msg)
	m.chat = newChat.(*components.ChatModel)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func nextPermissionMode(mode types.PermissionMode) types.PermissionMode {
	switch mode {
	case types.PermissionModeDefault:
		return types.PermissionModeAcceptEdits
	case types.PermissionModeAcceptEdits:
		return types.PermissionModePlan
	default:
		return types.PermissionModeDefault
	}
}

func formatPermissionRequest(request PermissionRequest) string {
	impact := "modifies state"
	if request.Destructive {
		impact = "potentially destructive"
	} else if request.ReadOnly {
		impact = "read-only"
	}

	input := "{}"
	if len(request.Input) > 0 {
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, request.Input, "", "  "); err == nil {
			input = formatted.String()
		} else {
			input = string(request.Input)
		}
	}

	formatted := fmt.Sprintf(
		"Permission required\nTool: %s\nImpact: %s\nRead-only: %t\nDestructive: %t\nInput:\n%s",
		request.ToolName,
		impact,
		request.ReadOnly,
		request.Destructive,
		input,
	)
	if request.RiskReason != "" {
		formatted = fmt.Sprintf("%s\nSecurity note: %s", formatted, request.RiskReason)
	}
	return formatted
}

func (m *AppModel) waitForPermissionRequest() tea.Cmd {
	return func() tea.Msg {
		request, ok := <-m.permissionRequests
		if !ok {
			return nil
		}
		return permissionRequestMsg{Request: request}
	}
}

func (m *AppModel) startQuery(prompt string) tea.Cmd {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "/diff" {
		m.chat.ClearError()
		m.chat.OpenDiffView()
		return nil
	}
	m.chat.AddUserMessage(prompt)
	m.chat.ClearError()
	m.chat.State = components.ChatStateProcessing
	if m.submit == nil {
		m.chat.SetError(fmt.Errorf("query submitter is not configured"))
		return nil
	}

	output, err := m.submit(m.ctx, prompt)
	if err != nil {
		m.chat.SetError(err)
		return nil
	}
	m.messageChan = output
	return m.waitForMessages()
}

// View renders the app.
func (m *AppModel) View() string {
	return m.chat.View()
}

// QueryEngineMsg wraps messages from QueryEngine for Bubble Tea.
type QueryEngineMsg struct{ Data interface{} }

func (m *AppModel) waitForMessages() tea.Cmd {
	return func() tea.Msg {
		if m.messageChan == nil {
			return queryCompleteMsg{}
		}
		data, ok := <-m.messageChan
		if !ok {
			return queryCompleteMsg{}
		}
		return QueryEngineMsg{Data: data}
	}
}

func (m *AppModel) handleSDKMessage(msg query.SDKMessage) {
	switch msg.Type {
	case "assistant_delta":
		switch delta := msg.Message.(type) {
		case query.AssistantDelta:
			m.chat.AppendAssistantDelta(delta.Text)
		case *query.AssistantDelta:
			if delta != nil {
				m.chat.AppendAssistantDelta(delta.Text)
			}
		}

	case "assistant":
		if response, ok := msg.Message.(*api.MessageResponse); ok {
			var textParts []string
			for _, block := range response.Content {
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			}
			m.chat.FinalizeAssistantMessage(strings.Join(textParts, "\n"))
			for _, block := range response.Content {
				if block.Type == "tool_use" {
					m.chat.AddToolUse(block.Name, block.ID, block.Input)
				}
			}
		}

	case "tool_result":
		if data, ok := msg.Message.(map[string]interface{}); ok {
			toolName, _ := data["tool_name"].(string)
			toolUseID, _ := data["tool_use_id"].(string)
			content, _ := data["content"].(string)
			isError, _ := data["is_error"].(bool)
			truncated, _ := data["truncated"].(bool)
			originalChars, _ := data["original_chars"].(int)
			display, _ := data["display"].(*types.ToolDisplay)
			m.chat.AddToolResult(toolName, toolUseID, content, isError, truncated, originalChars, display)
		}

	case "tool_progress":
		if data, ok := msg.Message.(map[string]interface{}); ok {
			toolName, _ := data["tool_name"].(string)
			toolUseID, _ := data["tool_use_id"].(string)
			m.chat.UpdateToolProgress(toolName, toolUseID, formatToolProgress(data["data"]))
		}

	case "system":
		if data, ok := msg.Message.(map[string]interface{}); ok {
			subtype, _ := data["subtype"].(string)
			switch subtype {
			case "interrupted":
				m.chat.AddSystemMessage("Operation interrupted")
			case "error":
				if errMsg, ok := data["error"].(string); ok {
					m.chat.SetError(fmt.Errorf("%s", errMsg))
				}
			case "message":
				if content, ok := data["content"].(string); ok {
					m.chat.AddSystemMessage(content)
				}
			case "permission_mode":
				if mode, ok := data["mode"].(string); ok {
					m.permissionMode = types.PermissionMode(mode)
					m.chat.PermissionMode = mode
				}
				if content, ok := data["content"].(string); ok {
					m.chat.AddSystemMessage(content)
				}
			case "recovery":
				reason, _ := data["reason"].(string)
				attempt, _ := data["attempt"].(int)
				m.chat.AddSystemMessage(fmt.Sprintf("Recovering from %s (attempt %d)", reason, attempt))
			}
		}
	}
}

func formatToolProgress(progress interface{}) string {
	if text, ok := progress.(string); ok {
		return text
	}
	if encoded, err := json.Marshal(progress); err == nil {
		return string(encoded)
	}
	return fmt.Sprintf("%v", progress)
}

func (m *AppModel) handleResultMessage(msg query.ResultMessage) {
	if msg.IsError {
		m.chat.SetError(fmt.Errorf("query failed: %s: %s", msg.Subtype, msg.Result))
		return
	}
	m.chat.State = components.ChatStateIdle
	duration := fmt.Sprintf("%.2fs", float64(msg.DurationMs)/1000.0)
	cost := fmt.Sprintf("$%.6f", msg.TotalCostUsd)
	m.chat.AddCompletionSummary(duration, cost, msg.NumTurns)
}

// RunUI runs the interactive UI.
func RunUI(engine *query.QueryEngine) error {
	p := tea.NewProgram(NewAppModel(engine, 80, 24), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
