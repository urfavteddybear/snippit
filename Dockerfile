# syntax=docker/dockerfile:1

# 1. Frontend assets (CodeMirror, HTMX, Tailwind) -> internal/web/static
FROM node:24-alpine AS assets
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
# Tailwind scans the templates for class names.
COPY internal/web/templates /src/internal/web/templates
RUN npm run build

# 2. Static Go binary with the assets embedded
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=assets /src/internal/web/static ./internal/web/static
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/snippit ./cmd/snippit \
 && mkdir -p /out/data

# 3. Minimal runtime: no shell, runs as uid 65532
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/snippit /snippit
# A named volume mounted here inherits this ownership on first use.
COPY --from=build --chown=65532:65532 /out/data /data
ENV SNIPPIT_ADDR=:8080 \
    SNIPPIT_DB_PATH=/data/snippit.db
EXPOSE 8080
VOLUME ["/data"]
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/snippit", "healthcheck"]
ENTRYPOINT ["/snippit"]
