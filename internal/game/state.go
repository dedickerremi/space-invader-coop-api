package game

import (
	"fmt"
	"math"

	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/types"
)

const (
	playerSpeed  = 5
	bulletSpeed  = 8
	gameWidth    = 800
	gameHeight   = 600
	playerY      = 550
)

// GetState returns a deep copy of the game state for a match, or nil.
func GetState(matchID string) *types.GameState {
	m := match.GetMatch(matchID)
	if m == nil {
		return nil
	}
	m.Mu.RLock()
	state := deepCopyState(&m.State)
	m.Mu.RUnlock()
	return state
}

func deepCopyState(s *types.GameState) *types.GameState {
	players := make([]types.Player, len(s.Players))
	copy(players, s.Players)
	bullets := make([]types.Bullet, len(s.Bullets))
	copy(bullets, s.Bullets)
	var pausedBy *string
	if s.PausedBy != nil {
		p := *s.PausedBy
		pausedBy = &p
	}
	return &types.GameState{
		Players:  players,
		Bullets:  bullets,
		Started:  s.Started,
		Paused:   s.Paused,
		PausedBy: pausedBy,
	}
}

// AddPlayer adds a player to the match and returns the player, or nil.
func AddPlayer(matchID, playerID string) *types.Player {
	m := match.GetMatch(matchID)
	if m == nil {
		return nil
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	x := 200
	if len(m.State.Players) > 0 {
		x = 600
	}
	p := types.Player{
		ID:        playerID,
		X:         x,
		Alive:     true,
		Direction: 0,
	}
	m.State.Players = append(m.State.Players, p)
	if len(m.State.Players) == 2 {
		m.State.Started = true
	}
	return &p
}

// RemovePlayer removes a player from the match.
func RemovePlayer(matchID, playerID string) {
	m := match.GetMatch(matchID)
	if m == nil {
		return
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	var newPlayers []types.Player
	for _, p := range m.State.Players {
		if p.ID != playerID {
			newPlayers = append(newPlayers, p)
		}
	}
	m.State.Players = newPlayers
	var newBullets []types.Bullet
	for _, b := range m.State.Bullets {
		if b.OwnerID != playerID {
			newBullets = append(newBullets, b)
		}
	}
	m.State.Bullets = newBullets
	if m.State.PausedBy != nil && *m.State.PausedBy == playerID {
		m.State.Paused = false
		m.State.PausedBy = nil
	}
	if len(m.State.Players) < 2 {
		m.State.Started = false
	}
}

// SetPlayerDirection sets the movement direction for a player.
func SetPlayerDirection(matchID, playerID string, dir int) {
	m := match.GetMatch(matchID)
	if m == nil {
		return
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if m.State.Paused {
		return
	}
	for i := range m.State.Players {
		if m.State.Players[i].ID == playerID && m.State.Players[i].Alive {
			m.State.Players[i].Direction = dir
			return
		}
	}
}

// PlayerShoot fires a bullet for the player.
func PlayerShoot(matchID, playerID string) {
	m := match.GetMatch(matchID)
	if m == nil {
		return
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if m.State.Paused || !m.State.Started {
		return
	}
	var px int
	for i := range m.State.Players {
		if m.State.Players[i].ID == playerID && m.State.Players[i].Alive {
			px = m.State.Players[i].X
			break
		}
	}
	bullet := types.Bullet{
		X:       px,
		Y:       playerY - 10,
		OwnerID: playerID,
	}
	m.State.Bullets = append(m.State.Bullets, bullet)
}

// PauseGame pauses the match for the given player.
func PauseGame(matchID, playerID string) bool {
	m := match.GetMatch(matchID)
	if m == nil {
		return false
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if !m.State.Started || m.State.Paused {
		return false
	}
	m.State.Paused = true
	m.State.PausedBy = &playerID
	fmt.Printf("[GAME] Match %s paused by %s\n", matchID, playerID)
	return true
}

// ResumeGame resumes the match.
func ResumeGame(matchID, playerID string) bool {
	m := match.GetMatch(matchID)
	if m == nil {
		return false
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if !m.State.Paused {
		return false
	}
	if m.State.PausedBy != nil && *m.State.PausedBy != playerID {
		return false
	}
	m.State.Paused = false
	m.State.PausedBy = nil
	fmt.Printf("[GAME] Match %s resumed by %s\n", matchID, playerID)
	return true
}

// Tick advances the game state by one frame.
func Tick(matchID string) {
	m := match.GetMatch(matchID)
	if m == nil {
		return
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if !m.State.Started || m.State.Paused {
		return
	}
	for i := range m.State.Players {
		if !m.State.Players[i].Alive {
			continue
		}
		m.State.Players[i].X += m.State.Players[i].Direction * playerSpeed
		m.State.Players[i].X = int(math.Max(20, math.Min(float64(gameWidth-20), float64(m.State.Players[i].X))))
	}
	var newBullets []types.Bullet
	for _, b := range m.State.Bullets {
		b.Y -= bulletSpeed
		if b.Y > 0 {
			newBullets = append(newBullets, b)
		}
	}
	m.State.Bullets = newBullets
}

// GetPlayerCount returns the number of players in the match.
func GetPlayerCount(matchID string) int {
	m := match.GetMatch(matchID)
	if m == nil {
		return 0
	}
	m.Mu.RLock()
	n := len(m.State.Players)
	m.Mu.RUnlock()
	return n
}
