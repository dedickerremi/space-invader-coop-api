package game

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed levels/*.json
var levelFiles embed.FS

// --- Enemy kinds ---

type EnemyKind string

const (
	EnemyStatic EnemyKind = "static"
	EnemyPatrol EnemyKind = "patrol"
)

// --- Spawn event (one enemy to create) ---

type SpawnEvent struct {
	TickOffset int       // ticks after wave start to spawn
	Kind       EnemyKind // enemy type
	X          int       // horizontal position (px)
	Y          int       // vertical position (px)
}

// --- Wave definition ---

type WaveDefinition struct {
	Number int
	Name   string
	Spawns []SpawnEvent
}

// --- Level definition ---

type LevelDefinition struct {
	Waves []WaveDefinition
}

// --- JSON schema ---

type jsonLevel struct {
	Waves []jsonWave `json:"waves"`
}

type jsonWave struct {
	Name     string   `json:"name"`
	RowDelay int      `json:"rowDelay"` // ticks between rows (default 30)
	Rows     []string `json:"rows"`
}

const (
	defaultRowDelay = 30
	defaultSpawnY   = 20
	gridMargin      = 40
)

// LoadLevel loads and parses a level JSON file.
func LoadLevel(filename string) (*LevelDefinition, error) {
	data, err := levelFiles.ReadFile("levels/" + filename)
	if err != nil {
		return nil, fmt.Errorf("read level file %s: %w", filename, err)
	}
	return ParseLevelJSON(data)
}

// ParseLevelJSON parses level JSON data.
func ParseLevelJSON(data []byte) (*LevelDefinition, error) {
	var raw jsonLevel
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid level JSON: %w", err)
	}

	if len(raw.Waves) == 0 {
		return nil, fmt.Errorf("level has no waves")
	}

	level := &LevelDefinition{}

	for i, jw := range raw.Waves {
		rowDelay := jw.RowDelay
		if rowDelay <= 0 {
			rowDelay = defaultRowDelay
		}

		wave := WaveDefinition{
			Number: i + 1,
			Name:   jw.Name,
		}

		for rowIdx, row := range jw.Rows {
			cols := len(row)
			if cols == 0 {
				continue
			}

			tickOffset := rowIdx * rowDelay

			for colIdx, ch := range row {
				var kind EnemyKind
				switch ch {
				case 'S', 's':
					kind = EnemyStatic
				case 'P', 'p':
					kind = EnemyPatrol
				default:
					continue
				}

				// Map column to X position
				usableWidth := gameWidth - 2*gridMargin
				var x int
				if cols <= 1 {
					x = gameWidth / 2
				} else {
					x = gridMargin + (colIdx * usableWidth / (cols - 1))
				}

				wave.Spawns = append(wave.Spawns, SpawnEvent{
					TickOffset: tickOffset,
					Kind:       kind,
					X:          x,
					Y:          defaultSpawnY,
				})
			}
		}

		if len(wave.Spawns) > 0 {
			level.Waves = append(level.Waves, wave)
		}
	}

	if len(level.Waves) == 0 {
		return nil, fmt.Errorf("no valid waves in level")
	}

	fmt.Printf("[LEVELS] Loaded %d waves\n", len(level.Waves))
	for _, w := range level.Waves {
		fmt.Printf("[LEVELS]   Wave %d (%s): %d enemies\n", w.Number, w.Name, len(w.Spawns))
	}

	return level, nil
}
