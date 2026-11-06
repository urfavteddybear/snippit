# Snippit

A lightweight, anonymous pastebin for developers. Paste code, logs or config, get a link. No accounts.

- Syntax highlighting for 21 languages (CodeMirror 6), with automatic language detection
- Expiration: 1 hour, 1 day, 7 days, 30 days or never
- Optional password: content is encrypted at rest (scrypt + AES-256-GCM)
- Optional delete key: delete the paste later from any browser
- Raw text, download, copy, line wrap, QR code, Markdown preview
- Local history of the pastes you created, kept only in your browser
- JSON API for scripts and the terminal
- Single Go binary with embedded assets, SQLite (WAL) storage

## Quick start (Docker Compose)

Requirements: Docker with the Compose plugin.

```bash
git clone https://github.com/urfavteddybear/snippit.git
cd snippit
cp .env.example .env
docker compose up -d --build
```

Open http://localhost:8080.

The stack has two services:

| Service   | Role                                                                   |
|-----------|------------------------------------------------------------------------|
| `nginx`   | Reverse proxy on host port `SNIPPIT_PORT` (default `8080`)              |
| `snippit` | The Go app. Not published to the host; reachable only through `nginx`. |

Pastes are stored in the `snippit-data` named volume, so they survive `docker compose down` and container rebuilds. `docker compose down -v` deletes the volume and every paste in it.

## Configuration

Set these in `.env`. Compose passes them to the app.

| Variable                        | Default      | Description                                                         |
|---------------------------------|--------------|---------------------------------------------------------------------|
| `SNIPPIT_PORT`                  | `8080`       | Host port Nginx listens on                                          |
| `SNIPPIT_BASE_URL`              | *(empty)*    | Public origin for generated links, e.g. `https://paste.example.com`. When empty, it is derived from the request. Set it in production. |
| `SNIPPIT_MAX_PASTE_BYTES`       | `524288`     | Maximum size of one paste (512 KiB)                                 |
| `SNIPPIT_MAX_TOTAL_BYTES`       | `1073741824` | Storage quota for all pastes (1 GiB). `0` disables it.             |
| `SNIPPIT_RATE_LIMIT_PER_MINUTE` | `10`         | Paste creations per minute per client IP                            |
| `SNIPPIT_RATE_LIMIT_BURST`      | `5`          | Burst allowance for paste creation                                  |
| `SNIPPIT_CLEANUP_INTERVAL`      | `10m`        | How often expired pastes are deleted                                |
| `SNIPPIT_GITHUB_URL`            | *(empty)*    | Optional GitHub link in the navbar                                  |

These are also read by the app but not wired into `docker-compose.yml`. Add them under `environment:` if you need them.

| Variable                          | Default            | Description                                                  |
|-----------------------------------|--------------------|--------------------------------------------------------------|
| `SNIPPIT_ADDR`                    | `:8080`            | Listen address                                               |
| `SNIPPIT_DB_PATH`                 | `data/snippit.db`  | SQLite file (`/data/snippit.db` in the image)                |
| `SNIPPIT_TRUST_PROXY`             | `false`            | Read the client IP from `X-Real-IP`. Compose sets it to `true`. |
| `SNIPPIT_UNLOCK_LIMIT_PER_MINUTE` | `10`               | Password and delete-key attempts per minute per IP           |
| `SNIPPIT_UNLOCK_LIMIT_BURST`      | `5`                | Burst allowance for those attempts                           |
| `SNIPPIT_MAX_CONCURRENT`          | `100`              | Requests handled at once before queueing                     |
| `SNIPPIT_REQUEST_TIMEOUT`         | `15s`              | Per-request timeout                                          |

## Production notes

**TLS.** The bundled Nginx listens on plain HTTP. Either add a `443` server block with your certificates to `deploy/nginx.conf`, or put Snippit behind a proxy or CDN that terminates TLS. Set `SNIPPIT_BASE_URL` to the `https://` origin either way.

**Client IP behind another proxy.** The app rate-limits by the `X-Real-IP` header, which Nginx sets from `$remote_addr`. If another proxy sits in front of Nginx, every client appears to have that proxy's IP. Configure Nginx's `set_real_ip_from` / `real_ip_header` for your proxy so the real client IP is used.

**Hardening.** Both containers run read-only with `no-new-privileges`. The app runs as a non-root user in a distroless image with all capabilities dropped.

## Backups

The `backup` subcommand writes a consistent snapshot while the server keeps running:

```bash
make backup
```

This writes the snapshot inside the volume (`/data/backup-<timestamp>.db`) and copies it to `./backups/`. Delete old snapshots from the volume yourself when you no longer need them.

To restore, run:

```bash
make restore FILE=backups/snippit-20261106-120000.db
```

This stops the app, replaces `/data/snippit.db` in the volume with the backup, and starts the app again. Every paste created after that backup is lost.

Deleting a paste removes it from the live database. Earlier backups still contain it until they are rotated.

## Deleting pastes

A paste can be deleted in two ways:

- **From the browser that created it.** The server gives that browser a deletion token, which is stored in its local History. The Delete button works there with one click.
- **With a delete key.** If the creator set a delete key, anyone who knows it can delete the paste from any browser, or through the API with the `X-Delete-Key` header.

A paste without a delete key that was created in a browser you no longer have can't be deleted. It still expires on schedule. The public link alone never allows deletion.

## API

Full reference at `/docs` on a running instance.

```bash
# Create
curl -s http://localhost:8080/api/pastes \
  -H 'Content-Type: application/json' \
  -d '{"title":"hello.py","content":"print(1)","expiration":"1d","delete_key":"my-key"}'

# Read raw content
curl -s http://localhost:8080/raw/<id>

# Delete
curl -X DELETE http://localhost:8080/api/pastes/<id> -H 'X-Delete-Key: my-key'
```

| Method   | Path                | Description                                   |
|----------|---------------------|-----------------------------------------------|
| `POST`   | `/api/pastes`       | Create a paste                                |
| `GET`    | `/api/pastes/:id`   | Metadata and content as JSON                  |
| `DELETE` | `/api/pastes/:id`   | Delete (`X-Delete-Key` or `X-Delete-Token`)    |
| `GET`    | `/raw/:id`          | Plain text                                    |
| `GET`    | `/download/:id`     | Plain text as an attachment                   |
| `GET`    | `/qr/:id`           | QR code PNG of the paste URL                  |
| `GET`    | `/healthz`          | Health check                                  |

Password-protected pastes need the `X-Paste-Password` header on the JSON, raw and download endpoints.

## Development

Requirements: Go 1.27+, Node.js 24+.

```bash
make run    # build frontend assets and the binary, serve on :8080
make test   # go vet + go test -race
```

The frontend lives in `frontend/` and is bundled by esbuild and the Tailwind CLI into `internal/web/static/`, which is embedded into the binary. That directory is generated and not committed, so run `make assets` (or any target above) after cloning, before running `go build` or `go test` directly. For live CSS/JS rebuilds, run `cd frontend && npm run watch` and restart the Go server after changes.

```
cmd/snippit/          entry point (serve, backup, healthcheck)
internal/config/      environment configuration
internal/paste/       domain: validation, IDs, encryption, language detection
internal/store/       SQLite repository and migrations
internal/web/         HTTP handlers, middleware, templates
frontend/             CodeMirror, HTMX and Tailwind sources
deploy/nginx.conf     reverse proxy config used by docker compose
```

## Privacy

Snippit stores paste content and metadata, never accounts. Client IPs are held in memory for rate limiting only and are not written to the database or the app logs. See `/privacy` on a running instance.
