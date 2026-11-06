package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claude-code-go/internal/types"
)

func TestStoreRoundTripAndProjectIsolation(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	store, err := NewStore(project)
	if err != nil {
		t.Fatal(err)
	}
	first := store.New("model", "openai")
	second := store.New("model", "openai")
	if first.ID == second.ID {
		t.Fatal("new sessions reuse an ID")
	}
	first.Messages = []types.Message{{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"继续实现会话保存"}]`)}}
	first.History = first.Messages
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	path, _ := store.Path(first.ID)
	info, _ := os.Stat(path)
	dir, _ := os.Stat(filepath.Dir(path))
	if info.Mode().Perm() != 0600 || dir.Mode().Perm() != 0700 {
		t.Fatalf("permissions: %o %o", info.Mode().Perm(), dir.Mode().Perm())
	}
	// A new store instance models a fresh process.
	reopened, _ := NewStore(project)
	loaded, err := reopened.Load(first.ID)
	if err != nil || loaded.Title != "继续实现会话保存" || len(loaded.Messages) != 1 {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	if err := store.Save(second); err != nil {
		t.Fatal(err)
	}
	list, err := store.List()
	if err != nil || len(list) != 2 || list[0].ID != second.ID {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	other, _ := NewStore(t.TempDir())
	if _, err := other.Load(first.ID); err == nil {
		t.Fatal("another project loaded the session")
	}
	if list, err := other.List(); err != nil || len(list) != 0 {
		t.Fatalf("other list=%+v err=%v", list, err)
	}
}

func TestStoreRejectsInvalidAndCorruptFiles(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_HOME", t.TempDir())
	store, _ := NewStore(t.TempDir())
	for _, id := range []string{"../escape", "/tmp/file", "", strings.Repeat("x", 129)} {
		if _, err := store.Load(id); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	session := store.New("m", "openai")
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}
	path, _ := store.Path(session.ID)
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(session.ID); err == nil {
		t.Fatal("loaded corrupt snapshot")
	}
	if _, err := store.List(); err == nil {
		t.Fatal("hid corrupt snapshot")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, []byte("untouched"), 0600)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(session.ID); err == nil {
		t.Fatal("loaded symlink")
	}
	if err := store.Save(session); err == nil {
		t.Fatal("saved through symlink")
	}
}
