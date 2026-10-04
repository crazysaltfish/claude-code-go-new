package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"claude-code-go/internal/commands"
	"claude-code-go/internal/memory"
	"claude-code-go/internal/query"
	"claude-code-go/internal/state"
	"claude-code-go/internal/tools"
	"claude-code-go/internal/types"
	"claude-code-go/internal/ui"
	"claude-code-go/internal/utils"
	"claude-code-go/pkg/api"
)

// App represents the CLI application.
type App struct {
	config             *Config
	registry           *commands.Registry
	toolRegistry       *tools.Registry
	queryEngine        *query.QueryEngine
	stateManager       *state.StateManager
	apiClient          api.MessageClient
	ctx                context.Context
	cancel             context.CancelFunc
	initialPrompt      string
	version            string
	permissionRequests chan ui.PermissionRequest
	permissionUI       bool
	printStreamActive  bool
	memoryDir          string
	permissionMu       sync.RWMutex
	permissionPromptMu sync.Mutex
}

// Config holds CLI configuration.
type Config struct {
	Debug          bool
	Verbose        bool
	PrintMode      bool
	Model          string
	PermissionMode string
	Cwd            string
	MaxTokens      int
	MaxTurns       int
	Provider       string
	APIKey         string
	BaseURL        string
}

// NewApp creates a new CLI application.
func NewApp(config *Config, version string) *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		config:             config,
		ctx:                ctx,
		cancel:             cancel,
		version:            version,
		permissionRequests: make(chan ui.PermissionRequest),
	}
}

// Initialize sets up all application components.
func (a *App) Initialize() error {
	// Get current working directory
	cwd := a.config.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}
	a.config.Cwd = cwd

	a.stateManager = state.NewStateManager()
	if err := a.stateManager.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize state: %w", err)
	}
	provider := strings.ToLower(strings.TrimSpace(a.config.Provider))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_API_PROVIDER")))
	}
	if provider == "" {
		provider = "anthropic"
	}
	if provider == "openai-compatible" {
		provider = "openai"
	}
	if provider != "anthropic" && provider != "openai" {
		return fmt.Errorf("unsupported API provider %q (expected anthropic or openai)", provider)
	}
	a.config.Provider = provider

	if a.config.Model == "" {
		if provider == "openai" {
			a.config.Model = os.Getenv("OPENAI_MODEL")
		} else {
			a.config.Model = os.Getenv("CLAUDE_MODEL")
		}
	}
	if a.config.Model == "" {
		if provider == "openai" {
			return fmt.Errorf("OpenAI provider requires a model via --model or OPENAI_MODEL")
		}
		a.config.Model = a.stateManager.GetCurrentModel()
	}
	if a.config.PermissionMode == "" {
		a.config.PermissionMode = os.Getenv("CLAUDE_PERMISSION_MODE")
	}
	if a.config.PermissionMode == "" {
		a.config.PermissionMode = a.stateManager.GetPermissionMode()
	}
	if a.config.PermissionMode != "" && !isExternalPermissionMode(types.PermissionMode(a.config.PermissionMode)) {
		return fmt.Errorf("invalid permission mode: %s", a.config.PermissionMode)
	}
	if permissionFlag := a.config.PermissionMode; permissionFlag != "" &&
		permissionFlag != string(types.PermissionModeSession) && permissionFlag != a.stateManager.GetPermissionMode() {
		if err := a.stateManager.SetPermissionMode(permissionFlag); err != nil {
			return fmt.Errorf("failed to persist permission mode: %w", err)
		}
	}

	// Initialize command registry
	a.registry = commands.NewRegistry()
	a.registerCommands()

	// Initialize tool registry
	a.toolRegistry = tools.NewToolRegistryWithOptions(tools.RegistryOptionsFromEnv())

	// Initialize the selected provider client.
	switch provider {
	case "openai":
		apiKey := a.config.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		baseURL := a.config.BaseURL
		if baseURL == "" {
			baseURL = os.Getenv("OPENAI_BASE_URL")
		}
		a.apiClient = api.NewOpenAIClient(api.Config{
			APIKey:       apiKey,
			BaseURL:      baseURL,
			Organization: os.Getenv("OPENAI_ORGANIZATION"),
			Project:      os.Getenv("OPENAI_PROJECT"),
		})
	default:
		apiKey := a.config.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		baseURL := a.config.BaseURL
		if baseURL == "" {
			baseURL = os.Getenv("ANTHROPIC_BASE_URL")
		}
		a.apiClient = api.NewClient(api.Config{
			APIKey:    apiKey,
			AuthToken: os.Getenv("ANTHROPIC_AUTH_TOKEN"),
			BaseURL:   baseURL,
		})
	}

	// Initialize query engine
	memoryContext, err := memory.Load(cwd)
	if err != nil && a.config.Debug {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize memory: %v\n", err)
	}
	a.memoryDir = memoryContext.Directory
	var sessionMemory *memory.SessionMemory
	if memory.SessionMemoryEnabled() && !a.config.PrintMode {
		sessionMemory, err = memory.NewSessionMemory(cwd, a.stateManager.GetSessionID(), memory.DefaultSessionMemoryConfig)
		if err != nil && a.config.Debug {
			fmt.Fprintf(os.Stderr, "Warning: failed to initialize session memory: %v\n", err)
		}
	}
	queryConfig := query.QueryEngineConfig{
		SessionID:       a.stateManager.GetSessionID(),
		Cwd:             cwd,
		Tools:           a.toolRegistry.ListEnabled(),
		MaxTokens:       a.config.MaxTokens,
		MaxTurns:        a.config.MaxTurns,
		APIClient:       a.apiClient,
		CanUseTool:      a.canUseTool,
		MemoryPrompt:    memoryContext.Prompt,
		MemoryDirectory: memoryContext.Directory,
		SessionMemory:   sessionMemory,
		GetAppState: func() *types.AppState {
			permissionMode := a.currentPermissionMode()
			return &types.AppState{
				MainLoopModel: a.config.Model,
				Settings: types.SettingsJson{
					PermissionMode: string(permissionMode),
				},
				ToolPermissionContext: types.ToolPermissionContext{
					Mode: permissionMode,
				},
			}
		},
	}

	if a.config.Model != "" {
		queryConfig.UserSpecifiedModel = a.config.Model
	}

	a.queryEngine = query.NewQueryEngine(queryConfig)

	return nil
}

// Run starts the application.
func (a *App) Run(initialPrompt string) error {
	a.initialPrompt = initialPrompt

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		a.cancel()
	}()

	if a.config.PrintMode {
		return a.runPrintMode()
	}
	return a.runInteractiveMode()
}

// runPrintMode runs in non-interactive mode.
func (a *App) runPrintMode() error {
	if a.initialPrompt == "" {
		return fmt.Errorf("no prompt provided in print mode")
	}

	// Process the prompt
	output, err := a.submitInteractiveInput(a.ctx, a.initialPrompt)
	if err != nil {
		return fmt.Errorf("failed to process prompt: %w", err)
	}

	// Collect and display results
	for msg := range output {
		switch m := msg.(type) {
		case query.SDKMessage:
			a.printSDKMessage(m)
		case query.ResultMessage:
			a.printResultMessage(m)
		}
	}

	return nil
}

// runInteractiveMode runs the interactive UI.
func (a *App) runInteractiveMode() error {
	a.permissionUI = true
	defer func() { a.permissionUI = false }()
	model := ui.NewAppModelWithContext(a.ctx, a.submitInteractiveInput, 80, 24)
	model.SetInitialPrompt(a.initialPrompt)
	model.SetPermissionRequests(a.permissionRequests)
	model.SetPermissionMode(a.currentPermissionMode(), a.setPermissionMode)

	// Create and run the tea program
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Handle UI events in a goroutine
	go func() {
		for {
			select {
			case <-a.ctx.Done():
				p.Quit()
				return
			}
		}
	}()

	// Start the UI
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	// Handle final state
	_ = finalModel

	return nil
}

// submitInteractiveInput executes local slash commands or submits a model query.
func (a *App) submitInteractiveInput(ctx context.Context, input string) (<-chan interface{}, error) {
	if !strings.HasPrefix(input, "/") {
		return a.queryEngine.SubmitMessage(ctx, input)
	}

	cmdName, args := commands.ParseCommand(input)
	cmd, ok := a.registry.Get(cmdName)
	if !ok {
		return a.queryEngine.SubmitMessage(ctx, input)
	}
	if cmdName == "compact" {
		return a.queryEngine.Compact(ctx, args)
	}
	if cmdName == "permissions" || cmdName == "perm" {
		return a.executePermissionCommand(args), nil
	}

	output := make(chan interface{}, 1)
	go func() {
		defer close(output)
		result, err := cmd.Execute(ctx, args, &commands.CommandContext{
			Cwd:           a.config.Cwd,
			Args:          args,
			IsInteractive: a.permissionUI,
			Verbose:       a.config.Verbose,
			Debug:         a.config.Debug,
		})
		if err != nil {
			output <- query.SDKMessage{Type: "system", Message: map[string]interface{}{
				"subtype": "error",
				"error":   err.Error(),
			}}
			return
		}
		if result != nil && result.Value != "" {
			if cmdName == "model" && strings.HasPrefix(result.Value, "Model set to:") {
				a.config.Model = strings.TrimSpace(strings.TrimPrefix(result.Value, "Model set to:"))
				a.queryEngine.SetModel(a.config.Model)
				if err := a.stateManager.SetCurrentModel(a.config.Model); err != nil {
					output <- query.SDKMessage{Type: "system", Message: map[string]interface{}{
						"subtype": "error",
						"error":   fmt.Sprintf("model changed but could not be persisted: %v", err),
					}}
					return
				}
			}
			output <- query.SDKMessage{Type: "system", Message: map[string]interface{}{
				"subtype": "message",
				"content": result.Value,
			}}
		}
	}()
	return output, nil
}

func (a *App) canUseTool(ctx context.Context, toolName string, input json.RawMessage) (*types.PermissionDecision, error) {
	tool, ok := a.toolRegistry.Get(toolName)
	if !ok {
		return &types.PermissionDecision{Behavior: types.PermissionBehaviorDeny, Message: "unknown tool"}, nil
	}

	mode := a.currentPermissionMode()
	riskReason := ""
	if tool.Name() == "Bash" {
		var bashInput struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(input, &bashInput); err == nil {
			securityResult := utils.BashCommandIsSafe(bashInput.Command)
			if securityResult.Behavior == "deny" {
				return &types.PermissionDecision{
					Behavior: types.PermissionBehaviorDeny,
					Message:  fmt.Sprintf("command blocked by security policy: %s", securityResult.Message),
				}, nil
			}
			if securityResult.Behavior == "ask" {
				riskReason = securityResult.Message
			}
		}
	}
	if mode == types.PermissionModeBypassPermissions || mode == types.PermissionModeSession {
		return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
	}
	pathsWithinCwd := true
	pathsWithinMemory := false
	if provider, ok := tool.(types.ToolPathProvider); ok {
		paths := provider.InputPaths(input)
		pathsWithinMemory = a.memoryDir != "" && len(paths) > 0
		for _, target := range paths {
			if !tools.IsPathWithin(a.config.Cwd, target) {
				pathsWithinCwd = false
			}
			if a.memoryDir == "" || !tools.IsPathWithin(a.memoryDir, target) {
				pathsWithinMemory = false
			}
		}
	}
	if mode == types.PermissionModePlan {
		if tool.IsReadOnly(input) && (pathsWithinCwd || pathsWithinMemory) {
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		}
		return &types.PermissionDecision{
			Behavior: types.PermissionBehaviorDeny,
			Message:  fmt.Sprintf("tool %s is not allowed in plan mode", tool.Name()),
		}, nil
	}
	if mode == types.PermissionModeDontAsk {
		if tool.IsReadOnly(input) && pathsWithinCwd {
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		}
		return &types.PermissionDecision{
			Behavior: types.PermissionBehaviorDeny,
			Message:  fmt.Sprintf("tool %s is not pre-approved in dontAsk mode", tool.Name()),
		}, nil
	}
	if pathsWithinMemory && mode == types.PermissionModeAcceptEdits {
		switch tool.Name() {
		case "Read":
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		case "Write", "Edit":
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		}
	}
	if mode == types.PermissionModeAcceptEdits && tool.IsReadOnly(input) && pathsWithinCwd {
		return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
	}
	if mode == types.PermissionModeAcceptEdits && pathsWithinCwd {
		switch tool.Name() {
		case "Write", "Edit", "MultiEdit", "NotebookEdit":
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		}
	}
	if !a.permissionUI {
		return &types.PermissionDecision{
			Behavior: types.PermissionBehaviorDeny,
			Message:  fmt.Sprintf("tool %s requires approval and cannot run in print mode", tool.Name()),
		}, nil
	}

	// Serialize prompts so concurrent safe tools cannot overlap approval panels.
	// Re-check the mode after acquiring the slot because an earlier approval
	// may have enabled all remaining tools for this session.
	a.permissionPromptMu.Lock()
	defer a.permissionPromptMu.Unlock()
	mode = a.currentPermissionMode()
	if mode == types.PermissionModeSession || mode == types.PermissionModeBypassPermissions {
		return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
	}

	response := make(chan ui.PermissionResponse, 1)
	request := ui.PermissionRequest{
		ToolName:    tool.Name(),
		Input:       append(json.RawMessage(nil), input...),
		ReadOnly:    tool.IsReadOnly(input),
		Destructive: tool.IsDestructive(input),
		RiskReason:  riskReason,
		Response:    response,
	}
	select {
	case a.permissionRequests <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case permissionResponse := <-response:
		switch permissionResponse {
		case ui.PermissionAllowOnce:
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		case ui.PermissionAllowSession:
			if err := a.setPermissionMode(types.PermissionModeSession); err != nil {
				return nil, err
			}
			return &types.PermissionDecision{Behavior: types.PermissionBehaviorAllow}, nil
		}
		return &types.PermissionDecision{Behavior: types.PermissionBehaviorDeny, Message: fmt.Sprintf("user denied tool %s", tool.Name())}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *App) currentPermissionMode() types.PermissionMode {
	a.permissionMu.RLock()
	defer a.permissionMu.RUnlock()
	mode := types.PermissionMode(a.config.PermissionMode)
	if mode == "" {
		return types.PermissionModeDefault
	}
	return mode
}

func (a *App) setPermissionMode(mode types.PermissionMode) error {
	if !isExternalPermissionMode(mode) {
		return fmt.Errorf("invalid permission mode %q", mode)
	}
	a.permissionMu.Lock()
	a.config.PermissionMode = string(mode)
	a.permissionMu.Unlock()
	return nil
}

func (a *App) executePermissionCommand(args string) <-chan interface{} {
	output := make(chan interface{}, 1)
	go func() {
		defer close(output)
		requested := strings.TrimSpace(args)
		if requested == "" {
			mode := a.currentPermissionMode()
			output <- permissionModeMessage(mode, permissionModeHelp(mode))
			return
		}
		mode := types.PermissionMode(requested)
		if strings.EqualFold(requested, "allowAll") || strings.EqualFold(requested, "allow-session") {
			mode = types.PermissionModeSession
		}
		if err := a.setPermissionMode(mode); err != nil {
			output <- query.SDKMessage{Type: "system", Message: map[string]interface{}{
				"subtype": "error", "error": err.Error(),
			}}
			return
		}
		output <- permissionModeMessage(mode, "Permission mode: "+string(mode))
	}()
	return output
}

func permissionModeMessage(mode types.PermissionMode, content string) query.SDKMessage {
	return query.SDKMessage{Type: "system", Message: map[string]interface{}{
		"subtype": "permission_mode", "mode": string(mode), "content": content,
	}}
}

func permissionModeHelp(current types.PermissionMode) string {
	return fmt.Sprintf(`Permission modes:
  default           Ask before every tool execution
  session           Allow all tools for this session only
  acceptEdits       Auto-approve reads and in-workspace file edits
  plan              Allow read-only exploration; deny changes
  dontAsk           Deny operations that are not pre-approved
  bypassPermissions Allow all tools without prompts (dangerous)

Current mode: %s
Use /permissions <mode> or Shift+Tab to switch common modes.`, current)
}

func isExternalPermissionMode(mode types.PermissionMode) bool {
	for _, candidate := range types.ExternalPermissionModes {
		if mode == candidate {
			return true
		}
	}
	return false
}

// printSDKMessage prints an SDK message in print mode.
func (a *App) printSDKMessage(msg query.SDKMessage) {
	if msg.Type == "assistant_delta" {
		if delta, ok := msg.Message.(query.AssistantDelta); ok {
			if delta.Text != "" {
				fmt.Print(delta.Text)
				a.printStreamActive = true
			}
		}
		return
	}
	if msg.Type == "assistant" && a.printStreamActive {
		fmt.Println()
		a.printStreamActive = false
		return
	}
	data, _ := json.MarshalIndent(msg, "", "  ")
	fmt.Println(string(data))
}

// printResultMessage prints a result message in print mode.
func (a *App) printResultMessage(msg query.ResultMessage) {
	fmt.Printf("\n--- Result ---\n")
	fmt.Printf("Status: %s\n", msg.Subtype)
	fmt.Printf("Duration: %.2fs\n", float64(msg.DurationMs)/1000)
	fmt.Printf("Turns: %d\n", msg.NumTurns)
	fmt.Printf("Cost: $%.6f\n", msg.TotalCostUsd)
	fmt.Printf("Tokens: %d input, %d output\n", msg.Usage.InputTokens, msg.Usage.OutputTokens)
}

// registerCommands registers all built-in commands.
func (a *App) registerCommands() {
	a.registry.Register(commands.NewHelpCommand(a.registry))
	a.registry.Register(commands.NewDiffCommand())
	a.registry.Register(commands.NewExitCommand())
	a.registry.Register(commands.NewClearCommand())
	a.registry.Register(commands.NewModelCommand())
	a.registry.Register(commands.NewConfigCommand())
	a.registry.Register(commands.NewCostCommand())
	a.registry.Register(commands.NewThemeCommand())
	a.registry.Register(commands.NewCompactCommand())
	a.registry.Register(commands.NewPermissionCommand())
	a.registry.RegisterAlias("perm", "permissions")
}

// Shutdown cleans up resources.
func (a *App) Shutdown() {
	if a.queryEngine != nil {
		a.queryEngine.WaitForSessionMemory(a.ctx)
	}
	if a.cancel != nil {
		a.cancel()
	}
}

// RunWithPrompt runs the app with a specific prompt (for scripting).
func (a *App) RunWithPrompt(prompt string) (string, error) {
	output, err := a.queryEngine.SubmitMessage(a.ctx, prompt)
	if err != nil {
		return "", err
	}

	var result string
	var resultErr error
	for msg := range output {
		if m, ok := msg.(query.ResultMessage); ok {
			result = m.Result
			if m.IsError {
				resultErr = fmt.Errorf("query failed: %s: %s", m.Subtype, m.Result)
			}
		}
	}
	return result, resultErr
}
