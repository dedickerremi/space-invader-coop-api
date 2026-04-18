package game

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// levelsDir is the directory where level JSON files are stored on disk.
// Used as a fallback when the DB is not configured.
var (
	levelsDir   = "./internal/game/levels"
	levelsMu    sync.RWMutex
	currentName = "level1.json"

	// dbPool is the optional Postgres pool. When non-nil, all level storage
	// operations go through the DB. When nil, the filesystem + embed fallback
	// is used (useful for local dev without Neon).
	dbPool *pgxpool.Pool
)

func init() {
	if d := os.Getenv("LEVELS_DIR"); d != "" {
		levelsDir = d
	}
}

// UseDB enables Postgres-backed level storage. Pass the shared pool from
// the db package. Passing nil disables DB storage (tests, local dev).
func UseDB(p *pgxpool.Pool) {
	levelsMu.Lock()
	defer levelsMu.Unlock()
	dbPool = p
}

// SetLevelsDir overrides the levels directory (useful for tests/configuration).
func SetLevelsDir(dir string) {
	levelsMu.Lock()
	defer levelsMu.Unlock()
	levelsDir = dir
}

// SeedLevels ensures the active storage backend has the embedded level set.
// - DB mode: if the `levels` table is empty, inserts every embedded level.
// - Filesystem mode: copies embedded files to levelsDir if missing.
// Existing rows / files are never overwritten.
func SeedLevels(ctx context.Context) error {
	levelsMu.RLock()
	pool := dbPool
	dir := levelsDir
	levelsMu.RUnlock()

	if pool != nil {
		return seedLevelsDB(ctx, pool)
	}
	return seedLevelsDir(dir)
}

func seedLevelsDB(ctx context.Context, pool *pgxpool.Pool) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM levels`).Scan(&count); err != nil {
		return fmt.Errorf("count levels: %w", err)
	}
	if count > 0 {
		return nil
	}

	entries, err := levelFiles.ReadDir("levels")
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := levelFiles.ReadFile("levels/" + name)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO levels (name, definition) VALUES ($1, $2::jsonb)
			 ON CONFLICT (name) DO NOTHING`,
			name, string(data)); err != nil {
			return fmt.Errorf("seed %s: %w", name, err)
		}
		fmt.Printf("[LEVELS] Seeded %s into DB\n", name)
	}
	return nil
}

func seedLevelsDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create levels dir: %w", err)
	}
	entries, err := levelFiles.ReadDir("levels")
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		dst := filepath.Join(dir, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := levelFiles.ReadFile("levels/" + name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("[LEVELS] Seeded %s -> %s\n", name, dst)
	}
	return nil
}

// SeedLevelsDir is kept for backwards compatibility; prefer SeedLevels.
func SeedLevelsDir() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return SeedLevels(ctx)
}

// ListLevelFiles returns the available level names (with .json suffix).
func ListLevelFiles() ([]string, error) {
	levelsMu.RLock()
	pool := dbPool
	dir := levelsDir
	levelsMu.RUnlock()

	if pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rows, err := pool.Query(ctx, `SELECT name FROM levels ORDER BY name`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, rows.Err()
	}

	seen := map[string]bool{}
	var out []string
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	if entries, err := levelFiles.ReadDir("levels"); err == nil {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// ReadLevelFile returns the raw JSON bytes of a level.
func ReadLevelFile(name string) ([]byte, error) {
	if err := validateLevelName(name); err != nil {
		return nil, err
	}

	levelsMu.RLock()
	pool := dbPool
	dir := levelsDir
	levelsMu.RUnlock()

	if pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var def []byte
		err := pool.QueryRow(ctx,
			`SELECT definition::text FROM levels WHERE name = $1`, name,
		).Scan(&def)
		if err == nil {
			return def, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		// Fall through to embed fallback when row is missing.
	} else if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
		return data, nil
	}
	return levelFiles.ReadFile("levels/" + name)
}

// WriteLevelFile validates the JSON and upserts the level.
func WriteLevelFile(name string, data []byte) error {
	if err := validateLevelName(name); err != nil {
		return err
	}
	if _, err := ParseLevelJSON(data); err != nil {
		return fmt.Errorf("invalid level JSON: %w", err)
	}

	levelsMu.RLock()
	pool := dbPool
	dir := levelsDir
	levelsMu.RUnlock()

	if pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := pool.Exec(ctx,
			`INSERT INTO levels (name, definition) VALUES ($1, $2::jsonb)
			 ON CONFLICT (name) DO UPDATE
			 SET definition = EXCLUDED.definition, updated_at = now()`,
			name, string(data))
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create levels dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0o644)
}

// DeleteLevelFile removes a level. Embedded fallbacks cannot be deleted.
func DeleteLevelFile(name string) error {
	if err := validateLevelName(name); err != nil {
		return err
	}

	levelsMu.RLock()
	pool := dbPool
	dir := levelsDir
	levelsMu.RUnlock()

	if pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tag, err := pool.Exec(ctx, `DELETE FROM levels WHERE name = $1`, name)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("level not found")
		}
		return nil
	}

	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("level not found on disk (cannot delete embedded fallback)")
	}
	return os.Remove(path)
}

// ReloadCurrentLevel re-reads the active level and replaces the in-memory
// cache. New matches pick up the reloaded definition.
func ReloadCurrentLevel() error {
	data, err := ReadLevelFile(currentName)
	if err != nil {
		return err
	}
	level, err := ParseLevelJSON(data)
	if err != nil {
		return err
	}
	levelsMu.Lock()
	currentLevel = level
	levelsMu.Unlock()
	return nil
}

// CurrentLevelName returns the filename of the level used by new matches.
func CurrentLevelName() string {
	levelsMu.RLock()
	defer levelsMu.RUnlock()
	return currentName
}

func validateLevelName(name string) error {
	if name == "" {
		return fmt.Errorf("empty level name")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid level name")
	}
	if !strings.HasSuffix(name, ".json") {
		return fmt.Errorf("level name must end with .json")
	}
	return nil
}
