package fastlearning

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"claude-code-go/pkg/appagent"
)

type scriptedResearch struct {
	plans      []string
	searches   []appagent.Search
	searchFail bool
}

func (a *scriptedResearch) Complete(_ context.Context, input appagent.Completion) (string, error) {
	if input.System == planPrompt {
		a.plans = append(a.plans, input.Prompt)
		return `{"query":"晶圆台 对准","english":"wafer stage alignment"}`, nil
	}
	return `{"title":"晶圆定位","summary":"先测量，再移动和对准。","kind":"flow",
		"concepts":[{"label":"晶圆台","detail":"移动工件","question":"谁来测量？","refs":[1,5,1]},
		{"label":"测量","detail":"获取位置","question":"如何检测偏差？","refs":[1]},
		{"label":"控制","detail":"根据测量补偿","question":"何时校准？","refs":[]}],
		"edges":[{"from":1,"to":2,"label":"送至"},{"from":2,"to":3,"label":"反馈"}]}`, nil
}

func (a *scriptedResearch) Search(_ context.Context, input appagent.Search) (appagent.Results, error) {
	a.searches = append(a.searches, input)
	if a.searchFail {
		return appagent.Results{}, errors.New("offline")
	}
	return appagent.Results{Method: "wikipedia", Sources: []appagent.Source{
		{Title: "检索资料", URL: "https://example.com/stage", Excerpt: "测量用于定位晶圆。"},
		{Title: "无效来源", URL: "javascript:alert(1)", Excerpt: "无效内容。"},
	}}, nil
}

func TestLearningPersistsGroundedBranches(t *testing.T) {
	dir := t.TempDir()
	agent := &scriptedResearch{}
	app := New(dir, agent)
	first := learnRequest(t, app, question{Text: "光刻机有什么结构？"})
	root := first.Nodes[0]
	if len(agent.searches) != 1 || agent.searches[0].English != "wafer stage alignment" {
		t.Fatalf("expected planned retrieval, got %v", agent.searches)
	}
	if root.Method != "wikipedia" || len(root.Sources) != 1 || len(root.Concepts[0].Refs) != 1 {
		t.Fatalf("expected retrieved-only citations: %#v", root)
	}

	agent.searchFail = true
	result := learnRequest(t, app, question{Text: "它如何测量？", CanvasID: first.ID, ParentID: root.ID, Anchor: 1})
	if result.ID != first.ID || len(result.Nodes) != 2 {
		t.Fatalf("follow-up wrote a different canvas: %#v", result)
	}
	branch := result.Nodes[1]
	if branch.ParentID != root.ID || branch.Anchor != 1 || branch.Method != "none" || len(branch.Concepts[0].Refs) != 0 {
		t.Fatalf("invalid branch lineage or invented sources: %#v", branch)
	}
	if len(agent.plans) != 2 || !strings.Contains(agent.plans[1], "晶圆台") || !strings.Contains(agent.plans[1], root.Question) {
		t.Fatalf("Agent did not receive branch context: %v", agent.plans)
	}

	restarted := New(dir, nil)
	recorder := httptest.NewRecorder()
	restarted.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/canvases/"+first.ID, nil))
	var restored Canvas
	if err := json.Unmarshal(recorder.Body.Bytes(), &restored); err != nil || len(restored.Nodes) != 2 {
		t.Fatalf("did not restore canvas: %v, %s", err, recorder.Body.String())
	}
}

func learnRequest(t *testing.T, app http.Handler, input question) Canvas {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/api/learn", strings.NewReader(string(data)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, request)
	updates := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
	if recorder.Code != http.StatusOK || len(updates) != 4 {
		t.Fatalf("expected three stages and final result: %d %s", recorder.Code, recorder.Body.String())
	}
	var update event
	if err := json.Unmarshal([]byte(updates[3]), &update); err != nil || update.Canvas == nil {
		t.Fatalf("expected a canvas: %v %s", err, updates[3])
	}
	return *update.Canvas
}
