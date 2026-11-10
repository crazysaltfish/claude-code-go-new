// Package sessions stores resumable conversations independently of generated memory summaries.
package sessions

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"claude-code-go/internal/types"
	"claude-code-go/internal/utils"
)

const Version = 1

// Session keeps the active model context and the complete, uncompacted transcript.
type Session struct {
	Version   int             `json:"version"`
	ID        string          `json:"id"`
	Cwd       string          `json:"cwd"`
	Model     string          `json:"model"`
	Provider  string          `json:"provider"`
	Title     string          `json:"title"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Messages  []types.Message `json:"messages"`
	History   []types.Message `json:"history"`
}

type Store struct{ dir, cwd string }

// NewStore scopes history to the absolute project path, avoiding sanitized-path collisions.
func NewStore(cwd string) (*Store, error) {
	absolute, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	absolute = filepath.Clean(absolute)
	hash := sha256.Sum256([]byte(absolute))
	base := utils.GetClaudeConfigHome()
	if !filepath.IsAbs(base) {
		return nil, fmt.Errorf("session config home must be absolute")
	}
	return &Store{dir: filepath.Join(base, "sessions", fmt.Sprintf("%x", hash[:16])), cwd: absolute}, nil
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

func (s *Store) Path(id string) (string, error) {
	if !validID.MatchString(id) {
		return "", fmt.Errorf("invalid session ID %q", id)
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *Store) New(model, provider string) *Session {
	now := time.Now().UTC()
	return &Session{Version: Version, ID: utils.GenerateUUID(), Cwd: s.cwd, Model: model, Provider: provider, CreatedAt: now, UpdatedAt: now}
}

// Save atomically replaces a private snapshot; a failed write leaves the previous one intact.
func (s *Store) Save(session *Session) error {
	path, err := s.Path(session.ID)
	if err != nil {
		return err
	}
	if session.Cwd != s.cwd {
		return fmt.Errorf("session belongs to another project")
	}
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(s.dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("session directory must be a regular directory")
	}
	if err := os.Chmod(s.dir, 0700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("session must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	session.UpdatedAt = time.Now().UTC()
	if session.Title == "" {
		session.Title = Title(session.History)
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(append(data, '\n')); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (s *Store) Load(id string) (*Session, error) {
	path, err := s.Path(id)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("load session %s: %w", id, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("session must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("decode session %s: %w", id, err)
	}
	if session.Version != Version || session.ID != id || session.Cwd != s.cwd {
		return nil, fmt.Errorf("invalid or unsupported session %s", id)
	}
	for _, message := range append(append([]types.Message(nil), session.Messages...), session.History...) {
		if (message.Role != "user" && message.Role != "assistant") || !json.Valid(message.Content) {
			return nil, fmt.Errorf("invalid message in session %s", id)
		}
	}
	return &session, nil
}

// List returns newest first. Corrupt snapshots are reported instead of silently hidden.
func (s *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []*Session
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		session, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func Title(messages []types.Message) string {
	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		var blocks []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(m.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			title := strings.Join(strings.Fields(b.Text), " ")
			if title == "" {
				continue
			}
			r := []rune(title)
			if len(r) > 80 {
				title = string(r[:80]) + "…"
			}
			return title
		}
	}
	return "Untitled session"
}
