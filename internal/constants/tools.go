package constants

// =============================================================================
// Tool Names
// =============================================================================

const (
	// Core file tools
	ToolFileRead  = "Read"
	ToolFileWrite = "Write"
	ToolFileEdit  = "Edit"
	ToolGlob      = "Glob"
	ToolGrep      = "Grep"

	// Shell tools
	ToolBash = "Bash"

	// Web tools
	ToolWebFetch  = "WebFetch"
	ToolWebSearch = "WebSearch"

	// Task tools
	ToolTaskCreate = "TaskCreate"
	ToolTaskOutput = "TaskOutput"
	ToolTaskStop   = "TaskStop"
	ToolTaskGet    = "TaskGet"
	ToolTaskList   = "TaskList"
	ToolTaskUpdate = "TaskUpdate"

	// Agent tools
	ToolAgent           = "Agent"
	ToolAskUser         = "AskUserQuestion"
	ToolSendMessage     = "SendMessage"
	ToolSkill           = "Skill"
	ToolSyntheticOutput = "SyntheticOutput"

	// Plan mode tools
	ToolEnterPlanMode = "EnterPlanMode"
	ToolExitPlanMode  = "ExitPlanMode"

	// Workflow tools
	ToolWorkflow      = "Workflow"
	ToolEnterWorktree = "EnterWorktree"
	ToolExitWorktree  = "ExitWorktree"

	// Todo tools
	ToolTodoWrite = "TodoWrite"

	// Schedule tools
	ToolCronCreate = "CronCreate"
	ToolCronDelete = "CronDelete"
	ToolCronList   = "CronList"

	// MCP tools
	ToolMCP              = "mcp__"
	ToolListMcpResources = "ListMcpResources"
	ToolReadMcpResource  = "ReadMcpResource"

	// Notebook tools
	ToolNotebookEdit = "NotebookEdit"

	// Config tool
	ToolConfig = "Config"

	// Tool search
	ToolToolSearch = "ToolSearch"

	// Sleep tool
	ToolSleep = "Sleep"

	// Brief tool
	ToolBrief = "Brief"

	// Discover skills tool
	ToolDiscoverSkills = "DiscoverSkills"
)

// =============================================================================
// Tool Groups
// =============================================================================

// AllAgentDisallowedTools are tools that cannot be used by sub-agents
var AllAgentDisallowedTools = map[string]bool{
	ToolTaskOutput:    true,
	ToolExitPlanMode:  true,
	ToolEnterPlanMode: true,
	ToolAgent:         true,
	ToolAskUser:       true,
	ToolTaskStop:      true,
	ToolWorkflow:      true,
}

// CustomAgentDisallowedTools are tools that cannot be used by custom agents
var CustomAgentDisallowedTools = map[string]bool{
	ToolTaskOutput:    true,
	ToolExitPlanMode:  true,
	ToolEnterPlanMode: true,
	ToolAgent:         true,
	ToolAskUser:       true,
	ToolTaskStop:      true,
	ToolWorkflow:      true,
}

// AsyncAgentAllowedTools are tools allowed for async agents
var AsyncAgentAllowedTools = map[string]bool{
	ToolFileRead:        true,
	ToolWebSearch:       true,
	ToolTodoWrite:       true,
	ToolGrep:            true,
	ToolWebFetch:        true,
	ToolGlob:            true,
	ToolBash:            true,
	ToolFileEdit:        true,
	ToolFileWrite:       true,
	ToolNotebookEdit:    true,
	ToolSkill:           true,
	ToolSyntheticOutput: true,
	ToolToolSearch:      true,
	ToolEnterWorktree:   true,
	ToolExitWorktree:    true,
}

// InProcessTeammateAllowedTools are tools allowed only for in-process teammates
var InProcessTeammateAllowedTools = map[string]bool{
	ToolTaskCreate:  true,
	ToolTaskGet:     true,
	ToolTaskList:    true,
	ToolTaskUpdate:  true,
	ToolSendMessage: true,
	ToolCronCreate:  true,
	ToolCronDelete:  true,
	ToolCronList:    true,
}

// CoordinatorModeAllowedTools are tools allowed in coordinator mode
var CoordinatorModeAllowedTools = map[string]bool{
	ToolAgent:           true,
	ToolTaskStop:        true,
	ToolSendMessage:     true,
	ToolSyntheticOutput: true,
}

// =============================================================================
// Tool Limits
// =============================================================================

const (
	// Maximum result size in characters
	MaxResultSizeChars = 100000

	// Maximum file size to read (10MB)
	MaxFileSizeToRead = 10 * 1024 * 1024

	// Default command timeout in milliseconds
	DefaultCommandTimeoutMs = 120000 // 2 minutes

	// Maximum command timeout in milliseconds
	MaxCommandTimeoutMs = 600000 // 10 minutes

	// Default file read limit
	DefaultFileReadLimit = 2000

	// Maximum grep output lines
	MaxGrepOutputLines = 500
)

// =============================================================================
// Tool Descriptions
// =============================================================================

const (
	// DescBash is the complete description for the Bash tool
	DescBash = `Executes a given bash command and returns its output.

The working directory persists between commands, but shell state does not. The shell environment is initialized from the user's profile (bash or zsh).

IMPORTANT: Avoid using this tool to run ` + "`" + `find` + "`" + `, ` + "`" + `grep` + "`" + `, ` + "`" + `cat` + "`" + `, ` + "`" + `head` + "`" + `, ` + "`" + `tail` + "`" + `, ` + "`" + `sed` + "`" + `, ` + "`" + `awk` + "`" + `, or ` + "`" + `echo` + "`" + ` commands, unless explicitly instructed or after you have verified that a dedicated tool cannot accomplish your task. Instead, use the appropriate dedicated tool as this will provide a much better experience for the user:

 - File search: Use Glob (NOT find or ls)
 - Content search: Use Grep (NOT grep or rg)
 - Read files: Use Read (NOT cat/head/tail)
 - Edit files: Use Edit (NOT sed/awk)
 - Write files: Use Write (NOT echo >/cat <<EOF)
 - Communication: Output text directly (NOT echo/printf)

While the Bash tool can do similar things, it's better to use the built-in tools as they provide a better user experience and make it easier to review tool calls and give permission.

# Instructions

 - If your command will create new directories or files, first use this tool to run ` + "`" + `ls` + "`" + ` to verify the parent directory exists and is the correct location.
 - Always quote file paths that contain spaces with double quotes in your command (e.g., cd "path with spaces/file.txt")
 - Try to maintain your current working directory throughout the session by using absolute paths and avoiding usage of ` + "`" + `cd` + "`" + `. You may use ` + "`" + `cd` + "`" + ` if the User explicitly requests it.
 - You may specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). By default, your command will timeout after 120000ms (2 minutes).
 - You can use the ` + "`" + `is_background` + "`" + ` parameter to run the command in the background. Only use this if you don't need the result immediately and are OK being notified when the command completes later. You do not need to check the output right away - you'll be notified when it finishes. You do not need to use '&' at the end of the command when using this parameter.
 - When issuing multiple commands:
   - If the commands are independent and can run in parallel, make multiple Bash tool calls in a single message. Example: if you need to run "git status" and "git diff", send a single message with two Bash tool calls in parallel.
   - If the commands depend on each other and must run sequentially, use a single Bash call with '&&' to chain them together.
   - Use ';' only when you need to run commands sequentially but don't care if earlier commands fail.
   - DO NOT use newlines to separate commands (newlines are ok in quoted strings).
 - For git commands:
   - Prefer to create a new commit rather than amending an existing commit.
   - Before running destructive operations (e.g., git reset --hard, git push --force, git checkout --), consider whether there is a safer alternative that achieves the same goal. Only use destructive operations when they are truly the best approach.
   - Never skip hooks (--no-verify) or bypass signing (--no-gpg-sign, -c commit.gpgsign=false) unless the user has explicitly asked for it. If a hook fails, investigate and fix the underlying issue.
 - Avoid unnecessary ` + "`" + `sleep` + "`" + ` commands:
   - Do not sleep between commands that can run immediately — just run them.
   - If your command is long running and you would like to be notified when it finishes — use ` + "`" + `is_background` + "`" + `. No sleep needed.
   - Do not retry failing commands in a sleep loop — diagnose the root cause.
   - If waiting for a background task you started with ` + "`" + `is_background` + "`" + `, you will be notified when it completes — do not poll.

# Committing changes with git

Only create commits when requested by the user. If unclear, ask first. When the user asks you to create a new git commit, follow these steps carefully:

Git Safety Protocol:
 - NEVER update the git config
 - NEVER run destructive git commands (push --force, reset --hard, checkout ., restore ., clean -f, branch -D) unless the user explicitly requests these actions.
 - NEVER skip hooks (--no-verify, --no-gpg-sign, etc) unless the user explicitly requests it
 - NEVER run force push to main/master, warn the user if they request it
 - CRITICAL: Always create NEW commits rather than amending, unless the user explicitly requests a git amend. When a pre-commit hook fails, the commit did NOT happen — so --amend would modify the PREVIOUS commit, which may result in destroying work or losing previous changes. Instead, after hook failure, fix the issue, re-stage, and create a NEW commit
 - When staging files, prefer adding specific files by name rather than using "git add -A" or "git add .", which can accidentally include sensitive files (.env, credentials) or large binaries
 - NEVER commit changes unless the user explicitly asks you to. It is VERY IMPORTANT to only commit when explicitly asked.

1. Run the following bash commands in parallel:
   - Run a git status command to see all untracked files. IMPORTANT: Never use the -uall flag as it can cause memory issues on large repos.
   - Run a git diff command to see both staged and unstaged changes that will be committed.
   - Run a git log command to see recent commit messages, so that you can follow this repository's commit message style.
2. Analyze all staged changes and draft a commit message:
   - Summarize the nature of the changes (eg. new feature, enhancement, bug fix, refactoring, test, docs, etc.)
   - Do not commit files that likely contain secrets (.env, credentials.json, etc). Warn the user if they specifically request to commit those files
   - Draft a concise (1-2 sentences) commit message that focuses on the "why" rather than the "what"
3. Run the following commands in parallel:
   - Add relevant untracked files to the staging area.
   - Create the commit with a message.
   - Run git status after the commit completes to verify success.

# Creating pull requests

Use the gh command via the Bash tool for ALL GitHub-related tasks including working with issues, pull requests, checks, and releases.

When the user asks you to create a pull request:

1. Run the following bash commands in parallel:
   - Run a git status command to see all untracked files
   - Run a git diff command to see both staged and unstaged changes
   - Check if the current branch tracks a remote branch and is up to date
   - Run a git log and ` + "`" + `git diff [base-branch]...HEAD` + "`" + ` to understand the full commit history
2. Analyze all changes and draft a PR title and summary:
   - Keep the PR title short (under 70 characters)
   - Use the description/body for details, not the title
3. Run the following commands in parallel:
   - Create new branch if needed
   - Push to remote with -u flag if needed
   - Create PR using gh pr create`

	DescRead = `Reads a file from the local filesystem. You can access any file directly by using this tool.
Assume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- By default, it reads up to 2000 lines starting from the beginning of the file
- You can optionally specify a line offset and limit (especially handy for long files), but it's recommended to read the whole file by not providing these parameters
- Results are returned using cat -n format, with line numbers starting at 1
- This tool allows Claude Code to read images (eg PNG, JPG, etc). When reading an image file the contents are presented visually as Claude Code is a multimodal LLM.
- This tool can read PDF files (.pdf). For large PDFs (more than 10 pages), you MUST provide the pages parameter to read specific page ranges (e.g., pages: "1-5"). Reading a large PDF without the pages parameter will fail. Maximum 20 pages per request.
- This tool can read Jupyter notebooks (.ipynb files) and returns all cells with their outputs, combining code, text, and visualizations.
- This tool can only read files, not directories. To read a directory, use an ls command via the Bash tool.
- You will regularly be asked to read screenshots. If the user provides a path to a screenshot, ALWAYS use this tool to view the file at the path. This tool will work with all temporary file paths.
- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.

Parameters:
- target_file: The file path to read (must be an absolute path)
- offset: Optional line number to start reading from
- limit: Optional number of lines to read (max 2000)
- pages: Optional page range for PDF files (e.g., "1-5", "1,3,5")`

	DescWrite = `Writes a file to the local filesystem, overwriting if exists.

Usage Guidelines:
- Always prefer editing existing files; NEVER write new files unless explicitly required
- NEVER proactively create documentation files (*.md) or README files
- Only create documentation if explicitly requested by the User

Parameters:
- file_path: The path to write to (relative to workspace or absolute)
- contents: The content to write

File Handling:
- This tool will overwrite existing files if they exist
- For existing files, MUST read the file first to understand current content
- ALWAYS prefer editing existing files over creating new ones`

	DescEdit = `Performs exact string replacements in files.

Usage:
- You must use your Read tool at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file.
- When editing text from Read tool output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. The line number prefix format is: line number + tab. Everything after that is the actual file content to match. Never include any part of the line number prefix in the old_string or new_string.
- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.
- Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.
- The edit will FAIL if old_string is not unique in the file. Either provide a larger string with more surrounding context to make it unique or use replace_all to change every instance of old_string.
- Use replace_all for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.

Parameters:
- file_path: The target file to modify (must be an absolute path)
- old_string: The text to replace (must match exactly including whitespace)
- new_string: The replacement text (must be different from old_string)
- replace_all: Set true to replace all occurrences (default false)

Editing Rules:
- The edit will FAIL if old_string is not unique
- Provide larger context string to make it unique if needed
- Use replace_all for replacing/renaming across the file

Critical Requirements:
- Use the smallest old_string that's clearly unique — usually 2-4 adjacent lines is sufficient. Avoid including 10+ lines of context when less uniquely identifies the target.
- Edits are atomic: either all succeed or none are applied
- Plan edits carefully to avoid conflicts in sequential operations`

	DescGlob = `Searches for files matching a glob pattern.

Usage:
- Works fast with codebases of any size
- Returns matching file paths sorted by modification time
- Use when you need to find files by name patterns

Parameters:
- pattern: The glob pattern to match files against
  (Patterns not starting with "**/" are automatically prepended with "**/")
- target_directory: Optional directory to search in (defaults to workspace root)

Pattern Examples:
- "*.js" → finds all .js files (becomes "**/*.js")
- "**/node_modules/**" → finds all node_modules directories
- "**/test/**/test_*.ts" → finds test_*.ts files in any test directory

Guidelines:
- Always provide ONE directory or file path; [] searches whole repo
- No globs or wildcards for target_directory parameter
- Bad: ["frontend/", "backend/"] (multiple paths)
- Bad: ["src/**/utils/**"] (globs)
- Bad: ["*.ts"] or ["**/*"] (wildcard paths)`

	DescGrep = `A powerful search tool built on ripgrep.

Usage:
- ALWAYS use Grep for search tasks. NEVER invoke grep or rg as a Bash command. The Grep tool has been optimized for correct permissions and access.
- Supports full regex syntax (e.g., "log.*Error", "function\\s+\\w+")
- Filter files with glob parameter (e.g., "*.js", "**/*.tsx") or type parameter (e.g., "js", "py", "rust")
- Output modes: "content" shows matching lines, "files_with_matches" shows only file paths (default), "count" shows match counts
- Use Agent tool for open-ended searches requiring multiple rounds
- Pattern syntax: Uses ripgrep (not grep) - literal braces need escaping (use interface\\{\\} to find interface{} in Go code)
- Multiline matching: By default patterns match within single lines only. For cross-line patterns like struct \\{[\\s\\S]*?field, use multiline: true

Parameters:
- pattern: The regular expression pattern to search (rg --regexp)
- path: Optional file or directory to search in (defaults to workspace root)
- glob: Optional glob pattern to filter files (e.g., "*.js", "*.{ts,tsx}")
- type: Optional file type filter (e.g., "js", "py", "rust")
- output_mode: "content", "files_with_matches", or "count"
- case_insensitive: Set true for case-insensitive search
- multiline: Set true for cross-line pattern matching

Output Modes:
- "content": Shows matching lines with line numbers and context (default)
- "files_with_matches": Shows only file paths
- "count": Shows match counts

Guidelines:
- Avoid overly broad glob patterns (--glob *) that bypass .gitignore
- Note: import paths may not match source file types (.js vs .ts)
- Results are capped for responsiveness`

	DescWebFetch = `Fetches content from a specified URL and processes it using an AI model.

Overview:
- Takes a URL and a prompt as input
- Fetches the URL content, converts HTML to markdown
- Processes the content with the prompt using a small, fast model
- Returns the model's response about the content
- Use when you need to retrieve and analyze web content

Usage Notes:
- IMPORTANT: If an MCP-provided web fetch tool is available, prefer using that tool instead of this one, as it may have fewer restrictions.
- The URL must be a fully-formed valid URL
- HTTP URLs will be automatically upgraded to HTTPS
- The prompt should describe what information you want to extract from the page
- This tool is read-only and does not modify any files
- Results may be summarized if the content is very large
- Includes a self-cleaning 15-minute cache for faster responses when repeatedly accessing the same URL
- When a URL redirects to a different host, the tool will inform you and provide the redirect URL in a special format. You should then make a new WebFetch request with the redirect URL to fetch the content.
- For GitHub URLs, prefer using the gh CLI via Bash instead (e.g., gh pr view, gh issue view, gh api).

Parameters:
- url: The URL to fetch (must be a fully-formed valid URL)
- prompt: The prompt describing what information to extract from the page`

	// DescTodoWrite is the complete description for the TodoWrite tool
	DescTodoWrite = `Use this tool to create and manage a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user.
It also helps the user understand the progress of the task and overall progress of their requests.

## When to Use This Tool
Use this tool proactively in these scenarios:

1. Complex multi-step tasks - When a task requires 3 or more distinct steps or actions
2. Non-trivial and complex tasks - Tasks that require careful planning or multiple operations
3. User explicitly requests todo list - When the user directly asks you to use the todo list
4. User provides multiple tasks - When users provide a list of things to be done (numbered or comma-separated)
5. After receiving new instructions - Immediately capture user requirements as todos
6. When you start working on a task - Mark it as in_progress BEFORE beginning work. Ideally you should only have one todo as in_progress at a time
7. After completing a task - Mark it as completed and add any new follow-up tasks discovered during implementation

## When NOT to Use This Tool

Skip using this tool when:
1. There is only a single, straightforward task
2. The task is trivial and tracking it provides no organizational benefit
3. The task can be completed in less than 3 trivial steps
4. The task is purely conversational or informational

NOTE that you should not use this tool if there is only one trivial task to do. In this case you are better off just doing the task directly.

## Task States and Management

1. **Task States**: Use these states to track progress:
   - pending: Task not yet started
   - in_progress: Currently working on (limit to ONE task at a time)
   - completed: Task finished successfully
   - cancelled: No longer needed

   **IMPORTANT**: Task descriptions must have two forms:
   - content: The imperative form describing what needs to be done (e.g., "Run tests", "Build the project")
   - activeForm: The present continuous form shown during execution (e.g., "Running tests", "Building the project")

2. **Task Management**:
   - Update task status in real-time as you work
   - Mark tasks complete IMMEDIATELY after finishing (don't batch completions)
   - Exactly ONE task must be in_progress at any time (not less, not more)
   - Complete current tasks before starting new ones
   - Remove tasks that are no longer relevant from the list entirely

3. **Task Completion Requirements**:
   - ONLY mark a task as completed when you have FULLY accomplished it
   - If you encounter errors, blockers, or cannot finish, keep the task as in_progress
   - When blocked, create a new task describing what needs to be resolved
   - Never mark a task as completed if:
     - Tests are failing
     - Implementation is partial
     - You encountered unresolved errors
     - You couldn't find necessary files or dependencies

4. **Task Breakdown**:
   - Create specific, actionable items
   - Break complex tasks into smaller, manageable steps
   - Use clear, descriptive task names
   - Always provide both forms:
     - content: "Fix authentication bug"
     - activeForm: "Fixing authentication bug"

When in doubt, use this tool. Being proactive with task management demonstrates attentiveness and ensures you complete all requirements successfully.`

	// DescTask is the complete description for the Agent/Task tool
	DescTask = `Launch a new agent to handle complex, multi-step tasks autonomously.

The Agent tool launches specialized agents (subprocesses) that autonomously handle complex tasks. Each agent type has specific capabilities and tools available to it.

Available agent types and the tools they have access to:
- general-agent: General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks (Tools: All tools)
- explorer-agent: Fast agent specialized for exploring codebases. Use when you need to quickly find files by patterns, search code for keywords, or answer questions about the codebase (Tools: All tools)

When using the Agent tool, specify a subagent_type parameter to select which agent type to use. If omitted, the general-purpose agent is used.

When NOT to use the Agent tool:
- If you want to read a specific file path, use the Read tool or Glob tool instead of the Agent tool, to find the match more quickly
- If you are searching for a specific class definition like "class Foo", use the Glob tool instead, to find the match more quickly
- If you are searching for code within a specific file or set of 2-3 files, use the Read tool instead of the Agent tool, to find the match more quickly
- Other tasks that are not related to the agent descriptions above

Usage notes:
- Always include a short description (3-5 words) summarizing what the agent will do
- Launch multiple agents concurrently whenever possible, to maximize performance; to do that, use a single message with multiple tool uses
- When the agent is done, it will return a single message back to you. The result returned by the agent is not visible to the user. To show the user the result, you should send a text message back to the user with a concise summary of the result.
- You can optionally run agents in the background using the is_background parameter. When an agent runs in the background, you will be automatically notified when it completes - do NOT sleep, poll, or proactively check on its progress. Continue with other work or respond to the user instead.
- Foreground vs background: Use foreground (default) when you need the agent's results before you can proceed - e.g., research agents whose findings inform your next steps. Use background when you have genuinely independent work to do in parallel.
- To continue a previously spawned agent, use SendMessage with the agent's ID or name as the "to" field. The agent resumes with its full context preserved.
- Each Agent invocation starts fresh - provide a complete task description.
- The agent's outputs should generally be trusted
- Clearly tell the agent whether you expect it to write code or just to do research (search, file reads, web fetches, etc.)
- If the agent description mentions that it should be used proactively, then you should try your best to use it without the user having to ask for it first. Use your judgement.
- If the user specifies that they want you to run agents "in parallel", you MUST send a single message with multiple Agent tool use content blocks.

## Writing the prompt

Brief the agent like a smart colleague who just walked into the room - it hasn't seen this conversation, doesn't know what you've tried, doesn't understand why this task matters.
- Explain what you're trying to accomplish and why.
- Describe what you've already learned or ruled out.
- Give enough context about the surrounding problem that the agent can make judgment calls rather than just following a narrow instruction.
- If you need a short response, say so ("report in under 200 words").
- Lookups: hand over the exact command. Investigations: hand over the question - prescribed steps become dead weight when the premise is wrong.

Terse command-style prompts produce shallow, generic work.

**Never delegate understanding.** Don't write "based on your findings, fix the bug" or "based on the research, implement it." Those phrases push synthesis onto the agent instead of doing it yourself. Write prompts that prove you understood: include file paths, line numbers, what specifically to change.

Example usage:

<example>
user: "Please write a function that checks if a number is prime"
assistant: I'm going to use the Write tool to write the following code:
<code>
function isPrime(n) {
  if (n <= 1) return false
  for (let i = 2; i * i <= n; i++) {
    if (n % i === 0) return false
  }
  return true
}
</code>
<commentary>
Since a significant piece of code was written and the task was completed, now use the test-runner agent to run the tests
</commentary>
assistant: Uses the Agent tool to launch the test-runner agent
</example>`
)

// =============================================================================
// Tool Input Schemas
// =============================================================================

// GetBashToolSchema returns the input schema for the Bash tool
func GetBashToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"minLength":   1,
				"description": "The command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"minimum":     1,
				"maximum":     MaxCommandTimeoutMs,
				"description": "Timeout in milliseconds",
			},
			"is_background": map[string]interface{}{
				"type":        "boolean",
				"description": "Run command in background",
			},
		},
		"required": []string{"command"},
	}
}

// GetReadToolSchema returns the input schema for the Read tool
func GetReadToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target_file": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path of the file to read",
			},
			"offset": map[string]interface{}{
				"type":        "number",
				"description": "Line number to start reading from",
			},
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Number of lines to read (max 2000)",
			},
			"pages": map[string]interface{}{
				"type":        "string",
				"description": "Page range for PDF files (e.g., \"1-5\", \"1,3,5\")",
			},
		},
		"required": []string{"target_file"},
	}
}

// GetWriteToolSchema returns the input schema for the Write tool
func GetWriteToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The path of the file to write",
			},
			"contents": map[string]interface{}{
				"type":        "string",
				"description": "The contents to write",
			},
		},
		"required": []string{"file_path", "contents"},
	}
}

// GetEditToolSchema returns the input schema for the Edit tool
func GetEditToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The path of the file to modify",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace with",
			},
			"replace_all": map[string]interface{}{
				"type":        "boolean",
				"description": "Replace all occurrences",
			},
		},
		"required": []string{"file_path", "old_string", "new_string"},
	}
}

// GetGlobToolSchema returns the input schema for the Glob tool
func GetGlobToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The glob pattern to match",
			},
			"target_directory": map[string]interface{}{
				"type":        "string",
				"description": "The directory to search in",
			},
		},
		"required": []string{"pattern"},
	}
}

// GetGrepToolSchema returns the input schema for the Grep tool
func GetGrepToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The regular expression pattern to search",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File or directory to search in",
			},
			"glob": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern to filter files (e.g., \"*.js\", \"**/*.tsx\")",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "File type filter (e.g., \"js\", \"py\", \"rust\")",
			},
			"output_mode": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"content", "files_with_matches", "count"},
				"description": "Output mode",
			},
			"case_insensitive": map[string]interface{}{
				"type":        "boolean",
				"description": "Case-insensitive search",
			},
			"multiline": map[string]interface{}{
				"type":        "boolean",
				"description": "Enable cross-line pattern matching",
			},
		},
		"required": []string{"pattern"},
	}
}

// GetWebFetchToolSchema returns the input schema for the WebFetch tool
func GetWebFetchToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"urls": map[string]interface{}{
				"type":        "array",
				"minItems":    1,
				"items":       map[string]interface{}{"type": "string"},
				"description": "HTTPS URLs to fetch",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "The prompt describing what information to extract from the page",
			},
		},
		"required": []string{"urls"},
	}
}

// GetTodoWriteToolSchema returns the input schema for the TodoWrite tool
func GetTodoWriteToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"todos": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Unique identifier",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Task description",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
							"description": "Task status",
						},
					},
					"required": []string{"id", "content", "status"},
				},
				"description": "Array of todo items",
			},
			"merge": map[string]interface{}{
				"type":        "boolean",
				"description": "Merge with existing todos",
			},
		},
		"required": []string{"todos"},
	}
}

// GetTaskToolSchema returns the input schema for the Task tool
func GetTaskToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Short task description",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Detailed task instructions",
			},
			"subagent_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"general-agent", "explorer-agent"},
				"description": "Type of agent to use",
			},
			"model": map[string]interface{}{
				"type":        "number",
				"description": "Model to use",
			},
		},
		"required": []string{"description", "prompt", "subagent_type"},
	}
}

// =============================================================================
// Advanced Tool Descriptions
// =============================================================================

const (
	// DescEnterPlanMode is the complete description for the EnterPlanMode tool
	DescEnterPlanMode = `Use this tool proactively when you're about to start a non-trivial implementation task. Getting user sign-off on your approach before writing code prevents wasted effort and ensures alignment. This tool transitions you into plan mode where you can explore the codebase and design an implementation approach for user approval.

## When to Use This Tool

**Prefer using EnterPlanMode** for implementation tasks unless they're simple. Use it when ANY of these conditions apply:

1. **New Feature Implementation**: Adding meaningful new functionality
   - Example: "Add a logout button" - where should it go? What should happen on click?
   - Example: "Add form validation" - what rules? What error messages?

2. **Multiple Valid Approaches**: The task can be solved in several different ways
   - Example: "Add caching to the API" - could use Redis, in-memory, file-based, etc.
   - Example: "Improve performance" - many optimization strategies possible

3. **Code Modifications**: Changes that affect existing behavior or structure
   - Example: "Update the login flow" - what exactly should change?
   - Example: "Refactor this component" - what's the target architecture?

4. **Architectural Decisions**: The task requires choosing between patterns or technologies
   - Example: "Add real-time updates" - WebSockets vs SSE vs polling
   - Example: "Implement state management" - Redux vs Context vs custom solution

5. **Multi-File Changes**: The task will likely touch more than 2-3 files
   - Example: "Refactor the authentication system"
   - Example: "Add a new API endpoint with tests"

6. **Unclear Requirements**: You need to explore before understanding the full scope
   - Example: "Make the app faster" - need to profile and identify bottlenecks
   - Example: "Fix the bug in checkout" - need to investigate root cause

7. **User Preferences Matter**: The implementation could reasonably go multiple ways
   - If you would use AskUserQuestion to clarify the approach, use EnterPlanMode instead
   - Plan mode lets you explore first, then present options with context

## When NOT to Use This Tool

Only skip EnterPlanMode for simple tasks:
- Single-line or few-line fixes (typos, obvious bugs, small tweaks)
- Adding a single function with clear requirements
- Tasks where the user has given very specific, detailed instructions
- Pure research/exploration tasks (use the Agent tool with explore agent instead)

## What Happens in Plan Mode

In plan mode, you'll:
1. Thoroughly explore the codebase using Glob, Grep, and Read tools
2. Understand existing patterns and architecture
3. Design an implementation approach
4. Present your plan to the user for approval
5. Use AskUserQuestion if you need to clarify approaches
6. Exit plan mode with ExitPlanMode when ready to implement

## Important Notes

- This tool REQUIRES user approval - they must consent to entering plan mode
- If unsure whether to use it, err on the side of planning - it's better to get alignment upfront than to redo work
- Users appreciate being consulted before significant changes are made to their codebase`

	// DescExitPlanMode is the complete description for the ExitPlanMode tool
	DescExitPlanMode = `Use this tool to exit plan mode after you have finished planning and are ready to implement your plan.

## When to Use This Tool

1. You have thoroughly explored the codebase and understand the context
2. You have designed a clear implementation approach
3. You are ready to present your plan to the user for approval
4. The user has approved your plan and you want to start implementation

## Parameters

- plan_summary: A concise summary of your implementation plan (optional but recommended)

## Important Notes

- This tool signals that you're ready to implement
- Include a clear summary of what you plan to do
- The user will have a chance to review and approve before you proceed`

	// DescAskUserQuestion is the complete description for the AskUserQuestion tool
	DescAskUserQuestion = `Use this tool when you need to ask the user questions during execution. This allows you to:
1. Gather user preferences or requirements
2. Clarify ambiguous instructions
3. Get decisions on implementation choices as you work
4. Offer choices to the user about what direction to take.

Usage notes:
- Users will always be able to select "Other" to provide custom text input
- Use multiSelect: true to allow multiple answers to be selected for a question
- If you recommend a specific option, make that the first option in the list and add "(Recommended)" at the end of the label

Plan mode note: In plan mode, use this tool to clarify requirements or choose between approaches BEFORE finalizing your plan. Do NOT use this tool to ask "Is my plan ready?" or "Should I proceed?" - use ExitPlanMode for plan approval. IMPORTANT: Do not reference "the plan" in your questions (e.g., "Do you have feedback about the plan?", "Does the plan look good?") because the user cannot see the plan in the UI until you call ExitPlanMode. If you need plan approval, use ExitPlanMode instead.`
)

// GetEnterPlanModeToolSchema returns the input schema for the EnterPlanMode tool
func GetEnterPlanModeToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"goal": map[string]interface{}{
				"type":        "string",
				"description": "The goal to plan for",
			},
		},
		"required": []string{"goal"},
	}
}

// GetExitPlanModeToolSchema returns the input schema for the ExitPlanMode tool
func GetExitPlanModeToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"plan_summary": map[string]interface{}{
				"type":        "string",
				"description": "Optional summary of the plan",
			},
		},
	}
}

// GetAskUserQuestionToolSchema returns the input schema for the AskUserQuestion tool
func GetAskUserQuestionToolSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"question": map[string]interface{}{
				"type":        "string",
				"description": "The question to ask the user",
			},
			"suggestions": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional suggested answers",
			},
			"multiSelect": map[string]interface{}{
				"type":        "boolean",
				"description": "Allow multiple answers to be selected",
			},
		},
		"required": []string{"question"},
	}
}
