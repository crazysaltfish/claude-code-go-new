# Claude Code Go - Development Guidelines

This document provides guidelines for working on the Claude Code Go project itself.

## Project Overview

**Claude Code Go** is a Go language port of Anthropic's Claude Code, an AI-powered coding assistant that runs as a single binary. It provides both an interactive terminal UI and non-interactive modes.

- **Language**: Go 1.26+
- **Status**: Active development (ported from TypeScript)
- **License**: Refer to the original Claude Code license

---

## Architecture Quick Reference

```
claude-code-go-new/
├── cmd/cli/main.go           # CLI entry point (Cobra)
├── pkg/api/                  # API clients (Anthropic & OpenAI)
└── internal/
    ├── cli/app.go            # Main application orchestration
    ├── query/engine.go       # Core query engine
    ├── tools/                # Tool implementations
    ├── ui/                   # TUI (Bubble Tea)
    ├── state/                # State management
    ├── commands/             # Slash commands
    ├── memory/               # Memory & conversation management
    ├── permissions/          # Permission system
    ├── services/             # Supporting services (MCP, OAuth, etc.)
    ├── types/                # Type definitions
    ├── constants/            # Prompts & constants
    └── utils/                # Utilities
```

---

## Development Workflow

### Setting Up

```bash
# Clone and cd into project
cd /Users/bytedance/GolandProjects/claude-code-go-new

# Install dependencies
go mod download

# Run tests
go test ./internal/...
```

### Running Locally

```bash
# Interactive mode
go run ./cmd/cli --provider openai

# Print mode (one-off prompt)
go run ./cmd/cli --provider openai --print "your prompt here"

# With debug logging
go run ./cmd/cli --provider openai --debug
```

### Environment Variables

```bash
# Anthropic
export ANTHROPIC_API_KEY="sk-ant-..."
export ANTHROPIC_BASE_URL="https://api.anthropic.com/v1"  # optional

# OpenAI-compatible
export OPENAI_API_KEY="sk-..."
export OPENAI_BASE_URL="https://api.openai.com/v1"
export OPENAI_MODEL="gpt-4o"

# Settings
export CLAUDE_PERMISSION_MODE="default|acceptEdits|bypassPermissions"
export CLAUDE_CODE_SIMPLE="1"  # disable advanced features
```

---

## Key Architectural Concepts

### 1. Query Engine Flow

**File**: `internal/query/engine.go`

The core orchestrator:

```
User Input → Message Processing → API Call → Streaming →
  Tool Calls (if any) → Permission Checks → Results → Next Turn
```

**Key Types**:
- `QueryEngine`: Main struct owning the conversation
- `QueryEngineConfig`: Configuration (tools, API client, etc.)
- `Usage`: Token usage tracking

**When modifying**: Be careful with thread safety - uses `sync.RWMutex`.

---

### 2. Tool System

**Directory**: `internal/tools/`

Each tool implements the `Tool` interface:

```go
type Tool interface {
    Name() string
    Aliases() []string
    Description(ctx context.Context, input json.RawMessage, options types.ToolOptions) (string, error)
    InputSchema() types.ToolInputJSONSchema
    Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, 
         canUseTool types.CanUseToolFunc, parentMessage *types.Message, 
         onProgress func(progress interface{})) (*types.ToolResult, error)
    IsEnabled() bool
    IsConcurrencySafe(input json.RawMessage) bool
    IsReadOnly(input json.RawMessage) bool
    IsDestructive(input json.RawMessage) bool
    CheckPermissions(ctx context.Context, input json.RawMessage, context *types.ToolContext) (*types.PermissionResult, error)
    // ... more methods
}
```

**Adding a New Tool**:
1. Create a new file in `internal/tools/` (or add to existing)
2. Embed `BaseTool` for common functionality
3. Register in `NewToolRegistryWithOptions()` in `tools.go`
4. Add constants in `internal/constants/tools.go` if needed

**Existing Tools**:
- `BashTool`: Shell command execution (security-checked)
- `FileReadTool`, `FileWriteTool`, `FileEditTool`: File operations
- `GlobTool`, `GrepTool`: Search operations
- `AgentTool`: Sub-agent spawning
- `TaskListTool`, `TaskStopTool`, `TaskGetTool`, `TaskOutputTool`: Agent task management
- `TodoWriteTool`: Todo list
- `WebFetchTool`, `WebSearchTool`: Web access
- `MultiEditTool`: Batch edits
- `NotebookEditTool`: Jupyter notebook editing
- `ListMcpResourcesTool`, `ReadMcpResourceTool`: MCP integration
- `SkillTool`: Skill execution
- `LSPTool`: Language Server Protocol
- `ConfigTool`: Configuration management
- `SleepTool`: Pause execution
- `AskUserQuestionTool`: User interaction
- `BriefTool`: Send messages to user

---

### 3. Permission System

**Files**: 
- `internal/types/permissions.go`: Type definitions
- `internal/utils/bash_security.go`: Bash command security
- `internal/tools/path_security.go`: Path validation
- `internal/cli/app.go`: `canUseTool()` callback

**Permission Modes**:
- `default`: Ask for write/unsafe operations
- `acceptEdits`: Auto-approve file edits within CWD
- `bypassPermissions`: No approval needed (use carefully!)
- `plan`: Planning-only mode
- `dontAsk`: Deny all requests

**Adding a Permission Check**:
Implement `CheckPermissions()` on your tool, or use the path security utilities.

---

### 4. UI Layer

**Directory**: `internal/ui/`

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea):

- `AppModel`: Main TUI orchestrator
- `ChatModel`: Message history
- Components in `internal/ui/components/`

**Key Concepts**:
- Event-driven via `tea.Msg`
- Channels for cross-component communication
- Permission requests via channel

---

### 5. API Clients

**Directory**: `pkg/api/`

Both clients implement the same interface:

```go
type MessageClient interface {
    CreateMessage(context.Context, MessageRequest) (*MessageResponse, error)
    StreamMessage(context.Context, MessageRequest, func(StreamEvent) error) error
    CountTokens(context.Context, MessageRequest) (int, error)
}
```

**Anthropic**: Native Claude API support
**OpenAI-compatible**: Works with OpenAI, Llama, Ollama, etc.

---

### 6. Memory System

**Directory**: `internal/memory/`

- `memory.go`: Loads memories from `.claude/memory/`
- `session.go`: Maintains session summary
- `recall.go`: Selects relevant memories

**Memory File Format**: Markdown with YAML frontmatter

---

### 7. Conversation Compaction

**File**: `internal/services/compact.go`

Reduces conversation length while preserving context:
- Creates a summary of earlier messages
- Keeps recent messages intact
- Uses AI for high-quality summarization

---

## Testing

### Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./internal/query

# With verbose output
go test ./internal/tools -v
```

### Test Files

Tests are in `_test.go` files alongside the implementation:
- `internal/tools/tools_test.go`
- `internal/cli/app_test.go`
- `internal/query/engine_test.go`
- `pkg/api/client_test.go`, `pkg/api/openai_client_test.go`

---

## Code Style Guidelines

### General Go Practices

Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments) guide.

### Specific Conventions

1. **Error Handling**: Wrap errors with context using `fmt.Errorf("context: %w", err)`
2. **Context**: Pass `ctx` as first parameter to functions that need cancellation
3. **Concurrency**: Use `sync.RWMutex` for read-heavy state, `sync.Mutex` for write-heavy
4. **Type Definitions**: Put shared types in `internal/types/`
5. **Constants**: Put magic strings/numbers in `internal/constants/`

### Naming Patterns

- Interfaces: Descriptive names, often ending in `er` (`Tool`, `MessageClient`)
- Implementations: Concrete names (`BashTool`, `AnthropicClient`)
- Errors: `Err` prefix (`ErrToolNotFound`, `ErrPermissionDenied`)

---

## Common Tasks & Patterns

### Adding a New Slash Command

**File**: `internal/commands/commands.go`

1. Create a type implementing `CommandHandler`:
```go
type MyCommand struct {}

func (c *MyCommand) Name() string { return "mycommand" }
func (c *MyCommand) Description() string { return "Does something cool" }
func (c *MyCommand) IsEnabled() bool { return true }
func (c *MyCommand) IsHidden() bool { return false }
func (c *MyCommand) Execute(ctx context.Context, args string, context *CommandContext) (*CommandResult, error) {
    // Implementation
}
```

2. Register in `NewRegistry()`

---

### Working with the State Manager

**File**: `internal/state/manager.go`

```go
// Get the state manager
sm := state.NewStateManager()
sm.Initialize()

// Read (thread-safe)
model := sm.GetCurrentModel()

// Write (thread-safe, persists to disk)
sm.SetCurrentModel("claude-3-5-sonnet")

// Atomic updates
sm.UpdateState(func(s *AppState) {
    s.Custom["key"] = "value"
})
```

---

### Adding a New MIME Type or Language

**File**: `internal/tools/advanced_tools.go` → `DetectLanguageFromExtension()`

```go
languageMap[".ext"] = "language-name"
```

---

### Working with System Prompts

**File**: `internal/constants/prompts.go`

Modify carefully - these guide the AI's behavior.

---

## Debugging Tips

### Enable Debug Logging

```bash
go run ./cmd/cli --provider openai --debug
```

### Logging in Code

Use standard `log` package or create a debug logger:

```go
if debug {
    log.Printf("Debug: %v", value)
}
```

### Common Issues

1. **Permission Denied**: Check `path_security.go` to ensure paths are within allowed directories
2. **Token Limits**: Use conversation compaction or lower `max_tokens`
3. **API Errors**: Check retry logic in `pkg/api/client.go`
4. **UI Glitches**: Verify Bubble Tea model update logic in `internal/ui/app.go`

---

## Git Workflow

### Branching

```bash
# Create a feature branch
git checkout -b feature/my-feature

# Commit
git add .
git commit -m "feat: add my feature"
```

### Git Safety

Never:
- Modify `.git/config`
- Use `--no-verify` or `--no-gpg-sign` without explicit user request
- Amend commits without user request
- Commit secrets or `.env` files

---

## Project-Specific Files

These files help Claude Code understand how to work with this project:

- `CLAUDE.md` (this file): Project guidelines
- `AGENTS.md`: Agent architecture documentation
- `README.MD`: User-facing documentation

When you make architectural changes, update these files!

---

## Roadmap & Areas for Improvement

### Already Implemented

✅ Core query engine
✅ Basic tools (Read, Write, Edit, Bash, Glob, Grep)
✅ TUI with Bubble Tea
✅ Dual API support (Anthropic, OpenAI-compatible)
✅ Permission system
✅ Session memory
✅ Conversation compaction

### Partially Implemented

⏳ MCP integration (placeholders exist, need full implementation)
⏳ LSP integration (placeholder exists)
⏳ Plugin system (loader exists, need full implementation)
⏳ Voice interface (placeholder)
⏳ Notebook editing (placeholder)

### Future Ideas

💡 Agent collaboration (multiple agents working together)
💡 More specialized agent types
💡 Improved Vim mode
💡 Better visualization of token usage
💡 Plugin marketplace
💡 Remote development integration

---

## Getting Help

- Check `README.MD` for user documentation
- See `AGENTS.md` for agent architecture
- Read comments in the code - they're detailed!
- Look at the original TypeScript Claude Code for reference patterns

---

## Security Notes

This project handles potentially sensitive operations:

1. **Never commit secrets**: Check git status before committing
2. **Bash commands**: Security checked in `utils/bash_security.go`
3. **File paths**: Validated in `tools/path_security.go`
4. **Web access**: Restricted by `tools/web_security.go`
5. **Permission prompts**: Always err on the side of asking the user

When adding new features, think about security first!
