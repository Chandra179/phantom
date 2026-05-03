#!/usr/bin/env bash
set -euo pipefail

# Quick setup: install tools, generate C headers, build Rust + Go.
# Run once after cloning or when Rust C ABI signatures change.
# Requires: cargo, go

cd "$(dirname "$0")/.."

PROTOC="${HOME}/protoc/bin/protoc"

echo "==> Installing cbindgen..."
cargo install cbindgen

echo "==> Generating C headers..."
mkdir -p rust/headers
cbindgen --config rust/cbindgen.toml rust/graphic_processor --output rust/headers/graphic_processor.h
cbindgen --config rust/cbindgen.toml rust/backtesting --output rust/headers/backtesting.h

echo "==> Building Rust (compute_server)..."
PROTOC="${PROTOC}" cargo build --release --manifest-path rust/compute_server/Cargo.toml

echo "==> Building Go..."
go build -o ingestion ./cmd/

echo "==> Done."
