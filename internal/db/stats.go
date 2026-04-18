package db

import (
	"context"
	"time"
)

// HealthStatus describes the current DB availability.
type HealthStatus struct {
	Configured bool   // DATABASE_URL was set
	Connected  bool   // ping succeeded
	Error      string // last error, if any
}

// Health returns the current DB health. Never blocks longer than 2 seconds.
func Health(ctx context.Context) HealthStatus {
	if pool == nil {
		return HealthStatus{Configured: false}
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return HealthStatus{Configured: true, Connected: false, Error: err.Error()}
	}
	return HealthStatus{Configured: true, Connected: true}
}

// Counts is a snapshot of row counts for the dashboard.
// A nil pointer means the table is absent (migration not yet applied).
type Counts struct {
	Levels *int64
	Games  *int64
	Users  *int64
}

// GetCounts queries row counts for the main tables. Missing tables are
// reported as nil (not an error), so the dashboard can render "—" until
// future migrations create them.
func GetCounts(ctx context.Context) (Counts, error) {
	var out Counts
	if pool == nil {
		return out, nil
	}
	qctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	out.Levels = tryCount(qctx, "levels")
	out.Games = tryCount(qctx, "match_summaries")
	out.Users = tryCount(qctx, "users")
	return out, nil
}

// tryCount returns a pointer to the row count, or nil if the table is
// missing. Any other error is swallowed and returned as nil to keep the
// dashboard best-effort.
func tryCount(ctx context.Context, table string) *int64 {
	var n int64
	// to_regclass returns NULL if the relation does not exist, letting
	// us probe for missing tables without catching a SQLSTATE.
	var exists *string
	if err := pool.QueryRow(ctx,
		`SELECT to_regclass($1)::text`, table,
	).Scan(&exists); err != nil {
		return nil
	}
	if exists == nil {
		return nil
	}
	// Safe because `table` came from a hardcoded allowlist above.
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM `+*exists,
	).Scan(&n); err != nil {
		return nil
	}
	return &n
}
