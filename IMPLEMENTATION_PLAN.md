# Quant Event Engine — Implementation Plan

End-to-end blueprint. Free data → ingestion → event study → backtest. Grounded in published methodology.

---

## 1. Academic Foundations

### Core papers (event study methodology)
- **Fama, Fisher, Jensen, Roll (1969)** — "The Adjustment of Stock Prices to New Information." Original event-study framework. Abnormal return decomposition.
- **Brown & Warner (1980, 1985)** — "Measuring Security Price Performance" / "Using Daily Stock Returns." Defines AR, CAR, t-test under daily data. Benchmark for any AR estimator.
- **MacKinlay (1997)** — "Event Studies in Economics and Finance," *J. Econ. Lit.* Canonical reference. Estimation window L1 (~120-250 days), event window L2 (e.g. [-10, +10]), market model.
- **Campbell, Lo, MacKinlay (1997)** — *The Econometrics of Financial Markets*, Ch. 4. Textbook treatment. Use as primary spec source.
- **Boehmer, Musumeci, Poulsen (1991)** — Standardized cross-sectional test. Robust to event-induced variance.
- **Kolari & Pynnönen (2010)** — Cross-sectional correlation adjustment. Use when events cluster in calendar time.

### Pattern matching / DTW
- **Sakoe & Chiba (1978)** — "Dynamic programming algorithm optimization for spoken word recognition." Original DTW. Sakoe-Chiba band constraint.
- **Berndt & Clifford (1994)** — "Using Dynamic Time Warping to Find Patterns in Time Series." DTW applied to financial series.
- **Keogh & Ratanamahatana (2005)** — "Exact indexing of dynamic time warping." LB_Keogh lower bound. Mandatory for scale.

### Backtesting hygiene
- **López de Prado (2018)** — *Advances in Financial Machine Learning.* Ch. 7 (purged k-fold CV), Ch. 11-13 (backtest overfit, deflated Sharpe). Avoid look-ahead, leakage.
- **Bailey & López de Prado (2014)** — Deflated Sharpe Ratio. Adjust for multiple testing.
- **Harvey, Liu, Zhu (2016)** — "…and the Cross-Section of Expected Returns." t > 3.0 threshold given p-hacking.

---

## 2. Free Data Sources

| Asset | API | Limits | Use |
|-------|-----|--------|-----|
| US equities (EOD) | **Stooq** (`stooq.com/q/d/l/`) | None, CSV | Daily OHLCV, 20+ yr history |
| US equities (intraday) | **Alpha Vantage** | 25 req/day free | 1-min bars, last 2 yr |
| Crypto | **Binance public REST** (`api.binance.com/api/v3/klines`) | 1200 req/min, no key | Klines 1m-1d, full history |
| Crypto (alt) | **CoinGecko** | 30 req/min free | Cross-exchange ref price |
| FX / macro | **FRED** (`api.stlouisfed.org`) | Free w/ key | Rates, CPI, NFP releases |
| Earnings calendar | **Finnhub** free tier | 60 req/min | Event timestamps (T0) |
| Economic calendar | **Trading Economics** guest / **FRED** releases | low | Macro event T0 |
| Corporate actions | **SEC EDGAR** (`data.sec.gov`) | 10 req/s, no key | 8-K filings, exact T0 timestamp |
| News (events) | **GDELT 2.0** | Free, BigQuery or REST | Global event database, T0 |
| Filings full-text | **SEC EDGAR submissions** | 10 req/s | 10-K/10-Q/8-K |

Primary picks: **Stooq + Binance + SEC EDGAR + FRED**. Zero-cost, no key for 3 of 4.

---

## 3. Pipeline Architecture (mapped to repo)

```
[Source APIs] → Fetcher → Deduper → Store → EventLookup → WindowBuilder
                                                              ↓
                                                         RustBridge (gRPC)
                                                              ↓
                              ┌──────────────┬───────────────┬──────────────┐
                       graphic_processor  backtesting   shape_matching   (compute_server)
                              ↓              ↓               ↓
                       pct_changes,     AR, CAR,        Euclid, DTW
                       windows          t-test          (LB_Keogh)
                                              ↓
                                       Aggregated stats → Report / Strategy
```

---

## 4. Stage-by-Stage Plan

### Stage 1 — Ingestion (`pkg/ingestion`, `cmd/ingestion`)

**Fetcher iface impls:**
- `StooqFetcher` — HTTP GET CSV, parse to `[]shared.PricePoint`.
- `BinanceFetcher` — `/api/v3/klines?symbol=BTCUSDT&interval=1m&startTime=...`. Paginate 1000 bars/req.
- `EdgarFetcher` — `data.sec.gov/submissions/CIK{n}.json`, then 8-K index. T0 = `acceptedDateTime` (NOT filing date).
- `FredFetcher` — `series/observations?series_id=GDP&api_key=...`.

**Deduper:** SHA-256 over `(asset_id, timestamp, source)`. Dedup at `Store.Put`. Use `sync.Map` for in-mem, Redis/SQLite for persist.

**Store iface:** Start w/ Parquet on disk (via `github.com/xitongsys/parquet-go`) → migrate to DuckDB/ClickHouse. Partition by `asset_id/year/month`.

**Pipeline:** `errgroup` fan-out. Rate-limit per source (token bucket, `golang.org/x/time/rate`). Retry w/ expo backoff (`cenkalti/backoff/v4`).

**Critical: corporate actions adjustment.** Stooq gives split-adj only. For dividend-adjusted total returns, pull CRSP-style adj from Yahoo `?events=div,split` endpoint or compute from EDGAR filings. **Do this pre-store, not post.**

### Stage 2 — Event Lookup (`pkg/signalmatrix`)

`EventLookup.Find(criteria) -> []shared.Event`. Index events by `(EventType, AssetID, Timestamp)`. SQLite FTS5 or BoltDB key prefix scan.

Event types to index:
- Earnings surprise (Finnhub estimate vs actual)
- 8-K material events (EDGAR item codes 1.01, 2.02, 5.02)
- FOMC announcements (FRED release calendar)
- Crypto: exchange listings, halvings (manual seed)

### Stage 3 — Window Builder

`WindowBuilder.Build(event, L1, L2) -> shared.PriceWindow` where:
- **Estimation window L1**: `[T0-250, T0-11]` trading days (MacKinlay default).
- **Event window L2**: `[T0-10, T0+10]` configurable.
- **Gap**: 10 days between L1 end & L2 start. Prevents contamination.

Skip event if:
- < 200 obs in L1 (Brown-Warner threshold).
- Overlapping prior event within L1 (clustering).
- Trading halt during L2.

Output: aligned matrix, log-returns `r_t = ln(P_t / P_{t-1})`, NOT simple returns. Formula:

```
R_it = ln(P_it) - ln(P_i,t-1)
R_mt = ln(P_mt) - ln(P_m,t-1)   # market index (SPY for equities, BTCUSDT for crypto basket)
```

### Stage 4 — Rust Compute (`rust/`, `compute_server`)

#### `graphic_processor`
- `percent_changes(prices: &[f64]) -> Vec<f64>` — log-returns.
- `build_window(prices, t0_idx, l1, l2) -> Window` — slice + align.
- Use `nalgebra::DVector`. SIMD via `packed_simd` optional.

#### `backtesting`
**Market model (MacKinlay §4.4.1):**
```
R_it = α_i + β_i · R_mt + ε_it     (estimated on L1 via OLS)
AR_it = R_it - (α̂_i + β̂_i · R_mt)  (computed on L2)
CAR_i(τ1, τ2) = Σ_{t=τ1}^{τ2} AR_it
```

Variance (Brown-Warner):
```
σ²(AR_it) = σ²_ε,i · [1 + 1/L1 + (R_mt - R̄_m)² / Σ(R_ms - R̄_m)²]
σ²(CAR_i) = Σ σ²(AR_it)             (assuming serial indep)
```

Test stat:
```
t_CAR = CAR̄ / sqrt(σ²(CAR̄)/N)        # cross-sectional avg over N events
```

Functions:
- `ols_market_model(r_i, r_m) -> (alpha, beta, sigma_eps)`
- `abnormal_return(r_it, r_mt, alpha, beta) -> f64`
- `cumulative_abnormal_return(ars: &[f64]) -> f64`
- `t_test_one_sample(cars: &[f64]) -> TStat`
- `bmp_test(standardized_cars: &[f64]) -> f64`  # Boehmer-Musumeci-Poulsen, robust

Use `ndarray` + `ndarray-linalg` (LAPACK) for OLS. `statrs` for t-distribution CDF.

#### `shape_matching`
- `euclidean_distance(a: &[f64], b: &[f64]) -> f64` — z-normalize first (Keogh).
- `dtw_distance(a, b, band: usize) -> f64` — Sakoe-Chiba band = 10% of len.
- `lb_keogh(query, candidate, band) -> f64` — early-abandon lower bound.

Workflow: z-norm windows → LB_Keogh prune → DTW only on survivors.

#### `compute_server` (gRPC)
Proto already in `proto/`. Endpoints:
- `ComputeAR(EventBatch) -> ARResults`
- `ComputeCAR(ARBatch, Window) -> CARResults`
- `MatchShape(Query, Candidates) -> Distances`

Stream large batches. Use `tonic` (Rust) ↔ generated Go client (`gen/`).

### Stage 5 — Aggregation & Backtest

In Go (`pkg/signalmatrix` consumer or new `pkg/strategy`):

1. **Cross-sectional aggregation:** mean CAR across N events of same type.
2. **Significance:** BMP test + Kolari-Pynnönen if events cluster.
3. **Out-of-sample:** purged k-fold (López de Prado Ch.7). Embargo = L2 length.
4. **Strategy sim:** if `CAR(0,+5) > threshold`, enter long T0+1 close, exit T0+6 close. Compute:
   - Sharpe (annualized, rf from FRED `DGS3MO`).
   - Max drawdown.
   - Hit rate.
   - **Deflated Sharpe** (Bailey-LdP) — adjust for # of strategies tried.
5. **Costs:** model 5 bps slippage + commission. Without costs = lying.

### Stage 6 — Reporting

CSV/Parquet dump → Jupyter notebook (matplotlib). Plot avg CAR ± 2σ band over event window. Standard event-study chart.

---

## 5. Build Order (tracer-bullet slices)

1. **Slice 1**: Stooq fetch → Parquet store → Go-only mean return calc. End-to-end skeleton, no Rust.
2. **Slice 2**: EDGAR 8-K events → window build → Rust market-model AR via gRPC.
3. **Slice 3**: CAR aggregation + t-test. First real event-study output.
4. **Slice 4**: Binance crypto + halving/listing events.
5. **Slice 5**: DTW shape matching for pattern-based event detection.
6. **Slice 6**: Backtest harness w/ purged CV + deflated Sharpe.

---

## 6. Validation Checklist

- [ ] Replicate Brown-Warner (1985) Table 2 AR variance on CRSP-equivalent sample.
- [ ] Earnings-announcement CAR matches Ball-Brown (1968) drift sign.
- [ ] FOMC surprise sign matches Bernanke-Kuttner (2005).
- [ ] No look-ahead: event T0 strictly < window data timestamps used in L2 entry decision.
- [ ] Survivorship: include delisted tickers (Stooq has them; verify).

---

## 7. Risks / Known Pitfalls

- **Look-ahead via earnings-est revisions** — Finnhub estimates are point-in-time? Verify. If snapshot, leakage.
- **Stooq adj quality** — cross-check vs Yahoo on 5 random tickers w/ splits.
- **EDGAR `acceptedDateTime` vs market hours** — after-hours filing → T0 = next open.
- **DTW O(n²)** — band + LB_Keogh mandatory > 1k candidates.
- **Multiple testing** — every event type tried inflates false-positive rate. Track # of hypotheses.

---

## 8. References (BibTeX-ready short form)

- Fama, Fisher, Jensen, Roll. *Int. Econ. Rev.* 10(1), 1969.
- Brown & Warner. *J. Financ. Econ.* 8, 1980; 14, 1985.
- MacKinlay. *J. Econ. Lit.* 35(1), 1997.
- Campbell, Lo, MacKinlay. *Econometrics of Financial Markets.* Princeton, 1997.
- Boehmer, Musumeci, Poulsen. *J. Financ. Econ.* 30, 1991.
- Kolari & Pynnönen. *Rev. Financ. Stud.* 23, 2010.
- Sakoe & Chiba. *IEEE ASSP* 26, 1978.
- Keogh & Ratanamahatana. *KAIS* 7, 2005.
- López de Prado. *Advances in Financial Machine Learning.* Wiley, 2018.
- Bailey & López de Prado. *J. Portf. Manag.* 40, 2014.
- Harvey, Liu, Zhu. *Rev. Financ. Stud.* 29, 2016.

---

## 9. TDD Todo List

Red→green→refactor each item. Iface + mock first, real impl after failing test.

### Slice 1 — Stooq → Parquet → mean return

#### shared types
- [x] test: `PricePoint` zero-val, `Event` equality, `AssetID` parse → impl types.
- [x] test: `PriceWindow.LogReturns()` known input → known output → impl.

#### Fetcher iface
- [x] define `Fetcher` (`Fetch(ctx, AssetID, range) ([]PricePoint, error)`).
- [x] test: `MockFetcher` canned CSV; pipeline consumes.
- [x] test: `StooqFetcher` vs `httptest.Server` fixture → impl real.
- [x] test: malformed CSV → typed err → impl parser.

#### Deduper iface
- [x] define `Deduper` (`Seen(key) bool`, `Mark(key)`).
- [x] test: SHA256 key stable across runs → impl `HashKey(asset,ts,src)`.
- [x] test: `MemDeduper` dup detect → impl `sync.Map`.
- [x] test: concurrent `Mark` no race (`-race`) → impl.

#### Store iface
- [x] define `Store` (`Put([]PricePoint)`, `Get(AssetID, range) ([]PricePoint)`).
- [x] test: `MemStore` round-trip → impl.
- [x] test: `ParquetStore` write→read identity → impl `parquet-go`.
- [x] test: partition path `asset/year/month` → impl.

#### Pipeline
- [x] test: wires Fetcher+Deduper+Store w/ mocks; assert order, dedup skip.
- [x] test: token bucket rate limit (fake clock) → impl.
- [ ] test: expo backoff retry on transient err → impl.
- [x] test: errgroup cancel on ctx done → impl.

#### Slice-1 e2e
- [x] integration: real Stooq HTTP (gated `-short`) → Parquet → Go mean return = manual calc.

### Slice 2 — EDGAR → window → Rust AR via gRPC

#### EventLookup iface
- [x] define `EventLookup.Find(criteria) ([]Event)`.
- [x] test: in-mem index by `(type,asset,ts)` → impl.
- [x] test: `EdgarFetcher` parse 8-K JSON fixture; `acceptedDateTime` → T0.
- [x] test: after-hours filing → next open T0 → impl rule.

#### WindowBuilder iface
- [x] define `WindowBuilder.Build(event, L1, L2) (PriceWindow, error)`.
- [x] test: L1=[T0-250,T0-11], L2=[T0-10,T0+10] indices → impl.
- [x] test: skip if <200 obs L1 (Brown-Warner) → impl.
- [x] test: skip if overlap prior event → impl.
- [x] test: skip if halt in L2 → impl.
- [x] test: log-returns `ln(P_t/P_{t-1})` → impl.

#### Rust `graphic_processor`
- [x] test: `percent_changes` known vec → known logret.
- [x] test: NaN/zero price → typed err.
- [x] test: `build_window` slice align → impl.

#### Rust `backtesting`
- [x] test: `ols_market_model` synth → recover known α,β,σ_ε within tol.
- [x] test: `abnormal_return` hand calc match.
- [x] test: `cumulative_abnormal_return` sum.
- [x] test: variance incl `(R_mt - R̄_m)²` term (Brown-Warner spec).
- [x] test: `t_test_one_sample` vs `scipy.stats.ttest_1samp` golden.
- [x] test: `bmp_test` vs published BMP example.

#### gRPC bridge
- [x] test: tonic stub server → canned `ARResults`; Go client decode.
- [x] test: `RustBridge.ComputeAR` mock server → impl Go client.
- [x] integration: real Rust server + Go client; AR = Rust unit test output.

### Slice 3 — CAR aggregation + t-test
- [x] test: cross-sectional mean CAR over N synth events.
- [x] test: BMP differs from naive t on clustered events.
- [x] test: Kolari-Pynnönen adj when calendar overlap > threshold.
- [x] e2e: EDGAR earnings 8-K → CAR sign matches Ball-Brown drift.

### Slice 4 — Binance crypto
- [x] test: `BinanceFetcher` paginate 1000 bars (recorded fixture) → impl.
- [x] test: rate-limit 1200 req/min → impl.
- [x] test: halving seed → window build → impl seed loader.
- [x] e2e: BTC halving 2024-04 CAR computed.

### Slice 5 — DTW shape matching

#### Rust `shape_matching`
- [ ] test: `euclidean_distance` z-norm pre-step golden.
- [ ] test: `dtw_distance` Sakoe-Chiba band vs `dtw` py ref golden.
- [ ] test: `lb_keogh` ≤ true DTW (property test, 1000 random pairs).
- [ ] test: prune workflow — LB_Keogh reject → DTW skip count correct.

### Slice 6 — Backtest harness
- [ ] test: purged k-fold split, embargo=L2 len, no train/test overlap.
- [ ] test: Sharpe vs `numpy` golden.
- [ ] test: max drawdown known series.
- [ ] test: deflated Sharpe (Bailey-LdP) vs paper example.
- [ ] test: 5bps slippage applied each fill.
- [ ] e2e: full pipeline Stooq→event→AR→CAR→strategy sim → report.

### Validation gates (per §6)
- [ ] replicate Brown-Warner 1985 Tbl 2 AR variance, CRSP-equiv sample.
- [ ] Bernanke-Kuttner FOMC sign check.
- [ ] look-ahead audit: assert T0 < L2 entry ts in test.
- [ ] survivorship: delisted ticker present in Stooq pull, test asserts.

### Cross-cutting
- [ ] CI: `go test -race ./... && cargo test --all`.
- [ ] golden-file harness Go+Rust shared via JSON fixtures in `testdata/`.
- [ ] property tests: `gopter` (Go), `proptest` (Rust) for math fns.
- [ ] mocks generated: `mockery` for Go ifaces.

Order per slice: ifaces+mocks → unit tests red → impl green → integration → e2e. No impl without failing test.
