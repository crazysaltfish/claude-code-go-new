package ui

import (
	"context"
	"time"

	"claude-code-go/internal/query"
	tea "github.com/charmbracelet/bubbletea"
)

const progressBatchInterval = 75 * time.Millisecond
const progressBatchLimit = 256

type queryBatchMsg struct{ events []interface{} }

func coalescible(event interface{}) bool {
	message, ok := event.(query.SDKMessage)
	return ok && (message.Type == "tool_progress" || message.Type == "assistant_delta")
}

// Read from a captured stream, never the mutable model pointer. Final responses,
// errors and tool results flush pending updates immediately, preserving order.
func readQueryBatch(ctx context.Context, stream <-chan interface{}) tea.Msg {
	if stream == nil {
		return queryCompleteMsg{}
	}
	var first interface{}
	select {
	case <-ctx.Done():
		return queryCompleteMsg{}
	case event, ok := <-stream:
		if !ok {
			return queryCompleteMsg{}
		}
		first = event
	}
	if !coalescible(first) {
		return QueryEngineMsg{Data: first}
	}
	timer := time.NewTimer(progressBatchInterval)
	defer timer.Stop()
	events := []interface{}{first}
	for count := 1; count < progressBatchLimit; count++ {
		select {
		case <-ctx.Done():
			return queryBatchMsg{events: compactQueryEvents(events)}
		case <-timer.C:
			return queryBatchMsg{events: compactQueryEvents(events)}
		case event, ok := <-stream:
			if !ok {
				return queryBatchMsg{events: compactQueryEvents(events)}
			}
			events = append(events, event)
			if !coalescible(event) {
				return queryBatchMsg{events: compactQueryEvents(events)}
			}
		}
	}
	return queryBatchMsg{events: compactQueryEvents(events)}
}

func compactQueryEvents(events []interface{}) []interface{} {
	result := make([]interface{}, 0, len(events))
	progress := make(map[string]int)
	for _, event := range events {
		message, ok := event.(query.SDKMessage)
		if !ok {
			result = append(result, event)
			continue
		}
		if message.Type == "tool_progress" {
			data, ok := message.Message.(map[string]interface{})
			id, _ := data["tool_use_id"].(string)
			if ok && id != "" {
				if index, exists := progress[id]; exists {
					result[index] = message
					continue
				}
				progress[id] = len(result)
			}
		}
		if message.Type == "assistant_delta" && len(result) > 0 {
			previous, ok := result[len(result)-1].(query.SDKMessage)
			nextDelta, nextOK := message.Message.(query.AssistantDelta)
			oldDelta, oldOK := previous.Message.(query.AssistantDelta)
			if ok && previous.Type == "assistant_delta" && previous.UUID == message.UUID && nextOK && oldOK {
				oldDelta.Text += nextDelta.Text
				oldDelta.Thinking += nextDelta.Thinking
				previous.Message = oldDelta
				result[len(result)-1] = previous
				continue
			}
		}
		result = append(result, event)
	}
	return result
}

func (m *AppModel) handleQueryEvent(event interface{}) {
	switch data := event.(type) {
	case query.SDKMessage:
		m.handleSDKMessage(data)
	case query.ResultMessage:
		m.handleResultMessage(data)
	}
}
