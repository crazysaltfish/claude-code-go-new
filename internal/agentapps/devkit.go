package agentapps

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed claude-app-devkit/app.json claude-app-devkit/app/frontend template
var assets embed.FS

var builtinManifest = func() Manifest {
	manifest, err := readInstalledManifest(mustSub("claude-app-devkit"))
	if err != nil {
		panic(err)
	}
	return manifest
}()

func mustSub(dir string) fs.FS {
	root, err := fs.Sub(assets, dir)
	if err != nil {
		panic(err)
	}
	return root
}

func (m *Manager) devkit() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(mustSub("claude-app-devkit/app/frontend")))
	mux.HandleFunc("GET /api/apps", func(w http.ResponseWriter, _ *http.Request) {
		apps, err := m.List()
		devResponse(w, apps, err)
	})
	mux.HandleFunc("POST /api/apps/{action}", m.devkitAction)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}

type devkitInput struct {
	Directory string `json:"directory"`
	ID        string `json:"id"`
	Name      string `json:"name"`
}

func (m *Manager) devkitAction(w http.ResponseWriter, r *http.Request) {
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return
	}
	var input devkitInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || decoder.Decode(new(any)) != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid App management request"})
		return
	}
	switch r.PathValue("action") {
	case "inspect", "install":
		manifest, binary, err := openPackage(input.Directory)
		if err != nil {
			devResponse(w, nil, err)
			return
		}
		defer binary.Close()
		if r.PathValue("action") == "install" {
			err = m.install(manifest, binary)
		}
		devResponse(w, manifest, err)
	case "scaffold":
		dir, err := m.scaffold(input.ID, input.Name)
		devResponse(w, map[string]string{
			"directory": dir,
			"build":     "go build -o " + serverFile() + " ./app/server-go",
		}, err)
	case "stop", "remove":
		if !appID.MatchString(input.ID) || input.ID == devkitID {
			devResponse(w, nil, errors.New("invalid or reserved App ID"))
			return
		}
		err := m.manage(input.ID, r.PathValue("action") == "remove")
		devResponse(w, map[string]string{"id": input.ID}, err)
	default:
		http.NotFound(w, r)
	}
}

func (m *Manager) install(manifest Manifest, binary io.Reader) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("App runtime is shutting down")
	}
	if err := savePackage(m.registry, manifest, binary); err != nil {
		return err
	}
	m.stop(manifest.ID)
	return nil
}

func (m *Manager) manage(id string, remove bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if remove {
		if err := os.Remove(filepath.Join(m.registry, id+".zip")); err != nil {
			return err
		}
	}
	m.stop(id)
	return nil
}

func (m *Manager) scaffold(id, name string) (string, error) {
	manifest := Manifest{SchemaVersion: 1, ID: id, Version: "0.1.0", Name: strings.TrimSpace(name)}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if _, err := readManifest(bytes.NewReader(data)); err != nil {
		return "", err
	}
	if id == devkitID {
		return "", errors.New("the default DevKit ID is reserved")
	}
	dir := filepath.Join(m.project, id)
	if err := os.Mkdir(dir, 0755); err != nil {
		return "", err
	}
	if err := writeScaffold(dir, append(data, '\n')); err != nil {
		return "", fmt.Errorf("write scaffold in %s: %w", dir, err)
	}
	return dir, nil
}

func writeScaffold(dir string, manifest []byte) error {
	template := mustSub("template")
	return fs.WalkDir(template, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == "." {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(path, ".tmpl")))
		if entry.IsDir() {
			return os.Mkdir(target, 0755)
		}
		data, err := fs.ReadFile(template, path)
		if err != nil {
			return err
		}
		if path == "app.json.tmpl" {
			data = manifest
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		_, err = file.Write(data)
		return errors.Join(err, file.Close())
	})
}

func devResponse(w http.ResponseWriter, data any, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
