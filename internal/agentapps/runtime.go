package agentapps

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	fastlearning "claude-code-go/apps/fast-learning"
	"claude-code-go/pkg/appagent"
)

type Manager struct {
	ctx      context.Context
	project  string
	registry string
	host     string
	webPort  string
	agent    appagent.Agent
	mu       sync.Mutex
	running  map[string]*instance
	closed   bool
}

type instance struct {
	server *http.Server
	port   string
	stop   context.CancelFunc
	done   <-chan struct{}
}

var errAppNotInstalled = fmt.Errorf("App is not installed: %w", fs.ErrNotExist)

var fastLearningManifest = func() Manifest {
	manifest, err := readManifest(strings.NewReader(fastlearning.ManifestJSON))
	if err != nil {
		panic(err)
	}
	return manifest
}()

func New(ctx context.Context, project, dataHome, webAddress string, agent appagent.Agent) *Manager {
	host, port, _ := net.SplitHostPort(webAddress)
	return &Manager{
		ctx: ctx, project: project, registry: registryPath(project, dataHome),
		host: host, webPort: port, agent: agent, running: make(map[string]*instance),
	}
}

func (m *Manager) List() ([]App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	apps := []App{m.app(builtinManifest, true)}
	entries, err := os.ReadDir(m.registry)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	hasFastLearning := false
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		manifest, err := installedManifest(filepath.Join(m.registry, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read installed App %s: %w", entry.Name(), err)
		}
		apps = append(apps, m.app(manifest, false))
		hasFastLearning = hasFastLearning || manifest.ID == fastLearningID
	}
	if !hasFastLearning {
		apps = append(apps, m.app(fastLearningManifest, true))
	}
	return apps, nil
}

func installedManifest(path string) (Manifest, error) {
	pack, err := zip.OpenReader(path)
	if err != nil {
		return Manifest{}, err
	}
	defer pack.Close()
	manifest, err := readInstalledManifest(pack)
	if err == nil && (manifest.ID == devkitID || filepath.Base(path) != manifest.ID+".zip") {
		err = errors.New("installed package does not match its App ID")
	}
	return manifest, err
}

func (m *Manager) app(manifest Manifest, builtin bool) App {
	status := "stopped"
	if app := m.running[manifest.ID]; app != nil && app.alive() {
		status = "running"
	}
	return App{Manifest: manifest, Builtin: builtin, Status: status}
}

func (app *instance) alive() bool {
	select {
	case <-app.done:
		return false
	default:
		return true
	}
}

func (m *Manager) Launch(id, browserHost string) (string, error) {
	if !appID.MatchString(id) {
		return "", errAppNotInstalled
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.ctx.Err() != nil {
		return "", errors.New("App runtime is shutting down")
	}
	if app := m.running[id]; app != nil && app.alive() {
		return app.url(browserHost), nil
	}
	m.stop(id)

	listener, err := net.Listen("tcp", net.JoinHostPort(m.host, "0"))
	if err != nil {
		return "", err
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	app := &instance{port: port}
	var handler http.Handler
	switch id {
	case devkitID:
		handler = m.devkit()
	default:
		handler, app.stop, app.done, err = m.startProcess(id)
		if id == fastLearningID && errors.Is(err, errAppNotInstalled) {
			handler = fastlearning.New(m.appData(id), m.agent)
			err = nil
		}
		if err != nil {
			listener.Close()
			return "", err
		}
	}

	app.server = &http.Server{
		Handler:           m.appGateway(port, handler),
		ReadHeaderTimeout: 5 * time.Second,
	}
	m.running[id] = app
	go func() { _ = app.server.Serve(listener) }()
	if app.done != nil {
		go func() {
			<-app.done
			_ = app.server.Close()
		}()
	}
	return app.url(browserHost), nil
}

func (m *Manager) appData(id string) string {
	return filepath.Join(filepath.Dir(m.registry), "data", id)
}

func (app *instance) url(host string) string {
	return "http://" + net.JoinHostPort(host, app.port) + "/"
}

// stop expects m.mu to be held.
func (m *Manager) stop(id string) {
	app := m.running[id]
	if app == nil {
		return
	}
	delete(m.running, id)
	_ = app.server.Close()
	if app.stop != nil {
		app.stop()
		<-app.done
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for id := range m.running {
		m.stop(id)
	}
}

func (m *Manager) startProcess(id string) (http.Handler, context.CancelFunc, <-chan struct{}, error) {
	pack, err := zip.OpenReader(filepath.Join(m.registry, id+".zip"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil, errAppNotInstalled
	}
	if err != nil {
		return nil, nil, nil, err
	}
	defer pack.Close()
	manifest, err := readInstalledManifest(pack)
	if err != nil {
		return nil, nil, nil, err
	}
	if manifest.ID != id {
		return nil, nil, nil, errors.New("installed package does not match its App ID")
	}
	dir, err := os.MkdirTemp("", "ca-")
	if err != nil {
		return nil, nil, nil, err
	}
	socket := filepath.Join(dir, "s")
	binary := filepath.Join(dir, filepath.Base(serverFile()))
	dataDir := m.appData(id)
	if err = unpackServer(pack, binary); err == nil {
		err = os.MkdirAll(dataDir, 0700)
	}
	if err != nil {
		os.RemoveAll(dir)
		return nil, nil, nil, err
	}

	ctx, cancel := context.WithCancel(m.ctx)
	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = m.project
	cmd.Env = []string{
		"CLAUDE_APP_SOCKET=" + socket,
		"CLAUDE_APP_PROJECT=" + m.project,
		"CLAUDE_APP_DATA_DIR=" + dataDir,
	}
	var bridge *http.Server
	if manifest.hasAgent() && m.agent != nil {
		agentSocket := filepath.Join(dir, "a")
		bridge, err = m.listenAgent(ctx, agentSocket)
		if err != nil {
			cancel()
			os.RemoveAll(dir)
			return nil, nil, nil, err
		}
		cmd.Env = append(cmd.Env, "CLAUDE_APP_AGENT_SOCKET="+agentSocket)
	}
	log, err := os.OpenFile(filepath.Join(dataDir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err == nil {
		cmd.Stdout, cmd.Stderr = log, log
		err = cmd.Start()
		log.Close()
	}
	if err != nil {
		cancel()
		if bridge != nil {
			_ = bridge.Close()
		}
		os.RemoveAll(dir)
		return nil, nil, nil, fmt.Errorf("start Go server: %w", err)
	}
	done := make(chan struct{})
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}
	go func() {
		_ = cmd.Wait()
		if bridge != nil {
			_ = bridge.Close()
		}
		transport.CloseIdleConnections()
		os.RemoveAll(dir)
		close(done)
	}()
	if err := waitReady(ctx, done, transport); err != nil {
		cancel()
		<-done
		return nil, nil, nil, fmt.Errorf("%w (log: %s)", err, filepath.Join(dataDir, "server.log"))
	}
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(&url.URL{Scheme: "http", Host: "app"})
			request.Out.Host = request.In.Host
		},
	}
	return proxy, cancel, done, nil
}

func (m *Manager) listenAgent(ctx context.Context, path string) (*http.Server, error) {
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen for App Agent requests: %w", err)
	}
	server := &http.Server{
		Handler: appagent.Handler(m.agent), ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	go func() { _ = server.Serve(listener) }()
	return server, nil
}

func waitReady(ctx context.Context, done <-chan struct{}, transport http.RoundTripper) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ticker := time.NewTicker(60 * time.Millisecond)
	defer ticker.Stop()
	for {
		probe, cancelProbe := context.WithTimeout(ctx, 500*time.Millisecond)
		request, _ := http.NewRequestWithContext(probe, http.MethodGet, "http://app/health", nil)
		response, err := transport.RoundTrip(request)
		if err == nil {
			response.Body.Close()
		}
		cancelProbe()
		if err == nil && response.StatusCode == http.StatusOK {
			return nil
		}
		select {
		case <-done:
			return errors.New("Go server exited before becoming ready")
		case <-ctx.Done():
			return fmt.Errorf("Go server did not pass GET /health: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (m *Manager) appGateway(port string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, requestPort, _ := net.SplitHostPort(r.Host)
		if requestPort != port || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
			http.Error(w, "unexpected host", http.StatusForbidden)
			return
		}
		origin := "http://" + r.Host
		if source := r.Header.Get("Origin"); source != "" && source != origin {
			http.Error(w, "unexpected origin", http.StatusForbidden)
			return
		}
		switch r.Header.Get("Sec-Fetch-Site") {
		case "cross-site":
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		case "same-site":
			// The parent can load the entry page. Other browser requests
			// must originate from this App's own origin.
			if r.Method != http.MethodGet || r.URL.Path != "/" || r.Header.Get("Sec-Fetch-Dest") != "iframe" {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Content-Security-Policy", "frame-ancestors http://"+net.JoinHostPort(host, m.webPort))
		handler.ServeHTTP(w, r)
	})
}
