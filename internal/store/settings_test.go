package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"path/filepath"
	"testing"
)

func TestSealRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	sealed, err := Seal(key, []byte("secret"), "slot-a")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains([]byte(sealed), []byte("secret")) {
		t.Fatal("plaintext leaked into ciphertext")
	}
	pt, err := Unseal(key, sealed, "slot-a")
	if err != nil || string(pt) != "secret" {
		t.Fatalf("Unseal = %q, %v", pt, err)
	}
	if _, err := Unseal(key, sealed, "slot-b"); err == nil {
		t.Fatal("Unseal with other aad succeeded")
	}
	other := make([]byte, 32)
	rand.Read(other)
	if _, err := Unseal(other, sealed, "slot-a"); err == nil {
		t.Fatal("Unseal with other key succeeded")
	}
}

func TestParseKey(t *testing.T) {
	if _, err := ParseKey("c2hvcnQ="); err == nil {
		t.Fatal("short key accepted")
	}
	if _, err := ParseKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="); err != nil {
		t.Fatal(err)
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, v := range []string{"one", "two", ""} {
		if err := SetSetting(ctx, db, "k", v); err != nil {
			t.Fatal(err)
		}
		if got, _ := GetSetting(ctx, db, "k"); got != v {
			t.Fatalf("got %q, want %q", got, v)
		}
	}
}
