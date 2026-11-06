// Package paste holds the paste domain model and business rules.
package paste

import (
	"context"
	"errors"
	"time"
)

type Paste struct {
	ID                string
	Title             string
	Content           []byte // plaintext, or nonce||ciphertext when Encrypted
	Language          string
	CreatedAt         time.Time
	ExpiresAt         *time.Time
	DeletionTokenHash string
	DeleteKeyHash     string // optional creator-chosen key, scrypt-hashed
	Encrypted         bool
	KDFSalt           []byte
	Size              int64 // plaintext size in bytes
}

// Repository is the persistence boundary used by Service.
type Repository interface {
	Insert(ctx context.Context, p *Paste) error
	Get(ctx context.Context, id string) (*Paste, error)
	Delete(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
	TotalSize(ctx context.Context) (int64, error)
}

var (
	ErrNotFound          = errors.New("paste not found")
	ErrExpired           = errors.New("paste expired")
	ErrIDCollision       = errors.New("paste id collision")
	ErrInvalidContent    = errors.New("content must be non-empty UTF-8 text")
	ErrContentTooLarge   = errors.New("content too large")
	ErrInvalidTitle      = errors.New("title too long")
	ErrInvalidLanguage   = errors.New("unsupported language")
	ErrInvalidExpiration = errors.New("unsupported expiration")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrPasswordRequired  = errors.New("password required")
	ErrPasswordTooLong   = errors.New("password too long")
	ErrDeleteKeyTooLong  = errors.New("delete key too long")
	ErrForbidden         = errors.New("invalid deletion token")
	ErrQuotaExceeded     = errors.New("storage quota exceeded")
)

const (
	MaxTitleRunes     = 120
	MaxPasswordBytes  = 256
	DefaultExpiration = "7d"
)

// Expirations lists the allowed expiration choices in display order.
var Expirations = []struct {
	Key   string
	Label string
	TTL   time.Duration // zero means never
}{
	{"1h", "1 hour", time.Hour},
	{"1d", "1 day", 24 * time.Hour},
	{"7d", "7 days", 7 * 24 * time.Hour},
	{"30d", "30 days", 30 * 24 * time.Hour},
	{"never", "Never", 0},
}

func expirationTTL(key string) (time.Duration, bool) {
	for _, e := range Expirations {
		if e.Key == key {
			return e.TTL, true
		}
	}
	return 0, false
}
