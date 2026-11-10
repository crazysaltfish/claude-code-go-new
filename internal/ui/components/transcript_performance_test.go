package components

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

func TestToolPreviewsBoundAllSectionsWithoutMutatingSource(t *testing.T) {
	output := strings.Repeat("中文🙂 long output\n", 10000) + "OUTPUT-TAIL"
	input, _ := json.Marshal(map[string]string{"command": strings.Repeat("long argument ", 10000) + "INPUT-TAIL"})
	diff := strings.Repeat("+changed line\n", 10000) + "DIFF-TAIL"
	block := ContentBlock{Type: "tool", Name: "Bash", Input: input, Content: output, Diff: diff, Status: "completed"}
	view := renderMessageWithMode(MessageModel{Role: "tool", Content: []ContentBlock{block}}, 80, TranscriptVerbose)
	if len(strings.Split(view, "\n")) > 90 || len(view) > 32*1024 {
		t.Fatalf("preview is unbounded: bytes=%d lines=%d", len(view), strings.Count(view, "\n"))
	}
	if !utf8.ValidString(view) || !strings.Contains(view, "Ctrl+T") {
		t.Fatal("missing usable detail hint")
	}
	for _, tail := range []string{"OUTPUT-TAIL", "INPUT-TAIL", "DIFF-TAIL"} {
		if strings.Contains(view, tail) {
			t.Fatalf("full %s leaked into preview", tail)
		}
	}
	if block.Content != output || string(block.Input) != string(input) || block.Diff != diff {
		t.Fatal("preview altered full source")
	}
	detail := NewToolDetail([]ContentBlock{block}, 80, 24)
	for _, tail := range []string{"OUTPUT-TAIL", "INPUT-TAIL", "DIFF-TAIL"} {
		if !strings.Contains(detail.text, tail) {
			t.Fatalf("details lost %s", tail)
		}
	}
}

func TestToolPreviewBoundsSingleHugeLineAndJSON(t *testing.T) {
	for _, output := range []string{strings.Repeat("中🙂", 100000), `{"value":"` + strings.Repeat("x", 100000) + `"}`} {
		view := toolPreview(output, 60, renderTextOutput)
		if !utf8.ValidString(view) || strings.Count(view, "\n") > toolPreviewLines || len(view) > 10000 {
			t.Fatal("single line preview unbounded or invalid")
		}
	}
}

func TestTranscriptRendersOnlyViewportAndInvalidatesChangedMessage(t *testing.T) {
	list := NewMessageList(80, 12)
	list.Mode = TranscriptVerbose
	for i := 0; i < 5000; i++ {
		list.UpsertToolUse("Bash", fmt.Sprint(i), json.RawMessage(`{"command":"test"}`))
		list.CompleteToolUse("Bash", fmt.Sprint(i), strings.Repeat("row\n", 1000), false, false, 4000, nil)
	}
	list.View()
	if len(list.renderCache) > 3 || len(list.lineHeights) > 3 {
		t.Fatalf("formatted offscreen history: cache=%d heights=%d", len(list.renderCache), len(list.lineHeights))
	}
	list.CompleteToolUse("Bash", "4999", "latest result", false, false, 13, nil)
	if view := list.View(); !strings.Contains(view, "latest result") {
		t.Fatal("stale cached result")
	}
	list.Width = 30
	list.View()
	if list.cacheWidth != 30 {
		t.Fatal("resize did not invalidate cache")
	}
	list.CycleTranscriptMode()
	list.View()
	if list.cacheMode != TranscriptSummary {
		t.Fatal("mode did not invalidate cache")
	}
}

func TestViewportMatchesFullTranscriptAcrossModesAndScroll(t *testing.T) {
	list := NewMessageList(50, 9)
	for i := 0; i < 35; i++ {
		list.AddMessage(MessageModel{Role: "assistant", Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("reply %d\nsecond line", i)}}})
		list.UpsertToolUse("Bash", fmt.Sprint(i), json.RawMessage(`{"command":"test"}`))
		list.CompleteToolUse("Bash", fmt.Sprint(i), "output\nsecond output", false, false, 20, nil)
	}
	for _, mode := range []TranscriptMode{TranscriptNormal, TranscriptVerbose, TranscriptSummary} {
		list.Mode = mode
		// Compute an independent full render for comparison with the virtual viewport.
		var all strings.Builder
		for i, msg := range list.Messages {
			if list.messageVisible(i, msg) {
				all.WriteString(renderMessageWithMode(msg, list.Width, mode))
				all.WriteByte('\n')
			}
		}
		lines := strings.Split(strings.TrimSuffix(all.String(), "\n"), "\n")
		for _, requested := range []int{0, 1, 5, 30, 10000} {
			list.ScrollOffset = requested
			offset := min(requested, max(0, len(lines)-list.MaxVisible))
			end := len(lines) - offset
			start := max(0, end-list.MaxVisible)
			visible := append([]string(nil), lines[start:end]...)
			for len(visible) < list.MaxVisible {
				visible = append(visible, "")
			}
			want := strings.Join(visible, "\n") + "\n"
			if got := list.View(); got != want {
				t.Fatalf("mode %v offset %d viewport mismatch", mode, requested)
			}
		}
	}
	if len(list.renderCache) > transcriptCacheEntries {
		t.Fatal("unbounded rendered cache")
	}
}

func TestStreamingWithinScrolledMessagePreservesAnchor(t *testing.T) {
	list := NewMessageList(60, 8)
	var lines strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&lines, "line-%03d\n", i)
	}
	list.AppendAssistantDelta(lines.String())
	list.PageUp()
	before := list.View()
	list.AppendAssistantDelta("line-040\nline-041\n")
	if got := list.View(); got != before {
		t.Fatalf("streaming inside anchored message moved viewport: before=%q after=%q offset=%d", before, got, list.ScrollOffset)
	}
}

func TestToolDetailSearchNavigationAndReturn(t *testing.T) {
	chat := NewChatModel(80, 24)
	chat.AddToolUse("Bash", "first", json.RawMessage(`{"command":"first"}`))
	chat.AddToolResult("Bash", "first", "first output", false, false, 12, nil)
	chat.AddToolUse("Read", "second", json.RawMessage(`{"file_path":"a.go"}`))
	output := strings.Repeat("row\n", 1000) + "hidden needle" + strings.Repeat("x", 2000) + "second needle"
	chat.AddToolResult("Read", "second", output, false, false, len(output), nil)
	chat.Messages.PageUp()
	offset := chat.Messages.ScrollOffset
	chat.Input.Value = "draft prompt"
	chat.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	viewer := chat.ToolDetail
	if viewer == nil || viewer.Index != 1 {
		t.Fatal("latest tool did not open")
	}
	viewer.Search = "hidden needle"
	viewer.find(false)
	if !strings.Contains(viewer.View(), "hidden needle") || !viewer.found {
		t.Fatal("cannot find hidden output")
	}
	viewer.Search = "second needle"
	viewer.searchPos = -1
	viewer.find(false)
	if !strings.Contains(viewer.View(), "second needle") {
		t.Fatal("cannot find horizontally hidden output")
	}
	viewer.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if viewer.Index != 0 || !strings.Contains(viewer.text, "first output") {
		t.Fatal("cannot select previous tool")
	}
	chat.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if chat.ToolDetail != nil || chat.Input.Value != "draft prompt" || chat.Messages.ScrollOffset != offset {
		t.Fatal("return lost transcript state")
	}
}

func BenchmarkTranscriptViewport(b *testing.B) {
	list := NewMessageList(100, 24)
	list.Mode = TranscriptVerbose
	output := strings.Repeat("long output\n", 10000)
	for i := 0; i < 2000; i++ {
		list.Messages = append(list.Messages, MessageModel{Role: "tool", Content: []ContentBlock{{Type: "tool", Name: "Agent", Content: output, Status: "completed"}}})
	}
	list.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list.View()
	}
}

func BenchmarkTranscriptFullRender(b *testing.B) {
	list := NewMessageList(100, 24)
	list.Mode = TranscriptVerbose
	output := strings.Repeat("long output\n", 10000)
	for i := 0; i < 2000; i++ {
		list.Messages = append(list.Messages, MessageModel{Role: "tool", Content: []ContentBlock{{Type: "tool", Name: "Agent", Content: output, Status: "completed"}}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, message := range list.Messages {
			renderMessageWithMode(message, list.Width, list.Mode)
		}
	}
}

func TestTranscriptCacheIsBoundedAcrossHistoryNavigation(t *testing.T) {
	list := NewMessageList(80, 12)
	for i := 0; i < 1000; i++ {
		list.AddMessage(MessageModel{Role: "assistant", Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("message %d", i)}}})
	}
	list.ScrollOffset = 100000
	if view := list.View(); !strings.Contains(view, "message 0") {
		t.Fatal("cannot scroll to oldest message")
	}
	if len(list.renderCache) > transcriptCacheEntries || len(list.lineHeights) != 1000 {
		t.Fatalf("cache=%d indexed heights=%d", len(list.renderCache), len(list.lineHeights))
	}
	list.ScrollOffset = 0
	if view := list.View(); !strings.Contains(view, "message 999") {
		t.Fatal("eviction lost newest message")
	}
	if len(list.renderCache) > transcriptCacheEntries {
		t.Fatal("rendered cache exceeded bound")
	}
}

func BenchmarkTranscriptColdViewport(b *testing.B) {
	list := NewMessageList(100, 24)
	list.Mode = TranscriptVerbose
	output := strings.Repeat("long output\n", 10000)
	for i := 0; i < 2000; i++ {
		list.Messages = append(list.Messages, MessageModel{Role: "tool", Content: []ContentBlock{{Type: "tool", Name: "Agent", Content: output, Status: "completed"}}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list.renderCache = nil
		list.View()
	}
}

func TestLinearWrapPreservesExistingUnicodeWrapping(t *testing.T) {
	for _, text := range []string{"", "abc\ndef", "中文🙂alpha世界", "a\tb\nc", "e\u0301🙂", strings.Repeat("x", 1000)} {
		for _, width := range []int{1, 2, 6, 80} {
			if got, want := wrapText(text, width), runewidth.Wrap(sanitizeTerminalText(text), width); got != want {
				t.Fatalf("width %d got %q want %q", width, got, want)
			}
		}
	}
}
