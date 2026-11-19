package fastlearning

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"claude-code-go/pkg/appagent"
)

type Concept struct {
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Question string `json:"question"`
	Refs     []int  `json:"refs,omitempty"`
}

type Edge struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Label string `json:"label"`
}

type Diagram struct {
	Title    string    `json:"title"`
	Summary  string    `json:"summary"`
	Kind     string    `json:"kind"`
	Concepts []Concept `json:"concepts"`
	Edges    []Edge    `json:"edges"`
}

type Node struct {
	ID       string            `json:"id"`
	ParentID string            `json:"parent_id,omitempty"`
	Anchor   int               `json:"anchor,omitempty"`
	Question string            `json:"question"`
	Method   string            `json:"method"`
	Sources  []appagent.Source `json:"sources"`
	Diagram
}

type Canvas struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Updated time.Time `json:"updated"`
	Nodes   []Node    `json:"nodes"`
}

type canvasSummary struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Updated time.Time `json:"updated"`
	Count   int       `json:"count"`
}

var validID = regexp.MustCompile(`^[a-f0-9]{24}$`)

type store struct {
	dir string
	mu  sync.Mutex
}

func newID() string {
	var value [12]byte
	_, _ = rand.Read(value[:])
	return hex.EncodeToString(value[:])
}

func (s *store) open(id string) (Canvas, error) {
	if !validID.MatchString(id) {
		return Canvas{}, os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil {
		return Canvas{}, err
	}
	var canvas Canvas
	err = json.Unmarshal(data, &canvas)
	return canvas, err
}

func (s *store) list() ([]canvasSummary, error) {
	items := []canvasSummary{}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return items, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".json")]
		if !validID.MatchString(id) {
			continue
		}
		canvas, err := s.open(id)
		if err != nil {
			return nil, err
		}
		items = append(items, canvasSummary{ID: canvas.ID, Title: canvas.Title, Updated: canvas.Updated, Count: len(canvas.Nodes)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Updated.After(items[j].Updated) })
	return items, nil
}

func (s *store) create(nodes ...Node) (Canvas, error) {
	canvas := Canvas{ID: newID(), Title: nodes[0].Title, Nodes: nodes, Updated: time.Now().UTC()}
	err := s.save(canvas)
	return canvas, err
}

func (s *store) append(id string, node Node) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	canvas, err := s.open(id)
	if err != nil {
		return canvas, err
	}
	canvas.Nodes = append(canvas.Nodes, node)
	canvas.Updated = time.Now().UTC()
	return canvas, s.save(canvas)
}

func (s *store) save(canvas Canvas) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.dir, ".canvas-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = errors.Join(json.NewEncoder(file).Encode(canvas), file.Close()); err != nil {
		return fmt.Errorf("保存画布: %w", err)
	}
	return os.Rename(file.Name(), filepath.Join(s.dir, canvas.ID+".json"))
}
