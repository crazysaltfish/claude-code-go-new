package agentapps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDevKitAndLocalAppLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and launches an Agent App")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	project, home := t.TempDir(), t.TempDir()
	manager := New(ctx, project, home, "127.0.0.1:8080", nil)
	t.Cleanup(manager.Close)
	client := &http.Client{Timeout: 10 * time.Second}
	devkit, err := manager.Launch(devkitID, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodGet, devkit, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Sec-Fetch-Site", "same-site")
	request.Header.Set("Sec-Fetch-Dest", "iframe")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if policy := response.Header.Values("Content-Security-Policy"); !strings.Contains(strings.Join(policy, " "), "frame-ancestors http://127.0.0.1:8080") {
		t.Fatalf("incorrect frame policy: %v", policy)
	}
	if body := appResponse(t, response); !strings.Contains(body, "Claude App DevKit") {
		t.Fatalf("missing DevKit frontend: %s", body)
	}

	const id = "example-app"
	scaffold := postApp(t, client, devkit+"api/apps/scaffold", map[string]string{"id": id, "name": "示例 App"})
	dir := filepath.Join(project, id)
	if !strings.Contains(scaffold, "go build") {
		t.Fatalf("missing build command: %s", scaffold)
	}
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	build := exec.CommandContext(ctx, goBinary, "build", "-o", serverFile(), "./app/server-go")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build scaffold: %v\n%s", err, output)
	}
	input := map[string]string{"directory": dir}
	postApp(t, client, devkit+"api/apps/inspect", input)
	postApp(t, client, devkit+"api/apps/install", input)

	anotherProject := New(ctx, t.TempDir(), home, "127.0.0.1:8080", nil)
	t.Cleanup(anotherProject.Close)
	if apps, err := anotherProject.List(); err != nil || len(apps) != 2 || !apps[0].Builtin || !apps[1].Builtin {
		t.Fatalf("project isolation: %v, %v", apps, err)
	}

	appURL, err := manager.Launch(id, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if reused, err := manager.Launch(id, "127.0.0.1"); err != nil || reused != appURL || appURL == devkit {
		t.Fatalf("App origin %q, reused %q, DevKit origin %q, error %v", appURL, reused, devkit, err)
	}
	response, err = client.Get(appURL)
	if err != nil {
		t.Fatal(err)
	}
	if body := appResponse(t, response); !strings.Contains(body, "发送给 Go 服务") {
		t.Fatalf("missing App frontend: %s", body)
	}
	if result := postApp(t, client, appURL+"api/echo", map[string]string{"message": "hello"}); !strings.Contains(result, "hello") {
		t.Fatalf("Go server did not respond to interaction: %s", result)
	}

	postApp(t, client, devkit+"api/apps/stop", map[string]string{"id": id})
	if apps, err := manager.List(); err != nil || len(apps) != 3 || apps[1].Status != "stopped" {
		t.Fatalf("stopped App list: %v, %v", apps, err)
	}
	relaunched, err := manager.Launch(id, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if app, err := manager.Stop(id); err != nil || app.Status != "stopped" || app.Builtin {
		t.Fatalf("stop installed App: %+v, %v", app, err)
	}
	if response, err := client.Get(relaunched); err == nil {
		response.Body.Close()
		t.Fatal("stopped App still accepts connections")
	}
	manager.Close()
	manager = New(ctx, project, home, "127.0.0.1:8080", nil)
	t.Cleanup(manager.Close)
	if _, err = manager.Launch(id, "127.0.0.1"); err != nil {
		t.Fatalf("reload installed app after manager restart: %v", err)
	}
	devkit, err = manager.Launch(devkitID, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	postApp(t, client, devkit+"api/apps/remove", map[string]string{"id": id})
	if _, err = manager.Launch(id, "127.0.0.1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed App launched: %v", err)
	}
}

func TestAppGatewayBlocksSiblingAPIs(t *testing.T) {
	manager := New(context.Background(), ".", ".", "127.0.0.1:8080", nil)
	handler := manager.appGateway("9090", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct {
		site, origin, path, destination string
		status                          int
	}{
		{"same-site", "", "/", "iframe", http.StatusNoContent},
		{"same-origin", "http://127.0.0.1:9090", "/api/apps", "empty", http.StatusNoContent},
		{"same-site", "", "/api/apps", "empty", http.StatusForbidden},
		{"same-site", "http://127.0.0.1:7777", "/", "iframe", http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9090"+test.path, nil)
		request.Header.Set("Sec-Fetch-Site", test.site)
		request.Header.Set("Origin", test.origin)
		request.Header.Set("Sec-Fetch-Dest", test.destination)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != test.status {
			t.Errorf("%s %s: status %d, want %d", test.site, test.path, recorder.Code, test.status)
		}
	}
}

func postApp(t *testing.T, client *http.Client, address string, payload any) string {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Post(address, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return appResponse(t, response)
}

func appResponse(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("%s: status %d, error %v, body %s", response.Request.URL, response.StatusCode, err, body)
	}
	return string(body)
}
