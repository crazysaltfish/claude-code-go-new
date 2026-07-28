package query

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"claude-code-go/internal/constants"
	"claude-code-go/internal/types"
	"claude-code-go/pkg/api"
)

// QueryEngineConfig contains configuration for the QueryEngine.
type QueryEngineConfig struct {
	SessionID          string
	Cwd                string
	Tools              []types.Tool
	Commands           []types.Command
	MCPClients         []types.MCPServerConnection
	CanUseTool         types.CanUseToolFunc
	GetAppState        func() *types.AppState
	SetAppState        func(func(*types.AppState) *types.AppState)
	InitialMessages    []types.Message
	ReadFileCache      types.FileStateCache
	CustomSystemPrompt string
	AppendSystemPrompt string
	UserSpecifiedModel string
	FallbackModel      string
	ThinkingConfig     *types.ThinkingConfig
	MaxTurns           int
	MaxBudgetUsd       float64
	Verbose            bool
	AbortController    *types.AbortController
	APIClient          api.MessageClient
}

// QueryEngine owns the query lifecycle and session state for a conversation.
// It extracts the core logic from ask() into a standalone class that can be
// used by both the headless/SDK path and the REPL.
type QueryEngine struct {
	config             QueryEngineConfig
	mutableMessages    []types.Message
	abortController    *types.AbortController
	totalUsage         Usage
	apiDuration        time.Duration
	hasHandledOrphaned bool
	readFileState      types.FileStateCache
	mu                 sync.RWMutex
	sessionID          string
	startTime          time.Time
}

// Usage tracks API usage.
type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// SDKMessage represents a message sent to SDK consumers.
type SDKMessage struct {
	Type      string      `json:"type"`
	Message   interface{} `json:"message,omitempty"`
	SessionID string      `json:"session_id"`
	UUID      string      `json:"uuid"`
	Timestamp int64       `json:"timestamp"`
}

// AssistantDelta is emitted while an assistant response is still streaming.
// Text and thinking are separated so UI consumers can choose whether to
// display model reasoning.
type AssistantDelta struct {
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

// ResultMessage represents the final result of a query.
type ResultMessage struct {
	Type          string  `json:"type"`
	Subtype       string  `json:"subtype"`
	IsError       bool    `json:"is_error"`
	DurationMs    int64   `json:"duration_ms"`
	DurationApiMs int64   `json:"duration_api_ms"`
	NumTurns      int     `json:"num_turns"`
	Result        string  `json:"result"`
	StopReason    string  `json:"stop_reason"`
	SessionID     string  `json:"session_id"`
	TotalCostUsd  float64 `json:"total_cost_usd"`
	Usage         Usage   `json:"usage"`
	FastModeState string  `json:"fast_mode_state"`
	UUID          string  `json:"uuid"`
}

// NewQueryEngine creates a new query engine.
func NewQueryEngine(config QueryEngineConfig) *QueryEngine {
	abortController := config.AbortController
	if abortController == nil {
		abortController = types.NewAbortController()
	}

	sessionID := config.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}
	return &QueryEngine{
		config:          config,
		mutableMessages: config.InitialMessages,
		abortController: abortController,
		totalUsage:      Usage{},
		readFileState:   config.ReadFileCache,
		sessionID:       sessionID,
	}
}

// SubmitMessage submits a message to the query engine and yields responses.
func (e *QueryEngine) SubmitMessage(ctx context.Context, prompt string) (<-chan interface{}, error) {
	// Create output channel
	output := make(chan interface{}, 100)

	go func() {
		defer close(output)
		e.mu.Lock()
		defer e.mu.Unlock()
		e.startTime = time.Now()
		e.apiDuration = 0
		e.totalUsage = Usage{}

		// Process user input and get messages
		userMessages := e.processUserInput(prompt)
		e.mutableMessages = append(e.mutableMessages, userMessages...)

		// Yield user message
		for _, msg := range userMessages {
			output <- SDKMessage{
				Type:      "user",
				Message:   msg,
				SessionID: e.sessionID,
				UUID:      generateUUID(),
				Timestamp: time.Now().UnixMilli(),
			}
		}

		// Check if we should query the API
		shouldQuery := e.shouldQueryAPI(prompt)
		if !shouldQuery {
			// Return early for slash commands that don't need API
			output <- e.createResultMessage(0, "", "")
			return
		}

		// Execute the query loop
		e.executeQueryLoop(ctx, output)
	}()

	return output, nil
}

// executeQueryLoop runs the main query loop.
func (e *QueryEngine) executeQueryLoop(ctx context.Context, output chan<- interface{}) {
	turnCount := 0
	maxTurns := e.config.MaxTurns
	if maxTurns == 0 {
		maxTurns = 100 // Default max turns
	}

	for turnCount < maxTurns {
		// Check for abort
		if e.abortController.IsAborted() {
			output <- SDKMessage{
				Type:      "system",
				Message:   map[string]string{"subtype": "interrupted"},
				SessionID: e.sessionID,
				UUID:      generateUUID(),
			}
			output <- e.createErrorResultMessage("interrupted", "operation interrupted", turnCount)
			return
		}

		turnCount++

		// Build system prompt
		systemPrompt := e.buildSystemPrompt()

		// Get the model to use
		model := e.getModel()

		// Create API request
		req := api.MessageRequest{
			Model:     model,
			MaxTokens: 4096,
			Messages:  e.convertMessagesForAPI(),
			System:    systemPrompt,
			Tools:     e.convertToolsForAPI(),
		}

		// Configure thinking if enabled
		if e.config.ThinkingConfig != nil && e.config.ThinkingConfig.Enabled {
			req.Thinking = &api.ThinkingConfig{
				Type:        e.config.ThinkingConfig.Type,
				BudgetToken: e.config.ThinkingConfig.BudgetToken,
			}
		}

		// Stream the API response. All deltas for this turn share a UUID so
		// consumers can update one in-progress assistant message in place.
		apiStarted := time.Now()
		responseUUID := generateUUID()
		response, err := e.callAPIStream(ctx, req, func(delta AssistantDelta) error {
			select {
			case output <- SDKMessage{
				Type:      "assistant_delta",
				Message:   delta,
				SessionID: e.sessionID,
				UUID:      responseUUID,
				Timestamp: time.Now().UnixMilli(),
			}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		e.apiDuration += time.Since(apiStarted)
		if err != nil {
			output <- SDKMessage{
				Type:      "system",
				Message:   map[string]string{"subtype": "error", "error": err.Error()},
				SessionID: e.sessionID,
				UUID:      generateUUID(),
			}
			output <- e.createErrorResultMessage("api_error", err.Error(), turnCount)
			return
		}

		// Update usage
		e.totalUsage.InputTokens += response.Usage.InputTokens
		e.totalUsage.OutputTokens += response.Usage.OutputTokens
		e.totalUsage.CacheCreationInputTokens += response.Usage.CacheCreationInputTokens
		e.totalUsage.CacheReadInputTokens += response.Usage.CacheReadInputTokens

		// Create assistant message
		assistantMsg := types.Message{
			Role:    "assistant",
			Content: mustMarshalJSON(response.Content),
		}
		e.mutableMessages = append(e.mutableMessages, assistantMsg)

		// Yield assistant message
		output <- SDKMessage{
			Type:      "assistant",
			Message:   response,
			SessionID: e.sessionID,
			UUID:      responseUUID,
			Timestamp: time.Now().UnixMilli(),
		}

		// Check for max_output_tokens error
		// Note: The full recovery logic for max_output_tokens is handled at a higher level
		// (similar to TypeScript's query.ts recovery loop). Here we just detect and signal.
		// TODO: Implement full recovery loop if needed for SDK consumers.

		// Check for tool use
		toolUseBlocks := e.extractToolUseBlocks(response)
		if len(toolUseBlocks) == 0 {
			// No tool use, we're done
			output <- e.createResultMessage(turnCount, response.StopReason, extractResponseText(response))
			return
		}

		// Execute tools
		toolResults := e.executeTools(ctx, toolUseBlocks, output)

		// Add tool results to messages
		for _, result := range toolResults {
			e.mutableMessages = append(e.mutableMessages, result)
		}

	}

	// Max turns reached
	output <- ResultMessage{
		Type:          "result",
		Subtype:       "max_turns_reached",
		IsError:       true,
		DurationMs:    time.Since(e.startTime).Milliseconds(),
		DurationApiMs: e.apiDuration.Milliseconds(),
		NumTurns:      turnCount,
		Result:        fmt.Sprintf("maximum turn limit reached (%d)", maxTurns),
		SessionID:     e.sessionID,
		TotalCostUsd:  e.calculateCost(),
		Usage:         e.totalUsage,
		FastModeState: "disabled",
		UUID:          generateUUID(),
	}
}

// processUserInput processes user input and returns messages.
func (e *QueryEngine) processUserInput(prompt string) []types.Message {
	return []types.Message{
		{
			Role:    "user",
			Content: mustMarshalJSON([]map[string]string{{"type": "text", "text": prompt}}),
		},
	}
}

// shouldQueryAPI determines if we should query the API.
func (e *QueryEngine) shouldQueryAPI(prompt string) bool {
	// Check for slash commands
	if len(prompt) > 0 && prompt[0] == '/' {
		// Some slash commands don't need API
		cmd := e.findCommand(prompt)
		if cmd != nil && cmd.Immediate {
			return false
		}
	}
	return true
}

// findCommand finds a command by name.
func (e *QueryEngine) findCommand(prompt string) *types.Command {
	for _, cmd := range e.config.Commands {
		if cmd.Name == prompt[1:] {
			return &cmd
		}
	}
	return nil
}

// buildSystemPrompt builds the system prompt.
func (e *QueryEngine) buildSystemPrompt() string {
	var prompt string

	if e.config.CustomSystemPrompt != "" {
		prompt = e.config.CustomSystemPrompt
	} else {
		// Build complete system prompt using the constants package
		cwd := e.config.Cwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}

		// Check if we're in a git repo
		isGit := e.isGitRepo(cwd)

		// Get shell info
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}

		// Get model ID
		modelId := e.getModel()

		// Build the complete system prompt
		prompt = constants.BuildSystemPrompt(
			cwd,
			isGit,
			runtime.GOOS,
			shell,
			getOSVersion(),
			modelId,
			nil, // additional working directories
			"",  // language preference
			"",  // scratchpad dir
		)
	}

	if e.config.AppendSystemPrompt != "" {
		prompt += "\n\n" + e.config.AppendSystemPrompt
	}

	return prompt
}

// isGitRepo checks if a directory is a git repository.
func (e *QueryEngine) isGitRepo(dir string) bool {
	gitDir := dir + "/.git"
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}
	// Check parent directories
	for dir != "/" {
		dir = dir[:len(dir)-1]
		if idx := len(dir) - 1; idx >= 0 {
			for idx >= 0 && dir[idx] != '/' {
				idx--
			}
			if idx > 0 {
				dir = dir[:idx]
			}
		}
		gitDir = dir + "/.git"
		if _, err := os.Stat(gitDir); err == nil {
			return true
		}
	}
	return false
}

// getOSVersion returns the OS version string.
func getOSVersion() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}

// getModel returns the model to use.
func (e *QueryEngine) getModel() string {
	if e.config.UserSpecifiedModel != "" {
		return e.config.UserSpecifiedModel
	}

	appState := e.config.GetAppState()
	if appState != nil && appState.MainLoopModel != "" {
		return appState.MainLoopModel
	}

	return "claude-sonnet-4-20250514" // Default model
}

// convertMessagesForAPI converts internal messages to API format.
func (e *QueryEngine) convertMessagesForAPI() []api.Message {
	messages := make([]api.Message, len(e.mutableMessages))
	for i, msg := range e.mutableMessages {
		var content []api.ContentBlock
		if err := json.Unmarshal(msg.Content, &content); err != nil {
			// Fallback to text content
			content = []api.ContentBlock{{
				Type: "text",
				Text: string(msg.Content),
			}}
		}
		messages[i] = api.Message{
			Role:    msg.Role,
			Content: content,
		}
	}
	return messages
}

// convertToolsForAPI converts internal tools to API format.
func (e *QueryEngine) convertToolsForAPI() []api.ToolDefinition {
	tools := make([]api.ToolDefinition, 0, len(e.config.Tools))
	for _, tool := range e.config.Tools {
		schema := tool.InputSchema()
		description, err := tool.Description(context.Background(), nil, types.ToolOptions{})
		if err != nil {
			description = tool.Name() // Fallback to tool name
		}
		tools = append(tools, api.ToolDefinition{
			Name:        tool.Name(),
			Description: description,
			InputSchema: map[string]interface{}{
				"type":       schema.Type,
				"properties": schema.Properties,
				"required":   schema.Required,
			},
		})
	}
	return tools
}

// callAPIStream streams the configured provider and assembles its canonical
// events into the same complete response used by the tool execution loop.
func (e *QueryEngine) callAPIStream(
	ctx context.Context,
	req api.MessageRequest,
	onDelta func(AssistantDelta) error,
) (*api.MessageResponse, error) {
	if e.config.APIClient == nil {
		return nil, fmt.Errorf("API client not configured")
	}

	builder := newStreamResponseBuilder(req.Model)
	eventCount := 0
	err := e.config.APIClient.StreamMessage(ctx, req, func(event api.StreamEvent) error {
		eventCount++
		return builder.apply(event, onDelta)
	})
	if err != nil {
		return nil, err
	}
	if eventCount == 0 {
		return nil, fmt.Errorf("API stream ended without any events")
	}
	return builder.response(), nil
}

type streamBlockBuilder struct {
	block        api.ContentBlock
	partialInput strings.Builder
}

type streamResponseBuilder struct {
	result api.MessageResponse
	blocks map[int]*streamBlockBuilder
}

func newStreamResponseBuilder(model string) *streamResponseBuilder {
	return &streamResponseBuilder{
		result: api.MessageResponse{
			Type:  "message",
			Role:  "assistant",
			Model: model,
		},
		blocks: make(map[int]*streamBlockBuilder),
	}
}

func (b *streamResponseBuilder) apply(event api.StreamEvent, onDelta func(AssistantDelta) error) error {
	if event.Message != nil {
		b.mergeMessage(*event.Message)
	}
	if event.Usage != nil {
		mergeStreamUsage(&b.result.Usage, *event.Usage)
	}

	switch event.Type {
	case "content_block_start":
		if event.ContentBlock == nil {
			return nil
		}
		block := *event.ContentBlock
		b.blocks[event.Index] = &streamBlockBuilder{block: block}
		if block.Type == "text" && block.Text != "" && onDelta != nil {
			return onDelta(AssistantDelta{Text: block.Text})
		}
		if block.Type == "thinking" && block.Thinking != "" && onDelta != nil {
			return onDelta(AssistantDelta{Thinking: block.Thinking})
		}

	case "content_block_delta":
		if event.Delta == nil {
			return nil
		}
		block := b.ensureBlock(event.Index, event.Delta)
		switch event.Delta.Type {
		case "text_delta":
			block.block.Text += event.Delta.Text
			if event.Delta.Text != "" && onDelta != nil {
				return onDelta(AssistantDelta{Text: event.Delta.Text})
			}
		case "thinking_delta":
			block.block.Thinking += event.Delta.Thinking
			if event.Delta.Thinking != "" && onDelta != nil {
				return onDelta(AssistantDelta{Thinking: event.Delta.Thinking})
			}
		case "input_json_delta":
			block.partialInput.WriteString(event.Delta.PartialJSON)
		}

	case "message_delta":
		if event.Delta != nil && event.Delta.StopReason != "" {
			b.result.StopReason = event.Delta.StopReason
		}
	}
	return nil
}

func (b *streamResponseBuilder) ensureBlock(index int, delta *api.EventDelta) *streamBlockBuilder {
	if block, ok := b.blocks[index]; ok {
		return block
	}
	blockType := "text"
	if delta != nil {
		switch delta.Type {
		case "thinking_delta":
			blockType = "thinking"
		case "input_json_delta":
			blockType = "tool_use"
		}
	}
	block := &streamBlockBuilder{block: api.ContentBlock{Type: blockType}}
	b.blocks[index] = block
	return block
}

func (b *streamResponseBuilder) mergeMessage(message api.MessageResponse) {
	if message.ID != "" {
		b.result.ID = message.ID
	}
	if message.Type != "" {
		b.result.Type = message.Type
	}
	if message.Role != "" {
		b.result.Role = message.Role
	}
	if message.Model != "" {
		b.result.Model = message.Model
	}
	if message.StopReason != "" {
		b.result.StopReason = message.StopReason
	}
	if message.StopSequence != "" {
		b.result.StopSequence = message.StopSequence
	}
	mergeStreamUsage(&b.result.Usage, message.Usage)
}

func mergeStreamUsage(target *api.Usage, update api.Usage) {
	// Streaming providers report cumulative counters at different points in
	// the stream, so the latest non-zero value replaces rather than adds.
	if update.InputTokens != 0 {
		target.InputTokens = update.InputTokens
	}
	if update.OutputTokens != 0 {
		target.OutputTokens = update.OutputTokens
	}
	if update.CacheCreationInputTokens != 0 {
		target.CacheCreationInputTokens = update.CacheCreationInputTokens
	}
	if update.CacheReadInputTokens != 0 {
		target.CacheReadInputTokens = update.CacheReadInputTokens
	}
}

func (b *streamResponseBuilder) response() *api.MessageResponse {
	indexes := make([]int, 0, len(b.blocks))
	for index := range b.blocks {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	b.result.Content = make([]api.ContentBlock, 0, len(indexes))
	for _, index := range indexes {
		part := b.blocks[index]
		if part.block.Type == "tool_use" {
			if input := part.partialInput.String(); input != "" {
				part.block.Input = json.RawMessage(input)
			} else if len(part.block.Input) == 0 {
				part.block.Input = json.RawMessage(`{}`)
			}
		}
		b.result.Content = append(b.result.Content, part.block)
	}
	return &b.result
}

// extractToolUseBlocks extracts tool use blocks from a response.
func (e *QueryEngine) extractToolUseBlocks(response *api.MessageResponse) []api.ContentBlock {
	var blocks []api.ContentBlock
	for _, block := range response.Content {
		if block.Type == "tool_use" {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// executeTools executes tool calls and yields progress.
func (e *QueryEngine) executeTools(ctx context.Context, blocks []api.ContentBlock, output chan<- interface{}) []types.Message {
	var results []types.Message

	for _, block := range blocks {
		// Find the tool
		tool := e.findTool(block.Name)
		if tool == nil {
			results = append(results, types.Message{
				Role: "user",
				Content: mustMarshalJSON([]map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": block.ID,
						"content":     fmt.Sprintf("Unknown tool: %s", block.Name),
						"is_error":    true,
					},
				}),
			})
			output <- toolResultSDKMessage(e.sessionID, block.Name, block.ID, fmt.Sprintf("Unknown tool: %s", block.Name), true)
			continue
		}

		if e.config.CanUseTool != nil {
			decision, err := e.config.CanUseTool(ctx, block.Name, block.Input)
			if err != nil {
				results = append(results, toolErrorMessage(block.ID, err.Error()))
				output <- toolResultSDKMessage(e.sessionID, block.Name, block.ID, err.Error(), true)
				continue
			}
			if decision != nil && decision.Behavior != types.PermissionBehaviorAllow {
				message := decision.Message
				if message == "" {
					message = fmt.Sprintf("permission %s for tool %s", decision.Behavior, block.Name)
				}
				results = append(results, toolErrorMessage(block.ID, message))
				output <- toolResultSDKMessage(e.sessionID, block.Name, block.ID, message, true)
				continue
			}
		}

		// Execute the tool
		toolCtx := &types.ToolContext{
			ToolUseId: block.ID,
			Options:   types.ToolOptions{},
			GetAppState: func() interface{} {
				if e.config.GetAppState == nil {
					return nil
				}
				return e.config.GetAppState()
			},
		}

		result, err := tool.Call(ctx, block.Input, toolCtx, e.config.CanUseTool, nil, nil)
		if err != nil {
			results = append(results, toolErrorMessage(block.ID, err.Error()))
			output <- toolResultSDKMessage(e.sessionID, block.Name, block.ID, err.Error(), true)
			continue
		}
		if result == nil {
			message := fmt.Sprintf("tool %s returned no result", block.Name)
			results = append(results, toolErrorMessage(block.ID, message))
			output <- toolResultSDKMessage(e.sessionID, block.Name, block.ID, message, true)
			continue
		}

		// Create tool result message
		var content string
		if result.Error != nil {
			content = result.Error.Error()
		} else {
			content = fmt.Sprintf("%v", result.Output)
		}

		resultBlock := map[string]interface{}{
			"type":        "tool_result",
			"tool_use_id": block.ID,
			"content":     content,
		}
		if result.Error != nil {
			resultBlock["is_error"] = true
		}
		results = append(results, types.Message{
			Role:    "user",
			Content: mustMarshalJSON([]map[string]interface{}{resultBlock}),
		})

		// Yield progress
		output <- toolResultSDKMessage(e.sessionID, block.Name, block.ID, content, result.Error != nil)
	}

	return mergeToolResultMessages(results)
}

func mergeToolResultMessages(messages []types.Message) []types.Message {
	if len(messages) <= 1 {
		return messages
	}
	var blocks []map[string]interface{}
	for _, message := range messages {
		var content []map[string]interface{}
		if err := json.Unmarshal(message.Content, &content); err != nil {
			return messages
		}
		blocks = append(blocks, content...)
	}
	return []types.Message{{Role: "user", Content: mustMarshalJSON(blocks)}}
}

// findTool finds a tool by name.
func (e *QueryEngine) findTool(name string) types.Tool {
	for _, tool := range e.config.Tools {
		if tool.Name() == name {
			return tool
		}
		for _, alias := range tool.Aliases() {
			if alias == name {
				return tool
			}
		}
	}
	return nil
}

// createResultMessage creates a result message.
func (e *QueryEngine) createResultMessage(turnCount int, stopReason, result string) ResultMessage {
	return ResultMessage{
		Type:          "result",
		Subtype:       "success",
		IsError:       false,
		DurationMs:    time.Since(e.startTime).Milliseconds(),
		DurationApiMs: e.apiDuration.Milliseconds(),
		NumTurns:      turnCount,
		SessionID:     e.sessionID,
		TotalCostUsd:  e.calculateCost(),
		Usage:         e.totalUsage,
		FastModeState: "disabled",
		UUID:          generateUUID(),
		StopReason:    stopReason,
		Result:        result,
	}
}

func (e *QueryEngine) createErrorResultMessage(subtype, result string, turnCount int) ResultMessage {
	return ResultMessage{
		Type:          "result",
		Subtype:       subtype,
		IsError:       true,
		DurationMs:    time.Since(e.startTime).Milliseconds(),
		DurationApiMs: e.apiDuration.Milliseconds(),
		NumTurns:      turnCount,
		Result:        result,
		SessionID:     e.sessionID,
		TotalCostUsd:  e.calculateCost(),
		Usage:         e.totalUsage,
		FastModeState: "disabled",
		UUID:          generateUUID(),
	}
}

func extractResponseText(response *api.MessageResponse) string {
	if response == nil {
		return ""
	}
	var text string
	for _, block := range response.Content {
		if block.Type == "text" {
			if text != "" {
				text += "\n"
			}
			text += block.Text
		}
	}
	return text
}

func toolErrorMessage(toolUseID, message string) types.Message {
	return types.Message{Role: "user", Content: mustMarshalJSON([]map[string]interface{}{{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     message,
		"is_error":    true,
	}})}
}

func toolResultSDKMessage(sessionID, toolName, toolUseID, content string, isError bool) SDKMessage {
	return SDKMessage{
		Type: "tool_result",
		Message: map[string]interface{}{
			"tool_name":   toolName,
			"tool_use_id": toolUseID,
			"content":     content,
			"is_error":    isError,
		},
		SessionID: sessionID,
		UUID:      generateUUID(),
		Timestamp: time.Now().UnixMilli(),
	}
}

// calculateCost calculates the cost of the API calls.
func (e *QueryEngine) calculateCost() float64 {
	// Simplified cost calculation
	// Real implementation would use model-specific pricing
	inputCost := float64(e.totalUsage.InputTokens) * 0.000003
	outputCost := float64(e.totalUsage.OutputTokens) * 0.000015
	cacheReadCost := float64(e.totalUsage.CacheReadInputTokens) * 0.0000003
	cacheWriteCost := float64(e.totalUsage.CacheCreationInputTokens) * 0.00000375

	return inputCost + outputCost + cacheReadCost + cacheWriteCost
}

// Interrupt interrupts the current query.
func (e *QueryEngine) Interrupt() {
	e.abortController.Abort()
}

// GetMessages returns the current messages.
func (e *QueryEngine) GetMessages() []types.Message {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mutableMessages
}

// GetSessionID returns the session ID.
func (e *QueryEngine) GetSessionID() string {
	return e.sessionID
}

// SetModel updates the model used by subsequent turns.
func (e *QueryEngine) SetModel(model string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config.UserSpecifiedModel = model
}

// Helper functions

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func generateSessionID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "session_" + hex.EncodeToString(b)
}

func mustMarshalJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
