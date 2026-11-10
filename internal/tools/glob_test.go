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

func TestGlobToolCall(t *testing.T) {
	workspace, err := CanonicalizePath(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root.go", "src/direct.go", "src/deep/nested.go", "src/deep/notes.txt"} {
		file := filepath.Join(workspace, name)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	tool := NewGlobTool()
	for _, tc := range []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{name: "implicit recursion", input: `{"pattern":"*.go"}`, want: []string{"root.go", "src/deep/nested.go", "src/direct.go"}},
		{name: "recursive segment", input: `{"pattern":"**/src/**/*.go"}`, want: []string{"src/deep/nested.go", "src/direct.go"}},
		{name: "trailing recursive segment", input: `{"pattern":"**/src/**"}`, want: []string{"src/deep/nested.go", "src/deep/notes.txt", "src/direct.go"}},
		{name: "single segment", input: `{"pattern":"src/*.go"}`, want: []string{"src/direct.go"}},
		{name: "relative target", input: `{"pattern":"*.go","target_directory":"src"}`, want: []string{"src/deep/nested.go", "src/direct.go"}},
		{name: "single file target", input: `{"pattern":"*.go","target_directory":"src/direct.go"}`, want: []string{"src/direct.go"}},
		{name: "no matches", input: `{"pattern":"*.rs"}`},
		{name: "empty pattern", input: `{"pattern":""}`, wantErr: "glob pattern is required"},
		{name: "invalid pattern", input: `{"pattern":"src/z*["}`, wantErr: "invalid glob pattern"},
		{name: "missing directory", input: `{"pattern":"*.go","target_directory":"missing"}`, wantErr: "glob failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, err := tool.NormalizeInput(json.RawMessage(tc.input), workspace)
			if err != nil {
				t.Fatal(err)
			}
			result, err := tool.Call(context.Background(), args, &types.ToolContext{Cwd: workspace}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != "" {
				if result.Error == nil || !strings.Contains(result.Error.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", result.Error, tc.wantErr)
				}
				return
			}
			if result.Error != nil {
				t.Fatal(result.Error)
			}
			want := "No files found"
			if len(tc.want) > 0 {
				paths := make([]string, 0, len(tc.want))
				for _, name := range tc.want {
					paths = append(paths, filepath.Join(workspace, name))
				}
				want = strings.Join(paths, "\n")
			}
			if result.Output != want {
				t.Fatalf("output = %q, want %q", result.Output, want)
			}
		})
	}
}
