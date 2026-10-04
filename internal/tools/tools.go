package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
// Helper Functions
// =============================================================================

// convertSchema converts a map[string]interface{} to ToolInputJSONSchema
func convertSchema(schema map[string]interface{}) types.ToolInputJSONSchema {
	result := types.ToolInputJSONSchema{}
	if t, ok := schema["type"].(string); ok {
		result.Type = t
	}
	if props, ok := schema["properties"].(map[string]interface{}); ok {
		result.Properties = make(map[string]map[string]interface{})
		for k, v := range props {
			if m, ok := v.(map[string]interface{}); ok {
				result.Properties[k] = m
			}
		}
	}
	if req, ok := schema["required"].([]string); ok {
		result.Required = req
	} else if req, ok := schema["required"].([]interface{}); ok {
		result.Required = make([]string, 0, len(req))
		for _, r := range req {
			if s, ok := r.(string); ok {
				result.Required = append(result.Required, s)
			}
		}
	}
	return result
}

// =============================================================================
// Base Tool
// =============================================================================

// BaseTool provides common functionality for all tools.
type BaseTool struct {
	name          string
	aliases       []string
	description   string
	inputSchema   types.ToolInputJSONSchema
	isEnabled     bool
	isReadOnly    bool
	isDestructive bool
}

// NewBaseTool creates a new base tool.
func NewBaseTool(name string, description string) *BaseTool {
	return &BaseTool{
		name:        name,
		description: description,
		isEnabled:   true,
		isReadOnly:  false,
	}
}

func (t *BaseTool) Name() string      { return t.name }
func (t *BaseTool) Aliases() []string { return t.aliases }
func (t *BaseTool) Description(ctx context.Context, input json.RawMessage, options types.ToolOptions) (string, error) {
	return t.description, nil
}
func (t *BaseTool) InputSchema() types.ToolInputJSONSchema       { return t.inputSchema }
func (t *BaseTool) IsEnabled() bool                              { return t.isEnabled }
func (t *BaseTool) IsConcurrencySafe(input json.RawMessage) bool { return false }
func (t *BaseTool) IsReadOnly(input json.RawMessage) bool        { return t.isReadOnly }
func (t *BaseTool) IsDestructive(input json.RawMessage) bool     { return t.isDestructive }

func (t *BaseTool) CheckPermissions(ctx context.Context, input json.RawMessage, context *types.ToolContext) (*types.PermissionResult, error) {
	return &types.PermissionResult{
		Behavior: types.PermissionBehaviorAllow,
	}, nil
}

func (t *BaseTool) UserFacingName(input json.RawMessage) string {
	return t.name
}

func (t *BaseTool) MapToolResultToAPI(content interface{}, toolUseID string) (interface{}, error) {
	return map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     content,
	}, nil
}

func (t *BaseTool) RenderToolUseMessage(input json.RawMessage, options types.ToolRenderOptions) string {
	return fmt.Sprintf("Using tool: %s", t.name)
}

func (t *BaseTool) MaxResultSizeChars() int {
	return constants.MaxResultSizeChars
}

// =============================================================================
// Bash Tool
// =============================================================================

// BashTool executes shell commands.
type BashTool struct {
	*BaseTool
}

// NewBashTool creates a new Bash tool.
func NewBashTool() *BashTool {
	return &BashTool{
		BaseTool: &BaseTool{
			name:          constants.ToolBash,
			description:   constants.DescBash,
			inputSchema:   convertSchema(constants.GetBashToolSchema()),
			isEnabled:     true,
			isReadOnly:    false,
			isDestructive: true,
		},
	}
}

// Call executes the bash command.
func (t *BashTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Command      string `json:"command"`
		Timeout      int    `json:"timeout,omitempty"`
		IsBackground bool   `json:"is_background,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
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

	// Set timeout
	timeout := input.Timeout
	if timeout == 0 {
		timeout = constants.DefaultCommandTimeoutMs
	}
	if timeout > constants.MaxCommandTimeoutMs {
		timeout = constants.MaxCommandTimeoutMs
	}

	timeoutDuration := time.Duration(timeout) * time.Millisecond
	execCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	// Determine shell based on OS
	shell := "/bin/sh"
	shellFlag := "-c"
	if shellPath := os.Getenv("SHELL"); shellPath != "" {
		shell = shellPath
	}

	cmd := exec.CommandContext(execCtx, shell, shellFlag, input.Command)
	if toolCtx != nil && toolCtx.Cwd != "" {
		cmd.Dir = toolCtx.Cwd
	} else if cwd, err := os.Getwd(); err == nil {
		cmd.Dir = cwd
	}

	// Set environment
	cmd.Env = os.Environ()

	// Handle background execution
	if input.IsBackground {
		if err := cmd.Start(); err != nil {
			return &types.ToolResult{
				Error:     fmt.Errorf("failed to start command: %w", err),
				ToolUseID: toolCtx.ToolUseId,
			}, nil
		}
		return &types.ToolResult{
			Output:    fmt.Sprintf("Command started in background (PID: %d)", cmd.Process.Pid),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &types.ToolResult{
			Output:    string(output),
			Error:     fmt.Errorf("command failed: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    string(output),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

func (t *BashTool) IsConcurrencySafe(input json.RawMessage) bool { return false }

// =============================================================================
// File Read Tool
// =============================================================================

// FileReadTool reads file contents.
type FileReadTool struct {
	*BaseTool
}

// NewFileReadTool creates a new file read tool.
func NewFileReadTool() *FileReadTool {
	return &FileReadTool{
		BaseTool: &BaseTool{
			name:        constants.ToolFileRead,
			description: constants.DescRead,
			inputSchema: convertSchema(constants.GetReadToolSchema()),
			isEnabled:   true,
			isReadOnly:  true,
		},
	}
}

// Call reads the file content.
func (t *FileReadTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		TargetFile string `json:"target_file"`
		Offset     int    `json:"offset,omitempty"`
		Limit      int    `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Check file size
	info, err := os.Stat(input.TargetFile)
	if err != nil {
		return &types.ToolResult{
			Error:     fmt.Errorf("failed to stat file: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	if info.Size() > constants.MaxFileSizeToRead {
		return &types.ToolResult{
			Error:     fmt.Errorf("file too large (max %d bytes)", constants.MaxFileSizeToRead),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// Read file
	content, err := os.ReadFile(input.TargetFile)
	if err != nil {
		return &types.ToolResult{
			Error:     fmt.Errorf("failed to read file: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// Handle offset and limit
	lines := strings.Split(string(content), "\n")
	if input.Offset > 0 && input.Offset < len(lines) {
		lines = lines[input.Offset:]
	}
	if input.Limit > 0 && input.Limit < len(lines) {
		lines = lines[:input.Limit]
	}

	// Format with line numbers
	var result strings.Builder
	for i, line := range lines {
		lineNum := i + 1
		if input.Offset > 0 {
			lineNum += input.Offset
		}
		result.WriteString(fmt.Sprintf("%d|%s\n", lineNum, line))
	}

	return &types.ToolResult{
		Output:    result.String(),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// File Write Tool
// =============================================================================

// FileWriteTool writes content to files.
type FileWriteTool struct {
	*BaseTool
}

// NewFileWriteTool creates a new file write tool.
func NewFileWriteTool() *FileWriteTool {
	return &FileWriteTool{
		BaseTool: &BaseTool{
			name:          constants.ToolFileWrite,
			description:   constants.DescWrite,
			inputSchema:   convertSchema(constants.GetWriteToolSchema()),
			isEnabled:     true,
			isReadOnly:    false,
			isDestructive: true,
		},
	}
}

// Call writes content to the file.
func (t *FileWriteTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		FilePath string `json:"file_path"`
		Contents string `json:"contents"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
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

	oldContent, readErr := os.ReadFile(input.FilePath)
	isNewFile := os.IsNotExist(readErr)
	if readErr != nil && !isNewFile {
		return &types.ToolResult{
			Error:     fmt.Errorf("failed to read existing file: %w", readErr),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// Create directory if needed
	dir := filepath.Dir(input.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &types.ToolResult{
			Error:     fmt.Errorf("failed to create directory: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// Write file
	err = os.WriteFile(input.FilePath, []byte(input.Contents), 0644)
	if err != nil {
		return &types.ToolResult{
			Error:     fmt.Errorf("failed to write file: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Successfully wrote to %s", input.FilePath),
		ToolUseID: toolCtx.ToolUseId,
		Display: &types.ToolDisplay{
			Summary: fileChangeSummary(
				isNewFile,
				input.FilePath,
				len(diffContentLines(input.Contents)),
				len(diffContentLines(string(oldContent))),
			),
			FilePath:  input.FilePath,
			Diff:      buildFileDiff(input.FilePath, string(oldContent), input.Contents, isNewFile),
			Artifacts: []string{input.FilePath},
		},
	}, nil
}

// =============================================================================
// File Edit Tool
// =============================================================================

// FileEditTool edits files with string replacement.
type FileEditTool struct {
	*BaseTool
}

// NewFileEditTool creates a new file edit tool.
func NewFileEditTool() *FileEditTool {
	return &FileEditTool{
		BaseTool: &BaseTool{
			name:          constants.ToolFileEdit,
			description:   constants.DescEdit,
			inputSchema:   convertSchema(constants.GetEditToolSchema()),
			isEnabled:     true,
			isReadOnly:    false,
			isDestructive: true,
		},
	}
}

// Call edits the file.
func (t *FileEditTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
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

	// Read file
	content, err := os.ReadFile(input.FilePath)
	if err != nil {
		return &types.ToolResult{
			Error:     fmt.Errorf("failed to read file: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	oldContent := string(content)
	var newContent string

	if input.ReplaceAll {
		newContent = strings.ReplaceAll(oldContent, input.OldString, input.NewString)
	} else {
		count := strings.Count(oldContent, input.OldString)
		if count == 0 {
			return &types.ToolResult{
				Error:     fmt.Errorf("old_string not found in file"),
				ToolUseID: toolCtx.ToolUseId,
			}, nil
		}
		if count > 1 {
			return &types.ToolResult{
				Error:     fmt.Errorf("old_string appears %d times; provide more context or use replace_all", count),
				ToolUseID: toolCtx.ToolUseId,
			}, nil
		}
		newContent = strings.Replace(oldContent, input.OldString, input.NewString, 1)
	}

	// Write back
	err = os.WriteFile(input.FilePath, []byte(newContent), 0644)
	if err != nil {
		return &types.ToolResult{
			Error:     fmt.Errorf("failed to write file: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Successfully edited %s", input.FilePath),
		ToolUseID: toolCtx.ToolUseId,
		Display: &types.ToolDisplay{
			Summary: fmt.Sprintf(
				"Updated %s (+%d -%d)",
				input.FilePath,
				len(diffContentLines(input.NewString)),
				len(diffContentLines(input.OldString)),
			),
			FilePath:  input.FilePath,
			Diff:      buildReplacementDiff(input.FilePath, input.OldString, input.NewString),
			Artifacts: []string{input.FilePath},
		},
	}, nil
}

func fileChangeSummary(isNew bool, path string, added, removed int) string {
	if isNew {
		return fmt.Sprintf("Created %s (+%d -0)", path, added)
	}
	return fmt.Sprintf("Updated %s (+%d -%d)", path, added, removed)
}

func buildFileDiff(path, oldContent, newContent string, isNew bool) string {
	oldPath := path
	if isNew {
		oldPath = "/dev/null"
	}
	lines := []string{"--- " + oldPath, "+++ " + path, "@@ file contents @@"}
	for _, line := range diffContentLines(oldContent) {
		lines = append(lines, "-"+line)
	}
	for _, line := range diffContentLines(newContent) {
		lines = append(lines, "+"+line)
	}
	return strings.Join(lines, "\n")
}

func buildReplacementDiff(path, oldContent, newContent string) string {
	lines := []string{"--- " + path, "+++ " + path, "@@ replacement @@"}
	for _, line := range diffContentLines(oldContent) {
		lines = append(lines, "-"+line)
	}
	for _, line := range diffContentLines(newContent) {
		lines = append(lines, "+"+line)
	}
	return strings.Join(lines, "\n")
}

func diffContentLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

// =============================================================================
// Glob Tool
// =============================================================================

// GlobTool finds files matching patterns.
type GlobTool struct {
	*BaseTool
}

// NewGlobTool creates a new glob tool.
func NewGlobTool() *GlobTool {
	return &GlobTool{
		BaseTool: &BaseTool{
			name:        constants.ToolGlob,
			description: constants.DescGlob,
			inputSchema: convertSchema(constants.GetGlobToolSchema()),
			isEnabled:   true,
			isReadOnly:  true,
		},
	}
}

// Call finds matching files.
func (t *GlobTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Pattern         string `json:"pattern"`
		TargetDirectory string `json:"target_directory,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	searchDir := input.TargetDirectory
	if searchDir == "" {
		searchDir = "."
	}

	// Prepend **/ if pattern doesn't start with it
	pattern := input.Pattern
	if !strings.HasPrefix(pattern, "**/") {
		pattern = "**/" + pattern
	}

	// Use filepath.Glob with the pattern
	var matches []string
	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			matched, _ := filepath.Match(pattern, path)
			if matched {
				matches = append(matches, path)
			}
		}
		return nil
	})
	if err != nil {
		return &types.ToolResult{
			Error:     fmt.Errorf("glob failed: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    strings.Join(matches, "\n"),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Grep Tool
// =============================================================================

// GrepTool searches for patterns in files.
type GrepTool struct {
	*BaseTool
}

// NewGrepTool creates a new grep tool.
func NewGrepTool() *GrepTool {
	return &GrepTool{
		BaseTool: &BaseTool{
			name:        constants.ToolGrep,
			description: constants.DescGrep,
			inputSchema: convertSchema(constants.GetGrepToolSchema()),
			isEnabled:   true,
			isReadOnly:  true,
		},
	}
}

// Call searches for the pattern.
func (t *GrepTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Pattern         string `json:"pattern"`
		Path            string `json:"path,omitempty"`
		Glob            string `json:"glob,omitempty"`
		OutputMode      string `json:"output_mode,omitempty"`
		CaseInsensitive bool   `json:"case_insensitive,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	searchPath := input.Path
	if searchPath == "" {
		searchPath = "."
	}

	// Build grep command
	args_ := []string{"--no-heading", "--with-filename", "--line-number"}
	if input.CaseInsensitive {
		args_ = append(args_, "-i")
	}
	if input.Glob != "" {
		args_ = append(args_, "--glob", input.Glob)
	}
	if input.OutputMode == "files_with_matches" {
		args_ = append(args_, "-l")
	}
	args_ = append(args_, input.Pattern, searchPath)

	// Try ripgrep first, fallback to grep
	cmd := exec.CommandContext(ctx, "rg", args_...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to grep
		grepArgs := []string{"-rn"}
		if input.CaseInsensitive {
			grepArgs = append(grepArgs, "-i")
		}
		grepArgs = append(grepArgs, input.Pattern, searchPath)
		cmd = exec.CommandContext(ctx, "grep", grepArgs...)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return &types.ToolResult{
				Output:    "",
				ToolUseID: toolCtx.ToolUseId,
			}, nil
		}
	}

	// Limit output lines
	lines := strings.Split(string(output), "\n")
	if len(lines) > constants.MaxGrepOutputLines {
		lines = lines[:constants.MaxGrepOutputLines]
		lines = append(lines, fmt.Sprintf("... (truncated, %d more lines)", len(lines)-constants.MaxGrepOutputLines))
	}

	return &types.ToolResult{
		Output:    strings.Join(lines, "\n"),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Web Fetch Tool
// =============================================================================

// WebFetchTool fetches content from URLs.
type WebFetchTool struct {
	*BaseTool
}

// NewWebFetchTool creates a new web fetch tool.
func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		BaseTool: &BaseTool{
			name:        constants.ToolWebFetch,
			description: constants.DescWebFetch,
			inputSchema: convertSchema(constants.GetWebFetchToolSchema()),
			isEnabled:   true,
			isReadOnly:  false,
		},
	}
}

// Call fetches content from URLs.
func (t *WebFetchTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		URLs []string `json:"urls"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	var results []string
	client := newSecureWebClient()

	for _, rawURL := range input.URLs {
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			results = append(results, fmt.Sprintf("Error: invalid URL %s: %v", rawURL, err))
			continue
		}

		req, err := http.NewRequestWithContext(ctx, "GET", parsedURL.String(), nil)
		if err != nil {
			results = append(results, fmt.Sprintf("Error: failed to create request for %s: %v", rawURL, err))
			continue
		}

		req.Header.Set("User-Agent", "claude-code-go/1.0")
		resp, err := client.Do(req)
		if err != nil {
			results = append(results, fmt.Sprintf("Error: failed to fetch %s: %v", rawURL, err))
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxWebResponseBytes+1))
		resp.Body.Close()
		if err != nil {
			results = append(results, fmt.Sprintf("Error: failed to read response from %s: %v", rawURL, err))
			continue
		}
		if len(body) > maxWebResponseBytes {
			results = append(results, fmt.Sprintf("Error: response from %s exceeds %d bytes", rawURL, maxWebResponseBytes))
			continue
		}

		// Convert HTML to markdown-like format
		content := string(body)
		if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			content = htmlToMarkdown(content)
		}
		content = SanitizeUntrustedText(content)

		results = append(results, fmt.Sprintf("=== %s ===\n%s", rawURL, content))
	}

	return &types.ToolResult{
		Output:    strings.Join(results, "\n\n"),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

func (t *WebFetchTool) ValidateInput(args json.RawMessage) error {
	var input struct {
		URLs []string `json:"urls"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return err
	}
	for _, rawURL := range input.URLs {
		if err := validatePublicHTTPSURL(rawURL); err != nil {
			return err
		}
	}
	return nil
}

// htmlToMarkdown converts basic HTML to markdown-like format
func htmlToMarkdown(html string) string {
	// Remove scripts and styles
	html = regexp.MustCompile(`<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`<style[^>]*>.*?</style>`).ReplaceAllString(html, "")

	// Convert headers
	html = regexp.MustCompile(`<h1[^>]*>(.*?)</h1>`).ReplaceAllString(html, "# $1\n")
	html = regexp.MustCompile(`<h2[^>]*>(.*?)</h2>`).ReplaceAllString(html, "## $1\n")
	html = regexp.MustCompile(`<h3[^>]*>(.*?)</h3>`).ReplaceAllString(html, "### $1\n")

	// Convert links
	html = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`).ReplaceAllString(html, "[$2]($1)")

	// Convert paragraphs and divs
	html = regexp.MustCompile(`</?p[^>]*>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`</?div[^>]*>`).ReplaceAllString(html, "\n")

	// Remove remaining HTML tags
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")

	// Clean up whitespace
	html = regexp.MustCompile(`\n{3,}`).ReplaceAllString(html, "\n\n")
	html = strings.TrimSpace(html)

	return html
}

// =============================================================================
// Todo Write Tool
// =============================================================================

// TodoItem represents a todo item
type TodoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

// TodoWriteTool manages a task list.
type TodoWriteTool struct {
	*BaseTool
	todos []TodoItem
}

// NewTodoWriteTool creates a new todo write tool.
func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{
		BaseTool: &BaseTool{
			name:        constants.ToolTodoWrite,
			description: constants.DescTodoWrite,
			inputSchema: convertSchema(constants.GetTodoWriteToolSchema()),
			isEnabled:   true,
			isReadOnly:  false,
		},
		todos: make([]TodoItem, 0),
	}
}

// Call updates the todo list.
func (t *TodoWriteTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Todos []TodoItem `json:"todos"`
		Merge bool       `json:"merge,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	if input.Merge {
		// Merge with existing todos
		existingMap := make(map[string]TodoItem)
		for _, todo := range t.todos {
			existingMap[todo.ID] = todo
		}
		for _, todo := range input.Todos {
			existingMap[todo.ID] = todo
		}
		t.todos = make([]TodoItem, 0, len(existingMap))
		for _, todo := range existingMap {
			t.todos = append(t.todos, todo)
		}
	} else {
		t.todos = input.Todos
	}

	// Format output
	var result strings.Builder
	result.WriteString("Todo list updated:\n")
	for _, todo := range t.todos {
		statusIcon := "○"
		switch todo.Status {
		case "in_progress":
			statusIcon = "◐"
		case "completed":
			statusIcon = "●"
		case "cancelled":
			statusIcon = "✗"
		}
		result.WriteString(fmt.Sprintf("  %s [%s] %s\n", statusIcon, todo.ID, todo.Content))
	}

	return &types.ToolResult{
		Output:    result.String(),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// GetTodos returns the current todo list
func (t *TodoWriteTool) GetTodos() []TodoItem {
	return t.todos
}

// =============================================================================
// Task Tool (Agent)
// =============================================================================

// TaskTool launches sub-agents.
type TaskTool struct {
	*BaseTool
}

// NewTaskTool creates a new task tool.
func NewTaskTool() *TaskTool {
	return &TaskTool{
		BaseTool: &BaseTool{
			name:        constants.ToolTaskCreate,
			description: constants.DescTask,
			inputSchema: convertSchema(constants.GetTaskToolSchema()),
			isEnabled:   true,
			isReadOnly:  false,
		},
	}
}

// Call launches a sub-agent task.
func (t *TaskTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Description  string `json:"description"`
		Prompt       string `json:"prompt"`
		SubagentType string `json:"subagent_type"`
		Model        int    `json:"model,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// For now, return a placeholder - actual agent execution would require more infrastructure
	return &types.ToolResult{
		Output:    fmt.Sprintf("Task '%s' queued for execution with %s agent", input.Description, input.SubagentType),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Multi-Edit Tool
// =============================================================================

// MultiEditTool performs multiple edits on a single file.
type MultiEditTool struct {
	*BaseTool
}

// MultiEditOperation represents a single edit operation
type MultiEditOperation struct {
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// NewMultiEditTool creates a new multi-edit tool.
func NewMultiEditTool() *MultiEditTool {
	return &MultiEditTool{
		BaseTool: &BaseTool{
			name:        "MultiEdit",
			description: "Perform multiple edits on a single file in one operation",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"file_path": {
						"type":        "string",
						"description": "The path to the file to modify",
					},
					"edits": {
						"type":        "array",
						"description": "Array of edit operations",
					},
				},
			},
			isEnabled:     true,
			isReadOnly:    false,
			isDestructive: true,
		},
	}
}

// Call performs multiple edits on the file.
func (t *MultiEditTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		FilePath string               `json:"file_path"`
		Edits    []MultiEditOperation `json:"edits"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Read file
	content, err := os.ReadFile(input.FilePath)
	if err != nil {
		return &types.ToolResult{
			Error:     fmt.Errorf("failed to read file: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	result := string(content)
	editCount := 0
	addedLines := 0
	removedLines := 0

	// Apply edits sequentially
	for i, edit := range input.Edits {
		addedLines += len(diffContentLines(edit.NewString))
		removedLines += len(diffContentLines(edit.OldString))
		if edit.NewString == edit.OldString {
			return &types.ToolResult{
				Error:     fmt.Errorf("edit %d: old_string and new_string are identical", i+1),
				ToolUseID: toolCtx.ToolUseId,
			}, nil
		}

		if edit.ReplaceAll {
			result = strings.ReplaceAll(result, edit.OldString, edit.NewString)
			editCount++
		} else {
			count := strings.Count(result, edit.OldString)
			if count == 0 {
				return &types.ToolResult{
					Error:     fmt.Errorf("edit %d: old_string not found", i+1),
					ToolUseID: toolCtx.ToolUseId,
				}, nil
			}
			if count > 1 {
				return &types.ToolResult{
					Error:     fmt.Errorf("edit %d: old_string appears %d times; provide more context or use replace_all", i+1, count),
					ToolUseID: toolCtx.ToolUseId,
				}, nil
			}
			result = strings.Replace(result, edit.OldString, edit.NewString, 1)
			editCount++
		}
	}

	// Write back
	err = os.WriteFile(input.FilePath, []byte(result), 0644)
	if err != nil {
		return &types.ToolResult{
			Error:     fmt.Errorf("failed to write file: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Successfully applied %d edits to %s", editCount, input.FilePath),
		ToolUseID: toolCtx.ToolUseId,
		Display: &types.ToolDisplay{
			Summary:   fmt.Sprintf("Updated %s (%d edits, +%d -%d)", input.FilePath, editCount, addedLines, removedLines),
			FilePath:  input.FilePath,
			Diff:      buildFileDiff(input.FilePath, string(content), result, false),
			Artifacts: []string{input.FilePath},
		},
	}, nil
}

// =============================================================================
// Tool Registry
// =============================================================================

// Registry manages all available tools.
type Registry struct {
	tools          map[string]types.Tool
	order          []string
	options        RegistryOptions
	todoTool       *TodoWriteTool
	taskCreateTool *TaskCreateTool
}

// RegistryOptions controls which registered tools are exposed to the model.
// Registration stays stable for prompt-cache and tool-search consistency;
// policy is applied only when producing the enabled tool pool.
type RegistryOptions struct {
	SimpleMode    bool
	DisabledTools map[string]bool
}

// ToolCapability describes why a registered tool is or is not reachable.
type ToolCapability struct {
	Name    string
	Enabled bool
	Gate    string
}

// RegistryOptionsFromEnv builds the runtime tool policy used by the CLI.
// CLAUDE_CODE_SIMPLE mirrors the reference client's Bash/Read/Edit preset.
// CLAUDE_CODE_DISABLE_TOOLS accepts a comma-separated list of canonical names.
func RegistryOptionsFromEnv() RegistryOptions {
	options := RegistryOptions{
		SimpleMode:    envTruthy(os.Getenv("CLAUDE_CODE_SIMPLE")),
		DisabledTools: make(map[string]bool),
	}
	for _, name := range strings.Split(os.Getenv("CLAUDE_CODE_DISABLE_TOOLS"), ",") {
		if name = strings.TrimSpace(name); name != "" {
			options.DisabledTools[name] = true
		}
	}
	return options
}

func envTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// NewToolRegistry creates a new tool registry.
func NewToolRegistry() *Registry {
	return NewToolRegistryWithOptions(RegistryOptions{})
}

// NewToolRegistryWithOptions creates the complete stable registry and applies
// runtime exposure policy when ListEnabled is requested.
func NewToolRegistryWithOptions(options RegistryOptions) *Registry {
	r := &Registry{
		tools:   make(map[string]types.Tool),
		order:   make([]string, 0),
		options: options,
	}

	// Register core file tools
	r.Register(NewBashTool())
	r.Register(NewFileReadTool())
	r.Register(NewFileWriteTool())
	r.Register(NewFileEditTool())
	r.Register(NewGlobTool())
	r.Register(NewGrepTool())

	// Register web tools
	r.Register(NewWebFetchTool())
	r.Register(NewWebSearchTool())

	// Register todo tool
	todoTool := NewTodoWriteTool()
	r.todoTool = todoTool
	r.Register(todoTool)

	// Register task management tools
	taskCreateTool := NewTaskCreateTool()
	r.taskCreateTool = taskCreateTool
	r.Register(taskCreateTool)
	r.Register(NewTaskListTool(taskCreateTool))
	r.Register(NewTaskStopTool(taskCreateTool))
	r.Register(NewTaskGetTool(taskCreateTool))
	r.Register(NewTaskUpdateTool(taskCreateTool))
	r.Register(NewTaskTool())

	// Register advanced editing tools
	r.Register(NewMultiEditTool())

	// Register LSP tool
	r.Register(NewLSPTool())

	// Register skill tool
	r.Register(NewSkillTool())

	// Register MCP tools
	r.Register(NewListMcpResourcesTool())
	r.Register(NewReadMcpResourceTool())

	// Register notebook tool
	r.Register(NewNotebookEditTool())

	// Register config tool
	r.Register(NewConfigTool())

	// Register standalone utility tools with complete local behavior.
	r.Register(NewSleepTool())
	r.Register(NewPowerShellTool())
	r.Register(NewToolSearchTool(r))

	return r
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool types.Tool) {
	if _, exists := r.tools[tool.Name()]; !exists {
		r.order = append(r.order, tool.Name())
	}
	r.tools[tool.Name()] = tool
	for _, alias := range tool.Aliases() {
		r.tools[alias] = tool
	}
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (types.Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tools.
func (r *Registry) List() []types.Tool {
	result := make([]types.Tool, 0, len(r.order))
	for _, name := range r.order {
		if tool, ok := r.tools[name]; ok {
			result = append(result, tool)
		}
	}
	return result
}

// ListEnabled returns all enabled tools.
func (r *Registry) ListEnabled() []types.Tool {
	result := make([]types.Tool, 0)
	for _, tool := range r.List() {
		if enabled, _ := r.toolEnabled(tool); enabled {
			result = append(result, tool)
		}
	}
	return result
}

// Capabilities returns a stable matrix for diagnostics and regression tests.
func (r *Registry) Capabilities() []ToolCapability {
	result := make([]ToolCapability, 0, len(r.order))
	for _, tool := range r.List() {
		enabled, gate := r.toolEnabled(tool)
		result = append(result, ToolCapability{Name: tool.Name(), Enabled: enabled, Gate: gate})
	}
	return result
}

func (r *Registry) toolEnabled(tool types.Tool) (bool, string) {
	if !tool.IsEnabled() {
		return false, "implementation"
	}
	if r.options.DisabledTools[tool.Name()] {
		return false, "disabled_tools"
	}
	if r.options.SimpleMode {
		switch tool.Name() {
		case constants.ToolBash, constants.ToolFileRead, constants.ToolFileEdit:
		default:
			return false, "simple_mode"
		}
	}
	return true, ""
}

// GetTodoTool returns the todo tool instance
func (r *Registry) GetTodoTool() *TodoWriteTool {
	return r.todoTool
}

// FilterToolsForAgent filters tools available to a sub-agent
func (r *Registry) FilterToolsForAgent(agentType string) []types.Tool {
	allowedMap := constants.AsyncAgentAllowedTools
	result := make([]types.Tool, 0)

	for _, tool := range r.ListEnabled() {
		if allowedMap[tool.Name()] {
			result = append(result, tool)
		}
	}

	return result
}
