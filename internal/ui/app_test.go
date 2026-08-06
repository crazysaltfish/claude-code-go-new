package ui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatPermissionRequestShowsCompleteInput(t *testing.T) {
	longTail := strings.Repeat("x", 256) + "END-OF-COMMAND"
	input, err := json.Marshal(map[string]interface{}{
		"command": "printf " + longTail,
		"paths":   []string{"/tmp/first", "/tmp/second"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := formatPermissionRequest(PermissionRequest{
		ToolName:    "Bash",
		Input:       input,
		ReadOnly:    false,
		Destructive: true,
		RiskReason:  "Command contains command substitution",
	})

	for _, want := range []string{
		"Tool: Bash",
		"Impact: potentially destructive",
		"Read-only: false",
		"Destructive: true",
		`"command": "printf `,
		`"paths": [`,
		"END-OF-COMMAND",
		"Security note: Command contains command substitution",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted approval does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "truncated") || strings.Contains(got, "...") {
		t.Fatalf("formatted approval unexpectedly truncates input:\n%s", got)
	}
}

func TestFormatPermissionRequestPreservesInvalidJSON(t *testing.T) {
	const raw = `{"command": "unterminated"`
	got := formatPermissionRequest(PermissionRequest{
		ToolName: "Bash",
		Input:    json.RawMessage(raw),
	})
	if !strings.Contains(got, raw) {
		t.Fatalf("raw input missing from formatted approval:\n%s", got)
	}
}
