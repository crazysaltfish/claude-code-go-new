package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"claude-code-go/internal/types"
)

type bm25FixtureTool struct {
	*BaseTool
}

func newBM25FixtureTool(name, description string, fields map[string]string) *bm25FixtureTool {
	properties := make(map[string]map[string]interface{}, len(fields))
	for field, description := range fields {
		properties[field] = map[string]interface{}{
			"type":        "string",
			"description": description,
		}
	}
	return &bm25FixtureTool{BaseTool: &BaseTool{
		name:        name,
		description: description,
		inputSchema: types.ToolInputJSONSchema{
			Type:       "object",
			Properties: properties,
		},
		isEnabled:  true,
		isReadOnly: true,
	}}
}

func (t *bm25FixtureTool) Call(context.Context, json.RawMessage, *types.ToolContext, types.CanUseToolFunc, *types.Message, func(interface{})) (*types.ToolResult, error) {
	return nil, fmt.Errorf("fixture tool cannot be called")
}

func TestToolSearchBM25RanksRelevantToolFirst(t *testing.T) {
	registry := &Registry{tools: make(map[string]types.Tool)}
	registry.Register(newBM25FixtureTool(
		"FileRead",
		"Read contents from a local file",
		map[string]string{"file_path": "Path of the file to read"},
	))
	registry.Register(newBM25FixtureTool(
		"TextSearch",
		"Search text inside files using regular expressions",
		map[string]string{"pattern": "Regular expression to locate"},
	))
	registry.Register(newBM25FixtureTool(
		"LogViewer",
		"Display application logs and diagnostics",
		map[string]string{"service": "Service whose logs should be shown"},
	))

	search := NewToolSearchTool(registry)
	registry.Register(search)

	output := callToolSearch(t, search, `{"query":"reading files"}`)
	fileReadIndex := strings.Index(output, "- FileRead:")
	textSearchIndex := strings.Index(output, "- TextSearch:")
	if fileReadIndex < 0 {
		t.Fatalf("FileRead was not returned:\n%s", output)
	}
	if textSearchIndex >= 0 && fileReadIndex > textSearchIndex {
		t.Fatalf("name and description matches should rank FileRead first:\n%s", output)
	}
}

func TestToolSearchBM25UsesInputFieldMetadata(t *testing.T) {
	registry := &Registry{tools: make(map[string]types.Tool)}
	registry.Register(newBM25FixtureTool(
		"Locator",
		"Locate structured content",
		map[string]string{"regular_expression": "Pattern used to match file contents"},
	))
	registry.Register(newBM25FixtureTool(
		"Browser",
		"Open and inspect web pages",
		map[string]string{"url": "Page address"},
	))

	search := NewToolSearchTool(registry)
	output := callToolSearch(t, search, `{"query":"regular expressions","max_results":1}`)
	if !strings.Contains(output, "- Locator:") {
		t.Fatalf("schema metadata did not make Locator discoverable:\n%s", output)
	}
	if strings.Contains(output, "- Browser:") {
		t.Fatalf("max_results was not applied:\n%s", output)
	}
}

func TestToolSearchBM25NoLexicalMatch(t *testing.T) {
	registry := &Registry{tools: make(map[string]types.Tool)}
	registry.Register(newBM25FixtureTool("FileRead", "Read a local file", nil))
	search := NewToolSearchTool(registry)

	output := callToolSearch(t, search, `{"query":"astronomy telescope"}`)
	if !strings.Contains(output, "No tools found") {
		t.Fatalf("unexpected result for an unmatched query:\n%s", output)
	}
}

func TestTokenizeForBM25SplitsCamelCaseAndNormalizesPlurals(t *testing.T) {
	got := tokenizeForBM25("FileRead reads matching files")
	want := []string{"file", "read", "read", "match", "file"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected tokens: got=%v want=%v", got, want)
	}
}

func callToolSearch(t *testing.T, search *ToolSearchTool, input string) string {
	t.Helper()
	result, err := search.Call(
		context.Background(),
		json.RawMessage(input),
		&types.ToolContext{ToolUseId: "search-test"},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	output, ok := result.Output.(string)
	if !ok {
		t.Fatalf("unexpected output type %T", result.Output)
	}
	return output
}
