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

The stack is a single `snippit` container, published on host port `SNIPPIT_PORT` (default `8080`). No separate web server is needed: the app handles rate limiting, request size limits, timeouts, security headers and compression itself.

Pastes are stored in the `snippit-data` named volume, so they survive `docker compose down` and container rebuilds. `docker compose down -v` deletes the volume and every paste in it.

## Without Docker

Snippit is a single binary with the web assets built in. It needs no Go, Node.js or database server at runtime.

### 1. Get the binary

Download the archive for your platform from [Releases](https://github.com/urfavteddybear/snippit/releases): Linux, macOS and Windows on amd64 and arm64. Check it against `SHA256SUMS`, then extract it:

```bash
tar -xzf snippit_<version>_linux_amd64.tar.gz
cd snippit_<version>_linux_amd64
./snippit version
```

Or build from source (Go 1.27+, Node.js 24+):

```bash
git clone https://github.com/urfavteddybear/snippit.git
cd snippit
make build        # produces ./snippit
```

### 2. Run it

```bash
./snippit
```

It listens on `:8080` and creates its database at `data/snippit.db`, relative to the current directory. Configure it with environment variables. `snippit.env` in the archive lists them all:

```bash
set -a; . ./snippit.env; set +a
./snippit
```

On Windows, run `snippit.exe` and set variables with `set NAME=value` (cmd) or `$env:NAME="value"` (PowerShell).

### 3. Run as a service (Linux, systemd)

The Linux archive includes `snippit.service`, which runs Snippit as a dedicated `snippit` user with the database in `/var/lib/snippit` and a hardened sandbox:

```bash
sudo install -m 755 snippit /usr/local/bin/snippit
sudo useradd --system --home-dir /var/lib/snippit --shell /usr/sbin/nologin snippit
sudo install -d -m 755 /etc/snippit
sudo install -m 640 -g snippit snippit.env /etc/snippit/snippit.env
sudo install -m 644 snippit.service /etc/systemd/system/snippit.service
sudo systemctl daemon-reload
sudo systemctl enable --now snippit
```

Edit `/etc/snippit/snippit.env` to change settings, then `sudo systemctl restart snippit`. Logs: `journalctl -u snippit -f`.

To upgrade, replace `/usr/local/bin/snippit` and restart the service. Database migrations run automatically on startup.

For HTTPS, put a reverse proxy in front and set `SNIPPIT_ADDR=127.0.0.1:8080`. See [Behind a reverse proxy](#behind-a-reverse-proxy).

## Configuration

With Docker, set these in `.env` and Compose passes them to the app. Without Docker, set them as environment variables (see `deploy/snippit.env`).

| Variable                        | Default      | Description                                                         |
|---------------------------------|--------------|---------------------------------------------------------------------|
| `SNIPPIT_PORT`                  | `8080`       | Host port the app is published on                                   |
| `SNIPPIT_TRUST_PROXY`           | `false`      | Read the client IP from `X-Real-IP`. See [Behind a reverse proxy](#behind-a-reverse-proxy). |
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
| `SNIPPIT_UNLOCK_LIMIT_PER_MINUTE` | `10`               | Password and delete-key attempts per minute per IP           |
| `SNIPPIT_UNLOCK_LIMIT_BURST`      | `5`                | Burst allowance for those attempts                           |
| `SNIPPIT_NOTFOUND_LIMIT_PER_MINUTE` | `30`             | Lookups of nonexistent pastes per minute per IP              |
| `SNIPPIT_NOTFOUND_LIMIT_BURST`    | `30`               | Burst allowance for those lookups                            |
| `SNIPPIT_MAX_CONCURRENT`          | `100`              | Requests handled at once before queueing                     |
| `SNIPPIT_REQUEST_TIMEOUT`         | `15s`              | Per-request timeout                                          |

## Production notes

**HTTPS.** Snippit serves plain HTTP. For a public deployment, put it behind something that terminates TLS: your existing reverse proxy (Caddy, Traefik, Nginx), a load balancer, or a tunnel such as Cloudflare Tunnel. Set `SNIPPIT_BASE_URL` to the `https://` origin.

**Paste links.** Links use 6-character random IDs (about 57 billion combinations) so they are short to share. To keep the space from being scanned, each client IP gets a budget of lookups that hit no paste (`SNIPPIT_NOTFOUND_LIMIT_*`). When it runs out, every paste lookup from that IP returns `429` until the budget refills. A scanner spread over many IPs is slowed down, not stopped, so use a password for anything sensitive. Links created before the switch to 6 characters keep working.

**Hardening.** The container runs read-only with `no-new-privileges`, as a non-root user in a distroless image with all capabilities dropped.

### Behind a reverse proxy

Snippit rate-limits by client IP. When it is exposed directly, it uses the connection's address and ignores headers. Behind a proxy, every request would appear to come from the proxy, so:

1. Make the proxy **overwrite** `X-Real-IP` with the client address. Appending is not enough.
2. Set `SNIPPIT_TRUST_PROXY=true`.
3. Make sure the app port is reachable only through the proxy, e.g. publish it as `127.0.0.1:8080:8080` in `docker-compose.yml`.

Never enable `SNIPPIT_TRUST_PROXY` while the app is directly reachable: clients could send a fake `X-Real-IP` and bypass rate limits.

Caddy:

```caddyfile
paste.example.com {
    reverse_proxy 127.0.0.1:8080 {
        header_up X-Real-IP {remote_host}
    }
}
```

Nginx:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $http_host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    client_max_body_size 2m;
}
```

Cloudflare (proxy or Tunnel) does not send `X-Real-IP`. Add a Request Header Transform Rule that sets `X-Real-IP` to the dynamic value `ip.src`.

## Backups

The `backup` subcommand writes a consistent snapshot while the server keeps running. It reads `SNIPPIT_DB_PATH` to find the database.

### Docker

```bash
make backup
```

This writes the snapshot inside the volume (`/data/backup-<timestamp>.db`) and copies it to `./backups/`. Delete old snapshots from the volume yourself when you no longer need them.

To restore, run:

```bash
make restore FILE=backups/snippit-20261106-120000.db
```

This stops the app, replaces `/data/snippit.db` in the volume with the backup, and starts the app again. Every paste created after that backup is lost.

### systemd

Run the backup as the service user, so any SQLite files it touches keep the right owner:

```bash
sudo install -d -o snippit -g snippit -m 750 /var/backups/snippit
sudo -u snippit SNIPPIT_DB_PATH=/var/lib/snippit/snippit.db \
  /usr/local/bin/snippit backup /var/backups/snippit/snippit-$(date +%Y%m%d-%H%M%S).db
```

To restore:

```bash
sudo systemctl stop snippit
sudo -u snippit cp /var/backups/snippit/<file>.db /var/lib/snippit/snippit.db
sudo -u snippit rm -f /var/lib/snippit/snippit.db-wal /var/lib/snippit/snippit.db-shm
sudo systemctl start snippit
```

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
make dist   # release archives for every platform in dist/
```

Pushing a `v*` tag (e.g. `git tag v1.0.0 && git push origin v1.0.0`) runs `.github/workflows/release.yml`, which tests, builds the archives and publishes a GitHub release.

The frontend lives in `frontend/` and is bundled by esbuild and the Tailwind CLI into `internal/web/static/`, which is embedded into the binary. That directory is generated and not committed, so run `make assets` (or any target above) after cloning, before running `go build` or `go test` directly. For live CSS/JS rebuilds, run `cd frontend && npm run watch` and restart the Go server after changes.

```
cmd/snippit/          entry point (serve, backup, healthcheck)
internal/config/      environment configuration
internal/paste/       domain: validation, IDs, encryption, language detection
internal/store/       SQLite repository and migrations
internal/web/         HTTP handlers, middleware, templates
frontend/             CodeMirror, HTMX and Tailwind sources
deploy/               systemd unit and env file for running without Docker
scripts/dist.sh       cross-compiles release archives
```

## Privacy

Snippit stores paste content and metadata, never accounts. Client IPs are held in memory for rate limiting only and are not written to the database or the app logs. See `/privacy` on a running instance.
