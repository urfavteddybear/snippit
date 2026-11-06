package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"
)

type ctxKey int

const (
	nonceKey ctxKey = iota
	clientIPKey
)

// securityHeaders sets a strict CSP with a per-request nonce.
// The nonce is only used for <style> tags injected by CodeMirror.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		// base64url avoids '+', which html/template would entity-escape in attributes.
		nonce := base64.RawURLEncoding.EncodeToString(b)
		h := w.Header()
		h.Set("Content-Security-Policy", fmt.Sprintf(
			"default-src 'self'; script-src 'self'; style-src 'self' 'nonce-%s'; img-src 'self' data:; font-src 'self'; "+
				"connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'", nonce))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), nonceKey, nonce)))
	})
}

func nonceFrom(ctx context.Context) string {
	s, _ := ctx.Value(nonceKey).(string)
	return s
}

// clientIP resolves the caller's IP. X-Real-IP is trusted only behind our proxy,
// which overwrites it with $remote_addr.
func clientIP(trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ""
			if trustProxy {
				if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(v) != nil {
					ip = v
				}
			}
			if ip == "" {
				host, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					host = r.RemoteAddr
				}
				ip = host
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey, ip)))
		})
	}
}

func ipFrom(ctx context.Context) string {
	s, _ := ctx.Value(clientIPKey).(string)
	return s
}

// requestLogger logs request metadata only: no bodies, queries or client IPs.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			if r.URL.Path == "/healthz" {
				return
			}
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds())
		})
	}
}

// ipLimiter is a per-IP token bucket. Entries are kept only in memory and
// swept once idle, so IP addresses are never persisted.
type ipLimiter struct {
	mu      sync.Mutex
	entries map[string]*limiterEntry
	limit   rate.Limit
	burst   int
}

type limiterEntry struct {
	lim  *rate.Limiter
	seen time.Time
}

const maxLimiterEntries = 100_000

func newIPLimiter(perMinute float64, burst int) *ipLimiter {
	return &ipLimiter{
		entries: make(map[string]*limiterEntry),
		limit:   rate.Limit(perMinute / 60),
		burst:   burst,
	}
}

func (l *ipLimiter) Allow(ip string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[ip]
	if !ok {
		if len(l.entries) >= maxLimiterEntries {
			l.sweepLocked(now, time.Minute)
		}
		e = &limiterEntry{lim: rate.NewLimiter(l.limit, l.burst)}
		l.entries[ip] = e
	}
	e.seen = now
	return e.lim.AllowN(now, 1)
}

func (l *ipLimiter) sweep(idle time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked(time.Now(), idle)
}

func (l *ipLimiter) sweepLocked(now time.Time, idle time.Duration) {
	for ip, e := range l.entries {
		if now.Sub(e.seen) > idle {
			delete(l.entries, ip)
		}
	}
}
