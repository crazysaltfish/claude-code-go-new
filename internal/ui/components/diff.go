package components

import (
	"fmt"
	"strings"
)

// DiffEntry is one file-changing tool execution shown by the /diff viewer.
type DiffEntry struct {
	Path    string
	Summary string
	Diff    string
}

// DiffViewModel is a full-screen, read-only browser for session changes.
type DiffViewModel struct {
	Entries []DiffEntry
	Index   int
	Offset  int
	Width   int
	Height  int
}

func NewDiffView(entries []DiffEntry, width, height int) *DiffViewModel {
	return &DiffViewModel{Entries: append([]DiffEntry(nil), entries...), Width: width, Height: height}
}

func (m *DiffViewModel) Next() {
	if len(m.Entries) == 0 {
		return
	}
	m.Index = (m.Index + 1) % len(m.Entries)
	m.Offset = 0
}

func (m *DiffViewModel) Previous() {
	if len(m.Entries) == 0 {
		return
	}
	m.Index--
	if m.Index < 0 {
		m.Index = len(m.Entries) - 1
	}
	m.Offset = 0
}

func (m *DiffViewModel) Scroll(delta int) {
	m.Offset += delta
	if m.Offset < 0 {
		m.Offset = 0
	}
	maxOffset := m.maxOffset()
	if m.Offset > maxOffset {
		m.Offset = maxOffset
	}
}

func (m *DiffViewModel) Page(delta int) {
	step := m.bodyHeight() - 1
	if step < 1 {
		step = 1
	}
	m.Scroll(delta * step)
}

func (m *DiffViewModel) View() string {
	width := m.Width
	if width < 24 {
		width = 24
	}
	var b strings.Builder
	b.WriteString(chatHeaderStyle.Render("Claude Code · Diff") + "\n")
	b.WriteString(dividerStyle.Render(strings.Repeat("─", width)) + "\n")

	if len(m.Entries) == 0 {
		b.WriteString("\n" + systemStyle.Render("No file changes are available in this conversation.") + "\n")
		for line := 2; line < m.bodyHeight(); line++ {
			b.WriteString("\n")
		}
	} else {
		entry := m.Entries[m.Index]
		title := entry.Path
		if title == "" {
			title = entry.Summary
		}
		b.WriteString(filePathStyle.Render(fmt.Sprintf("[%d/%d] %s", m.Index+1, len(m.Entries), title)) + "\n")
		lines := m.diffLines()
		end := m.Offset + m.bodyHeight()
		if end > len(lines) {
			end = len(lines)
		}
		visible := lines[m.Offset:end]
		b.WriteString(strings.Join(visible, "\n"))
		b.WriteString("\n")
		for len(visible) < m.bodyHeight() {
			b.WriteString("\n")
			visible = append(visible, "")
		}
	}

	b.WriteString(dividerStyle.Render(strings.Repeat("─", width)) + "\n")
	footer := "←/→: change · ↑/↓ PgUp/PgDn: scroll · Esc/q: back"
	if len(m.Entries) > 0 {
		footer += fmt.Sprintf(" · line %d/%d", m.Offset+1, len(m.diffLines()))
	}
	b.WriteString(chatFooterStyle.Render(footer))
	return chatContainerStyle.Render(b.String())
}

func (m *DiffViewModel) bodyHeight() int {
	height := m.Height - 7
	if height < 3 {
		height = 3
	}
	return height
}

func (m *DiffViewModel) diffLines() []string {
	if len(m.Entries) == 0 || m.Index < 0 || m.Index >= len(m.Entries) {
		return nil
	}
	rendered := strings.TrimSuffix(renderDiffOutput(m.Entries[m.Index].Diff, m.Width-6), "\n")
	if rendered == "" {
		return nil
	}
	return strings.Split(rendered, "\n")
}

func (m *DiffViewModel) maxOffset() int {
	maxOffset := len(m.diffLines()) - m.bodyHeight()
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}
