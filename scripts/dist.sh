#!/usr/bin/env bash
# Cross-compiles release archives into dist/.
# Run through `make dist`, which builds the frontend assets first.
set -euo pipefail

VERSION="${VERSION:-dev}"
TARGETS="${TARGETS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64}"

rm -rf dist
mkdir -p dist

for target in $TARGETS; do
  goos="${target%/*}"
  goarch="${target#*/}"
  name="snippit_${VERSION#v}_${goos}_${goarch}"
  dir="dist/$name"
  bin="snippit"
  [ "$goos" = windows ] && bin="snippit.exe"

  echo "building $name"
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$dir/$bin" ./cmd/snippit
  cp README.md deploy/snippit.env "$dir/"
  [ "$goos" = linux ] && cp deploy/snippit.service "$dir/"

  if [ "$goos" = windows ]; then
    (cd dist && zip -qr "$name.zip" "$name")
  else
    tar -C dist -czf "dist/$name.tar.gz" "$name"
  fi
  rm -rf "$dir"
done

(cd dist && sha256sum ./*.tar.gz ./*.zip | sed 's| \./| |' > SHA256SUMS)
echo "done:"
ls -1 dist
