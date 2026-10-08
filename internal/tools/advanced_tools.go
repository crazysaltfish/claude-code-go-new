package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"claude-code-go/internal/constants"
	"claude-code-go/internal/tasks"
	"claude-code-go/internal/types"
)

// =============================================================================
// Web Search Tool
// =============================================================================

// WebSearchTool searches the web for information.
type WebSearchTool struct {
	*BaseTool
}

// NewWebSearchTool creates a new web search tool.
func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		BaseTool: &BaseTool{
			name:        constants.ToolWebSearch,
			description: "Search the web for current information using built-in search capabilities",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"query": {
						"type":        "string",
						"minLength":   2,
						"description": "The search query to use (minimum 2 characters)",
					},
					"allowed_domains": {
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Only include search results from these domains",
					},
					"blocked_domains": {
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Never include search results from these domains",
					},
				},
				Required: []string{"query"},
			},
			isEnabled:  false,
			isReadOnly: true,
		},
	}
}

// Call performs a web search.
func (t *WebSearchTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Query          string   `json:"query"`
		AllowedDomains []string `json:"allowed_domains,omitempty"`
		BlockedDomains []string `json:"blocked_domains,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Validate input
	if len(input.Query) < 2 {
		return &types.ToolResult{
			Error:     fmt.Errorf("query must be at least 2 characters"),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	if len(input.AllowedDomains) > 0 && len(input.BlockedDomains) > 0 {
		return &types.ToolResult{
			Error:     fmt.Errorf("cannot specify both allowed_domains and blocked_domains"),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// Check permissions
	permResult, err := t.CheckPermissions(ctx, args, toolCtx)
	if err != nil {
		return nil, err
	}
	if permResult.Behavior == types.PermissionBehaviorDeny {
		return &types.ToolResult{
			Error:     fmt.Errorf("permission denied: %s", permResult.Message),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// For now, return a placeholder - actual implementation would use API
	// This is similar to how the TS version defers to the model's built-in web search
	return &types.ToolResult{
		Output:    fmt.Sprintf("Web search for '%s' would be performed. (Requires API integration)", input.Query),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// ValidateInput rejects contradictory search scopes before permission approval.
func (t *WebSearchTool) ValidateInput(args json.RawMessage) error {
	var input struct {
		Query          string   `json:"query"`
		AllowedDomains []string `json:"allowed_domains,omitempty"`
		BlockedDomains []string `json:"blocked_domains,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return err
	}
	if len(input.Query) < 2 {
		return fmt.Errorf("query must be at least 2 characters")
	}
	if len(input.AllowedDomains) > 0 && len(input.BlockedDomains) > 0 {
		return fmt.Errorf("cannot specify both allowed_domains and blocked_domains")
	}
	return nil
}

// =============================================================================
// Skill Tool
// =============================================================================

// SkillTool executes predefined skills.
type SkillTool struct {
	*BaseTool
	skills map[string]SkillDefinition
}

// SkillDefinition represents a predefined skill.
type SkillDefinition struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Prompt      string            `json:"prompt"`
	Tools       []string          `json:"tools,omitempty"`
	Parameters  map[string]string `json:"parameters,omitempty"`
}

// NewSkillTool creates a new skill tool.
func NewSkillTool() *SkillTool {
	return &SkillTool{
		BaseTool: &BaseTool{
			name:        constants.ToolSkill,
			description: "Execute predefined skills for common tasks",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"skill_name": {
						"type":        "string",
						"description": "The name of the skill to execute",
					},
					"parameters": {
						"type":        "object",
						"description": "Parameters to pass to the skill",
					},
				},
				Required: []string{"skill_name"},
			},
			isEnabled:  false,
			isReadOnly: false,
		},
		skills: make(map[string]SkillDefinition),
	}
}

// RegisterSkill registers a new skill.
func (t *SkillTool) RegisterSkill(skill SkillDefinition) {
	t.skills[skill.Name] = skill
}

// Call executes a skill.
func (t *SkillTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		SkillName  string                 `json:"skill_name"`
		Parameters map[string]interface{} `json:"parameters,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	skill, ok := t.skills[input.SkillName]
	if !ok {
		return &types.ToolResult{
			Error:     fmt.Errorf("skill '%s' not found", input.SkillName),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// Execute skill (placeholder - actual implementation would expand prompt)
	return &types.ToolResult{
		Output:    fmt.Sprintf("Skill '%s' executed: %s", skill.Name, skill.Description),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// LSP Tool
// =============================================================================

// LSPTool provides language server protocol features.
type LSPTool struct {
	*BaseTool
	servers map[string]*LSPClient
}

// LSPClient represents a connection to an LSP server.
type LSPClient struct {
	Name    string
	Command string
	Args    []string
	Process *exec.Cmd
}

// NewLSPTool creates a new LSP tool.
func NewLSPTool() *LSPTool {
	return &LSPTool{
		BaseTool: &BaseTool{
			name:        "LSP",
			description: "Provides code intelligence features (definitions, references, symbols, hover)",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"operation": {
						"type":        "string",
						"enum":        []string{"goToDefinition", "findReferences", "hover", "documentSymbol", "workspaceSymbol", "goToImplementation", "prepareCallHierarchy", "incomingCalls", "outgoingCalls"},
						"description": "The LSP operation to perform",
					},
					"filePath": {
						"type":        "string",
						"description": "The absolute or relative path to the file",
					},
					"line": {
						"type":        "number",
						"description": "The line number (1-based, as shown in editors)",
					},
					"character": {
						"type":        "number",
						"description": "The character offset (1-based, as shown in editors)",
					},
				},
				Required: []string{"operation", "filePath", "line", "character"},
			},
			isEnabled:  false,
			isReadOnly: true,
		},
		servers: make(map[string]*LSPClient),
	}
}

// Call performs an LSP operation.
func (t *LSPTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Operation string `json:"operation"`
		FilePath  string `json:"filePath"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Validate file exists
	if _, err := os.Stat(input.FilePath); os.IsNotExist(err) {
		return &types.ToolResult{
			Error:     fmt.Errorf("file does not exist: %s", input.FilePath),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// Get file extension to determine language server
	ext := filepath.Ext(input.FilePath)

	// Placeholder response - actual implementation would communicate with LSP server
	return &types.ToolResult{
		Output: fmt.Sprintf("LSP operation '%s' on %s (line %d, char %d). Language server for %s files would process this.",
			input.Operation, input.FilePath, input.Line, input.Character, ext),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// IsConcurrencySafe returns true for LSP tool.
func (t *LSPTool) IsConcurrencySafe(input json.RawMessage) bool {
	return true
}

// =============================================================================
// Task Management Tools
// =============================================================================

// TaskCreateTool creates new tasks.
type TaskCreateTool struct {
	*BaseTool
	mu    sync.RWMutex
	tasks map[string]*Task
}

// Task represents a task in the system.
type Task struct {
	ID          string                 `json:"id"`
	Subject     string                 `json:"subject"`
	Description string                 `json:"description"`
	ActiveForm  string                 `json:"activeForm,omitempty"`
	Status      string                 `json:"status"`
	Owner       string                 `json:"owner,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
}

// NewTaskCreateTool creates a new task create tool.
func NewTaskCreateTool() *TaskCreateTool {
	return &TaskCreateTool{
		BaseTool: &BaseTool{
			name:        constants.ToolTaskCreate,
			description: "Create a new task in the task list",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"subject": {
						"type":        "string",
						"description": "A brief title for the task",
					},
					"description": {
						"type":        "string",
						"description": "What needs to be done",
					},
					"activeForm": {
						"type":        "string",
						"description": "Present continuous form shown in spinner when in_progress",
					},
					"metadata": {
						"type":        "object",
						"description": "Arbitrary metadata to attach to the task",
					},
				},
				Required: []string{"subject", "description"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
		tasks: make(map[string]*Task),
	}
}

// Call creates a new task.
func (t *TaskCreateTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Subject     string                 `json:"subject"`
		Description string                 `json:"description"`
		ActiveForm  string                 `json:"activeForm,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}
	input.Subject = strings.TrimSpace(input.Subject)
	input.Description = strings.TrimSpace(input.Description)
	if input.Subject == "" || input.Description == "" {
		return nil, fmt.Errorf("subject and description are required")
	}

	// Generate task ID
	taskID := generateTaskID()

	task := &Task{
		ID:          taskID,
		Subject:     input.Subject,
		Description: input.Description,
		ActiveForm:  input.ActiveForm,
		Status:      "pending",
		Metadata:    input.Metadata,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	t.mu.Lock()
	t.tasks[taskID] = task
	t.mu.Unlock()

	return &types.ToolResult{
		Output:    fmt.Sprintf("Task #%s created successfully: %s", taskID, input.Subject),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// GetTask retrieves a task by ID.
func (t *TaskCreateTool) GetTask(id string) (*Task, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	task, ok := t.tasks[id]
	return cloneManagedTask(task), ok
}

func (t *TaskCreateTool) updateTask(id string, update func(*Task) error) (*Task, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	task, ok := t.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err := update(task); err != nil {
		return nil, err
	}
	task.UpdatedAt = time.Now()
	return cloneManagedTask(task), nil
}

func (t *TaskCreateTool) listTasks() []*Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*Task, 0, len(t.tasks))
	for _, task := range t.tasks {
		result = append(result, cloneManagedTask(task))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

func cloneManagedTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	copy := *task
	if task.Metadata != nil {
		copy.Metadata = make(map[string]interface{}, len(task.Metadata))
		for key, value := range task.Metadata {
			copy.Metadata[key] = value
		}
	}
	return &copy
}

// TaskListTool lists all tasks.
type TaskListTool struct {
	*BaseTool
	taskCreateTool *TaskCreateTool
	taskManager    *tasks.Manager
}

// NewTaskListTool creates a new task list tool.
func NewTaskListTool(taskCreateTool *TaskCreateTool, managers ...*tasks.Manager) *TaskListTool {
	var taskManager *tasks.Manager
	if len(managers) > 0 {
		taskManager = managers[0]
	}
	return &TaskListTool{
		BaseTool: &BaseTool{
			name:        constants.ToolTaskList,
			description: "List all tasks",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"status": {
						"type":        "string",
						"enum":        []string{"pending", "in_progress", "running", "completed", "failed", "cancelled", "killed"},
						"description": "Filter by status",
					},
				},
			},
			isEnabled:  true,
			isReadOnly: true,
		},
		taskCreateTool: taskCreateTool,
		taskManager:    taskManager,
	}
}

// Call lists all tasks.
func (t *TaskListTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Status string `json:"status,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	var result strings.Builder
	result.WriteString("Tasks:\n")

	for _, task := range t.taskCreateTool.listTasks() {
		if input.Status != "" && task.Status != input.Status {
			continue
		}

		statusIcon := "○"
		switch task.Status {
		case "in_progress":
			statusIcon = "◐"
		case "completed":
			statusIcon = "●"
		case "cancelled":
			statusIcon = "✗"
		}

		result.WriteString(fmt.Sprintf("  %s [#%s] %s\n", statusIcon, task.ID, task.Subject))
	}
	if t.taskManager != nil {
		executionTasks := t.taskManager.GetAllTasks()
		ids := make([]string, 0, len(executionTasks))
		for id := range executionTasks {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			task := executionTasks[id].GetBase()
			if input.Status != "" && string(task.Status) != input.Status {
				continue
			}
			statusIcon := "◐"
			if task.Status == tasks.TaskStatusCompleted {
				statusIcon = "●"
			} else if task.Status == tasks.TaskStatusFailed || task.Status == tasks.TaskStatusKilled {
				statusIcon = "✗"
			}
			result.WriteString(fmt.Sprintf("  %s [#%s] %s (%s)\n", statusIcon, task.ID, task.Description, task.Status))
		}
	}

	return &types.ToolResult{
		Output:    result.String(),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// TaskStopTool stops a running task.
type TaskStopTool struct {
	*BaseTool
	taskCreateTool *TaskCreateTool
	taskManager    *tasks.Manager
}

// NewTaskStopTool creates a new task stop tool.
func NewTaskStopTool(taskCreateTool *TaskCreateTool, managers ...*tasks.Manager) *TaskStopTool {
	var taskManager *tasks.Manager
	if len(managers) > 0 {
		taskManager = managers[0]
	}
	return &TaskStopTool{
		BaseTool: &BaseTool{
			name:        constants.ToolTaskStop,
			description: "Stop a running task",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"task_id": {
						"type":        "string",
						"description": "The ID of the task to stop",
					},
				},
				Required: []string{"task_id"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
		taskCreateTool: taskCreateTool,
		taskManager:    taskManager,
	}
}

// Call stops a task.
func (t *TaskStopTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	input.TaskID = strings.TrimSpace(input.TaskID)
	if input.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	task, err := t.taskCreateTool.updateTask(input.TaskID, func(task *Task) error {
		if task.Status == "completed" || task.Status == "cancelled" {
			return fmt.Errorf("task %s is already %s", task.ID, task.Status)
		}
		task.Status = "cancelled"
		return nil
	})
	if err != nil {
		if t.taskManager != nil && t.taskManager.GetTask(input.TaskID) != nil {
			if killErr := t.taskManager.KillTask(input.TaskID); killErr != nil {
				return &types.ToolResult{Error: killErr, ToolUseID: toolCtx.ToolUseId}, nil
			}
			return &types.ToolResult{Output: fmt.Sprintf("Task #%s stopped", input.TaskID), ToolUseID: toolCtx.ToolUseId}, nil
		}
		return &types.ToolResult{
			Error:     err,
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Task #%s stopped", task.ID),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// TaskGetTool gets details of a specific task.
type TaskGetTool struct {
	*BaseTool
	taskCreateTool *TaskCreateTool
	taskManager    *tasks.Manager
}

// NewTaskGetTool creates a new task get tool.
func NewTaskGetTool(taskCreateTool *TaskCreateTool, managers ...*tasks.Manager) *TaskGetTool {
	var taskManager *tasks.Manager
	if len(managers) > 0 {
		taskManager = managers[0]
	}
	return &TaskGetTool{
		BaseTool: &BaseTool{
			name:        "TaskGet",
			description: "Get details of a specific task",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"task_id": {
						"type":        "string",
						"description": "The ID of the task to get",
					},
				},
				Required: []string{"task_id"},
			},
			isEnabled:  true,
			isReadOnly: true,
		},
		taskCreateTool: taskCreateTool,
		taskManager:    taskManager,
	}
}

// Call gets task details.
func (t *TaskGetTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}
	input.TaskID = strings.TrimSpace(input.TaskID)
	if input.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	task, ok := t.taskCreateTool.GetTask(input.TaskID)
	if !ok {
		if t.taskManager != nil {
			if executionTask := t.taskManager.GetTask(input.TaskID); executionTask != nil {
				base := executionTask.GetBase()
				result := fmt.Sprintf("Task #%s\n  Status: %s\n  Description: %s\n  Created: %s\n", base.ID, base.Status, base.Description, base.StartTime.Format(time.RFC3339))
				if base.EndTime != nil {
					result += fmt.Sprintf("  Finished: %s\n", base.EndTime.Format(time.RFC3339))
				}
				return &types.ToolResult{Output: result, ToolUseID: toolCtx.ToolUseId}, nil
			}
		}
		return &types.ToolResult{
			Error:     fmt.Errorf("task not found: %s", input.TaskID),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	result := fmt.Sprintf("Task #%s\n", task.ID)
	result += fmt.Sprintf("  Subject: %s\n", task.Subject)
	result += fmt.Sprintf("  Status: %s\n", task.Status)
	result += fmt.Sprintf("  Description: %s\n", task.Description)
	if task.ActiveForm != "" {
		result += fmt.Sprintf("  Active Form: %s\n", task.ActiveForm)
	}
	result += fmt.Sprintf("  Created: %s\n", task.CreatedAt.Format(time.RFC3339))
	result += fmt.Sprintf("  Updated: %s\n", task.UpdatedAt.Format(time.RFC3339))

	return &types.ToolResult{
		Output:    result,
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// TaskUpdateTool updates a task.
type TaskUpdateTool struct {
	*BaseTool
	taskCreateTool *TaskCreateTool
}

// NewTaskUpdateTool creates a new task update tool.
func NewTaskUpdateTool(taskCreateTool *TaskCreateTool) *TaskUpdateTool {
	return &TaskUpdateTool{
		BaseTool: &BaseTool{
			name:        "TaskUpdate",
			description: "Update a task's status or properties",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"task_id": {
						"type":        "string",
						"description": "The ID of the task to update",
					},
					"status": {
						"type":        "string",
						"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
						"description": "New status for the task",
					},
					"subject": {
						"type":        "string",
						"description": "New subject for the task",
					},
					"description": {
						"type":        "string",
						"description": "New description for the task",
					},
				},
				Required: []string{"task_id"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
		taskCreateTool: taskCreateTool,
	}
}

// Call updates a task.
func (t *TaskUpdateTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		TaskID      string `json:"task_id"`
		Status      string `json:"status,omitempty"`
		Subject     string `json:"subject,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	if input.Status == "" && input.Subject == "" && input.Description == "" {
		return nil, fmt.Errorf("at least one field must be provided")
	}
	_, err := t.taskCreateTool.updateTask(input.TaskID, func(task *Task) error {
		if input.Status != "" {
			task.Status = input.Status
		}
		if input.Subject != "" {
			task.Subject = input.Subject
		}
		if input.Description != "" {
			task.Description = input.Description
		}
		return nil
	})
	if err != nil {
		return &types.ToolResult{
			Error:     err,
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Task #%s updated successfully", input.TaskID),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// TaskOutputTool retrieves output from running or completed tasks.
type TaskOutputTool struct {
	*BaseTool
	taskManager *tasks.Manager
}

// SetTaskManager sets the task manager for the tool.
func (t *TaskOutputTool) SetTaskManager(mgr *tasks.Manager) {
	t.taskManager = mgr
}

// NewTaskOutputTool creates a new Task Output tool.
func NewTaskOutputTool() *TaskOutputTool {
	return &TaskOutputTool{
		BaseTool: &BaseTool{
			name:        "TaskOutput",
			aliases:     []string{"AgentOutputTool", "BashOutputTool"},
			description: "[Deprecated] Retrieve output from a background task",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"task_id": {
						"type":        "string",
						"description": "The task ID to get output from",
					},
					"block": {
						"type":        "boolean",
						"default":     true,
						"description": "Whether to wait for completion",
					},
					"timeout": {
						"type":        "number",
						"default":     30000,
						"description": "Max wait time in ms",
					},
				},
				Required: []string{"task_id"},
			},
			isEnabled:  true,
			isReadOnly: true,
		},
	}
}

// Call retrieves task output.
func (t *TaskOutputTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		TaskID  string `json:"task_id"`
		Block   bool   `json:"block"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	if input.TaskID == "" {
		return nil, fmt.Errorf("task ID is required")
	}

	if input.Timeout == 0 {
		input.Timeout = 30000
	}

	if t.taskManager != nil {
		task := t.taskManager.GetTask(input.TaskID)
		if task == nil {
			return nil, fmt.Errorf("task %s not found", input.TaskID)
		}

		base := task.GetBase()
		if input.Block && (base.Status == tasks.TaskStatusPending || base.Status == tasks.TaskStatusRunning) {
			timeout := time.Duration(input.Timeout) * time.Millisecond
			deadline := time.Now().Add(timeout)

			for time.Now().Before(deadline) {
				task = t.taskManager.GetTask(input.TaskID)
				if task == nil {
					return nil, fmt.Errorf("task %s disappeared", input.TaskID)
				}

				base = task.GetBase()
				if base.Status != tasks.TaskStatusPending && base.Status != tasks.TaskStatusRunning {
					break
				}

				time.Sleep(100 * time.Millisecond)
			}
		}

		switch tt := task.(type) {
		case *tasks.LocalAgentTaskState:
			if tt.Error != "" {
				return &types.ToolResult{
					Output: fmt.Sprintf("Task failed: %s", tt.Error),
					Error:  fmt.Errorf("task failed: %s", tt.Error),
				}, nil
			}
			if tt.Result != nil {
				return &types.ToolResult{
					Output: fmt.Sprintf("%v", tt.Result),
				}, nil
			}
			output, err := tasks.ReadTaskOutput(input.TaskID)
			if err != nil {
				return &types.ToolResult{
					Output: fmt.Sprintf("Task %s completed (no output available)", input.TaskID),
				}, nil
			}
			return &types.ToolResult{Output: output}, nil

		case *tasks.LocalShellTaskState:
			if tt.Error != "" {
				return &types.ToolResult{
					Output: fmt.Sprintf("Shell task failed: %s", tt.Error),
					Error:  fmt.Errorf("shell task failed: %s", tt.Error),
				}, nil
			}
			output, err := tasks.ReadTaskOutput(input.TaskID)
			if err != nil {
				exitCodeStr := ""
				if tt.ExitCode != nil {
					exitCodeStr = fmt.Sprintf(" (exit code: %d)", *tt.ExitCode)
				}
				return &types.ToolResult{
					Output: fmt.Sprintf("Shell task completed%s", exitCodeStr),
				}, nil
			}
			return &types.ToolResult{Output: output}, nil

		case *tasks.RemoteAgentTaskState:
			if tt.Error != "" {
				return &types.ToolResult{
					Output: fmt.Sprintf("Remote agent failed: %s", tt.Error),
					Error:  fmt.Errorf("remote agent failed: %s", tt.Error),
				}, nil
			}
			return &types.ToolResult{
				Output: fmt.Sprintf("Remote agent task %s completed", input.TaskID),
			}, nil

		default:
			return &types.ToolResult{
				Output: fmt.Sprintf("Task %s status: %s", input.TaskID, base.Status),
			}, nil
		}
	}

	return &types.ToolResult{
		Output: fmt.Sprintf("Task %s output placeholder", input.TaskID),
	}, nil
}

// UserFacingName returns the user-facing name.
func (t *TaskOutputTool) UserFacingName(input json.RawMessage) string {
	return "Task Output"
}

// Description returns the tool description.
func (t *TaskOutputTool) Description(ctx context.Context, input json.RawMessage, options types.ToolOptions) (string, error) {
	return "[Deprecated] — prefer Read on the task output file path", nil
}

// =============================================================================
// MCP Tool
// =============================================================================

// MCPTool is a placeholder for MCP (Model Context Protocol) tools.
// The actual implementation is in mcpClient.go which dynamically creates tools.
type MCPTool struct {
	*BaseTool
}

// NewMCPTool creates a new MCP tool placeholder.
func NewMCPTool() *MCPTool {
	return &MCPTool{
		BaseTool: &BaseTool{
			name:        "mcp",
			description: "Execute MCP (Model Context Protocol) tools",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
			},
			isEnabled:  true,
			isReadOnly: false,
		},
	}
}

// Call executes the MCP tool (overridden in mcpClient).
func (t *MCPTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	return &types.ToolResult{
		Output: "",
	}, nil
}

// CheckPermissions returns passthrough permission (overridden in mcpClient).
func (t *MCPTool) CheckPermissions(ctx context.Context, input json.RawMessage, context *types.ToolContext) (*types.PermissionResult, error) {
	return &types.PermissionResult{
		Behavior: types.PermissionBehavior("passthrough"),
		Message:  "MCPTool requires permission.",
	}, nil
}

// UserFacingName returns the user-facing name.
func (t *MCPTool) UserFacingName(input json.RawMessage) string {
	return "mcp"
}

// IsMCP returns true for MCP tools.
func (t *MCPTool) IsMCP() bool {
	return true
}

// =============================================================================
// MCP Resource Tools
// =============================================================================

// ListMcpResourcesTool lists resources from MCP servers.
type ListMcpResourcesTool struct {
	*BaseTool
}

// NewListMcpResourcesTool creates a new list MCP resources tool.
func NewListMcpResourcesTool() *ListMcpResourcesTool {
	return &ListMcpResourcesTool{
		BaseTool: &BaseTool{
			name:        constants.ToolListMcpResources,
			description: "List resources available from MCP servers",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"server_name": {
						"type":        "string",
						"description": "The name of the MCP server to list resources from",
					},
				},
			},
			isEnabled:  false,
			isReadOnly: true,
		},
	}
}

// Call lists MCP resources.
func (t *ListMcpResourcesTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		ServerName string `json:"server_name,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Placeholder - would integrate with MCP client
	return &types.ToolResult{
		Output:    fmt.Sprintf("MCP resources listed for server: %s", input.ServerName),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// ReadMcpResourceTool reads a resource from an MCP server.
type ReadMcpResourceTool struct {
	*BaseTool
}

// NewReadMcpResourceTool creates a new read MCP resource tool.
func NewReadMcpResourceTool() *ReadMcpResourceTool {
	return &ReadMcpResourceTool{
		BaseTool: &BaseTool{
			name:        constants.ToolReadMcpResource,
			description: "Read a specific resource from an MCP server",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"uri": {
						"type":        "string",
						"description": "The URI of the resource to read",
					},
				},
				Required: []string{"uri"},
			},
			isEnabled:  false,
			isReadOnly: true,
		},
	}
}

// Call reads an MCP resource.
func (t *ReadMcpResourceTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Placeholder - would integrate with MCP client
	return &types.ToolResult{
		Output:    fmt.Sprintf("MCP resource read: %s", input.URI),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// MCP Auth Tool
// =============================================================================

// McpAuthTool handles MCP server authentication.
type McpAuthTool struct {
	*BaseTool
}

// NewMcpAuthTool creates a new MCP auth tool.
func NewMcpAuthTool() *McpAuthTool {
	return &McpAuthTool{
		BaseTool: &BaseTool{
			name:        "McpAuth",
			description: "Authenticate with MCP servers",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"server_name": {
						"type":        "string",
						"description": "The name of the MCP server to authenticate with",
					},
					"action": {
						"type":        "string",
						"enum":        []string{"login", "logout", "status"},
						"description": "The authentication action to perform",
					},
				},
				Required: []string{"server_name", "action"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
	}
}

// Call handles MCP authentication.
func (t *McpAuthTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		ServerName string `json:"server_name"`
		Action     string `json:"action"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	switch input.Action {
	case "login":
		return &types.ToolResult{
			Output:    fmt.Sprintf("Initiating authentication for MCP server: %s", input.ServerName),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "logout":
		return &types.ToolResult{
			Output:    fmt.Sprintf("Logged out from MCP server: %s", input.ServerName),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "status":
		return &types.ToolResult{
			Output:    fmt.Sprintf("Authentication status for MCP server '%s': not authenticated", input.ServerName),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	default:
		return &types.ToolResult{
			Error:     fmt.Errorf("unknown action: %s", input.Action),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}
}

// =============================================================================
// Notebook Edit Tool
// =============================================================================

// NotebookEditTool edits Jupyter notebooks.
type NotebookEditTool struct {
	*BaseTool
}

// NewNotebookEditTool creates a new notebook edit tool.
func NewNotebookEditTool() *NotebookEditTool {
	return &NotebookEditTool{
		BaseTool: &BaseTool{
			name:        constants.ToolNotebookEdit,
			description: "Edit Jupyter notebook cells",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"notebook_path": {
						"type":        "string",
						"description": "The path to the notebook file",
					},
					"cell_number": {
						"type":        "number",
						"description": "The cell number to edit",
					},
					"new_source": {
						"type":        "string",
						"description": "The new source for the cell",
					},
				},
				Required: []string{"notebook_path", "cell_number", "new_source"},
			},
			isEnabled:     false,
			isReadOnly:    false,
			isDestructive: true,
		},
	}
}

// Call edits a notebook cell.
func (t *NotebookEditTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		NotebookPath string `json:"notebook_path"`
		CellNumber   int    `json:"cell_number"`
		NewSource    string `json:"new_source"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Check if notebook exists
	if _, err := os.Stat(input.NotebookPath); os.IsNotExist(err) {
		return &types.ToolResult{
			Error:     fmt.Errorf("notebook does not exist: %s", input.NotebookPath),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// Placeholder - would parse and edit the notebook JSON
	return &types.ToolResult{
		Output:    fmt.Sprintf("Notebook cell %d edited in %s", input.CellNumber, input.NotebookPath),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Config Tool
// =============================================================================

// ConfigTool manages configuration settings.
type ConfigTool struct {
	*BaseTool
}

// NewConfigTool creates a new config tool.
func NewConfigTool() *ConfigTool {
	return &ConfigTool{
		BaseTool: &BaseTool{
			name:        constants.ToolConfig,
			description: "View and modify configuration settings",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"action": {
						"type":        "string",
						"enum":        []string{"get", "set", "list", "reset"},
						"description": "The action to perform",
					},
					"key": {
						"type":        "string",
						"description": "The configuration key",
					},
					"value": {
						"description": "The value to set",
					},
				},
				Required: []string{"action"},
			},
			isEnabled:  false,
			isReadOnly: false,
		},
	}
}

// Call manages configuration.
func (t *ConfigTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Action string      `json:"action"`
		Key    string      `json:"key,omitempty"`
		Value  interface{} `json:"value,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	switch input.Action {
	case "list":
		return &types.ToolResult{
			Output:    "Configuration settings listed",
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "get":
		return &types.ToolResult{
			Output:    fmt.Sprintf("Configuration value for '%s'", input.Key),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "set":
		return &types.ToolResult{
			Output:    fmt.Sprintf("Configuration '%s' set to: %v", input.Key, input.Value),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "reset":
		return &types.ToolResult{
			Output:    fmt.Sprintf("Configuration '%s' reset to default", input.Key),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	default:
		return &types.ToolResult{
			Error:     fmt.Errorf("unknown action: %s", input.Action),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}
}

// =============================================================================
// Agent Tool
// =============================================================================

// AgentTool spawns and manages sub-agents.
type AgentTool struct {
	*BaseTool
	taskManager *tasks.Manager
	runner      AgentRunner
}

// AgentRunner executes one isolated sub-agent query.
type AgentRunner func(context.Context, *tasks.LocalAgentTaskState, *types.ToolContext, func(interface{})) (interface{}, error)

// SetTaskManager sets the task manager for the agent tool.
func (t *AgentTool) SetTaskManager(mgr *tasks.Manager) {
	t.taskManager = mgr
}

// SetRunner configures the query implementation used by spawned agents.
func (t *AgentTool) SetRunner(runner AgentRunner) {
	t.runner = runner
}

// NewAgentTool creates a new Agent tool.
func NewAgentTool() *AgentTool {
	return &AgentTool{
		BaseTool: &BaseTool{
			name:        constants.ToolAgent,
			aliases:     []string{"Task"},
			description: "Spawn a sub-agent to handle complex, multi-step tasks autonomously",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"description": {
						"type":        "string",
						"description": "A short (3-5 word) description of the task",
					},
					"prompt": {
						"type":        "string",
						"description": "The task for the agent to perform",
					},
					"subagent_type": {
						"type":        "string",
						"description": "The type of specialized agent to use for this task",
					},
					"model": {
						"type":        "string",
						"enum":        []string{"sonnet", "opus", "haiku"},
						"description": "Optional model override for this agent",
					},
					"run_in_background": {
						"type":        "boolean",
						"description": "Set to true to run this agent in the background",
					},
				},
				Required: []string{"description", "prompt"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
	}
}

// Call spawns a sub-agent.
func (t *AgentTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Description     string `json:"description"`
		Prompt          string `json:"prompt"`
		SubagentType    string `json:"subagent_type,omitempty"`
		Model           string `json:"model,omitempty"`
		RunInBackground bool   `json:"run_in_background,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}
	input.Description = strings.TrimSpace(input.Description)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.Description == "" || input.Prompt == "" {
		return nil, fmt.Errorf("description and prompt are required")
	}

	// Determine agent type
	agentType := input.SubagentType
	if agentType == "" {
		agentType = "general-agent"
	}

	if t.taskManager == nil || t.runner == nil {
		return nil, fmt.Errorf("agent runtime is not configured")
	}

	task, err := t.taskManager.SpawnLocalAgent(ctx, input.Prompt, agentType, input.Description, input.RunInBackground)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn agent task: %w", err)
	}
	if err := t.taskManager.SetAgentModel(task.ID, input.Model); err != nil {
		return nil, err
	}
	task.Model = input.Model
	executionCtx := ctx
	if input.RunInBackground {
		executionCtx = context.WithoutCancel(ctx)
	}
	if err := t.taskManager.StartExecution(executionCtx, task.ID, func(runCtx context.Context, state *tasks.LocalAgentTaskState) (interface{}, error) {
		return t.runner(runCtx, state, toolCtx, onProgress)
	}); err != nil {
		return nil, fmt.Errorf("failed to start agent task: %w", err)
	}

	toolUseID := ""
	if toolCtx != nil {
		toolUseID = toolCtx.ToolUseId
	}
	if input.RunInBackground {
		return &types.ToolResult{
			Output:    fmt.Sprintf("Agent task '%s' started in background.\nTask ID: %s", input.Description, task.ID),
			ToolUseID: toolUseID,
		}, nil
	}

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		current := t.taskManager.GetTask(task.ID)
		if current == nil {
			return nil, fmt.Errorf("agent task %s disappeared", task.ID)
		}
		state := current.(*tasks.LocalAgentTaskState)
		switch state.Status {
		case tasks.TaskStatusCompleted:
			return &types.ToolResult{Output: state.Result, ToolUseID: toolUseID}, nil
		case tasks.TaskStatusFailed:
			return &types.ToolResult{Output: state.Error, Error: fmt.Errorf("agent task failed: %s", state.Error), ToolUseID: toolUseID}, nil
		case tasks.TaskStatusKilled:
			return &types.ToolResult{Output: "Agent task was stopped", Error: context.Canceled, ToolUseID: toolUseID}, nil
		}
		select {
		case <-ctx.Done():
			_ = t.taskManager.KillTask(task.ID)
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// CheckPermissions checks agent permissions.
func (t *AgentTool) CheckPermissions(ctx context.Context, input json.RawMessage, context *types.ToolContext) (*types.PermissionResult, error) {
	return &types.PermissionResult{
		Behavior: types.PermissionBehaviorAllow,
	}, nil
}

// UserFacingName returns the user-facing name.
func (t *AgentTool) UserFacingName(input json.RawMessage) string {
	var inputData struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &inputData); err == nil && inputData.Description != "" {
		return inputData.Description
	}
	return "Agent"
}

// =============================================================================
// REPL Tool
// =============================================================================

// REPLTool manages REPL primitive tools.
type REPLTool struct {
	*BaseTool
	primitiveTools []types.Tool
}

// NewREPLTool creates a new REPL tool manager.
func NewREPLTool() *REPLTool {
	return &REPLTool{
		BaseTool: &BaseTool{
			name:        "repl",
			description: "REPL primitive tools management",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
			},
			isEnabled:  true,
			isReadOnly: true,
		},
	}
}

// GetPrimitiveTools returns the list of REPL primitive tools.
func (t *REPLTool) GetPrimitiveTools() []types.Tool {
	if t.primitiveTools == nil {
		t.primitiveTools = []types.Tool{
			NewFileReadTool(),
			NewFileWriteTool(),
			NewFileEditTool(),
			NewGlobTool(),
			NewGrepTool(),
			NewBashTool(),
			NewNotebookEditTool(),
			NewAgentTool(),
		}
	}
	return t.primitiveTools
}

// Call is not implemented for REPL tool.
func (t *REPLTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	return nil, fmt.Errorf("REPLTool is not directly callable")
}

// =============================================================================
// Helper Functions
// =============================================================================

var taskCounter int
var taskCounterMu sync.Mutex

func generateTaskID() string {
	taskCounterMu.Lock()
	defer taskCounterMu.Unlock()
	taskCounter++
	return fmt.Sprintf("%d", taskCounter)
}

// DetectLanguageFromExtension returns the language name from a file extension.
func DetectLanguageFromExtension(ext string) string {
	languageMap := map[string]string{
		".go":    "go",
		".ts":    "typescript",
		".tsx":   "typescriptreact",
		".js":    "javascript",
		".jsx":   "javascriptreact",
		".py":    "python",
		".rs":    "rust",
		".java":  "java",
		".c":     "c",
		".cpp":   "cpp",
		".h":     "c",
		".hpp":   "cpp",
		".cs":    "csharp",
		".rb":    "ruby",
		".php":   "php",
		".swift": "swift",
		".kt":    "kotlin",
		".scala": "scala",
		".json":  "json",
		".yaml":  "yaml",
		".yml":   "yaml",
		".md":    "markdown",
		".html":  "html",
		".css":   "css",
		".scss":  "scss",
		".less":  "less",
		".sql":   "sql",
		".sh":    "bash",
		".zsh":   "zsh",
	}

	if lang, ok := languageMap[strings.ToLower(ext)]; ok {
		return lang
	}
	return "plaintext"
}

// HTMLEntitiesToText converts HTML entities to plain text.
func HTMLEntitiesToText(html string) string {
	// Decode common HTML entities
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// Remove remaining HTML tags
	re := regexp.MustCompile(`<[^>]+>`)
	return re.ReplaceAllString(html, "")
}
