package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"claude-code-go/internal/types"
	"claude-code-go/internal/ui/components"
	"claude-code-go/pkg/api"
)

// RestoreSession replays the full transcript into the chat while the engine
// separately uses the saved (possibly compacted) model context.
func (m *AppModel) RestoreSession(id string, history []types.Message) {
	size := m.chatSize()
	m.chat = components.NewChatModel(size.Width, size.Height)
	m.chat.PermissionMode = string(m.permissionMode)
	names := make(map[string]string)
	for _, message := range history {
		var blocks []api.ContentBlock
		if json.Unmarshal(message.Content, &blocks) != nil {
			continue
		}
		var text []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				text = append(text, b.Text)
			}
		}
		if len(text) > 0 {
			if message.Role == "assistant" {
				m.chat.AddAssistantMessage(strings.Join(text, "\n"))
			} else {
				m.chat.AddUserMessage(strings.Join(text, "\n"))
			}
		}
		for _, b := range blocks {
			switch b.Type {
			case "tool_use":
				names[b.ID] = b.Name
				m.chat.AddToolUse(b.Name, b.ID, b.Input)
			case "tool_result":
				content, ok := b.Content.(string)
				if !ok {
					data, _ := json.Marshal(b.Content)
					content = string(data)
				}
				m.chat.AddToolResult(names[b.ToolUseID], b.ToolUseID, content, b.IsError, false, len([]rune(content)), nil)
			}
		}
	}
	if len(history) > 0 {
		m.chat.AddSystemMessage(fmt.Sprintf("Resumed session %s (%d saved messages)", id, len(history)))
	} else {
		m.chat.AddSystemMessage("Session: " + id)
	}
}
