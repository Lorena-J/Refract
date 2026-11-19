package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostPolicyStartup(t *testing.T) {
	loopback4 := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
	loopback6 := &net.TCPAddr{IP: net.IPv6loopback, Port: 8080}
	wildcard := &net.TCPAddr{IP: net.IPv6unspecified, Port: 8080}
	lan := &net.TCPAddr{IP: net.IPv4(192, 168, 1, 20), Port: 8080}
	for _, tc := range []struct {
		addr                   net.Addr
		auth, publicURL, hosts string
		wantErr                string // part of the refusal; empty means it starts
		wantAny                bool   // accepts any Host, with a warning
	}{
		{loopback4, "none", "", "", "", false},
		{loopback6, "none", "", "", "", false},
		// No sign-in on an address others can reach: only with explicit names.
		{wildcard, "none", "", "", "REFRACT_AUTH=none", false},
		{lan, "none", "", "", "REFRACT_AUTH=none", false},
		{wildcard, "none", "", "localhost", "", false},
		{wildcard, "none", "https://r.example.com", "", "", false},
		// Signed-in modes start anyway, warning that any name is accepted.
		{wildcard, "builtin", "", "", "", true},
		{wildcard, "cloudflare", "", "", "", true},
		{wildcard, "builtin", "https://r.example.com", "", "", false},
		{wildcard, "builtin", "", "refract.lan, 192.168.1.20", "", false},
		{wildcard, "builtin", "", "refract.lan:8080, [fd00::1], fd00::2, [fd00::3]:80, REFRACT.LAN.", "", false},
		// Invalid values stop startup, even on loopback, and never count as configured.
		{loopback4, "none", "", "https://refract.lan", "REFRACT_ALLOWED_HOSTS", false},
		{wildcard, "none", "", "[localhost]", "REFRACT_ALLOWED_HOSTS", false},
		{wildcard, "none", "", "[127.0.0.1]", "REFRACT_ALLOWED_HOSTS", false},
		{wildcard, "none", "", "[fd00::1", "REFRACT_ALLOWED_HOSTS", false},
		{wildcard, "none", "", "refract.lan:http", "REFRACT_ALLOWED_HOSTS", false},
		{wildcard, "builtin", "", "refract.lan/app", "REFRACT_ALLOWED_HOSTS", false},
		{wildcard, "builtin", "", "*.example.com", "REFRACT_ALLOWED_HOSTS", false},
		{wildcard, "builtin", "", "-bad.example", "REFRACT_ALLOWED_HOSTS", false},
		{wildcard, "none", "//localhost", "", "REFRACT_PUBLIC_URL", false},
		{wildcard, "builtin", "not a url", "", "REFRACT_PUBLIC_URL", false},
		{wildcard, "builtin", "https://user@r.example.com", "", "REFRACT_PUBLIC_URL", false},
	} {
		name := fmt.Sprintf("%s %s %q %q", tc.addr, tc.auth, tc.publicURL, tc.hosts)
		p, err := newHostPolicy(tc.publicURL, tc.hosts)
		var warning string
		if err == nil {
			warning, err = p.forListener(tc.addr, tc.auth)
		}
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%s: err = %v, want one mentioning %q", name, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if p.any != tc.wantAny || (warning != "") != tc.wantAny {
			t.Errorf("%s: any = %v, warning = %q", name, p.any, warning)
		}
	}
}

func policy(t *testing.T, addr net.Addr, auth, publicURL, hosts string) hostPolicy {
	t.Helper()
	p, err := newHostPolicy(publicURL, hosts)
	if err == nil {
		_, err = p.forListener(addr, auth)
	}
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHostPolicyRequests(t *testing.T) {
	strict := policy(t, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, "none", "https://r.example.com", "refract.lan, 192.168.1.20, [fd00::1]")
	open := policy(t, &net.TCPAddr{IP: net.IPv4zero}, "builtin", "", "")
	for _, tc := range []struct {
		policy       hostPolicy
		method, host string
		origins      []string // nil sends no Origin
		want         int
	}{
		// A rebound name is refused.
		{strict, "GET", "evil.example", nil, http.StatusMisdirectedRequest},
		{strict, "GET", "evil.example:8080", nil, http.StatusMisdirectedRequest},
		{strict, "POST", "evil.example", nil, http.StatusMisdirectedRequest},
		{strict, "GET", "localhost.evil.example", nil, http.StatusMisdirectedRequest},
		{strict, "GET", "2130706433", nil, http.StatusMisdirectedRequest}, // 127.0.0.1 as a number

		{strict, "GET", "localhost:8080", nil, http.StatusOK},
		{strict, "GET", "LOCALHOST.", nil, http.StatusOK},
		{strict, "GET", "127.0.0.1:8080", nil, http.StatusOK},
		{strict, "GET", "[::1]:8080", nil, http.StatusOK},
		{strict, "GET", "app.localhost:8080", nil, http.StatusOK},
		{strict, "GET", "r.example.com", nil, http.StatusOK},
		{strict, "GET", "refract.lan:8080", nil, http.StatusOK},
		{strict, "GET", "192.168.1.20:8080", nil, http.StatusOK},
		{strict, "GET", "[fd00::1]:8080", nil, http.StatusOK},

		// Malformed Host headers are refused, whatever the policy.
		{strict, "GET", "localhost:weird", nil, http.StatusBadRequest},
		{strict, "GET", "[localhost]", nil, http.StatusBadRequest},
		{strict, "GET", "[::1", nil, http.StatusBadRequest},
		{strict, "GET", "[::1]x", nil, http.StatusBadRequest},
		{open, "GET", "localhost:weird", nil, http.StatusBadRequest},

		// Changes: a foreign Origin is refused; a matching one, or none, passes.
		{strict, "POST", "localhost:8080", []string{"https://evil.example"}, http.StatusForbidden},
		{strict, "POST", "localhost:8080", []string{"null"}, http.StatusForbidden},
		{strict, "DELETE", "localhost:8080", []string{"http://evil.example:8080"}, http.StatusForbidden},
		{strict, "POST", "localhost:8080", []string{"http://localhost.evil.example"}, http.StatusForbidden},
		{strict, "POST", "localhost:8080", []string{"http://localhost:8080"}, http.StatusOK},
		{strict, "POST", "localhost:8080", []string{"http://localhost:5173"}, http.StatusOK}, // Vite in development
		{strict, "POST", "r.example.com", []string{"https://r.example.com"}, http.StatusOK},
		{strict, "POST", "localhost:8080", nil, http.StatusOK},
		{strict, "GET", "localhost:8080", []string{"https://evil.example"}, http.StatusOK},
		// Malformed or ambiguous Origin headers are refused.
		{strict, "POST", "localhost:8080", []string{""}, http.StatusForbidden},
		{strict, "POST", "localhost:8080", []string{"//localhost"}, http.StatusForbidden},
		{strict, "POST", "localhost:8080", []string{"http://user@localhost"}, http.StatusForbidden},
		{strict, "POST", "localhost:8080", []string{"http://localhost/path"}, http.StatusForbidden},
		{strict, "POST", "localhost:8080", []string{"ftp://localhost"}, http.StatusForbidden},
		{strict, "POST", "localhost:8080", []string{"http://localhost:8080", "https://evil.example"}, http.StatusForbidden},
		{strict, "POST", "localhost:8080", []string{"http://localhost:8080", "http://localhost:8080"}, http.StatusForbidden},

		// Any host accepted: the Origin must still be the host asked for.
		{open, "GET", "refract.anything", nil, http.StatusOK},
		{open, "POST", "refract.anything", []string{"https://refract.anything"}, http.StatusOK},
		{open, "POST", "refract.anything", []string{"https://evil.example"}, http.StatusForbidden},
		{open, "POST", "refract.anything", []string{"http://localhost"}, http.StatusForbidden},
	} {
		h := tc.policy.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		req := httptest.NewRequest(tc.method, "/api/projects", nil)
		req.Host = tc.host
		for _, o := range tc.origins {
			req.Header.Add("Origin", o)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s Host=%s Origin=%q: %d, want %d (%s)", tc.method, tc.host, tc.origins, rec.Code, tc.want, rec.Body)
		}
	}
}

// TestHostPolicyServer sends raw requests to a real server built like the
// production one: no route, and no form of request, skips the check.
func TestHostPolicyServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/api/auth/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("auth")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("app")) })
	srv := newServer(mux, policy(t, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, "none", "", ""))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	for _, tc := range []struct {
		target, host string
		method       string
		want         int
	}{
		{"/healthz", "evil.example", "GET", http.StatusMisdirectedRequest},
		{"/api/auth/config", "evil.example", "GET", http.StatusMisdirectedRequest},
		{"/", "evil.example", "GET", http.StatusMisdirectedRequest},
		{"*", "evil.example", "OPTIONS", http.StatusMisdirectedRequest},
		{"/healthz", "localhost", "GET", http.StatusOK},
		{"/api/auth/config", "127.0.0.1", "GET", http.StatusOK},
	} {
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(conn, "%s %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", tc.method, tc.target, tc.host)
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		conn.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("%s %s Host=%s: %d, want %d", tc.method, tc.target, tc.host, resp.StatusCode, tc.want)
		}
		if resp.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s %s Host=%s: no security headers", tc.method, tc.target, tc.host)
		}
	}
}
