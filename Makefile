.PHONY: build-rust build-go setup proto test run run-compute

PROTOC ?= $(HOME)/protoc/bin/protoc

proto:
	buf generate

build-rust:
	PROTOC=$(PROTOC) cargo build --release --manifest-path rust/compute_server/Cargo.toml

build-go:
	go build -o ingestion ./cmd/

setup:
	bash scripts/local.sh

run: build-go
	./ingestion

run-compute: build-rust
	./rust/compute_server/target/release/compute_server

test:
	go test ./...
	PROTOC=$(PROTOC) cargo test --manifest-path rust/compute_server/Cargo.toml
