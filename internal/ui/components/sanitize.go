package components

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// sanitizeTerminalText removes terminal escape sequences and control bytes from
// untrusted model/tool content before it is styled and written to the terminal.
// Newlines and tabs remain available for normal transcript formatting.
func sanitizeTerminalText(value string) string {
	value = ansi.Strip(value)
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\t':
			return r
		case '\r':
			return -1
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, value)
}
