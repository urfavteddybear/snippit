// Package store persists pastes in SQLite (WAL mode).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"snippit/internal/paste"

	_ "modernc.org/sqlite"
)

// migrations run in order; PRAGMA user_version records how many have been applied.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS pastes (
	id                  TEXT PRIMARY KEY,
	title               TEXT NOT NULL DEFAULT '',
	content             BLOB NOT NULL,
	language            TEXT NOT NULL,
	created_at          INTEGER NOT NULL,
	expires_at          INTEGER,
	deletion_token_hash TEXT,
	encrypted           INTEGER NOT NULL DEFAULT 0,
	kdf_salt            BLOB,
	size                INTEGER NOT NULL
) STRICT;
	CREATE INDEX IF NOT EXISTS idx_pastes_expires_at ON pastes(expires_at) WHERE expires_at IS NOT NULL;`,
	`ALTER TABLE pastes ADD COLUMN delete_key_hash TEXT`,
}

type SQLite struct {
	db *sql.DB
}

func Open(path string) (*SQLite, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	q := url.Values{}
	for _, p := range []string{"journal_mode(WAL)", "busy_timeout(5000)", "synchronous(NORMAL)", "foreign_keys(ON)"} {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &SQLite{db: db}, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		// PRAGMA does not accept bound parameters; i is an int we control.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// QuickCheck runs SQLite's integrity check and returns its findings.
func (s *SQLite) QuickCheck(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var problems []string
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return err
		}
		if msg != "ok" {
			problems = append(problems, msg)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(problems) > 0 {
		return fmt.Errorf("integrity check failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Backup writes a consistent snapshot of the database to dest.
func (s *SQLite) Backup(ctx context.Context, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("backup target %s already exists", dest)
	}
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dest)
	return err
}

func (s *SQLite) Insert(ctx context.Context, p *paste.Paste) error {
	var exp sql.NullInt64
	if p.ExpiresAt != nil {
		exp = sql.NullInt64{Int64: p.ExpiresAt.Unix(), Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pastes (id, title, content, language, created_at, expires_at, deletion_token_hash, delete_key_hash, encrypted, kdf_salt, size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Title, p.Content, p.Language, p.CreatedAt.Unix(), exp,
		nullString(p.DeletionTokenHash), nullString(p.DeleteKeyHash), p.Encrypted, p.KDFSalt, p.Size)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return paste.ErrIDCollision
	}
	return err
}

func (s *SQLite) Get(ctx context.Context, id string) (*paste.Paste, error) {
	var (
		p       paste.Paste
		created int64
		exp     sql.NullInt64
		hash    sql.NullString
		keyHash sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, content, language, created_at, expires_at, deletion_token_hash, delete_key_hash, encrypted, kdf_salt, size
		FROM pastes WHERE id = ?`, id).
		Scan(&p.ID, &p.Title, &p.Content, &p.Language, &created, &exp, &hash, &keyHash, &p.Encrypted, &p.KDFSalt, &p.Size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, paste.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.CreatedAt = time.Unix(created, 0).UTC()
	if exp.Valid {
		t := time.Unix(exp.Int64, 0).UTC()
		p.ExpiresAt = &t
	}
	p.DeletionTokenHash = hash.String
	p.DeleteKeyHash = keyHash.String
	return &p, nil
}

func (s *SQLite) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM pastes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return paste.ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM pastes WHERE expires_at IS NOT NULL AND expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SQLite) TotalSize(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size), 0) FROM pastes`).Scan(&n)
	return n, err
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
