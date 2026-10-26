package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

// GetSetting returns the value for key, or "" if unset.
func GetSetting(ctx context.Context, db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetSetting upserts key. An empty value deletes it.
func SetSetting(ctx context.Context, db *sql.DB, key, value string) error {
	if value == "" {
		_, err := db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// ParseKey decodes a base64 32-byte AES-256 key (REFRACT_SECRET_KEY).
func ParseKey(b64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(key) != 32 {
		return nil, errors.New("REFRACT_SECRET_KEY must be 32 bytes, base64-encoded (openssl rand -base64 32)")
	}
	return key, nil
}

// Seal encrypts plaintext with AES-GCM. aad binds the ciphertext to its use
// (e.g. the settings key) so it can't be swapped into another slot.
func Seal(key, plaintext []byte, aad string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plaintext, []byte(aad))), nil
}

// Unseal reverses Seal.
func Unseal(key []byte, sealed, aad string) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < gcm.NonceSize() {
		return nil, errors.New("unseal: malformed ciphertext")
	}
	pt, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(aad))
	if err != nil {
		return nil, fmt.Errorf("unseal: %w (wrong REFRACT_SECRET_KEY?)", err)
	}
	return pt, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
