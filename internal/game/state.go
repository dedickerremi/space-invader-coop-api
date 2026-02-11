package game

import (
	"fmt"
	"math/rand"

	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/types"
)

// currentLevel holds the loaded level definition. Initialized on first use.
var currentLevel *LevelDefinition

func getLevel() *LevelDefinition {
	if currentLevel == nil {
		level, err := LoadLevel("level1.json")
		if err != nil {
			fmt.Printf("[GAME] Warning: could not load level: %v, using fallback\n", err)
			currentLevel = &LevelDefinition{
				Waves: []WaveDefinition{
					{Number: 1, Spawns: []SpawnEvent{
						{TickOffset: 0, Kind: EnemyStatic, X: 200, Y: 20},
						{TickOffset: 0, Kind: EnemyStatic, X: 400, Y: 20},
						{TickOffset: 0, Kind: EnemyStatic, X: 600, Y: 20},
					}},
				},
			}
		} else {
			currentLevel = level
		}
	}
	return currentLevel
}

const (
	playerSpeed  = 5
	bulletSpeed  = 8
	gameWidth    = 800
	gameHeight   = 600
	playerY      = 550
	playerWidth  = 40
	playerHeight = 20
	enemySize    = 28
	initialLives = 3
	pointsPerKill         = 100
	pointsPerKillPatrol   = 250

	// Enemy movement
	staticEnemySpeed = 1          // static enemies drift down slowly
	patrolEnemySpeed = 1          // patrol enemies drift down
	patrolHorizontalSpeed = 3     // patrol horizontal zigzag speed
	patrolAmplitude       = 80    // max px from spawn X before reversing

	// Enemy shooting
	staticShootInterval  = 90     // ~3s between shots for static
	patrolShootInterval  = 120    // ~4s between shots for patrol
	enemyBulletSpeed     = 4.0    // enemy bullet speed (downward)
	enemyBulletDiagSpeed = 2.5    // diagonal X component for patrol shots

	// Wave timing
	waveCooldownTicks = 60        // ~2s pause between waves

	// Player respawn
	respawnTicks    = 150          // 5s at 30Hz
	invincibleTicks = 60           // 2s invincibility after respawn
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
	enemyBullets := make([]types.EnemyBullet, len(s.EnemyBullets))
	copy(enemyBullets, s.EnemyBullets)
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
		EnemyBullets:      enemyBullets,
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
// If the player is already in the match, returns the existing player (idempotent).
func AddPlayer(matchID, playerID string) *types.Player {
	m := match.GetMatch(matchID)
	if m == nil {
		return nil
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()

	// Check if player is already in the match (e.g. reconnect / StrictMode remount)
	for i := range m.State.Players {
		if m.State.Players[i].ID == playerID {
			return &m.State.Players[i]
		}
	}

	x := 200
	if len(m.State.Players) > 0 {
		x = 600
	}
	p := types.Player{
		ID:        playerID,
		X:         x,
		Alive:     true,
		Direction: 0,
		Lives:     initialLives,
	}
	m.State.Players = append(m.State.Players, p)
	if len(m.State.Players) == 2 {
		m.State.Started = true
		// Compute total lives for backward compat
		m.State.Lives = 0
		for _, pl := range m.State.Players {
			m.State.Lives += pl.Lives
		}
		m.State.Points = make(map[string]int)
		m.State.Kills = make(map[string]int)
		for _, pl := range m.State.Players {
			m.State.Points[pl.ID] = 0
			m.State.Kills[pl.ID] = 0
		}
		// Start wave 1
		m.State.WaveNumber = 1
		m.State.WaveTick = 0
		m.State.WaveCleared = false
		m.State.WaveCooldown = 0
		m.State.NextWaveCountdown = 0
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
	s := &m.State
	if !s.Started || s.Paused || s.GameOver {
		return
	}

	level := getLevel()

	// --- Wave management ---
	tickWaveSpawning(s, level)

	// --- Player respawn timers + invincibility ---
	tickPlayerRespawn(s)

	// --- Player movement ---
	for i := range s.Players {
		if !s.Players[i].Alive {
			continue
		}
		s.Players[i].X += s.Players[i].Direction * playerSpeed
		s.Players[i].X = clamp(s.Players[i].X, 20, gameWidth-20)
	}

	// --- Enemy AI (movement + shooting) ---
	tickEnemyAI(s)

	// --- Move player bullets + check collisions ---
	tickPlayerBullets(s)

	// --- Move enemy bullets + check collisions ---
	tickEnemyBullets(s)

	// --- Enemy-player collision ---
	tickEnemyPlayerCollision(s)

	// --- Recompute total lives (for HUD backward compat) ---
	totalLives := 0
	for _, p := range s.Players {
		totalLives += p.Lives
	}
	s.Lives = totalLives

	// --- Game over check: all players permanently dead ---
	allDead := true
	for _, p := range s.Players {
		if p.Lives > 0 || p.Alive {
			allDead = false
			break
		}
	}
	if allDead {
		s.GameOver = true
		scores := make([]types.PlayerScore, 0, len(s.Players))
		for _, p := range s.Players {
			pts, k := 0, 0
			if s.Points != nil {
				pts = s.Points[p.ID]
			}
			if s.Kills != nil {
				k = s.Kills[p.ID]
			}
			scores = append(scores, types.PlayerScore{PlayerID: p.ID, Points: pts, Kills: k})
		}
		s.GameOverSummary = &types.GameOverSummary{PlayerScores: scores}
	}
}

// --- Wave spawning ---

func tickWaveSpawning(s *types.GameState, level *LevelDefinition) {
	// If in cooldown between waves
	if s.WaveCooldown > 0 {
		s.WaveCooldown--
		s.NextWaveCountdown = s.WaveCooldown
		if s.WaveCooldown <= 0 {
			// Start next wave
			s.WaveNumber++
			s.WaveTick = 0
			s.WaveCleared = false
		}
		return
	}

	// Current wave index (loops through level waves)
	if len(level.Waves) == 0 {
		return
	}
	waveIdx := (s.WaveNumber - 1) % len(level.Waves)
	wave := &level.Waves[waveIdx]

	// Spawn enemies whose tick offset has been reached
	for _, spawn := range wave.Spawns {
		if spawn.TickOffset == s.WaveTick {
			enemy := types.Enemy{
				X:          spawn.X,
				Y:          spawn.Y,
				Type:       string(spawn.Kind),
				SpawnX:     spawn.X,
				PatternDir: 1,
				ShootTimer: randomShootDelay(spawn.Kind),
			}
			s.Enemies = append(s.Enemies, enemy)
		}
	}

	s.WaveTick++

	// Check if all spawns have been triggered and all enemies are gone
	allSpawned := true
	for _, spawn := range wave.Spawns {
		if spawn.TickOffset >= s.WaveTick {
			allSpawned = false
			break
		}
	}

	if allSpawned && len(s.Enemies) == 0 {
		// Wave cleared — start cooldown for next wave
		s.WaveCleared = true
		s.WaveCooldown = waveCooldownTicks
		s.NextWaveCountdown = waveCooldownTicks
	}
}

func randomShootDelay(kind EnemyKind) int {
	base := staticShootInterval
	if kind == EnemyPatrol {
		base = patrolShootInterval
	}
	// Randomize ±30% so enemies don't all fire in sync
	variance := base * 30 / 100
	if variance == 0 {
		return base
	}
	return base - variance + rand.Intn(2*variance)
}

// --- Enemy AI ---

func tickEnemyAI(s *types.GameState) {
	for i := range s.Enemies {
		e := &s.Enemies[i]

		switch e.Type {
		case "static":
			// Static: drift down slowly
			e.Y += staticEnemySpeed

			// Shoot straight down
			e.ShootTimer--
			if e.ShootTimer <= 0 {
				e.ShootTimer = randomShootDelay(EnemyStatic)
				s.EnemyBullets = append(s.EnemyBullets, types.EnemyBullet{
					X: float64(e.X), Y: float64(e.Y + enemySize/2),
					DX: 0, DY: enemyBulletSpeed,
				})
			}

		case "patrol":
			// Patrol: zigzag horizontally + drift down
			e.X += e.PatternDir * patrolHorizontalSpeed
			e.Y += patrolEnemySpeed

			// Reverse at amplitude bounds
			if e.X > e.SpawnX+patrolAmplitude || e.X >= gameWidth-20 {
				e.PatternDir = -1
			} else if e.X < e.SpawnX-patrolAmplitude || e.X <= 20 {
				e.PatternDir = 1
			}

			// Shoot 3 bullets: straight + 2 diagonals
			e.ShootTimer--
			if e.ShootTimer <= 0 {
				e.ShootTimer = randomShootDelay(EnemyPatrol)
				baseY := float64(e.Y + enemySize/2)
				baseX := float64(e.X)
				// Straight down
				s.EnemyBullets = append(s.EnemyBullets, types.EnemyBullet{
					X: baseX, Y: baseY, DX: 0, DY: enemyBulletSpeed,
				})
				// Diagonal left
				s.EnemyBullets = append(s.EnemyBullets, types.EnemyBullet{
					X: baseX, Y: baseY, DX: -enemyBulletDiagSpeed, DY: enemyBulletSpeed,
				})
				// Diagonal right
				s.EnemyBullets = append(s.EnemyBullets, types.EnemyBullet{
					X: baseX, Y: baseY, DX: enemyBulletDiagSpeed, DY: enemyBulletSpeed,
				})
			}
		}
	}
}

// --- Player bullets vs enemies ---

func tickPlayerBullets(s *types.GameState) {
	type hit struct{ bi, ei int }
	var hits []hit

	for bi, b := range s.Bullets {
		for ei, e := range s.Enemies {
			if bulletHitEnemy(b.X, b.Y, e.X, e.Y) {
				pts := pointsPerKill
				if e.Type == "patrol" {
					pts = pointsPerKillPatrol
				}
				if s.Points != nil {
					s.Points[b.OwnerID] += pts
					s.Kills[b.OwnerID]++
				}
				hits = append(hits, hit{bi, ei})
				break
			}
		}
	}

	hitBullets := make(map[int]bool)
	hitEnemies := make(map[int]bool)
	for _, h := range hits {
		hitBullets[h.bi] = true
		hitEnemies[h.ei] = true
	}

	// Update bullets
	var newBullets []types.Bullet
	for bi, b := range s.Bullets {
		if hitBullets[bi] {
			continue
		}
		b.Y -= bulletSpeed
		if b.Y > 0 {
			newBullets = append(newBullets, b)
		}
	}
	s.Bullets = newBullets

	// Update enemies (remove hit + off-screen)
	var newEnemies []types.Enemy
	for ei, e := range s.Enemies {
		if hitEnemies[ei] {
			continue
		}
		if e.Y >= gameHeight {
			// Enemy escaped: damage a random alive player
			damageRandomPlayer(s)
			continue
		}
		newEnemies = append(newEnemies, e)
	}
	s.Enemies = newEnemies
}

// --- Enemy bullets vs players ---

func tickEnemyBullets(s *types.GameState) {
	var remaining []types.EnemyBullet

	for _, eb := range s.EnemyBullets {
		eb.X += eb.DX
		eb.Y += eb.DY

		// Off-screen?
		if eb.Y > float64(gameHeight) || eb.X < 0 || eb.X > float64(gameWidth) {
			continue
		}

		// Check collision with alive, non-invincible players
		hitPlayer := false
		for pi := range s.Players {
			if !s.Players[pi].Alive || s.Players[pi].InvincibleTimer > 0 {
				continue
			}
			px := s.Players[pi].X
			if enemyBulletHitPlayer(eb.X, eb.Y, float64(px), float64(playerY)) {
				hitPlayer = true
				killPlayer(&s.Players[pi])
				break
			}
		}

		if !hitPlayer {
			remaining = append(remaining, eb)
		}
	}

	s.EnemyBullets = remaining
}

func enemyBulletHitPlayer(bx, by, px, py float64) bool {
	pw2 := float64(playerWidth) / 2
	ph2 := float64(playerHeight) / 2
	return bx >= px-pw2 && bx <= px+pw2 && by >= py-ph2 && by <= py+ph2
}

// --- Enemy-player body collision ---

func tickEnemyPlayerCollision(s *types.GameState) {
	var remaining []types.Enemy
	for _, e := range s.Enemies {
		hitPlayer := false
		for pi := range s.Players {
			if !s.Players[pi].Alive || s.Players[pi].InvincibleTimer > 0 {
				continue
			}
			if enemyHitPlayer(e.X, e.Y, s.Players[pi].X, playerY) {
				hitPlayer = true
				killPlayer(&s.Players[pi])
				break
			}
		}
		if !hitPlayer {
			remaining = append(remaining, e)
		}
	}
	s.Enemies = remaining
}

// --- Player damage & respawn ---

// killPlayer handles a player taking a hit: lose a life, mark dead, start respawn timer.
func killPlayer(p *types.Player) {
	p.Lives--
	p.Alive = false
	p.Direction = 0
	if p.Lives > 0 {
		p.RespawnTimer = respawnTicks
	} else {
		p.RespawnTimer = 0 // permanently dead
	}
	p.InvincibleTimer = 0
}

// damageRandomPlayer picks a random alive player and kills them.
func damageRandomPlayer(s *types.GameState) {
	var alive []*types.Player
	for i := range s.Players {
		if s.Players[i].Alive && s.Players[i].InvincibleTimer <= 0 {
			alive = append(alive, &s.Players[i])
		}
	}
	if len(alive) == 0 {
		return
	}
	target := alive[rand.Intn(len(alive))]
	killPlayer(target)
}

// tickPlayerRespawn decrements respawn and invincibility timers, and revives players.
func tickPlayerRespawn(s *types.GameState) {
	for i := range s.Players {
		p := &s.Players[i]

		// Tick invincibility
		if p.InvincibleTimer > 0 {
			p.InvincibleTimer--
		}

		// Tick respawn
		if !p.Alive && p.RespawnTimer > 0 {
			p.RespawnTimer--
			if p.RespawnTimer <= 0 {
				// Respawn!
				p.Alive = true
				p.InvincibleTimer = invincibleTicks
				// Respawn at starting position
				if i == 0 {
					p.X = 200
				} else {
					p.X = 600
				}
				p.Direction = 0
			}
		}
	}
}

// --- Helpers ---

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
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
