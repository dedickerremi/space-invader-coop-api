package game

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// levelsDir is the directory where level JSON files are stored on disk.
// Falls back to embedded files if the directory does not exist or a file is missing.
var (
	levelsDir   = "./internal/game/levels"
	levelsMu    sync.RWMutex
	currentName = "level1.json"
)

func init() {
	if d := os.Getenv("LEVELS_DIR"); d != "" {
		levelsDir = d
	}
}

// SetLevelsDir overrides the levels directory (useful for tests/configuration).
func SetLevelsDir(dir string) {
	levelsMu.Lock()
	defer levelsMu.Unlock()
	levelsDir = dir
}

// SeedLevelsDir copies embedded level files to the levels directory if missing.
// Safe to call at startup; existing files on disk are never overwritten.
func SeedLevelsDir() error {
	levelsMu.RLock()
	dir := levelsDir
	levelsMu.RUnlock()

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

// ListLevelFiles returns the .json filenames available, preferring the on-disk
// directory and falling back to the embedded files.
func ListLevelFiles() ([]string, error) {
	levelsMu.RLock()
	dir := levelsDir
	levelsMu.RUnlock()

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

// ReadLevelFile returns the raw bytes of a level file, preferring disk over embed.
func ReadLevelFile(name string) ([]byte, error) {
	if err := validateLevelName(name); err != nil {
		return nil, err
	}

	levelsMu.RLock()
	dir := levelsDir
	levelsMu.RUnlock()

	if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
		return data, nil
	}
	return levelFiles.ReadFile("levels/" + name)
}

// WriteLevelFile validates the JSON, ensures the directory exists, and writes the file.
func WriteLevelFile(name string, data []byte) error {
	if err := validateLevelName(name); err != nil {
		return err
	}
	if _, err := ParseLevelJSON(data); err != nil {
		return fmt.Errorf("invalid level JSON: %w", err)
	}

	levelsMu.Lock()
	dir := levelsDir
	levelsMu.Unlock()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create levels dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0o644)
}

// DeleteLevelFile removes a level file from disk. Embedded files cannot be deleted.
func DeleteLevelFile(name string) error {
	if err := validateLevelName(name); err != nil {
		return err
	}

	levelsMu.RLock()
	dir := levelsDir
	levelsMu.RUnlock()

	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("level not found on disk (cannot delete embedded fallback)")
	}
	return os.Remove(path)
}

// ReloadCurrentLevel re-reads the active level from disk/embed and replaces the
// in-memory cache. New matches will use the reloaded definition.
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
