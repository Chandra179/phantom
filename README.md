# Quant Event Engine (Monorepo)

A prototype pipeline for event‑driven trading research, built with **Go** (concurrency/coordination) and **Rust** (high‑performance math).

## Directory Structure

.
├── cmd
│   └── ingestion          # Go binary entrypoint (placeholder)
├── pkg
│   ├── ingestion          # Go library: fetcher, dedup, store interfaces
│   ├── signalmatrix       # Go library: event lookup, window builder, Rust bridge
│   └── shared             # Common Go types (Event, Price, Timestamp)
├── rust
│   ├── graphic_processor  # Rust crate: percent changes, window arrays
│   ├── backtesting        # Rust crate: abnormal returns, CAR, t‑test
│   └── shape_matching     # Rust crate: Euclidean distance, DTW
├── go.mod
├── Makefile
└── README.md

## Dependencies

### Go
- Standard library: `crypto/sha256`, `encoding/csv`, `sync`, `context`
- External: `golang.org/x/sync/errgroup` (concurrency), `github.com/klauspost/compress` (optional)

### Rust
- Workspace crates:
  - `graphic_processor`: `nalgebra` (linear algebra), `serde`
  - `backtesting`: `ndarray`, `ndarray-stats`, `serde`
  - `shape_matching`: `ndarray`, `smartcore` (optional for DTW)
- `libc` (for C ABI exports)

## Build & Prototype

# Install Rust toolchain
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh

# Build all Rust crates (static/shared libs)
make build-rust

# Build Go binaries (will link against Rust libs)
make build-go

# Run tests
make test

## Interface-Only Phase
All Go files contain **interface definitions** and minimal type stubs.  
All Rust `lib.rs` files expose **public function signatures** with `unimplemented!()` bodies.  
This allows fast agreement on the API before investing in implementation.\