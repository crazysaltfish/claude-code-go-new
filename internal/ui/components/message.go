package components

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"claude-code-go/internal/types"
)

// =============================================================================
// Message Components
// =============================================================================

// Styles for message display
var (
	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	assistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("141")).
			Bold(true)

	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)

	toolUseStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	toolResultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("78"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	codeBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	messageBoxStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	streamCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Blink(true)
)

// ContentBlock represents a block of content in a message.
type ContentBlock struct {
	Type string `json:"type"`

	// For text blocks
	Text string `json:"text,omitempty"`

	// For tool_use blocks
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// For tool_result blocks
	ToolUseID     string      `json:"tool_use_id,omitempty"`
	Content       interface{} `json:"content,omitempty"`
	IsError       bool        `json:"is_error,omitempty"`
	Status        string      `json:"status,omitempty"`
	Truncated     bool        `json:"truncated,omitempty"`
	OriginalChars int         `json:"original_chars,omitempty"`
	Summary       string      `json:"summary,omitempty"`
	FilePath      string      `json:"file_path,omitempty"`
	Diff          string      `json:"diff,omitempty"`
	Artifacts     []string    `json:"artifacts,omitempty"`

	// For thinking blocks
	Thinking string `json:"thinking,omitempty"`
}

// MessageModel represents a message for display.
type MessageModel struct {
	Role        string         `json:"role"`
	Content     []ContentBlock `json:"content"`
	Timestamp   string         `json:"timestamp,omitempty"`
	IsStreaming bool           `json:"isStreaming,omitempty"`
}

// TranscriptMode controls how much conversation detail is rendered.
type TranscriptMode int

const (
	TranscriptNormal TranscriptMode = iota
	TranscriptVerbose
	TranscriptSummary
)

func (m TranscriptMode) String() string {
	switch m {
	case TranscriptVerbose:
		return "Verbose"
	case TranscriptSummary:
		return "Summary"
	default:
		return "Normal"
	}
}

func (m TranscriptMode) Next() TranscriptMode {
	return TranscriptMode((int(m) + 1) % 3)
}

// RenderMessage renders a message with styling.
func RenderMessage(msg MessageModel, width int) string {
	return renderMessageWithMode(msg, width, TranscriptNormal)
}

func renderMessageWithMode(msg MessageModel, width int, mode TranscriptMode) string {
	var b strings.Builder

	// Render role prefix
	var prefix string
	switch msg.Role {
	case "user":
		prefix = userStyle.Render("You:")
	case "assistant":
		prefix = assistantStyle.Render("Claude:")
	case "system":
		prefix = systemStyle.Render("System:")
	case "summary":
		prefix = toolResultStyle.Bold(true).Render("Result:")
	case "tool", "tool_progress", "tool_result":
		prefix = toolUseStyle.Render("Tool:")
	default:
		prefix = msg.Role + ":"
	}

	b.WriteString(prefix + "\n")

	// Render content blocks
	for i, block := range msg.Content {
		renderedBlock := renderContentBlock(block, width-4, mode)
		if msg.IsStreaming && i == len(msg.Content)-1 && block.Type == "text" {
			renderedBlock += streamCursorStyle.Render("▌")
		}
		b.WriteString(messageBoxStyle.Render(renderedBlock) + "\n")
	}
	if msg.IsStreaming && len(msg.Content) == 0 {
		b.WriteString(messageBoxStyle.Render(streamCursorStyle.Render("▌")) + "\n")
	}

	// Add timestamp if present
	if msg.Timestamp != "" {
		b.WriteString(timestampStyle.Render(msg.Timestamp) + "\n")
	}

	return b.String()
}

// renderContentBlock renders a single content block.
func renderContentBlock(block ContentBlock, width int, mode TranscriptMode) string {
	switch block.Type {
	case "text":
		return wrapText(block.Text, width)
	case "tool_use":
		return renderToolUse(block, width)
	case "tool_result":
		return renderToolResult(block, width)
	case "tool_progress":
		return renderToolProgress(block, width)
	case "tool":
		return renderToolCard(block, width, mode)
	case "thinking":
		return renderThinking(block, width)
	default:
		return fmt.Sprintf("[%s block]", block.Type)
	}
}

func renderToolCard(block ContentBlock, width int, mode TranscriptMode) string {
	var b strings.Builder
	icon := "↻"
	style := toolUseStyle
	switch block.Status {
	case "completed":
		icon, style = "✓", toolResultStyle
	case "error":
		icon, style = "✗", errorStyle
	}

	summary := block.Summary
	if summary == "" {
		summary = toolCallSummary(block)
	}
	expanded := mode == TranscriptVerbose
	disclosure := "▸"
	if expanded {
		disclosure = "▾"
	}
	b.WriteString(style.Render(fmt.Sprintf("%s %s %s", icon, disclosure, summary)))
	if !expanded {
		if block.IsError && block.Content != nil {
			b.WriteString("\n" + errorStyle.Render(compactSummary(fmt.Sprintf("%v", block.Content), 120)))
		}
		return b.String()
	}

	if len(block.Input) > 0 {
		b.WriteString("\n\n" + systemStyle.Render("Input"))
		b.WriteString("\n" + renderToolInput(block.Input, width-2))
	}
	diff := block.Diff
	if diff == "" {
		diff = renderToolDiff(block, width-2)
	} else {
		diff = renderDiffOutput(diff, width-2)
	}
	if diff != "" {
		b.WriteString("\n\n" + systemStyle.Render("Changes"))
		b.WriteString("\n" + diff)
	}
	if block.Content != nil {
		b.WriteString("\n\n" + systemStyle.Render("Output"))
		b.WriteString("\n" + renderTextOutput(fmt.Sprintf("%v", block.Content), width-2))
	}
	if block.Truncated {
		b.WriteString("\n" + truncationStyle.Render(fmt.Sprintf("Output truncated from %d characters", block.OriginalChars)))
	}
	return b.String()
}

func renderToolInput(input json.RawMessage, width int) string {
	var value interface{}
	if err := json.Unmarshal(input, &value); err != nil {
		return toolOutputStyle.Render(wrapText(string(input), width))
	}
	pretty, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return toolOutputStyle.Render(wrapText(string(input), width))
	}
	return toolOutputStyle.Render(wrapText(string(pretty), width))
}

func toolCallSummary(block ContentBlock) string {
	var input map[string]interface{}
	_ = json.Unmarshal(block.Input, &input)
	switch block.Name {
	case "Read":
		return "Read " + firstString(input, "target_file", "file_path")
	case "Write", "Edit", "MultiEdit":
		path := firstString(input, "file_path", "target_file")
		added, removed := toolDiffStats(block.Name, input)
		stats := ""
		if added > 0 || removed > 0 {
			stats = fmt.Sprintf(" (+%d -%d)", added, removed)
		}
		return strings.TrimSpace(block.Name+" "+path) + stats
	case "Bash":
		return "Bash " + compactSummary(firstString(input, "command"), 72)
	case "Grep":
		return "Grep " + firstString(input, "pattern")
	case "Glob":
		return "Glob " + firstString(input, "glob_pattern", "pattern")
	default:
		return block.Name
	}
}

func firstString(input map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func compactSummary(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func renderToolDiff(block ContentBlock, width int) string {
	var input map[string]interface{}
	if err := json.Unmarshal(block.Input, &input); err != nil {
		return ""
	}
	path := firstString(input, "file_path", "target_file")
	var lines []string
	switch block.Name {
	case "Write":
		lines = append(lines, "--- /dev/null", "+++ "+path, "@@ new file @@")
		for _, line := range splitContentLines(firstString(input, "contents", "content")) {
			lines = append(lines, "+"+line)
		}
	case "Edit":
		lines = append(lines, "--- "+path, "+++ "+path, "@@ replacement @@")
		for _, line := range splitContentLines(firstString(input, "old_string")) {
			lines = append(lines, "-"+line)
		}
		for _, line := range splitContentLines(firstString(input, "new_string")) {
			lines = append(lines, "+"+line)
		}
	case "MultiEdit":
		lines = append(lines, "--- "+path, "+++ "+path)
		edits, _ := input["edits"].([]interface{})
		for index, raw := range edits {
			edit, _ := raw.(map[string]interface{})
			lines = append(lines, fmt.Sprintf("@@ edit %d @@", index+1))
			for _, line := range splitContentLines(firstString(edit, "old_string")) {
				lines = append(lines, "-"+line)
			}
			for _, line := range splitContentLines(firstString(edit, "new_string")) {
				lines = append(lines, "+"+line)
			}
		}
	default:
		return ""
	}
	return renderDiffOutput(strings.Join(lines, "\n"), width)
}

func toolDiffStats(name string, input map[string]interface{}) (added, removed int) {
	switch name {
	case "Write":
		return countContentLines(firstString(input, "contents", "content")), 0
	case "Edit":
		return countContentLines(firstString(input, "new_string")), countContentLines(firstString(input, "old_string"))
	case "MultiEdit":
		edits, _ := input["edits"].([]interface{})
		for _, raw := range edits {
			edit, _ := raw.(map[string]interface{})
			added += countContentLines(firstString(edit, "new_string"))
			removed += countContentLines(firstString(edit, "old_string"))
		}
	}
	return added, removed
}

func splitContentLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

func countContentLines(content string) int { return len(splitContentLines(content)) }

func renderToolProgress(block ContentBlock, width int) string {
	label := "Running"
	if block.Name != "" {
		label = "Running " + block.Name
	}
	return toolUseStyle.Render("↻ "+label) + "\n" + wrapText(fmt.Sprintf("%v", block.Content), width-2)
}

// renderToolUse renders a tool use block.
func renderToolUse(block ContentBlock, width int) string {
	var b strings.Builder

	// Tool name with icon
	b.WriteString(toolUseStyle.Render("🔧 "+block.Name) + "\n")

	// Input preview (truncated if too long)
	if len(block.Input) > 0 {
		inputPreview := string(block.Input)
		if len(inputPreview) > 200 {
			inputPreview = inputPreview[:200] + "..."
		}
		b.WriteString(wrapText(inputPreview, width-2))
	}

	return b.String()
}

// renderToolResult renders a tool result block.
func renderToolResult(block ContentBlock, width int) string {
	var b strings.Builder

	// Result header
	icon := "✓"
	style := toolResultStyle
	if block.IsError {
		icon = "✗"
		style = errorStyle
	}

	b.WriteString(style.Render(icon+" Tool Result") + "\n")

	// Content preview
	contentStr := fmt.Sprintf("%v", block.Content)
	if len(contentStr) > 500 {
		contentStr = contentStr[:500] + "\n... (truncated)"
	}
	b.WriteString(wrapText(contentStr, width-2))

	return b.String()
}

// renderThinking renders a thinking block.
func renderThinking(block ContentBlock, width int) string {
	var b strings.Builder

	b.WriteString(systemStyle.Render("💭 Thinking:") + "\n")

	// Truncate thinking if too long
	thinking := block.Thinking
	if len(thinking) > 300 {
		thinking = thinking[:300] + "..."
	}
	b.WriteString(wrapText(thinking, width-2))

	return b.String()
}

// wrapText wraps text to the specified width.
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}

		// Wrap long lines
		for len(line) > width {
			// Find a good break point
			breakPoint := width
			for j := width - 1; j >= 0 && j >= width-20; j-- {
				if line[j] == ' ' || line[j] == '\t' {
					breakPoint = j
					break
				}
			}

			result.WriteString(line[:breakPoint] + "\n")
			line = line[breakPoint:]
			if line[0] == ' ' || line[0] == '\t' {
				line = line[1:]
			}
		}
		result.WriteString(line)
	}

	return result.String()
}

// =============================================================================
// Message List Component
// =============================================================================

// MessageListModel represents a list of messages.
type MessageListModel struct {
	Messages     []MessageModel
	ScrollOffset int
	MaxVisible   int
	Width        int
	Height       int
	Mode         TranscriptMode
}

// NewMessageList creates a new message list.
func NewMessageList(width, height int) *MessageListModel {
	return &MessageListModel{
		Messages:     []MessageModel{},
		ScrollOffset: 0,
		MaxVisible:   height - 4, // Reserve space for input
		Width:        width,
		Height:       height,
		Mode:         TranscriptNormal,
	}
}

// AddMessage adds a message to the list.
func (m *MessageListModel) AddMessage(msg MessageModel) {
	m.Messages = append(m.Messages, msg)
	// Auto-scroll to bottom
	m.ScrollOffset = 0
}

// UpsertToolUse creates or refreshes a tool card while keeping expansion state.
func (m *MessageListModel) UpsertToolUse(toolName, toolUseID string, input json.RawMessage) {
	if block := m.findToolBlock(toolUseID); block != nil {
		block.Name = toolName
		block.Input = append(json.RawMessage(nil), input...)
		if block.Status == "" {
			block.Status = "running"
		}
		m.ScrollOffset = 0
		return
	}
	m.AddMessage(MessageModel{Role: "tool", Content: []ContentBlock{{
		Type: "tool", Name: toolName, ToolUseID: toolUseID,
		Input: append(json.RawMessage(nil), input...), Status: "running",
	}}})
}

// UpsertToolProgress keeps only the latest progress payload for each tool call.
func (m *MessageListModel) UpsertToolProgress(toolName, toolUseID, content string) {
	if block := m.findToolBlock(toolUseID); block != nil {
		block.Name = toolName
		block.Status = "running"
		block.Content = content
		m.ScrollOffset = 0
		return
	}
	for i := len(m.Messages) - 1; i >= 0; i-- {
		message := &m.Messages[i]
		if message.Role != "tool_progress" || len(message.Content) == 0 ||
			message.Content[0].ToolUseID != toolUseID {
			continue
		}
		message.Content[0].Name = toolName
		message.Content[0].Content = content
		m.ScrollOffset = 0
		return
	}
	m.AddMessage(MessageModel{
		Role: "tool_progress",
		Content: []ContentBlock{{
			Type:      "tool_progress",
			Name:      toolName,
			ToolUseID: toolUseID,
			Content:   content,
		}},
	})
}

// CompleteToolUse updates a tool card in place, preserving the call input for
// summaries, expandable details, and diff rendering.
func (m *MessageListModel) CompleteToolUse(toolName, toolUseID, content string, isError, truncated bool, originalChars int, display *types.ToolDisplay) {
	block := m.findToolBlock(toolUseID)
	if block == nil {
		m.UpsertToolUse(toolName, toolUseID, nil)
		block = m.findToolBlock(toolUseID)
	}
	block.Name = toolName
	block.Content = content
	block.IsError = isError
	block.Truncated = truncated
	block.OriginalChars = originalChars
	if display != nil {
		block.Summary = display.Summary
		block.FilePath = display.FilePath
		block.Diff = display.Diff
		block.Artifacts = append([]string(nil), display.Artifacts...)
	}
	block.Status = "completed"
	if isError {
		block.Status = "error"
	}
	m.ScrollOffset = 0
}

func (m *MessageListModel) findToolBlock(toolUseID string) *ContentBlock {
	for i := len(m.Messages) - 1; i >= 0; i-- {
		for j := range m.Messages[i].Content {
			block := &m.Messages[i].Content[j]
			if block.Type == "tool" && block.ToolUseID == toolUseID {
				return block
			}
		}
	}
	return nil
}

// CycleTranscriptMode rotates Normal → Verbose → Summary.
func (m *MessageListModel) CycleTranscriptMode() TranscriptMode {
	m.Mode = m.Mode.Next()
	m.ScrollOffset = 0
	return m.Mode
}

// ArtifactPaths returns successful file outputs in first-seen order.
func (m *MessageListModel) ArtifactPaths() []string {
	seen := make(map[string]bool)
	var paths []string
	for _, message := range m.Messages {
		for _, block := range message.Content {
			if block.Type != "tool" || block.IsError || block.Status != "completed" {
				continue
			}
			artifacts := block.Artifacts
			if len(artifacts) == 0 && (block.Name == "Write" || block.Name == "Edit" || block.Name == "MultiEdit") {
				var input map[string]interface{}
				_ = json.Unmarshal(block.Input, &input)
				if path := firstString(input, "file_path", "target_file"); path != "" {
					artifacts = []string{path}
				}
			}
			for _, path := range artifacts {
				if path != "" && !seen[path] {
					seen[path] = true
					paths = append(paths, path)
				}
			}
		}
	}
	return paths
}

// RemoveToolProgress removes the transient row once a final result arrives.
func (m *MessageListModel) RemoveToolProgress(toolUseID string) {
	for i := len(m.Messages) - 1; i >= 0; i-- {
		message := m.Messages[i]
		if message.Role != "tool_progress" || len(message.Content) == 0 ||
			message.Content[0].ToolUseID != toolUseID {
			continue
		}
		m.Messages = append(m.Messages[:i], m.Messages[i+1:]...)
		m.ScrollOffset = 0
		return
	}
}

// AppendAssistantDelta appends text to the active assistant message, creating
// that message when the first streamed chunk arrives.
func (m *MessageListModel) AppendAssistantDelta(text string) {
	if text == "" {
		return
	}
	if len(m.Messages) == 0 || m.Messages[len(m.Messages)-1].Role != "assistant" ||
		!m.Messages[len(m.Messages)-1].IsStreaming {
		m.Messages = append(m.Messages, MessageModel{
			Role:        "assistant",
			Content:     []ContentBlock{{Type: "text"}},
			IsStreaming: true,
		})
	}
	message := &m.Messages[len(m.Messages)-1]
	if len(message.Content) == 0 || message.Content[len(message.Content)-1].Type != "text" {
		message.Content = append(message.Content, ContentBlock{Type: "text"})
	}
	message.Content[len(message.Content)-1].Text += text
	m.ScrollOffset = 0
}

// FinalizeAssistantStream marks the active response complete. The complete
// response text is authoritative and also supports providers that emitted no
// text deltas.
func (m *MessageListModel) FinalizeAssistantStream(content string) {
	if len(m.Messages) > 0 {
		message := &m.Messages[len(m.Messages)-1]
		if message.Role == "assistant" && message.IsStreaming {
			if content != "" {
				message.Content = []ContentBlock{{Type: "text", Text: content}}
			}
			message.IsStreaming = false
			m.ScrollOffset = 0
			return
		}
	}
	if content != "" {
		m.AddMessage(MessageModel{
			Role:    "assistant",
			Content: []ContentBlock{{Type: "text", Text: content}},
		})
	}
}

// AbortAssistantStream removes the streaming marker while retaining any text
// that was received before an error or cancellation.
func (m *MessageListModel) AbortAssistantStream() {
	if len(m.Messages) == 0 {
		return
	}
	message := &m.Messages[len(m.Messages)-1]
	if message.Role == "assistant" {
		message.IsStreaming = false
	}
}

// HasStreamingAssistant reports whether the latest message is in progress.
func (m *MessageListModel) HasStreamingAssistant() bool {
	if len(m.Messages) == 0 {
		return false
	}
	message := m.Messages[len(m.Messages)-1]
	return message.Role == "assistant" && message.IsStreaming
}

// ScrollUp scrolls the rendered conversation up by one line.
func (m *MessageListModel) ScrollUp() {
	maxOffset := m.maxScrollOffset()
	if m.ScrollOffset < maxOffset {
		m.ScrollOffset++
	}
}

// ScrollDown scrolls the rendered conversation down by one line.
func (m *MessageListModel) ScrollDown() {
	if m.ScrollOffset > 0 {
		m.ScrollOffset--
	}
}

// PageUp scrolls upward by one viewport.
func (m *MessageListModel) PageUp() {
	step := m.MaxVisible - 1
	if step < 1 {
		step = 1
	}
	m.ScrollOffset += step
	maxOffset := m.maxScrollOffset()
	if m.ScrollOffset > maxOffset {
		m.ScrollOffset = maxOffset
	}
}

// PageDown scrolls downward by one viewport.
func (m *MessageListModel) PageDown() {
	step := m.MaxVisible - 1
	if step < 1 {
		step = 1
	}
	m.ScrollOffset -= step
	if m.ScrollOffset < 0 {
		m.ScrollOffset = 0
	}
}

// IsScrolled reports whether the viewport is above the latest messages.
func (m *MessageListModel) IsScrolled() bool {
	return m.ScrollOffset > 0
}

// View renders the message list as a fixed-height, line-based viewport.
func (m *MessageListModel) View() string {
	height := m.MaxVisible
	if height < 1 {
		height = 1
	}
	lines := m.renderedLines()
	maxOffset := len(lines) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.ScrollOffset > maxOffset {
		m.ScrollOffset = maxOffset
	}

	end := len(lines) - m.ScrollOffset
	start := end - height
	if start < 0 {
		start = 0
	}
	visible := append([]string(nil), lines[start:end]...)
	for len(visible) < height {
		visible = append(visible, "")
	}
	return strings.Join(visible, "\n") + "\n"
}

func (m *MessageListModel) renderedLines() []string {
	if len(m.Messages) == 0 {
		return nil
	}
	var b strings.Builder
	for index, msg := range m.Messages {
		if !m.messageVisible(index, msg) {
			continue
		}
		b.WriteString(renderMessageWithMode(msg, m.Width, m.Mode))
		b.WriteString("\n")
	}
	rendered := strings.TrimSuffix(b.String(), "\n")
	return strings.Split(rendered, "\n")
}

func (m *MessageListModel) messageVisible(index int, message MessageModel) bool {
	if m.Mode != TranscriptSummary {
		return true
	}
	switch message.Role {
	case "user", "summary":
		return true
	case "tool":
		for _, block := range message.Content {
			if block.Type == "tool" && (block.Diff != "" || len(block.Artifacts) > 0) {
				return true
			}
		}
		return false
	case "assistant":
		for next := index + 1; next < len(m.Messages); next++ {
			switch m.Messages[next].Role {
			case "user":
				return true
			case "assistant":
				return false
			}
		}
		return true
	default:
		return false
	}
}

// DiffEntries returns file changes in execution order for the /diff viewer.
func (m *MessageListModel) DiffEntries() []DiffEntry {
	var entries []DiffEntry
	for _, message := range m.Messages {
		for _, block := range message.Content {
			if block.Type != "tool" || block.Diff == "" {
				continue
			}
			path := block.FilePath
			if path == "" {
				var input map[string]interface{}
				_ = json.Unmarshal(block.Input, &input)
				path = firstString(input, "file_path", "target_file")
			}
			entries = append(entries, DiffEntry{Path: path, Summary: block.Summary, Diff: block.Diff})
		}
	}
	return entries
}

func (m *MessageListModel) maxScrollOffset() int {
	maxOffset := len(m.renderedLines()) - m.MaxVisible
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}
