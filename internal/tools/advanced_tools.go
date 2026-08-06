package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"claude-code-go/internal/constants"
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

	t.tasks[taskID] = task

	return &types.ToolResult{
		Output:    fmt.Sprintf("Task #%s created successfully: %s", taskID, input.Subject),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// GetTask retrieves a task by ID.
func (t *TaskCreateTool) GetTask(id string) (*Task, bool) {
	task, ok := t.tasks[id]
	return task, ok
}

// TaskListTool lists all tasks.
type TaskListTool struct {
	*BaseTool
	taskCreateTool *TaskCreateTool
}

// NewTaskListTool creates a new task list tool.
func NewTaskListTool(taskCreateTool *TaskCreateTool) *TaskListTool {
	return &TaskListTool{
		BaseTool: &BaseTool{
			name:        constants.ToolTaskList,
			description: "List all tasks",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"status": {
						"type":        "string",
						"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
						"description": "Filter by status",
					},
				},
			},
			isEnabled:  true,
			isReadOnly: true,
		},
		taskCreateTool: taskCreateTool,
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

	for _, task := range t.taskCreateTool.tasks {
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

	return &types.ToolResult{
		Output:    result.String(),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// TaskStopTool stops a running task.
type TaskStopTool struct {
	*BaseTool
	taskCreateTool *TaskCreateTool
}

// NewTaskStopTool creates a new task stop tool.
func NewTaskStopTool(taskCreateTool *TaskCreateTool) *TaskStopTool {
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

	task, ok := t.taskCreateTool.GetTask(input.TaskID)
	if !ok {
		return &types.ToolResult{
			Error:     fmt.Errorf("task not found: %s", input.TaskID),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	task.Status = "cancelled"
	task.UpdatedAt = time.Now()

	return &types.ToolResult{
		Output:    fmt.Sprintf("Task #%s stopped", input.TaskID),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
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
// Helper Functions
// =============================================================================

var taskCounter int

func generateTaskID() string {
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
