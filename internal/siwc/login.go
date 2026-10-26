package siwc

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// NewHostID returns a fresh ext_agent_host_id (urn:uuid, UUIDv4). Generate once per host and persist.
func NewHostID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// LoginOptions configures the local loopback sign-in.
type LoginOptions struct {
	HostID       string
	Saved        *Credentials // previous registration for this account: reuse its client id
	Port         int          // 0 = DefaultPort, negative = any free port
	OpenBrowser  func(authURL string)
	OAuth        *OAuth
	AuthorizeURL string // "" = real endpoint
}

// Login runs the browser sign-in on a 127.0.0.1 listener and returns validated credentials.
func Login(ctx context.Context, o LoginOptions) (*Credentials, error) {
	if o.HostID == "" {
		return nil, errors.New("login: host id required")
	}
	port := max(cmp.Or(o.Port, DefaultPort), 0) // negative: any free port (tests)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("login: listen on 127.0.0.1 (is another login running?): %w", err)
	}
	defer ln.Close()
	// The docs require 127.0.0.1, never localhost; only the port may vary.
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", ln.Addr().(*net.TCPAddr).Port, CallbackPath)

	verifier, state, nonce := randToken(), randToken(), randToken()
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {RegisterID},
		"redirect_uri":          {redirectURI},
		"scope":                 {Scopes},
		"resource":              {Resource},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"ext_agent_host_id":     {o.HostID},
	}
	if o.Saved != nil {
		q.Set("client_id", o.Saved.ClientID)
		if o.Saved.IDToken != "" {
			q.Set("id_token_hint", o.Saved.IDToken)
		}
		if o.Saved.Email != "" {
			q.Set("login_hint", o.Saved.Email)
		}
	} else {
		q.Set("agent_name_hint", agentNameHint)
	}

	type result struct {
		code, clientID string
		err            error
	}
	done := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query()
		if subtle.ConstantTimeCompare([]byte(p.Get("state")), []byte(state)) != 1 {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return // ignore stray requests; keep waiting for the real callback
		}
		var res result
		switch {
		case p.Get("error") == "access_denied":
			res.err = errors.New("sign-in was cancelled or denied")
		case p.Get("error") != "":
			res.err = fmt.Errorf("authorization failed: %s %s", p.Get("error"), p.Get("error_description"))
		case p.Get("code") == "":
			res.err = errors.New("callback without code")
		default:
			res = result{code: p.Get("code"), clientID: p.Get("client_id")}
		}
		if res.err != nil {
			http.Error(w, res.err.Error()+". You can close this tab.", http.StatusBadRequest)
		} else {
			fmt.Fprint(w, "Signed in. You can close this tab and return to the terminal.")
		}
		select {
		case done <- res:
		default:
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()

	o.OpenBrowser(cmp.Or(o.AuthorizeURL, AuthorizeURL) + "?" + q.Encode())

	var res result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res = <-done:
	}
	if res.err != nil {
		return nil, res.err
	}

	clientID := res.clientID
	if o.Saved != nil {
		// Reauthorization may omit client_id; a different one means a mixed-up registration.
		if clientID != "" && clientID != o.Saved.ClientID {
			return nil, fmt.Errorf("callback returned client id %q, expected %q", clientID, o.Saved.ClientID)
		}
		clientID = o.Saved.ClientID
	} else if clientID == "" {
		return nil, errors.New("registration callback did not include the issued client_id")
	}

	tr, err := o.OAuth.Exchange(ctx, clientID, res.code, verifier, redirectURI)
	if err != nil {
		return nil, err
	}
	idt, err := o.OAuth.VerifyIDToken(ctx, clientID, tr.IDToken, false)
	if err != nil {
		return nil, fmt.Errorf("id token: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(nonce)) != 1 {
		return nil, errors.New("id token: nonce mismatch")
	}
	if o.Saved != nil && o.Saved.Subject != "" && idt.Subject != o.Saved.Subject {
		return nil, errors.New("signed in with a different ChatGPT account than the saved one; use -new to register it separately")
	}
	var claims struct {
		Email string `json:"email"`
	}
	idt.Claims(&claims)

	c := &Credentials{ClientID: clientID, Subject: idt.Subject, Email: claims.Email}
	c.apply(tr, time.Now())
	return c, nil
}

func randToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
