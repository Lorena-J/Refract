package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"refract/internal/ai"
	"refract/internal/siwc"
)

type api struct {
	session  *siwc.Session
	provider *ai.SIWC
	model    string // default model (REFRACT_MODEL_AGENT)
}

func (a *api) routes(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/chatgpt/status", a.status)
	mux.HandleFunc("POST /api/chatgpt/credentials", a.importCredentials)
	mux.HandleFunc("POST /api/chatgpt/signout", a.signOut)
	mux.HandleFunc("POST /api/chatgpt/welcome-seen", a.welcomeSeen)
	mux.HandleFunc("GET /api/chatgpt/models", a.models)
	if dev {
		mux.HandleFunc("POST /api/dev/expire", a.devExpire)
		mux.HandleFunc("POST /api/dev/chat", a.devChat)
	}
}

func (a *api) status(w http.ResponseWriter, r *http.Request) {
	seen, err := a.session.WelcomeSeen(r.Context())
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"status":        a.provider.Status(r.Context()),
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
	if _, err := a.session.Import(r.Context(), raw); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	slog.Info("chatgpt credentials imported")
	a.status(w, r)
}

func (a *api) signOut(w http.ResponseWriter, r *http.Request) {
	if err := a.session.SignOut(r.Context()); err != nil {
		httpError(w, err, http.StatusBadGateway)
		return
	}
	a.status(w, r)
}

func (a *api) welcomeSeen(w http.ResponseWriter, r *http.Request) {
	if err := a.session.MarkWelcomeSeen(r.Context()); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) models(w http.ResponseWriter, r *http.Request) {
	ms, err := a.provider.Models(r.Context())
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

func (a *api) devExpire(w http.ResponseWriter, r *http.Request) {
	if err := a.session.ExpireNow(r.Context()); err != nil {
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
		ch, err := a.provider.Stream(r.Context(), req)
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
