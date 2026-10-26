package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeviceFlowAndRepos(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.URL.Path {
		case "/login/device/code":
			if r.Form.Get("client_id") != "cid" || r.Form.Get("scope") != "repo" {
				t.Errorf("device request = %v", r.Form)
			}
			w.Write([]byte(`{"device_code":"dc","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`))
		case "/login/oauth/access_token":
			if polls++; polls == 1 {
				w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			w.Write([]byte(`{"access_token":"gho_x","token_type":"bearer"}`))
		case "/user":
			if r.Header.Get("Authorization") != "Bearer gho_x" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"login":"octo"}`))
		case "/user/repos":
			if r.Method == "POST" {
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"full_name":"octo/thesis","clone_url":"https://github.com/octo/thesis.git","private":true}`))
				return
			}
			w.Write([]byte(`[{"full_name":"octo/a","clone_url":"https://github.com/octo/a.git","size":0},{"full_name":"octo/b","clone_url":"https://github.com/octo/b.git","size":12}]`))
		}
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), ClientID: "cid", Web: srv.URL, API: srv.URL}
	ctx := context.Background()

	d, err := c.StartDevice(ctx)
	if err != nil || d.UserCode != "ABCD-1234" {
		t.Fatalf("device = %+v, %v", d, err)
	}
	if _, err := c.PollDevice(ctx, d.DeviceCode); !errors.Is(err, ErrPending) {
		t.Fatalf("first poll = %v", err)
	}
	tok, err := c.PollDevice(ctx, d.DeviceCode)
	if err != nil || tok != "gho_x" {
		t.Fatalf("token = %q, %v", tok, err)
	}
	if u, err := c.User(ctx, tok); err != nil || u.Login != "octo" {
		t.Fatalf("user = %+v, %v", u, err)
	}
	if _, err := c.User(ctx, "bad"); err == nil {
		t.Error("bad token accepted")
	}
	rs, err := c.Repos(ctx, tok)
	if err != nil || len(rs) != 2 || !rs[0].Empty || rs[1].Empty {
		t.Fatalf("repos = %+v, %v", rs, err)
	}
	if r, err := c.CreateRepo(ctx, tok, "thesis", true); err != nil || r.CloneURL != "https://github.com/octo/thesis.git" {
		t.Fatalf("create = %+v, %v", r, err)
	}
}
