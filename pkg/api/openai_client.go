package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIClient implements MessageClient using the OpenAI-compatible
// Chat Completions protocol.
type OpenAIClient struct {
	apiKey       string
	authToken    string
	baseURL      string
	organization string
	project      string
	httpClient   *http.Client
	maxRetries   int
}

type openAIChatRequest struct {
	Model             string                 `json:"model"`
	Messages          []openAIChatMessage    `json:"messages"`
	Tools             []openAIToolDefinition `json:"tools,omitempty"`
	MaxTokens         int                    `json:"max_tokens,omitempty"`
	Temperature       *float64               `json:"temperature,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Stream            bool                   `json:"stream,omitempty"`
	StreamOptions     *openAIStreamOptions   `json:"stream_options,omitempty"`
	ParallelToolCalls *bool                  `json:"parallel_tool_calls,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIChatMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolDefinition struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Arguments   string                 `json:"arguments,omitempty"`
}

type openAIToolCall struct {
	Index    int            `json:"index,omitempty"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function openAIFunction `json:"function"`
}

type openAIChatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

type openAIChoice struct {
	Index        int               `json:"index"`
	Message      openAIWireMessage `json:"message"`
	Delta        openAIWireMessage `json:"delta"`
	FinishReason string            `json:"finish_reason"`
}

type openAIWireMessage struct {
	Role      string           `json:"role"`
	Content   json.RawMessage  `json:"content"`
	ToolCalls []openAIToolCall `json:"tool_calls"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	PromptDetails    struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// NewOpenAIClient creates an OpenAI-compatible Chat Completions client.
func NewOpenAIClient(config Config) *OpenAIClient {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 2
	}
	return &OpenAIClient{
		apiKey:       config.APIKey,
		authToken:    config.AuthToken,
		baseURL:      strings.TrimRight(baseURL, "/"),
		organization: config.Organization,
		project:      config.Project,
		httpClient:   &http.Client{Timeout: timeout},
		maxRetries:   maxRetries,
	}
}

// CreateMessage sends a non-streaming Chat Completions request and converts
// the response into the canonical message representation used by QueryEngine.
func (c *OpenAIClient) CreateMessage(ctx context.Context, req MessageRequest) (*MessageResponse, error) {
	wireRequest := buildOpenAIChatRequest(req, false)
	body, err := json.Marshal(wireRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	for attempt := 0; ; attempt++ {
		httpReq, err := c.newChatCompletionsRequest(ctx, body)
		if err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			if attempt < c.maxRetries {
				if err := waitForRetry(ctx, retryDelay(attempt, "")); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("OpenAI request failed: %w", err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read OpenAI response: %w", readErr)
		}
		if resp.StatusCode == http.StatusOK {
			var wireResponse openAIChatResponse
			if err := json.Unmarshal(respBody, &wireResponse); err != nil {
				return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
			}
			return convertOpenAIResponse(wireResponse)
		}
		if attempt < c.maxRetries && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
			if err := waitForRetry(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); err != nil {
				return nil, err
			}
			continue
		}
		return nil, parseAPIError(resp.Status, respBody)
	}
}

// StreamMessage streams Chat Completions chunks and translates them into the
// same events exposed by the Anthropic client.
func (c *OpenAIClient) StreamMessage(ctx context.Context, req MessageRequest, onEvent func(StreamEvent) error) error {
	wireRequest := buildOpenAIChatRequest(req, true)
	body, err := json.Marshal(wireRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}
	httpReq, err := c.newChatCompletionsRequest(ctx, body)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("OpenAI request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return parseAPIError(resp.Status, respBody)
	}

	startedToolCalls := make(map[int]bool)
	startedMessage := false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk openAIChatResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("failed to decode OpenAI stream event: %w", err)
		}
		if !startedMessage {
			startedMessage = true
			message := MessageResponse{
				ID:    chunk.ID,
				Type:  "message",
				Role:  "assistant",
				Model: chunk.Model,
			}
			if err := onEvent(StreamEvent{Type: "message_start", Message: &message}); err != nil {
				return err
			}
		}
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			usage := convertOpenAIUsage(chunk.Usage)
			if err := onEvent(StreamEvent{Type: "message_delta", Usage: &usage}); err != nil {
				return err
			}
		}
		for _, choice := range chunk.Choices {
			text := decodeOpenAIContent(choice.Delta.Content)
			if text != "" {
				if err := onEvent(StreamEvent{
					Type:  "content_block_delta",
					Index: 0,
					Delta: &EventDelta{Type: "text_delta", Text: text},
				}); err != nil {
					return err
				}
			}
			for _, toolCall := range choice.Delta.ToolCalls {
				blockIndex := toolCall.Index + 1
				if !startedToolCalls[toolCall.Index] && (toolCall.ID != "" || toolCall.Function.Name != "") {
					startedToolCalls[toolCall.Index] = true
					block := ContentBlock{
						Type:  "tool_use",
						ID:    toolCall.ID,
						Name:  toolCall.Function.Name,
						Input: json.RawMessage(`{}`),
					}
					if err := onEvent(StreamEvent{
						Type:         "content_block_start",
						Index:        blockIndex,
						ContentBlock: &block,
					}); err != nil {
						return err
					}
				}
				if toolCall.Function.Arguments != "" {
					if err := onEvent(StreamEvent{
						Type:  "content_block_delta",
						Index: blockIndex,
						Delta: &EventDelta{
							Type:        "input_json_delta",
							PartialJSON: toolCall.Function.Arguments,
						},
					}); err != nil {
						return err
					}
				}
			}
			if choice.FinishReason != "" {
				if err := onEvent(StreamEvent{
					Type: "message_delta",
					Delta: &EventDelta{
						StopReason: mapOpenAIStopReason(choice.FinishReason),
					},
				}); err != nil {
					return err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read OpenAI stream: %w", err)
	}
	return nil
}

// CountTokens is not part of the OpenAI-compatible Chat Completions protocol.
func (c *OpenAIClient) CountTokens(context.Context, MessageRequest) (int, error) {
	return 0, fmt.Errorf("token counting is not supported by the OpenAI Chat Completions API")
}

// SetAPIKey updates the bearer API key.
func (c *OpenAIClient) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// SetAuthToken updates the bearer token, which takes precedence over APIKey.
func (c *OpenAIClient) SetAuthToken(authToken string) {
	c.authToken = authToken
}

// SetBaseURL updates the OpenAI-compatible API root.
func (c *OpenAIClient) SetBaseURL(baseURL string) {
	c.baseURL = strings.TrimRight(baseURL, "/")
}

func (c *OpenAIClient) newChatCompletionsRequest(ctx context.Context, body []byte) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	credential := c.authToken
	if credential == "" {
		credential = c.apiKey
	}
	if credential != "" {
		httpReq.Header.Set("Authorization", "Bearer "+credential)
	}
	if c.organization != "" {
		httpReq.Header.Set("OpenAI-Organization", c.organization)
	}
	if c.project != "" {
		httpReq.Header.Set("OpenAI-Project", c.project)
	}
	return httpReq, nil
}

func buildOpenAIChatRequest(req MessageRequest, stream bool) openAIChatRequest {
	wireRequest := openAIChatRequest{
		Model:     req.Model,
		Messages:  convertOpenAIMessages(req),
		Tools:     convertOpenAITools(req.Tools),
		MaxTokens: req.MaxTokens,
		Metadata:  req.Metadata,
		Stream:    stream,
	}
	if req.Temperature != 0 {
		temperature := req.Temperature
		wireRequest.Temperature = &temperature
	}
	if stream {
		wireRequest.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	return wireRequest
}

func convertOpenAIMessages(req MessageRequest) []openAIChatMessage {
	messages := make([]openAIChatMessage, 0, len(req.Messages)+1)
	if system := openAISystemText(req.System); system != "" {
		messages = append(messages, openAIChatMessage{Role: "system", Content: system})
	}
	for _, message := range req.Messages {
		switch message.Role {
		case "assistant":
			wireMessage := openAIChatMessage{Role: "assistant"}
			var textParts []string
			for _, block := range message.Content {
				switch block.Type {
				case "text":
					if block.Text != "" {
						textParts = append(textParts, block.Text)
					}
				case "tool_use":
					arguments := string(block.Input)
					if arguments == "" {
						arguments = "{}"
					}
					wireMessage.ToolCalls = append(wireMessage.ToolCalls, openAIToolCall{
						ID:   block.ID,
						Type: "function",
						Function: openAIFunction{
							Name:      block.Name,
							Arguments: arguments,
						},
					})
				}
			}
			if len(textParts) > 0 {
				wireMessage.Content = strings.Join(textParts, "\n")
			}
			messages = append(messages, wireMessage)
		default:
			var textParts []string
			flushText := func() {
				if len(textParts) == 0 {
					return
				}
				messages = append(messages, openAIChatMessage{
					Role:    message.Role,
					Content: strings.Join(textParts, "\n"),
				})
				textParts = nil
			}
			for _, block := range message.Content {
				if block.Type == "tool_result" {
					flushText()
					content := stringifyToolResult(block.Content)
					if block.IsError {
						content = "Error: " + content
					}
					messages = append(messages, openAIChatMessage{
						Role:       "tool",
						Content:    content,
						ToolCallID: block.ToolUseID,
					})
					continue
				}
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			}
			flushText()
		}
	}
	return messages
}

func convertOpenAITools(tools []ToolDefinition) []openAIToolDefinition {
	result := make([]openAIToolDefinition, 0, len(tools))
	for _, tool := range tools {
		result = append(result, openAIToolDefinition{
			Type: "function",
			Function: openAIFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}
	return result
}

func convertOpenAIResponse(response openAIChatResponse) (*MessageResponse, error) {
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI response contained no choices")
	}
	choice := response.Choices[0]
	content := make([]ContentBlock, 0, len(choice.Message.ToolCalls)+1)
	if text := decodeOpenAIContent(choice.Message.Content); text != "" {
		content = append(content, ContentBlock{Type: "text", Text: text})
	}
	for _, toolCall := range choice.Message.ToolCalls {
		arguments := json.RawMessage(toolCall.Function.Arguments)
		if len(arguments) == 0 {
			arguments = json.RawMessage(`{}`)
		}
		content = append(content, ContentBlock{
			Type:  "tool_use",
			ID:    toolCall.ID,
			Name:  toolCall.Function.Name,
			Input: arguments,
		})
	}
	return &MessageResponse{
		ID:         response.ID,
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      response.Model,
		StopReason: mapOpenAIStopReason(choice.FinishReason),
		Usage:      convertOpenAIUsage(response.Usage),
	}, nil
}

func convertOpenAIUsage(usage openAIUsage) Usage {
	return Usage{
		InputTokens:          usage.PromptTokens,
		OutputTokens:         usage.CompletionTokens,
		CacheReadInputTokens: usage.PromptDetails.CachedTokens,
	}
}

func decodeOpenAIContent(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var texts []string
		for _, part := range parts {
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return string(raw)
}

func openAISystemText(system interface{}) string {
	switch value := system.(type) {
	case nil:
		return ""
	case string:
		return value
	case []ContentBlock:
		var parts []string
		for _, block := range value {
			if block.Type == "text" && block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		data, err := json.Marshal(value)
		if err == nil {
			var blocks []ContentBlock
			if json.Unmarshal(data, &blocks) == nil {
				return openAISystemText(blocks)
			}
		}
		return fmt.Sprint(value)
	}
}

func stringifyToolResult(content interface{}) string {
	if content == nil {
		return ""
	}
	if text, ok := content.(string); ok {
		return text
	}
	data, err := json.Marshal(content)
	if err != nil {
		return fmt.Sprint(content)
	}
	return string(data)
}

func mapOpenAIStopReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls", "function_call":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}
