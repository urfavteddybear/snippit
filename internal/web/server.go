// Package web serves the Snippit HTML pages, JSON API and static assets.
package web

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"snippit/internal/config"
	"snippit/internal/paste"
)

//go:embed all:static templates
var embedded embed.FS

// Pinger reports database health.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Server struct {
	cfg     config.Config
	svc     *paste.Service
	db      Pinger
	log     *slog.Logger
	pages   *pages
	static  fs.FS
	create  *ipLimiter
	unlock  *ipLimiter
	handler http.Handler
}

func New(cfg config.Config, svc *paste.Service, db Pinger, log *slog.Logger) (*Server, error) {
	static, err := fs.Sub(embedded, "static")
	if err != nil {
		return nil, err
	}
	pg, err := loadPages(embedded, static)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:    cfg,
		svc:    svc,
		db:     db,
		log:    log,
		pages:  pg,
		static: static,
		create: newIPLimiter(cfg.CreatePerMinute, cfg.CreateBurst),
		unlock: newIPLimiter(cfg.UnlockPerMinute, cfg.UnlockBurst),
	}
	s.handler = s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// SweepLimiters drops idle rate-limit entries until ctx is done.
func (s *Server) SweepLimiters(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.create.sweep(10 * time.Minute)
			s.unlock.sweep(10 * time.Minute)
		}
	}
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(clientIP(s.cfg.TrustProxy))
	r.Use(requestLogger(s.log))
	r.Use(securityHeaders)
	r.Use(middleware.ThrottleBacklog(s.cfg.MaxConcurrent, s.cfg.MaxConcurrent*2, 5*time.Second))
	r.Use(middleware.Timeout(s.cfg.RequestTimeout))
	r.Use(middleware.Compress(5, "text/html", "text/css", "text/plain", "text/javascript", "application/javascript", "application/json", "image/svg+xml"))

	r.Get("/healthz", s.handleHealth)
	r.Handle("/static/*", http.StripPrefix("/static/", s.staticHandler()))
	r.Get("/favicon.ico", s.handleFavicon)

	r.Get("/", s.handleHome)
	r.Post("/pastes", s.handleCreateForm)
	r.Get("/docs", s.handleDocs)
	r.Get("/privacy", s.handlePrivacy)
	r.Get("/history", s.handleHistory)

	r.Route("/api", func(r chi.Router) {
		r.Post("/pastes", s.handleAPICreate)
		r.Get("/pastes/{id}", s.handleAPIGet)
		r.Delete("/pastes/{id}", s.handleAPIDelete)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "The requested endpoint does not exist.")
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
		})
	})

	r.Get("/raw/{id}", s.handleRaw)
	r.Get("/download/{id}", s.handleDownload)
	r.Get("/qr/{id}", s.handleQR)
	r.Get("/{id}", s.handleView)
	r.Post("/{id}/unlock", s.handleUnlock)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		s.renderError(w, r, http.StatusNotFound, "Page not found", "The page you are looking for does not exist.")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "This page does not accept that request.")
	})
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	w.Header().Set("Cache-Control", "no-store")
	if err := s.db.Ping(ctx); err != nil {
		s.log.Error("health check failed", "err", err)
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

// staticHandler serves embedded assets. Fingerprinted requests (?v= or hashed
// chunk names) are cached for a year.
func (s *Server) staticHandler() http.Handler {
	files := http.FileServerFS(s.static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Has("v") || strings.HasPrefix(r.URL.Path, "chunks/") || strings.HasPrefix(r.URL.Path, "fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		files.ServeHTTP(w, r)
	})
}

// handleFavicon answers browsers' automatic /favicon.ico request; there is no icon.
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusNoContent)
}

// baseURL returns the public origin used in generated links.
func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.BaseURL != "" {
		return s.cfg.BaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if s.cfg.TrustProxy && r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
