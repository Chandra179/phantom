# Phantom

Event-driven trading research pipeline. **Go** (HTTP + orchestration) + **Rust** (gRPC math server).

## What This Project Does

**Core question:** Does a specific event (earnings report, Fed rate decision, Bitcoin halving) predictably move stock/crypto prices?

**The problem:** Prices move every day for many reasons — market-wide trends, random noise, sector shifts. If a stock goes up after an event, was it *because* of the event, or just because the whole market went up?

**What we do (step-by-step):**

### 1. Fetch raw prices
From Binance (crypto), Yahoo Finance, or Stooq (stocks). Get Open/High/Low/Close/Volume each day. Raw material for everything else.

### 2. Build windows around each event
Raw prices alone are useless. Day after event stock up 2% — but market up 3%. Stock with β=1.5 *should* be up 4.5%. 2% up = actually *underperformed*.

We need to learn each stock's normal behavior first, then measure deviation.

- **L1 (estimation window):** days T0-250 to T0-11 = 240 days of normal trading. Enough data to measure "when market moves X%, this stock typically moves Y*X%." L1 must have ≥200 observations (Brown-Warner 1985 threshold) for stable OLS fit. Fewer → beta estimate noisy → AR unreliable.
- **L2 (event window):** days T0-10 to T0+10 = 21 days around event. Captures pre-event anticipation (-10 to -1), immediate reaction (T0), and post-event drift (+1 to +10). This is what we measure.
- **10-day gap** between L1 and L2 (T0-11 to T0-10) prevents event contamination. If event leaks early (earnings rumor, Fed hint), price drifts before T0. Without gap, L1 would include leaked behavior → model thinks "this drift is normal" → AR ≈ 0 → miss real effect.

### 3. Compute log-returns
`r_t = ln(P_t / P_{t-1})`

Why log-returns, not simple returns? Simple returns fail the additivity test: price day1=100, day2=110 (+10%), day3=99 (-10%). Sum = 0, but final price 99 ≠ 100. Log-returns: ln(110/100)=+0.0953, ln(99/110)=-0.1054. Sum = -0.0101. exp(-0.0101)=0.99 = 99/100. **Log-returns add up correctly.** Also: log-returns ≈ normally distributed (prices are lognormal) — OLS regression assumes normality for inference.

### 4. Fit market model (OLS regression on L1)
Market model: `stock_return = α + β × market_return + ε`

OLS finds best α̂, β̂ by minimizing sum of squared ε over L1. This isolates stock's personality:
- **β (beta):** Stock's sensitivity to market. β=1.5 → market up 1%, stock typically up 1.5%. β=0.5 → market moves barely affect stock (utilities).
- **α (alpha):** Stock's daily drift independent of market. Usually ≈ 0.
- **σ_ε (sigma residual):** Idiosyncratic volatility. σ_ε=0.01 → ~1% daily residual noise. Smaller = AR more precise.

**Why market model, not raw "stock minus market"?** If stock and market have different beta, raw difference misleads. Stock returned 1%, market 2%. Raw = -1% (looks bad). But if β=0.3, AR = 1% - (α̂ + 0.3×2%) ≈ 0.4% (actually good for this stock). Raw subtraction assumes β=1 — wrong for most stocks.

### 5. Compute Abnormal Return (AR) on L2
`AR_t = R_stock,t − (α̂ + β̂ × R_market,t)`

AR strips out market-driven movement. The leftover is what happened *because of the event*. AR_t = +0.02 = stock beat market-predicted return by 2% that day.

| AR Value | Meaning |
|----------|---------|
| AR ≈ 0 | Stock moved exactly as market model predicted — no event effect |
| AR > 0 | Stock outperformed β-adjusted market expectation — possible event gain |
| AR < 0 | Stock underperformed — possible event loss |
| |AR| > 2×σ_ε | Statistically unusual (≈95% CI) — outlier |

### 6. Cumulate AR → CAR (Cumulative Abnormal Return)
`CAR(τ₁, τ₂) = Σ_{t=τ₁}^{τ₂} AR_t`

Single day's AR is noisy. Event effects unfold over days. Summing AR cancels random noise and grows signal. CAR = +0.08 → stock cumulatively outperformed market prediction by 8% over event window.

**Sign interpretation:**
- CAR > 0 → event associated with price increase (good for longs)
- CAR < 0 → event associated with price decrease (good for shorts)
- CAR ≈ 0 → no detectable price impact

**Benchmark pass/fail** checks sign (mean CAR > 0). Simple directional test.

### 7. Test significance — is CAR real or random?

**Why t-test?** CAR could be coincidence. t-stat measures confidence:
`t = mean(CAR_i) / (std(CAR_i) / √N)`

Numerator = average effect. Denominator = uncertainty (standard error). Bigger |t| = more sure.

| t | | Rough meaning |
|-----|------|
| < 1.0 | Could be random. Don't trade. |
| 1.0 – 2.0 | Weak. Interesting but inconclusive. |
| 2.0 – 3.0 | p < 0.05. Probable real effect. |
| > 3.0 | p < 0.003. Strong signal. |

**Why BMP (Boehmer-Musumeci-Poulsen)?** Different stocks have different volatility (biotech near FDA decision = huge daily moves, utility = tiny moves). Naive t-test treats all CARs equally — high-vol events dominate. BMP standardizes each CAR by its own sigma: `SCAR_i = CAR_i / σ_i`, then t-tests SCARs. Down-weights noisy events, up-weights precise ones.

**Why Kolari-Pynnönen (KP)?** Events cluster in calendar time (e.g., all stocks hit by same Fed announcement). Their CARs correlate. Correlated data inflates t-stat (t-test assumes independence) → false positives. KP corrects: `t_KP = t_BMP / √(1 + (N-1)·r̄)` where r̄ = mean SCAR cross-correlation.

| r̄ | Effect | When to use |
|------|--------|-------------|
| 0 | KP = BMP (no change) | Independent events |
| 0.1 | Shrink ≈ 5-10% | Same-sector events |
| 0.3 | Shrink ≈ 15-25% | Clustered calendar events |
| 0.5 | Shrink ≈ 30-40% | Strong clustering (same day) |

**Hierarchy:** Naive t → baseline. BMP → handles uneven volatility. KP → handles correlated events. If all three agree → robust result. If they diverge → simpler test is misleading.

**Output:** A number: "this event type produces X% CAR, with |t|=Y (p≈Z)." Used for trading signals, risk models, academic research.

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
