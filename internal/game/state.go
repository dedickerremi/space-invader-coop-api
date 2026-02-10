package game

import (
	"fmt"
	"math"
	"math/rand"

	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/types"
)

const (
	playerSpeed        = 5
	bulletSpeed        = 8
	gameWidth          = 800
	gameHeight         = 600
	playerY            = 550
	playerWidth        = 40
	playerHeight       = 20
	enemySize             = 24
	enemySpeed            = 2
	waveIntervalTicks     = 90   // start: ~3s between waves
	minWaveIntervalTicks  = 35   // minimum: ~1.2s between waves (ramp stops here)
	waveIntervalDecrease  = 4   // decrease interval by this many ticks per wave
	minEnemiesPerWave     = 2
	maxEnemiesPerWave     = 12  // cap so late game doesn't explode
	enemiesPerWaveBonus   = 1   // +1 enemy per wave (wave 1: 2, wave 5: 6, etc.)
	initialLives          = 3
	pointsPerKill         = 100
	enemySpawnMargin      = 40
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
	enemies := make([]types.Enemy, len(s.Enemies))
	copy(enemies, s.Enemies)
	points := make(map[string]int)
	for k, v := range s.Points {
		points[k] = v
	}
	kills := make(map[string]int)
	for k, v := range s.Kills {
		kills[k] = v
	}
	var pausedBy *string
	if s.PausedBy != nil {
		p := *s.PausedBy
		pausedBy = &p
	}
	var summary *types.GameOverSummary
	if s.GameOverSummary != nil {
		scores := make([]types.PlayerScore, len(s.GameOverSummary.PlayerScores))
		copy(scores, s.GameOverSummary.PlayerScores)
		summary = &types.GameOverSummary{PlayerScores: scores}
	}
	return &types.GameState{
		Players:           players,
		Bullets:           bullets,
		Enemies:           enemies,
		Lives:             s.Lives,
		Points:            points,
		Kills:             kills,
		WaveNumber:        s.WaveNumber,
		Started:           s.Started,
		Paused:            s.Paused,
		PausedBy:          pausedBy,
		GameOver:          s.GameOver,
		GameOverSummary:   summary,
		NextWaveCountdown: s.NextWaveCountdown,
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
		m.State.Lives = initialLives
		m.State.Points = make(map[string]int)
		m.State.Kills = make(map[string]int)
		for _, pl := range m.State.Players {
			m.State.Points[pl.ID] = 0
			m.State.Kills[pl.ID] = 0
		}
		m.State.NextWaveCountdown = waveIntervalTicks
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
	if m.State.Paused || m.State.GameOver {
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
	if m.State.Paused || !m.State.Started || m.State.GameOver {
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
	if !m.State.Started || m.State.Paused || m.State.GameOver {
		return
	}

	// Wave spawn (incremental: more enemies + shorter interval each wave)
	m.State.NextWaveCountdown--
	if m.State.NextWaveCountdown <= 0 {
		m.State.WaveNumber++
		// Next wave comes sooner each time, down to a minimum
		nextInterval := waveIntervalTicks - (m.State.WaveNumber-1)*waveIntervalDecrease
		if nextInterval < minWaveIntervalTicks {
			nextInterval = minWaveIntervalTicks
		}
		m.State.NextWaveCountdown = nextInterval
		// More enemies per wave as game progresses (capped)
		baseEnemies := minEnemiesPerWave + (m.State.WaveNumber-1)*enemiesPerWaveBonus
		if baseEnemies > maxEnemiesPerWave {
			baseEnemies = maxEnemiesPerWave
		}
		n := baseEnemies + rand.Intn(2) // small random +0 or +1
		if n > maxEnemiesPerWave {
			n = maxEnemiesPerWave
		}
		for i := 0; i < n; i++ {
			x := enemySpawnMargin + rand.Intn(gameWidth-2*enemySpawnMargin)
			m.State.Enemies = append(m.State.Enemies, types.Enemy{X: x, Y: 20})
		}
	}

	// Player movement
	for i := range m.State.Players {
		if !m.State.Players[i].Alive {
			continue
		}
		m.State.Players[i].X += m.State.Players[i].Direction * playerSpeed
		m.State.Players[i].X = int(math.Max(20, math.Min(float64(gameWidth-20), float64(m.State.Players[i].X))))
	}

	// Bullet–enemy collision (bullet hits enemy -> remove both, add points/kill to owner)
	type pair struct{ bi, ei int }
	var toRemove []pair
	for bi, b := range m.State.Bullets {
		for ei, e := range m.State.Enemies {
			if bulletHitEnemy(b.X, b.Y, e.X, e.Y) {
				toRemove = append(toRemove, pair{bi, ei})
				if m.State.Points != nil {
					m.State.Points[b.OwnerID] += pointsPerKill
					m.State.Kills[b.OwnerID]++
				}
				goto nextBullet
			}
		}
	nextBullet:
	}
	// Remove hit bullets and enemies (reverse order to preserve indices)
	hitEnemies := make(map[int]bool)
	for _, p := range toRemove {
		hitEnemies[p.ei] = true
	}
	var newBullets []types.Bullet
	for bi, b := range m.State.Bullets {
		removed := false
		for _, p := range toRemove {
			if p.bi == bi {
				removed = true
				break
			}
		}
		if !removed {
			b.Y -= bulletSpeed
			if b.Y > 0 {
				newBullets = append(newBullets, b)
			}
		}
	}
	m.State.Bullets = newBullets
	var newEnemies []types.Enemy
	for ei, e := range m.State.Enemies {
		if hitEnemies[ei] {
			continue
		}
		e.Y += enemySpeed
		if e.Y < gameHeight {
			newEnemies = append(newEnemies, e)
		} else {
			// Enemy reached bottom -> lose a life
			m.State.Lives--
		}
	}
	m.State.Enemies = newEnemies

	// Enemy–player collision
	var finalEnemies []types.Enemy
	for _, e := range m.State.Enemies {
		hitPlayer := false
		for pi := range m.State.Players {
			if !m.State.Players[pi].Alive {
				continue
			}
			if enemyHitPlayer(e.X, e.Y, m.State.Players[pi].X, playerY) {
				hitPlayer = true
				m.State.Lives--
				break
			}
		}
		if !hitPlayer {
			finalEnemies = append(finalEnemies, e)
		}
	}
	m.State.Enemies = finalEnemies

	// Game over
	if m.State.Lives <= 0 {
		m.State.GameOver = true
		scores := make([]types.PlayerScore, 0, len(m.State.Players))
		for _, p := range m.State.Players {
			pts := 0
			k := 0
			if m.State.Points != nil {
				pts = m.State.Points[p.ID]
			}
			if m.State.Kills != nil {
				k = m.State.Kills[p.ID]
			}
			scores = append(scores, types.PlayerScore{PlayerID: p.ID, Points: pts, Kills: k})
		}
		m.State.GameOverSummary = &types.GameOverSummary{PlayerScores: scores}
	}
}

func bulletHitEnemy(bx, by, ex, ey int) bool {
	half := enemySize / 2
	return bx >= ex-half && bx <= ex+half && by >= ey-half && by <= ey+half
}

func enemyHitPlayer(ex, ey, px, py int) bool {
	// AABB: enemy center (ex,ey), size enemySize; player center (px,py), size playerWidth x playerHeight
	half := enemySize / 2
	pw2 := playerWidth / 2
	ph2 := playerHeight / 2
	return !(ex+half < px-pw2 || ex-half > px+pw2 || ey+half < py-ph2 || ey-half > py+ph2)
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
