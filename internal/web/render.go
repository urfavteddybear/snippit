package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"snippit/internal/paste"
)

type pages struct {
	tmpl   map[string]*template.Template
	hashes map[string]string // static path -> content hash
}

var pageNames = []string{"home", "view", "unlock", "error", "docs", "privacy", "history"}

func loadPages(root, static fs.FS) (*pages, error) {
	p := &pages{tmpl: make(map[string]*template.Template), hashes: make(map[string]string)}
	err := fs.WalkDir(static, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(static, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		p.hashes[name] = hex.EncodeToString(sum[:])[:10]
		return nil
	})
	if err != nil {
		return nil, err
	}

	funcs := template.FuncMap{
		"asset":    p.asset,
		"langName": paste.LanguageName,
		"ago":      ago,
		"until":    until,
		"iso":      func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
		"utc":      func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") },
		"bytes":    humanBytes,
		"int64":    func(n int) int64 { return int64(n) },
	}
	for _, name := range pageNames {
		t, err := template.New(name).Funcs(funcs).ParseFS(root, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		p.tmpl[name] = t
	}
	return p, nil
}

// asset returns a cache-busting URL for a static file.
func (p *pages) asset(name string) string {
	if h, ok := p.hashes[name]; ok {
		return "/static/" + name + "?v=" + h
	}
	return "/static/" + name
}

// Common is embedded in every page model.
type Common struct {
	Nonce     string
	GitHubURL string
	PageTitle string
	Active    string
}

func (s *Server) common(r *http.Request, title, active string) Common {
	return Common{Nonce: nonceFrom(r.Context()), GitHubURL: s.cfg.GitHubURL, PageTitle: title, Active: active}
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.pages.tmpl[name].ExecuteTemplate(&buf, "layout", data); err != nil {
		s.log.Error("render template", "template", name, "err", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "no-cache")
	}
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

type errorPage struct {
	Common
	Status  int
	Heading string
	Message string
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, heading, message string) {
	s.render(w, status, "error", errorPage{
		Common:  s.common(r, heading, ""),
		Status:  status,
		Heading: heading,
		Message: message,
	})
}

// apiError is the JSON error envelope from the PRD.
type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var e apiError
	e.Error.Code, e.Error.Message = code, message
	writeJSON(w, status, e)
}

func ago(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	return humanDuration(d) + " ago"
}

func until(t time.Time) string {
	d := time.Until(t)
	if d <= 0 {
		return "now"
	}
	return "in " + humanDuration(d)
}

func humanDuration(d time.Duration) string {
	unit := func(n int, s string) string {
		if n == 1 {
			return "1 " + s
		}
		return fmt.Sprintf("%d %ss", n, s)
	}
	switch {
	case d < time.Minute:
		return "a few seconds"
	case d < time.Hour:
		return unit(int(d/time.Minute), "minute")
	case d < 48*time.Hour:
		return unit(int(d/time.Hour), "hour")
	case d < 60*24*time.Hour:
		return unit(int(d/(24*time.Hour)), "day")
	case d < 730*24*time.Hour:
		return unit(int(d/(30*24*time.Hour)), "month")
	default:
		return unit(int(d/(365*24*time.Hour)), "year")
	}
}

func humanBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// downloadName builds a safe attachment filename from the title.
func downloadName(p *paste.Paste) string {
	base := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		case r == ' ':
			return '-'
		}
		return -1
	}, p.Title)
	base = strings.Trim(path.Base("/"+base), ".-")
	if base == "" || base == "/" {
		base = p.ID
	}
	if len(base) > 80 {
		base = base[:80]
	}
	ext := "txt"
	if l, ok := paste.LookupLanguage(p.Language); ok {
		if l.ID == "dockerfile" {
			return "Dockerfile"
		}
		ext = l.Ext
	}
	if strings.HasSuffix(strings.ToLower(base), "."+ext) {
		return base
	}
	return base + "." + ext
}
