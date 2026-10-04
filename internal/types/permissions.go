package types

// Permission Modes - the different modes that control how permissions are handled.
type PermissionMode string

const (
	// External permission modes - user-addressable
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModePlan              PermissionMode = "plan"
	// PermissionModeSession allows every tool for the lifetime of the current
	// process only. It is intentionally never persisted as a default.
	PermissionModeSession PermissionMode = "session"

	// Internal permission modes
	PermissionModeAuto   PermissionMode = "auto"
	PermissionModeBubble PermissionMode = "bubble"
)

// ExternalPermissionModes is the list of user-addressable permission modes.
var ExternalPermissionModes = []PermissionMode{
	PermissionModeAcceptEdits,
	PermissionModeBypassPermissions,
	PermissionModeDefault,
	PermissionModeDontAsk,
	PermissionModePlan,
	PermissionModeSession,
}

// PermissionBehavior represents the action to take for a permission.
type PermissionBehavior string

const (
	PermissionBehaviorAllow PermissionBehavior = "allow"
	PermissionBehaviorDeny  PermissionBehavior = "deny"
	PermissionBehaviorAsk   PermissionBehavior = "ask"
)

// PermissionRuleSource indicates where a permission rule originated from.
type PermissionRuleSource string

const (
	SourceUserSettings    PermissionRuleSource = "userSettings"
	SourceProjectSettings PermissionRuleSource = "projectSettings"
	SourceLocalSettings   PermissionRuleSource = "localSettings"
	SourceFlagSettings    PermissionRuleSource = "flagSettings"
	SourcePolicySettings  PermissionRuleSource = "policySettings"
	SourceCliArg          PermissionRuleSource = "cliArg"
	SourceCommand         PermissionRuleSource = "command"
	SourceSession         PermissionRuleSource = "session"
)

// PermissionRuleValue specifies which tool and optional content for a rule.
type PermissionRuleValue struct {
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent,omitempty"`
}

// PermissionRule represents a permission rule with its source and behavior.
type PermissionRule struct {
	Source       PermissionRuleSource `json:"source"`
	RuleBehavior PermissionBehavior   `json:"ruleBehavior"`
	RuleValue    PermissionRuleValue  `json:"ruleValue"`
}

// PermissionUpdateDestination specifies where a permission update should be persisted.
type PermissionUpdateDestination string

const (
	DestUserSettings    PermissionUpdateDestination = "userSettings"
	DestProjectSettings PermissionUpdateDestination = "projectSettings"
	DestLocalSettings   PermissionUpdateDestination = "localSettings"
	DestSession         PermissionUpdateDestination = "session"
	DestCliArg          PermissionUpdateDestination = "cliArg"
)

// PermissionUpdate represents an update operation for permission configuration.
// It is a discriminated union on the 'type' field, matching the TypeScript source.
type PermissionUpdate struct {
	Type        PermissionUpdateType        `json:"type"`
	Destination PermissionUpdateDestination `json:"destination"`
	Rules       []PermissionRuleValue       `json:"rules,omitempty"`
	Behavior    PermissionBehavior          `json:"behavior,omitempty"`
	Mode        PermissionMode              `json:"mode,omitempty"`
	Directories []string                    `json:"directories,omitempty"`
}

// NewPermissionAddRulesUpdate creates an add-rules update.
func NewPermissionAddRulesUpdate(dest PermissionUpdateDestination, rules []PermissionRuleValue, behavior PermissionBehavior) PermissionUpdate {
	return PermissionUpdate{Type: UpdateTypeAddRules, Destination: dest, Rules: rules, Behavior: behavior}
}

// NewPermissionReplaceRulesUpdate creates a replace-rules update.
func NewPermissionReplaceRulesUpdate(dest PermissionUpdateDestination, rules []PermissionRuleValue, behavior PermissionBehavior) PermissionUpdate {
	return PermissionUpdate{Type: UpdateTypeReplaceRules, Destination: dest, Rules: rules, Behavior: behavior}
}

// NewPermissionRemoveRulesUpdate creates a remove-rules update.
func NewPermissionRemoveRulesUpdate(dest PermissionUpdateDestination, rules []PermissionRuleValue, behavior PermissionBehavior) PermissionUpdate {
	return PermissionUpdate{Type: UpdateTypeRemoveRules, Destination: dest, Rules: rules, Behavior: behavior}
}

// NewPermissionSetModeUpdate creates a set-mode update.
func NewPermissionSetModeUpdate(dest PermissionUpdateDestination, mode PermissionMode) PermissionUpdate {
	return PermissionUpdate{Type: UpdateTypeSetMode, Destination: dest, Mode: mode}
}

// NewPermissionAddDirectoriesUpdate creates an add-directories update.
func NewPermissionAddDirectoriesUpdate(dest PermissionUpdateDestination, dirs []string) PermissionUpdate {
	return PermissionUpdate{Type: UpdateTypeAddDirectories, Destination: dest, Directories: dirs}
}

// NewPermissionRemoveDirectoriesUpdate creates a remove-directories update.
func NewPermissionRemoveDirectoriesUpdate(dest PermissionUpdateDestination, dirs []string) PermissionUpdate {
	return PermissionUpdate{Type: UpdateTypeRemoveDirectories, Destination: dest, Directories: dirs}
}

type PermissionUpdateType string

const (
	UpdateTypeAddRules          PermissionUpdateType = "addRules"
	UpdateTypeReplaceRules      PermissionUpdateType = "replaceRules"
	UpdateTypeRemoveRules       PermissionUpdateType = "removeRules"
	UpdateTypeSetMode           PermissionUpdateType = "setMode"
	UpdateTypeAddDirectories    PermissionUpdateType = "addDirectories"
	UpdateTypeRemoveDirectories PermissionUpdateType = "removeDirectories"
)

// WorkingDirectorySource is the source of an additional working directory permission.
type WorkingDirectorySource = PermissionRuleSource

// AdditionalWorkingDirectory represents a directory included in permission scope.
type AdditionalWorkingDirectory struct {
	Path   string                 `json:"path"`
	Source WorkingDirectorySource `json:"source"`
}

// PermissionDecision represents a decision about a permission request.
type PermissionDecision struct {
	Behavior       PermissionBehavior        `json:"behavior"`
	Message        string                    `json:"message,omitempty"`
	UpdatedInput   map[string]interface{}    `json:"updatedInput,omitempty"`
	UserModified   bool                      `json:"userModified,omitempty"`
	DecisionReason *PermissionDecisionReason `json:"decisionReason,omitempty"`
	ToolUseID      string                    `json:"toolUseID,omitempty"`
	AcceptFeedback string                    `json:"acceptFeedback,omitempty"`
	// For ask behavior
	Suggestions            []PermissionUpdate      `json:"suggestions,omitempty"`
	BlockedPath            string                  `json:"blockedPath,omitempty"`
	PendingClassifierCheck *PendingClassifierCheck `json:"pendingClassifierCheck,omitempty"`
}

// PendingClassifierCheck contains metadata for a pending classifier check.
type PendingClassifierCheck struct {
	Command      string   `json:"command"`
	Cwd          string   `json:"cwd"`
	Descriptions []string `json:"descriptions"`
}

// PermissionDecisionReason explains why a permission decision was made.
// It is a discriminated union on the 'type' field.
type PermissionDecisionReason struct {
	Type string `json:"type"`

	// For type "rule"
	Rule *PermissionRule `json:"rule,omitempty"`

	// For type "mode"
	Mode PermissionMode `json:"mode,omitempty"`

	// For type "subcommandResults"
	Reasons map[string]interface{} `json:"reasons,omitempty"`

	// For type "permissionPromptTool"
	PermissionPromptToolName string      `json:"permissionPromptToolName,omitempty"`
	ToolResult               interface{} `json:"toolResult,omitempty"`

	// For type "hook"
	HookName   string `json:"hookName,omitempty"`
	HookSource string `json:"hookSource,omitempty"`
	Reason     string `json:"reason,omitempty"`

	// For type "classifier"
	Classifier  string `json:"classifier,omitempty"`
	ClassReason string `json:"classReason,omitempty"` // for type classifier

	// For type "safetyCheck"
	SafetyCheckReason    string `json:"safetyCheckReason,omitempty"`
	ClassifierApprovable bool   `json:"classifierApprovable,omitempty"`

	// For type "asyncAgent"
	AsyncReason string `json:"asyncReason,omitempty"`

	// For type "workingDir"
	WorkingDirReason string `json:"workingDirReason,omitempty"`

	// For type "sandboxOverride"
	SandboxOverrideReason string `json:"sandboxOverrideReason,omitempty"`

	// For type "other"
	OtherReason string `json:"otherReason,omitempty"`
}

// PermissionResult represents the result of a permission check.
// It is a discriminated union: allow, deny, ask, or passthrough.
type PermissionResult struct {
	Behavior               PermissionBehavior        `json:"behavior"`
	Message                string                    `json:"message,omitempty"`
	UpdatedInput           map[string]interface{}    `json:"updatedInput,omitempty"`
	DecisionReason         *PermissionDecisionReason `json:"decisionReason,omitempty"`
	Suggestions            []PermissionUpdate        `json:"suggestions,omitempty"`
	BlockedPath            string                    `json:"blockedPath,omitempty"`
	PendingClassifierCheck *PendingClassifierCheck   `json:"pendingClassifierCheck,omitempty"`
	UserModified           bool                      `json:"userModified,omitempty"`
	ToolUseID              string                    `json:"toolUseID,omitempty"`
	AcceptFeedback         string                    `json:"acceptFeedback,omitempty"`
}

// NewPermissionAllowResult creates an allow permission result.
func NewPermissionAllowResult(updatedInput map[string]interface{}) PermissionResult {
	return PermissionResult{
		Behavior:     PermissionBehaviorAllow,
		UpdatedInput: updatedInput,
	}
}

// NewPermissionDenyResult creates a deny permission result.
func NewPermissionDenyResult(message string, reason *PermissionDecisionReason) PermissionResult {
	return PermissionResult{
		Behavior:       PermissionBehaviorDeny,
		Message:        message,
		DecisionReason: reason,
	}
}

// NewPermissionAskResult creates an ask permission result.
func NewPermissionAskResult(message string, suggestions []PermissionUpdate) PermissionResult {
	return PermissionResult{
		Behavior:    PermissionBehaviorAsk,
		Message:     message,
		Suggestions: suggestions,
	}
}

// NewPermissionPassthroughResult creates a passthrough permission result.
func NewPermissionPassthroughResult(message string, suggestions []PermissionUpdate) PermissionResult {
	return PermissionResult{
		Behavior:    "passthrough",
		Message:     message,
		Suggestions: suggestions,
	}
}

// IsAllow returns true if the permission result is an allow.
func (r *PermissionResult) IsAllow() bool {
	return r.Behavior == PermissionBehaviorAllow
}

// IsDeny returns true if the permission result is a deny.
func (r *PermissionResult) IsDeny() bool {
	return r.Behavior == PermissionBehaviorDeny
}

// IsAsk returns true if the permission result requires user prompting.
func (r *PermissionResult) IsAsk() bool {
	return r.Behavior == PermissionBehaviorAsk
}

// IsPassthrough returns true if the permission should be passed through.
func (r *PermissionResult) IsPassthrough() bool {
	return r.Behavior == "passthrough"
}

// ToolPermissionRulesBySource maps permission rules by their source.
type ToolPermissionRulesBySource map[PermissionRuleSource][]string

// ToolPermissionContext provides context needed for permission checking in tools.
type ToolPermissionContext struct {
	Mode                             PermissionMode                        `json:"mode"`
	AdditionalWorkingDirectories     map[string]AdditionalWorkingDirectory `json:"additionalWorkingDirectories"`
	AlwaysAllowRules                 ToolPermissionRulesBySource           `json:"alwaysAllowRules"`
	AlwaysDenyRules                  ToolPermissionRulesBySource           `json:"alwaysDenyRules"`
	AlwaysAskRules                   ToolPermissionRulesBySource           `json:"alwaysAskRules"`
	IsBypassPermissionsModeAvailable bool                                  `json:"isBypassPermissionsModeAvailable"`
	IsAutoModeAvailable              bool                                  `json:"isAutoModeAvailable,omitempty"`
	StrippedDangerousRules           ToolPermissionRulesBySource           `json:"strippedDangerousRules,omitempty"`
	ShouldAvoidPermissionPrompts     bool                                  `json:"shouldAvoidPermissionPrompts,omitempty"`
	AwaitAutomatedChecksBeforeDialog bool                                  `json:"awaitAutomatedChecksBeforeDialog,omitempty"`
	PrePlanMode                      PermissionMode                        `json:"prePlanMode,omitempty"`
}

// GetEmptyToolPermissionContext returns an empty ToolPermissionContext with default values.
func GetEmptyToolPermissionContext() ToolPermissionContext {
	return ToolPermissionContext{
		Mode:                             PermissionModeDefault,
		AdditionalWorkingDirectories:     make(map[string]AdditionalWorkingDirectory),
		AlwaysAllowRules:                 make(ToolPermissionRulesBySource),
		AlwaysDenyRules:                  make(ToolPermissionRulesBySource),
		AlwaysAskRules:                   make(ToolPermissionRulesBySource),
		IsBypassPermissionsModeAvailable: false,
	}
}

// ClassifierResult represents the result of a classifier evaluation.
type ClassifierResult struct {
	Matches            bool   `json:"matches"`
	MatchedDescription string `json:"matchedDescription,omitempty"`
	Confidence         string `json:"confidence"` // high | medium | low
	Reason             string `json:"reason"`
}

// ClassifierBehavior represents the behavior a classifier recommends.
type ClassifierBehavior string

const (
	ClassifierBehaviorDeny  ClassifierBehavior = "deny"
	ClassifierBehaviorAsk   ClassifierBehavior = "ask"
	ClassifierBehaviorAllow ClassifierBehavior = "allow"
)

// YoloClassifierResult represents the result of a YOLO classifier check.
type YoloClassifierResult struct {
	Thinking          string           `json:"thinking,omitempty"`
	ShouldBlock       bool             `json:"shouldBlock"`
	Reason            string           `json:"reason"`
	Unavailable       bool             `json:"unavailable,omitempty"`
	TranscriptTooLong bool             `json:"transcriptTooLong,omitempty"`
	Model             string           `json:"model"`
	Usage             *ClassifierUsage `json:"usage,omitempty"`
	DurationMs        int64            `json:"durationMs,omitempty"`
	PromptLengths     *PromptLengths   `json:"promptLengths,omitempty"`
	ErrorDumpPath     string           `json:"errorDumpPath,omitempty"`
	Stage             string           `json:"stage,omitempty"` // fast | thinking
	Stage1Usage       *ClassifierUsage `json:"stage1Usage,omitempty"`
	Stage1DurationMs  int64            `json:"stage1DurationMs,omitempty"`
	Stage1RequestId   string           `json:"stage1RequestId,omitempty"`
	Stage1MsgId       string           `json:"stage1MsgId,omitempty"`
	Stage2Usage       *ClassifierUsage `json:"stage2Usage,omitempty"`
	Stage2DurationMs  int64            `json:"stage2DurationMs,omitempty"`
	Stage2RequestId   string           `json:"stage2RequestId,omitempty"`
	Stage2MsgId       string           `json:"stage2MsgId,omitempty"`
}

// ClassifierUsage contains token usage from a classifier API call.
type ClassifierUsage struct {
	InputTokens              int64 `json:"inputTokens"`
	OutputTokens             int64 `json:"outputTokens"`
	CacheReadInputTokens     int64 `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int64 `json:"cacheCreationInputTokens"`
}

// PromptLengths contains character lengths of prompt components.
type PromptLengths struct {
	SystemPrompt int `json:"systemPrompt"`
	ToolCalls    int `json:"toolCalls"`
	UserPrompts  int `json:"userPrompts"`
}

// RiskLevel represents the risk level of an operation.
type RiskLevel string

const (
	RiskLevelLow    RiskLevel = "LOW"
	RiskLevelMedium RiskLevel = "MEDIUM"
	RiskLevelHigh   RiskLevel = "HIGH"
)

// PermissionExplanation provides an explanation for a permission decision.
type PermissionExplanation struct {
	RiskLevel   RiskLevel `json:"riskLevel"`
	Explanation string    `json:"explanation"`
	Reasoning   string    `json:"reasoning"`
	Risk        string    `json:"risk"`
}
