package utils

import "testing"

func TestBashCommandIsSafeFailsClosedWithoutPanicking(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		behavior string
	}{
		{name: "ordinary", command: "go test ./...", behavior: "allow"},
		{name: "unicode obfuscation", command: "git\u00a0status", behavior: "ask"},
		{name: "hard denial", command: `jq 'system("id")'`, behavior: "deny"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := BashCommandIsSafe(test.command)
			if result.Behavior != test.behavior {
				t.Fatalf("behavior = %q (%s), want %q", result.Behavior, result.Message, test.behavior)
			}
		})
	}
}
