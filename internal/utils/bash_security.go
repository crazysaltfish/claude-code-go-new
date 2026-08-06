package utils

import (
	"fmt"
	"regexp"
	"strings"
)

// =============================================================================
// Bash Security Types
// =============================================================================

// BashSecurityCheckResult represents the result of a security check.
type BashSecurityCheckResult struct {
	Behavior       string          `json:"behavior"` // "allow", "deny", "ask", "passthrough"
	Message        string          `json:"message,omitempty"`
	DecisionReason *SecurityReason `json:"decisionReason,omitempty"`
}

// SecurityReason explains why a security decision was made.
type SecurityReason struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

// =============================================================================
// Command Substitution Patterns
// =============================================================================

// CommandSubstitutionPatterns contains patterns that indicate command substitution.
var CommandSubstitutionPatterns = []struct {
	Pattern string
	Message string
}{
	{`<\(`, "process substitution <()"},
	{`>\(`, "process substitution >()"},
	{`=\(`, "Zsh process substitution =()"},
	{`(?:^|[\s;&|])=[a-zA-Z_]`, "Zsh equals expansion (=cmd)"},
	{`\$\(`, "$() command substitution"},
	{`\$\{`, "${} parameter substitution"},
	{`\$\[`, "$[] legacy arithmetic expansion"},
	{`~\[`, "Zsh-style parameter expansion"},
	{`\(e:`, "Zsh-style glob qualifiers"},
	{`\(\+`, "Zsh glob qualifier with command execution"},
	{`\}\s*always\s*\{`, "Zsh always block (try/always construct)"},
	{`<#`, "PowerShell comment syntax"},
}

// ZshDangerousCommands contains Zsh-specific dangerous commands.
var ZshDangerousCommands = map[string]bool{
	"zmodload":  true,
	"emulate":   true,
	"sysopen":   true,
	"sysread":   true,
	"syswrite":  true,
	"sysseek":   true,
	"zpty":      true,
	"ztcp":      true,
	"zsocket":   true,
	"mapfile":   true,
	"readarray": true,
	"zf_rm":     true,
	"zf_mv":     true,
	"zf_ln":     true,
	"zf_chmod":  true,
	"zf_chown":  true,
	"zf_mkdir":  true,
	"zf_rmdir":  true,
	"zf_chgrp":  true,
}

// Note: evalLikeBuiltins, subscriptEvalFlags, testArithCmpOps, bareSubscriptNameBuiltins, readDataFlags, arithLeafRE
// are defined in shell_ast.go to avoid duplication.

// =============================================================================
// Quote Extraction
// =============================================================================

// QuoteExtraction contains extracted quoted content.
type QuoteExtraction struct {
	WithDoubleQuotes      string
	FullyUnquoted         string
	UnquotedKeepQuoteChar string
}

// ExtractQuotedContent extracts quoted content from a command string.
func ExtractQuotedContent(command string) QuoteExtraction {
	var withDoubleQuotes, fullyUnquoted, unquotedKeepQuoteChar strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i := 0; i < len(command); i++ {
		char := command[i]

		if escaped {
			escaped = false
			if !inSingleQuote {
				withDoubleQuotes.WriteByte(char)
			}
			if !inSingleQuote && !inDoubleQuote {
				fullyUnquoted.WriteByte(char)
				unquotedKeepQuoteChar.WriteByte(char)
			}
			continue
		}

		if char == '\\' && !inSingleQuote {
			escaped = true
			if !inSingleQuote {
				withDoubleQuotes.WriteByte(char)
			}
			if !inSingleQuote && !inDoubleQuote {
				fullyUnquoted.WriteByte(char)
				unquotedKeepQuoteChar.WriteByte(char)
			}
			continue
		}

		if char == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			unquotedKeepQuoteChar.WriteByte(char)
			continue
		}

		if char == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			unquotedKeepQuoteChar.WriteByte(char)
			continue
		}

		if !inSingleQuote {
			withDoubleQuotes.WriteByte(char)
		}
		if !inSingleQuote && !inDoubleQuote {
			fullyUnquoted.WriteByte(char)
			unquotedKeepQuoteChar.WriteByte(char)
		}
	}

	return QuoteExtraction{
		WithDoubleQuotes:      withDoubleQuotes.String(),
		FullyUnquoted:         fullyUnquoted.String(),
		UnquotedKeepQuoteChar: unquotedKeepQuoteChar.String(),
	}
}

// =============================================================================
// Security Validation Functions
// =============================================================================

// ValidateIncompleteCommands checks for incomplete command patterns.
func ValidateIncompleteCommands(command string) BashSecurityCheckResult {
	trimmed := strings.TrimSpace(command)

	// Check for tab at start
	if strings.HasPrefix(command, "\t") {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Command appears to be an incomplete fragment (starts with tab)",
		}
	}

	// Check for flag at start
	if strings.HasPrefix(trimmed, "-") {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Command appears to be an incomplete fragment (starts with flags)",
		}
	}

	// Check for operators at start
	if matched, _ := regexp.MatchString(`^\s*(&&|\|\||;|>>?|<)`, command); matched {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Command appears to be a continuation line (starts with operator)",
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "Command appears complete"}
}

// ValidateDangerousPatterns checks for dangerous command patterns.
func ValidateDangerousPatterns(command string) BashSecurityCheckResult {
	extraction := ExtractQuotedContent(command)
	fullyUnquoted := extraction.FullyUnquoted

	// Check command substitution patterns
	for _, pattern := range CommandSubstitutionPatterns {
		matched, err := regexp.MatchString(pattern.Pattern, fullyUnquoted)
		if err == nil && matched {
			return BashSecurityCheckResult{
				Behavior: "ask",
				Message:  fmt.Sprintf("Command contains %s", pattern.Message),
			}
		}
	}

	// Check for unescaped backticks
	if hasUnescapedChar(fullyUnquoted, '`') {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Command contains backtick command substitution",
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "No dangerous patterns detected"}
}

// ValidateZshDangerousCommands checks for Zsh-specific dangerous commands.
func ValidateZshDangerousCommands(command string) BashSecurityCheckResult {
	extraction := ExtractQuotedContent(command)
	fullyUnquoted := extraction.FullyUnquoted

	// Split into words and check base command
	words := strings.Fields(fullyUnquoted)
	if len(words) > 0 {
		baseCommand := words[0]
		// Remove path prefix if present
		if idx := strings.LastIndex(baseCommand, "/"); idx != -1 {
			baseCommand = baseCommand[idx+1:]
		}

		if ZshDangerousCommands[baseCommand] {
			return BashSecurityCheckResult{
				Behavior: "deny",
				Message:  fmt.Sprintf("Dangerous Zsh command detected: %s", baseCommand),
			}
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "No Zsh dangerous commands detected"}
}

// ValidateShellMetacharacters checks for dangerous shell metacharacters.
func ValidateShellMetacharacters(command string) BashSecurityCheckResult {
	extraction := ExtractQuotedContent(command)
	fullyUnquoted := extraction.FullyUnquoted

	// Check for IFS injection
	if strings.Contains(fullyUnquoted, "IFS=") {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Command attempts IFS injection",
		}
	}

	// Check for control characters
	controlPattern := regexp.MustCompile(`[\x00-\x1f\x7f]`)
	if controlPattern.MatchString(fullyUnquoted) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Command contains control characters",
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "No dangerous metacharacters detected"}
}

// ValidateObfuscatedFlags checks for obfuscated command flags.
func ValidateObfuscatedFlags(command string) BashSecurityCheckResult {
	extraction := ExtractQuotedContent(command)
	fullyUnquoted := extraction.FullyUnquoted

	// Check for backslash-escaped whitespace at word boundaries
	escapedSpacePattern := regexp.MustCompile(`\\[ \t]`)
	if escapedSpacePattern.MatchString(fullyUnquoted) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Command contains escaped whitespace (possible obfuscation)",
		}
	}

	// Check for Unicode whitespace
	unicodeWhitespace := regexp.MustCompile(`[\x{00a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`)
	if unicodeWhitespace.MatchString(command) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Command contains Unicode whitespace (possible obfuscation)",
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "No obfuscated flags detected"}
}

// ValidateBraceExpansion checks for dangerous brace expansion.
func ValidateBraceExpansion(command string) BashSecurityCheckResult {
	extraction := ExtractQuotedContent(command)
	fullyUnquoted := extraction.FullyUnquoted

	// Check for brace expansion that could be used for obfuscation
	bracePattern := regexp.MustCompile(`\{[^}]*,.*\}`)
	if bracePattern.MatchString(fullyUnquoted) {
		// Check if it's in a safe context
		if strings.Contains(fullyUnquoted, "echo {") || strings.Contains(fullyUnquoted, "printf {") {
			return BashSecurityCheckResult{
				Behavior: "passthrough",
				Message:  "Brace expansion appears safe",
			}
		}
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Command contains brace expansion",
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "No dangerous brace expansion detected"}
}

// ValidateJqSystemFunction checks for dangerous jq system functions.
func ValidateJqSystemFunction(command string) BashSecurityCheckResult {
	// Check if this is a jq command
	if !strings.Contains(command, "jq ") {
		return BashSecurityCheckResult{Behavior: "passthrough", Message: "Not a jq command"}
	}

	// Check for system function in jq
	if strings.Contains(command, "system(") || strings.Contains(command, "exec(") {
		return BashSecurityCheckResult{
			Behavior: "deny",
			Message:  "jq command contains system/exec function",
		}
	}

	// Check for input/output file arguments
	jqPatterns := []string{"-f ", "--from-file ", "-o ", "--output-file "}
	for _, pattern := range jqPatterns {
		if strings.Contains(command, pattern) {
			return BashSecurityCheckResult{
				Behavior: "ask",
				Message:  "jq command uses file I/O",
			}
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "jq command appears safe"}
}

// ValidateProcEnvironAccess checks for /proc/environ access patterns.
func ValidateProcEnvironAccess(command string) BashSecurityCheckResult {
	// Check for /proc environ access
	environPatterns := []string{
		"/proc/self/environ",
		"/proc/*/environ",
		"/proc/self/cmdline",
		"/proc/*/cmdline",
	}

	for _, pattern := range environPatterns {
		if strings.Contains(command, pattern) {
			return BashSecurityCheckResult{
				Behavior: "ask",
				Message:  fmt.Sprintf("Command accesses %s", pattern),
			}
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "No /proc access detected"}
}

// ValidateGitCommitSubstitution checks for git commit message substitution.
func ValidateGitCommitSubstitution(command string) BashSecurityCheckResult {
	// Check if this is a git commit command
	if !strings.Contains(command, "git commit") {
		return BashSecurityCheckResult{Behavior: "passthrough", Message: "Not a git commit command"}
	}

	// Check for substitution in commit message
	extraction := ExtractQuotedContent(command)
	fullyUnquoted := extraction.FullyUnquoted

	if strings.Contains(fullyUnquoted, "$(") || strings.Contains(fullyUnquoted, "`") {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "git commit message contains command substitution",
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "git commit appears safe"}
}

// ValidateCommentQuoteDesync checks for comment/quote desynchronization attacks.
func ValidateCommentQuoteDesync(command string) BashSecurityCheckResult {
	// Track quote state and look for # after unclosed quotes
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i := 0; i < len(command); i++ {
		char := command[i]

		if escaped {
			escaped = false
			continue
		}

		if char == '\\' && !inSingleQuote {
			escaped = true
			continue
		}

		if char == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}

		if char == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}

		// Check for # in unclosed quote context
		if char == '#' && (inSingleQuote || inDoubleQuote) {
			return BashSecurityCheckResult{
				Behavior: "ask",
				Message:  "Potential comment/quote desynchronization detected",
			}
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough", Message: "No quote desync detected"}
}

// =============================================================================
// Main Security Check Function
// =============================================================================

// BashCommandIsSafe performs comprehensive security checks on a bash command.
// Returns true if the command is considered safe, false otherwise.
func BashCommandIsSafe(command string) BashSecurityCheckResult {
	// Empty command is safe
	if strings.TrimSpace(command) == "" {
		return BashSecurityCheckResult{
			Behavior: "allow",
			Message:  "Empty command is safe",
		}
	}

	// Run all validators in sequence
	validators := []func(string) BashSecurityCheckResult{
		ValidateIncompleteCommands,
		ValidateDangerousPatterns,
		ValidateZshDangerousCommands,
		ValidateShellMetacharacters,
		ValidateObfuscatedFlags,
		ValidateBraceExpansion,
		ValidateJqSystemFunction,
		ValidateProcEnvironAccess,
		ValidateGitCommitSubstitution,
		ValidateCommentQuoteDesync,
		ValidateEvalLikeBuiltins,
		ValidateSubscriptEvaluation,
		ValidateArithmeticExpansion,
		ValidateVariableAssignment,
		ValidateHeredocSafety,
		ValidateWrapperCommands,
	}

	for _, validator := range validators {
		result := validator(command)
		if result.Behavior != "passthrough" {
			return result
		}
	}

	// All checks passed
	return BashSecurityCheckResult{
		Behavior: "allow",
		Message:  "Command passed all security checks",
	}
}

// ValidateEvalLikeBuiltins checks for eval-like builtins that execute code.
func ValidateEvalLikeBuiltins(command string) BashSecurityCheckResult {
	extraction := ExtractQuotedContent(command)
	fullyUnquoted := extraction.FullyUnquoted

	// Get the base command
	words := strings.Fields(fullyUnquoted)
	if len(words) == 0 {
		return BashSecurityCheckResult{Behavior: "passthrough"}
	}

	baseCmd := words[0]

	// Remove path prefix if present
	if idx := strings.LastIndex(baseCmd, "/"); idx != -1 {
		baseCmd = baseCmd[idx+1:]
	}

	// Check for eval-like builtins
	if evalLikeBuiltins[baseCmd] {
		// command -v/-V are safe
		if baseCmd == "command" && len(words) > 1 && (words[1] == "-v" || words[1] == "-V") {
			return BashSecurityCheckResult{Behavior: "passthrough"}
		}

		// fc without -e/-s is safe
		if baseCmd == "fc" {
			for _, arg := range words[1:] {
				if matched, _ := regexp.MatchString(`^-[^-]*[es]`, arg); matched {
					return BashSecurityCheckResult{
						Behavior: "ask",
						Message:  "'fc -e/-s' evaluates arguments as shell code",
					}
				}
			}
			return BashSecurityCheckResult{Behavior: "passthrough"}
		}

		// compgen without -C/-F/-W is safe
		if baseCmd == "compgen" {
			for _, arg := range words[1:] {
				if matched, _ := regexp.MatchString(`^-[^-]*[CFW]`, arg); matched {
					return BashSecurityCheckResult{
						Behavior: "ask",
						Message:  "'compgen -C/-F/-W' evaluates arguments as shell code",
					}
				}
			}
			return BashSecurityCheckResult{Behavior: "passthrough"}
		}

		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  fmt.Sprintf("'%s' evaluates arguments as shell code", baseCmd),
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough"}
}

// ValidateSubscriptEvaluation checks for subscript evaluation vulnerabilities.
func ValidateSubscriptEvaluation(command string) BashSecurityCheckResult {
	extraction := ExtractQuotedContent(command)
	fullyUnquoted := extraction.FullyUnquoted

	words := strings.Fields(fullyUnquoted)
	if len(words) == 0 {
		return BashSecurityCheckResult{Behavior: "passthrough"}
	}

	baseCmd := words[0]

	// Check for flags that take NAME argument
	dangerFlags, hasDangerFlags := subscriptEvalFlags[baseCmd]
	if hasDangerFlags {
		for i := 1; i < len(words); i++ {
			arg := words[i]

			// Separate form: -v then NAME in next arg
			if dangerFlags[arg] && i+1 < len(words) && strings.Contains(words[i+1], "[") {
				return BashSecurityCheckResult{
					Behavior: "ask",
					Message:  fmt.Sprintf("'%s %s' operand contains array subscript — bash evaluates $(cmd) in subscripts", baseCmd, arg),
				}
			}

			// Fused form: -vNAME in one arg
			for flag := range dangerFlags {
				if len(flag) == 2 && strings.HasPrefix(arg, flag) && len(arg) > 2 && strings.Contains(arg, "[") {
					return BashSecurityCheckResult{
						Behavior: "ask",
						Message:  fmt.Sprintf("'%s %s' (fused) operand contains array subscript — bash evaluates $(cmd) in subscripts", baseCmd, flag),
					}
				}
			}
		}
	}

	// Check for [[ ]] arithmetic comparison
	if baseCmd == "[[" {
		for i := 2; i < len(words); i++ {
			if testArithCmpOps[words[i]] {
				if i > 0 && strings.Contains(words[i-1], "[") {
					return BashSecurityCheckResult{
						Behavior: "ask",
						Message:  fmt.Sprintf("'[[ ... %s ... ]]' operand contains array subscript", words[i]),
					}
				}
				if i+1 < len(words) && strings.Contains(words[i+1], "[") {
					return BashSecurityCheckResult{
						Behavior: "ask",
						Message:  fmt.Sprintf("'[[ ... %s ... ]]' operand contains array subscript", words[i]),
					}
				}
			}
		}
	}

	// Check for read/unset positional NAME args
	if bareSubscriptNameBuiltins[baseCmd] {
		skipNext := false
		for i := 1; i < len(words); i++ {
			arg := words[i]

			if skipNext {
				skipNext = false
				continue
			}

			if strings.HasPrefix(arg, "-") {
				if baseCmd == "read" && readDataFlags[arg] {
					skipNext = true
				}
				continue
			}

			if strings.Contains(arg, "[") {
				return BashSecurityCheckResult{
					Behavior: "ask",
					Message:  fmt.Sprintf("'%s' positional NAME '%s' contains array subscript", baseCmd, arg),
				}
			}
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough"}
}

// ValidateArithmeticExpansion checks for dangerous arithmetic expansion.
func ValidateArithmeticExpansion(command string) BashSecurityCheckResult {
	// Check for $(( )) with variable references
	arithPattern := regexp.MustCompile(`\$\(\([^)]*[A-Za-z_][A-Za-z0-9_]*[^)]*\)\)`)
	if arithPattern.MatchString(command) {
		// Check if it contains only safe numeric literals
		extracted := arithPattern.FindString(command)
		if !arithLeafRE.MatchString(extracted) {
			return BashSecurityCheckResult{
				Behavior: "ask",
				Message:  "Arithmetic expansion references variable — potential code execution",
			}
		}
	}

	// Check for let command with variables
	letPattern := regexp.MustCompile(`\blet\s+[^;]*[A-Za-z_]`)
	if letPattern.MatchString(command) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "'let' command with variable reference — potential code execution",
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough"}
}

// ValidateVariableAssignment checks for dangerous variable assignments.
func ValidateVariableAssignment(command string) BashSecurityCheckResult {
	// Check for IFS assignment
	ifsPattern := regexp.MustCompile(`(?:^|[\s;|&])IFS=`)
	if ifsPattern.MatchString(command) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "IFS assignment changes word-splitting — cannot model statically",
		}
	}

	// Check for PS4 assignment (trace-time RCE)
	ps4Pattern := regexp.MustCompile(`(?:^|[\s;|&])PS4=`)
	if ps4Pattern.MatchString(command) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "PS4 assignment — potential trace-time code execution",
		}
	}

	// Check for tilde in variable value
	tildePattern := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*=~[^/]*`)
	if tildePattern.MatchString(command) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Tilde in assignment value — bash may expand at assignment time",
		}
	}

	// Check for invalid variable names (bash treats as command)
	invalidVarPattern := regexp.MustCompile(`\b[0-9][A-Za-z0-9_]*=`)
	if invalidVarPattern.MatchString(command) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Invalid variable name (bash treats as command)",
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough"}
}

// ValidateHeredocSafety checks for heredoc safety issues.
func ValidateHeredocSafety(command string) BashSecurityCheckResult {
	// Check for unquoted heredoc delimiter
	unquotedHeredocPattern := regexp.MustCompile(`<<[ \t]*[^ \t\n'"\\]`)
	if unquotedHeredocPattern.MatchString(command) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Heredoc with unquoted delimiter undergoes shell expansion",
		}
	}

	// Check for here-string with expansion
	hereStringPattern := regexp.MustCompile(`<<<[^'"].*[$\x60]`)
	if hereStringPattern.MatchString(command) {
		return BashSecurityCheckResult{
			Behavior: "ask",
			Message:  "Here-string with variable/expansion — dynamic content",
		}
	}

	return BashSecurityCheckResult{Behavior: "passthrough"}
}

// ValidateWrapperCommands validates wrapper commands (timeout, nice, env, stdbuf, time, nohup).
func ValidateWrapperCommands(command string) BashSecurityCheckResult {
	extraction := ExtractQuotedContent(command)
	fullyUnquoted := extraction.FullyUnquoted

	words := strings.Fields(fullyUnquoted)
	if len(words) == 0 {
		return BashSecurityCheckResult{Behavior: "passthrough"}
	}

	baseCmd := words[0]

	// Check timeout with unknown flags
	if baseCmd == "timeout" {
		for i := 1; i < len(words); i++ {
			arg := words[i]
			if strings.HasPrefix(arg, "--") {
				// Allow known long flags
				knownLongFlags := []string{"--foreground", "--preserve-status", "--verbose", "--kill-after", "--signal"}
				known := false
				for _, knownFlag := range knownLongFlags {
					if strings.HasPrefix(arg, knownFlag) {
						known = true
						break
					}
				}
				if !known {
					return BashSecurityCheckResult{
						Behavior: "ask",
						Message:  fmt.Sprintf("timeout with %s flag cannot be statically analyzed", arg),
					}
				}
			}
			if strings.HasPrefix(arg, "-") && len(arg) > 1 {
				// Allow known short flags
				knownShortPattern := regexp.MustCompile(`^-[vks]`)
				if !knownShortPattern.MatchString(arg) {
					return BashSecurityCheckResult{
						Behavior: "ask",
						Message:  fmt.Sprintf("timeout with %s flag cannot be statically analyzed", arg),
					}
				}
			}
		}
	}

	// Check nice with expansion in argument
	if baseCmd == "nice" && len(words) > 1 {
		arg := words[1]
		if strings.Contains(arg, "$") || strings.Contains(arg, "`") {
			return BashSecurityCheckResult{
				Behavior: "ask",
				Message:  "nice argument contains expansion — cannot statically determine wrapped command",
			}
		}
	}

	// Check env with dangerous flags
	if baseCmd == "env" {
		for i := 1; i < len(words); i++ {
			arg := words[i]
			if arg == "-S" || arg == "-C" || arg == "-P" {
				return BashSecurityCheckResult{
					Behavior: "ask",
					Message:  fmt.Sprintf("env with %s flag cannot be statically analyzed", arg),
				}
			}
		}
	}

	// Check stdbuf with unknown flags
	if baseCmd == "stdbuf" {
		for i := 1; i < len(words); i++ {
			arg := words[i]
			if strings.HasPrefix(arg, "-") {
				// Allow -i, -o, -e with optional value
				stdbufShortPattern := regexp.MustCompile(`^-[ioe]`)
				stdbufLongPattern := regexp.MustCompile(`^--(input|output|error)=`)
				if !stdbufShortPattern.MatchString(arg) && !stdbufLongPattern.MatchString(arg) {
					return BashSecurityCheckResult{
						Behavior: "ask",
						Message:  fmt.Sprintf("stdbuf with %s flag cannot be statically analyzed", arg),
					}
				}
			}
		}
	}

	// time and nohup wrap other commands - need to check wrapped command
	if baseCmd == "time" || baseCmd == "nohup" {
		// These are generally safe wrappers, but we should note they wrap commands
		return BashSecurityCheckResult{Behavior: "passthrough"}
	}

	return BashSecurityCheckResult{Behavior: "passthrough"}
}

// =============================================================================
// Helper Functions
// =============================================================================

// hasUnescapedChar checks if a string contains an unescaped character.
func hasUnescapedChar(content string, char byte) bool {
	i := 0
	for i < len(content) {
		if content[i] == '\\' && i+1 < len(content) {
			i += 2 // Skip backslash and escaped character
			continue
		}
		if content[i] == char {
			return true
		}
		i++
	}
	return false
}

// StripSafeRedirections removes safe redirection patterns from a command.
func StripSafeRedirections(content string) string {
	// Remove 2>&1
	re1 := regexp.MustCompile(`\s+2\s*>&\s*1(?=\s|$)`)
	content = re1.ReplaceAllString(content, "")

	// Remove >/dev/null variants
	re2 := regexp.MustCompile(`[012]?\s*>\s*/dev/null(?=\s|$)`)
	content = re2.ReplaceAllString(content, "")

	// Remove </dev/null
	re3 := regexp.MustCompile(`\s*<\s*/dev/null(?=\s|$)`)
	content = re3.ReplaceAllString(content, "")

	return content
}

// GetBaseCommand extracts the base command from a command string.
func GetBaseCommand(command string) string {
	// Remove leading whitespace
	command = strings.TrimSpace(command)

	// Split by whitespace
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}

	baseCommand := parts[0]

	// Remove path prefix
	if idx := strings.LastIndex(baseCommand, "/"); idx != -1 {
		baseCommand = baseCommand[idx+1:]
	}

	return baseCommand
}

// IsCommandReadOnly checks if a command is read-only (non-destructive).
func IsCommandReadOnly(command string) bool {
	baseCommand := GetBaseCommand(command)
	readOnlyCommands := map[string]bool{
		"cat":    true,
		"ls":     true,
		"head":   true,
		"tail":   true,
		"grep":   true,
		"find":   true,
		"wc":     true,
		"sort":   true,
		"uniq":   true,
		"cut":    true,
		"awk":    true,
		"sed":    true, // sed without -i
		"echo":   true,
		"printf": true,
		"pwd":    true,
		"whoami": true,
		"date":   true,
		"uname":  true,
		"which":  true,
		"type":   true,
		"file":   true,
		"stat":   true,
		"diff":   true,
		"tree":   true,
		"git":    false, // Git can be destructive
	}

	// Check for write operations
	destructivePatterns := []string{
		">", ">>", ">|",
		"rm ", "rmdir ",
		"mv ", "cp ",
		"mkdir ", "touch ",
		"chmod ", "chown ",
		"dd ",
		"truncate ",
		"git push",
		"git reset",
		"git checkout",
	}

	for _, pattern := range destructivePatterns {
		if strings.Contains(command, pattern) {
			return false
		}
	}

	return readOnlyCommands[baseCommand]
}

// ParseCommandForAnalysis parses a command into components for security analysis.
func ParseCommandForAnalysis(command string) []string {
	var components []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i := 0; i < len(command); i++ {
		char := command[i]

		if escaped {
			escaped = false
			current.WriteByte(char)
			continue
		}

		if char == '\\' && !inSingleQuote {
			escaped = true
			current.WriteByte(char)
			continue
		}

		if char == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			current.WriteByte(char)
			continue
		}

		if char == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(char)
			continue
		}

		if char == ' ' && !inSingleQuote && !inDoubleQuote {
			if current.Len() > 0 {
				components = append(components, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteByte(char)
	}

	if current.Len() > 0 {
		components = append(components, current.String())
	}

	return components
}
