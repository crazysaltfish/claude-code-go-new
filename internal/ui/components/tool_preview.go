package components

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const toolPreviewBytes = 8 * 1024
const toolPreviewLines = 20

func toolContentText(content interface{}) string {
	if text, ok := content.(string); ok {
		return text
	}
	return fmt.Sprint(content)
}

// Bound work before parsing, wrapping or styling, including a single huge line.
func previewSource(text string) (string, bool) {
	if len(text) <= toolPreviewBytes {
		return text, false
	}
	end := toolPreviewBytes
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end], true
}

func toolPreview(text string, width int, render func(string, int) string) string {
	source, cut := previewSource(text)
	// Discard invisible source rows before any syntax detection or styling.
	offset := 0
	for line := 0; line < toolPreviewLines; line++ {
		next := strings.IndexByte(source[offset:], '\n')
		if next < 0 {
			break
		}
		offset += next + 1
		if line == toolPreviewLines-1 && offset < len(source) {
			source = source[:offset-1]
			cut = true
		}
	}
	rendered := render(source, width)
	lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
	if len(lines) > toolPreviewLines {
		lines = lines[:toolPreviewLines]
		cut = true
	}
	result := strings.Join(lines, "\n")
	if cut {
		result += "\n" + truncationStyle.Render("… preview limited; Ctrl+T: full tool details")
	}
	return result
}
