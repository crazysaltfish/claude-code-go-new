package cli

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"claude-code-go/internal/utils"
)

func TestWebWorkspaceFiles(t *testing.T) {
	server, chat, handler := webTestServer(t, &webScriptClient{})
	write := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("file contents"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	project := server.root.config.Cwd
	write(filepath.Join(project, "src", "main.go"))
	write(filepath.Join(utils.GetClaudeConfigHome(), "skills", "example.md"))
	agents, err := server.root.sessionStore.AgentDir(chat.id)
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(agents, "agent.json"))
	if err := os.Symlink(utils.GetClaudeConfigHome(), filepath.Join(project, "external")); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		scope, path, chatID string
		status              int
		names               []string
	}{
		{"public", "", "", http.StatusOK, []string{"src", "external"}},
		{"public", "src", "", http.StatusOK, []string{"main.go"}},
		{"personal", "skills", "", http.StatusOK, []string{"example.md"}},
		{"session", "", chat.id, http.StatusOK, []string{"agents", chat.id + ".json"}},
		{"session", "agents", chat.id, http.StatusOK, []string{"agent.json"}},
		{"session", "agents", "unknown-session", http.StatusNotFound, nil},
		{"session", "../agents", chat.id, http.StatusBadRequest, nil},
		{"public", "/tmp", "", http.StatusBadRequest, nil},
		{"public", "external/skills", "", http.StatusNotFound, nil},
	} {
		query := url.Values{"path": {test.path}, "chat": {test.chatID}}
		response := webRequest(handler, http.MethodGet, "/api/workspace/"+test.scope+"?"+query.Encode(), "")
		if response.Code != test.status {
			t.Fatalf("%s/%s: got %d, want %d: %s", test.scope, test.path, response.Code, test.status, response.Body)
		}
		if test.status != http.StatusOK {
			continue
		}
		var listing workspaceListing
		err := json.Unmarshal(response.Body.Bytes(), &listing)
		if err != nil || len(listing.Entries) != len(test.names) {
			t.Fatalf("%s/%s: %s (%v)", test.scope, test.path, response.Body, err)
		}
		for i, name := range test.names {
			if listing.Entries[i].Name != name {
				t.Fatalf("%s/%s: entry %d = %q, want %q", test.scope, test.path, i, listing.Entries[i].Name, name)
			}
		}
	}
}
