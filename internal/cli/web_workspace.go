package cli

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"claude-code-go/internal/utils"
)

type workspaceFile struct {
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Modified time.Time `json:"modified"`
}

type workspaceListing struct {
	Path    string          `json:"path"`
	Entries []workspaceFile `json:"entries"`
}

func (s *webServer) workspaceFiles(w http.ResponseWriter, r *http.Request) {
	scope := r.PathValue("scope")
	relative := r.URL.Query().Get("path")
	if relative == "" {
		relative = "."
	}
	local, err := filepath.Localize(relative)
	if err != nil {
		webError(w, http.StatusBadRequest, "无效的目录路径")
		return
	}

	var listing workspaceListing
	switch scope {
	case "session":
		listing, err = s.sessionFiles(r.URL.Query().Get("chat"), local)
	case "personal":
		listing, err = listWorkspace(utils.GetClaudeConfigHome(), local)
		if errors.Is(err, fs.ErrNotExist) && local == "." {
			listing = workspaceListing{Path: utils.GetClaudeConfigHome(), Entries: []workspaceFile{}}
			err = nil
		}
	case "public":
		listing, err = listWorkspace(s.root.config.Cwd, local)
	default:
		webError(w, http.StatusNotFound, "找不到目录")
		return
	}
	if err != nil {
		status, message := http.StatusInternalServerError, "无法读取目录"
		switch {
		case errors.Is(err, fs.ErrNotExist):
			status, message = http.StatusNotFound, "找不到目录"
		case errors.Is(err, fs.ErrPermission):
			status, message = http.StatusForbidden, "无法访问此目录"
		}
		webError(w, status, message)
		return
	}
	writeWebJSON(w, http.StatusOK, listing)
}

func (s *webServer) sessionFiles(id, relative string) (workspaceListing, error) {
	snapshot, err := s.root.sessionStore.Path(id)
	if err != nil {
		return workspaceListing{}, fs.ErrNotExist
	}
	records, err := s.root.sessionStore.AgentDir(id)
	if err != nil {
		return workspaceListing{}, err
	}
	base := filepath.Dir(snapshot)
	root, err := os.OpenRoot(base)
	if err != nil {
		return workspaceListing{}, err
	}
	defer root.Close()

	info, err := root.Lstat(filepath.Base(snapshot))
	if err != nil {
		return workspaceListing{}, err
	}
	if !info.Mode().IsRegular() {
		return workspaceListing{}, fs.ErrNotExist
	}
	recordsPath, err := filepath.Rel(base, records)
	if err != nil {
		return workspaceListing{}, err
	}
	// Project the session snapshot and its own Agent records into one view.
	if relative == "." {
		files := make([]workspaceFile, 0, 2)
		if recordInfo, err := workspaceDirectory(root, recordsPath); err == nil {
			folder := fileMetadata(recordInfo, "directory")
			folder.Name = "agents"
			files = append(files, folder)
		}
		files = append(files, fileMetadata(info, "file"))
		return workspaceListing{Entries: files}, nil
	}
	folder, rest, _ := strings.Cut(relative, string(filepath.Separator))
	if folder != "agents" {
		return workspaceListing{}, fs.ErrNotExist
	}
	return readWorkspace(root, filepath.Join(recordsPath, rest))
}

func listWorkspace(base, relative string) (workspaceListing, error) {
	root, err := os.OpenRoot(base)
	if err != nil {
		return workspaceListing{}, err
	}
	defer root.Close()
	return readWorkspace(root, relative)
}

func workspaceDirectory(root *os.Root, relative string) (fs.FileInfo, error) {
	prefix := "."
	var info fs.FileInfo
	for part := range strings.SplitSeq(relative, string(filepath.Separator)) {
		prefix = filepath.Join(prefix, part)
		entry, err := root.Lstat(prefix)
		if err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			return nil, fs.ErrNotExist
		}
		info = entry
	}
	return info, nil
}

func readWorkspace(root *os.Root, relative string) (workspaceListing, error) {
	if _, err := workspaceDirectory(root, relative); err != nil {
		return workspaceListing{}, err
	}
	entries, err := fs.ReadDir(root.FS(), filepath.ToSlash(relative))
	if err != nil {
		return workspaceListing{}, err
	}
	files := make([]workspaceFile, 0, len(entries))
	for _, entry := range entries {
		kind := "file"
		switch {
		case entry.IsDir():
			kind = "directory"
		case entry.Type()&os.ModeSymlink != 0:
			kind = "link"
		case !entry.Type().IsRegular():
			continue
		}
		if info, err := entry.Info(); err == nil {
			files = append(files, fileMetadata(info, kind))
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Kind == "directory" && files[j].Kind != "directory"
	})
	return workspaceListing{Path: filepath.Join(root.Name(), relative), Entries: files}, nil
}

func fileMetadata(info fs.FileInfo, kind string) workspaceFile {
	return workspaceFile{Name: info.Name(), Kind: kind, Modified: info.ModTime()}
}
