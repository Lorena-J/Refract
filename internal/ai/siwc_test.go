package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ponytojas/Refract/internal/siwc"
)

type fakeTokens struct {
	creds  *siwc.Credentials
	forced int
}

func (f *fakeTokens) Credentials(context.Context) (*siwc.Credentials, error) { return f.creds, nil }
func (f *fakeTokens) AccessToken(_ context.Context, force bool) (string, error) {
	if force {
		f.forced++
		return "fresh", nil
	}
	return "stale-or-valid", nil
}

func signedIn() *fakeTokens {
	return &fakeTokens{creds: &siwc.Credentials{RefreshToken: "r", Scope: siwc.Scopes, Email: "u@example.com"}}
}

func sse(events ...string) string {
	var s string
	for _, e := range events {
		s += "event: x\ndata: " + e + "\n\n"
	}
	return s
}

func provider(t *testing.T, tokens *fakeTokens, h http.HandlerFunc) *SIWC {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &SIWC{Tokens: tokens, BaseURL: srv.URL, HTTP: srv.Client()}
}

func collect(t *testing.T, p *SIWC) ([]Event, error) {
	ch, err := p.Stream(context.Background(), Request{Model: "m", Input: []json.RawMessage{UserMessage("hi")}})
	if err != nil {
		return nil, err
	}
	var evs []Event
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs, nil
}

func TestStreamTextToolCallAndCompletion(t *testing.T) {
	var body map[string]any
	p := provider(t, signedIn(), func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer stale-or-valid" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, sse(
			`{"type":"response.created"}`,
			`{"type":"response.output_text.delta","delta":"Hel"}`,
			`{"type":"response.output_text.delta","delta":"lo"}`,
			`{"type":"response.output_item.done","item":{"type":"function_call","call_id":"c1","name":"get_time","arguments":"{}"}}`,
			`{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`,
		))
	})
	evs, err := collect(t, p)
	if err != nil {
		t.Fatal(err)
	}
	if body["store"] != false || body["stream"] != true || body["previous_response_id"] != nil {
		t.Fatalf("plan route requirements not met: %v", body)
	}
	if len(evs) != 4 || evs[0].Text+evs[1].Text != "Hello" {
		t.Fatalf("events = %+v", evs)
	}
	if c := evs[2].Call; c == nil || c.CallID != "c1" || c.Name != "get_time" {
		t.Fatalf("function call = %+v", evs[2])
	}
	if evs[3].Type != EventCompleted || evs[3].Usage.TotalTokens != 5 {
		t.Fatalf("last event = %+v", evs[3])
	}
}

func TestStreamFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   ErrorKind
	}{
		{"usage limit", 429, `{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":"limit"}}`, ErrUsageLimit},
		{"not eligible", 403, `{"error":{"code":"subscription_sharing_user_not_eligible","message":"no"}}`, ErrNotEligible},
		{"model", 400, `{"error":{"code":"subscription_sharing_unsupported_capability","param":"model","message":"m"}}`, ErrModelUnavailable},
		{"detail admission", 403, `{"detail":"region not supported"}`, ErrNotEligible},
		{"unavailable", 503, `{"detail":"try later"}`, ErrUnavailable},
		{"failed mid-stream", 200, sse(`{"type":"response.output_text.delta","delta":"a"}`, `{"type":"response.failed","response":{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":"limit"}}}`), ErrUsageLimit},
		{"interrupted", 200, sse(`{"type":"response.output_text.delta","delta":"a"}`), ErrUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := provider(t, signedIn(), func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				io.WriteString(w, c.body)
			})
			evs, err := collect(t, p)
			var got *Error
			if err != nil {
				got = err.(*Error)
			} else if last := evs[len(evs)-1]; last.Type == EventError {
				got = last.Err
			}
			if got == nil || got.Kind != c.want {
				t.Fatalf("got %v, want kind %s", got, c.want)
			}
			if limit := p.Status(context.Background()).LimitReached; limit != (c.want == ErrUsageLimit) {
				t.Fatalf("LimitReached = %v", limit)
			}
		})
	}
}

func TestUnauthorizedRefreshesOnceAndRetries(t *testing.T) {
	tokens := signedIn()
	calls := 0
	p := provider(t, tokens, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fresh" {
			w.WriteHeader(401)
			return
		}
		io.WriteString(w, sse(`{"type":"response.completed","response":{}}`))
	})
	evs, err := collect(t, p)
	if err != nil || evs[0].Type != EventCompleted || tokens.forced != 1 || calls != 2 {
		t.Fatalf("err=%v evs=%+v forced=%d calls=%d", err, evs, tokens.forced, calls)
	}
}

func TestNoCallWithoutPlanOrSignIn(t *testing.T) {
	cases := map[ErrorKind]*siwc.Credentials{
		ErrNotSignedIn:    nil,
		ErrPlanNotGranted: {RefreshToken: "r", Scope: "openid profile email offline_access"},
	}
	for want, creds := range cases {
		p := provider(t, &fakeTokens{creds: creds}, func(w http.ResponseWriter, r *http.Request) {
			t.Error("API called")
		})
		_, err := collect(t, p)
		if e, ok := err.(*Error); !ok || e.Kind != want {
			t.Errorf("err = %v, want %s", err, want)
		}
	}
}

func TestModelsKeepsListedOnly(t *testing.T) {
	p := provider(t, signedIn(), func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"models":[{"slug":"a","display_name":"A","visibility":"list"},{"slug":"b","visibility":"hide"}]}`)
	})
	ms, err := p.Models(context.Background())
	if err != nil || len(ms) != 1 || ms[0].Slug != "a" {
		t.Fatalf("models = %+v, %v", ms, err)
	}
}
