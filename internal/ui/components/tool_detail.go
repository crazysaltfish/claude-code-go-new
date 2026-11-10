package components

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

// ToolDetailModel indexes plain source lines once, and renders only visible
// rows. Input/output remain complete even when the transcript preview is short.
// Entries are a snapshot, so ongoing progress cannot move the reader's cursor.
type ToolDetailModel struct {
	Entries                   []ContentBlock
	Index, Offset, Horizontal int
	Width, Height             int
	text                      string
	starts                    []int
	Search                    string
	Searching                 bool
	searchPos                 int
	found                     bool
}

func NewToolDetail(entries []ContentBlock, width, height int) *ToolDetailModel {
	m := &ToolDetailModel{Entries: entries, Index: max(0, len(entries)-1), Width: width, Height: height, searchPos: -1}
	m.load()
	return m
}

func (m *MessageListModel) ToolEntries() []ContentBlock {
	var entries []ContentBlock
	for _, message := range m.Messages {
		for _, block := range message.Content {
			if block.Type == "tool" {
				entries = append(entries, block)
			}
		}
	}
	return entries
}

func (m *ToolDetailModel) load() {
	m.Offset, m.Horizontal, m.searchPos = 0, 0, -1
	m.found = false
	if len(m.Entries) == 0 {
		m.text = "No tool calls in this conversation."
	} else {
		block := m.Entries[m.Index]
		var b strings.Builder
		fmt.Fprintf(&b, "Tool: %s\nID: %s\nStatus: %s\n", block.Name, block.ToolUseID, block.Status)
		if block.IsError {
			b.WriteString("Error: true\n")
		}
		if len(block.Input) > 0 {
			b.WriteString("\nInput\n")
			b.Write(block.Input)
			b.WriteByte('\n')
		}
		if block.Diff != "" {
			b.WriteString("\nChanges\n")
			b.WriteString(block.Diff)
			b.WriteByte('\n')
		}
		if block.Content != nil {
			b.WriteString("\nOutput\n")
			b.WriteString(toolContentText(block.Content))
			b.WriteByte('\n')
		}
		if block.Truncated {
			fmt.Fprintf(&b, "\nTool output was truncated upstream from %d characters; showing all retained content.\n", block.OriginalChars)
		}
		m.text = sanitizeTerminalText(b.String())
	}
	m.starts = []int{0}
	for offset := 0; offset < len(m.text); {
		next := strings.IndexByte(m.text[offset:], '\n')
		if next < 0 {
			break
		}
		offset += next + 1
		m.starts = append(m.starts, offset)
	}
}

func (m *ToolDetailModel) bodyHeight() int { return max(1, m.Height-8) }
func (m *ToolDetailModel) Scroll(delta int) {
	m.Offset = max(0, min(m.Offset+delta, max(0, len(m.starts)-m.bodyHeight())))
}
func (m *ToolDetailModel) selectTool(delta int) {
	if len(m.Entries) == 0 {
		return
	}
	m.Index = (m.Index + delta + len(m.Entries)) % len(m.Entries)
	m.load()
}

func (m *ToolDetailModel) find(reverse bool) {
	if m.Search == "" {
		return
	}
	pos := -1
	if reverse {
		end := m.searchPos
		if end < 0 {
			end = len(m.text)
		}
		pos = strings.LastIndex(m.text[:end], m.Search)
		if pos < 0 {
			pos = strings.LastIndex(m.text, m.Search)
		}
	} else {
		start := m.searchPos + 1
		if start > len(m.text) {
			start = 0
		}
		if next := strings.Index(m.text[start:], m.Search); next >= 0 {
			pos = start + next
		} else {
			pos = strings.Index(m.text, m.Search)
		}
	}
	m.found = pos >= 0
	if pos < 0 {
		return
	}
	m.searchPos = pos
	row := sort.Search(len(m.starts), func(i int) bool { return m.starts[i] > pos }) - 1
	m.Offset = min(max(0, row), max(0, len(m.starts)-m.bodyHeight()))
	m.Horizontal = utf8.RuneCountInString(m.text[m.starts[max(0, row)]:pos])
}

// Update returns true when the viewer should close. It never handles approval
// keystrokes; the parent routes permission prompts before this viewer.
func (m *ToolDetailModel) Update(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
		m.Scroll(0)
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp {
			m.Scroll(-3)
		}
		if msg.Button == tea.MouseButtonWheelDown {
			m.Scroll(3)
		}
	case tea.KeyMsg:
		if m.Searching {
			switch msg.Type {
			case tea.KeyEsc:
				m.Searching = false
			case tea.KeyEnter:
				m.Searching = false
				m.searchPos = -1
				m.find(false)
			case tea.KeyBackspace, tea.KeyCtrlH:
				if len(m.Search) > 0 {
					_, size := utf8.DecodeLastRuneInString(m.Search)
					m.Search = m.Search[:len(m.Search)-size]
				}
			case tea.KeyRunes:
				m.Search += string(msg.Runes)
			}
			return false
		}
		switch msg.Type {
		case tea.KeyEsc:
			return true
		case tea.KeyUp:
			m.Scroll(-1)
		case tea.KeyDown:
			m.Scroll(1)
		case tea.KeyPgUp:
			m.Scroll(-m.bodyHeight())
		case tea.KeyPgDown:
			m.Scroll(m.bodyHeight())
		case tea.KeyHome:
			m.Offset = 0
			m.Horizontal = 0
		case tea.KeyEnd:
			m.Offset = max(0, len(m.starts)-m.bodyHeight())
		case tea.KeyLeft:
			m.Horizontal = max(0, m.Horizontal-8)
		case tea.KeyRight:
			m.Horizontal += 8
		case tea.KeyTab:
			m.selectTool(1)
		case tea.KeyShiftTab:
			m.selectTool(-1)
		default:
			switch msg.String() {
			case "q":
				return true
			case "/":
				m.Search = ""
				m.Searching = true
			case "n":
				m.find(false)
			case "N":
				m.find(true)
			}
		}
	}
	return false
}

func (m *ToolDetailModel) View() string {
	width := terminalContentWidth(m.Width)
	var b strings.Builder
	title := fmt.Sprintf("Tool details [%d/%d] · snapshot", min(m.Index+1, len(m.Entries)), len(m.Entries))
	b.WriteString(runewidth.Truncate(title, width, "…") + "\n")
	b.WriteString(strings.Repeat("─", width) + "\n")
	for row := m.Offset; row < m.Offset+m.bodyHeight(); row++ {
		if row < len(m.starts) {
			end := len(m.text)
			if row+1 < len(m.starts) {
				end = m.starts[row+1] - 1
			}
			line := m.text[m.starts[row]:end]
			// Skip horizontally without allocating a rune slice for an enormous line.
			for i := 0; i < m.Horizontal && len(line) > 0; i++ {
				_, size := utf8.DecodeRuneInString(line)
				line = line[size:]
			}
			b.WriteString(runewidth.Truncate(line, width, "…"))
		}
		b.WriteByte('\n')
	}
	status := fmt.Sprintf("Line %d/%d · Column %d", m.Offset+1, len(m.starts), m.Horizontal+1)
	if m.Searching {
		status = "Search: /" + m.Search
	} else if m.Search != "" {
		status = fmt.Sprintf("Search: %s · found: %t", m.Search, m.found)
	}
	b.WriteString(runewidth.Truncate(sanitizeTerminalText(status), width, "…") + "\n")
	b.WriteString(runewidth.Truncate("↑↓/PgUp/PgDn: scroll · ←→: pan · Tab/Shift+Tab: tool", width, "…") + "\n")
	b.WriteString(runewidth.Truncate("/: search · n/N: next/previous match · Esc/q: back", width, "…"))
	return chatContainerStyle.Render(b.String())
}
