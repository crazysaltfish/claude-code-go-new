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
