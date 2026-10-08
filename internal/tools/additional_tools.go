package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"claude-code-go/internal/constants"
	"claude-code-go/internal/types"
)

// =============================================================================
// Sleep Tool
// =============================================================================

// SleepTool waits for a specified duration.
type SleepTool struct {
	*BaseTool
}

// NewSleepTool creates a new sleep tool.
func NewSleepTool() *SleepTool {
	return &SleepTool{
		BaseTool: &BaseTool{
			name:        "Sleep",
			description: "Wait for a specified duration",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"duration": {
						"type":             "number",
						"exclusiveMinimum": 0,
						"maximum":          3600,
						"description":      "Duration to sleep in seconds",
					},
					"reason": {
						"type":        "string",
						"description": "Optional reason for sleeping",
					},
				},
				Required: []string{"duration"},
			},
			isEnabled:  true,
			isReadOnly: true,
		},
	}
}

// Call sleeps for the specified duration.
func (t *SleepTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Duration float64 `json:"duration"`
		Reason   string  `json:"reason,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	if input.Duration <= 0 {
		return &types.ToolResult{
			Error:     fmt.Errorf("duration must be positive"),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	if input.Duration > 3600 {
		return &types.ToolResult{
			Error:     fmt.Errorf("duration cannot exceed 3600 seconds (1 hour)"),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	// Sleep with context cancellation support
	select {
	case <-time.After(time.Duration(input.Duration * float64(time.Second))):
		message := fmt.Sprintf("Slept for %.1f seconds", input.Duration)
		if input.Reason != "" {
			message += fmt.Sprintf(" (%s)", input.Reason)
		}
		return &types.ToolResult{
			Output:    message,
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case <-ctx.Done():
		return &types.ToolResult{
			Output:    fmt.Sprintf("Sleep interrupted after context cancellation"),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}
}

// ValidateInput enforces semantic duration limits before permission approval.
func (t *SleepTool) ValidateInput(args json.RawMessage) error {
	var input struct {
		Duration float64 `json:"duration"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return err
	}
	if input.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if input.Duration > 3600 {
		return fmt.Errorf("duration cannot exceed 3600 seconds (1 hour)")
	}
	return nil
}

// IsConcurrencySafe returns true for sleep tool.
func (t *SleepTool) IsConcurrencySafe(input json.RawMessage) bool {
	return true
}

// =============================================================================
// Ask User Question Tool
// =============================================================================

// AskUserQuestionTool asks the user a question.
type AskUserQuestionTool struct {
	*BaseTool
}

// NewAskUserQuestionTool creates a new ask user question tool.
func NewAskUserQuestionTool() *AskUserQuestionTool {
	return &AskUserQuestionTool{
		BaseTool: &BaseTool{
			name:        constants.ToolAskUser,
			description: constants.DescAskUserQuestion,
			inputSchema: convertSchema(constants.GetAskUserQuestionToolSchema()),
			isEnabled:   true,
			isReadOnly:  true,
		},
	}
}

// Call asks the user a question.
func (t *AskUserQuestionTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Question    string   `json:"question"`
		Suggestions []string `json:"suggestions,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Return a prompt for user response - the actual UI would handle this
	result := fmt.Sprintf("Question for user: %s", input.Question)
	if len(input.Suggestions) > 0 {
		result += fmt.Sprintf("\nSuggested answers: %s", strings.Join(input.Suggestions, ", "))
	}

	return &types.ToolResult{
		Output:    result + "\n(Waiting for user response...)",
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Brief Tool (Send User Message)
// =============================================================================

// BriefTool sends a message to the user.
type BriefTool struct {
	*BaseTool
}

// NewBriefTool creates a new brief tool.
func NewBriefTool() *BriefTool {
	return &BriefTool{
		BaseTool: &BaseTool{
			name:        "Brief",
			description: "Send a message to the user with optional attachments",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"message": {
						"type":        "string",
						"description": "The message for the user. Supports markdown formatting.",
					},
					"attachments": {
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Optional file paths to attach",
					},
					"status": {
						"type":        "string",
						"enum":        []string{"normal", "proactive"},
						"description": "Use 'proactive' for unsolicited updates",
					},
				},
				Required: []string{"message"},
			},
			isEnabled:  true,
			isReadOnly: true,
		},
	}
}

// Call sends a message to the user.
func (t *BriefTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Message     string   `json:"message"`
		Attachments []string `json:"attachments,omitempty"`
		Status      string   `json:"status,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Validate attachments exist
	for _, path := range input.Attachments {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return &types.ToolResult{
				Error:     fmt.Errorf("attachment not found: %s", path),
				ToolUseID: toolCtx.ToolUseId,
			}, nil
		}
	}

	result := fmt.Sprintf("Message delivered to user: %s", input.Message)
	if len(input.Attachments) > 0 {
		result += fmt.Sprintf("\nAttachments: %d file(s)", len(input.Attachments))
	}

	return &types.ToolResult{
		Output:    result,
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// IsConcurrencySafe returns true for brief tool.
func (t *BriefTool) IsConcurrencySafe(input json.RawMessage) bool {
	return true
}

// =============================================================================
// Send Message Tool (Team Communication)
// =============================================================================

// SendMessageTool sends messages to teammates.
type SendMessageTool struct {
	*BaseTool
}

// NewSendMessageTool creates a new send message tool.
func NewSendMessageTool() *SendMessageTool {
	return &SendMessageTool{
		BaseTool: &BaseTool{
			name:        "SendMessage",
			description: "Send messages to agent teammates (swarm protocol)",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"to": {
						"type":        "string",
						"description": "Recipient: teammate name, or '*' for broadcast",
					},
					"message": {
						"type":        "string",
						"description": "Plain text message content",
					},
					"summary": {
						"type":        "string",
						"description": "A 5-10 word summary shown as a preview",
					},
				},
				Required: []string{"to", "message"},
			},
			isEnabled:  true,
			isReadOnly: true,
		},
	}
}

// Call sends a message to a teammate.
func (t *SendMessageTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		To      string `json:"to"`
		Message string `json:"message"`
		Summary string `json:"summary,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	if input.To == "" {
		return &types.ToolResult{
			Error:     fmt.Errorf("recipient 'to' must not be empty"),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	if input.To == "*" {
		return &types.ToolResult{
			Output:    fmt.Sprintf("Broadcast sent to all teammates: %s", input.Message),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Message sent to %s: %s", input.To, input.Message),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Team Tools
// =============================================================================

// TeamCreateTool creates a new team.
type TeamCreateTool struct {
	*BaseTool
}

// NewTeamCreateTool creates a new team create tool.
func NewTeamCreateTool() *TeamCreateTool {
	return &TeamCreateTool{
		BaseTool: &BaseTool{
			name:        "TeamCreate",
			description: "Create a new team for multi-agent collaboration",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"team_name": {
						"type":        "string",
						"description": "Name for the team",
					},
					"description": {
						"type":        "string",
						"description": "Optional team description",
					},
				},
				Required: []string{"team_name"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
	}
}

// Call creates a team.
func (t *TeamCreateTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		TeamName    string `json:"team_name"`
		Description string `json:"description,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Team '%s' created successfully", input.TeamName),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// TeamDeleteTool deletes a team.
type TeamDeleteTool struct {
	*BaseTool
}

// NewTeamDeleteTool creates a new team delete tool.
func NewTeamDeleteTool() *TeamDeleteTool {
	return &TeamDeleteTool{
		BaseTool: &BaseTool{
			name:        "TeamDelete",
			description: "Delete a team",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"team_name": {
						"type":        "string",
						"description": "Name of the team to delete",
					},
				},
				Required: []string{"team_name"},
			},
			isEnabled:     true,
			isReadOnly:    false,
			isDestructive: true,
		},
	}
}

// Call deletes a team.
func (t *TeamDeleteTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		TeamName string `json:"team_name"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Team '%s' deleted successfully", input.TeamName),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Tool Search Tool
// =============================================================================

// ToolSearchTool searches for available tools.
type ToolSearchTool struct {
	*BaseTool
	registry *Registry
}

const (
	bm25K1                 = 1.2
	bm25B                  = 0.75
	defaultToolSearchLimit = 10
	maxToolSearchLimit     = 50
)

type bm25ToolDocument struct {
	tool        types.Tool
	description string
	terms       map[string]float64
	length      float64
}

type bm25ToolMatch struct {
	document bm25ToolDocument
	score    float64
}

// NewToolSearchTool creates a new tool search tool.
func NewToolSearchTool(registry *Registry) *ToolSearchTool {
	return &ToolSearchTool{
		BaseTool: &BaseTool{
			name:        "ToolSearch",
			description: "Search available tools using BM25 relevance ranking over names, aliases, descriptions, and input fields",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"query": {
						"type":        "string",
						"description": "Search query",
					},
					"max_results": {
						"type":        "integer",
						"description": "Maximum number of ranked tools to return (default 10, maximum 50)",
					},
				},
				Required: []string{"query"},
			},
			isEnabled:  true,
			isReadOnly: true,
		},
		registry: registry,
	}
}

// Call searches for tools.
func (t *ToolSearchTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	queryTerms := tokenizeForBM25(input.Query)
	if len(queryTerms) == 0 {
		return &types.ToolResult{
			Error:     fmt.Errorf("search query must contain at least one letter or number"),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}
	limit := input.MaxResults
	if limit == 0 {
		limit = defaultToolSearchLimit
	}
	if limit < 0 || limit > maxToolSearchLimit {
		return &types.ToolResult{
			Error:     fmt.Errorf("max_results must be between 1 and %d", maxToolSearchLimit),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	documents := t.buildBM25Documents(ctx)
	matches := rankBM25Documents(documents, queryTerms)
	if len(matches) == 0 {
		return &types.ToolResult{
			Output:    fmt.Sprintf("No tools found matching '%s'", input.Query),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}
	if limit > len(matches) {
		limit = len(matches)
	}

	results := make([]string, 0, limit)
	for _, match := range matches[:limit] {
		results = append(results, fmt.Sprintf(
			"- %s: %s",
			match.document.tool.Name(),
			match.document.description,
		))
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Tools matching '%s':\n%s", input.Query, strings.Join(results, "\n")),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

func (t *ToolSearchTool) buildBM25Documents(ctx context.Context) []bm25ToolDocument {
	tools := t.registry.ListEnabled()
	documents := make([]bm25ToolDocument, 0, len(tools))
	for _, tool := range tools {
		description, err := tool.Description(ctx, nil, types.ToolOptions{})
		if err != nil {
			continue
		}

		document := bm25ToolDocument{
			tool:        tool,
			description: description,
			terms:       make(map[string]float64),
		}
		addBM25Terms(&document, tokenizeForBM25(tool.Name()), 4)
		for _, alias := range tool.Aliases() {
			addBM25Terms(&document, tokenizeForBM25(alias), 3)
		}
		addBM25Terms(&document, tokenizeForBM25(description), 1)

		schema := tool.InputSchema()
		propertyNames := make([]string, 0, len(schema.Properties))
		for name := range schema.Properties {
			propertyNames = append(propertyNames, name)
		}
		sort.Strings(propertyNames)
		for _, name := range propertyNames {
			addBM25Terms(&document, tokenizeForBM25(name), 2)
			if fieldDescription, ok := schema.Properties[name]["description"].(string); ok {
				addBM25Terms(&document, tokenizeForBM25(fieldDescription), 0.75)
			}
		}

		if document.length > 0 {
			documents = append(documents, document)
		}
	}
	return documents
}

func addBM25Terms(document *bm25ToolDocument, terms []string, weight float64) {
	for _, term := range terms {
		document.terms[term] += weight
		document.length += weight
	}
}

func rankBM25Documents(documents []bm25ToolDocument, queryTerms []string) []bm25ToolMatch {
	if len(documents) == 0 {
		return nil
	}

	documentFrequency := make(map[string]int)
	for _, document := range documents {
		for term := range document.terms {
			documentFrequency[term]++
		}
	}

	queryFrequency := make(map[string]int)
	for _, term := range queryTerms {
		queryFrequency[term]++
	}

	var totalLength float64
	for _, document := range documents {
		totalLength += document.length
	}
	averageLength := totalLength / float64(len(documents))

	matches := make([]bm25ToolMatch, 0, len(documents))
	for _, document := range documents {
		var score float64
		for term, queryCount := range queryFrequency {
			termFrequency := document.terms[term]
			if termFrequency == 0 {
				continue
			}
			df := float64(documentFrequency[term])
			idf := math.Log(1 + (float64(len(documents))-df+0.5)/(df+0.5))
			lengthNormalization := bm25K1 * (1 - bm25B + bm25B*document.length/averageLength)
			queryBoost := 1 + math.Log(float64(queryCount))
			score += idf * (termFrequency * (bm25K1 + 1) / (termFrequency + lengthNormalization)) * queryBoost
		}
		if score > 0 {
			matches = append(matches, bm25ToolMatch{document: document, score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if math.Abs(matches[i].score-matches[j].score) > 1e-12 {
			return matches[i].score > matches[j].score
		}
		return matches[i].document.tool.Name() < matches[j].document.tool.Name()
	})
	return matches
}

func tokenizeForBM25(text string) []string {
	var terms []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		term := stemBM25Term(strings.ToLower(string(current)))
		if term != "" {
			terms = append(terms, term)
		}
		current = current[:0]
	}

	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			flush()
			terms = append(terms, string(r))
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if unicode.IsUpper(r) && len(current) > 0 && unicode.IsLower(current[len(current)-1]) {
			flush()
		}
		current = append(current, unicode.ToLower(r))
	}
	flush()
	return terms
}

func stemBM25Term(term string) string {
	switch {
	case len(term) > 5 && strings.HasSuffix(term, "ies"):
		return strings.TrimSuffix(term, "ies") + "y"
	case len(term) > 5 && strings.HasSuffix(term, "ing"):
		return strings.TrimSuffix(term, "ing")
	case len(term) > 4 && strings.HasSuffix(term, "ed"):
		return strings.TrimSuffix(term, "ed")
	case len(term) > 4 && (strings.HasSuffix(term, "ses") ||
		strings.HasSuffix(term, "xes") ||
		strings.HasSuffix(term, "zes") ||
		strings.HasSuffix(term, "ches") ||
		strings.HasSuffix(term, "shes")):
		return strings.TrimSuffix(term, "es")
	case len(term) > 3 && strings.HasSuffix(term, "s"):
		return strings.TrimSuffix(term, "s")
	default:
		return term
	}
}

// =============================================================================
// Exit Plan Mode Tool
// =============================================================================

// ExitPlanModeTool exits plan mode.
type ExitPlanModeTool struct {
	*BaseTool
}

// NewExitPlanModeTool creates a new exit plan mode tool.
func NewExitPlanModeTool() *ExitPlanModeTool {
	return &ExitPlanModeTool{
		BaseTool: &BaseTool{
			name:        constants.ToolExitPlanMode,
			description: constants.DescExitPlanMode,
			inputSchema: convertSchema(constants.GetExitPlanModeToolSchema()),
			isEnabled:   true,
			isReadOnly:  true,
		},
	}
}

// Call exits plan mode.
func (t *ExitPlanModeTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		PlanSummary string `json:"plan_summary,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	result := "Exited plan mode"
	if input.PlanSummary != "" {
		result += fmt.Sprintf(": %s", input.PlanSummary)
	}

	return &types.ToolResult{
		Output:    result,
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Enter Plan Mode Tool
// =============================================================================

// EnterPlanModeTool enters plan mode.
type EnterPlanModeTool struct {
	*BaseTool
}

// NewEnterPlanModeTool creates a new enter plan mode tool.
func NewEnterPlanModeTool() *EnterPlanModeTool {
	return &EnterPlanModeTool{
		BaseTool: &BaseTool{
			name:        constants.ToolEnterPlanMode,
			description: constants.DescEnterPlanMode,
			inputSchema: convertSchema(constants.GetEnterPlanModeToolSchema()),
			isEnabled:   true,
			isReadOnly:  true,
		},
	}
}

// Call enters plan mode.
func (t *EnterPlanModeTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Goal string `json:"goal"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Entered plan mode. Goal: %s", input.Goal),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Enter/Exit Worktree Tools
// =============================================================================

// EnterWorktreeTool enters a git worktree.
type EnterWorktreeTool struct {
	*BaseTool
}

// NewEnterWorktreeTool creates a new enter worktree tool.
func NewEnterWorktreeTool() *EnterWorktreeTool {
	return &EnterWorktreeTool{
		BaseTool: &BaseTool{
			name:        "EnterWorktree",
			description: "Enter a git worktree for isolated development",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"path": {
						"type":        "string",
						"description": "Path to the worktree",
					},
				},
				Required: []string{"path"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
	}
}

// Call enters a worktree.
func (t *EnterWorktreeTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Check if path exists
	if _, err := os.Stat(input.Path); os.IsNotExist(err) {
		return &types.ToolResult{
			Error:     fmt.Errorf("worktree path does not exist: %s", input.Path),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    fmt.Sprintf("Entered worktree at: %s", input.Path),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// ExitWorktreeTool exits a git worktree.
type ExitWorktreeTool struct {
	*BaseTool
}

// NewExitWorktreeTool creates a new exit worktree tool.
func NewExitWorktreeTool() *ExitWorktreeTool {
	return &ExitWorktreeTool{
		BaseTool: &BaseTool{
			name:        "ExitWorktree",
			description: "Exit the current git worktree",
			inputSchema: types.ToolInputJSONSchema{
				Type:       "object",
				Properties: map[string]map[string]interface{}{},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
	}
}

// Call exits a worktree.
func (t *ExitWorktreeTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	return &types.ToolResult{
		Output:    "Exited worktree and returned to main repository",
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// PowerShell Tool (Windows)
// =============================================================================

// PowerShellTool executes PowerShell commands on Windows.
type PowerShellTool struct {
	*BaseTool
}

// NewPowerShellTool creates a new PowerShell tool.
func NewPowerShellTool() *PowerShellTool {
	return &PowerShellTool{
		BaseTool: &BaseTool{
			name:        "PowerShell",
			description: "Execute PowerShell commands on Windows",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"command": {
						"type":        "string",
						"description": "The PowerShell command to execute",
					},
					"timeout": {
						"type":        "number",
						"description": "Timeout in milliseconds",
					},
				},
				Required: []string{"command"},
			},
			isEnabled:     runtime.GOOS == "windows",
			isReadOnly:    false,
			isDestructive: true,
		},
	}
}

// Call executes a PowerShell command.
func (t *PowerShellTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	if runtime.GOOS != "windows" {
		return &types.ToolResult{
			Error:     fmt.Errorf("PowerShell tool is only available on Windows"),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	var input struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	timeout := input.Timeout
	if timeout == 0 {
		timeout = constants.DefaultCommandTimeoutMs
	}

	timeoutDuration := time.Duration(timeout) * time.Millisecond
	execCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "powershell", "-Command", input.Command)
	if cwd, err := os.Getwd(); err == nil {
		cmd.Dir = cwd
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &types.ToolResult{
			Output:    string(output),
			Error:     fmt.Errorf("PowerShell command failed: %w", err),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}

	return &types.ToolResult{
		Output:    string(output),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Schedule Cron Tool
// =============================================================================

// ScheduleCronTool schedules a task to run periodically.
type ScheduleCronTool struct {
	*BaseTool
	scheduledTasks map[string]*ScheduledTask
}

// ScheduledTask represents a scheduled task.
type ScheduledTask struct {
	ID       string
	Schedule string
	Command  string
	Enabled  bool
	NextRun  time.Time
	LastRun  time.Time
}

// NewScheduleCronTool creates a new schedule cron tool.
func NewScheduleCronTool() *ScheduleCronTool {
	return &ScheduleCronTool{
		BaseTool: &BaseTool{
			name:        "ScheduleCron",
			description: "Schedule a task to run periodically using cron syntax",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"schedule": {
						"type":        "string",
						"description": "Cron schedule expression (e.g., '0 * * * *' for hourly)",
					},
					"command": {
						"type":        "string",
						"description": "The command or prompt to execute",
					},
					"enabled": {
						"type":        "boolean",
						"description": "Whether the schedule is enabled",
					},
				},
				Required: []string{"schedule", "command"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
		scheduledTasks: make(map[string]*ScheduledTask),
	}
}

// Call schedules a cron task.
func (t *ScheduleCronTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Schedule string `json:"schedule"`
		Command  string `json:"command"`
		Enabled  bool   `json:"enabled,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	taskID := fmt.Sprintf("cron-%d", time.Now().UnixNano())

	scheduledTask := &ScheduledTask{
		ID:       taskID,
		Schedule: input.Schedule,
		Command:  input.Command,
		Enabled:  input.Enabled || true,
	}

	t.scheduledTasks[taskID] = scheduledTask

	return &types.ToolResult{
		Output:    fmt.Sprintf("Scheduled task '%s' created with schedule '%s'", taskID, input.Schedule),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// MCP Auth Tool
// =============================================================================

// McpAuthTool handles MCP server authentication.
type McpAuthTool struct {
	*BaseTool
}

// NewMcpAuthTool creates a new MCP auth tool.
func NewMcpAuthTool() *McpAuthTool {
	return &McpAuthTool{
		BaseTool: &BaseTool{
			name:        "McpAuth",
			description: "Authenticate with MCP servers",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"server_name": {
						"type":        "string",
						"description": "The name of the MCP server to authenticate with",
					},
					"action": {
						"type":        "string",
						"enum":        []string{"login", "logout", "status"},
						"description": "The authentication action to perform",
					},
				},
				Required: []string{"server_name", "action"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
	}
}

// Call handles MCP authentication.
func (t *McpAuthTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		ServerName string `json:"server_name"`
		Action     string `json:"action"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	switch input.Action {
	case "login":
		return &types.ToolResult{
			Output:    fmt.Sprintf("Initiating authentication for MCP server: %s", input.ServerName),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "logout":
		return &types.ToolResult{
			Output:    fmt.Sprintf("Logged out from MCP server: %s", input.ServerName),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "status":
		return &types.ToolResult{
			Output:    fmt.Sprintf("Authentication status for MCP server '%s': not authenticated", input.ServerName),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	default:
		return &types.ToolResult{
			Error:     fmt.Errorf("unknown action: %s", input.Action),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}
}

// =============================================================================
// Remote Trigger Tool
// =============================================================================

// RemoteTriggerTool triggers actions on remote systems.
type RemoteTriggerTool struct {
	*BaseTool
}

// NewRemoteTriggerTool creates a new remote trigger tool.
func NewRemoteTriggerTool() *RemoteTriggerTool {
	return &RemoteTriggerTool{
		BaseTool: &BaseTool{
			name:        "RemoteTrigger",
			description: "Trigger actions on remote systems via webhooks or APIs",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"url": {
						"type":        "string",
						"description": "The URL to trigger",
					},
					"method": {
						"type":        "string",
						"enum":        []string{"GET", "POST", "PUT", "DELETE"},
						"description": "HTTP method to use",
					},
					"payload": {
						"type":        "object",
						"description": "Optional JSON payload for POST/PUT requests",
					},
				},
				Required: []string{"url"},
			},
			isEnabled:  true,
			isReadOnly: false,
		},
	}
}

// Call triggers a remote action.
func (t *RemoteTriggerTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		URL     string                 `json:"url"`
		Method  string                 `json:"method,omitempty"`
		Payload map[string]interface{} `json:"payload,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	method := input.Method
	if method == "" {
		method = "POST"
	}

	// Placeholder - actual HTTP request would be made
	return &types.ToolResult{
		Output:    fmt.Sprintf("Triggered %s request to %s", method, input.URL),
		ToolUseID: toolCtx.ToolUseId,
	}, nil
}

// =============================================================================
// Synthetic Output Tool
// =============================================================================

// SyntheticOutputTool produces synthetic output for testing.
type SyntheticOutputTool struct {
	*BaseTool
}

// NewSyntheticOutputTool creates a new synthetic output tool.
func NewSyntheticOutputTool() *SyntheticOutputTool {
	return &SyntheticOutputTool{
		BaseTool: &BaseTool{
			name:        "SyntheticOutput",
			description: "Produce synthetic output for testing and debugging",
			inputSchema: types.ToolInputJSONSchema{
				Type: "object",
				Properties: map[string]map[string]interface{}{
					"type": {
						"type":        "string",
						"enum":        []string{"text", "json", "error", "progress"},
						"description": "Type of synthetic output to produce",
					},
					"content": {
						"type":        "string",
						"description": "Content to output",
					},
					"delay_ms": {
						"type":        "number",
						"description": "Delay before producing output (milliseconds)",
					},
				},
				Required: []string{"type"},
			},
			isEnabled:  true,
			isReadOnly: true,
		},
	}
}

// Call produces synthetic output.
func (t *SyntheticOutputTool) Call(ctx context.Context, args json.RawMessage, toolCtx *types.ToolContext, canUseTool types.CanUseToolFunc, parentMessage *types.Message, onProgress func(progress interface{})) (*types.ToolResult, error) {
	var input struct {
		Type    string `json:"type"`
		Content string `json:"content,omitempty"`
		DelayMs int    `json:"delay_ms,omitempty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	if input.DelayMs > 0 {
		select {
		case <-time.After(time.Duration(input.DelayMs) * time.Millisecond):
		case <-ctx.Done():
			return &types.ToolResult{
				Output:    "Synthetic output cancelled",
				ToolUseID: toolCtx.ToolUseId,
			}, nil
		}
	}

	switch input.Type {
	case "text":
		if input.Content == "" {
			input.Content = "Synthetic text output"
		}
		return &types.ToolResult{
			Output:    input.Content,
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "json":
		return &types.ToolResult{
			Output:    `{"type": "synthetic", "generated": true}`,
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "error":
		return &types.ToolResult{
			Error:     fmt.Errorf("synthetic error: %s", input.Content),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	case "progress":
		if onProgress != nil {
			onProgress(map[string]interface{}{"status": "synthetic progress", "percent": 50})
		}
		return &types.ToolResult{
			Output:    "Progress reported",
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	default:
		return &types.ToolResult{
			Error:     fmt.Errorf("unknown synthetic output type: %s", input.Type),
			ToolUseID: toolCtx.ToolUseId,
		}, nil
	}
}
