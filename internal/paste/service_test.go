package paste_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"snippit/internal/paste"
	"snippit/internal/store"
)

func newService(t *testing.T, maxBytes int, quota int64) (*paste.Service, *store.SQLite) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return paste.NewService(db, maxBytes, quota), db
}

func TestCreateAndGet(t *testing.T) {
	svc, _ := newService(t, 1024, 0)
	ctx := context.Background()

	c, err := svc.Create(ctx, paste.CreateInput{Title: "  hi\x00there ", Content: "package main\n\nfunc main() {}\n", Expiration: "1h"})
	if err != nil {
		t.Fatal(err)
	}
	if !paste.ValidID(c.Paste.ID) {
		t.Fatalf("bad id %q", c.Paste.ID)
	}
	if c.DeletionToken == "" || c.Paste.DeletionTokenHash == c.DeletionToken {
		t.Fatal("deletion token must be returned and stored hashed")
	}
	got, err := svc.Get(ctx, c.Paste.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "hi there" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Language != "go" {
		t.Errorf("detected language = %q, want go", got.Language)
	}
	if got.ExpiresAt == nil || got.ExpiresAt.Sub(got.CreatedAt) != time.Hour {
		t.Errorf("expires_at = %v", got.ExpiresAt)
	}
	if content, _ := svc.Open(got, ""); content != "package main\n\nfunc main() {}\n" {
		t.Errorf("content = %q", content)
	}
}

func TestCreateValidation(t *testing.T) {
	svc, _ := newService(t, 16, 0)
	ctx := context.Background()
	cases := []struct {
		name string
		in   paste.CreateInput
		want error
	}{
		{"empty", paste.CreateInput{Content: "  \n "}, paste.ErrInvalidContent},
		{"binary", paste.CreateInput{Content: "a\x00b"}, paste.ErrInvalidContent},
		{"invalid utf8", paste.CreateInput{Content: "\xff\xfe"}, paste.ErrInvalidContent},
		{"too large", paste.CreateInput{Content: strings.Repeat("x", 17)}, paste.ErrContentTooLarge},
		{"title", paste.CreateInput{Content: "x", Title: strings.Repeat("é", 121)}, paste.ErrInvalidTitle},
		{"language", paste.CreateInput{Content: "x", Language: "cobol"}, paste.ErrInvalidLanguage},
		{"expiration", paste.CreateInput{Content: "x", Expiration: "2d"}, paste.ErrInvalidExpiration},
		{"password", paste.CreateInput{Content: "x", Password: strings.Repeat("p", 257)}, paste.ErrPasswordTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(ctx, tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestLanguageAlias(t *testing.T) {
	svc, _ := newService(t, 1024, 0)
	c, err := svc.Create(context.Background(), paste.CreateInput{Content: "x", Language: "YML"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Paste.Language != "yaml" {
		t.Errorf("language = %q", c.Paste.Language)
	}
}

func TestNeverExpires(t *testing.T) {
	svc, _ := newService(t, 1024, 0)
	c, err := svc.Create(context.Background(), paste.CreateInput{Content: "x", Expiration: "never"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Paste.ExpiresAt != nil {
		t.Fatal("never-expiring paste has expires_at")
	}
}

func TestExpirationAndCleanup(t *testing.T) {
	svc, db := newService(t, 1024, 0)
	ctx := context.Background()
	c, err := svc.Create(ctx, paste.CreateInput{Content: "x", Expiration: "1h"})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := svc.Create(ctx, paste.CreateInput{Content: "y", Expiration: "never"})
	if err != nil {
		t.Fatal(err)
	}

	paste.SetClock(svc, func() time.Time { return time.Now().Add(2 * time.Hour) })
	if _, err := svc.Get(ctx, c.Paste.ID); !errors.Is(err, paste.ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
	n, err := svc.Cleanup(ctx)
	if err != nil || n != 1 {
		t.Fatalf("cleanup = %d, %v; want 1", n, err)
	}
	if _, err := db.Get(ctx, c.Paste.ID); !errors.Is(err, paste.ErrNotFound) {
		t.Fatalf("expired row still present: %v", err)
	}
	if _, err := svc.Get(ctx, keep.Paste.ID); err != nil {
		t.Fatalf("non-expiring paste removed: %v", err)
	}
}

func TestPasswordEncryption(t *testing.T) {
	svc, db := newService(t, 1024, 0)
	ctx := context.Background()
	secret := "API_KEY=supersecret"
	c, err := svc.Create(ctx, paste.CreateInput{Content: secret, Password: "hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	row, err := db.Get(ctx, c.Paste.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.Encrypted || strings.Contains(string(row.Content), "supersecret") {
		t.Fatal("content stored in plaintext")
	}
	if _, err := svc.Open(row, ""); !errors.Is(err, paste.ErrPasswordRequired) {
		t.Errorf("no password: %v", err)
	}
	if _, err := svc.Open(row, "wrong"); !errors.Is(err, paste.ErrInvalidPassword) {
		t.Errorf("wrong password: %v", err)
	}
	got, err := svc.Open(row, "hunter2")
	if err != nil || got != secret {
		t.Errorf("open = %q, %v", got, err)
	}
}

func TestDelete(t *testing.T) {
	svc, _ := newService(t, 1024, 0)
	ctx := context.Background()
	c, err := svc.Create(ctx, paste.CreateInput{Content: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, c.Paste.ID, "", ""); !errors.Is(err, paste.ErrForbidden) {
		t.Errorf("empty token: %v", err)
	}
	if err := svc.Delete(ctx, c.Paste.ID, "nope", "nope"); !errors.Is(err, paste.ErrForbidden) {
		t.Errorf("wrong token: %v", err)
	}
	if err := svc.Delete(ctx, c.Paste.ID, c.DeletionToken, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, c.Paste.ID); !errors.Is(err, paste.ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
}

func TestQuota(t *testing.T) {
	svc, _ := newService(t, 1024, 10)
	ctx := context.Background()
	if _, err := svc.Create(ctx, paste.CreateInput{Content: "123456"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, paste.CreateInput{Content: "123456"}); !errors.Is(err, paste.ErrQuotaExceeded) {
		t.Fatalf("err = %v, want ErrQuotaExceeded", err)
	}
}

func TestGetRejectsMalformedIDs(t *testing.T) {
	svc, _ := newService(t, 1024, 0)
	for _, id := range []string{"", "../../etc", "abc", "abcde", "abcdefghijk", "abcdefghij'", "abcdefghi/"} {
		if _, err := svc.Get(context.Background(), id); !errors.Is(err, paste.ErrNotFound) {
			t.Errorf("Get(%q) = %v", id, err)
		}
	}
}

func TestNewIDUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 10000; i++ {
		id, err := paste.NewID()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) != 6 || !paste.ValidID(id) || seen[id] {
			t.Fatalf("bad or duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestDeleteKey(t *testing.T) {
	svc, db := newService(t, 1024, 0)
	ctx := context.Background()
	c, err := svc.Create(ctx, paste.CreateInput{Content: "x", DeleteKey: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	row, _ := db.Get(ctx, c.Paste.ID)
	if row.DeleteKeyHash == "" || strings.Contains(row.DeleteKeyHash, "correct horse") {
		t.Fatalf("delete key not hashed: %q", row.DeleteKeyHash)
	}
	if err := svc.Delete(ctx, c.Paste.ID, "", "wrong"); !errors.Is(err, paste.ErrForbidden) {
		t.Errorf("wrong key: %v", err)
	}
	if err := svc.Delete(ctx, c.Paste.ID, "", "correct horse"); err != nil {
		t.Fatalf("right key: %v", err)
	}

	// Without a delete key, a key guess must never match.
	c2, _ := svc.Create(ctx, paste.CreateInput{Content: "y"})
	if err := svc.Delete(ctx, c2.Paste.ID, "", "anything"); !errors.Is(err, paste.ErrForbidden) {
		t.Errorf("paste without key: %v", err)
	}
	if _, err := svc.Create(ctx, paste.CreateInput{Content: "z", DeleteKey: strings.Repeat("k", 257)}); !errors.Is(err, paste.ErrDeleteKeyTooLong) {
		t.Errorf("long key: %v", err)
	}
}

func TestMigratesExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	// Reopening must be idempotent.
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}
