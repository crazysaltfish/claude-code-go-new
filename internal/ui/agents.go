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
			h -= 6
		}
	}
	return tea.WindowSizeMsg{Width: w, Height: h}
}

func (m *AppModel) updateChatSize(size tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	_, cmd := m.chat.Update(size)
	return m, cmd
}

func (m *AppModel) agentPanel(width, height int) string {
	lines := []string{"Sub-agents"}
	for i, task := range m.agentTasks {
		// Reserve a line to make hidden tasks discoverable in small terminals.
		if len(lines)+3 > height-1 {
			lines = append(lines, fmt.Sprintf("+ %d more agents", len(m.agentTasks)-i))
			break
		}
		if height > 6 {
			lines = append(lines, "")
		}
		lines = append(lines, string(task.Status)+" · "+task.Description, "ID: "+task.ID)
		summary := "Waiting to start"
		if task.Progress != nil {
			summary = task.Progress.Summary
		}
		switch task.Status {
		case tasks.TaskStatusCompleted:
			summary = "Finished"
		case tasks.TaskStatusFailed:
			summary = task.Error
		case tasks.TaskStatusKilled:
			summary = "Stopped"
		}
		if len(lines) < height-1 {
			lines = append(lines, summary)
		}
		if task.Progress != nil && len(lines) < height-1 {
			if activity := task.Progress.LastActivity; activity != nil {
				lines = append(lines, activity.ToolName+": "+activity.ActivityDescription)
			}
		}
	}
	for i := range lines {
		lines[i] = runewidth.Truncate(strings.Join(strings.Fields(lines[i]), " "), width-2, "…")
	}
	return lipgloss.NewStyle().Width(width - 2).Height(height).MaxHeight(height).PaddingLeft(1).BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Render(strings.Join(lines, "\n"))
}

func (m *AppModel) viewWithAgents() string {
	chat := m.chat.View()
	if len(m.agentTasks) == 0 || m.height < 16 {
		return chat
	}
	if m.width >= 110 {
		return lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(m.width-36).MaxWidth(m.width-36).Render(chat), m.agentPanel(36, m.height))
	}
	return lipgloss.JoinVertical(lipgloss.Left, chat, m.agentPanel(m.width, 6))
}
