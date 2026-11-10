package components

import "strings"

const transcriptCacheEntries = 256

type cachedTranscriptMessage struct {
	lines []string
	used  uint64
}

func (m *MessageListModel) invalidateMessage(index int) {
	delete(m.renderCache, index)
	delete(m.lineHeights, index)
}

func (m *MessageListModel) invalidateFrom(index int) {
	for i := range m.renderCache {
		if i >= index {
			delete(m.renderCache, i)
		}
	}
	for i := range m.lineHeights {
		if i >= index {
			delete(m.lineHeights, i)
		}
	}
	m.toolIndex = nil
	m.toolIndexed = 0
}

func (m *MessageListModel) prepareCache() {
	if m.renderCache == nil || m.cacheWidth != m.Width || m.cacheMode != m.Mode {
		m.renderCache = make(map[int]cachedTranscriptMessage)
		m.lineHeights = make(map[int]int)
		m.cacheWidth, m.cacheMode = m.Width, m.Mode
	}
}

func (m *MessageListModel) messageHeight(index int) int {
	m.prepareCache()
	if !m.messageVisible(index, m.Messages[index]) {
		return 0
	}
	if height, ok := m.lineHeights[index]; ok {
		return height
	}
	return len(m.messageLines(index))
}

func (m *MessageListModel) messageLines(index int) []string {
	m.prepareCache()
	if !m.messageVisible(index, m.Messages[index]) {
		return nil
	}
	m.cacheClock++
	if cached, ok := m.renderCache[index]; ok {
		cached.used = m.cacheClock
		m.renderCache[index] = cached
		return cached.lines
	}
	// Cache only a bounded number of rendered messages; offscreen heights remain
	// available so scrolling past them never requires formatting their text again.
	if len(m.renderCache) >= transcriptCacheEntries {
		oldest := -1
		var stamp uint64
		for i, cached := range m.renderCache {
			if oldest < 0 || cached.used < stamp {
				oldest, stamp = i, cached.used
			}
		}
		delete(m.renderCache, oldest)
	}
	rendered := renderMessageWithMode(m.Messages[index], m.Width, m.Mode)
	lines := strings.Split(rendered, "\n")
	m.renderCache[index] = cachedTranscriptMessage{lines: lines, used: m.cacheClock}
	m.lineHeights[index] = len(lines)
	return lines
}

// viewportLines skips offscreen messages by their cached heights and allocates
// rows only for the requested screen, even when scrolled far into history.
func (m *MessageListModel) viewportLines(skip, height int) []string {
	var chunks [][]string
	count := 0
	for i := len(m.Messages) - 1; i >= 0 && count < height; i-- {
		size := m.messageHeight(i)
		if skip >= size {
			skip -= size
			continue
		}
		lines := m.messageLines(i)
		end := len(lines) - skip
		start := max(0, end-(height-count))
		chunks = append(chunks, lines[start:end])
		count += end - start
		skip = 0
	}
	result := make([]string, 0, height)
	for i := len(chunks) - 1; i >= 0; i-- {
		result = append(result, chunks[i]...)
	}
	return result
}

// Used for diagnostic/test transcript export, never for normal screen updates.
func (m *MessageListModel) tailLines(needed int) []string {
	var chunks [][]string
	count := 0
	for i := len(m.Messages) - 1; i >= 0 && count < needed; i-- {
		lines := m.messageLines(i)
		if len(lines) == 0 {
			continue
		}
		chunks = append(chunks, lines)
		count += len(lines)
	}
	result := make([]string, 0, count)
	for i := len(chunks) - 1; i >= 0; i-- {
		result = append(result, chunks[i]...)
	}
	return result
}

func (m *MessageListModel) clampOffset(offset int) int {
	height := max(1, m.MaxVisible)
	needed := height + max(0, offset)
	count := 0
	for i := len(m.Messages) - 1; i >= 0 && count < needed; i-- {
		count += m.messageHeight(i)
	}
	return min(max(0, offset), max(0, count-height))
}

type toolBlockLocation struct{ message, block int }

func (m *MessageListModel) indexedTool(toolUseID string) *ContentBlock {
	if m.toolIndex == nil || m.toolIndexed > len(m.Messages) {
		m.toolIndex = make(map[string]toolBlockLocation)
		m.toolIndexed = 0
	}
	for i := m.toolIndexed; i < len(m.Messages); i++ {
		for j, block := range m.Messages[i].Content {
			if block.Type == "tool" {
				m.toolIndex[block.ToolUseID] = toolBlockLocation{i, j}
			}
		}
	}
	m.toolIndexed = len(m.Messages)
	if location, ok := m.toolIndex[toolUseID]; ok {
		m.invalidateMessage(location.message)
		return &m.Messages[location.message].Content[location.block]
	}
	return nil
}
