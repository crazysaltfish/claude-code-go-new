package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claude-code-go/internal/types"
)

func TestFileWriteReturnsDisplayDiffForOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]interface{}{
		"file_path": path,
		"contents":  "after\n",
	})

	result, err := NewFileWriteTool().Call(
		context.Background(), input, &types.ToolContext{ToolUseId: "write_1"}, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != nil || result.Display == nil {
		t.Fatalf("write result missing display metadata: %#v", result)
	}
	for _, want := range []string{"--- " + path, "+++ " + path, "-before", "+after"} {
		if !strings.Contains(result.Display.Diff, want) {
			t.Fatalf("write diff missing %q:\n%s", want, result.Display.Diff)
		}
	}
	if strings.Contains(result.Display.Diff, "--- /dev/null") {
		t.Fatalf("overwrite incorrectly rendered as new file:\n%s", result.Display.Diff)
	}
	if len(result.Display.Artifacts) != 1 || result.Display.Artifacts[0] != path {
		t.Fatalf("unexpected artifacts: %#v", result.Display.Artifacts)
	}
}

func TestFileWriteReturnsNewFileArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.txt")
	input, _ := json.Marshal(map[string]interface{}{
		"file_path": path,
		"contents":  "created\n",
	})
	result, err := NewFileWriteTool().Call(
		context.Background(), input, &types.ToolContext{ToolUseId: "write_2"}, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Display == nil || !strings.Contains(result.Display.Diff, "--- /dev/null") ||
		!strings.Contains(result.Display.Diff, "+created") {
		t.Fatalf("new file display diff is incomplete: %#v", result.Display)
	}
}
