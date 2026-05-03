package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	parquet "github.com/parquet-go/parquet-go"

	"phantom/pkg/shared"
)

// ---- MemStore ----

// MemStore is an in-memory Store, safe for concurrent use.
type MemStore struct {
	mu     sync.RWMutex
	points map[shared.AssetID][]shared.PricePoint
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{points: make(map[shared.AssetID][]shared.PricePoint)}
}

// Put appends points into the store and keeps them sorted by Timestamp.
func (s *MemStore) Put(_ context.Context, pts []shared.PricePoint) error {
	if len(pts) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range pts {
		s.points[p.AssetID] = append(s.points[p.AssetID], p)
	}
	for asset := range s.points {
		slice := s.points[asset]
		sort.Slice(slice, func(i, j int) bool {
			return slice[i].Timestamp.Before(slice[j].Timestamp)
		})
		s.points[asset] = slice
	}
	return nil
}

// Get returns points for asset whose Timestamp falls within [r.From, r.To] inclusive.
func (s *MemStore) Get(_ context.Context, asset shared.AssetID, r shared.TimeRange) ([]shared.PricePoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.points[asset]
	var out []shared.PricePoint
	for _, p := range all {
		if !p.Timestamp.Before(r.From) && !p.Timestamp.After(r.To) {
			out = append(out, p)
		}
	}
	return out, nil
}

// ---- ParquetStore ----

// parquetRow is the on-disk representation of a PricePoint (parquet schema).
type parquetRow struct {
	AssetID   string  `parquet:"asset_id"`
	Timestamp int64   `parquet:"timestamp_ns"` // UTC UnixNano
	Open      float64 `parquet:"open"`
	High      float64 `parquet:"high"`
	Low       float64 `parquet:"low"`
	Close     float64 `parquet:"close"`
	Volume    float64 `parquet:"volume"`
	Source    string  `parquet:"source"`
}

// ParquetStore partitions data as {baseDir}/{asset}/{year}/{month}/data.parquet.
type ParquetStore struct {
	baseDir string
	mu      sync.Mutex
}

// NewParquetStore creates a ParquetStore rooted at baseDir.
func NewParquetStore(baseDir string) *ParquetStore {
	return &ParquetStore{baseDir: baseDir}
}

// partitionPath returns the file path for the given asset + timestamp partition.
func partitionPath(baseDir string, asset shared.AssetID, ts time.Time) string {
	return filepath.Join(
		baseDir,
		string(asset),
		fmt.Sprintf("%04d", ts.UTC().Year()),
		fmt.Sprintf("%02d", int(ts.UTC().Month())),
		"data.parquet",
	)
}

// Put writes points to the appropriate partition files.
// Existing data in a partition is read, merged, deduplicated by timestamp, and rewritten.
func (s *ParquetStore) Put(ctx context.Context, pts []shared.PricePoint) error {
	// Group by partition key.
	type partKey struct {
		asset string
		year  int
		month int
	}
	grouped := make(map[partKey][]shared.PricePoint)
	for _, p := range pts {
		k := partKey{string(p.AssetID), p.Timestamp.UTC().Year(), int(p.Timestamp.UTC().Month())}
		grouped[k] = append(grouped[k], p)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, newPts := range grouped {
		// Build path using the first point's timestamp.
		path := partitionPath(s.baseDir, shared.AssetID(k.asset), newPts[0].Timestamp)

		// Read existing rows.
		var existing []parquetRow
		if _, err := os.Stat(path); err == nil {
			rows, err := parquet.ReadFile[parquetRow](path)
			if err != nil {
				return fmt.Errorf("parquet read %s: %w", path, err)
			}
			existing = rows
		}

		// Merge: build a map keyed by timestamp to deduplicate.
		byTs := make(map[int64]parquetRow, len(existing)+len(newPts))
		for _, r := range existing {
			byTs[r.Timestamp] = r
		}
		for _, p := range newPts {
			byTs[p.Timestamp.UTC().UnixNano()] = toRow(p)
		}

		// Sort merged rows by timestamp.
		merged := make([]parquetRow, 0, len(byTs))
		for _, r := range byTs {
			merged = append(merged, r)
		}
		sort.Slice(merged, func(i, j int) bool { return merged[i].Timestamp < merged[j].Timestamp })

		// Ensure directory exists.
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
		}

		// Write.
		if err := parquet.WriteFile(path, merged); err != nil {
			return fmt.Errorf("parquet write %s: %w", path, err)
		}
	}
	return nil
}

// Get reads all matching partition files and filters by time range.
func (s *ParquetStore) Get(_ context.Context, asset shared.AssetID, r shared.TimeRange) ([]shared.PricePoint, error) {
	// Find all year/month partitions that could overlap [r.From, r.To].
	partitions := monthsInRange(r)

	s.mu.Lock()
	defer s.mu.Unlock()

	var out []shared.PricePoint
	for _, ym := range partitions {
		path := filepath.Join(
			s.baseDir,
			string(asset),
			fmt.Sprintf("%04d", ym.year),
			fmt.Sprintf("%02d", ym.month),
			"data.parquet",
		)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		rows, err := parquet.ReadFile[parquetRow](path)
		if err != nil {
			return nil, fmt.Errorf("parquet read %s: %w", path, err)
		}
		fromNs := r.From.UTC().UnixNano()
		toNs := r.To.UTC().UnixNano()
		for _, row := range rows {
			if row.Timestamp >= fromNs && row.Timestamp <= toNs {
				out = append(out, fromRow(row, asset))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out, nil
}

type yearMonth struct{ year, month int }

// monthsInRange returns all (year,month) pairs that fall within the time range.
func monthsInRange(r shared.TimeRange) []yearMonth {
	var result []yearMonth
	cur := time.Date(r.From.UTC().Year(), r.From.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(r.To.UTC().Year(), r.To.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	for !cur.After(end) {
		result = append(result, yearMonth{cur.Year(), int(cur.Month())})
		cur = cur.AddDate(0, 1, 0)
	}
	return result
}

func toRow(p shared.PricePoint) parquetRow {
	return parquetRow{
		AssetID:   string(p.AssetID),
		Timestamp: p.Timestamp.UTC().UnixNano(),
		Open:      p.Open,
		High:      p.High,
		Low:       p.Low,
		Close:     p.Close,
		Volume:    p.Volume,
		Source:    p.Source,
	}
}

func fromRow(r parquetRow, asset shared.AssetID) shared.PricePoint {
	return shared.PricePoint{
		AssetID:   asset,
		Timestamp: time.Unix(0, r.Timestamp).UTC(),
		Open:      r.Open,
		High:      r.High,
		Low:       r.Low,
		Close:     r.Close,
		Volume:    r.Volume,
		Source:    r.Source,
	}
}
