package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ponytojas/Refract/internal/ai"
	"github.com/ponytojas/Refract/internal/assistant"
	"github.com/ponytojas/Refract/internal/auth"
	"github.com/ponytojas/Refract/internal/quick"
	"github.com/ponytojas/Refract/internal/siwc"
)

type api struct {
	base  *siwc.Session // DB, key, OAuth and allowlist shared by every user's session
	model string        // default model (REFRACT_MODEL_AGENT)
	quick string        // default model for quick actions (REFRACT_MODEL_QUICK)

	users sync.Map // user id → *userAI
}

// userAI is one user's ChatGPT session and provider, kept for the life of the
// server: the session serializes that user's token refreshes, the provider
// remembers their usage-limit state.
type userAI struct {
	session  *siwc.Session
	provider *ai.SIWC
}

func (a *api) forUser(ctx context.Context) *userAI {
	u, _ := auth.UserFrom(ctx)
	if v, ok := a.users.Load(u.ID); ok {
		return v.(*userAI)
	}
	s := &siwc.Session{DB: a.base.DB, Key: a.base.Key, OAuth: a.base.OAuth, AllowedSubject: a.base.AllowedSubject, Owner: u.ID}
	v, _ := a.users.LoadOrStore(u.ID, &userAI{session: s, provider: &ai.SIWC{Tokens: s, BaseURL: siwc.Resource, HTTP: &http.Client{}}})
	return v.(*userAI)
}

// providerFor is the request user's AI provider.
func (a *api) providerFor(ctx context.Context) ai.Provider { return a.forUser(ctx).provider }

func (a *api) routes(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFrom(r.Context())
		writeJSON(w, u)
	})
	mux.HandleFunc("GET /api/chatgpt/status", a.status)
	mux.HandleFunc("POST /api/chatgpt/credentials", a.importCredentials)
	mux.HandleFunc("POST /api/chatgpt/signout", a.signOut)
	mux.HandleFunc("POST /api/chatgpt/welcome-seen", a.welcomeSeen)
	mux.HandleFunc("GET /api/chatgpt/models", a.models)
	mux.HandleFunc("POST /api/quick", a.quickAction)
	if dev {
		mux.HandleFunc("POST /api/dev/expire", a.devExpire)
		mux.HandleFunc("POST /api/dev/chat", a.devChat)
	}
}

func (a *api) status(w http.ResponseWriter, r *http.Request) {
	seen, err := a.forUser(r.Context()).session.WelcomeSeen(r.Context())
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"status":        a.forUser(r.Context()).provider.Status(r.Context()),
		"welcome_seen":  seen,
		"manage_usage":  siwc.ManageUsage,
		"default_model": a.model,
	})
}

func (a *api) importCredentials(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	if _, err := a.forUser(r.Context()).session.Import(r.Context(), raw); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	slog.Info("chatgpt credentials imported")
	a.status(w, r)
}

func (a *api) signOut(w http.ResponseWriter, r *http.Request) {
	if err := a.forUser(r.Context()).session.SignOut(r.Context()); err != nil {
		httpError(w, err, http.StatusBadGateway)
		return
	}
	a.status(w, r)
}

func (a *api) welcomeSeen(w http.ResponseWriter, r *http.Request) {
	if err := a.forUser(r.Context()).session.MarkWelcomeSeen(r.Context()); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) models(w http.ResponseWriter, r *http.Request) {
	ms, err := a.forUser(r.Context()).provider.Models(r.Context())
	if err != nil {
		var e *ai.Error
		if errors.As(err, &e) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]any{"error": e})
			return
		}
		httpError(w, err, http.StatusBadGateway)
		return
	}
	writeJSON(w, ms)
}

// choose resolves a model pick against the plan's models: the UI's pick, else
// the configured model, else the plan's first. Unknown efforts fall back to
// the model's default.
func (a *api) choose(ctx context.Context, want assistant.Choice, configured string) (assistant.Choice, error) {
	ms, err := a.forUser(ctx).provider.Models(ctx)
	if err != nil {
		return assistant.Choice{}, err
	}
	slug := cmp.Or(want.Model, configured)
	m, effort, ok := ai.Pick(ms, slug, want.Effort)
	switch {
	case ok:
		return assistant.Choice{Model: m.Slug, Effort: effort}, nil
	case want.Model == "" && slug != "":
		return assistant.Choice{Model: slug}, nil // configured, possibly unlisted: trust it
	case len(ms) == 0:
		return assistant.Choice{}, errors.New("no models available on this ChatGPT plan; set REFRACT_MODEL_AGENT")
	default:
		return assistant.Choice{}, fmt.Errorf("model %q is not available on this ChatGPT plan", want.Model)
	}
}

// quickAction runs one quick action on a selection and returns the
// replacement text (or, for "explain", the explanation).
func (a *api) quickAction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		quick.Request
		Model string `json:"model"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&in); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	if err := in.Validate(); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	// Quick actions should feel instant: light reasoning whatever the composer uses.
	c, err := a.choose(r.Context(), assistant.Choice{Model: in.Model, Effort: "low"}, a.quick)
	if err == nil {
		var text string
		if text, err = quick.Run(r.Context(), a.providerFor(r.Context()), c.Model, c.Effort, in.Request); err == nil {
			writeJSON(w, map[string]string{"text": text})
			return
		}
	}
	if r.Context().Err() != nil {
		return // the user cancelled
	}
	var ae *ai.Error
	if !errors.As(err, &ae) {
		ae = &ai.Error{Kind: ai.ErrOther, Message: err.Error()}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(w).Encode(map[string]any{"error": ae})
}

func (a *api) devExpire(w http.ResponseWriter, r *http.Request) {
	if err := a.forUser(r.Context()).session.ExpireNow(r.Context()); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var devTimeTool = ai.Tool{
	Type:        "function",
	Name:        "get_server_time",
	Description: "Returns the current time on the Refract server.",
	Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
}

// devChat streams one prompt, optionally with a function tool, running the
// tool round-trip the agent loop (M6) will need: replay output items plus
// function_call_output in the next stateless call.
func (a *api) devChat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Tools  bool   `json:"tools"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil || in.Prompt == "" {
		httpError(w, errors.New("prompt required"), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		rc.Flush()
	}

	req := ai.Request{
		Model:        in.Model,
		Instructions: "You are testing Refract's connection. Answer briefly.",
		Input:        []json.RawMessage{ai.UserMessage(in.Prompt)},
	}
	if req.Model == "" {
		req.Model = a.model
	}
	if in.Tools {
		req.Tools = []ai.Tool{devTimeTool}
	}

	for range 5 {
		ch, err := a.providerFor(r.Context()).Stream(r.Context(), req)
		if err != nil {
			send(map[string]any{"type": "error", "error": err})
			return
		}
		called := false
		for ev := range ch {
			switch ev.Type {
			case ai.EventText:
				send(map[string]any{"type": "text", "text": ev.Text})
			case ai.EventItem:
				req.Input = append(req.Input, ev.Item)
				if ev.Call != nil {
					called = true
					result := time.Now().Format(time.RFC3339)
					req.Input = append(req.Input, ai.FunctionOutput(ev.Call.CallID, result))
					send(map[string]any{"type": "tool", "name": ev.Call.Name, "arguments": ev.Call.Arguments, "result": result})
				}
			case ai.EventCompleted:
				if !called {
					send(map[string]any{"type": "done", "usage": ev.Usage})
					return
				}
			case ai.EventError:
				send(map[string]any{"type": "error", "error": ev.Err})
				return
			}
		}
	}
	send(map[string]any{"type": "error", "error": ai.Error{Kind: ai.ErrOther, Message: "too many tool rounds"}})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"kind": "other", "message": err.Error()}})
}
