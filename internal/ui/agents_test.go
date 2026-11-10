package ui

import (
	"strings"
	"testing"

	"claude-code-go/internal/tasks"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestAgentPanelLayoutAndBackgroundRefresh(t *testing.T) {
	for _, width := range []int{80, 140} {
		m := NewAppModelWithSubmit(nil, width, 30)
		m.SetTaskManager(tasks.NewManager(nil))
		// The parent query is idle; a task snapshot still updates the panel.
		task := &tasks.LocalAgentTaskState{
			TaskStateBase: tasks.TaskStateBase{ID: "agent-1", Description: "检查中文文件", Status: tasks.TaskStatusRunning},
			Progress:      &tasks.AgentProgress{Summary: "Running: Read", LastActivity: &tasks.ToolActivity{ToolName: "Read", ActivityDescription: "/tmp/example.go"}},
		}
		_, cmd := m.Update(agentSnapshotMsg{tasks: []*tasks.LocalAgentTaskState{task}})
		if cmd == nil {
			t.Fatal("background refresh not scheduled")
		}
		wantWidth, wantHeight := width, 24
		if width >= 110 {
			wantWidth, wantHeight = width-36, 30
		}
		if m.chat.Width != wantWidth || m.chat.Height != wantHeight {
			t.Fatalf("chat size: %dx%d", m.chat.Width, m.chat.Height)
		}
		panel := m.agentPanel(width, 6)
		for _, text := range []string{"running", "检查中文文件", "Read", "/tmp/example.go"} {
			if !strings.Contains(panel, text) {
				t.Fatalf("missing %q in %s", text, panel)
			}
		}
		view := m.View()
		if lipgloss.Width(view) > width {
			t.Fatalf("view width %d exceeds %d", lipgloss.Width(view), width)
		}
		task.Status = tasks.TaskStatusFailed
		task.Error = "permission denied"
		m.Update(agentSnapshotMsg{tasks: []*tasks.LocalAgentTaskState{task}})
		if !strings.Contains(m.View(), "permission denied") {
			t.Fatal("missing failure")
		}
		m.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
		if m.chat.Width != 70 || m.chat.Height != 18 {
			t.Fatal("resize did not switch layout")
		}
		m.Update(agentSnapshotMsg{})
		if m.chat.Height != 24 {
			t.Fatal("empty panel did not restore chat")
		}
	}
}
