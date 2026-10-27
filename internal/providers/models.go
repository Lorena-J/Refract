package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/ponytojas/Refract/internal/ai"
)

// Reasoning levels offered per family, mapped to each API in effortOptions.
// A model that turns reasoning down gets retried without it (see Stream).
var efforts = map[string][]ai.Effort{
	"openai":    {{Effort: "low", Description: "Faster, lighter reasoning"}, {Effort: "medium", Description: "Balanced"}, {Effort: "high", Description: "Deeper reasoning"}},
	"anthropic": {{Effort: "low", Description: "Short extended thinking"}, {Effort: "medium", Description: "Balanced extended thinking"}, {Effort: "high", Description: "Long extended thinking"}},
	"google":    {{Effort: "low", Description: "Faster, lighter thinking"}, {Effort: "high", Description: "Deeper thinking"}},
}

// Not chat models, in OpenAI's list (it returns every model the key can use).
var openaiNonChat = regexp.MustCompile(`audio|realtime|tts|transcribe|whisper|dall-e|image|embedding|moderation|search|davinci|babbage|codex-mini|computer-use`)

// ListModels asks the provider which models the key can use, plus the ids the
// user typed in. Slugs are prefixed with the config's id.
func ListModels(ctx context.Context, hc *http.Client, c Config) ([]ai.Model, error) {
	k, _ := kind(c.Kind)
	var ids [][2]string // id, display name
	if k.listsModels {
		var err error
		if ids, err = fetchModels(ctx, hc, c, k); err != nil && len(c.Models) == 0 {
			return nil, err
		}
	}
	for _, m := range c.Models {
		ids = append(ids, [2]string{m, m})
	}
	seen := map[string]bool{}
	out := []ai.Model{}
	for _, id := range ids {
		if seen[id[0]] {
			continue
		}
		seen[id[0]] = true
		out = append(out, ai.Model{Slug: c.ID + ":" + id[0], DisplayName: id[1], Efforts: append([]ai.Effort{}, efforts[c.Kind]...)})
	}
	if out == nil {
		out = []ai.Model{}
	}
	return out, nil
}

func fetchModels(ctx context.Context, hc *http.Client, c Config, k Kind) ([][2]string, error) {
	u := strings.TrimRight(c.base(), "/") + k.modelsPath
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	switch c.Kind {
	case "anthropic":
		req.Header.Set("x-api-key", c.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "google":
		req.Header.Set("x-goog-api-key", c.APIKey)
	default:
		if c.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.APIKey)
		}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s is unreachable: %w", k.Name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, &ai.Error{Kind: ai.ErrInvalidKey, Message: k.Name + " rejected the API key", Status: resp.StatusCode}
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s answered HTTP %d listing models", k.Name, resp.StatusCode)
	}
	var out [][2]string
	switch c.Kind {
	case "google":
		var r struct {
			Models []struct {
				Name        string   `json:"name"`
				DisplayName string   `json:"displayName"`
				Methods     []string `json:"supportedGenerationMethods"`
			} `json:"models"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		for _, m := range r.Models {
			if slices.Contains(m.Methods, "generateContent") {
				out = append(out, [2]string{strings.TrimPrefix(m.Name, "models/"), m.DisplayName})
			}
		}
	case "ollama":
		var r struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		for _, m := range r.Models {
			out = append(out, [2]string{m.Name, m.Name})
		}
	default: // OpenAI-style {"data":[{"id","name"|"display_name"}]}
		var r struct {
			Data []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		for _, m := range r.Data {
			if c.Kind == "openai" && (openaiNonChat.MatchString(m.ID) || !(strings.HasPrefix(m.ID, "gpt") || strings.HasPrefix(m.ID, "chatgpt") || regexp.MustCompile(`^o\d`).MatchString(m.ID))) {
				continue
			}
			name := m.ID
			if m.DisplayName != "" {
				name = m.DisplayName
			} else if m.Name != "" {
				name = m.Name
			}
			out = append(out, [2]string{m.ID, name})
		}
	}
	slices.SortStableFunc(out, func(a, b [2]string) int { return strings.Compare(a[1], b[1]) })
	return out, nil
}
