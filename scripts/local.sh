#!/usr/bin/env bash
set -euo pipefail

# One-time setup: build Rust + Go.
# gRPC is primary Go↔Rust boundary (C ABI legacy).
# Requires: cargo, go, buf, protoc

cd "$(dirname "$0")/.."

PROTOC="${HOME}/protoc/bin/protoc"

echo "==> Building Rust (compute_server)..."
PROTOC="${PROTOC}" cargo build --release --manifest-path rust/compute_server/Cargo.toml

echo "==> Building Go..."
go build -o ingestion ./cmd/

echo "==> Done."
