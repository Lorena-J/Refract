package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/ponytojas/Refract/internal/store"
	"github.com/ponytojas/Refract/internal/users"
)

// runUsers manages accounts for REFRACT_AUTH=password, on the server's own
// database (same REFRACT_DATA_DIR and REFRACT_SECRET_KEY as the server).
func runUsers(args []string) error {
	if len(args) == 0 || (args[0] != "list" && (args[0] != "add" || len(args) != 2)) {
		return errors.New("usage: refract users add <email> | refract users list")
	}
	key, err := store.ParseKey(os.Getenv("REFRACT_SECRET_KEY"))
	if err != nil {
		return err
	}
	dataDir := cmp.Or(os.Getenv("REFRACT_DATA_DIR"), "/data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(dataDir, "refract.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	svc := &users.Service{DB: db, Key: key}
	ctx := context.Background()

	if args[0] == "list" {
		all, err := svc.List(ctx)
		if err != nil {
			return err
		}
		for _, email := range slices.Sorted(maps.Keys(all)) {
			state := "password"
			if !all[email] {
				state = "no password"
			}
			fmt.Printf("%s\t%s\n", email, state)
		}
		return nil
	}
	// A generated password: nothing to type into a shell history or a
	// terminal without echo control (docker exec without -t).
	pw := rand.Text()
	u, err := svc.SetPassword(ctx, args[1], pw)
	if err != nil {
		return err
	}
	fmt.Printf("%s can sign in with this password (change it in Settings → General):\n\n  %s\n\n", u.Email, pw)
	return nil
}
