package types

import "sync"

// Player represents a player in the game.
type Player struct {
	ID              string `json:"id"`
	UserID          string `json:"userId,omitempty"`      // Clerk user id, empty for guests
	DisplayName     string `json:"displayName,omitempty"` // Clerk profile name (empty for guests)
	X               int    `json:"x"`
	Y               int    `json:"y"`
	Alive           bool   `json:"alive"`
	Direction       int    `json:"direction"`       // -1 left, 0 stopped, 1 right
	DirectionY      int    `json:"directionY"`      // -1 forward (up), 0 stopped, 1 backward (down)
	Lives           int    `json:"lives"`            // individual lives
	InvincibleTimer int    `json:"invincibleTimer"`  // ticks of invincibility after respawn (0 = vulnerable)
	DoubleShotTimer int    `json:"doubleShotTimer"`  // ticks of double-shot power-up remaining
	SpeedBoostTimer int    `json:"speedBoostTimer"`  // ticks of speed-boost power-up remaining
	ShieldTimer     int    `json:"shieldTimer"`      // ticks of shield power-up remaining
	SpawnX          int    `json:"-"`                // starting X for respawn
	SpawnY          int    `json:"-"`                // starting Y for respawn
}

// PowerUp is a falling pickup that grants an effect when collected.
type PowerUp struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Kind string `json:"kind"` // extra_life | double_shot | speed_boost | shield | points_bonus
}

// Bullet represents a player bullet in the game.
type Bullet struct {
	X       int    `json:"x"`
	Y       int    `json:"y"`
	OwnerID string `json:"ownerId"`
}

// EnemyBullet represents a bullet fired by an enemy or boss.
type EnemyBullet struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	DX   float64 `json:"dx"`             // velocity X per tick
	DY   float64 `json:"dy"`             // velocity Y per tick
	Kind string  `json:"kind,omitempty"` // "" (default) | "aimed" | "comet" — hint for frontend styling
}

// Spark is a short-lived visual effect (e.g. bullet-vs-bullet collision).
type Spark struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	TTL  int     `json:"ttl"`  // ticks remaining
	Life int     `json:"life"` // initial TTL (for client-side fade ratio)
	Kind string  `json:"kind"` // "bullet" for now
}

// Enemy represents an enemy mob.
type Enemy struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Type string `json:"type"` // "static" or "patrol"

	// Internal fields (not sent to client via JSON tags with -)
	SpawnX     int `json:"-"` // original X for patrol pattern
	PatternDir int `json:"-"` // current horizontal direction for patrol: -1 or 1
	ShootTimer int `json:"-"` // ticks until next shot
}

// PlayerScore is one player's score in the game-over summary.
type PlayerScore struct {
	PlayerID string `json:"playerId"`
	Points   int    `json:"points"`
	Kills    int    `json:"kills"`
}

// GameOverSummary is sent when the game ends (no lives left).
type GameOverSummary struct {
	PlayerScores []PlayerScore `json:"playerScores"`
}

// Boss is the end-of-level boss entity. Absent from STATE when no boss
// fight is active. Internal timers (marked `json:"-"`) drive the boss
// behavior per kind.
type Boss struct {
	Kind         string  `json:"kind"` // "sentinel" | "warden" | "citadel" | "nexus"
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	HP           int     `json:"hp"`
	MaxHP        int     `json:"maxHp"`
	Phase        int     `json:"phase"`
	ShieldActive bool    `json:"shieldActive,omitempty"`

	// Internal — not serialized.
	PatternDir    int `json:"-"` // horizontal travel direction (-1 / +1)
	PatternTick   int `json:"-"` // ticks since spawn, for movement curves
	AttackState   int `json:"-"` // 0 = resting, 1 = firing burst
	AttackTimer   int `json:"-"` // ticks until next event in the attack cycle
	AttackShotsLeft int `json:"-"` // bullets remaining in the current burst
	SummonTimer   int `json:"-"` // ticks until the next escort summon (warden)
}

// GameState is the full game state for a match.
type GameState struct {
	Players           []Player          `json:"players"`
	Bullets           []Bullet          `json:"bullets"`
	EnemyBullets      []EnemyBullet     `json:"enemyBullets"`
	Enemies           []Enemy           `json:"enemies"`
	Sparks            []Spark           `json:"sparks"`
	PowerUps          []PowerUp         `json:"powerUps"`
	KillStreaks       map[string]int    `json:"killStreaks"`
	Lives             int               `json:"lives"`
	Points            map[string]int    `json:"points"`  // playerId -> points
	Kills             map[string]int    `json:"kills"`   // playerId -> kills
	LevelName         string            `json:"levelName"`
	WaveNumber        int               `json:"waveNumber"`
	WaveName          string            `json:"waveName"`
	TotalWaves        int               `json:"totalWaves"`
	Started           bool              `json:"started"`
	Paused            bool              `json:"paused"`
	PausedBy          *string           `json:"pausedBy,omitempty"`
	GameOver          bool              `json:"gameOver"`
	Victory           bool              `json:"victory,omitempty"`
	GameOverSummary   *GameOverSummary  `json:"gameOverSummary,omitempty"`
	NextWaveCountdown int               `json:"nextWaveCountdown"`
	Boss              *Boss             `json:"boss,omitempty"`

	// Internal wave tracking (not sent to client)
	WaveTick      int  `json:"-"` // ticks since current wave started
	WaveCleared   bool `json:"-"` // all enemies from current wave are dead/gone
	WaveCooldown  int  `json:"-"` // ticks to wait before starting next wave
	BossDefeated  bool `json:"-"` // true once the current level's boss has been killed
}

// Match holds match metadata and game state.
// Mu protects State and PlayerIDs/Tokens from concurrent access.
type Match struct {
	Mu        sync.RWMutex
	MatchID   string
	PlayerIDs []string
	Tokens    map[string]string // playerId -> token
	State     GameState
	CreatedAt int64
	Mode      string // "solo" or "coop"
}

// --- Client messages (inputs) ---

// ClientMessage is a union of all client message types.
type ClientMessage struct {
	Type      string   `json:"type"`
	Dir       *int     `json:"dir,omitempty"`       // for MOVE: -1, 0, or 1 (X axis)
	DirY      *int     `json:"dirY,omitempty"`      // for MOVE: -1, 0, or 1 (Y axis)
	Timestamp *float64 `json:"timestamp,omitempty"` // for PING: client timestamp in ms
}

// PongMessage is the server response to a client PING.
type PongMessage struct {
	Type      string  `json:"type"`
	Timestamp float64 `json:"timestamp"` // echo back the client timestamp
}

// --- Server messages (outputs) ---

// StateMessage is sent every tick with current game state.
type StateMessage struct {
	Type  string    `json:"type"`
	State GameState `json:"state"`
}

// WelcomeMessage is sent on successful connection.
type WelcomeMessage struct {
	Type     string `json:"type"`
	PlayerID string `json:"playerId"`
	MatchID  string `json:"matchId"`
	Mode     string `json:"mode"`
}

// ErrorMessage is sent on error.
type ErrorMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// MatchEndedMessage is sent when match ends.
type MatchEndedMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}
