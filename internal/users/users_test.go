package users

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ponytojas/Refract/internal/auth"
	"github.com/ponytojas/Refract/internal/store"
)

func TestFirstUserAdoptsSingleUserData(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "u.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := make([]byte, 32)
	// A database from before users: an ownerless project and global settings.
	db.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'thesis', 0, 0)`)
	sealed, _ := store.Seal(key, []byte(`{"refresh_token":"r"}`), "siwc.credentials")
	store.SetSetting(ctx, db, "siwc.credentials", sealed)
	store.SetSetting(ctx, db, "siwc.welcome_seen", "1")
	store.SetSetting(ctx, db, "siwc.host_id", "urn:uuid:x") // the server's: stays global

	s := &Service{DB: db, Key: key}
	ana, err := s.Resolve(ctx, "ana@example.com")
	if err != nil || ana.ID == "" {
		t.Fatalf("resolve = %+v, %v", ana, err)
	}
	if again, _ := (&Service{DB: db, Key: key}).Resolve(ctx, "ana@example.com"); again.ID != ana.ID {
		t.Errorf("same email, new user: %q vs %q", again.ID, ana.ID)
	}
	var owner string
	db.QueryRow(`SELECT owner FROM projects WHERE id = 'p1'`).Scan(&owner)
	if owner != ana.ID {
		t.Errorf("project owner = %q, want the first user", owner)
	}
	moved, _ := store.GetSetting(ctx, db, store.UserKey(ana.ID, "siwc.credentials"))
	if plain, err := store.Unseal(key, moved, store.UserKey(ana.ID, "siwc.credentials")); err != nil || string(plain) != `{"refresh_token":"r"}` {
		t.Errorf("credentials not re-sealed for the user: %q, %v", plain, err)
	}
	if old, _ := store.GetSetting(ctx, db, "siwc.credentials"); old != "" {
		t.Error("old global credentials left behind")
	}
	if w, _ := store.GetSetting(ctx, db, store.UserKey(ana.ID, "siwc.welcome_seen")); w != "1" {
		t.Error("welcome flag not moved")
	}
	if h, _ := store.GetSetting(ctx, db, "siwc.host_id"); h != "urn:uuid:x" {
		t.Error("host id moved")
	}

	// A second user starts empty.
	db.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p2', 'stray', 0, 0)`)
	ben, _ := s.Resolve(ctx, "ben@example.com")
	db.QueryRow(`SELECT owner FROM projects WHERE id = 'p2'`).Scan(&owner)
	if ben.ID == ana.ID || owner != "" {
		t.Errorf("second user adopted data: owner %q", owner)
	}
}

func TestMiddleware(t *testing.T) {
	db, _ := store.Open(filepath.Join(t.TempDir(), "u.db"))
	defer db.Close()
	s := &Service{DB: db, Key: make([]byte, 32)}
	var got auth.User
	h := s.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got, _ = auth.UserFrom(r.Context()) }))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil)) // no identity
	if w.Code != http.StatusForbidden {
		t.Errorf("no identity: %d", w.Code)
	}
	w = httptest.NewRecorder()
	auth.Unauthenticated(h).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || got.Email != auth.LocalIdentity || got.ID == "" {
		t.Errorf("local user = %+v (%d)", got, w.Code)
	}
}
