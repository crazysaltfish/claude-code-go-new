package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"claude-code-go/internal/commands"
	"claude-code-go/internal/query"
	"claude-code-go/internal/tools"
	"claude-code-go/internal/types"
	"claude-code-go/internal/ui"
)

func TestDefaultModeRequiresApprovalForEveryTool(t *testing.T) {
	workspace := t.TempDir()
	app := &App{
		config:       &Config{Cwd: workspace, PermissionMode: string(types.PermissionModeDefault)},
		toolRegistry: tools.NewToolRegistry(),
		permissionUI: false,
	}

	inside, _ := json.Marshal(map[string]string{"target_file": filepath.Join(workspace, "source.go")})
	decision, err := app.canUseTool(context.Background(), "Read", inside)
	if err != nil || decision.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("inside read decision = %#v, err=%v; want approval requirement", decision, err)
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
		config:       &Config{Cwd: workspace, PermissionMode: string(types.PermissionModeAcceptEdits)},
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

func TestInteractiveApprovalAllowsOnceWithoutChangingMode(t *testing.T) {
	workspace := t.TempDir()
	app := &App{
		config:             &Config{Cwd: workspace, PermissionMode: string(types.PermissionModeDefault)},
		toolRegistry:       tools.NewToolRegistry(),
		permissionUI:       true,
		permissionRequests: make(chan ui.PermissionRequest),
	}
	input, _ := json.Marshal(map[string]string{"target_file": filepath.Join(workspace, "source.go")})
	type result struct {
		decision *types.PermissionDecision
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		decision, err := app.canUseTool(context.Background(), "Read", input)
		resultCh <- result{decision: decision, err: err}
	}()

	request := <-app.permissionRequests
	if request.ToolName != "Read" || !request.ReadOnly {
		t.Fatalf("unexpected permission request: %#v", request)
	}
	request.Response <- ui.PermissionAllowOnce
	got := <-resultCh
	if got.err != nil || got.decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("allow-once decision = %#v, err=%v", got.decision, got.err)
	}
	if mode := app.currentPermissionMode(); mode != types.PermissionModeDefault {
		t.Fatalf("allow once changed mode to %s", mode)
	}
}

func TestInteractiveApprovalCanAllowRemainderOfSession(t *testing.T) {
	workspace := t.TempDir()
	app := &App{
		config:             &Config{Cwd: workspace, PermissionMode: string(types.PermissionModeDefault)},
		toolRegistry:       tools.NewToolRegistry(),
		permissionUI:       true,
		permissionRequests: make(chan ui.PermissionRequest),
	}
	input, _ := json.Marshal(map[string]string{"file_path": filepath.Join(workspace, "source.go"), "contents": "ok"})
	decisionCh := make(chan *types.PermissionDecision, 1)
	go func() {
		decision, _ := app.canUseTool(context.Background(), "Write", input)
		decisionCh <- decision
	}()

	request := <-app.permissionRequests
	request.Response <- ui.PermissionAllowSession
	if decision := <-decisionCh; decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("session approval decision = %#v", decision)
	}
	if mode := app.currentPermissionMode(); mode != types.PermissionModeSession {
		t.Fatalf("session approval mode = %s", mode)
	}

	second, err := app.canUseTool(context.Background(), "Bash", json.RawMessage(`{"command":"echo ok"}`))
	if err != nil || second.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("session mode did not auto-allow next tool: %#v, err=%v", second, err)
	}
}

func TestPermissionCommandChangesOnlyRuntimeMode(t *testing.T) {
	app := &App{config: &Config{PermissionMode: string(types.PermissionModeDefault)}}
	for range app.executePermissionCommand("acceptEdits") {
	}
	if mode := app.currentPermissionMode(); mode != types.PermissionModeAcceptEdits {
		t.Fatalf("permission command mode = %s", mode)
	}
	var event query.SDKMessage
	for value := range app.executePermissionCommand("not-a-mode") {
		event = value.(query.SDKMessage)
	}
	data := event.Message.(map[string]interface{})
	if data["subtype"] != "error" || app.currentPermissionMode() != types.PermissionModeAcceptEdits {
		t.Fatalf("invalid permission command changed mode or omitted error: %#v", data)
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

func TestPlanModeAllowsReadOnlyBashAndRejectsMutatingBash(t *testing.T) {
	app := &App{
		config:       &Config{Cwd: t.TempDir(), PermissionMode: string(types.PermissionModePlan)},
		toolRegistry: tools.NewToolRegistry(),
	}
	read, err := app.canUseTool(context.Background(), "Bash", json.RawMessage(`{"command":"ls -la"}`))
	if err != nil || read.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("plan read-only Bash decision = %#v, err=%v", read, err)
	}
	write, err := app.canUseTool(context.Background(), "Bash", json.RawMessage(`{"command":"touch generated.txt"}`))
	if err != nil || write.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("plan mutating Bash decision = %#v, err=%v", write, err)
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
	for _, mode := range []types.PermissionMode{types.PermissionModeBypassPermissions, types.PermissionModeSession} {
		t.Run(string(mode), func(t *testing.T) {
			app := &App{
				config:       &Config{Cwd: t.TempDir(), PermissionMode: string(mode)},
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
		})
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
	if _, ok := app.registry.Get("permissions"); !ok {
		t.Fatal("permissions command was not registered")
	}
	if _, ok := app.registry.Get("perm"); !ok {
		t.Fatal("permission command alias was not registered")
	}
}
