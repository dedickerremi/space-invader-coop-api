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

	// levelCache maps level name -> parsed definition. Populated lazily on
	// first access and cleared wholesale by ReloadCurrentLevel so the editor
	// "reload" endpoint picks up freshly saved definitions.
	levelCache = make(map[string]*LevelDefinition)

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
	// Idempotent: insert every embedded level that isn't already in the DB.
	// Legacy rows are left untouched (admins may have edited them via the
	// editor); campaign rows follow the repo. Runs on every boot so new and
	// retuned embedded levels land on the next deploy without manual import.
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
		// Campaign levels are tuned in the repo, so the repo wins: a changed
		// file replaces the stored row on the next deploy. jsonb equality is
		// semantic, so an unchanged level is not rewritten on every boot.
		// Legacy levels keep the insert-only rule.
		query := `INSERT INTO levels (name, definition) VALUES ($1, $2::jsonb)
			 ON CONFLICT (name) DO NOTHING`
		if isCampaignLevel(name) {
			query = `INSERT INTO levels (name, definition) VALUES ($1, $2::jsonb)
			 ON CONFLICT (name) DO UPDATE
			   SET definition = EXCLUDED.definition, updated_at = now()
			   WHERE levels.definition IS DISTINCT FROM EXCLUDED.definition`
		}
		tag, err := pool.Exec(ctx, query, name, string(data))
		if err != nil {
			return fmt.Errorf("seed %s: %w", name, err)
		}
		if tag.RowsAffected() > 0 {
			fmt.Printf("[LEVELS] Seeded %s into DB\n", name)
		}
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

// ReloadCurrentLevel clears the in-memory level cache. The next Tick that
// needs a level will re-parse it from the active storage backend. Called by
// the editor "reload" endpoint after a save.
func ReloadCurrentLevel() error {
	levelsMu.Lock()
	levelCache = make(map[string]*LevelDefinition)
	levelsMu.Unlock()
	return nil
}

// CurrentLevelName returns the filename of the level used by new matches.
func CurrentLevelName() string {
	levelsMu.RLock()
	defer levelsMu.RUnlock()
	return currentName
}

// Solo and coop play separate campaigns. A campaign is every level whose
// name starts with its prefix ("solo-level1.json", "coop-level1.json"),
// played in alphabetical order. Levels without a campaign prefix are the
// legacy shared set, still used as a fallback when a campaign is missing.
func campaignPrefix(mode string) string {
	if mode == "solo" {
		return "solo-"
	}
	return "coop-"
}

func isCampaignLevel(name string) bool {
	return strings.HasPrefix(name, "solo-") || strings.HasPrefix(name, "coop-")
}

// campaignOf returns the levels sharing name's campaign, in play order.
func campaignOf(files []string, prefix string) []string {
	var out []string
	for _, f := range files {
		if prefix == "" && !isCampaignLevel(f) || prefix != "" && strings.HasPrefix(f, prefix) {
			out = append(out, f)
		}
	}
	return out
}

// CampaignLevels returns the levels of the campaign played in mode, in play
// order, falling back to the legacy shared set when the campaign is empty.
func CampaignLevels(mode string) []string {
	files, err := ListLevelFiles()
	if err != nil {
		return nil
	}
	if levels := campaignOf(files, campaignPrefix(mode)); len(levels) > 0 {
		return levels
	}
	return campaignOf(files, "")
}

// FirstLevel returns the first level of the campaign played in mode. Returns
// "" when no level is available (a fatal configuration error).
func FirstLevel(mode string) string {
	if levels := CampaignLevels(mode); len(levels) > 0 {
		return levels[0]
	}
	return ""
}

// NextLevel returns the level after current within current's campaign, or ""
// when current is the campaign's last level (→ victory condition).
func NextLevel(current string) string {
	files, err := ListLevelFiles()
	if err != nil {
		return ""
	}
	prefix := ""
	for _, p := range []string{"solo-", "coop-"} {
		if strings.HasPrefix(current, p) {
			prefix = p
		}
	}
	levels := campaignOf(files, prefix)
	for i, f := range levels {
		if f == current && i+1 < len(levels) {
			return levels[i+1]
		}
	}
	return ""
}

// GetLevelByName returns the parsed level definition for `name`, using a
// process-wide cache. Returns nil when the level can't be loaded.
func GetLevelByName(name string) *LevelDefinition {
	if name == "" {
		return nil
	}
	levelsMu.RLock()
	cached, ok := levelCache[name]
	levelsMu.RUnlock()
	if ok {
		return cached
	}
	level, err := LoadLevel(name)
	if err != nil {
		fmt.Printf("[GAME] Warning: could not load level %q: %v\n", name, err)
		return nil
	}
	levelsMu.Lock()
	levelCache[name] = level
	levelsMu.Unlock()
	return level
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
