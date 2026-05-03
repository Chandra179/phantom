# Phantom

Event-driven trading research pipeline. **Go** (HTTP + orchestration) + **Rust** (gRPC math server).

## Directory Structure

```
.
├── cmd
│   ├── ingestion.go          # Gin HTTP server (:8080), /health, /ingest
│   └── benchmark/main.go     # Benchmark harness: BTC halving, BW1985, survivorship, FOMC
├── pkg
│   ├── shared                # Core types: Event, PricePoint, PriceWindow, TimeRange
│   ├── ingestion             # Fetcher/Deduper/Store interfaces + Stooq/Binance/EDGAR + Mem/Parquet impls + Pipeline
│   ├── signalmatrix          # EventLookup, WindowBuilder, RustBridge (gRPC client), validation tests
│   ├── aggregation           # MeanCAR, CrossSectionalTTest, BMPTest, KolariPynnonen
│   └── strategy              # DeflatedSharpe, SharpeRatio, MaxDrawdown, PurgedKFold, SimulateTrades
├── rust
│   ├── compute_server        # tonic gRPC server (:50051), wraps all 3 lib crates
│   ├── graphic_processor     # log-returns, window slicing
│   ├── backtesting           # OLS market model, AR, CAR, t-test, BMP test
│   └── shape_matching        # z-normalize, Euclidean, DTW, LB_Keogh, find_matches
├── proto
│   └── compute.proto         # 8 RPC methods
├── gen/                      # Generated Go protobuf/gRPC stubs
├── scripts/local.sh          # cbindgen + build all
├── benchmarks/               # JSON results
├── AGENTS.md / CLAUDE.md     # Agent guidance
├── IMPLEMENTATION_PLAN.md    # Academic-pedigreed plan (Stages 1-6)
├── Makefile
└── go.mod
```

## Data Flow

```
External APIs (Stooq/Binance/EDGAR)
  → Pipeline (Fetcher → Deduper → Store)
  → EventLookup (find T0 timestamps)
  → WindowBuilder (slice L1=[T0-250,T0-11], L2=[T0-10,T0+10])
  → RustBridge (gRPC client) → compute_server (Rust)
  → aggregation (cross-sectional stats)
  → strategy (backtest sim + metrics)
```

## Dependencies

### Go
- `github.com/gin-gonic/gin` — HTTP framework
- `github.com/parquet-go/parquet-go` — Parquet store
- `github.com/cenkalti/backoff/v4` — retry logic
- `golang.org/x/time/rate` — rate limiter
- `google.golang.org/grpc` — gRPC client

### Rust
- `tonic` / `prost` — gRPC server
- `tokio` — async runtime
- `serde` — serialization (graphic_processor, backtesting)
- No external math crates — pure `f64` slice arithmetic

## Build & Run

```
make setup       # one-time: cbindgen + headers + build all
make proto       # buf generate → gen/*.pb.go
make build-rust  # cargo build --release compute_server
make build-go    # go build -o ingestion ./cmd/
make run         # build + run Go HTTP server (:8080)
make run-compute # build + run Rust gRPC server (:50051)
make test        # go test ./... + cargo test all crates
make benchmark   # full live benchmark suite
```

## Benchmark Suite

| Benchmark | Description |
|-----------|-------------|
| BTC Halving CAR | Mean CAR(0,+30) across 4 halving events |
| BW1985 | Brown-Warner null specification, std(t) ≈ 1 |

| FOMC Direction | Bernanke-Kuttner sign hypothesis |

Flags: `--stooq-apikey`, `--insecure`, `--skip-bw1985`, `--skip-halving`, `--skip-fomc`, `--fixtures`

## Validation Gates

- BW1985 Table 2 replication: 30 reps × 15 securities, std(t) in [0.65, 1.45]
- Bernanke-Kuttner sign test: FOMC rate changes → SPY direction
- Lookahead test: L1 ends before T0-10, no temporal leakage
- Slice E2E tests: Ball-Brown CAR sign, BTC halving CAR, full pipeline (gated `-short`)
