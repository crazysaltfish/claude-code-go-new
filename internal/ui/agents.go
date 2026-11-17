package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"claude-code-go/internal/tasks"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type agentSnapshotMsg struct{ tasks []*tasks.LocalAgentTaskState }

func (m *AppModel) SetTaskManager(manager *tasks.Manager) { m.taskManager = manager }

func (m *AppModel) pollAgents() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		var agents []*tasks.LocalAgentTaskState
		for _, state := range m.taskManager.GetAllTasks() {
			if task, ok := state.(*tasks.LocalAgentTaskState); ok && task.AgentType != "main-session" {
				agents = append(agents, task)
			}
		}
		sort.Slice(agents, func(i, j int) bool {
			active := func(t *tasks.LocalAgentTaskState) bool {
				return t.Status == tasks.TaskStatusRunning || t.Status == tasks.TaskStatusPending
			}
			if active(agents[i]) != active(agents[j]) {
				return active(agents[i])
			}
			if !agents[i].StartTime.Equal(agents[j].StartTime) {
				return agents[i].StartTime.After(agents[j].StartTime)
			}
			return agents[i].ID < agents[j].ID
		})
		return agentSnapshotMsg{tasks: agents}
	})
}

func (m *AppModel) chatSize() tea.WindowSizeMsg {
	w, h := m.width, m.height
	if len(m.agentTasks) > 0 && h >= 16 {
		if w >= 110 {
			w -= 36
		} else {
			h -= m.agentPanelHeight()
		}
	}
	return tea.WindowSizeMsg{Width: w, Height: h}
}

func (m *AppModel) updateChatSize(size tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	_, cmd := m.chat.Update(size)
	return m, cmd
}

func agentStatusStyle(status tasks.TaskStatus) lipgloss.Style {
	color := "245"
	switch status {
	case tasks.TaskStatusRunning:
		color = "6" // cyan
	case tasks.TaskStatusPending:
		color = "3" // yellow
	case tasks.TaskStatusCompleted:
		color = "2" // green
	case tasks.TaskStatusFailed:
		color = "1" // red
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color))
}

func (m *AppModel) agentPanelHeight() int {
	if len(m.agentTasks) > 1 && m.height >= 24 {
		return 11 // Two task cards and an overflow line.
	}
	return 6
}

func (m *AppModel) agentPanel(width, height int) string {
	contentWidth := max(1, width-2)
	fit := func(text string, limit int) string {
		if limit <= 0 {
			return ""
		}
		return runewidth.Truncate(strings.Join(strings.Fields(text), " "), limit, "…")
	}
	running := 0
	for _, task := range m.agentTasks {
		if task.Status == tasks.TaskStatusRunning {
			running++
		}
	}
	count := fmt.Sprintf("%d running", running)
	header := lipgloss.NewStyle().Bold(true).Render(fit("Sub-agents", contentWidth))
	countStyle := agentStatusStyle(tasks.TaskStatusRunning)
	if running == 0 {
		countStyle = agentStatusStyle(tasks.TaskStatusKilled)
	}
	if contentWidth >= runewidth.StringWidth("Sub-agents · ")+len(count) {
		header += " · " + countStyle.Render(count)
	} else {
		header = countStyle.Render(fit(count, contentWidth))
	}
	lines := []string{header}
	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("─", contentWidth))
	for i, task := range m.agentTasks {
		remaining := height - len(lines)
		if i+1 < len(m.agentTasks) {
			remaining-- // Keep room for the overflow indicator.
		}
		needed := 3 // Status, ID, and current phase.
		if i > 0 {
			needed++ // Divider between task cards.
		}
		if remaining < needed {
			lines = append(lines, fit(fmt.Sprintf("+ %d more agents", len(m.agentTasks)-i), contentWidth))
			break
		}
		if i > 0 {
			lines = append(lines, divider)
		}
		status := fit(string(task.Status), contentWidth)
		statusLine := agentStatusStyle(task.Status).Render(status)
		if available := contentWidth - runewidth.StringWidth(status) - 3; available > 0 && task.Description != "" {
			statusLine += " · " + fit(task.Description, available)
		}
		summary := "Waiting to start"
		if task.Progress != nil && task.Progress.Summary != "" {
			summary = task.Progress.Summary
		} else if task.Status == tasks.TaskStatusRunning {
			summary = "Starting"
		}
		switch task.Status {
		case tasks.TaskStatusCompleted:
			summary = "Finished"
		case tasks.TaskStatusFailed:
			summary = task.Error
			if summary == "" {
				summary = "Failed"
			}
		case tasks.TaskStatusKilled:
			summary = "Stopped"
		}
		lines = append(lines, statusLine, fit("ID: "+task.ID, contentWidth), fit(summary, contentWidth))
		if remaining > needed && task.Progress != nil {
			if activity := task.Progress.LastActivity; activity != nil {
				label := activity.ToolName
				if activity.ActivityDescription != "" {
					label += ": " + activity.ActivityDescription
				}
				lines = append(lines, fit(label, contentWidth))
			}
		}
	}
	return lipgloss.NewStyle().Width(contentWidth).Height(height).MaxHeight(height).PaddingLeft(1).BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Render(strings.Join(lines, "\n"))
}

func (m *AppModel) viewWithAgents() string {
	chat := m.chat.View()
	if len(m.agentTasks) == 0 || m.height < 16 {
		return chat
	}
	if m.width >= 110 {
		return lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(m.width-36).MaxWidth(m.width-36).Render(chat), m.agentPanel(36, m.height))
	}
	return lipgloss.JoinVertical(lipgloss.Left, chat, m.agentPanel(m.width, m.agentPanelHeight()))
}
