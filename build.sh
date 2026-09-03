#!/usr/bin/env bash
# Build muse-tools-stripper plugin for CLIProxyAPI.
# Uses a golang container so cgo (gcc/musl-dev) is available for -buildmode=c-shared.
#
# The mounted plugins dir on the host is bind-mounted into the container at
# /CLIProxyAPI/plugins, so output lands directly where cliproxy loads from.
set -euo pipefail

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_NAME="muse-tools-stripper"
OUT_DIR="${PLUGIN_OUT_DIR:-${SRC_DIR}/../../plugins/linux/amd64}"

# Runtime container is Debian (glibc): must build with glibc toolchain, not alpine/musl.
GO_IMAGE="${PLUGIN_GO_IMAGE:-golang:1.26}"

echo "[build] source=${SRC_DIR}"
echo "[build] output=${OUT_DIR}"

docker run --rm \
  -v "${SRC_DIR}:/src" \
  -v "${OUT_DIR}:/out" \
  -w /src \
  -e "CGO_ENABLED=1" \
  -e "GOFLAGS=-mod=mod" \
  "${GO_IMAGE}" \
  sh -ec '
    go mod tidy
    go vet ./...
    go test ./...
    go build -buildvcs=false -buildmode=c-shared -buildmode=c-shared -o /out/'"${PLUGIN_NAME}"'-v0.1.0.so .
    # c-shared also emits a header next to the .so; cliproxy does not need it.
    rm -f /out/'"${PLUGIN_NAME}"'-v0.1.0.h
  '

echo "[build] ok: ${OUT_DIR}/${PLUGIN_NAME}-v0.1.0.so"