package tasks

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// =============================================================================
// Task Registry
// =============================================================================

// Registry manages all active tasks.
type Registry struct {
	mu     sync.RWMutex
	tasks  map[string]TaskState
	output map[string]string // task ID -> output file path
}

// NewRegistry creates a new task registry.
func NewRegistry() *Registry {
	return &Registry{
		tasks:  make(map[string]TaskState),
		output: make(map[string]string),
	}
}

// Register adds a task to the registry.
func (r *Registry) Register(task TaskState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := task.GetBase().ID
	if _, exists := r.tasks[id]; exists {
		return fmt.Errorf("task %s already exists", id)
	}

	r.tasks[id] = cloneTask(task)
	return nil
}

// Unregister removes a task from the registry.
func (r *Registry) Unregister(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tasks, taskID)
	delete(r.output, taskID)
}

// Get retrieves a task by ID.
func (r *Registry) Get(taskID string) TaskState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return cloneTask(r.tasks[taskID])
}

// GetAll returns all tasks.
func (r *Registry) GetAll() map[string]TaskState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]TaskState)
	for k, v := range r.tasks {
		result[k] = cloneTask(v)
	}
	return result
}

// Update updates a task's state.
func (r *Registry) Update(taskID string, updateFn func(TaskState) TaskState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, exists := r.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	r.tasks[taskID] = cloneTask(updateFn(task))
	return nil
}

// cloneTask returns a detached snapshot so callers cannot race with executor
// state transitions by mutating a task stored in the registry.
func cloneTask(task TaskState) TaskState {
	switch source := task.(type) {
	case *LocalAgentTaskState:
		if source == nil {
			return nil
		}
		copy := *source
		copy.PendingMessages = append([]string(nil), source.PendingMessages...)
		copy.Messages = append([]interface{}(nil), source.Messages...)
		if source.Progress != nil {
			progress := *source.Progress
			progress.RecentActivities = append([]ToolActivity(nil), source.Progress.RecentActivities...)
			if source.Progress.LastActivity != nil {
				activity := *source.Progress.LastActivity
				progress.LastActivity = &activity
			}
			copy.Progress = &progress
		}
		return &copy
	case *LocalShellTaskState:
		if source == nil {
			return nil
		}
		copy := *source
		if source.ExitCode != nil {
			exitCode := *source.ExitCode
			copy.ExitCode = &exitCode
		}
		return &copy
	case *RemoteAgentTaskState:
		if source == nil {
			return nil
		}
		copy := *source
		return &copy
	default:
		return nil
	}
}

// =============================================================================
// Task Creation Helpers
// =============================================================================

// CreateLocalAgentTask creates a new local agent task.
func CreateLocalAgentTask(id, prompt, agentType, description string) *LocalAgentTaskState {
	return &LocalAgentTaskState{
		TaskStateBase: TaskStateBase{
			ID:          id,
			Type:        TaskTypeLocalAgent,
			Status:      TaskStatusPending,
			Description: description,
			StartTime:   time.Now(),
			Notified:    false,
		},
		AgentID:         id,
		Prompt:          prompt,
		AgentType:       agentType,
		Retrieved:       false,
		IsBackgrounded:  false,
		PendingMessages: []string{},
		Retain:          false,
		DiskLoaded:      false,
	}
}

// CreateLocalShellTask creates a new local shell task.
func CreateLocalShellTask(id, command, directory, description string) *LocalShellTaskState {
	return &LocalShellTaskState{
		TaskStateBase: TaskStateBase{
			ID:          id,
			Type:        TaskTypeLocalShell,
			Status:      TaskStatusPending,
			Description: description,
			StartTime:   time.Now(),
			Notified:    false,
		},
		Command:        command,
		Directory:      directory,
		IsBackgrounded: false,
	}
}

// CreateRemoteAgentTask creates a new remote agent task.
func CreateRemoteAgentTask(id, command, sessionID, description string) *RemoteAgentTaskState {
	return &RemoteAgentTaskState{
		TaskStateBase: TaskStateBase{
			ID:          id,
			Type:        TaskTypeRemoteAgent,
			Status:      TaskStatusPending,
			Description: description,
			StartTime:   time.Now(),
			Notified:    false,
		},
		Command:        command,
		SessionID:      sessionID,
		IsBackgrounded: true, // Remote tasks are always backgrounded
	}
}

// =============================================================================
// Task Output Management
// =============================================================================

// GetTaskOutputPath returns the path to a task's output file.
func GetTaskOutputPath(taskID string) string {
	// Use system temp directory
	return filepath.Join(os.TempDir(), "claude-code-go", "tasks", taskID+".output")
}

// InitTaskOutput initializes the task output file.
func InitTaskOutput(taskID string) error {
	outputPath := GetTaskOutputPath(taskID)
	dir := filepath.Dir(outputPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create task output directory: %w", err)
	}

	// Create empty file
	if err := ioutil.WriteFile(outputPath, []byte{}, 0644); err != nil {
		return fmt.Errorf("failed to create task output file: %w", err)
	}

	return nil
}

// AppendTaskOutput appends data to the task output file.
func AppendTaskOutput(taskID string, data []byte) error {
	outputPath := GetTaskOutputPath(taskID)

	f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open task output file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write task output: %w", err)
	}

	return nil
}

// ReadTaskOutput reads the task output file.
func ReadTaskOutput(taskID string) (string, error) {
	outputPath := GetTaskOutputPath(taskID)

	data, err := ioutil.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to read task output: %w", err)
	}

	return string(data), nil
}

// EvictTaskOutput removes the task output file.
func EvictTaskOutput(taskID string) error {
	outputPath := GetTaskOutputPath(taskID)

	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to evict task output: %w", err)
	}

	return nil
}

// =============================================================================
// Task Notification
// =============================================================================

// TaskNotification represents a notification about a task.
type TaskNotification struct {
	TaskID      string    `json:"taskId"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	OutputPath  string    `json:"outputPath"`
	Timestamp   time.Time `json:"timestamp"`
}

// CreateNotification creates a task notification.
func CreateNotification(task TaskState, errMsg string) TaskNotification {
	base := task.GetBase()
	return TaskNotification{
		TaskID:      base.ID,
		Description: base.Description,
		Status:      string(base.Status),
		Error:       errMsg,
		OutputPath:  GetTaskOutputPath(base.ID),
		Timestamp:   time.Now(),
	}
}

// ToJSON converts the notification to JSON.
func (n TaskNotification) ToJSON() (string, error) {
	data, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// =============================================================================
// Task State Transitions
// =============================================================================

// StartTask marks a task as running.
func StartTask(task TaskState) TaskState {
	base := task.GetBase()
	base.Status = TaskStatusRunning
	return task
}

// CompleteTask marks a task as completed.
func CompleteTask(task TaskState) TaskState {
	base := task.GetBase()
	base.Status = TaskStatusCompleted
	now := time.Now()
	base.EndTime = &now
	return task
}

// FailTask marks a task as failed.
func FailTask(task TaskState, errMsg string) TaskState {
	base := task.GetBase()
	base.Status = TaskStatusFailed
	now := time.Now()
	base.EndTime = &now

	// Set error message on specific task type
	switch t := task.(type) {
	case *LocalAgentTaskState:
		t.Error = errMsg
	case *LocalShellTaskState:
		t.Error = errMsg
	case *RemoteAgentTaskState:
		t.Error = errMsg
	}

	return task
}

// KillTask marks a task as killed.
func KillTask(task TaskState) TaskState {
	base := task.GetBase()
	base.Status = TaskStatusKilled
	now := time.Now()
	base.EndTime = &now
	return task
}
