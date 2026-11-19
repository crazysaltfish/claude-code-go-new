package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"claude-code-go/internal/tools"
	"claude-code-go/pkg/api"
	"claude-code-go/pkg/appagent"
)

type webAppAgent struct {
	app      *App
	search   *http.Client
	braveKey string
}

func newWebAppAgent(app *App) *webAppAgent {
	return &webAppAgent{app: app, braveKey: os.Getenv("BRAVE_SEARCH_API_KEY"), search: &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (a *webAppAgent) Complete(ctx context.Context, input appagent.Completion) (string, error) {
	if a.app.apiClient == nil {
		return "", errors.New("Web workspace model is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 85*time.Second)
	defer cancel()
	response, err := a.app.apiClient.CreateMessage(ctx, api.MessageRequest{
		Model: a.app.config.Model, System: input.System, MaxTokens: input.MaxTokens,
		Messages: []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: input.Prompt}}}},
	})
	if err != nil {
		return "", fmt.Errorf("workspace model: %w", err)
	}
	var text strings.Builder
	for _, block := range response.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() == 0 {
		return "", errors.New("workspace model returned no text")
	}
	return text.String(), nil
}

func (a *webAppAgent) Search(ctx context.Context, input appagent.Search) (appagent.Results, error) {
	if a.braveKey != "" {
		sources, err := a.brave(ctx, input.Query)
		if err == nil && len(sources) > 0 {
			return appagent.Results{Sources: sources, Method: "brave"}, nil
		}
	}
	english := input.English
	if english == "" {
		english = input.Query
	}
	type response struct {
		sources []appagent.Source
		err     error
	}
	results := make(chan response, 1)
	go func() {
		sources, err := a.wikipedia(ctx, "en", english)
		results <- response{sources, err}
	}()
	sources, err := a.wikipedia(ctx, "zh", input.Query)
	other := <-results
	if err != nil && other.err != nil {
		return appagent.Results{}, errors.New("public search is temporarily unavailable")
	}
	return appagent.Results{Sources: append(sources, other.sources...), Method: "wikipedia"}, nil
}

func (a *webAppAgent) brave(ctx context.Context, query string) ([]appagent.Source, error) {
	var result struct {
		Web struct {
			Results []struct {
				Title         string   `json:"title"`
				URL           string   `json:"url"`
				Description   string   `json:"description"`
				ExtraSnippets []string `json:"extra_snippets"`
			} `json:"results"`
		} `json:"web"`
	}
	err := a.searchJSON(ctx, "https://api.search.brave.com/res/v1/web/search", url.Values{
		"q": {limitRunes(query, 300)}, "count": {"8"}, "extra_snippets": {"true"}, "spellcheck": {"true"},
	}, a.braveKey, &result)
	if err != nil {
		return nil, err
	}
	var sources []appagent.Source
	seen := make(map[string]bool)
	for _, item := range result.Web.Results {
		address, err := url.Parse(item.URL)
		if err != nil || address.Scheme != "https" || address.User != nil || address.Hostname() == "" || seen[item.URL] {
			continue
		}
		title := cleanExcerpt(item.Title, 160)
		excerpt := cleanExcerpt(strings.Join(append([]string{item.Description}, item.ExtraSnippets...), " "), 850)
		if title == "" || excerpt == "" {
			continue
		}
		seen[item.URL] = true
		sources = append(sources, appagent.Source{Title: title, URL: item.URL, Excerpt: excerpt})
		if len(sources) == 6 {
			break
		}
	}
	return sources, nil
}

func (a *webAppAgent) wikipedia(ctx context.Context, language, query string) ([]appagent.Source, error) {
	base := "https://" + language + ".wikipedia.org"
	var result struct {
		Error json.RawMessage `json:"error"`
		Query struct {
			Pages []struct {
				ID      int    `json:"pageid"`
				Index   int    `json:"index"`
				Title   string `json:"title"`
				Extract string `json:"extract"`
			} `json:"pages"`
		} `json:"query"`
	}
	err := a.searchJSON(ctx, base+"/w/api.php", url.Values{
		"action": {"query"}, "format": {"json"}, "formatversion": {"2"},
		"generator": {"search"}, "gsrsearch": {limitRunes(query, 300)}, "gsrlimit": {"3"},
		"prop": {"extracts"}, "exintro": {"1"}, "explaintext": {"1"}, "exlimit": {"max"}, "exchars": {"850"},
	}, "", &result)
	if err != nil {
		return nil, err
	}
	if len(result.Error) != 0 {
		return nil, errors.New("Wikipedia search returned an error")
	}
	sort.SliceStable(result.Query.Pages, func(i, j int) bool {
		return result.Query.Pages[i].Index < result.Query.Pages[j].Index
	})
	var sources []appagent.Source
	for _, page := range result.Query.Pages {
		excerpt := cleanExcerpt(page.Extract, 850)
		if page.ID > 0 && excerpt != "" {
			sources = append(sources, appagent.Source{
				Title: cleanExcerpt(page.Title, 160), URL: fmt.Sprintf("%s/?curid=%d", base, page.ID), Excerpt: excerpt,
			})
		}
	}
	return sources, nil
}

func (a *webAppAgent) searchJSON(ctx context.Context, endpoint string, params url.Values, key string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "ClaudeCodeGo-AgentApps/1.0 (local learning research)")
	if key != "" {
		request.Header.Set("X-Subscription-Token", key)
	}
	response, err := a.search.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("search returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 512<<10)).Decode(output)
}

var searchMarkup = regexp.MustCompile(`<[^>]{1,300}>`)

func cleanExcerpt(value string, limit int) string {
	value = html.UnescapeString(searchMarkup.ReplaceAllString(value, " "))
	value = tools.SanitizeUntrustedText(value)
	return limitRunes(strings.Join(strings.Fields(value), " "), limit)
}

func limitRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
