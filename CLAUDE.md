# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make setup             # one-time: install cbindgen, gen C headers, build Rust+Go
make proto             # buf generate → gen/*.pb.go from proto/compute.proto
make build-rust        # cargo build --release for compute_server (needs PROTOC)
make build-go          # go build -o ingestion ./cmd/
make test              # go test ./... + cargo test on compute_server
make run               # build + run Go ingestion binary
make run-compute       # build + run Rust gRPC compute_server

# Single Go test
go test ./pkg/signalmatrix/... -run TestName

# Single Rust test
PROTOC=$HOME/protoc/bin/protoc cargo test --manifest-path rust/compute_server/Cargo.toml test_name
```

`PROTOC` env var required for any Rust build (tonic-build invokes protoc). Defaults to `$HOME/protoc/bin/protoc` in Makefile/scripts — override if installed elsewhere.

## Architecture

**Interface-only phase**: Go files = interfaces + type stubs. Rust `lib.rs` = pub fn signatures with `unimplemented!()`. No business logic yet — agree on API first.

### Data flow (intended)
```
External APIs → ingestion.Pipeline (Fetcher → Deduper → Store)
                     ↓
             signalmatrix.EventLookup (historical T0 timestamps)
                     ↓
             signalmatrix.WindowBuilder (price windows around T0)
                     ↓
             signalmatrix.RustBridge (gRPC) → rust/compute_server
```

### Go packages (`pkg/`)
- `shared`: core types — `Event`, `PricePoint`, `PriceWindow`, `AssetID`, `EventType`
- `ingestion`: `Fetcher`, `Deduper`, `Store` interfaces + `Pipeline` wires them
- `signalmatrix`: `EventLookup`, `WindowBuilder` interfaces + `RustBridge` (gRPC client to compute_server)

### Rust crates (`rust/`)
- `graphic_processor` (rlib): `percent_changes`, `build_window` — uses `nalgebra`
- `backtesting` (rlib): `abnormal_return`, `cumulative_abnormal_return`, `t_test_one_sample` — uses `ndarray`/`ndarray-stats`
- `shape_matching` (rlib): `euclidean_distance`, `dtw_distance` — uses `ndarray`
- `compute_server` (bin): tonic gRPC server exposing all three crates' functions; only crate that builds via Makefile

### Proto / gRPC
- Single source: `proto/compute.proto`
- `make proto` (buf) generates Go stubs into `gen/`
- Rust side generates via `tonic-build` in `compute_server/build.rs`
- Go side talks to `compute_server` through `pkg/signalmatrix/rust_bridge.go`

### C ABI headers (legacy / optional)
`scripts/local.sh` runs `cbindgen` to emit `rust/headers/*.h` for `graphic_processor` and `backtesting`. gRPC is the primary boundary; C headers exist for direct FFI if needed.
