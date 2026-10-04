package components

import (
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// =============================================================================
// Input Component
// =============================================================================

// Styles
var (
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	placeholderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243"))

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("86")).
			Bold(true)

	multilineHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))
)

// InputModel represents a text input field.
type InputModel struct {
	Value       string
	Placeholder string
	Prompt      string
	CursorPos   int
	Width       int
	Multiline   bool
	Focused     bool
	History     []string
	HistoryPos  int
}

// NewInput creates a new input field.
func NewInput(prompt, placeholder string, width int) *InputModel {
	return &InputModel{
		Value:       "",
		Placeholder: placeholder,
		Prompt:      prompt,
		CursorPos:   0,
		Width:       width,
		Multiline:   false,
		Focused:     true,
		History:     []string{},
		HistoryPos:  -1,
	}
}

// Update handles input updates.
func (m *InputModel) Update(msg tea.Msg) tea.Cmd {
	if !m.Focused {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyBackspace:
			if m.CursorPos > 0 {
				_, size := utf8.DecodeLastRuneInString(m.Value[:m.CursorPos])
				start := m.CursorPos - size
				m.Value = m.Value[:start] + m.Value[m.CursorPos:]
				m.CursorPos = start
			}
		case tea.KeyDelete:
			if m.CursorPos < len(m.Value) {
				_, size := utf8.DecodeRuneInString(m.Value[m.CursorPos:])
				m.Value = m.Value[:m.CursorPos] + m.Value[m.CursorPos+size:]
			}
		case tea.KeyLeft:
			if m.CursorPos > 0 {
				_, size := utf8.DecodeLastRuneInString(m.Value[:m.CursorPos])
				m.CursorPos -= size
			}
		case tea.KeyRight:
			if m.CursorPos < len(m.Value) {
				_, size := utf8.DecodeRuneInString(m.Value[m.CursorPos:])
				m.CursorPos += size
			}
		case tea.KeyHome:
			m.CursorPos = 0
		case tea.KeyEnd:
			m.CursorPos = len(m.Value)
		case tea.KeyUp:
			// Navigate history
			if len(m.History) > 0 && m.HistoryPos < len(m.History)-1 {
				m.HistoryPos++
				m.Value = m.History[len(m.History)-1-m.HistoryPos]
				m.CursorPos = len(m.Value)
			}
		case tea.KeyDown:
			// Navigate history
			if m.HistoryPos > 0 {
				m.HistoryPos--
				m.Value = m.History[len(m.History)-1-m.HistoryPos]
				m.CursorPos = len(m.Value)
			} else if m.HistoryPos == 0 {
				m.HistoryPos = -1
				m.Value = ""
				m.CursorPos = 0
			}
		case tea.KeyRunes:
			// Insert character at cursor position
			runes := msg.Runes
			m.Value = m.Value[:m.CursorPos] + string(runes) + m.Value[m.CursorPos:]
			m.CursorPos += len(string(runes))
		case tea.KeySpace:
			// Bubble Tea emits a dedicated KeySpace event instead of KeyRunes.
			space := string(msg.Runes)
			if space == "" {
				space = " "
			}
			m.Value = m.Value[:m.CursorPos] + space + m.Value[m.CursorPos:]
			m.CursorPos += len(space)
		}
	}

	return nil
}

// View renders the input field.
func (m *InputModel) View() string {
	var b strings.Builder

	// Render prompt
	b.WriteString(promptStyle.Render(m.Prompt) + " ")

	// Render value with cursor
	if m.Value == "" {
		// Keep a visible block cursor even before the first character.
		b.WriteString(cursorStyle.Render(" ") + placeholderStyle.Render(m.Placeholder))
	} else {
		// Show value with cursor
		before := m.Value[:m.CursorPos]
		atCursor := " "
		after := ""
		if m.CursorPos < len(m.Value) {
			r, size := utf8.DecodeRuneInString(m.Value[m.CursorPos:])
			atCursor = string(r)
			after = m.Value[m.CursorPos+size:]
		}

		b.WriteString(before + cursorStyle.Render(atCursor) + after)
	}

	// Multiline hint
	if m.Multiline {
		b.WriteString("\n" + multilineHintStyle.Render("Shift+Enter for new line"))
	}

	innerWidth := m.Width - inputBoxStyle.GetHorizontalFrameSize()
	if innerWidth < 1 {
		innerWidth = 1
	}
	return inputBoxStyle.Width(innerWidth).Render(ansi.Truncate(b.String(), innerWidth, ""))
}

// SetValue sets the input value.
func (m *InputModel) SetValue(value string) {
	m.Value = value
	m.CursorPos = len(value)
}

// Clear clears the input.
func (m *InputModel) Clear() {
	// Save to history if not empty
	if m.Value != "" {
		m.History = append(m.History, m.Value)
	}
	m.Value = ""
	m.CursorPos = 0
	m.HistoryPos = -1
}

// Focus focuses the input.
func (m *InputModel) Focus() {
	m.Focused = true
}

// Blur unfocuses the input.
func (m *InputModel) Blur() {
	m.Focused = false
}

// =============================================================================
// Multiline Input Component
// =============================================================================

// MultilineInputModel represents a multiline text input.
type MultilineInputModel struct {
	*InputModel
	Lines []string
}

// NewMultilineInput creates a new multiline input.
func NewMultilineInput(prompt, placeholder string, width int) *MultilineInputModel {
	return &MultilineInputModel{
		InputModel: NewInput(prompt, placeholder, width),
		Lines:      []string{""},
	}
}

// View renders the multiline input.
func (m *MultilineInputModel) View() string {
	var b strings.Builder

	// Render prompt
	b.WriteString(promptStyle.Render(m.Prompt) + "\n")

	// Render each line
	for i, line := range m.Lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  " + line)
	}

	// Show cursor on last line if empty
	if len(m.Lines) == 1 && m.Lines[0] == "" {
		b.WriteString(placeholderStyle.Render(m.Placeholder))
	}

	innerWidth := m.Width - inputBoxStyle.GetHorizontalFrameSize()
	if innerWidth < 1 {
		innerWidth = 1
	}
	return inputBoxStyle.Width(innerWidth).Render(ansi.Truncate(b.String(), innerWidth, ""))
}
