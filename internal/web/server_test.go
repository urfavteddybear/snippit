package web

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"snippit/internal/config"
	"snippit/internal/paste"
	"snippit/internal/store"
)

type testEnv struct {
	t   *testing.T
	srv *Server
	db  *store.SQLite
}

func newEnv(t *testing.T, mod func(*config.Config)) *testEnv {
	t.Helper()
	cfg := config.Config{
		MaxPasteBytes:   1024,
		CreatePerMinute: 600,
		CreateBurst:     100,
		UnlockPerMinute: 600,
		UnlockBurst:     100,
		MaxConcurrent:   10,
		RequestTimeout:  5 * time.Second,
		BaseURL:         "https://snippit.test",
	}
	if mod != nil {
		mod(&cfg)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := paste.NewService(db, cfg.MaxPasteBytes, cfg.MaxTotalBytes)
	srv, err := New(cfg, svc, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return &testEnv{t: t, srv: srv, db: db}
}

func (e *testEnv) do(req *http.Request) (*http.Response, string) {
	e.t.Helper()
	if req.RemoteAddr == "" || req.RemoteAddr == "192.0.2.1:1234" {
		req.RemoteAddr = "203.0.113.7:5555"
	}
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func (e *testEnv) get(path string, headers ...string) (*http.Response, string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return e.do(req)
}

func (e *testEnv) apiCreate(body string) (*http.Response, string) {
	req := httptest.NewRequest(http.MethodPost, "/api/pastes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return e.do(req)
}

func (e *testEnv) mustCreate(body string) apiCreateResponse {
	e.t.Helper()
	res, b := e.apiCreate(body)
	if res.StatusCode != http.StatusCreated {
		e.t.Fatalf("create: %d %s", res.StatusCode, b)
	}
	var out apiCreateResponse
	if err := json.Unmarshal([]byte(b), &out); err != nil {
		e.t.Fatal(err)
	}
	return out
}

func errorCode(t *testing.T, body string) string {
	t.Helper()
	var e apiError
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("not a JSON error: %q", body)
	}
	return e.Error.Code
}

func TestAPICreateAndRaw(t *testing.T) {
	e := newEnv(t, nil)
	content := "line1\r\nline2\n\ttabbed <b>x</b>\n"
	body, _ := json.Marshal(map[string]string{"title": "t", "content": content, "language": "plaintext", "expiration": "1d"})
	out := e.mustCreate(string(body))

	if out.URL != "https://snippit.test/"+out.ID || out.RawURL != "https://snippit.test/raw/"+out.ID {
		t.Errorf("urls = %q %q", out.URL, out.RawURL)
	}
	if out.ExpiresAt == nil || out.DeletionToken == "" {
		t.Errorf("missing expires_at or deletion_token: %+v", out)
	}

	res, raw := e.get("/raw/" + out.ID)
	if res.StatusCode != http.StatusOK || raw != content {
		t.Fatalf("raw = %d %q", res.StatusCode, raw)
	}
	for k, want := range map[string]string{
		"Content-Type":            "text/plain; charset=utf-8",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
	} {
		if got := res.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	res, b := e.get("/api/pastes/" + out.ID)
	var p apiPaste
	if res.StatusCode != http.StatusOK || json.Unmarshal([]byte(b), &p) != nil || p.Content != content || p.Title != "t" {
		t.Fatalf("api get = %d %s", res.StatusCode, b)
	}
}

func TestAPICreateErrors(t *testing.T) {
	e := newEnv(t, nil)
	cases := []struct {
		name, ctype, body string
		status            int
		code              string
	}{
		{"empty", "application/json", `{"content":"   "}`, 400, "INVALID_CONTENT"},
		{"too large", "application/json", `{"content":"` + strings.Repeat("a", 1025) + `"}`, 413, "CONTENT_TOO_LARGE"},
		{"body too large", "application/json", `{"content":"` + strings.Repeat("a", 200_000) + `"}`, 413, "CONTENT_TOO_LARGE"},
		{"malformed", "application/json", `{"content":`, 400, "INVALID_REQUEST"},
		{"unknown field", "application/json", `{"content":"x","admin":true}`, 400, "INVALID_REQUEST"},
		{"two objects", "application/json", `{"content":"x"}{"content":"y"}`, 400, "INVALID_REQUEST"},
		{"content type", "text/plain", `{"content":"x"}`, 415, "INVALID_REQUEST"},
		{"expiration", "application/json", `{"content":"x","expiration":"1y"}`, 400, "INVALID_EXPIRATION"},
		{"language", "application/json", `{"content":"x","language":"brainfuck"}`, 400, "INVALID_LANGUAGE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/pastes", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.ctype)
			res, b := e.do(req)
			if res.StatusCode != tc.status || errorCode(t, b) != tc.code {
				t.Fatalf("got %d %s, want %d %s", res.StatusCode, b, tc.status, tc.code)
			}
		})
	}
}

func TestNotFoundAndExpired(t *testing.T) {
	e := newEnv(t, nil)
	past := time.Now().Add(-time.Hour).UTC()
	err := e.db.Insert(context.Background(), &paste.Paste{
		ID: "Expired123", Content: []byte("old"), Language: "plaintext",
		CreatedAt: past.Add(-time.Hour), ExpiresAt: &past, Size: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path   string
		status int
		text   string
	}{
		{"/Missing123", 404, "Paste not found"},
		{"/Expired123", 410, "This paste has expired"},
		{"/raw/Missing123", 404, "does not exist"},
		{"/raw/Expired123", 410, "no longer available"},
		{"/raw/..%2f..%2fetc%2fpasswd", 404, ""},
		{"/not-a-valid-id!", 404, "Paste not found"},
		{"/a/b/c", 404, "Page not found"},
	} {
		res, b := e.get(tc.path)
		if res.StatusCode != tc.status || !strings.Contains(b, tc.text) {
			t.Errorf("GET %s = %d, want %d containing %q", tc.path, res.StatusCode, tc.status, tc.text)
		}
	}

	res, b := e.get("/api/pastes/Expired123")
	if res.StatusCode != 410 || errorCode(t, b) != "PASTE_EXPIRED" {
		t.Errorf("api expired = %d %s", res.StatusCode, b)
	}
	res, b = e.get("/api/pastes/Missing123")
	if res.StatusCode != 404 || errorCode(t, b) != "PASTE_NOT_FOUND" {
		t.Errorf("api missing = %d %s", res.StatusCode, b)
	}
}

func TestViewEscapesContent(t *testing.T) {
	e := newEnv(t, nil)
	evil := `</code></pre><script>alert(1)</script><img src=x onerror=alert(2)>`
	body, _ := json.Marshal(map[string]string{"title": `<script>alert(3)</script>`, "content": evil, "language": "html"})
	out := e.mustCreate(string(body))

	res, page := e.get("/" + out.ID)
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	for _, bad := range []string{"<script>alert", "<img src=x", "</code></pre><script>"} {
		if strings.Contains(page, bad) {
			t.Errorf("page contains unescaped %q", bad)
		}
	}
	if !strings.Contains(page, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("escaped content missing from page")
	}
}

func TestMarkdownPreviewIsSafe(t *testing.T) {
	e := newEnv(t, nil)
	md := "# Hello\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(2)>\n\n[click](javascript:alert(3))\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	body, _ := json.Marshal(map[string]string{"content": md, "language": "markdown"})
	out := e.mustCreate(string(body))

	_, page := e.get("/" + out.ID)
	preview := regexp.MustCompile(`(?s)<div id="panel-preview"[^>]*>(.*?)</div>\s*<div id="panel-code"`).FindStringSubmatch(page)
	if preview == nil {
		t.Fatal("markdown preview panel missing")
	}
	html := preview[1]
	if !strings.Contains(html, "<h1>Hello</h1>") || !strings.Contains(html, "<table>") {
		t.Errorf("markdown not rendered: %s", html)
	}
	for _, bad := range []string{"<script", "onerror", "javascript:"} {
		if strings.Contains(html, bad) {
			t.Errorf("preview contains %q: %s", bad, html)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	e := newEnv(t, nil)
	res, page := e.get("/")
	csp := res.Header.Get("Content-Security-Policy")
	m := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(csp)
	if m == nil {
		t.Fatalf("CSP without nonce: %q", csp)
	}
	if !strings.Contains(page, `<meta name="csp-nonce" content="`+m[1]+`">`) {
		t.Error("page nonce does not match CSP header")
	}
	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP allows unsafe sources: %q", csp)
	}
	for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if res.Header.Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
	_, page2 := e.get("/")
	if page == page2 {
		t.Error("nonce is not unique per request")
	}
}

func TestRateLimit(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.CreatePerMinute, c.CreateBurst = 1, 2 })
	for i := 0; i < 2; i++ {
		if res, b := e.apiCreate(`{"content":"x"}`); res.StatusCode != 201 {
			t.Fatalf("create %d: %d %s", i, res.StatusCode, b)
		}
	}
	res, b := e.apiCreate(`{"content":"x"}`)
	if res.StatusCode != 429 || errorCode(t, b) != "RATE_LIMITED" {
		t.Fatalf("third create = %d %s", res.StatusCode, b)
	}

	// Another IP has its own bucket.
	req := httptest.NewRequest(http.MethodPost, "/api/pastes", strings.NewReader(`{"content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.9:1111"
	if res, _ := e.do(req); res.StatusCode != 201 {
		t.Fatalf("other IP = %d", res.StatusCode)
	}

	// X-Real-IP is ignored unless the proxy is trusted.
	req = httptest.NewRequest(http.MethodPost, "/api/pastes", strings.NewReader(`{"content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "192.0.2.200")
	if res, _ := e.do(req); res.StatusCode != 429 {
		t.Fatalf("spoofed X-Real-IP bypassed limit: %d", res.StatusCode)
	}
}

func TestTrustedProxyIP(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.CreatePerMinute, c.CreateBurst, c.TrustProxy = 1, 1, true })
	send := func(ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/pastes", strings.NewReader(`{"content":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Real-IP", ip)
		res, _ := e.do(req)
		return res.StatusCode
	}
	if send("192.0.2.10") != 201 || send("192.0.2.10") != 429 || send("192.0.2.11") != 201 {
		t.Fatal("rate limit is not keyed on X-Real-IP behind a trusted proxy")
	}
}

func TestFormCreateAndDeletionToken(t *testing.T) {
	e := newEnv(t, nil)
	form := url.Values{"title": {"form"}, "content": {"a\r\nb"}, "language": {"auto"}, "expiration": {"1h"}}
	req := httptest.NewRequest(http.MethodPost, "/pastes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, _ := e.do(req)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	id := strings.TrimPrefix(loc, "/")
	if !paste.ValidID(id) || strings.Contains(loc, "#") || strings.Contains(loc, "?") {
		t.Fatalf("Location = %q", loc)
	}
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == createdCookie {
			cookie = c
		}
	}
	if cookie == nil || cookie.Path != "/"+id || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || !cookie.Secure {
		t.Fatalf("deletion cookie = %+v", cookie)
	}

	if _, raw := e.get("/raw/" + id); raw != "a\nb" {
		t.Errorf("CRLF not normalised: %q", raw)
	}

	req = httptest.NewRequest(http.MethodGet, "/"+id, nil)
	req.AddCookie(&http.Cookie{Name: createdCookie, Value: cookie.Value})
	res, page := e.do(req)
	if !strings.Contains(page, `data-token="`+cookie.Value+`"`) {
		t.Error("deletion token not shown after creation")
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("page with token is cacheable: %q", res.Header.Get("Cache-Control"))
	}
	cleared := false
	for _, c := range res.Cookies() {
		cleared = cleared || (c.Name == createdCookie && c.MaxAge < 0)
	}
	if !cleared {
		t.Error("deletion cookie not cleared")
	}

	if _, page := e.get("/" + id); strings.Contains(page, `id="created"`) {
		t.Error("deletion token shown without cookie")
	}
}

func TestFormCreateErrors(t *testing.T) {
	e := newEnv(t, nil)
	post := func(htmx bool) (*http.Response, string) {
		form := url.Values{"title": {"keep me"}, "content": {"  "}, "language": {"go"}}
		req := httptest.NewRequest(http.MethodPost, "/pastes", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		return e.do(req)
	}
	res, page := post(false)
	if res.StatusCode != 400 || !strings.Contains(page, "Content must be non-empty") || !strings.Contains(page, `value="keep me"`) {
		t.Errorf("plain form error = %d", res.StatusCode)
	}
	res, frag := post(true)
	if res.StatusCode != 400 || strings.Contains(frag, "<html") || !strings.Contains(frag, "Content must be non-empty") {
		t.Errorf("htmx error = %d %q", res.StatusCode, frag)
	}
}

func TestHTMXCreateRedirect(t *testing.T) {
	e := newEnv(t, nil)
	form := url.Values{"content": {"hello"}}
	req := httptest.NewRequest(http.MethodPost, "/pastes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	res, _ := e.do(req)
	if res.StatusCode != http.StatusCreated || !paste.ValidID(strings.TrimPrefix(res.Header.Get("HX-Redirect"), "/")) {
		t.Fatalf("htmx create = %d HX-Redirect=%q", res.StatusCode, res.Header.Get("HX-Redirect"))
	}
}

func TestPasswordProtected(t *testing.T) {
	e := newEnv(t, nil)
	out := e.mustCreate(`{"title":"secret","content":"TOKEN=abc","password":"pw"}`)

	res, page := e.get("/" + out.ID)
	if res.StatusCode != 200 || !strings.Contains(page, "password-protected") || strings.Contains(page, "TOKEN=abc") {
		t.Fatalf("locked page = %d", res.StatusCode)
	}
	if res, _ := e.get("/raw/" + out.ID); res.StatusCode != 401 {
		t.Errorf("raw without password = %d", res.StatusCode)
	}
	if res, _ := e.get("/raw/"+out.ID, passwordHeader, "nope"); res.StatusCode != 403 {
		t.Errorf("raw wrong password = %d", res.StatusCode)
	}
	if res, b := e.get("/raw/"+out.ID, passwordHeader, "pw"); res.StatusCode != 200 || b != "TOKEN=abc" {
		t.Errorf("raw right password = %d %q", res.StatusCode, b)
	}
	if res, b := e.get("/api/pastes/" + out.ID); res.StatusCode != 401 || errorCode(t, b) != "PASSWORD_REQUIRED" {
		t.Errorf("api without password = %d %s", res.StatusCode, b)
	}

	unlock := func(pw string) (*http.Response, string) {
		req := httptest.NewRequest(http.MethodPost, "/"+out.ID+"/unlock", strings.NewReader(url.Values{"password": {pw}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return e.do(req)
	}
	if res, page := unlock("bad"); res.StatusCode != 403 || !strings.Contains(page, "incorrect") {
		t.Errorf("unlock bad = %d", res.StatusCode)
	}
	res, page = unlock("pw")
	if res.StatusCode != 200 || !strings.Contains(page, "TOKEN=abc") || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("unlock good = %d cache=%q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	if !strings.Contains(page, `data-action="raw"`) {
		t.Error("unlocked view should use client-side raw (server raw needs the password)")
	}
}

func TestUnlockRateLimit(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.UnlockPerMinute, c.UnlockBurst = 1, 2 })
	out := e.mustCreate(`{"content":"x","password":"pw"}`)
	var last int
	for i := 0; i < 3; i++ {
		res, _ := e.get("/raw/"+out.ID, passwordHeader, "guess")
		last = res.StatusCode
	}
	if last != 429 {
		t.Fatalf("third guess = %d, want 429", last)
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t, nil)
	out := e.mustCreate(`{"content":"bye"}`)
	del := func(token string) (*http.Response, string) {
		req := httptest.NewRequest(http.MethodDelete, "/api/pastes/"+out.ID, nil)
		if token != "" {
			req.Header.Set(deleteHeader, token)
		}
		return e.do(req)
	}
	if res, b := del(""); res.StatusCode != 403 || errorCode(t, b) != "INVALID_DELETE_KEY" {
		t.Errorf("no token = %d %s", res.StatusCode, b)
	}
	if res, _ := del("wrong"); res.StatusCode != 403 {
		t.Errorf("wrong token = %d", res.StatusCode)
	}
	if res, _ := del(out.DeletionToken); res.StatusCode != 204 {
		t.Fatalf("delete = %d", res.StatusCode)
	}
	if res, _ := e.get("/" + out.ID); res.StatusCode != 404 {
		t.Errorf("after delete = %d", res.StatusCode)
	}
}

func TestDownload(t *testing.T) {
	e := newEnv(t, nil)
	out := e.mustCreate(`{"title":"../../my script","content":"echo hi","language":"bash"}`)
	res, b := e.get("/download/" + out.ID)
	if res.StatusCode != 200 || b != "echo hi" {
		t.Fatalf("download = %d %q", res.StatusCode, b)
	}
	if cd := res.Header.Get("Content-Disposition"); cd != `attachment; filename=my-script.sh` {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestQR(t *testing.T) {
	e := newEnv(t, nil)
	out := e.mustCreate(`{"content":"x"}`)
	res, b := e.get("/qr/" + out.ID)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" || !strings.HasPrefix(b, "\x89PNG") {
		t.Fatalf("qr = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if res, _ := e.get("/qr/Missing123"); res.StatusCode != 404 {
		t.Errorf("qr missing = %d", res.StatusCode)
	}
}

func TestStaticAssets(t *testing.T) {
	e := newEnv(t, nil)
	res, _ := e.get("/static/app.js?v=x")
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("app.js = %d cache=%q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	for _, p := range []string{"/static/", "/static/chunks/", "/static/../templates/layout.html"} {
		if res, _ := e.get(p); res.StatusCode == 200 {
			t.Errorf("GET %s = 200, want error", p)
		}
	}
}

func TestPagesRender(t *testing.T) {
	e := newEnv(t, nil)
	for _, p := range []string{"/", "/docs", "/privacy", "/history"} {
		if res, _ := e.get(p); res.StatusCode != 200 {
			t.Errorf("GET %s = %d", p, res.StatusCode)
		}
	}
	if res, _ := e.get("/healthz"); res.StatusCode != 200 {
		t.Errorf("healthz = %d", res.StatusCode)
	}
}

func TestDownloadName(t *testing.T) {
	cases := []struct{ title, lang, want string }{
		{"", "go", "AbCdEfGh12.go"},
		{"main.go", "go", "main.go"},
		{"hello world", "python", "hello-world.py"},
		{"../../etc/passwd", "plaintext", "etcpasswd.txt"},
		{"x", "dockerfile", "Dockerfile"},
		{"日本語", "rust", "AbCdEfGh12.rs"},
	}
	for _, tc := range cases {
		got := downloadName(&paste.Paste{ID: "AbCdEfGh12", Title: tc.title, Language: tc.lang})
		if got != tc.want {
			t.Errorf("downloadName(%q, %s) = %q, want %q", tc.title, tc.lang, got, tc.want)
		}
	}
}

func TestDeleteWithKey(t *testing.T) {
	e := newEnv(t, nil)
	out := e.mustCreate(`{"content":"bye","delete_key":"s3cret"}`)
	if _, page := e.get("/" + out.ID); !strings.Contains(page, `data-has-delete-key="true"`) {
		t.Error("view does not advertise the delete key")
	}
	del := func(key string) int {
		req := httptest.NewRequest(http.MethodDelete, "/api/pastes/"+out.ID, nil)
		req.Header.Set(deleteKeyHdr, key)
		res, _ := e.do(req)
		return res.StatusCode
	}
	if code := del("wrong"); code != 403 {
		t.Errorf("wrong key = %d", code)
	}
	if code := del("s3cret"); code != 204 {
		t.Fatalf("right key = %d", code)
	}

	plain := e.mustCreate(`{"content":"x"}`)
	if _, page := e.get("/" + plain.ID); !strings.Contains(page, `data-has-delete-key="false"`) {
		t.Error("paste without key advertises one")
	}
}

func TestFormDeleteKeyNotEchoed(t *testing.T) {
	e := newEnv(t, nil)
	form := url.Values{"content": {" "}, "delete_key": {"dont-echo-me"}, "password": {"nor-me"}}
	req := httptest.NewRequest(http.MethodPost, "/pastes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, page := e.do(req)
	if strings.Contains(page, "dont-echo-me") || strings.Contains(page, "nor-me") {
		t.Error("secret re-rendered into the form after an error")
	}
}

func TestCreatedTokenOnUnlockPage(t *testing.T) {
	e := newEnv(t, nil)
	form := url.Values{"content": {"secret"}, "password": {"pw"}}
	req := httptest.NewRequest(http.MethodPost, "/pastes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, _ := e.do(req)
	id := strings.TrimPrefix(res.Header.Get("Location"), "/")
	var token string
	for _, c := range res.Cookies() {
		if c.Name == createdCookie {
			token = c.Value
		}
	}
	req = httptest.NewRequest(http.MethodGet, "/"+id, nil)
	req.AddCookie(&http.Cookie{Name: createdCookie, Value: token})
	if _, page := e.do(req); !strings.Contains(page, `data-token="`+token+`"`) || strings.Contains(page, ">secret<") {
		t.Error("unlock page should hand the token to the creator without revealing content")
	}
}
