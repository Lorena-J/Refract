package users

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ponytojas/Refract/internal/auth"
	"github.com/ponytojas/Refract/internal/store"
)

func newService(t *testing.T) *Service {
	db, err := store.Open(filepath.Join(t.TempDir(), "u.db"))
	if err != nil {
		t.Fatal(err)
	}
	return &Service{DB: db, Key: make([]byte, 32)}
}

func TestPasswordLogin(t *testing.T) {
	s, ctx := newService(t), context.Background()
	if _, err := s.SetPassword(ctx, "ana@example.com", "short"); err == nil {
		t.Fatal("accepted a short password")
	}
	if _, err := s.SetPassword(ctx, "local", "long enough"); err == nil {
		t.Fatal("accepted a non-email account")
	}
	u, err := s.SetPassword(ctx, " Ana@Example.com ", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	s.DB.QueryRow(`SELECT password FROM users WHERE id = ?`, u.ID).Scan(&stored)
	if stored == "" || stored == "correct horse" {
		t.Fatalf("stored %q", stored)
	}
	if got, err := s.Login(ctx, "ANA@example.com", "correct horse"); err != nil || got.ID != u.ID {
		t.Fatalf("login: %v %+v", err, got)
	}
	for _, c := range [][2]string{{"ana@example.com", "wrong"}, {"nobody@example.com", "correct horse"}} {
		if _, err := s.Login(ctx, c[0], c[1]); !errors.Is(err, ErrBadLogin) {
			t.Fatalf("%v: got %v", c, err)
		}
	}
	if has, _ := s.HasPassword(ctx, u.ID); !has {
		t.Fatal("HasPassword")
	}
}

func TestLoginLimit(t *testing.T) {
	s, ctx := newService(t), context.Background()
	s.SetPassword(ctx, "ana@example.com", "correct horse")
	for range maxFailures {
		s.Login(ctx, "ana@example.com", "wrong")
	}
	if _, err := s.Login(ctx, "ana@example.com", "correct horse"); !errors.Is(err, ErrTooMany) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.Login(ctx, "bob@example.com", "x"); !errors.Is(err, ErrBadLogin) {
		t.Fatalf("other accounts are not limited: %v", err)
	}
}

func TestSessions(t *testing.T) {
	s, ctx := newService(t), context.Background()
	u, _ := s.SetPassword(ctx, "ana@example.com", "correct horse")
	token, err := s.NewSession(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM sessions WHERE id = ?`, token).Scan(&n)
	if n != 0 {
		t.Fatal("the token itself is stored")
	}
	if got, err := s.Session(ctx, token); err != nil || got.Email != "ana@example.com" {
		t.Fatalf("session: %v %+v", err, got)
	}
	s.DB.Exec(`UPDATE sessions SET expires_at = 1`)
	if _, err := s.Session(ctx, token); err == nil {
		t.Fatal("expired session accepted")
	}
	token, _ = s.NewSession(ctx, u.ID)
	other, _ := s.NewSession(ctx, u.ID)
	s.EndSession(ctx, token)
	if _, err := s.Session(ctx, token); err == nil {
		t.Fatal("ended session accepted")
	}
	s.SetPassword(ctx, "ana@example.com", "battery staple")
	if _, err := s.Session(ctx, other); err == nil {
		t.Fatal("a new password must sign out every session")
	}
}

func TestSessionGuard(t *testing.T) {
	s, ctx := newService(t), context.Background()
	u, _ := s.SetPassword(ctx, "ana@example.com", "correct horse")
	token, _ := s.NewSession(ctx, u.ID)
	public := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("public")) })
	h := s.SessionGuard(public)(s.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := auth.UserFrom(r.Context())
		w.Write([]byte(got.Email))
	})))
	cases := []struct {
		path, cookie string
		code         int
		body         string
	}{
		{"/api/projects", "", http.StatusUnauthorized, ""},
		{"/api/projects", "forged", http.StatusUnauthorized, ""},
		{"/p/123", "", http.StatusOK, "public"},
		{"/api/projects", token, http.StatusOK, "ana@example.com"},
		{"/p/123", token, http.StatusOK, "ana@example.com"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", c.path, nil)
		if c.cookie != "" {
			r.AddCookie(&http.Cookie{Name: SessionCookie, Value: c.cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.code || (c.body != "" && w.Body.String() != c.body) {
			t.Errorf("%s cookie=%q: %d %q", c.path, c.cookie, w.Code, w.Body.String())
		}
	}
}
