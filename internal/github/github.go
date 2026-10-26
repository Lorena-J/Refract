// Package github connects a GitHub account with the OAuth device flow (no
// client secret, no callback URL: works on any self-hosted server) and
// lists or creates repositories to use as project remotes.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client needs the client ID of a GitHub OAuth App with device flow enabled.
type Client struct {
	HTTP     *http.Client
	ClientID string
	Web      string // https://github.com
	API      string // https://api.github.com
}

// DeviceCode is what the user enters at VerificationURI.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

var (
	ErrPending = errors.New("authorization pending")
	ErrExpired = errors.New("the code expired: start again")
	ErrDenied  = errors.New("access was denied on GitHub")
)

// StartDevice asks GitHub for a user code. The "repo" scope lets Refract push
// to private repositories and create them.
func (c *Client) StartDevice(ctx context.Context) (*DeviceCode, error) {
	var d DeviceCode
	if err := c.form(ctx, "/login/device/code", url.Values{"client_id": {c.ClientID}, "scope": {"repo"}}, &d); err != nil {
		return nil, err
	}
	if d.DeviceCode == "" {
		return nil, errors.New("GitHub did not return a device code: check that device flow is enabled for the OAuth App")
	}
	return &d, nil
}

// PollDevice returns the token once the user approved, or ErrPending.
func (c *Client) PollDevice(ctx context.Context, deviceCode string) (string, error) {
	var r struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	err := c.form(ctx, "/login/oauth/access_token", url.Values{
		"client_id":   {c.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}, &r)
	switch {
	case err != nil:
		return "", err
	case r.AccessToken != "":
		return r.AccessToken, nil
	case r.Error == "authorization_pending" || r.Error == "slow_down":
		return "", ErrPending
	case r.Error == "expired_token":
		return "", ErrExpired
	case r.Error == "access_denied":
		return "", ErrDenied
	}
	return "", fmt.Errorf("GitHub: %s", r.Error)
}

func (c *Client) form(ctx context.Context, path string, v url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.Web+path, strings.NewReader(v.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return c.do(req, out)
}

// User is the connected account.
type User struct {
	Login string `json:"login"`
}

func (c *Client) User(ctx context.Context, token string) (*User, error) {
	var u User
	return &u, c.api(ctx, token, "GET", "/user", nil, &u)
}

// Repo is a repository the account can push to.
type Repo struct {
	FullName  string `json:"full_name"`
	CloneURL  string `json:"clone_url"`
	Private   bool   `json:"private"`
	UpdatedAt string `json:"updated_at"`
	Empty     bool   `json:"empty"`
	Size      int    `json:"size"`
}

// Repos lists the account's repositories, most recently updated first.
func (c *Client) Repos(ctx context.Context, token string) ([]Repo, error) {
	var rs []Repo
	if err := c.api(ctx, token, "GET", "/user/repos?per_page=100&sort=updated&affiliation=owner,collaborator", nil, &rs); err != nil {
		return nil, err
	}
	for i := range rs {
		rs[i].Empty = rs[i].Size == 0
	}
	return rs, nil
}

// CreateRepo makes an empty repository on the account.
func (c *Client) CreateRepo(ctx context.Context, token, name string, private bool) (*Repo, error) {
	var r Repo
	body, _ := json.Marshal(map[string]any{"name": name, "private": private, "auto_init": false})
	return &r, c.api(ctx, token, "POST", "/user/repos", body, &r)
}

func (c *Client) api(ctx context.Context, token, method, path string, body []byte, out any) error {
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.API+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	req.Header.Set("User-Agent", "Refract")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub is unreachable: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		json.Unmarshal(b, &e)
		if resp.StatusCode == http.StatusUnauthorized {
			return errors.New("GitHub rejected the token: connect the account again")
		}
		return fmt.Errorf("GitHub answered HTTP %d: %s", resp.StatusCode, e.Message)
	}
	return json.Unmarshal(b, out)
}
