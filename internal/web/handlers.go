package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"snippit/internal/paste"
)

const (
	createdCookie  = "snippit_created"
	passwordHeader = "X-Paste-Password"
	deleteHeader   = "X-Delete-Token"
	deleteKeyHdr   = "X-Delete-Key"
)

// markdown renders GitHub-flavoured Markdown. Raw HTML is omitted and
// dangerous link schemes are dropped (goldmark defaults; WithUnsafe is never set).
var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

type failure struct {
	status  int
	code    string
	message string
}

var errRateLimited = errors.New("rate limited")

func classify(err error) failure {
	switch {
	case errors.Is(err, paste.ErrNotFound):
		return failure{http.StatusNotFound, "PASTE_NOT_FOUND", "The requested paste does not exist."}
	case errors.Is(err, paste.ErrExpired):
		return failure{http.StatusGone, "PASTE_EXPIRED", "The paste is no longer available."}
	case errors.Is(err, paste.ErrInvalidContent):
		return failure{http.StatusBadRequest, "INVALID_CONTENT", "Content must be non-empty UTF-8 text."}
	case errors.Is(err, paste.ErrContentTooLarge):
		return failure{http.StatusRequestEntityTooLarge, "CONTENT_TOO_LARGE", "Content exceeds the maximum paste size."}
	case errors.Is(err, paste.ErrInvalidTitle):
		return failure{http.StatusBadRequest, "INVALID_TITLE", fmt.Sprintf("Title must be at most %d characters.", paste.MaxTitleRunes)}
	case errors.Is(err, paste.ErrInvalidLanguage):
		return failure{http.StatusBadRequest, "INVALID_LANGUAGE", "Unsupported language."}
	case errors.Is(err, paste.ErrInvalidExpiration):
		return failure{http.StatusBadRequest, "INVALID_EXPIRATION", "Expiration must be one of 1h, 1d, 7d, 30d or never."}
	case errors.Is(err, paste.ErrPasswordTooLong):
		return failure{http.StatusBadRequest, "INVALID_PASSWORD", fmt.Sprintf("Password must be at most %d bytes.", paste.MaxPasswordBytes)}
	case errors.Is(err, paste.ErrDeleteKeyTooLong):
		return failure{http.StatusBadRequest, "INVALID_DELETE_KEY", fmt.Sprintf("Delete key must be at most %d bytes.", paste.MaxPasswordBytes)}
	case errors.Is(err, paste.ErrPasswordRequired):
		return failure{http.StatusUnauthorized, "PASSWORD_REQUIRED", "This paste is password-protected."}
	case errors.Is(err, paste.ErrInvalidPassword):
		return failure{http.StatusForbidden, "INVALID_PASSWORD", "The password is incorrect."}
	case errors.Is(err, paste.ErrForbidden):
		return failure{http.StatusForbidden, "INVALID_DELETE_KEY", "The delete key is incorrect."}
	case errors.Is(err, paste.ErrQuotaExceeded):
		return failure{http.StatusInsufficientStorage, "STORAGE_FULL", "Storage is full. Please try again later."}
	case errors.Is(err, errRateLimited):
		return failure{http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Please slow down."}
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return failure{http.StatusRequestEntityTooLarge, "CONTENT_TOO_LARGE", "Request body is too large."}
	}
	return failure{http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong."}
}

func (s *Server) fail(err error) failure {
	f := classify(err)
	if f.status == http.StatusInternalServerError {
		s.log.Error("request failed", "err", err)
	}
	return f
}

// bodyLimit allows for form/JSON encoding overhead around the content.
func (s *Server) bodyLimit() int64 {
	return int64(s.svc.MaxBytes())*3 + 64*1024
}

// lookup loads the paste named in the URL. Short IDs are guessable in bulk, so
// each miss spends a per-IP token; once an IP runs out, every lookup from it is
// refused until the bucket refills, even for pastes that exist.
func (s *Server) lookup(r *http.Request) (*paste.Paste, error) {
	ip := ipFrom(r.Context())
	if s.miss.Exhausted(ip) {
		return nil, errRateLimited
	}
	p, err := s.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, paste.ErrNotFound) {
		s.miss.Allow(ip)
	}
	return p, err
}

// ---- HTML pages ----

type homePage struct {
	Common
	Languages   []paste.Language
	Expirations any
	MaxBytes    int
	Form        paste.CreateInput
	Error       string
}

func (s *Server) homeData(r *http.Request) homePage {
	return homePage{
		Common:      s.common(r, "", "home"),
		Languages:   paste.Languages,
		Expirations: paste.Expirations,
		MaxBytes:    s.svc.MaxBytes(),
		Form:        paste.CreateInput{Language: paste.Auto, Expiration: paste.DefaultExpiration},
	}
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "home", s.homeData(r))
}

func (s *Server) handleCreateForm(w http.ResponseWriter, r *http.Request) {
	htmx := r.Header.Get("HX-Request") == "true"
	in := paste.CreateInput{}
	var err error
	if !s.create.Allow(ipFrom(r.Context())) {
		err = errRateLimited
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, s.bodyLimit())
		if err = r.ParseForm(); err == nil {
			in = paste.CreateInput{
				Title: r.PostForm.Get("title"),
				// Browsers submit textarea newlines as CRLF; restore what the user typed.
				Content:    strings.ReplaceAll(r.PostForm.Get("content"), "\r\n", "\n"),
				Language:   r.PostForm.Get("language"),
				Expiration: r.PostForm.Get("expiration"),
				Password:   r.PostForm.Get("password"),
				DeleteKey:  r.PostForm.Get("delete_key"),
			}
		}
	}
	var created *paste.Created
	if err == nil {
		created, err = s.svc.Create(r.Context(), in)
	}
	if err != nil {
		f := s.fail(err)
		if htmx {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(f.status)
			_, _ = io.WriteString(w, template.HTMLEscapeString(f.message))
			return
		}
		data := s.homeData(r)
		in.Password, in.DeleteKey = "", ""
		if in.Language == "" {
			in.Language = paste.Auto
		}
		data.Form, data.Error = in, f.message
		s.render(w, f.status, "home", data)
		return
	}

	id := created.Paste.ID
	// The deletion token travels once in a short-lived cookie scoped to the
	// paste page, so it never appears in a URL, history or access log.
	http.SetCookie(w, &http.Cookie{
		Name:     createdCookie,
		Value:    created.DeletionToken,
		Path:     "/" + id,
		MaxAge:   120,
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.baseURL(r), "https://"),
		SameSite: http.SameSiteStrictMode,
	})
	if htmx {
		w.Header().Set("HX-Redirect", "/"+id)
		w.WriteHeader(http.StatusCreated)
		return
	}
	http.Redirect(w, r, "/"+id, http.StatusSeeOther)
}

type viewPage struct {
	Common
	Paste         *paste.Paste
	Content       string
	Lines         int
	URL           string
	Filename      string
	Markdown      template.HTML
	DeletionToken string
	HasDeleteKey  bool
	Unlocked      bool
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	p, err := s.lookup(r)
	if err != nil {
		s.renderPasteError(w, r, err)
		return
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if p.Encrypted {
		s.renderUnlock(w, r, p, http.StatusOK, "")
		return
	}
	s.renderView(w, r, p, string(p.Content), false)
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	p, err := s.lookup(r)
	if err != nil {
		s.renderPasteError(w, r, err)
		return
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if !p.Encrypted {
		http.Redirect(w, r, "/"+p.ID, http.StatusSeeOther)
		return
	}
	if !s.unlock.Allow(ipFrom(r.Context())) {
		s.renderUnlock(w, r, p, http.StatusTooManyRequests, "Too many attempts. Please wait a minute and try again.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		s.renderUnlock(w, r, p, http.StatusBadRequest, "Invalid request.")
		return
	}
	content, err := s.svc.Open(p, r.PostForm.Get("password"))
	if err != nil {
		f := s.fail(err)
		s.renderUnlock(w, r, p, f.status, f.message)
		return
	}
	s.renderView(w, r, p, content, true)
}

func (s *Server) renderView(w http.ResponseWriter, r *http.Request, p *paste.Paste, content string, unlocked bool) {
	data := viewPage{
		Common:        s.common(r, p.Title, ""),
		Paste:         p,
		Content:       content,
		Lines:         strings.Count(content, "\n") + 1,
		URL:           s.baseURL(r) + "/" + p.ID,
		Filename:      downloadName(p),
		HasDeleteKey:  p.DeleteKeyHash != "",
		Unlocked:      unlocked,
		DeletionToken: s.takeCreatedToken(w, r, p.ID),
	}
	if data.PageTitle == "" {
		data.PageTitle = "Untitled paste"
	}
	if p.Language == "markdown" {
		var buf bytes.Buffer
		if err := markdown.Convert([]byte(content), &buf); err == nil {
			data.Markdown = template.HTML(buf.String()) // safe: goldmark omits raw HTML and unsafe links
		}
	}
	if unlocked || data.DeletionToken != "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	s.render(w, http.StatusOK, "view", data)
}

// takeCreatedToken returns the deletion token handed over right after creation
// and clears its cookie, so the page shows it to the creator's browser once.
func (s *Server) takeCreatedToken(w http.ResponseWriter, r *http.Request, id string) string {
	c, err := r.Cookie(createdCookie)
	if err != nil || c.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: createdCookie, Value: "", Path: "/" + id, MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return c.Value
}

type unlockPage struct {
	Common
	Paste         *paste.Paste
	Error         string
	DeletionToken string
}

func (s *Server) renderUnlock(w http.ResponseWriter, r *http.Request, p *paste.Paste, status int, msg string) {
	title := p.Title
	if title == "" {
		title = "Protected paste"
	}
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, status, "unlock", unlockPage{
		Common:        s.common(r, title, ""),
		Paste:         p,
		Error:         msg,
		DeletionToken: s.takeCreatedToken(w, r, p.ID),
	})
}

func (s *Server) renderPasteError(w http.ResponseWriter, r *http.Request, err error) {
	switch f := s.fail(err); f.status {
	case http.StatusNotFound:
		s.renderError(w, r, f.status, "Paste not found", "This paste may have expired, been removed, or the URL may be incorrect.")
	case http.StatusGone:
		s.renderError(w, r, f.status, "This paste has expired", "The paste is no longer available.")
	case http.StatusTooManyRequests:
		s.renderError(w, r, f.status, "Too many requests", "Please wait a minute and try again.")
	default:
		s.renderError(w, r, f.status, "Something went wrong", "We couldn't process your request. Please try again.")
	}
}

type docsPage struct {
	Common
	BaseURL     string
	MaxBytes    int
	Languages   []paste.Language
	Expirations any
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "docs", docsPage{
		Common:      s.common(r, "API documentation", "docs"),
		BaseURL:     s.baseURL(r),
		MaxBytes:    s.svc.MaxBytes(),
		Languages:   paste.Languages,
		Expirations: paste.Expirations,
	})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "history", struct{ Common }{s.common(r, "History", "history")})
}

func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "privacy", struct{ Common }{s.common(r, "Privacy", "privacy")})
}

// ---- Raw, download, QR ----

// openPlain loads a paste for plain-text endpoints, honouring X-Paste-Password.
func (s *Server) openPlain(w http.ResponseWriter, r *http.Request) (*paste.Paste, string, bool) {
	p, err := s.lookup(r)
	if err == nil && p.Encrypted && r.Header.Get(passwordHeader) != "" && !s.unlock.Allow(ipFrom(r.Context())) {
		err = errRateLimited
	}
	var content string
	if err == nil {
		content, err = s.svc.Open(p, r.Header.Get(passwordHeader))
	}
	if err != nil {
		f := s.fail(err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, f.message+"\n")
		return nil, "", false
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	if p.Encrypted {
		h.Set("Cache-Control", "no-store")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	return p, content, true
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	if _, content, ok := s.openPlain(w, r); ok {
		_, _ = io.WriteString(w, content)
	}
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	p, content, ok := s.openPlain(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": downloadName(p)}))
	_, _ = io.WriteString(w, content)
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	p, err := s.lookup(r)
	if err != nil {
		f := s.fail(err)
		http.Error(w, f.message, f.status)
		return
	}
	png, err := qrcode.Encode(s.baseURL(r)+"/"+p.ID, qrcode.Medium, 320)
	if err != nil {
		f := s.fail(err)
		http.Error(w, f.message, f.status)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(png)
}

// ---- JSON API ----

type apiCreateRequest struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	Language   string `json:"language"`
	Expiration string `json:"expiration"`
	Password   string `json:"password"`
	DeleteKey  string `json:"delete_key"`
}

type apiCreateResponse struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	RawURL        string `json:"raw_url"`
	Language      string `json:"language"`
	ExpiresAt     *int64 `json:"expires_at"`
	DeletionToken string `json:"deletion_token"`
}

type apiPaste struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Language  string `json:"language"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt *int64 `json:"expires_at"`
	Encrypted bool   `json:"encrypted"`
	Size      int64  `json:"size"`
}

func (s *Server) handleAPICreate(w http.ResponseWriter, r *http.Request) {
	if !s.create.Allow(ipFrom(r.Context())) {
		f := classify(errRateLimited)
		w.Header().Set("Retry-After", "60")
		writeError(w, f.status, f.code, f.message)
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "INVALID_REQUEST", "Content-Type must be application/json.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.bodyLimit())
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req apiCreateRequest
	if err := dec.Decode(&req); err != nil {
		if f := classify(err); f.status == http.StatusRequestEntityTooLarge {
			writeError(w, f.status, f.code, f.message)
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request body must be a valid JSON object.")
		return
	}
	if dec.More() {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request body must contain a single JSON object.")
		return
	}
	created, err := s.svc.Create(r.Context(), paste.CreateInput(req))
	if err != nil {
		f := s.fail(err)
		writeError(w, f.status, f.code, f.message)
		return
	}
	p, base := created.Paste, s.baseURL(r)
	w.Header().Set("Location", "/api/pastes/"+p.ID)
	writeJSON(w, http.StatusCreated, apiCreateResponse{
		ID:            p.ID,
		URL:           base + "/" + p.ID,
		RawURL:        base + "/raw/" + p.ID,
		Language:      p.Language,
		ExpiresAt:     unixPtr(p.ExpiresAt),
		DeletionToken: created.DeletionToken,
	})
}

func (s *Server) handleAPIGet(w http.ResponseWriter, r *http.Request) {
	p, err := s.lookup(r)
	if err == nil && p.Encrypted && r.Header.Get(passwordHeader) != "" && !s.unlock.Allow(ipFrom(r.Context())) {
		err = errRateLimited
	}
	var content string
	if err == nil {
		content, err = s.svc.Open(p, r.Header.Get(passwordHeader))
	}
	if err != nil {
		f := s.fail(err)
		writeError(w, f.status, f.code, f.message)
		return
	}
	writeJSON(w, http.StatusOK, apiPaste{
		ID:        p.ID,
		Title:     p.Title,
		Content:   content,
		Language:  p.Language,
		CreatedAt: p.CreatedAt.Unix(),
		ExpiresAt: unixPtr(p.ExpiresAt),
		Encrypted: p.Encrypted,
		Size:      p.Size,
	})
}

func (s *Server) handleAPIDelete(w http.ResponseWriter, r *http.Request) {
	if !s.unlock.Allow(ipFrom(r.Context())) {
		f := classify(errRateLimited)
		writeError(w, f.status, f.code, f.message)
		return
	}
	if err := s.svc.Delete(r.Context(), chi.URLParam(r, "id"), r.Header.Get(deleteHeader), r.Header.Get(deleteKeyHdr)); err != nil {
		f := s.fail(err)
		writeError(w, f.status, f.code, f.message)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func unixPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.Unix()
	return &v
}
