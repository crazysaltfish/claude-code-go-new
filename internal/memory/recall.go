package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxMemoryFiles        = 200
	MaxFrontmatterLines   = 30
	MaxRecalledFiles      = 5
	MaxRecalledFileLines  = 200
	MaxRecalledFileBytes  = 4_096
	MaxRecallSessionBytes = 60 * 1_024
)

// Header is the bounded metadata exposed to the relevance selector.
type Header struct {
	Filename    string
	Path        string
	Description string
	Type        string
	ModTime     time.Time
}

// RecalledMemory is a selected topic file ready for contextual injection.
type RecalledMemory struct {
	Path      string
	Content   string
	ModTime   time.Time
	Truncated bool
}

// Selector chooses filenames from a manifest without receiving topic bodies.
type Selector func(ctx context.Context, query, manifest string) ([]string, error)

// Recall scans topic headers, asks a selector for relevant filenames, validates
// its output, and reads only the selected bounded bodies.
func Recall(
	ctx context.Context,
	query string,
	directory string,
	alreadySurfaced map[string]struct{},
	selector Selector,
) []RecalledMemory {
	if directory == "" || selector == nil || !queryHasEnoughContext(query) {
		return nil
	}
	headers := Scan(directory)
	filtered := headers[:0]
	for _, header := range headers {
		if _, seen := alreadySurfaced[header.Path]; !seen {
			filtered = append(filtered, header)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	selected, err := selector(ctx, query, FormatManifest(filtered))
	if err != nil || ctx.Err() != nil {
		return nil
	}
	byFilename := make(map[string]Header, len(filtered))
	for _, header := range filtered {
		byFilename[header.Filename] = header
	}

	result := make([]RecalledMemory, 0, MaxRecalledFiles)
	selectedSet := make(map[string]struct{}, MaxRecalledFiles)
	for _, filename := range selected {
		if len(result) == MaxRecalledFiles {
			break
		}
		header, valid := byFilename[filename]
		if !valid {
			continue
		}
		if _, duplicate := selectedSet[filename]; duplicate {
			continue
		}
		selectedSet[filename] = struct{}{}
		content, truncated, ok := readRecalledFile(header.Path)
		if !ok {
			continue
		}
		result = append(result, RecalledMemory{
			Path:      header.Path,
			Content:   content,
			ModTime:   header.ModTime,
			Truncated: truncated,
		})
	}
	return result
}

func queryHasEnoughContext(query string) bool {
	trimmed := strings.TrimSpace(query)
	return len(strings.Fields(trimmed)) > 1 || utf8.RuneCountInString(trimmed) >= 8
}

// Scan returns newest-first topic headers while excluding the entrypoint.
func Scan(directory string) []Header {
	headers := make([]Header, 0)
	_ = filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") || entry.Name() == EntrypointName {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		content, ok := readRegularFileNoFollow(path, 16*1_024)
		if !ok {
			return nil
		}
		description, memoryType := parseFrontmatterHeader(content)
		relative, err := filepath.Rel(directory, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil
		}
		headers = append(headers, Header{
			Filename:    relative,
			Path:        path,
			Description: description,
			Type:        memoryType,
			ModTime:     info.ModTime(),
		})
		return nil
	})
	sort.SliceStable(headers, func(i, j int) bool {
		return headers[i].ModTime.After(headers[j].ModTime)
	})
	if len(headers) > MaxMemoryFiles {
		headers = headers[:MaxMemoryFiles]
	}
	return headers
}

// FormatManifest exposes only filenames, timestamps, types, and descriptions.
func FormatManifest(headers []Header) string {
	lines := make([]string, 0, len(headers))
	for _, header := range headers {
		typePrefix := ""
		if header.Type != "" {
			typePrefix = "[" + header.Type + "] "
		}
		line := fmt.Sprintf("- %s%s (%s)", typePrefix, header.Filename, header.ModTime.UTC().Format(time.RFC3339))
		if header.Description != "" {
			line += ": " + header.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func parseFrontmatterHeader(content string) (string, string) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	limit := min(len(lines), MaxFrontmatterLines)
	description := ""
	memoryType := ""
	for _, line := range lines[1:limit] {
		line = strings.TrimSpace(line)
		if line == "---" {
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "description":
			description = value
		case "type":
			switch strings.ToLower(value) {
			case "user", "feedback", "project", "reference":
				memoryType = strings.ToLower(value)
			}
		}
	}
	return description, memoryType
}

func readRecalledFile(path string) (string, bool, bool) {
	raw, ok := readRegularFileNoFollow(path, MaxRecalledFileBytes+1)
	if !ok {
		return "", false, false
	}
	byteTruncated := len(raw) > MaxRecalledFileBytes
	if byteTruncated {
		raw = truncateUTF8Bytes(raw, MaxRecalledFileBytes)
	}
	lines := strings.Split(raw, "\n")
	lineTruncated := len(lines) > MaxRecalledFileLines
	if lineTruncated {
		lines = lines[:MaxRecalledFileLines]
	}
	content := strings.TrimSpace(strings.Join(lines, "\n"))
	truncated := byteTruncated || lineTruncated
	if truncated {
		content += fmt.Sprintf("\n\n> This memory was truncated. Read the complete file at %s if more detail is needed.", path)
	}
	return content, truncated, true
}
