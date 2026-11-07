// Package config loads runtime settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr            string
	DBPath          string
	BaseURL         string // public origin, e.g. https://snippit.example; derived from the request when empty
	MaxPasteBytes   int
	MaxTotalBytes   int64 // global storage quota; 0 disables it
	CreatePerMinute float64
	CreateBurst     int
	UnlockPerMinute float64
	UnlockBurst     int
	MissPerMinute   float64 // lookups of nonexistent pastes, per IP
	MissBurst       int
	TrustProxy      bool // read client IP from X-Real-IP (set by a trusted reverse proxy)
	MaxConcurrent   int
	RequestTimeout  time.Duration
	CleanupInterval time.Duration
	GitHubURL       string
}

func Load() (Config, error) {
	c := Config{
		Addr:            env("SNIPPIT_ADDR", ":8080"),
		DBPath:          env("SNIPPIT_DB_PATH", "data/snippit.db"),
		BaseURL:         strings.TrimRight(env("SNIPPIT_BASE_URL", ""), "/"),
		GitHubURL:       env("SNIPPIT_GITHUB_URL", ""),
		MaxPasteBytes:   512 * 1024,
		MaxTotalBytes:   1 << 30,
		CreatePerMinute: 10,
		CreateBurst:     5,
		UnlockPerMinute: 10,
		UnlockBurst:     5,
		MissPerMinute:   30,
		MissBurst:       30,
		MaxConcurrent:   100,
		RequestTimeout:  15 * time.Second,
		CleanupInterval: 10 * time.Minute,
	}

	var err error
	set := func(key string, parse func(string) error) {
		if err != nil {
			return
		}
		if v, ok := os.LookupEnv(key); ok && v != "" {
			if perr := parse(v); perr != nil {
				err = fmt.Errorf("config %s=%q: %w", key, v, perr)
			}
		}
	}
	set("SNIPPIT_MAX_PASTE_BYTES", intVar(&c.MaxPasteBytes))
	set("SNIPPIT_MAX_TOTAL_BYTES", func(v string) (e error) { c.MaxTotalBytes, e = strconv.ParseInt(v, 10, 64); return })
	set("SNIPPIT_RATE_LIMIT_PER_MINUTE", floatVar(&c.CreatePerMinute))
	set("SNIPPIT_RATE_LIMIT_BURST", intVar(&c.CreateBurst))
	set("SNIPPIT_UNLOCK_LIMIT_PER_MINUTE", floatVar(&c.UnlockPerMinute))
	set("SNIPPIT_UNLOCK_LIMIT_BURST", intVar(&c.UnlockBurst))
	set("SNIPPIT_NOTFOUND_LIMIT_PER_MINUTE", floatVar(&c.MissPerMinute))
	set("SNIPPIT_NOTFOUND_LIMIT_BURST", intVar(&c.MissBurst))
	set("SNIPPIT_MAX_CONCURRENT", intVar(&c.MaxConcurrent))
	set("SNIPPIT_TRUST_PROXY", func(v string) (e error) { c.TrustProxy, e = strconv.ParseBool(v); return })
	set("SNIPPIT_REQUEST_TIMEOUT", durationVar(&c.RequestTimeout))
	set("SNIPPIT_CLEANUP_INTERVAL", durationVar(&c.CleanupInterval))
	if err != nil {
		return c, err
	}

	switch {
	case c.MaxPasteBytes <= 0:
		return c, fmt.Errorf("config SNIPPIT_MAX_PASTE_BYTES must be positive")
	case c.CreatePerMinute <= 0 || c.CreateBurst <= 0 || c.UnlockPerMinute <= 0 || c.UnlockBurst <= 0 ||
		c.MissPerMinute <= 0 || c.MissBurst <= 0:
		return c, fmt.Errorf("config rate limits must be positive")
	case c.MaxConcurrent <= 0:
		return c, fmt.Errorf("config SNIPPIT_MAX_CONCURRENT must be positive")
	case c.CleanupInterval <= 0:
		return c, fmt.Errorf("config SNIPPIT_CLEANUP_INTERVAL must be positive")
	}
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intVar(dst *int) func(string) error {
	return func(v string) (err error) { *dst, err = strconv.Atoi(v); return }
}

func floatVar(dst *float64) func(string) error {
	return func(v string) (err error) { *dst, err = strconv.ParseFloat(v, 64); return }
}

func durationVar(dst *time.Duration) func(string) error {
	return func(v string) (err error) { *dst, err = time.ParseDuration(v); return }
}
