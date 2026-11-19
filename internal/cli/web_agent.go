package cli

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"claude-code-go/internal/query"
	"claude-code-go/internal/tasks"
	"claude-code-go/pkg/api"
)

const (
	webAgentLogLimit   = 200
	webAgentEntryLimit = 16 << 10
)

type webAgentLog struct {
	mu      sync.Mutex
	entries []webAgentEntry
	total   int
}

type webAgentEntry struct {
	ID        int    `json:"id"`
	Kind      string `json:"kind"`
	Tool      string `json:"tool,omitempty"`
	Text      string `json:"text"`
	Time      int64  `json:"time"`
	Failed    bool   `json:"failed,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

func (l *webAgentLog) record(message interface{}) {
	event, ok := message.(query.SDKMessage)
	if !ok {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	switch event.Type {
	case "assistant":
		response, ok := event.Message.(*api.MessageResponse)
		if !ok || response == nil {
			return
		}
		for _, block := range response.Content {
			switch block.Type {
			case "text":
				if strings.TrimSpace(block.Text) != "" {
					l.append(webAgentEntry{Kind: "message", Text: block.Text, Time: event.Timestamp})
				}
			case "tool_use":
				l.append(webAgentEntry{Kind: "tool_call", Tool: block.Name, Text: string(block.Input), Time: event.Timestamp})
			}
		}
	case "tool_result":
		data, ok := event.Message.(map[string]interface{})
		if !ok {
			return
		}
		name, _ := data["tool_name"].(string)
		output, _ := data["content"].(string)
		failed, _ := data["is_error"].(bool)
		truncated, _ := data["truncated"].(bool)
		l.append(webAgentEntry{Kind: "tool_result", Tool: name, Text: output, Time: event.Timestamp, Failed: failed, Truncated: truncated})
	}
}

func (l *webAgentLog) append(entry webAgentEntry) {
	if len(entry.Text) > webAgentEntryLimit {
		end := webAgentEntryLimit
		for !utf8.RuneStart(entry.Text[end]) {
			end--
		}
		entry.Text = strings.Clone(entry.Text[:end])
		entry.Truncated = true
	}
	l.total++
	entry.ID = l.total
	if len(l.entries) == webAgentLogLimit {
		l.entries = l.entries[1:]
	}
	l.entries = append(l.entries, entry)
}

func (c *webChat) agentDetail(w http.ResponseWriter, id string) {
	task, ok := c.app.toolRegistry.TaskManager().GetTask(id).(*tasks.LocalAgentTaskState)
	if !ok || task.SessionID != c.id {
		webError(w, http.StatusNotFound, "子 Agent 任务不存在")
		return
	}
	var entries []webAgentEntry
	var omitted int
	if value, ok := c.app.webAgentLogs.Load(id); ok {
		log := value.(*webAgentLog)
		log.mu.Lock()
		entries = append(entries, log.entries...)
		omitted = log.total - len(entries)
		log.mu.Unlock()
	}
	result := ""
	if task.Result != nil {
		result = fmt.Sprint(task.Result)
	}
	writeWebJSON(w, http.StatusOK, map[string]interface{}{
		"task": task, "entries": entries, "omitted": omitted, "result": result,
	})
}
