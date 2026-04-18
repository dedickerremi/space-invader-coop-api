// Package db provides a lazily-initialized Postgres connection pool and
// runs embedded SQL migrations on startup.
//
// Usage: call Init() once at boot. If DATABASE_URL is unset, Init returns
// nil and Pool() returns nil — callers should fall back to filesystem
// storage in that case.
package db

import (
	"context"
	"embed"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var pool *pgxpool.Pool

// Init opens the connection pool (if DATABASE_URL is set) and applies
// embedded migrations. Returns nil if DATABASE_URL is empty so callers
// can fall back to filesystem-backed storage.
func Init(ctx context.Context) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Println("[DB] DATABASE_URL not set, skipping Postgres init")
		return nil
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 5 * time.Minute

	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := p.Ping(pingCtx); err != nil {
		p.Close()
		return fmt.Errorf("ping: %w", err)
	}

	pool = p
	fmt.Println("[DB] Connected to Postgres")

	if err := runMigrations(ctx); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	return nil
}

// Pool returns the active pool, or nil if the DB was not initialized.
func Pool() *pgxpool.Pool {
	return pool
}

// Close shuts down the pool, if any.
func Close() {
	if pool != nil {
		pool.Close()
		pool = nil
	}
}

func runMigrations(ctx context.Context) error {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		sqlBytes, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		fmt.Printf("[DB] Migration %s applied\n", name)
	}
	return nil
}
