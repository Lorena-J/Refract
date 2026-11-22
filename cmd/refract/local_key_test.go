package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ponytojas/Refract/internal/localkey"
)

type fakeKeys struct {
	items  map[string]string
	err    error // unreachable store
	setErr error
}

func (f *fakeKeys) Get(service, account string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.items[account]
	if !ok {
		return "", localkey.ErrNotFound
	}
	return v, nil
}

func (f *fakeKeys) Set(service, account, secret string) error {
	if f.err != nil {
		return f.err
	}
	if f.setErr != nil {
		return f.setErr
	}
	if f.items == nil {
		f.items = map[string]string{}
	}
	f.items[account] = secret
	return nil
}

func TestResolveLocalKeyUsesKeychain(t *testing.T) {
	dir := t.TempDir()
	ks := &fakeKeys{}
	k1, err := resolveLocalKey(ks, dir)
	if err != nil || len(k1) != 32 {
		t.Fatalf("first run: %v %v", k1, err)
	}
	if _, err := os.Stat(filepath.Join(dir, localKeyFile)); !os.IsNotExist(err) {
		t.Fatalf("secret.key exists with a working keychain: %v", err)
	}
	if len(ks.items) != 1 {
		t.Fatalf("keychain items: %v", ks.items)
	}
	k2, err := resolveLocalKey(ks, dir)
	if err != nil || !bytes.Equal(k1, k2) {
		t.Fatalf("second run read a different key: %v", err)
	}
}

func TestResolveLocalKeyTwoFoldersTwoKeys(t *testing.T) {
	ks := &fakeKeys{}
	a, _ := resolveLocalKey(ks, t.TempDir())
	b, _ := resolveLocalKey(ks, t.TempDir())
	if bytes.Equal(a, b) || len(ks.items) != 2 {
		t.Fatalf("want two keys, got %d items", len(ks.items))
	}
}

func TestResolveLocalKeyFallsBackToFile(t *testing.T) {
	for name, ks := range map[string]*fakeKeys{
		"unreachable": {err: errors.New("no D-Bus session")},
		"cannot save": {setErr: errors.New("locked")},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			k, err := resolveLocalKey(ks, dir)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, localKeyFile)
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm()&0o077 != 0 {
				t.Fatalf("key file mode %v", fi.Mode().Perm())
			}
			again, err := resolveLocalKey(ks, dir)
			if err != nil || !bytes.Equal(k, again) {
				t.Fatalf("second run: %v", err)
			}
		})
	}
}

func TestResolveLocalKeyKeepsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, localKeyFile)
	want, err := loadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	ks := &fakeKeys{}
	got, err := resolveLocalKey(ks, dir)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("got %v %v, want the file's key", got, err)
	}
	if len(ks.items) != 0 {
		t.Fatal("the file was migrated into the keychain silently")
	}
}

func TestResolveLocalKeyDeletedItem(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "refract.db"), nil, 0o600)
	ks := &fakeKeys{}
	old, _ := resolveLocalKey(ks, dir)
	ks.items = nil // the item is deleted
	k, err := resolveLocalKey(ks, dir)
	if err != nil || len(k) != 32 || bytes.Equal(k, old) {
		t.Fatalf("want a clean report and a new key, got %v", err)
	}
}

func TestResolveLocalKeyInvalidItemIsKept(t *testing.T) {
	dir := t.TempDir()
	ks := &fakeKeys{items: map[string]string{localkey.Account(dir): "junk"}}
	if _, err := resolveLocalKey(ks, dir); !errors.Is(err, localkey.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
	if ks.items[localkey.Account(dir)] != "junk" {
		t.Fatal("the invalid item was replaced")
	}
}
