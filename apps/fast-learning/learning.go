package fastlearning

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"claude-code-go/apps/fast-learning/app/frontend"
	"claude-code-go/pkg/appagent"
)

//go:embed app.json
var ManifestJSON string

type service struct {
	agent appagent.Agent
	store store
	slots chan struct{}
}

func New(dataDir string, agent appagent.Agent) http.Handler {
	s := &service{agent: agent, store: store{dir: filepath.Join(dataDir, "canvases")}, slots: make(chan struct{}, 2)}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(frontend.Files))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /api/canvases", func(w http.ResponseWriter, _ *http.Request) {
		canvases, err := s.store.list()
		jsonResponse(w, canvases, err)
	})
	mux.HandleFunc("GET /api/canvases/{id}", func(w http.ResponseWriter, r *http.Request) {
		canvas, err := s.store.open(r.PathValue("id"))
		jsonResponse(w, canvas, err)
	})
	mux.HandleFunc("POST /api/example", func(w http.ResponseWriter, r *http.Request) {
		if !readJSON(w, r, &struct{}{}) {
			return
		}
		canvas, err := s.example()
		jsonResponse(w, canvas, err)
	})
	mux.HandleFunc("POST /api/learn", s.learn)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}

type question struct {
	CanvasID string `json:"canvas_id"`
	ParentID string `json:"parent_id"`
	Anchor   int    `json:"anchor"`
	Text     string `json:"question"`
}

type studyContext struct {
	Question string `json:"question"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
}

type event struct {
	Stage  string  `json:"stage,omitempty"`
	Canvas *Canvas `json:"canvas,omitempty"`
	Error  string  `json:"error,omitempty"`
}

func (s *service) learn(w http.ResponseWriter, r *http.Request) {
	var input question
	if !readJSON(w, r, &input) {
		return
	}
	input.Text = strings.TrimSpace(input.Text)
	if input.Text == "" || utf8.RuneCountInString(input.Text) > 400 {
		fail(w, http.StatusBadRequest, "请输入 1 至 400 字的问题")
		return
	}
	if s.agent == nil {
		fail(w, http.StatusServiceUnavailable, "当前项目没有可用的 Agent")
		return
	}
	branch, focus, err := s.context(input)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		fail(w, http.StatusTooManyRequests, "还有问题正在处理，请稍后重试")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	encoder := json.NewEncoder(w)
	controller := http.NewResponseController(w)
	emit := func(update event) {
		_ = encoder.Encode(update)
		_ = controller.Flush()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	emit(event{Stage: "planning"})
	node, err := s.generate(ctx, input, branch, focus, emit)
	if err == nil {
		err = ctx.Err()
	}
	var canvas Canvas
	if err == nil {
		if input.CanvasID == "" {
			canvas, err = s.store.create(node)
		} else {
			canvas, err = s.store.append(input.CanvasID, node)
		}
	}
	if err != nil {
		emit(event{Error: err.Error()})
		return
	}
	emit(event{Canvas: &canvas})
}

func (s *service) context(input question) ([]studyContext, string, error) {
	if input.CanvasID == "" {
		if input.ParentID != "" || input.Anchor != 0 {
			return nil, "", errors.New("根问题不能指定上级节点")
		}
		return nil, "", nil
	}
	canvas, err := s.store.open(input.CanvasID)
	if err != nil {
		return nil, "", errors.New("找不到当前画布")
	}
	nodes := make(map[string]Node, len(canvas.Nodes))
	for _, node := range canvas.Nodes {
		nodes[node.ID] = node
	}
	parent, ok := nodes[input.ParentID]
	if !ok || input.Anchor < 0 || input.Anchor > len(parent.Concepts) {
		return nil, "", errors.New("追问的知识点已失效")
	}
	var focus string
	if input.Anchor != 0 {
		concept := parent.Concepts[input.Anchor-1]
		focus = concept.Label + " — " + concept.Detail
	}
	var path []studyContext
	for current := parent; current.ID != "" && len(path) < 5; current = nodes[current.ParentID] {
		path = append(path, studyContext{Question: current.Question, Title: current.Title, Summary: current.Summary})
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path, focus, nil
}

func readJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "application/json" {
		fail(w, http.StatusUnsupportedMediaType, "请求必须使用 JSON")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil || decoder.Decode(new(any)) != io.EOF {
		fail(w, http.StatusBadRequest, "请求格式无效")
		return false
	}
	return true
}

func jsonResponse(w http.ResponseWriter, value any, err error) {
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, http.StatusNotFound, "找不到画布")
			return
		}
		fail(w, http.StatusInternalServerError, fmt.Sprintf("无法读取或保存学习画布：%v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(event{Error: message})
}
