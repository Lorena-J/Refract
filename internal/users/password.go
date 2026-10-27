package users

import (
	"cmp"
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ponytojas/Refract/internal/auth"
)

// Passwords and sessions, for REFRACT_AUTH=password.

const (
	pbkdf2Iter  = 600_000 // OWASP's figure for PBKDF2-HMAC-SHA256
	MinPassword = 8
	maxPassword = 1024

	SessionCookie = "refract_session"
	SessionTTL    = 30 * 24 * time.Hour

	maxFailures = 10
	failWindow  = 15 * time.Minute
)

var (
	ErrBadLogin = errors.New("wrong email or password")
	ErrTooMany  = errors.New("too many failed attempts: try again in a few minutes")
)

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	rand.Read(salt)
	k, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter, b64(salt), b64(k)), nil
}

func checkPassword(stored, pw string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err1 := strconv.Atoi(parts[1])
	salt, err2 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err3 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil || err3 != nil || iter < 1 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is checked against when the email is unknown, so a wrong email
// takes as long as a wrong password.
var dummyHash = sync.OnceValue(func() string { h, _ := hashPassword("refract"); return h })

func normEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// SetPassword sets a user's password, creating the user if needed, and signs
// them out everywhere.
func (s *Service) SetPassword(ctx context.Context, email, pw string) (auth.User, error) {
	email = normEmail(email)
	if !strings.Contains(email, "@") || strings.ContainsAny(email, " \t\r\n") {
		return auth.User{}, fmt.Errorf("%q is not an email address", email)
	}
	if len(pw) < MinPassword || len(pw) > maxPassword {
		return auth.User{}, fmt.Errorf("passwords need %d to %d characters", MinPassword, maxPassword)
	}
	u, err := s.Resolve(ctx, email)
	if err != nil {
		return auth.User{}, err
	}
	h, err := hashPassword(pw)
	if err != nil {
		return auth.User{}, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET password = ? WHERE id = ?`, h, u.ID); err != nil {
		return auth.User{}, err
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, u.ID)
	return u, err
}

// Login checks an email and password.
// ponytail: failures are counted per email in memory (lost on restart); per-IP limits if that's not enough.
func (s *Service) Login(ctx context.Context, email, pw string) (auth.User, error) {
	email = normEmail(email)
	if !s.allowAttempt(email) {
		return auth.User{}, ErrTooMany
	}
	u := auth.User{Email: email}
	var stored string
	err := s.DB.QueryRowContext(ctx, `SELECT id, password FROM users WHERE email = ?`, email).Scan(&u.ID, &stored)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, err
	}
	if stored == "" {
		checkPassword(dummyHash(), pw)
		s.fail(email)
		return auth.User{}, ErrBadLogin
	}
	if !checkPassword(stored, pw) {
		s.fail(email)
		return auth.User{}, ErrBadLogin
	}
	s.mu.Lock()
	delete(s.failures, email)
	s.mu.Unlock()
	return u, nil
}

type failures struct {
	n     int
	since time.Time
}

func (s *Service) allowAttempt(email string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.failures[email]
	if ok && time.Since(f.since) > failWindow {
		delete(s.failures, email)
		return true
	}
	return !ok || f.n < maxFailures
}

func (s *Service) fail(email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failures == nil {
		s.failures = map[string]failures{}
	}
	f := s.failures[email]
	if f.n == 0 {
		f.since = time.Now()
	}
	f.n++
	s.failures[email] = f
}

// HasPassword reports whether the user can sign in with a password.
func (s *Service) HasPassword(ctx context.Context, userID string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE id = ? AND password != ''`, userID).Scan(&n)
	return n > 0, err
}

// Rename changes a user's email, keeping everything they own. Moving from
// REFRACT_AUTH=none to a sign-in mode is "rename local you@example.com".
func (s *Service) Rename(ctx context.Context, from, to string) error {
	from, to = normEmail(from), normEmail(to)
	if !strings.Contains(to, "@") || strings.ContainsAny(to, " \t\r\n") {
		return fmt.Errorf("%q is not an email address", to)
	}
	if taken, err := s.Exists(ctx, to); err != nil || taken {
		return cmp.Or(err, fmt.Errorf("%s already has an account", to))
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE users SET email = ? WHERE email = ?`, to, from)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("there is no account %q", from)
	}
	s.cache.Delete(from)
	return nil
}

// Exists reports whether a user with this email exists.
func (s *Service) Exists(ctx context.Context, email string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE email = ?`, normEmail(email)).Scan(&n)
	return n > 0, err
}

// List returns every user's email, and whether they have a password.
func (s *Service) List(ctx context.Context) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT email, password != '' FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var email string
		var has bool
		if err := rows.Scan(&email, &has); err != nil {
			return nil, err
		}
		out[email] = has
	}
	return out, rows.Err()
}

// sessionID is what the database keeps: a leaked database holds no usable cookies.
func sessionID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewSession signs a user in and returns the cookie's token.
func (s *Service) NewSession(ctx context.Context, userID string) (string, error) {
	b := make([]byte, 32)
	rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	now := time.Now()
	s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now.Unix())
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`, sessionID(token), userID, now.Add(SessionTTL).Unix())
	return token, err
}

// Session returns the user a session token belongs to.
func (s *Service) Session(ctx context.Context, token string) (auth.User, error) {
	var u auth.User
	err := s.DB.QueryRowContext(ctx, `SELECT u.id, u.email FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.id = ? AND s.expires_at > ?`,
		sessionID(token), time.Now().Unix()).Scan(&u.ID, &u.Email)
	return u, err
}

func (s *Service) EndSession(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID(token))
	return err
}

// SessionGuard is the web UI guard for REFRACT_AUTH=password. Requests with a
// valid session go on as that user. Without one, /api gets 401 and every
// other path is served by public: the app's files, which show the sign-in page.
func (s *Service) SessionGuard(public http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(SessionCookie); err == nil {
				if u, err := s.Session(r.Context(), c.Value); err == nil {
					next.ServeHTTP(w, auth.WithIdentity(r, u.Email))
					return
				}
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"kind":"login_required","message":"Sign in to continue."}}`))
				return
			}
			public.ServeHTTP(w, r)
		})
	}
}

// SetSessionCookie and ClearSessionCookie write the session cookie. Secure
// when the request came over https (directly or through a proxy).
func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: token, Path: "/", MaxAge: int(SessionTTL.Seconds()),
		HttpOnly: true, Secure: Secure(r), SameSite: http.SameSiteLaxMode})
}

func ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: Secure(r), SameSite: http.SameSiteLaxMode})
}

func Secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
