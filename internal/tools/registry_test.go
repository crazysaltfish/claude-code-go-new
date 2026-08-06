package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"claude-code-go/internal/types"
)

type registryFixtureTool struct{ *BaseTool }

func newRegistryFixtureTool(name, description string) *registryFixtureTool {
	return &registryFixtureTool{BaseTool: NewBaseTool(name, description)}
}

func TestRegistrySimpleModeCapabilityMatrix(t *testing.T) {
	registry := NewToolRegistryWithOptions(RegistryOptions{SimpleMode: true})
	var enabled []string
	capabilities := make(map[string]ToolCapability)
	for _, capability := range registry.Capabilities() {
		capabilities[capability.Name] = capability
		if capability.Enabled {
			enabled = append(enabled, capability.Name)
		}
	}
	want := []string{"Bash", "Read", "Edit"}
	if !reflect.DeepEqual(enabled, want) {
		t.Fatalf("simple mode tools = %v, want %v", enabled, want)
	}
	if capability := capabilities["Write"]; capability.Enabled || capability.Gate != "simple_mode" {
		t.Fatalf("Write capability = %#v", capability)
	}
	if capability := capabilities["WebSearch"]; capability.Enabled || capability.Gate != "implementation" {
		t.Fatalf("WebSearch capability = %#v", capability)
	}
}

func TestRegistryDisabledToolsAndEnvironmentPolicy(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SIMPLE", "false")
	t.Setenv("CLAUDE_CODE_DISABLE_TOOLS", "Bash, Grep")
	registry := NewToolRegistryWithOptions(RegistryOptionsFromEnv())

	for _, name := range []string{"Bash", "Grep"} {
		for _, tool := range registry.ListEnabled() {
			if tool.Name() == name {
				t.Fatalf("disabled tool %s remained enabled", name)
			}
		}
		capability := findCapability(registry.Capabilities(), name)
		if capability.Gate != "disabled_tools" {
			t.Fatalf("%s gate = %q", name, capability.Gate)
		}
	}
}

func findCapability(capabilities []ToolCapability, name string) ToolCapability {
	for _, capability := range capabilities {
		if capability.Name == name {
			return capability
		}
	}
	return ToolCapability{}
}

func (t *registryFixtureTool) Call(
	context.Context,
	json.RawMessage,
	*types.ToolContext,
	types.CanUseToolFunc,
	*types.Message,
	func(interface{}),
) (*types.ToolResult, error) {
	return &types.ToolResult{Output: t.Name()}, nil
}

func TestRegistryPreservesRegistrationOrderWithoutDuplicateReplacement(t *testing.T) {
	registry := &Registry{tools: make(map[string]types.Tool)}
	registry.Register(newRegistryFixtureTool("First", "first"))
	registry.Register(newRegistryFixtureTool("Second", "second"))
	registry.Register(newRegistryFixtureTool("First", "replacement"))

	got := registry.List()
	if len(got) != 2 {
		t.Fatalf("registry contains %d tools, want 2", len(got))
	}
	if got[0].Name() != "First" || got[1].Name() != "Second" {
		t.Fatalf("registry order = [%s, %s], want [First, Second]", got[0].Name(), got[1].Name())
	}
	description, err := got[0].Description(context.Background(), nil, types.ToolOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if description != "replacement" {
		t.Fatalf("replacement was not retained: %q", description)
	}
}

func TestDefaultRegistryExposesStableCorePrefix(t *testing.T) {
	registry := NewToolRegistry()
	want := []string{"Bash", "Read", "Write", "Edit", "Glob", "Grep"}

	for run := 0; run < 3; run++ {
		got := registry.ListEnabled()
		if len(got) < len(want) {
			t.Fatalf("enabled tool count = %d, want at least %d", len(got), len(want))
		}
		for i, name := range want {
			if got[i].Name() != name {
				t.Fatalf("run %d tool[%d] = %q, want %q", run, i, got[i].Name(), name)
			}
		}
	}
}
