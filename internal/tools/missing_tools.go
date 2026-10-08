package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"claude-code-go/internal/constants"
	"claude-code-go/internal/tasks"
	"claude-code-go/internal/types"
)

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
// Task Output Tool
// =============================================================================

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

	// Default timeout
	if input.Timeout == 0 {
		input.Timeout = 30000
	}

	// If task manager is available, use it
	if t.taskManager != nil {
		task := t.taskManager.GetTask(input.TaskID)
		if task == nil {
			return nil, fmt.Errorf("task %s not found", input.TaskID)
		}

		base := task.GetBase()

		// If blocking and task is still running, wait for completion
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

		// Return output based on task type
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
			// Try to read from output file
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

	// Fallback: placeholder response
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
