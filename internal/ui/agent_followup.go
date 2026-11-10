package ui

import (
	"fmt"

	"claude-code-go/internal/ui/components"
	tea "github.com/charmbracelet/bubbletea"
)

type agentFinishedMsg struct{}

// SetAgentFollowupSubmitter connects automatic turns to the active query engine.
func (m *AppModel) SetAgentFollowupSubmitter(submit SubmitFunc) {
	m.submitAgentFollowup = submit
}

func (m *AppModel) waitForAgentFinished() tea.Cmd {
	ctx, finished := m.ctx, m.taskManager.AgentFinished()
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case <-finished:
			return agentFinishedMsg{}
		}
	}
}

func (m *AppModel) startAgentFollowup() tea.Cmd {
	// A result event can arrive before the stream closes. Only stream closure
	// releases the slot, so no follow-up can replace a stream still draining.
	if m.messageChan != nil || m.pendingPermission != nil || m.submitAgentFollowup == nil || m.ctx.Err() != nil {
		return nil
	}
	batch := m.agentFollowups.Next(m.taskManager, m.sessionID)
	if batch.Prompt == "" {
		return nil
	}
	output, err := m.submitAgentFollowup(m.ctx, batch.Prompt)
	if err != nil {
		m.chat.SetError(fmt.Errorf("sub-agent follow-up: %w", err))
		return nil
	}
	m.agentFollowups.Acknowledge(batch)
	m.chat.ClearError()
	m.chat.AddSystemMessage(fmt.Sprintf("Sub-agent update: %d task(s) finished", len(batch.IDs)))
	m.chat.State = components.ChatStateProcessing
	m.messageChan = output
	return m.waitForMessages()
}
