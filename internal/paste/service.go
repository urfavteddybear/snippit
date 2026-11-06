package paste

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Service struct {
	repo          Repository
	maxBytes      int
	maxTotalBytes int64
	now           func() time.Time
}

func NewService(repo Repository, maxBytes int, maxTotalBytes int64) *Service {
	return &Service{repo: repo, maxBytes: maxBytes, maxTotalBytes: maxTotalBytes, now: time.Now}
}

func (s *Service) MaxBytes() int { return s.maxBytes }

type CreateInput struct {
	Title      string
	Content    string
	Language   string // language ID, alias, "auto" or empty
	Expiration string // key from Expirations; empty means DefaultExpiration
	Password   string // optional; encrypts content at rest
	DeleteKey  string // optional; lets anyone who knows it delete the paste
}

type Created struct {
	Paste         *Paste
	DeletionToken string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*Created, error) {
	if !utf8.ValidString(in.Content) || strings.ContainsRune(in.Content, 0) || strings.TrimSpace(in.Content) == "" {
		return nil, ErrInvalidContent
	}
	if len(in.Content) > s.maxBytes {
		return nil, ErrContentTooLarge
	}
	title, err := cleanTitle(in.Title)
	if err != nil {
		return nil, err
	}
	lang := strings.ToLower(strings.TrimSpace(in.Language))
	if lang == "" || lang == Auto {
		lang = Detect(in.Content)
	} else if l, ok := LookupLanguage(lang); ok {
		lang = l.ID
	} else {
		return nil, ErrInvalidLanguage
	}
	expKey := in.Expiration
	if expKey == "" {
		expKey = DefaultExpiration
	}
	ttl, ok := expirationTTL(expKey)
	if !ok {
		return nil, ErrInvalidExpiration
	}
	if len(in.Password) > MaxPasswordBytes {
		return nil, ErrPasswordTooLong
	}
	if len(in.DeleteKey) > MaxPasswordBytes {
		return nil, ErrDeleteKeyTooLong
	}

	if s.maxTotalBytes > 0 {
		total, err := s.repo.TotalSize(ctx)
		if err != nil {
			return nil, err
		}
		if total+int64(len(in.Content)) > s.maxTotalBytes {
			return nil, ErrQuotaExceeded
		}
	}

	now := s.now().UTC().Truncate(time.Second)
	p := &Paste{
		Title:     title,
		Content:   []byte(in.Content),
		Language:  lang,
		CreatedAt: now,
		Size:      int64(len(in.Content)),
	}
	if ttl > 0 {
		exp := now.Add(ttl)
		p.ExpiresAt = &exp
	}
	if in.Password != "" {
		p.Encrypted = true
		if p.KDFSalt, p.Content, err = seal(in.Password, p.Content); err != nil {
			return nil, fmt.Errorf("encrypt paste: %w", err)
		}
	}
	token, hash, err := newDeletionToken()
	if err != nil {
		return nil, err
	}
	p.DeletionTokenHash = hash
	if in.DeleteKey != "" {
		if p.DeleteKeyHash, err = hashDeleteKey(in.DeleteKey); err != nil {
			return nil, err
		}
	}

	for attempt := 0; ; attempt++ {
		if p.ID, err = NewID(); err != nil {
			return nil, err
		}
		err = s.repo.Insert(ctx, p)
		if !errors.Is(err, ErrIDCollision) || attempt == 4 {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return &Created{Paste: p, DeletionToken: token}, nil
}

// Get returns a paste that exists and has not expired. Content may still be sealed.
func (s *Service) Get(ctx context.Context, id string) (*Paste, error) {
	if !ValidID(id) {
		return nil, ErrNotFound
	}
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.Expired(p) {
		return nil, ErrExpired
	}
	return p, nil
}

func (s *Service) Expired(p *Paste) bool {
	return p.ExpiresAt != nil && !s.now().Before(*p.ExpiresAt)
}

// Open returns the plaintext of p, decrypting it with password when needed.
func (s *Service) Open(p *Paste, password string) (string, error) {
	if !p.Encrypted {
		return string(p.Content), nil
	}
	if password == "" {
		return "", ErrPasswordRequired
	}
	if len(password) > MaxPasswordBytes {
		return "", ErrInvalidPassword
	}
	pt, err := open(password, p.KDFSalt, p.Content)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// Delete removes a paste when either the creator's deletion token or the
// creator-chosen delete key matches.
func (s *Service) Delete(ctx context.Context, id, token, key string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !tokenMatches(token, p.DeletionTokenHash) && !deleteKeyMatches(key, p.DeleteKeyHash) {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) Cleanup(ctx context.Context) (int64, error) {
	return s.repo.DeleteExpired(ctx, s.now())
}

func cleanTitle(t string) (string, error) {
	t = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, t)
	t = strings.TrimSpace(t)
	if !utf8.ValidString(t) {
		return "", ErrInvalidTitle
	}
	if utf8.RuneCountInString(t) > MaxTitleRunes {
		return "", ErrInvalidTitle
	}
	return t, nil
}
