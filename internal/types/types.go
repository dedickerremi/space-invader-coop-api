package types

import "sync"

// Player represents a player in the game.
type Player struct {
	ID        string `json:"id"`
	X         int    `json:"x"`
	Alive     bool   `json:"alive"`
	Direction int    `json:"direction"` // -1 left, 0 stopped, 1 right
}

// Bullet represents a bullet in the game.
type Bullet struct {
	X       int    `json:"x"`
	Y       int    `json:"y"`
	OwnerID string `json:"ownerId"`
}

// Enemy represents an enemy mob.
type Enemy struct {
	X int `json:"x"`
	Y int `json:"y"`
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

// GameState is the full game state for a match.
type GameState struct {
	Players           []Player          `json:"players"`
	Bullets           []Bullet          `json:"bullets"`
	Enemies           []Enemy           `json:"enemies"`
	Lives             int               `json:"lives"`
	Points            map[string]int    `json:"points"`  // playerId -> points
	Kills             map[string]int    `json:"kills"`   // playerId -> kills
	WaveNumber        int               `json:"waveNumber"`
	Started           bool              `json:"started"`
	Paused            bool              `json:"paused"`
	PausedBy          *string           `json:"pausedBy,omitempty"`
	GameOver          bool              `json:"gameOver"`
	GameOverSummary   *GameOverSummary  `json:"gameOverSummary,omitempty"`
	NextWaveCountdown int               `json:"nextWaveCountdown"` // ticks until next wave (internal, can expose for UI)
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
}

// --- Client messages (inputs) ---

// ClientMessage is a union of all client message types.
type ClientMessage struct {
	Type string `json:"type"`
	Dir  *int   `json:"dir,omitempty"` // for MOVE: -1 or 1
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
