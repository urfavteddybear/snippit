# Snippit — Product Requirements Document

**Product:** Snippit
**Version:** 1.0
**Status:** Draft
**Category:** Developer Tools / Pastebin
**Platform:** Web Application

---

## 1. Product Overview

### 1.1 Description

Snippit is a lightweight, anonymous pastebin service designed for developers who need to quickly share code snippets, configuration files, logs, and plain text.

The platform prioritizes speed, simplicity, reliability, and developer experience.

Users can create and share pastes without creating an account or signing in.

### 1.2 Product Vision

"Share code. Not accounts."

Snippit aims to provide the fastest and simplest way to share text and code snippets through a clean, developer-focused interface.

### 1.3 Core Principles

- Zero friction: No registration or authentication.
- Fast by default: Minimal dependencies and lightweight pages.
- Developer-first: Syntax highlighting, raw access, and clean code presentation.
- Privacy-conscious: No public paste discovery or user profiles.
- Reliable: Predictable storage, expiration, and retrieval behavior.

---

## 2. Target Users

### Primary Users
- Software developers.
- System administrators and DevOps engineers.
- Students learning programming.
- Open-source contributors.
- Technical support teams.

### Common Use Cases
- Sharing code snippets.
- Sharing server logs.
- Sharing configuration files.
- Sharing terminal output.
- Sharing JSON, YAML, and SQL data.
- Sending temporary debugging information.

---

## 3. Product Scope

### 3.1 MVP Features

| Feature | Priority |
|---|---|
| Anonymous paste creation | P0 |
| Code editor | P0 |
| Syntax highlighting | P0 |
| Automatic language detection | P1 |
| Custom paste title | P1 |
| Paste expiration | P0 |
| Unique shareable URL | P0 |
| Raw text endpoint | P0 |
| Copy paste URL | P0 |
| Copy raw content | P0 |
| Dark developer UI | P0 |
| Responsive layout | P0 |
| Rate limiting | P0 |
| Maximum content size | P0 |
| Paste deletion | P1 |
| Password-protected paste | P2 |
| Markdown preview | P2 |
| Download paste | P2 |
| QR code sharing | P2 |

P0 = Required for initial release.
P1 = Important follow-up.
P2 = Optional enhancement.

### 3.2 Explicitly Out of Scope

The initial release will NOT include:

- User registration.
- User login.
- User profiles.
- Personal paste dashboard.
- Public paste directory.
- Social features.
- Comments or reactions.
- Team collaboration.
- Paid subscriptions.
- Cloud synchronization.

---

## 4. User Experience & Workflows

### 4.1 Create Paste

1. User opens the homepage.
2. User enters or pastes content into the editor.
3. User optionally enters a title.
4. User selects a programming language or uses automatic detection.
5. User selects an expiration period.
6. User clicks Create Paste.
7. Backend validates the request.
8. Backend stores the paste.
9. User is redirected to the newly created paste page.
10. User can copy and share the URL.

### 4.2 View Paste

1. User opens a shared URL.
2. Backend retrieves the paste.
3. Backend checks whether it exists and has expired.
4. The page renders the content with syntax highlighting.
5. User can copy or download the content.

### 4.3 Raw Paste

Users can access the original content through:

`GET /raw/:id`

The response must use an appropriate plain-text content type and must not render pasted HTML.

### 4.4 Expired Paste

If a paste expires, the application must return a dedicated expiration page or HTTP 410 response for raw access.

---

## 5. Functional Requirements

### FR-01: Paste Creation

- Accept plain text and source code.
- Support optional title.
- Support programming language selection.
- Generate unpredictable paste IDs.
- Reject empty content.
- Enforce configurable maximum content size.
- Store creation timestamp.

### FR-02: Paste Retrieval

- Retrieve paste using its unique ID.
- Display syntax-highlighted content.
- Display title, language, creation date, and expiration information.
- Provide raw content access.
- Return 404 for nonexistent IDs.
- Return 410 for expired pastes.

### FR-03: Expiration

Initial expiration options:

- 1 hour.
- 1 day.
- 7 days.
- 30 days.
- Never.

The expiration timestamp must be stored in UTC.

A background cleanup process should periodically remove expired records.

### FR-04: Sharing

- Generate a unique URL.
- Provide copy URL functionality.
- Provide copy raw content functionality.
- Allow sharing without requiring authentication.

### FR-05: Security

- Rate-limit paste creation by IP.
- Validate all input.
- Limit request body size.
- Use cryptographically secure random ID generation.
- Prevent HTML and JavaScript execution in pasted content.
- Use parameterized database queries.
- Prevent SQL injection.
- Prevent path traversal.
- Configure secure HTTP response headers.
- Do not expose server filesystem paths.
- Avoid logging full paste contents.
- Do not expose internal database errors.

### FR-06: Abuse Prevention

- Configurable per-IP creation limits.
- Configurable maximum paste size.
- Reject malformed requests.
- Apply request timeouts.
- Protect against excessive concurrent requests.
- Consider a global storage quota.

---

## 6. Non-Functional Requirements

### Performance

- Minimal application startup time.
- Low memory footprint.
- Fast paste retrieval.
- Minimal external dependencies.
- Efficient database queries.
- Static assets should be cacheable.

Initial performance targets (to be validated through benchmarking):

| Metric | Target |
|---|---|
| Homepage response | p95 < 150 ms |
| Raw paste retrieval | p95 < 100 ms |
| Paste creation | p95 < 200 ms |
| Application memory | Target < 100 MB |
| Startup time | Target < 1 second |

Targets refer to application-side performance under a defined benchmark environment, excluding external network latency.

### Reliability

- Persistent storage.
- Graceful error handling.
- Database backup capability.
- Safe shutdown.
- Database integrity checks.

### Scalability

- Support concurrent HTTP requests.
- Use connection pooling where appropriate.
- Keep application stateless apart from persistent storage.
- Support future migration to PostgreSQL or object storage.

---

## 7. Technical Architecture

### 7.1 Stack

| Component | Technology |
|---|---|
| Language | Go |
| HTTP Router | Chi |
| Frontend | HTML Templates |
| Interactivity | HTMX |
| Styling | Tailwind CSS |
| Editor | CodeMirror 6 |
| Database | SQLite |
| Database Mode | WAL |
| Syntax Highlighting | CodeMirror Language Packages |
| Reverse Proxy | Nginx |
| Deployment | Docker Compose |

### 7.2 Architecture

    Internet
       |
      Nginx
       |
    Go HTTP Server
       |
    Middleware
       |
    Handlers
       |
    Services
       |
    Repository
       |
    SQLite WAL

### 7.3 Database Schema

**Table: pastes**

| Column | Type | Description |
|---|---|---|
| id | TEXT | Unique paste identifier |
| title | TEXT | Optional paste title |
| content | TEXT | Paste content |
| language | TEXT | Language identifier |
| created_at | INTEGER | Unix timestamp |
| expires_at | INTEGER | Nullable expiration timestamp |
| deletion_token_hash | TEXT | Optional hashed deletion capability |

Implementation notes:

- Store timestamps in UTC.
- Use an index on `expires_at`.
- Use an index or primary key on `id`.
- Store content without HTML transformation.
- Never store plaintext deletion tokens.

---

## 8. API Specification

### POST /api/pastes

Create a paste.

Request:

    {
      "title": "Example",
      "content": "console.log('Hello World')",
      "language": "javascript",
      "expiration": "7d"
    }

Response: `201 Created`

    {
      "id": "a8Kx92Lm",
      "url": "https://Snippit.example/a8Kx92Lm",
      "raw_url": "https://Snippit.example/raw/a8Kx92Lm",
      "expires_at": 1794000000
    }

### GET /api/pastes/:id

Retrieve paste metadata and content.

### GET /:id

Render paste page.

### GET /raw/:id

Retrieve raw paste content.

### GET /download/:id

Download paste as a text file.

### DELETE /api/pastes/:id

Delete a paste using its optional deletion capability.

No account authentication is required. Possession of a deletion capability is required for deletion; the public paste ID alone must not authorize deletion.

### Error Responses

Use consistent JSON error structures:

    {
      "error": {
        "code": "PASTE_NOT_FOUND",
        "message": "The requested paste does not exist."
      }
    }

Suggested error codes:

- INVALID_CONTENT
- CONTENT_TOO_LARGE
- RATE_LIMITED
- PASTE_NOT_FOUND
- PASTE_EXPIRED
- INVALID_EXPIRATION
- INTERNAL_ERROR

---

## 9. Privacy & Data Retention

- No user account information is collected.
- IP addresses may be processed temporarily for abuse prevention.
- IP addresses should not be stored indefinitely.
- Paste content is retained until expiration or deletion.
- Expired pastes should be physically deleted by cleanup jobs.
- Never promise that deleting a paste immediately removes all backup copies.
- Provide a privacy policy explaining retention and operational logging.

---

## 10. Deployment

Initial deployment:

- Single Go application instance.
- SQLite database on persistent storage.
- Nginx reverse proxy.
- Docker Compose.
- TLS termination at Nginx or an upstream proxy.

Suggested container services:

1. `Snippit`: Application.
2. `nginx`: Reverse proxy.

The SQLite database must live on a persistent local volume, not an ephemeral container filesystem.

---

## 11. Acceptance Criteria

The MVP is considered ready when:

- [ ] Homepage loads without authentication.
- [ ] Users can create a paste.
- [ ] Paste IDs are unpredictable.
- [ ] Generated links work.
- [ ] Syntax highlighting works for supported languages.
- [ ] Raw endpoint returns original content.
- [ ] Expiration works correctly.
- [ ] Expired content is cleaned up.
- [ ] Content cannot execute as HTML or JavaScript.
- [ ] Rate limiting works.
- [ ] Maximum paste size is enforced.
- [ ] Responsive design works on mobile and desktop.
- [ ] Database persists across container restarts.
- [ ] Application handles invalid and missing paste IDs gracefully.

---

## 12. Future Roadmap

### Phase 1 — MVP
Anonymous paste creation, viewing, raw access, expiration, and basic security.

### Phase 2 — Enhanced Developer Experience
Password-protected pastes, Markdown preview, download, automatic language detection, and deletion capability.

### Phase 3 — Performance & Reliability
Benchmarking, database optimization, storage management, monitoring, and improved abuse protection.

### Phase 4 — Advanced Features
Optional encrypted pastes, API improvements, and configurable deployment integrations.

---

**End of Product Requirements Document.**
