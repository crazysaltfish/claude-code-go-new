package components

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"claude-code-go/internal/types"
)

// =============================================================================
// Chat Interface Component
// =============================================================================

// Styles
var (
	chatContainerStyle = lipgloss.NewStyle().
				Padding(1, 2)

	chatHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true).
			Padding(0, 1)

	chatFooterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62"))

	approvalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("214")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1)
)

const chatHorizontalFrame = 4

func terminalContentWidth(width int) int {
	width -= chatHorizontalFrame
	if width < 1 {
		return 1
	}
	return width
}

// ChatState represents the state of the chat interface.
type ChatState int

const (
	ChatStateIdle ChatState = iota
	ChatStateProcessing
	ChatStateWaitingForInput
	ChatStateError
)

// ChatModel represents the main chat interface.
type ChatModel struct {
	Messages       *MessageListModel
	Input          *InputModel
	Status         *StatusBarModel
	Spinner        *SpinnerModel
	State          ChatState
	Width          int
	Height         int
	Error          error
	HelpVisible    bool
	ApprovalText   string
	ApprovalOffset int
	DiffView       *DiffViewModel
	runStartIndex  int
}

// NewChatModel creates a new chat interface.
func NewChatModel(width, height int) *ChatModel {
	contentWidth := terminalContentWidth(width)
	return &ChatModel{
		Messages:    NewMessageList(contentWidth, height-6),
		Input:       NewInput(">", "Type your message...", contentWidth),
		Status:      NewStatusBar(contentWidth),
		Spinner:     NewSpinner(),
		State:       ChatStateIdle,
		Width:       width,
		Height:      height,
		HelpVisible: false,
	}
}

// Init initializes the chat interface.
func (m *ChatModel) Init() tea.Cmd {
	return nil
}

// Update handles chat interface updates.
func (m *ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if m.DiffView != nil {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.Width, m.Height = msg.Width, msg.Height
			m.DiffView.Width, m.DiffView.Height = msg.Width, msg.Height
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyCtrlC:
				return m, tea.Quit
			case tea.KeyEsc:
				m.CloseDiffView()
			case tea.KeyLeft:
				m.DiffView.Previous()
			case tea.KeyRight:
				m.DiffView.Next()
			case tea.KeyUp:
				m.DiffView.Scroll(-1)
			case tea.KeyDown:
				m.DiffView.Scroll(1)
			case tea.KeyPgUp:
				m.DiffView.Page(-1)
			case tea.KeyPgDown:
				m.DiffView.Page(1)
			default:
				if strings.EqualFold(msg.String(), "q") {
					m.CloseDiffView()
				}
			}
		case tea.MouseMsg:
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.DiffView.Scroll(-3)
			case tea.MouseButtonWheelDown:
				m.DiffView.Scroll(3)
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		contentWidth := terminalContentWidth(msg.Width)
		m.Messages.Width = contentWidth
		m.updateMessageViewport()
		m.Input.Width = contentWidth
		m.Status.Width = contentWidth

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyCtrlH:
			m.HelpVisible = !m.HelpVisible
		case tea.KeyCtrlO:
			m.Messages.CycleTranscriptMode()
		case tea.KeyPgUp:
			m.Messages.PageUp()
		case tea.KeyPgDown:
			m.Messages.PageDown()
		case tea.KeyCtrlUp:
			m.Messages.ScrollUp()
		case tea.KeyCtrlDown:
			m.Messages.ScrollDown()
		case tea.KeyEnter:
			if m.State == ChatStateIdle && m.Input.Value != "" {
				// Add user message
				m.runStartIndex = len(m.Messages.Messages)
				m.Messages.AddMessage(MessageModel{
					Role:    "user",
					Content: []ContentBlock{{Type: "text", Text: m.Input.Value}},
				})
				m.Input.Clear()
				m.State = ChatStateProcessing
			}
		default:
			if m.State == ChatStateIdle {
				m.Input.Update(msg)
			}
		}
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			for i := 0; i < 3; i++ {
				m.Messages.ScrollUp()
			}
		case tea.MouseButtonWheelDown:
			for i := 0; i < 3; i++ {
				m.Messages.ScrollDown()
			}
		}
	}

	// Update spinner if processing
	if m.State == ChatStateProcessing {
		m.Spinner.Update()
	}

	return m, tea.Batch(cmds...)
}

// View renders the chat interface.
func (m *ChatModel) View() string {
	if m.DiffView != nil {
		return m.DiffView.View()
	}
	var b strings.Builder
	contentWidth := terminalContentWidth(m.Width)

	// Header
	headerWidth := contentWidth - chatHeaderStyle.GetHorizontalFrameSize()
	if headerWidth < 1 {
		headerWidth = 1
	}
	header := runewidth.Truncate("Claude Code · "+m.Messages.Mode.String(), headerWidth, "")
	b.WriteString(chatHeaderStyle.Render(header) + "\n")
	b.WriteString(dividerStyle.Render(strings.Repeat("─", contentWidth)) + "\n")

	// Messages area
	b.WriteString(m.Messages.View())

	// Status bar or processing indicator
	if m.State == ChatStateProcessing {
		status := "Thinking..."
		if m.Messages.HasStreamingAssistant() {
			status = "Responding..."
		}
		b.WriteString(m.Spinner.ViewWithText(status) + "\n")
	} else if m.State == ChatStateError {
		b.WriteString(errorStyle.Render(m.Error.Error()) + "\n")
	}
	if m.ApprovalText != "" {
		approvalWidth := contentWidth - approvalStyle.GetHorizontalFrameSize()
		if approvalWidth < 1 {
			approvalWidth = 1
		}
		b.WriteString(approvalStyle.Width(approvalWidth).Render(m.approvalView()) + "\n")
	}

	// Input area
	b.WriteString(dividerStyle.Render(strings.Repeat("─", contentWidth)) + "\n")
	b.WriteString(m.Input.View() + "\n")

	// Footer with help
	footerText := "Enter: send | Ctrl+O: transcript view | /diff: changes | Ctrl+C: quit"
	if m.ApprovalText != "" {
		footerText = "↑↓: inspect call | y: allow once | n/Esc: deny | Ctrl+C: quit"
	} else if m.Messages.IsScrolled() {
		footerText = fmt.Sprintf(
			"History: %d lines above latest | PgUp/PgDn or mouse wheel | Enter: send",
			m.Messages.ScrollOffset,
		)
	} else if m.HelpVisible {
		footerText = "↑↓: input history | Ctrl+O: Normal/Verbose/Summary | /diff: changes | Ctrl+H: hide help"
	}
	b.WriteString(chatFooterStyle.Render(wrapText(footerText, contentWidth)))

	return chatContainerStyle.Render(b.String())
}

// OpenDiffView opens the dedicated session diff browser.
func (m *ChatModel) OpenDiffView() {
	m.DiffView = NewDiffView(m.Messages.DiffEntries(), m.Width, m.Height)
}

// CloseDiffView returns to the conversation transcript.
func (m *ChatModel) CloseDiffView() {
	m.DiffView = nil
}

// AddAssistantMessage adds an assistant message.
func (m *ChatModel) AddAssistantMessage(content string) {
	m.Messages.AddMessage(MessageModel{
		Role:    "assistant",
		Content: []ContentBlock{{Type: "text", Text: content}},
	})
}

// AppendAssistantDelta updates the current assistant response in place.
func (m *ChatModel) AppendAssistantDelta(content string) {
	m.Messages.AppendAssistantDelta(content)
}

// FinalizeAssistantMessage completes the current streamed response.
func (m *ChatModel) FinalizeAssistantMessage(content string) {
	m.Messages.FinalizeAssistantStream(content)
}

// AbortAssistantMessage retains partial output but removes its stream cursor.
func (m *ChatModel) AbortAssistantMessage() {
	m.Messages.AbortAssistantStream()
}

// AddUserMessage adds a user message.
func (m *ChatModel) AddUserMessage(content string) {
	m.runStartIndex = len(m.Messages.Messages)
	m.Messages.AddMessage(MessageModel{
		Role:    "user",
		Content: []ContentBlock{{Type: "text", Text: content}},
	})
}

// AddSystemMessage adds a system message.
func (m *ChatModel) AddSystemMessage(content string) {
	m.Messages.AddMessage(MessageModel{
		Role:    "system",
		Content: []ContentBlock{{Type: "text", Text: content}},
	})
}

// AddCompletionSummary presents the run outcome and any produced artifacts.
func (m *ChatModel) AddCompletionSummary(duration, cost string, turns int) {
	content := fmt.Sprintf("Completed in %s · Cost %s · %d turns", duration, cost, turns)
	if artifacts := m.Messages.ArtifactPathsFrom(m.runStartIndex); len(artifacts) > 0 {
		content += fmt.Sprintf("\nChanged files (%d):", len(artifacts))
		for _, path := range artifacts {
			content += "\n  • " + path
		}
	}
	m.Messages.AddMessage(MessageModel{
		Role:    "summary",
		Content: []ContentBlock{{Type: "text", Text: content}},
	})
}

// UpdateToolProgress creates or refreshes a transient tool progress row.
func (m *ChatModel) UpdateToolProgress(toolName, toolUseID, content string) {
	m.Messages.UpsertToolProgress(toolName, toolUseID, content)
}

// AddToolUse creates a collapsed tool card before execution starts.
func (m *ChatModel) AddToolUse(toolName, toolUseID string, input json.RawMessage) {
	m.Messages.UpsertToolUse(toolName, toolUseID, input)
}

// AddToolResult completes the existing tool card while preserving its input.
func (m *ChatModel) AddToolResult(toolName, toolUseID, content string, isError, truncated bool, originalChars int, display *types.ToolDisplay) {
	m.Messages.CompleteToolUse(toolName, toolUseID, content, isError, truncated, originalChars, display)
}

// SetApproval displays a tool approval panel.
func (m *ChatModel) SetApproval(content string) {
	m.ApprovalText = content
	m.ApprovalOffset = 0
	m.updateMessageViewport()
}

// ClearApproval removes the current tool approval panel.
func (m *ChatModel) ClearApproval() {
	m.ApprovalText = ""
	m.ApprovalOffset = 0
	m.updateMessageViewport()
}

// ScrollApprovalUp scrolls the complete tool call toward its beginning.
func (m *ChatModel) ScrollApprovalUp() {
	if m.ApprovalOffset > 0 {
		m.ApprovalOffset--
	}
}

// ScrollApprovalDown scrolls the complete tool call toward its end.
func (m *ChatModel) ScrollApprovalDown() {
	lines := m.approvalLines()
	maxOffset := len(lines) - m.approvalViewportHeight()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.ApprovalOffset < maxOffset {
		m.ApprovalOffset++
	}
}

func (m *ChatModel) approvalView() string {
	lines := m.approvalLines()
	viewportHeight := m.approvalViewportHeight()
	maxOffset := len(lines) - viewportHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.ApprovalOffset > maxOffset {
		m.ApprovalOffset = maxOffset
	}

	end := m.ApprovalOffset + viewportHeight
	if end > len(lines) {
		end = len(lines)
	}
	visible := strings.Join(lines[m.ApprovalOffset:end], "\n")
	if len(lines) > viewportHeight {
		visible += fmt.Sprintf(
			"\n[%d-%d of %d lines]",
			m.ApprovalOffset+1,
			end,
			len(lines),
		)
	}
	return visible + "\n\n[y] Allow once    [n/Esc] Deny"
}

func (m *ChatModel) approvalLines() []string {
	if m.ApprovalText == "" {
		return nil
	}
	width := terminalContentWidth(m.Width) - approvalStyle.GetHorizontalFrameSize()
	if width < 1 {
		width = 1
	}
	return strings.Split(wrapText(m.ApprovalText, width), "\n")
}

func (m *ChatModel) approvalViewportHeight() int {
	height := m.Height / 3
	if height < 6 {
		height = 6
	}
	if height > 14 {
		height = 14
	}
	return height
}

func (m *ChatModel) updateMessageViewport() {
	height := m.Height - 10
	if m.ApprovalText != "" {
		height -= m.approvalViewportHeight() + 5
	}
	if height < 3 {
		height = 3
	}
	m.Messages.Height = height
	m.Messages.MaxVisible = height
}

// SetError sets an error state.
func (m *ChatModel) SetError(err error) {
	m.AbortAssistantMessage()
	m.Error = err
	m.State = ChatStateError
}

// ClearError clears any error.
func (m *ChatModel) ClearError() {
	m.Error = nil
	m.State = ChatStateIdle
}

// =============================================================================
// Conversation History
// =============================================================================

// ConversationHistory represents the conversation history.
type ConversationHistory struct {
	Messages []MessageModel
	Current  int
}

// NewConversationHistory creates a new conversation history.
func NewConversationHistory() *ConversationHistory {
	return &ConversationHistory{
		Messages: []MessageModel{},
		Current:  -1,
	}
}

// Add adds a message to the history.
func (h *ConversationHistory) Add(msg MessageModel) {
	h.Messages = append(h.Messages, msg)
	h.Current = len(h.Messages) - 1
}

// Previous returns the previous message.
func (h *ConversationHistory) Previous() *MessageModel {
	if h.Current > 0 {
		h.Current--
		return &h.Messages[h.Current]
	}
	return nil
}

// Next returns the next message.
func (h *ConversationHistory) Next() *MessageModel {
	if h.Current < len(h.Messages)-1 {
		h.Current++
		return &h.Messages[h.Current]
	}
	return nil
}

// Last returns the last message.
func (h *ConversationHistory) Last() *MessageModel {
	if len(h.Messages) > 0 {
		return &h.Messages[len(h.Messages)-1]
	}
	return nil
}

// =============================================================================
// Streaming Response Handler
// =============================================================================

// StreamingResponse represents a streaming response.
type StreamingResponse struct {
	Content  strings.Builder
	Done     bool
	Error    error
	OnUpdate func(string)
}

// NewStreamingResponse creates a new streaming response.
func NewStreamingResponse() *StreamingResponse {
	return &StreamingResponse{
		Content: strings.Builder{},
		Done:    false,
	}
}

// Append appends content to the response.
func (r *StreamingResponse) Append(content string) {
	r.Content.WriteString(content)
	if r.OnUpdate != nil {
		r.OnUpdate(r.Content.String())
	}
}

// Complete marks the response as complete.
func (r *StreamingResponse) Complete() {
	r.Done = true
}

// Fail marks the response as failed.
func (r *StreamingResponse) Fail(err error) {
	r.Error = err
	r.Done = true
}

// String returns the current content.
func (r *StreamingResponse) String() string {
	return r.Content.String()
}
