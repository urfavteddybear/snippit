# Local development helpers. Production runs through docker compose.
export PATH := $(PATH):/usr/local/go/bin

STAMP := $(shell date +%Y%m%d-%H%M%S)

VOLUME := snippit_snippit-data

.PHONY: assets build run test docker-up docker-down docker-logs backup restore

frontend/node_modules: frontend/package-lock.json
	cd frontend && npm ci --no-audit --no-fund
	touch $@

# Build CodeMirror/HTMX/Tailwind into internal/web/static (embedded by go:embed).
assets: frontend/node_modules
	cd frontend && npm run build

# Compile the server binary to ./snippit.
build: assets
	go build -o snippit ./cmd/snippit

# Build and run on :8080 with the database in ./data.
run: build
	SNIPPIT_DB_PATH=data/snippit.db ./snippit

test: assets
	go vet ./...
	go test -race ./...

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

# Snapshot the live database into the volume, then copy it to ./backups.
backup:
	mkdir -p backups
	docker compose exec snippit /snippit backup /data/backup-$(STAMP).db
	docker compose cp snippit:/data/backup-$(STAMP).db backups/snippit-$(STAMP).db

# Replace the live database with a backup: make restore FILE=backups/snippit-XXXX.db
# The app image has no shell, so a throwaway Alpine container does the copy.
restore:
	@test -n "$(FILE)" || (echo "usage: make restore FILE=backups/<file>.db" && exit 1)
	docker compose stop snippit
	docker run --rm -v $(VOLUME):/data -v "$(abspath $(FILE))":/restore.db:ro alpine:3.20 \
		sh -c 'cp /restore.db /data/snippit.db && rm -f /data/snippit.db-wal /data/snippit.db-shm && chown 65532:65532 /data/snippit.db'
	docker compose start snippit
