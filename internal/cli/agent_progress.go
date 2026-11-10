package cli

import (
	"context"
	"encoding/json"
	"sync"

	"claude-code-go/internal/query"
	"claude-code-go/internal/tasks"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

// Progress belongs to the task manager, never to the parent query's stream.
type agentProgress struct {
	mu      sync.Mutex
	manager *tasks.Manager
	id      string
	state   tasks.AgentProgress
}

func newAgentProgress(manager *tasks.Manager, id string) *agentProgress {
	p := &agentProgress{manager: manager, id: id}
	p.summary("Waiting for model")
	return p
}

func (p *agentProgress) summary(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.Summary = text
	_ = p.manager.UpdateProgress(p.id, p.state)
}

func (p *agentProgress) permissionCheck(check types.CanUseToolFunc) types.CanUseToolFunc {
	return func(ctx context.Context, name string, input json.RawMessage) (*types.PermissionDecision, error) {
		p.summary("Checking permission: " + name)
		decision, err := check(ctx, name, input)
		if err == nil && decision != nil && decision.Behavior == types.PermissionBehaviorAllow {
			p.summary("Running: " + name)
		} else {
			p.summary("Permission denied: " + name)
		}
		return decision, err
	}
}

func (p *agentProgress) record(message interface{}) {
	sdk, ok := message.(query.SDKMessage)
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch sdk.Type {
	case "assistant_delta":
		p.state.Summary = "Generating response"
	case "assistant":
		response, ok := sdk.Message.(*api.MessageResponse)
		if !ok {
			return
		}
		for _, block := range response.Content {
			if block.Type != "tool_use" {
				continue
			}
			activity := tasks.ToolActivity{ToolName: block.Name}
			_ = json.Unmarshal(block.Input, &activity.Input)
			for _, key := range []string{"description", "file_path", "target_file", "path", "command", "pattern", "query"} {
				if value, ok := activity.Input[key].(string); ok && value != "" {
					activity.ActivityDescription = value
					break
				}
			}
			p.state.ToolUseCount++
			p.state.LastActivity = &activity
			p.state.RecentActivities = append(p.state.RecentActivities, activity)
			if len(p.state.RecentActivities) > 5 {
				p.state.RecentActivities = p.state.RecentActivities[1:]
			}
			p.state.Summary = "Running: " + block.Name
		}
	case "tool_result":
		p.state.Summary = "Waiting for model"
	default:
		return
	}
	_ = p.manager.UpdateProgress(p.id, p.state)
}
