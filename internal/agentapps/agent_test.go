package agentapps

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"claude-code-go/pkg/appagent"
)

type bridgeResearch struct{}

func (bridgeResearch) Complete(_ context.Context, input appagent.Completion) (string, error) {
	if input.MaxTokens < 400 {
		return `{"query":"光刻机","english":"lithography machine"}`, nil
	}
	return `{"title":"桥接验证图解","summary":"已通过宿主生成","kind":"flow",
		"concepts":[{"label":"光源","detail":"产生光","question":"光如何产生？"},
		{"label":"掩模","detail":"编码图形","question":"图形如何编码？"},
		{"label":"晶圆","detail":"接收图形","question":"晶圆如何曝光？","refs":[1]}],
		"edges":[{"from":1,"to":2,"label":"照亮"},{"from":2,"to":3,"label":"投影"}]}`, nil
}

func (bridgeResearch) Search(_ context.Context, _ appagent.Search) (appagent.Results, error) {
	return appagent.Results{Method: "brave", Sources: []appagent.Source{
		{Title: "Host search result", URL: "https://example.org/source", Excerpt: "Lithography transfers a pattern to the wafer."},
	}}, nil
}

func TestInstalledFastLearningUsesPrivateAgentBridge(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and starts the standard Fast Learning package")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	manager := New(ctx, t.TempDir(), t.TempDir(), "127.0.0.1:8080", bridgeResearch{})
	t.Cleanup(manager.Close)

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(serverFile())), 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"),
		"build", "-o", filepath.Join(dir, serverFile()), "./apps/fast-learning/app/server-go")
	command.Dir = "../.."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Fast Learning: %v\n%s", err, output)
	}
	manifest := fastLearningManifest
	manifest.ID = "fast-learning-bridge-test"
	file, err := os.Open(filepath.Join(dir, serverFile()))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := manager.install(manifest, file); err != nil {
		t.Fatal(err)
	}
	address, err := manager.Launch(manifest.ID, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	output := postApp(t, &http.Client{Timeout: 10 * time.Second}, address+"api/learn", map[string]string{"question": "光刻机？"})
	if !strings.Contains(output, "桥接验证图解") || !strings.Contains(output, "Host search result") {
		t.Fatalf("expected standard App to receive host completion and search results: %s", output)
	}
}
