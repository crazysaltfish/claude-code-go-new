// Package appagent defines the model and research capabilities available to Agent Apps.
package appagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type Completion struct {
	System    string `json:"system"`
	Prompt    string `json:"prompt"`
	MaxTokens int    `json:"max_tokens"`
}

type Search struct {
	Query   string `json:"query"`
	English string `json:"english,omitempty"`
}

type Source struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Excerpt string `json:"excerpt"`
}

type Results struct {
	Sources []Source `json:"sources"`
	Method  string   `json:"method"`
}

type Agent interface {
	Complete(context.Context, Completion) (string, error)
	Search(context.Context, Search) (Results, error)
}

type reply struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

type Client struct {
	http *http.Client
}

func NewClient(socket string) *Client {
	return &Client{http: &http.Client{
		Timeout: 100 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				if socket == "" {
					return nil, errors.New("Agent socket is unavailable")
				}
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
			IdleConnTimeout: 30 * time.Second,
		},
	}}
}

func (c *Client) Complete(ctx context.Context, input Completion) (string, error) {
	var result reply
	err := c.post(ctx, "/v1/complete", input, &result)
	return result.Text, err
}

func (c *Client) Search(ctx context.Context, input Search) (Results, error) {
	var result Results
	err := c.post(ctx, "/v1/search", input, &result)
	return result, err
}

func (c *Client) post(ctx context.Context, path string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://agent"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call App Agent: %w", err)
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusOK {
		var failure reply
		if err := decoder.Decode(&failure); err == nil && failure.Error != "" {
			return errors.New(failure.Error)
		}
		return fmt.Errorf("App Agent returned HTTP %d", response.StatusCode)
	}
	return decoder.Decode(output)
}
