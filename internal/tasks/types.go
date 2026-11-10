package tasks

import (
	"time"
)

// =============================================================================
// Task Types
// =============================================================================

// TaskType represents the type of task.
type TaskType string

const (
	TaskTypeLocalShell    TaskType = "local_shell"
	TaskTypeLocalAgent    TaskType = "local_agent"
	TaskTypeRemoteAgent   TaskType = "remote_agent"
	TaskTypeLocalWorkflow TaskType = "local_workflow"
	TaskTypeMonitorMcp    TaskType = "monitor_mcp"
	TaskTypeDream         TaskType = "dream"
)

// TaskStatus represents the current status of a task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusKilled    TaskStatus = "killed"
)

// =============================================================================
// Task State Base
// =============================================================================

// TaskStateBase contains common fields for all task types.
type TaskStateBase struct {
	ID          string     `json:"id"`
	Type        TaskType   `json:"type"`
	Status      TaskStatus `json:"status"`
	Description string     `json:"description"`
	StartTime   time.Time  `json:"startTime"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	Notified    bool       `json:"notified"`
}

// =============================================================================
// Tool Activity
// =============================================================================

// ToolActivity represents activity from a tool use.
type ToolActivity struct {
	ToolName            string                 `json:"toolName"`
	Input               map[string]interface{} `json:"input"`
	ActivityDescription string                 `json:"activityDescription,omitempty"`
	IsSearch            bool                   `json:"isSearch,omitempty"`
	IsRead              bool                   `json:"isRead,omitempty"`
}

// =============================================================================
// Agent Progress
// =============================================================================

// AgentProgress tracks agent execution progress.
type AgentProgress struct {
	ToolUseCount     int            `json:"toolUseCount"`
	TokenCount       int            `json:"tokenCount"`
	LastActivity     *ToolActivity  `json:"lastActivity,omitempty"`
	RecentActivities []ToolActivity `json:"recentActivities,omitempty"`
	Summary          string         `json:"summary,omitempty"`
}

// ProgressTracker tracks token usage and tool use.
type ProgressTracker struct {
	ToolUseCount           int            `json:"toolUseCount"`
	LatestInputTokens      int            `json:"latestInputTokens"`
	CumulativeOutputTokens int            `json:"cumulativeOutputTokens"`
	RecentActivities       []ToolActivity `json:"recentActivities"`
}

// NewProgressTracker creates a new progress tracker.
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		ToolUseCount:           0,
		LatestInputTokens:      0,
		CumulativeOutputTokens: 0,
		RecentActivities:       []ToolActivity{},
	}
}

// GetTokenCount returns the total token count.
func (t *ProgressTracker) GetTokenCount() int {
	return t.LatestInputTokens + t.CumulativeOutputTokens
}

// GetProgressUpdate returns the current progress.
func (t *ProgressTracker) GetProgressUpdate() AgentProgress {
	var lastActivity *ToolActivity
	if len(t.RecentActivities) > 0 {
		lastActivity = &t.RecentActivities[len(t.RecentActivities)-1]
	}
	return AgentProgress{
		ToolUseCount:     t.ToolUseCount,
		TokenCount:       t.GetTokenCount(),
		LastActivity:     lastActivity,
		RecentActivities: append([]ToolActivity{}, t.RecentActivities...),
	}
}

// =============================================================================
// Local Agent Task
// =============================================================================

// LocalAgentTaskState represents a local agent task.
type LocalAgentTaskState struct {
	TaskStateBase
	SessionID              string         `json:"sessionId,omitempty"`
	AgentID                string         `json:"agentId"`
	Prompt                 string         `json:"prompt"`
	AgentType              string         `json:"agentType"`
	Model                  string         `json:"model,omitempty"`
	Error                  string         `json:"error,omitempty"`
	Result                 interface{}    `json:"result,omitempty"`
	Progress               *AgentProgress `json:"progress,omitempty"`
	Retrieved              bool           `json:"retrieved"`
	Messages               []interface{}  `json:"messages,omitempty"`
	LastReportedToolCount  int            `json:"lastReportedToolCount"`
	LastReportedTokenCount int            `json:"lastReportedTokenCount"`
	IsBackgrounded         bool           `json:"isBackgrounded"`
	PendingMessages        []string       `json:"pendingMessages"`
	Retain                 bool           `json:"retain"`
	DiskLoaded             bool           `json:"diskLoaded"`
	EvictAfter             *time.Time     `json:"evictAfter,omitempty"`
}

// IsLocalAgentTask checks if a task is a local agent task.
func IsLocalAgentTask(task interface{}) bool {
	t, ok := task.(*LocalAgentTaskState)
	return ok && t.Type == TaskTypeLocalAgent
}

// IsPanelAgentTask checks if a task should be shown in the panel.
func IsPanelAgentTask(task interface{}) bool {
	t, ok := task.(*LocalAgentTaskState)
	return ok && t.Type == TaskTypeLocalAgent && t.AgentType != "main-session"
}

// =============================================================================
// Local Shell Task
// =============================================================================

// LocalShellTaskState represents a background shell command task.
type LocalShellTaskState struct {
	TaskStateBase
	Command        string `json:"command"`
	Directory      string `json:"directory"`
	Error          string `json:"error,omitempty"`
	ExitCode       *int   `json:"exitCode,omitempty"`
	IsBackgrounded bool   `json:"isBackgrounded"`
}

// IsLocalShellTask checks if a task is a local shell task.
func IsLocalShellTask(task interface{}) bool {
	t, ok := task.(*LocalShellTaskState)
	return ok && t.Type == TaskTypeLocalShell
}

// =============================================================================
// Remote Agent Task
// =============================================================================

// RemoteAgentTaskState represents a remote agent task.
type RemoteAgentTaskState struct {
	TaskStateBase
	Command        string `json:"command"`
	SessionID      string `json:"sessionId"`
	URL            string `json:"url"`
	Error          string `json:"error,omitempty"`
	IsBackgrounded bool   `json:"isBackgrounded"`
}

// IsRemoteAgentTask checks if a task is a remote agent task.
func IsRemoteAgentTask(task interface{}) bool {
	t, ok := task.(*RemoteAgentTaskState)
	return ok && t.Type == TaskTypeRemoteAgent
}

// =============================================================================
// Task State Union
// =============================================================================

// TaskState is a union of all task state types.
type TaskState interface {
	GetBase() *TaskStateBase
}

// GetBase returns the base state.
func (t *LocalAgentTaskState) GetBase() *TaskStateBase {
	return &t.TaskStateBase
}

// GetBase returns the base state.
func (t *LocalShellTaskState) GetBase() *TaskStateBase {
	return &t.TaskStateBase
}

// GetBase returns the base state.
func (t *RemoteAgentTaskState) GetBase() *TaskStateBase {
	return &t.TaskStateBase
}

// IsBackgroundTask checks if a task should be shown in the background tasks indicator.
func IsBackgroundTask(task TaskState) bool {
	base := task.GetBase()
	if base.Status != TaskStatusRunning && base.Status != TaskStatusPending {
		return false
	}
	// Foreground tasks (isBackgrounded === false) are not yet "background tasks"
	switch t := task.(type) {
	case *LocalAgentTaskState:
		if !t.IsBackgrounded {
			return false
		}
	case *LocalShellTaskState:
		if !t.IsBackgrounded {
			return false
		}
	case *RemoteAgentTaskState:
		if !t.IsBackgrounded {
			return false
		}
	}
	return true
}
