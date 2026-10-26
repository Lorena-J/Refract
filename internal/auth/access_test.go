package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

const testIssuer = "https://team.cloudflareaccess.com"

func signJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	jws, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	s, err := jws.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAccessMiddleware(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	keys := &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{key.Public()}}
	h := accessMiddleware(oidc.NewVerifier(testIssuer, keys, &oidc.Config{ClientID: "app-aud"}))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	valid := map[string]any{"iss": testIssuer, "aud": []string{"app-aud"}, "exp": time.Now().Add(time.Hour).Unix(), "sub": "u"}
	with := func(k string, v any) map[string]any {
		c := map[string]any{}
		for kk, vv := range valid {
			c[kk] = vv
		}
		c[k] = v
		return c
	}

	cases := []struct {
		name   string
		header string
		cookie string
		want   int
	}{
		{"valid header", signJWT(t, key, valid), "", 200},
		{"valid cookie", "", signJWT(t, key, valid), 200},
		{"missing", "", "", 403},
		{"wrong aud", signJWT(t, key, with("aud", []string{"other"})), "", 403},
		{"wrong issuer", signJWT(t, key, with("iss", "https://evil.example")), "", 403},
		{"expired", signJWT(t, key, with("exp", time.Now().Add(-time.Hour).Unix())), "", 403},
		{"wrong key", signJWT(t, other, valid), "", 403},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if c.header != "" {
				r.Header.Set("Cf-Access-Jwt-Assertion", c.header)
			}
			if c.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "CF_Authorization", Value: c.cookie})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
		})
	}
}

func TestRequireCSRFHeader(t *testing.T) {
	h := RequireCSRFHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cases := []struct {
		path   string
		header bool
		want   int
	}{
		{"/api/projects", false, 403},
		{"/api/projects", true, 200},
		{"/", false, 200}, // static UI needs no header
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", c.path, nil)
		if c.header {
			r.Header.Set(CSRFHeader, "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s header=%v: status = %d, want %d", c.path, c.header, w.Code, c.want)
		}
	}
}
