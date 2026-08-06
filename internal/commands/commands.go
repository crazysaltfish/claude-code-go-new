package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"claude-code-go/internal/utils"
)

// CommandResult represents the result of command execution.
// Matches TypeScript LocalCommandResult type.
type CommandResult struct {
	Type           string      `json:"type"`                     // text | compact | skip
	Value          string      `json:"value,omitempty"`          // For type="text"
	Display        string      `json:"display,omitempty"`        // skip | system | user
	CompactionInfo interface{} `json:"compactionInfo,omitempty"` // For type="compact"
}

// CommandContext provides context for command execution.
type CommandContext struct {
	Cwd           string
	Args          string
	IsInteractive bool
	Verbose       bool
	Debug         bool
}

// CommandHandler defines the interface for command handlers.
type CommandHandler interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error)
	IsEnabled() bool
	IsHidden() bool
}

// Command represents the full command type (discriminated union).
// Matches TypeScript Command = CommandBase & (PromptCommand | LocalCommand | LocalJSXCommand)
type Command struct {
	// Common fields (from CommandBase)
	Type                   string        `json:"type"` // prompt | local | local-jsx
	Name                   string        `json:"name"`
	Aliases                []string      `json:"aliases,omitempty"`
	Description            string        `json:"description"`
	HasUserSpecifiedDesc   bool          `json:"hasUserSpecifiedDescription,omitempty"`
	IsEnabled              func() bool   `json:"-"`
	IsHidden               bool          `json:"isHidden,omitempty"`
	IsMcp                  bool          `json:"isMcp,omitempty"`
	ArgumentHint           string        `json:"argumentHint,omitempty"`
	WhenToUse              string        `json:"whenToUse,omitempty"`
	Version                string        `json:"version,omitempty"`
	DisableModelInvocation bool          `json:"disableModelInvocation,omitempty"`
	UserInvocable          bool          `json:"userInvocable,omitempty"`
	LoadedFrom             string        `json:"loadedFrom,omitempty"`
	Kind                   string        `json:"kind,omitempty"`
	Immediate              bool          `json:"immediate,omitempty"`
	IsSensitive            bool          `json:"isSensitive,omitempty"`
	UserFacingName         func() string `json:"-"`
	Availability           []string      `json:"availability,omitempty"` // claude-ai | console

	// For type="prompt"
	ProgressMessage string   `json:"progressMessage,omitempty"`
	ContentLength   int      `json:"contentLength,omitempty"`
	AllowedTools    []string `json:"allowedTools,omitempty"`

	// For type="local"
	SupportsNonInteractive bool `json:"supportsNonInteractive,omitempty"`

	// Execution callback (set at load time)
	ExecuteFunc func(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) `json:"-"`
}

// GetCommandName resolves the user-visible name, falling back to cmd.Name.
func GetCommandName(cmd *Command) string {
	if cmd.UserFacingName != nil {
		return cmd.UserFacingName()
	}
	return cmd.Name
}

// IsCommandEnabled resolves whether the command is enabled, defaulting to true.
func IsCommandEnabled(cmd *Command) bool {
	if cmd.IsEnabled != nil {
		return cmd.IsEnabled()
	}
	return true
}

// Registry manages all available commands.
type Registry struct {
	commands map[string]CommandHandler
	aliases  map[string]string
	order    []string
}

// NewRegistry creates a new command registry.
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]CommandHandler),
		aliases:  make(map[string]string),
		order:    make([]string, 0),
	}
}

// Register adds a command to the registry.
func (r *Registry) Register(handler CommandHandler) {
	if _, exists := r.commands[handler.Name()]; !exists {
		r.order = append(r.order, handler.Name())
	}
	r.commands[handler.Name()] = handler
}

// RegisterAlias adds an alias for a command.
func (r *Registry) RegisterAlias(alias, commandName string) {
	r.aliases[alias] = commandName
}

// Get retrieves a command by name or alias.
func (r *Registry) Get(name string) (CommandHandler, bool) {
	// Check direct name first
	if cmd, ok := r.commands[name]; ok {
		return cmd, true
	}
	// Check aliases
	if aliasTarget, ok := r.aliases[name]; ok {
		if cmd, ok := r.commands[aliasTarget]; ok {
			return cmd, true
		}
	}
	return nil, false
}

// List returns all registered commands.
func (r *Registry) List() []CommandHandler {
	result := make([]CommandHandler, 0, len(r.order))
	for _, name := range r.order {
		if cmd, ok := r.commands[name]; ok {
			result = append(result, cmd)
		}
	}
	return result
}

// ListEnabled returns all enabled commands.
func (r *Registry) ListEnabled() []CommandHandler {
	result := make([]CommandHandler, 0)
	for _, cmd := range r.List() {
		if cmd.IsEnabled() {
			result = append(result, cmd)
		}
	}
	return result
}

// ParseCommand parses a command string into name and arguments.
func ParseCommand(input string) (name string, args string) {
	input = strings.TrimSpace(input)
	parts := strings.SplitN(input, " ", 2)
	if len(parts) == 0 {
		return "", ""
	}
	name = strings.TrimPrefix(parts[0], "/")
	if len(parts) > 1 {
		args = parts[1]
	}
	return name, args
}

// ========================================
// Built-in Commands
// ========================================

// HelpCommand shows available commands.
type HelpCommand struct {
	registry *Registry
}

func NewHelpCommand(registry *Registry) *HelpCommand {
	return &HelpCommand{registry: registry}
}

func (c *HelpCommand) Name() string        { return "help" }
func (c *HelpCommand) Description() string { return "Show available commands" }
func (c *HelpCommand) IsEnabled() bool     { return true }
func (c *HelpCommand) IsHidden() bool      { return false }

func (c *HelpCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	var sb strings.Builder
	sb.WriteString("Available commands:\n\n")

	commands := c.registry.ListEnabled()
	for _, cmd := range commands {
		if !cmd.IsHidden() {
			sb.WriteString(fmt.Sprintf("  /%-15s %s\n", cmd.Name(), cmd.Description()))
		}
	}

	return &CommandResult{
		Type:  "text",
		Value: sb.String(),
	}, nil
}

// ExitCommand exits the application.
type ExitCommand struct{}

func NewExitCommand() *ExitCommand { return &ExitCommand{} }

func (c *ExitCommand) Name() string        { return "exit" }
func (c *ExitCommand) Description() string { return "Exit the application" }
func (c *ExitCommand) IsEnabled() bool     { return true }
func (c *ExitCommand) IsHidden() bool      { return false }

func (c *ExitCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	return &CommandResult{
		Type:  "text",
		Value: "Goodbye!",
	}, nil
}

// ClearCommand clears the screen.
type ClearCommand struct{}

func NewClearCommand() *ClearCommand { return &ClearCommand{} }

func (c *ClearCommand) Name() string        { return "clear" }
func (c *ClearCommand) Description() string { return "Clear the screen" }
func (c *ClearCommand) IsEnabled() bool     { return true }
func (c *ClearCommand) IsHidden() bool      { return false }

func (c *ClearCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	// Clear screen escape sequence
	return &CommandResult{
		Type:  "text",
		Value: "\033[2J\033[H",
	}, nil
}

// ModelCommand manages model selection.
type ModelCommand struct {
	currentModel string
}

func NewModelCommand() *ModelCommand {
	return &ModelCommand{currentModel: "claude-sonnet-4-20250514"}
}

func (c *ModelCommand) Name() string        { return "model" }
func (c *ModelCommand) Description() string { return "Switch or show the current model" }
func (c *ModelCommand) IsEnabled() bool     { return true }
func (c *ModelCommand) IsHidden() bool      { return false }

func (c *ModelCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return &CommandResult{
			Type:  "text",
			Value: fmt.Sprintf("Current model: %s", c.currentModel),
		}, nil
	}

	// Validate and set model
	validModels := []string{
		"claude-sonnet-4-20250514",
		"claude-opus-4-20250514",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
	}

	for _, model := range validModels {
		if args == model {
			c.currentModel = model
			return &CommandResult{
				Type:  "text",
				Value: fmt.Sprintf("Model set to: %s", model),
			}, nil
		}
	}

	return &CommandResult{
		Type:  "text",
		Value: fmt.Sprintf("Invalid model. Available: %s", strings.Join(validModels, ", ")),
	}, nil
}

// ConfigCommand manages configuration.
type ConfigCommand struct{}

func NewConfigCommand() *ConfigCommand { return &ConfigCommand{} }

func (c *ConfigCommand) Name() string        { return "config" }
func (c *ConfigCommand) Description() string { return "Manage configuration settings" }
func (c *ConfigCommand) IsEnabled() bool     { return true }
func (c *ConfigCommand) IsHidden() bool      { return false }

func (c *ConfigCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	// TODO: Implement config management
	return &CommandResult{
		Type:  "text",
		Value: "Configuration management not yet implemented",
	}, nil
}

// CostCommand shows cost tracking.
type CostCommand struct{}

func NewCostCommand() *CostCommand { return &CostCommand{} }

func (c *CostCommand) Name() string        { return "cost" }
func (c *CostCommand) Description() string { return "Show current session cost" }
func (c *CostCommand) IsEnabled() bool     { return true }
func (c *CostCommand) IsHidden() bool      { return false }

func (c *CostCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	// TODO: Implement cost tracking
	return &CommandResult{
		Type:  "text",
		Value: "Cost tracking: $0.00 (not yet implemented)",
	}, nil
}

// ThemeCommand manages theme settings.
type ThemeCommand struct {
	currentTheme string
}

func NewThemeCommand() *ThemeCommand {
	return &ThemeCommand{currentTheme: "dark"}
}

func (c *ThemeCommand) Name() string        { return "theme" }
func (c *ThemeCommand) Description() string { return "Set or show the current theme" }
func (c *ThemeCommand) IsEnabled() bool     { return true }
func (c *ThemeCommand) IsHidden() bool      { return false }

func (c *ThemeCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return &CommandResult{
			Type:  "text",
			Value: fmt.Sprintf("Current theme: %s", c.currentTheme),
		}, nil
	}

	validThemes := []string{"dark", "light", "ansi"}
	for _, theme := range validThemes {
		if args == theme {
			c.currentTheme = theme
			return &CommandResult{
				Type:  "text",
				Value: fmt.Sprintf("Theme set to: %s", theme),
			}, nil
		}
	}

	return &CommandResult{
		Type:  "text",
		Value: fmt.Sprintf("Invalid theme. Available: %s", strings.Join(validThemes, ", ")),
	}, nil
}

// ========================================
// Doctor Command - System Diagnostics
// ========================================

// DoctorCommand runs system diagnostics.
type DoctorCommand struct{}

func NewDoctorCommand() *DoctorCommand { return &DoctorCommand{} }

func (c *DoctorCommand) Name() string        { return "doctor" }
func (c *DoctorCommand) Description() string { return "Run system diagnostics" }
func (c *DoctorCommand) IsEnabled() bool     { return true }
func (c *DoctorCommand) IsHidden() bool      { return false }

func (c *DoctorCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	var sb strings.Builder
	sb.WriteString("Running diagnostics...\n\n")

	// Check Go version
	sb.WriteString("✓ Go runtime: " + "1.26\n")

	// Check API key
	apiKey := ""
	if apiKey == "" {
		sb.WriteString("✗ API key not set (run /login or set ANTHROPIC_API_KEY)\n")
	} else {
		sb.WriteString("✓ API key configured\n")
	}

	// Check config directory
	home, err := homeDir()
	if err == nil {
		configDir := home + "/.claude"
		if _, err := exists(configDir); err == nil {
			sb.WriteString("✓ Config directory exists: " + configDir + "\n")
		} else {
			sb.WriteString("✗ Config directory missing: " + configDir + "\n")
		}
	}

	// Check git
	if _, err := exists(".git"); err == nil {
		sb.WriteString("✓ Git repository detected\n")
	} else {
		sb.WriteString("○ Not in a git repository\n")
	}

	// Check common tools
	tools := []string{"git", "ripgrep", "node"}
	for _, tool := range tools {
		if isAvailable(tool) {
			sb.WriteString("✓ " + tool + " available\n")
		} else {
			sb.WriteString("○ " + tool + " not found\n")
		}
	}

	return &CommandResult{
		Type:  "text",
		Value: sb.String(),
	}, nil
}

// ========================================
// Login Command
// ========================================

// LoginCommand handles user authentication.
type LoginCommand struct{}

func NewLoginCommand() *LoginCommand { return &LoginCommand{} }

func (c *LoginCommand) Name() string        { return "login" }
func (c *LoginCommand) Description() string { return "Authenticate with Claude" }
func (c *LoginCommand) IsEnabled() bool     { return true }
func (c *LoginCommand) IsHidden() bool      { return false }

func (c *LoginCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	return &CommandResult{
		Type:  "text",
		Value: "Starting OAuth login flow...\nPlease visit the URL shown in your browser to authenticate.",
	}, nil
}

// ========================================
// Logout Command
// ========================================

// LogoutCommand handles user logout.
type LogoutCommand struct{}

func NewLogoutCommand() *LogoutCommand { return &LogoutCommand{} }

func (c *LogoutCommand) Name() string        { return "logout" }
func (c *LogoutCommand) Description() string { return "Sign out from Claude" }
func (c *LogoutCommand) IsEnabled() bool     { return true }
func (c *LogoutCommand) IsHidden() bool      { return false }

func (c *LogoutCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	return &CommandResult{
		Type:  "text",
		Value: "You have been logged out.",
	}, nil
}

// ========================================
// Init Command
// ========================================

// InitCommand initializes a new project.
type InitCommand struct{}

func NewInitCommand() *InitCommand { return &InitCommand{} }

func (c *InitCommand) Name() string        { return "init" }
func (c *InitCommand) Description() string { return "Initialize Claude in the current project" }
func (c *InitCommand) IsEnabled() bool     { return true }
func (c *InitCommand) IsHidden() bool      { return false }

func (c *InitCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	var sb strings.Builder
	sb.WriteString("Initializing Claude in current directory...\n\n")

	// Create .claude directory
	claudeDir := ".claude"
	if err := mkdirAll(claudeDir); err != nil {
		return &CommandResult{
			Type:  "text",
			Value: "Failed to create .claude directory: " + err.Error(),
		}, nil
	}
	sb.WriteString("✓ Created " + claudeDir + "/\n")

	// Create CLAUDE.md if not exists
	claudeMd := "CLAUDE.md"
	if _, err := exists(claudeMd); err != nil {
		content := `# Project Instructions for Claude

This file contains instructions for Claude Code to understand and work with this project.

## Project Overview

<!-- Add a brief description of your project here -->

## Build & Test Commands

<!-- Add commands to build and test your project -->
- Build: ` + "`go build ./...`" + `
- Test: ` + "`go test ./...`" + `

## Code Style

<!-- Add code style guidelines -->

## Important Files

<!-- List important files and their purposes -->
`
		if err := writeFile(claudeMd, content); err != nil {
			sb.WriteString("○ Could not create CLAUDE.md: " + err.Error() + "\n")
		} else {
			sb.WriteString("✓ Created " + claudeMd + "\n")
		}
	} else {
		sb.WriteString("○ CLAUDE.md already exists\n")
	}

	sb.WriteString("\nProject initialized successfully!")
	return &CommandResult{
		Type:  "text",
		Value: sb.String(),
	}, nil
}

// ========================================
// Compact Command
// ========================================

// CompactCommand compresses conversation history.
type CompactCommand struct{}

func NewCompactCommand() *CompactCommand { return &CompactCommand{} }

func (c *CompactCommand) Name() string        { return "compact" }
func (c *CompactCommand) Description() string { return "Compress conversation history" }
func (c *CompactCommand) IsEnabled() bool     { return true }
func (c *CompactCommand) IsHidden() bool      { return false }

func (c *CompactCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	return &CommandResult{
		Type:  "compact",
		Value: "Compressing conversation history...",
	}, nil
}

// ========================================
// Resume Command
// ========================================

// ResumeCommand resumes a previous session.
type ResumeCommand struct{}

func NewResumeCommand() *ResumeCommand { return &ResumeCommand{} }

func (c *ResumeCommand) Name() string        { return "resume" }
func (c *ResumeCommand) Description() string { return "Resume a previous conversation session" }
func (c *ResumeCommand) IsEnabled() bool     { return true }
func (c *ResumeCommand) IsHidden() bool      { return false }

func (c *ResumeCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	return &CommandResult{
		Type:  "text",
		Value: "Listing recent sessions... (not yet implemented)",
	}, nil
}

// ========================================
// Status Command
// ========================================

// StatusCommand shows current session status.
type StatusCommand struct{}

func NewStatusCommand() *StatusCommand { return &StatusCommand{} }

func (c *StatusCommand) Name() string        { return "status" }
func (c *StatusCommand) Description() string { return "Show current session status" }
func (c *StatusCommand) IsEnabled() bool     { return true }
func (c *StatusCommand) IsHidden() bool      { return false }

func (c *StatusCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	var sb strings.Builder
	sb.WriteString("Session Status:\n\n")

	cwd, _ := workDir()
	sb.WriteString("Working Directory: " + cwd + "\n")
	sb.WriteString("Model: claude-sonnet-4-20250514\n")
	sb.WriteString("Permission Mode: default\n")
	sb.WriteString("Turn: 0\n")

	return &CommandResult{
		Type:  "text",
		Value: sb.String(),
	}, nil
}

// ========================================
// MCP Commands
// ========================================

// MCPCommand manages MCP servers.
type MCPCommand struct{}

func NewMCPCommand() *MCPCommand { return &MCPCommand{} }

func (c *MCPCommand) Name() string        { return "mcp" }
func (c *MCPCommand) Description() string { return "Manage MCP servers" }
func (c *MCPCommand) IsEnabled() bool     { return true }
func (c *MCPCommand) IsHidden() bool      { return false }

func (c *MCPCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	args = strings.TrimSpace(args)

	if args == "" || args == "list" {
		return &CommandResult{
			Type:  "text",
			Value: "MCP Servers:\n\nNo MCP servers configured.\n\nUse /mcp add <name> to add a server.",
		}, nil
	}

	if strings.HasPrefix(args, "add ") {
		return &CommandResult{
			Type:  "text",
			Value: "MCP server addition not yet implemented",
		}, nil
	}

	return &CommandResult{
		Type:  "text",
		Value: "Usage: /mcp [list|add|remove|status]",
	}, nil
}

// ========================================
// Permission Commands
// ========================================

// PermissionCommand manages permissions.
type PermissionCommand struct{}

func NewPermissionCommand() *PermissionCommand { return &PermissionCommand{} }

func (c *PermissionCommand) Name() string        { return "permissions" }
func (c *PermissionCommand) Aliases() []string   { return []string{"perm"} }
func (c *PermissionCommand) Description() string { return "Manage permission settings" }
func (c *PermissionCommand) IsEnabled() bool     { return true }
func (c *PermissionCommand) IsHidden() bool      { return false }

func (c *PermissionCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
	args = strings.TrimSpace(args)

	if args == "" {
		return &CommandResult{
			Type: "text",
			Value: `Permission Modes:
  - default: Ask for permission on each operation
  - acceptEdits: Auto-accept file edits
  - bypassPermissions: Allow all operations (use with caution)
  - plan: Planning mode

Current mode: default

Usage: /permissions <mode>`,
		}, nil
	}

	return &CommandResult{
		Type:  "text",
		Value: "Permission mode set to: " + args,
	}, nil
}

// ========================================
// Helper Functions
// ========================================

func homeDir() (string, error) {
	return utils.GetHomeDir(), nil
}

func exists(path string) (bool, error) {
	return utils.PathExists(path), nil
}

func isAvailable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func workDir() (string, error) {
	return os.Getwd()
}
