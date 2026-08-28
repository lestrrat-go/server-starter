#!/bin/bash

set -euo pipefail

DIR=/work/src/github.com/lestrrat-go/server-starter
OUT=/work/artifacts/snapshot

cd "$DIR"
go mod download
mkdir -p "$OUT"

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
    os=${target%/*}
    arch=${target#*/}
    suffix=
    if [ "$os" = windows ]; then
        suffix=.exe
    fi

    GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build \
        -trimpath \
        -ldflags="-s -w" \
        -o "$OUT/start_server-${os}-${arch}${suffix}" \
        ./cmd/start_server
done