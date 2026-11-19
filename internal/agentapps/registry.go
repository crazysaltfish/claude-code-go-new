// Package agentapps manages local Agent Apps for one Web workspace.
package agentapps

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	devkitID        = "claude-app-devkit"
	fastLearningID  = "fast-learning"
	manifestFile    = "app.json"
	maxManifestSize = 16 << 10
	maxServerSize   = 128 << 20
)

var (
	appID      = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	appVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

type Manifest struct {
	SchemaVersion int      `json:"schema_version"`
	ID            string   `json:"id"`
	Version       string   `json:"version"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Capabilities  []string `json:"capabilities,omitempty"`
}

type App struct {
	Manifest
	Builtin bool   `json:"builtin"`
	Status  string `json:"status"`
}

func readManifest(reader io.Reader) (Manifest, error) {
	var manifest Manifest
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestSize+1))
	if err != nil {
		return manifest, err
	}
	if len(data) > maxManifestSize {
		return manifest, errors.New("app.json exceeds 16 KiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("app.json: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return manifest, errors.New("app.json must contain one JSON object")
	}
	if manifest.SchemaVersion != 1 {
		return manifest, errors.New("app.json requires schema_version 1")
	}
	if !appID.MatchString(manifest.ID) || !appVersion.MatchString(manifest.Version) ||
		strings.TrimSpace(manifest.Name) == "" || len(manifest.Name) > 160 || len(manifest.Description) > 1000 {
		return manifest, errors.New("app.json requires a valid id, version and name")
	}
	if len(manifest.Capabilities) > 1 || (len(manifest.Capabilities) == 1 && !manifest.hasAgent()) {
		return manifest, errors.New("app.json supports only the agent capability")
	}
	return manifest, nil
}

func (m Manifest) hasAgent() bool {
	return len(m.Capabilities) == 1 && m.Capabilities[0] == "agent"
}

func registryPath(project, dataHome string) string {
	hash := sha256.Sum256([]byte(project))
	return filepath.Join(dataHome, "agent-apps", fmt.Sprintf("%x", hash), "installed")
}

func serverFile() string {
	if runtime.GOOS == "windows" {
		return "app/server-go/server.exe"
	}
	return "app/server-go/server"
}

func openPackage(dir string) (Manifest, *os.File, error) {
	var manifest Manifest
	if !filepath.IsAbs(dir) {
		return manifest, nil, errors.New("enter an absolute App directory")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return manifest, nil, err
	}
	defer root.Close()
	config, err := root.Open(manifestFile)
	if err != nil {
		return manifest, nil, err
	}
	defer config.Close()
	manifest, err = readManifest(config)
	if err != nil {
		return manifest, nil, err
	}
	if manifest.ID == devkitID {
		return manifest, nil, errors.New("the default DevKit ID is reserved")
	}
	binary, err := root.Open(serverFile())
	if err != nil {
		return manifest, nil, fmt.Errorf("open the built Go server at %s: %w", serverFile(), err)
	}
	info, err := binary.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > maxServerSize {
		binary.Close()
		return manifest, nil, errors.New("the Go server must be a regular file up to 128 MiB")
	}
	return manifest, binary, nil
}

// An installed package is replaced with a single rename, so the manifest and
// executable used at launch always come from the same installation.
func savePackage(registry string, manifest Manifest, binary io.Reader) error {
	if err := os.MkdirAll(registry, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(registry, ".install-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	writer := zip.NewWriter(file)
	err = writePackage(writer, manifest, binary)
	err = errors.Join(err, writer.Close(), file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(registry, manifest.ID+".zip"))
}

func writePackage(writer *zip.Writer, manifest Manifest, binary io.Reader) error {
	item, err := writer.Create(manifestFile)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(item).Encode(manifest); err != nil {
		return err
	}
	item, err = writer.CreateHeader(&zip.FileHeader{Name: serverFile(), Method: zip.Store})
	if err != nil {
		return err
	}
	count, err := io.Copy(item, io.LimitReader(binary, maxServerSize+1))
	if err == nil && (count == 0 || count > maxServerSize) {
		return errors.New("the Go server must be a regular file up to 128 MiB")
	}
	return err
}

func readInstalledManifest(root fs.FS) (Manifest, error) {
	file, err := root.Open(manifestFile)
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()
	return readManifest(file)
}

func unpackServer(root fs.FS, target string) error {
	source, err := root.Open(serverFile())
	if err != nil {
		return err
	}
	defer source.Close()
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	count, copyErr := io.Copy(file, io.LimitReader(source, maxServerSize+1))
	if copyErr == nil && (count == 0 || count > maxServerSize) {
		copyErr = errors.New("invalid installed server size")
	}
	return errors.Join(copyErr, file.Close())
}
