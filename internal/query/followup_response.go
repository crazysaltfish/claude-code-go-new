package query

import (
	"regexp"
	"strings"

	"claude-code-go/pkg/api"
)

const followupTextInstruction = "For this automatic sub-agent update, respond only with a human-readable summary. Do not request tools or emit tool-call markup. Base the summary on the reported results and identify any unverified claims."

var (
	dsmlOpen  = regexp.MustCompile(`<\s*[|｜]{1,2}DSML[|｜]{1,2}\s*(tool_calls|function_calls|calls|invoke|parameter)\b`)
	dsmlClose = regexp.MustCompile(`\\?<\s*/\s*[|｜]{1,2}DSML[|｜]{1,2}\s*(tool_calls|function_calls|calls|invoke|parameter)\s*>`)
)

// cleanFollowupResponse removes provider tool markup before it can enter the
// transcript. A partial call at the end of a response is also discarded.
func cleanFollowupResponse(response *api.MessageResponse) bool {
	var hadCall bool
	blocks := response.Content[:0]
	for _, block := range response.Content {
		if block.Type == "tool_use" {
			hadCall = true
			continue
		}
		if block.Type == "text" {
			var changed bool
			block.Text, changed = stripDSMLCalls(block.Text)
			hadCall = hadCall || changed
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
		}
		blocks = append(blocks, block)
	}
	response.Content = blocks
	return hadCall
}

func stripDSMLCalls(value string) (string, bool) {
	var prose strings.Builder
	changed := false
	for {
		open := dsmlOpen.FindStringSubmatchIndex(value)
		if open == nil {
			prose.WriteString(value)
			break
		}
		changed = true
		prefix := value[:open[0]]
		prose.WriteString(strings.TrimSuffix(prefix, "\\"))
		name := value[open[2]:open[3]]
		rest := value[open[1]:]
		end := -1
		for _, closeTag := range dsmlClose.FindAllStringSubmatchIndex(rest, -1) {
			if rest[closeTag[2]:closeTag[3]] == name {
				end = closeTag[1]
				break
			}
		}
		if end < 0 {
			break
		}
		value = strings.TrimLeft(rest[end:], "\\")
	}
	if !changed {
		return value, false
	}
	return strings.TrimSpace(prose.String()), true
}
