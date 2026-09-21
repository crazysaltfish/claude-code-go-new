# Claude Code Go - Agent Architecture

## Overview

This document describes the multi-agent architecture used by Claude Code Go. Agents are autonomous sub-processes that handle complex tasks in parallel or in the background.

## Agent Types

### 1. Main Agent (Coordinator)

The primary interactive agent that users communicate with directly.

**File**: `internal/cli/app.go`

**Responsibilities**:
- User interaction via the REPL
- Command parsing and execution
- Tool invocation management
- Conversation state management
- Sub-agent coordination

**Tools Available**:
- All core tools (Read, Write, Edit, Bash, Glob, Grep)
- Task management tools
- Agent spawning tool

---

### 2. General Purpose Agent

A flexible sub-agent for handling multi-step tasks autonomously.

**File**: `internal/tools/missing_tools.go` (AgentTool)

**Use Cases**:
- Complex refactoring tasks
- Multi-file analysis
- Long-running build processes
- Background research

**Configuration**:
```go
type AgentTool struct {
    description string  // Short task description
    prompt     string  // Full task prompt
    subagent_type string // Optional specialized type
    model      string  // Optional model override
    run_in_background bool // Whether to run in background
}
```

**Tools Available**:
- File operations (Read, Write, Edit)
- Bash execution
- Glob, Grep
- WebSearch (if enabled)
- Skill execution

---

### 3. Specialized Agents (Planned)

#### Code Review Agent
Focused on code quality, security analysis, and best practices.

**Tools**:
- Grep (pattern searching)
- Read (code analysis)
- WebSearch (looking up best practices)

#### Test Agent
Specialized in writing and running tests.

**Tools**:
- Read (analyzing existing tests)
- Write (creating new tests)
- Bash (running test suites)

#### Refactor Agent
Focused on structural code transformations.

**Tools**:
- Read
- Write
- Edit
- Bash (for build verification)

---

## Agent Communication

### Task-Based Coordination

**File**: `internal/tasks/`

Agents communicate through tasks stored in the task manager:

```go
type Task interface {
    GetID() string
    GetStatus() TaskStatus
    GetResult() interface{}
    GetError() error
}
```

### Task States

- `pending`: Task created but not started
- `in_progress`: Agent is working on the task
- `completed`: Task finished successfully
- `cancelled`: User stopped the task
- `failed`: Task encountered an error

---

## Agent Spawning

### Using the Agent Tool

The `Agent` tool spawns sub-agents with a specific task:

```go
// NewAgentTool creates the agent spawning tool
func NewAgentTool() *AgentTool {
    return &AgentTool{
        name: "Agent",
        description: "Spawn a sub-agent to handle complex, multi-step tasks",
        // ...
    }
}
```

**Example Usage**:
```
Use Agent to:
- description: Refactor authentication module
- prompt: |
    1. Read the current auth files in internal/auth/
    2. Extract the JWT handling logic
    3. Create a new separate JWT service
    4. Update imports in all files that use it
    5. Run tests to verify everything works
```

### Background vs Foreground

**Background Agents**:
- Run asynchronously
- User can continue interacting
- Results stored in task manager
- Check via `TaskOutput` tool

**Foreground Agents**:
- Block user interaction
- Used for short, critical tasks
- Stream results immediately

---

## Agent Isolation

### Working Directory

Each agent starts in the project root by default, but can be configured:

- **Agent-specific CWD**: Some agents need specific directories
- **Path validation**: Tools check path safety before execution
- **Memory context**: Agents get project-specific memory

### Permission Context

Agents inherit permissions from the parent but can be restricted:

```go
type PermissionContext struct {
    Mode          PermissionMode  // default, acceptEdits, bypassPermissions
    AllowedTools  []string        // Tools this agent can use
    RestrictedDirs []string       // Directories off-limits
}
```

### Tool Restrictions

Some tools are agent-disallowed by default:

```go
// AllAgentDisallowedTools cannot be used by any sub-agent
var AllAgentDisallowedTools = map[string]bool{
    "TaskOutput": true,
    "ExitPlanMode": true,
    "EnterPlanMode": true,
    "Agent": true,        // No recursive spawning by default
    "AskUser": true,      // Only main agent interacts with user
    "TaskStop": true,
}
```

---

## Agent Memory

### Session Memory

**File**: `internal/memory/session.go`

Each agent gets its own memory context:

- Project-level memory (from `.claude/memory/`)
- Session-specific memory (for this conversation)
- Read-only access to parent memory

### Memory Recall

Agents use the memory system to:
- Recall previous task patterns
- Learn project conventions
- Avoid repeating mistakes
- Build on prior work

---

## Agent Monitoring

### Task Manager Integration

**File**: `internal/tasks/`

Track agent progress through the task manager:

```go
// Task management tools
TaskCreate      // Create a new agent task
TaskList        // List all active/finished tasks
TaskGet         // Get task details
TaskStop        // Stop a running agent
TaskOutput      // Retrieve agent results
```

### Progress Updates

Agents can send progress updates:

- Through tool `onProgress` callbacks
- Stored in task state
- Visible in the UI spinner

---

## Agent Error Handling

### Recovery Strategies

1. **Auto-Retry**: Transient errors trigger automatic retries
2. **Fallback**: Agent can suggest alternative approaches
3. **Escalation**: Complex errors go back to the user
4. **Snapshot**: State preserved for debugging

### Error Types

```go
// Common agent errors
ErrAgentCancelled     // User stopped the agent
ErrToolPermission     // Tool access denied
ErrContextExpired     // Context deadline exceeded
ErrQuotaExhausted     // Token/budget limit hit
```

---

## Planned: Advanced Agent Features

### Agent Pool

Multiple agents working on different parts of a large task:
- Coordinator assigns sub-tasks
- Agents share results
- Load balancing across models

### Agent Collaboration

Multiple agents with different specialties:
- Code review agent + test agent
- Pair programming scenario
- Peer review patterns

### Agent Persistence

Save agent state to resume later:
- Checkpoint long-running tasks
- Resume from interruptions
- Agent result caching

### Agent Skill Library

Pre-built agent profiles:
- "Bug fixer" agent
- "Code generator" agent
- "Test writer" agent
- "Refactoring" agent

---

## Agent Best Practices

### When to Use Agents

**Good Candidates**:
- Tasks with > 5 tool calls
- Tasks spanning multiple files
- Background work (tests, builds)
- Repetitive patterns
- Research / exploration

**Stay in Main Agent**:
- Simple, focused changes
- Single-file edits
- Quick questions
- Direct user interaction

### Agent Prompt Design

Effective agent prompts should:
1. **Goal first**: Clear objective statement
2. **Step-by-step**: Actionable, ordered steps
3. **Constraints**: What NOT to do
4. **Success criteria**: How to know when done
5. **Tools preferred**: Which tools to use

Example good prompt:
```
Goal: Add error handling to the user service

Steps:
1. Read internal/users/service.go
2. Identify functions without proper error handling
3. Add structured errors with context
4. Update tests to verify error cases
5. Run tests to confirm

Constraints:
- Don't change function signatures
- Keep error messages user-friendly
- Use the internal/errors package

Success: All tests pass and errors provide helpful context
```

---

## Related Files

- `internal/cli/app.go` - Main application and coordination
- `internal/query/engine.go` - Query engine (used by agents)
- `internal/tools/missing_tools.go` - Agent tool implementation
- `internal/tasks/` - Task management system
- `internal/types/` - Type definitions
