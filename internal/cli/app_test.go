package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"claude-code-go/internal/commands"
	"claude-code-go/internal/tools"
	"claude-code-go/internal/types"
)

func TestReadOutsideWorkspaceRequiresApproval(t *testing.T) {
	workspace := t.TempDir()
	app := &App{
		config:       &Config{Cwd: workspace, PermissionMode: string(types.PermissionModeDefault)},
		toolRegistry: tools.NewToolRegistry(),
		permissionUI: false,
	}

	inside, _ := json.Marshal(map[string]string{"target_file": filepath.Join(workspace, "source.go")})
	decision, err := app.canUseTool(context.Background(), "Read", inside)
	if err != nil || decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("inside read decision = %#v, err=%v", decision, err)
	}

	outside, _ := json.Marshal(map[string]string{"target_file": filepath.Join(t.TempDir(), "credentials")})
	decision, err = app.canUseTool(context.Background(), "Read", outside)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("outside read decision = %#v, want deny without approval UI", decision)
	}
}

func TestAcceptEditsDoesNotAutoApproveOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	app := &App{
		config:       &Config{Cwd: workspace, PermissionMode: string(types.PermissionModeAcceptEdits)},
		toolRegistry: tools.NewToolRegistry(),
		permissionUI: false,
	}

	inside, _ := json.Marshal(map[string]string{"file_path": filepath.Join(workspace, "source.go"), "contents": "ok"})
	decision, _ := app.canUseTool(context.Background(), "Write", inside)
	if decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("inside edit decision = %#v", decision)
	}

	outside, _ := json.Marshal(map[string]string{"file_path": filepath.Join(t.TempDir(), "source.go"), "contents": "no"})
	decision, _ = app.canUseTool(context.Background(), "Write", outside)
	if decision.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("outside edit decision = %#v, want deny without approval UI", decision)
	}
}

func TestMemoryFileToolsAreAllowedOnlyInsideMemoryDirectory(t *testing.T) {
	workspace := t.TempDir()
	memoryDir := t.TempDir()
	app := &App{
		config:       &Config{Cwd: workspace, PermissionMode: string(types.PermissionModeDefault)},
		toolRegistry: tools.NewToolRegistry(),
		permissionUI: false,
		memoryDir:    memoryDir,
	}

	inside, _ := json.Marshal(map[string]string{
		"file_path": filepath.Join(memoryDir, "preference.md"),
		"contents":  "remember this",
	})
	decision, err := app.canUseTool(context.Background(), "Write", inside)
	if err != nil || decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("memory write decision = %#v, err=%v", decision, err)
	}

	outside, _ := json.Marshal(map[string]string{
		"file_path": filepath.Join(t.TempDir(), "preference.md"),
		"contents":  "do not write",
	})
	decision, err = app.canUseTool(context.Background(), "Write", outside)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("outside write decision = %#v, want deny", decision)
	}
}

func TestPlanModeDoesNotAllowMemoryWrites(t *testing.T) {
	memoryDir := t.TempDir()
	app := &App{
		config:       &Config{Cwd: t.TempDir(), PermissionMode: string(types.PermissionModePlan)},
		toolRegistry: tools.NewToolRegistry(),
		permissionUI: false,
		memoryDir:    memoryDir,
	}
	input, _ := json.Marshal(map[string]string{
		"file_path": filepath.Join(memoryDir, "preference.md"),
		"contents":  "do not persist from plan mode",
	})
	decision, err := app.canUseTool(context.Background(), "Write", input)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("plan-mode memory write decision = %#v, want deny", decision)
	}
}

func TestWebFetchAlwaysRequiresApproval(t *testing.T) {
	app := &App{
		config:       &Config{Cwd: t.TempDir(), PermissionMode: string(types.PermissionModeDefault)},
		toolRegistry: tools.NewToolRegistry(),
		permissionUI: false,
	}
	input := json.RawMessage(`{"urls":["https://example.com"]}`)
	decision, err := app.canUseTool(context.Background(), "WebFetch", input)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("WebFetch decision = %#v, want approval requirement", decision)
	}
}

func TestBypassPermissionsCannotOverrideHardBashDenial(t *testing.T) {
	app := &App{
		config:       &Config{Cwd: t.TempDir(), PermissionMode: string(types.PermissionModeBypassPermissions)},
		toolRegistry: tools.NewToolRegistry(),
	}
	input := json.RawMessage(`{"command":"jq 'system(\"id\")'"}`)
	decision, err := app.canUseTool(context.Background(), "Bash", input)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("hard-denied Bash decision = %#v", decision)
	}
}

func TestRegisterCommandsIncludesInteractiveCommands(t *testing.T) {
	app := &App{registry: commands.NewRegistry()}
	app.registerCommands()
	if _, ok := app.registry.Get("compact"); !ok {
		t.Fatal("compact command was not registered")
	}
	if _, ok := app.registry.Get("diff"); !ok {
		t.Fatal("diff command was not registered")
	}
}
