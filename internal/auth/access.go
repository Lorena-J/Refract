// Package auth guards the web UI and API.
package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// CSRFHeader must be sent on every /api request. Browsers can't add custom
// headers to cross-site requests without a CORS preflight, which we never allow.
const CSRFHeader = "X-Refract"

// CloudflareAccess returns middleware that requires a valid Cloudflare Access
// JWT. teamDomain is e.g. "myteam.cloudflareaccess.com"; aud is the
// application's AUD tag. Checking it here means reaching the origin directly
// does not bypass Access.
func CloudflareAccess(ctx context.Context, teamDomain, aud string) func(http.Handler) http.Handler {
	issuer := "https://" + teamDomain
	keys := oidc.NewRemoteKeySet(ctx, issuer+"/cdn-cgi/access/certs")
	return accessMiddleware(oidc.NewVerifier(issuer, keys, &oidc.Config{ClientID: aud}))
}

func accessMiddleware(v *oidc.IDTokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Cf-Access-Jwt-Assertion")
			if token == "" {
				if c, err := r.Cookie("CF_Authorization"); err == nil {
					token = c.Value
				}
			}
			if token == "" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if _, err := v.Verify(r.Context(), token); err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireCSRFHeader rejects /api requests that lack CSRFHeader.
func RequireCSRFHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get(CSRFHeader) == "" {
			http.Error(w, "missing "+CSRFHeader+" header", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
