// Package providers adds AI providers that work with an API key (OpenAI,
// Anthropic, Gemini, OpenRouter, local models…) next to the ChatGPT plan.
// Calls go through GoAI (github.com/zendev-sh/goai) behind our ai.Provider,
// so the agent, quick actions and citations work with any of them.
package providers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"syscall"
	"time"

	"github.com/ponytojas/Refract/internal/store"
)

// Kind is a provider Refract knows how to call.
type Kind struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	NeedsKey    bool   `json:"needs_key"`
	BaseURL     string `json:"base_url"`     // default; editable
	NeedsBase   bool   `json:"needs_base"`   // no useful default (custom endpoints)
	KeyHelpURL  string `json:"key_help_url"` // where to create a key
	modelsPath  string // relative to the base URL
	listsModels bool
}

// Kinds, in the order the UI offers them.
var Kinds = []Kind{
	{ID: "openai", Name: "OpenAI", NeedsKey: true, BaseURL: "https://api.openai.com/v1", KeyHelpURL: "https://platform.openai.com/api-keys", modelsPath: "/models", listsModels: true},
	{ID: "anthropic", Name: "Anthropic", NeedsKey: true, BaseURL: "https://api.anthropic.com", KeyHelpURL: "https://console.anthropic.com/settings/keys", modelsPath: "/v1/models", listsModels: true},
	{ID: "google", Name: "Google Gemini", NeedsKey: true, BaseURL: "https://generativelanguage.googleapis.com", KeyHelpURL: "https://aistudio.google.com/apikey", modelsPath: "/v1beta/models", listsModels: true},
	{ID: "openrouter", Name: "OpenRouter", NeedsKey: true, BaseURL: "https://openrouter.ai/api/v1", KeyHelpURL: "https://openrouter.ai/keys", modelsPath: "/models", listsModels: true},
	{ID: "mistral", Name: "Mistral", NeedsKey: true, BaseURL: "https://api.mistral.ai/v1", KeyHelpURL: "https://console.mistral.ai/api-keys", modelsPath: "/models", listsModels: true},
	{ID: "deepseek", Name: "DeepSeek", NeedsKey: true, BaseURL: "https://api.deepseek.com", KeyHelpURL: "https://platform.deepseek.com/api_keys", modelsPath: "/models", listsModels: true},
	{ID: "xai", Name: "xAI", NeedsKey: true, BaseURL: "https://api.x.ai/v1", KeyHelpURL: "https://console.x.ai", modelsPath: "/models", listsModels: true},
	{ID: "groq", Name: "Groq", NeedsKey: true, BaseURL: "https://api.groq.com/openai/v1", KeyHelpURL: "https://console.groq.com/keys", modelsPath: "/models", listsModels: true},
	{ID: "ollama", Name: "Ollama", BaseURL: "http://localhost:11434", modelsPath: "/api/tags", listsModels: true},
	{ID: "compat", Name: "OpenAI-compatible", NeedsBase: true, modelsPath: "/models", listsModels: true},
}

func kind(id string) (Kind, bool) {
	for _, k := range Kinds {
		if k.ID == id {
			return k, true
		}
	}
	return Kind{}, false
}

// Config is one provider a user set up. ID prefixes its models ("anthropic:claude-…").
type Config struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	APIKey  string   `json:"api_key,omitempty"` // never sent to the client
	BaseURL string   `json:"base_url,omitempty"`
	Models  []string `json:"models,omitempty"` // extra model ids, for endpoints that can't list them
}

func (c Config) base() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	k, _ := kind(c.Kind)
	return k.BaseURL
}

var configID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// Validate checks a config at the API boundary.
func (c Config) Validate() error {
	k, ok := kind(c.Kind)
	if !ok {
		return fmt.Errorf("unknown provider %q", c.Kind)
	}
	if !configID.MatchString(c.ID) {
		return errors.New("provider ids use lowercase letters, digits and '-'")
	}
	if c.BaseURL != "" || k.NeedsBase {
		u, err := url.Parse(c.base())
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return errors.New("the base URL must be an http(s) URL without credentials")
		}
	}
	for _, m := range c.Models {
		if m == "" || len(m) > 200 {
			return errors.New("invalid model id")
		}
	}
	return nil
}

// Store keeps each user's configs, sealed (API keys are secrets).
type Store struct {
	DB  *sql.DB
	Key []byte
}

func storeKey(user string) string { return store.UserKey(user, "providers") }

func (s *Store) List(ctx context.Context, user string) ([]Config, error) {
	sealed, err := store.GetSetting(ctx, s.DB, storeKey(user))
	if err != nil || sealed == "" {
		return []Config{}, err
	}
	plain, err := store.Unseal(s.Key, sealed, storeKey(user))
	if err != nil {
		return nil, err
	}
	var cs []Config
	return cs, json.Unmarshal(plain, &cs)
}

func (s *Store) write(ctx context.Context, user string, cs []Config) error {
	plain, _ := json.Marshal(cs)
	sealed, err := store.Seal(s.Key, plain, storeKey(user))
	if err != nil {
		return err
	}
	return store.SetSetting(ctx, s.DB, storeKey(user), sealed)
}

// Save adds or replaces a config. An empty API key keeps the stored one.
func (s *Store) Save(ctx context.Context, user string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	cs, err := s.List(ctx, user)
	if err != nil {
		return err
	}
	for i, old := range cs {
		if old.ID == c.ID {
			if c.APIKey == "" {
				c.APIKey = old.APIKey
			}
			cs[i] = c
			return s.write(ctx, user, cs)
		}
	}
	if k, _ := kind(c.Kind); k.NeedsKey && c.APIKey == "" {
		return errors.New("this provider needs an API key")
	}
	return s.write(ctx, user, append(cs, c))
}

func (s *Store) Delete(ctx context.Context, user, id string) error {
	cs, err := s.List(ctx, user)
	if err != nil {
		return err
	}
	out := cs[:0]
	for _, c := range cs {
		if c.ID != id {
			out = append(out, c)
		}
	}
	return s.write(ctx, user, out)
}

// HTTPClient is for calls to provider endpoints. Base URLs are user-supplied,
// so link-local addresses (cloud metadata services such as 169.254.169.254)
// are refused. Private and loopback addresses stay allowed: that is where
// local models (Ollama, LM Studio) live.
func HTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip != nil && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
				return fmt.Errorf("refusing to connect to link-local address %s", ip)
			}
			return nil
		},
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = dialer.DialContext
	return &http.Client{Transport: t, Timeout: 10 * time.Minute} // long: reasoning streams
}
