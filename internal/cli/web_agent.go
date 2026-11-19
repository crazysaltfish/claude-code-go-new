package cli

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"claude-code-go/internal/query"
	"claude-code-go/internal/sessions"
	"claude-code-go/internal/tasks"
	"claude-code-go/pkg/api"
)

const webAgentEntryLimit = 16 << 10

type webAgentLog struct {
	mu      sync.Mutex
	entries []sessions.AgentEntry
}

func (l *webAgentLog) record(message interface{}) bool {
	event, ok := message.(query.SDKMessage)
	if !ok {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	switch event.Type {
	case "assistant":
		response, ok := event.Message.(*api.MessageResponse)
		if !ok || response == nil {
			return false
		}
		before := len(l.entries)
		for _, block := range response.Content {
			switch block.Type {
			case "text":
				if strings.TrimSpace(block.Text) != "" {
					l.append(sessions.AgentEntry{Kind: "message", Text: block.Text, Time: event.Timestamp})
				}
			case "tool_use":
				l.append(sessions.AgentEntry{Kind: "tool_call", CallID: block.ID, Tool: block.Name, Text: string(block.Input), Time: event.Timestamp})
			}
		}
		return len(l.entries) != before
	case "tool_result":
		data, ok := event.Message.(map[string]interface{})
		if !ok {
			return false
		}
		id, _ := data["tool_use_id"].(string)
		name, _ := data["tool_name"].(string)
		output, _ := data["content"].(string)
		failed, _ := data["is_error"].(bool)
		truncated, _ := data["truncated"].(bool)
		content, clipped := truncateAgentText(output)
		result := &sessions.AgentOutput{Text: content, Time: event.Timestamp, Failed: failed, Truncated: truncated || clipped}
		for i := len(l.entries) - 1; i >= 0; i-- {
			entry := &l.entries[i]
			if id != "" && entry.Kind == "tool_call" && entry.CallID == id {
				entry.Output = result
				return true
			}
		}
		l.append(sessions.AgentEntry{Kind: "tool_result", CallID: id, Tool: name, Time: event.Timestamp, Output: result})
		return true
	}
	return false
}

func (l *webAgentLog) append(entry sessions.AgentEntry) {
	var clipped bool
	entry.Text, clipped = truncateAgentText(entry.Text)
	entry.Truncated = entry.Truncated || clipped
	entry.ID = len(l.entries) + 1
	l.entries = append(l.entries, entry)
}

func truncateAgentText(value string) (string, bool) {
	if len(value) <= webAgentEntryLimit {
		return value, false
	}
	end := webAgentEntryLimit
	for !utf8.RuneStart(value[end]) {
		end--
	}
	return strings.Clone(value[:end]), true
}

func (a *App) webAgentLog(id string) *webAgentLog {
	value, _ := a.webAgentLogs.LoadOrStore(id, &webAgentLog{})
	return value.(*webAgentLog)
}

func (a *App) saveTerminalWebAgent(id string) {
	a.saveWebAgent(id, a.webAgentLog(id))
}

func (a *App) saveWebAgent(id string, recording *webAgentLog) {
	recording.mu.Lock()
	defer recording.mu.Unlock()
	task, ok := a.toolRegistry.TaskManager().GetTask(id).(*tasks.LocalAgentTaskState)
	if !ok || task.SessionID == "" {
		return
	}
	record := sessions.AgentRecord{Task: task, Entries: recording.entries}
	if err := a.sessionStore.SaveAgent(record); err != nil {
		log.Printf("save sub-agent record %s: %v", id, err)
	}
}

func (c *webChat) agentDetail(w http.ResponseWriter, id string) {
	var record sessions.AgentRecord
	if task, ok := c.app.toolRegistry.TaskManager().GetTask(id).(*tasks.LocalAgentTaskState); ok && task.SessionID == c.id {
		record.Task = task
		if value, ok := c.app.webAgentLogs.Load(id); ok {
			log := value.(*webAgentLog)
			log.mu.Lock()
			record.Entries = append(record.Entries, log.entries...)
			log.mu.Unlock()
		}
	} else {
		var err error
		record, err = c.app.sessionStore.LoadAgent(c.id, id)
		if err != nil {
			if os.IsNotExist(err) || c.archivedAgents[id] == nil {
				webError(w, http.StatusNotFound, "子 Agent 任务不存在")
			} else {
				webError(w, http.StatusInternalServerError, "无法读取子 Agent 执行记录")
			}
			return
		}
	}
	result := ""
	if record.Task.Result != nil {
		result = fmt.Sprint(record.Task.Result)
	}
	writeWebJSON(w, http.StatusOK, map[string]interface{}{
		"task": record.Task, "entries": record.Entries, "result": result,
	})
}
