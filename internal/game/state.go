package game

import (
	"fmt"
	"math"
	"math/rand"

	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/types"
)

// Exported constants for the /api/game-meta endpoint.
// Sizes used purely for client rendering (no backend collision impact) are
// still defined here so the backend remains the single source of truth.
const (
	PlayerSpeed  = 5
	BulletSpeed  = 8
	GameWidth    = 800
	GameHeight   = 600
	PlayerY      = 550
	PlayerWidth  = 40
	PlayerHeight = 20
	PlayerXMin   = 20
	PlayerXMax   = GameWidth - 20
	PlayerYMin   = 350 // furthest the player can move forward (toward enemies)
	PlayerYMax   = GameHeight - 20
	EnemySize    = 28
	PatrolSize   = EnemySize // backend uses one hitbox for static and patrol
	InitialLives = 3

	// Bullets are point-collision on the backend; these sizes are for client rendering.
	BulletWidth      = 6
	BulletHeight     = 14
	EnemyBulletWidth = 6
	EnemyBulletHeight = 10

	// Power-up visual size (used for pickup hitbox AABB).
	PowerUpSize = 20
)

const (
	playerSpeed  = PlayerSpeed
	bulletSpeed  = BulletSpeed
	gameWidth    = GameWidth
	gameHeight   = GameHeight
	playerY      = PlayerY
	playerXMin   = PlayerXMin
	playerXMax   = PlayerXMax
	playerYMin   = PlayerYMin
	playerYMax   = PlayerYMax
	playerWidth  = PlayerWidth
	playerHeight = PlayerHeight
	enemySize    = EnemySize
	initialLives = InitialLives
	pointsPerKill         = 100
	pointsPerKillPatrol   = 250
	pointsPerBulletShot   = 50
	sparkLifeTicks        = 10 // ~333ms at 30 Hz

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
	invincibleTicks = 90           // 3s invincibility after instant respawn

	// Power-ups. Bonuses are rare and last: double shot and speed stay until
	// the player loses a life, the shield until it has absorbed its hits.
	// Most arrive on scripted carriers; enemies only drop the odd small one.
	pointsBonusValue    = 500
	powerUpFallSpeed    = 2  // px per tick
	powerUpDropPct      = 5  // % chance an enemy drops a small bonus when killed
	shieldMaxCharges    = 3  // hits a fresh shield absorbs
	shieldHitGraceTicks = 15 // brief invulnerability after the shield absorbs a hit, so one volley can't drain it
)

// powerUpKinds lists every bonus a level may script onto a carrier.
var powerUpKinds = []string{"extra_life", "double_shot", "speed_boost", "shield", "points_bonus"}

// smallBonuses is what enemies drop at random and what an asteroid carries
// when the level doesn't say. Firepower and lives are never random.
var smallBonuses = []string{"points_bonus", "points_bonus", "speed_boost", "shield"}

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
	sparks := make([]types.Spark, len(s.Sparks))
	copy(sparks, s.Sparks)
	powerUps := make([]types.PowerUp, len(s.PowerUps))
	copy(powerUps, s.PowerUps)
	carriers := make([]types.Carrier, len(s.Carriers))
	copy(carriers, s.Carriers)
	points := make(map[string]int)
	for k, v := range s.Points {
		points[k] = v
	}
	kills := make(map[string]int)
	for k, v := range s.Kills {
		kills[k] = v
	}
	killStreaks := make(map[string]int)
	for k, v := range s.KillStreaks {
		killStreaks[k] = v
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
	var boss *types.Boss
	if s.Boss != nil {
		b := *s.Boss
		boss = &b
	}
	return &types.GameState{
		Players:           players,
		Bullets:           bullets,
		EnemyBullets:      enemyBullets,
		Enemies:           enemies,
		Sparks:            sparks,
		PowerUps:          powerUps,
		Carriers:          carriers,
		KillStreaks:       killStreaks,
		Lives:             s.Lives,
		Points:            points,
		Kills:             kills,
		LevelName:         s.LevelName,
		LevelTitle:        s.LevelTitle,
		Difficulty:        s.Difficulty,
		WaveNumber:        s.WaveNumber,
		WaveName:          s.WaveName,
		TotalWaves:        s.TotalWaves,
		Started:           s.Started,
		Paused:            s.Paused,
		PausedBy:          pausedBy,
		GameOver:          s.GameOver,
		Victory:           s.Victory,
		GameOverSummary:   summary,
		NextWaveCountdown: s.NextWaveCountdown,
		Boss:              boss,
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
	if m.Mode == "solo" {
		x = 400 // centered for solo
	} else if len(m.State.Players) > 0 {
		x = 600
	}
	p := types.Player{
		ID:         playerID,
		X:          x,
		Y:          playerY,
		Alive:      true,
		Direction:  0,
		DirectionY: 0,
		Lives:      difficultyOf(&m.State).Lives,
		SpawnX:     x,
		SpawnY:     playerY,
	}
	m.State.Players = append(m.State.Players, p)

	requiredPlayers := 2
	if m.Mode == "solo" {
		requiredPlayers = 1
	}
	if len(m.State.Players) == requiredPlayers {
		m.State.Started = true
		// Compute total lives for backward compat
		m.State.Lives = 0
		for _, pl := range m.State.Players {
			m.State.Lives += pl.Lives
		}
		m.State.Points = make(map[string]int)
		m.State.Kills = make(map[string]int)
		m.State.KillStreaks = make(map[string]int)
		m.State.Deaths = make(map[string]int)
		m.State.BestStreaks = make(map[string]int)
		for _, pl := range m.State.Players {
			m.State.Points[pl.ID] = 0
			m.State.Kills[pl.ID] = 0
			m.State.KillStreaks[pl.ID] = 0
			m.State.Deaths[pl.ID] = 0
			m.State.BestStreaks[pl.ID] = 0
		}
		// Start wave 1 of the first level
		m.State.LevelName = FirstLevel(m.Mode)
		m.State.WaveNumber = 1
		m.State.WaveTick = 0
		m.State.WaveCleared = false
		m.State.WaveCooldown = 0
		m.State.NextWaveCountdown = 0
		if level := GetLevelByName(m.State.LevelName); level != nil && len(level.Waves) > 0 {
			m.State.LevelTitle = level.Title
			m.State.WaveName = level.Waves[0].Name
			m.State.TotalWaves = len(level.Waves)
		}
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
	requiredPlayers := 2
	if m.Mode == "solo" {
		requiredPlayers = 1
	}
	if len(m.State.Players) < requiredPlayers {
		m.State.Started = false
	}
}

// SetPlayerAuth attaches Clerk identity info to a player. Called once
// after AddPlayer if the WebSocket handshake authenticated successfully.
// Guests (userID == "") leave the fields empty.
func SetPlayerAuth(matchID, playerID, userID, displayName string) {
	m := match.GetMatch(matchID)
	if m == nil {
		return
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	for i := range m.State.Players {
		if m.State.Players[i].ID == playerID {
			m.State.Players[i].UserID = userID
			m.State.Players[i].DisplayName = displayName
			return
		}
	}
}

// SetPlayerDirection sets the movement direction for a player.
// dirX and dirY are optional — pass nil to leave that axis unchanged.
func SetPlayerDirection(matchID, playerID string, dirX, dirY *int) {
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
			if dirX != nil {
				m.State.Players[i].Direction = *dirX
			}
			if dirY != nil {
				m.State.Players[i].DirectionY = *dirY
			}
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
	var px, py int
	doubleShot := false
	for i := range m.State.Players {
		if m.State.Players[i].ID == playerID && m.State.Players[i].Alive {
			px = m.State.Players[i].X
			py = m.State.Players[i].Y
			doubleShot = m.State.Players[i].DoubleShot
			break
		}
	}
	if doubleShot {
		offset := playerWidth / 3
		m.State.Bullets = append(m.State.Bullets,
			types.Bullet{X: px - offset, Y: py - 10, OwnerID: playerID},
			types.Bullet{X: px + offset, Y: py - 10, OwnerID: playerID},
		)
	} else {
		m.State.Bullets = append(m.State.Bullets, types.Bullet{
			X: px, Y: py - 10, OwnerID: playerID,
		})
	}
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

	// --- Wave management ---
	tickWaveSpawning(s)

	// --- Player invincibility window ---
	tickInvincibility(s)

	// --- Player movement ---
	for i := range s.Players {
		if !s.Players[i].Alive {
			continue
		}
		speed := playerSpeed
		if s.Players[i].SpeedBoost {
			speed = playerSpeed * 3 / 2 // 1.5x
		}
		s.Players[i].X += s.Players[i].Direction * speed
		s.Players[i].X = clamp(s.Players[i].X, playerXMin, playerXMax)
		s.Players[i].Y += s.Players[i].DirectionY * speed
		s.Players[i].Y = clamp(s.Players[i].Y, playerYMin, playerYMax)
	}

	// --- Enemy AI (movement + shooting) ---
	tickEnemyAI(s)

	// --- Bonus carriers drift across the screen ---
	tickCarriers(s)

	// --- Boss AI (movement + attacks) ---
	tickBoss(s)

	// --- Decay short-lived visual sparks ---
	tickSparks(s)

	// --- Player bullets vs enemy bullets (mutual destruction) ---
	tickBulletsVsBullets(s)

	// --- Player bullets vs boss (before the enemy sweep so the bullet is
	//     consumed by the boss if it hits, rather than falling through) ---
	tickBossCollision(s)

	// --- Player bullets vs bonus carriers ---
	tickCarrierHits(s)

	// --- Move player bullets + check collisions ---
	tickPlayerBullets(s)

	// --- Move enemy bullets + check collisions ---
	tickEnemyBullets(s)

	// --- Enemy-player collision ---
	tickEnemyPlayerCollision(s)

	// --- Power-ups: fall + pickup ---
	tickPowerUps(s)

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
		s.GameOverSummary = buildGameOverSummary(s)
	}
}

// buildGameOverSummary snapshots each player's points + kills for the
// end-of-match screen. Used for both defeat (all players permadead) and
// victory (last wave of last level cleared).
func buildGameOverSummary(s *types.GameState) *types.GameOverSummary {
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
	return &types.GameOverSummary{PlayerScores: scores}
}

// --- Wave spawning ---

func tickWaveSpawning(s *types.GameState) {
	level := GetLevelByName(s.LevelName)
	if level == nil || len(level.Waves) == 0 {
		return
	}

	// Boss fight in progress: no wave spawning or progression. The boss
	// handler owns the canvas until onBossKilled sets BossDefeated + the
	// post-boss cooldown.
	if s.Boss != nil {
		return
	}

	// Between-wave cooldown. When it expires, decide what comes next:
	// next wave, boss fight, next level, or victory.
	if s.WaveCooldown > 0 {
		s.WaveCooldown--
		s.NextWaveCountdown = s.WaveCooldown
		if s.WaveCooldown > 0 {
			return
		}

		onLastWave := s.WaveNumber >= len(level.Waves)

		// End of regular waves and the level has a boss we haven't beaten
		// yet — launch the boss fight instead of advancing.
		if onLastWave && level.BossKind != "" && !s.BossDefeated {
			spawnBoss(s, level.BossKind, level.BossHP)
			return
		}

		if !onLastWave {
			s.WaveNumber++
			s.WaveName = level.Waves[s.WaveNumber-1].Name
			s.WaveTick = 0
			s.WaveCleared = false
		} else {
			nextName := NextLevel(s.LevelName)
			nextLevel := GetLevelByName(nextName)
			if nextName == "" || nextLevel == nil || len(nextLevel.Waves) == 0 {
				s.Victory = true
				s.GameOver = true
				s.GameOverSummary = buildGameOverSummary(s)
				return
			}
			s.LevelName = nextName
			s.LevelTitle = nextLevel.Title
			s.WaveNumber = 1
			s.WaveName = nextLevel.Waves[0].Name
			s.TotalWaves = len(nextLevel.Waves)
			s.WaveTick = 0
			s.WaveCleared = false
			s.BossDefeated = false
			level = nextLevel
		}
	}

	waveIdx := s.WaveNumber - 1
	if waveIdx < 0 || waveIdx >= len(level.Waves) {
		return
	}
	wave := &level.Waves[waveIdx]

	if s.WaveTick == 0 || len(s.WaveGroups) != len(wave.Groups) || len(s.WaveCarriers) != len(wave.Carriers) {
		s.WaveGroups = make([]types.GroupProgress, len(wave.Groups))
		s.WaveCarriers = make([]bool, len(wave.Carriers))
	}

	diff := difficultyOf(s)

	// Groups start on a fixed tick or chain on the previous group. Each
	// member then spawns Stagger ticks after the one before it, which is
	// what turns a formation into a cascade.
	alive := groupsAlive(s.Enemies, len(wave.Groups))
	allSpawned := true
	for gi := range wave.Groups {
		g := &wave.Groups[gi]
		p := &s.WaveGroups[gi]

		if !p.Started {
			if !groupReady(s, wave, gi) {
				allSpawned = false
				continue
			}
			p.Started = true
			p.StartTick = s.WaveTick
		}

		for p.Spawned < len(g.Slots) && s.WaveTick >= p.StartTick+p.Spawned*g.Stagger {
			s.Enemies = append(s.Enemies, newGroupEnemy(g, gi+1, p.Spawned, level.FireRate*diff.FireRate, diff.HoldTime))
			p.Spawned++
			alive[gi]++
			if p.Spawned == len(g.Slots) {
				p.SpawnedAt = s.WaveTick
			}
		}

		if p.Spawned < len(g.Slots) {
			allSpawned = false
		} else if !p.Cleared && alive[gi] == 0 {
			p.Cleared = true
			p.ClearedAt = s.WaveTick
		}
	}

	launchCarriers(s, wave)

	s.WaveTick++

	if allSpawned && len(s.Enemies) == 0 {
		// Wave cleared — start cooldown for next wave
		s.WaveCleared = true
		s.WaveCooldown = waveCooldownTicks
		s.NextWaveCountdown = waveCooldownTicks
	}
}

// groupReady reports whether group gi may start on the current wave tick.
// Validation guarantees a chained group is never the first one.
func groupReady(s *types.GameState, wave *WaveDefinition, gi int) bool {
	g := &wave.Groups[gi]
	switch g.Trigger {
	case TriggerSpawned:
		prev := s.WaveGroups[gi-1]
		return prev.Spawned == len(wave.Groups[gi-1].Slots) && s.WaveTick >= prev.SpawnedAt+g.Delay
	case TriggerCleared:
		prev := s.WaveGroups[gi-1]
		return prev.Cleared && s.WaveTick >= prev.ClearedAt+g.Delay
	default:
		return s.WaveTick >= g.At
	}
}

// groupsAlive counts the wave's enemies still on screen, per group.
func groupsAlive(enemies []types.Enemy, groups int) []int {
	alive := make([]int, groups)
	for _, e := range enemies {
		if e.Group > 0 && e.Group <= groups {
			alive[e.Group-1]++
		}
	}
	return alive
}

// Entry paths, in ticks and px.
const (
	entryTopTicks  = 36  // ~1.2 s drop from above the screen
	entrySideTicks = 60  // ~2 s swoop from a screen edge
	entrySideStart = 60  // height a side swoop enters the screen at
	entrySideDip   = 170 // how far below its slot a side swoop dips
	entryDipMaxY   = 320 // keeps swoops above the players' zone
)

// newGroupEnemy creates member `member` of group g, placed at the start of
// its entry path. fireRate scales how often it shoots, holdTime how long it
// holds formation before diving.
func newGroupEnemy(g *GroupDefinition, group, member int, fireRate, holdTime float64) types.Enemy {
	slot := g.Slots[member]
	e := types.Enemy{
		X:            slot.X,
		Y:            slot.Y,
		Type:         string(g.Kind),
		SpawnX:       slot.X,
		PatternDir:   1,
		Group:        group,
		SlotX:        slot.X,
		SlotY:        slot.Y,
		Hold:         g.Hold,
		ReleaseTimer: g.Release,
	}
	if g.Release > 0 && holdTime > 0 {
		e.ReleaseTimer = max(1, round(float64(g.Release)*holdTime))
	}
	base := staticShootInterval
	switch {
	case g.Kind == EnemyPatrol:
		base = patrolShootInterval
	case g.Hold:
		base = holdShootInterval
	}
	if fireRate <= 0 {
		fireRate = 1
	}
	e.ShootEvery = max(1, round(float64(base)/fireRate))
	e.ShootTimer = jitter(e.ShootEvery)

	switch g.Entry {
	case EntryTop:
		// Control point halfway along the drop: a straight line, eased in.
		e.EntryFromX, e.EntryFromY = slot.X, -enemySize
		e.EntryCtrlX, e.EntryCtrlY = slot.X, (slot.Y-enemySize)/2
		e.EntryDur = entryTopTicks
	case EntryLeft, EntryRight:
		// Enter high at the edge, dip below the slot, rise back onto it.
		e.EntryFromY = entrySideStart
		e.EntryCtrlY = min(slot.Y+entrySideDip, entryDipMaxY)
		if g.Entry == EntryLeft {
			e.EntryFromX = -enemySize
			e.EntryCtrlX = slot.X * 6 / 10
		} else {
			e.EntryFromX = gameWidth + enemySize
			e.EntryCtrlX = gameWidth - (gameWidth-slot.X)*6/10
		}
		e.EntryDur = entrySideTicks
	}
	if e.EntryDur > 0 {
		e.X, e.Y = e.EntryFromX, e.EntryFromY
	}
	return e
}

func randomShootDelay(kind EnemyKind) int {
	base := staticShootInterval
	if kind == EnemyPatrol {
		base = patrolShootInterval
	}
	return jitter(base)
}

// jitter randomizes a shot interval by ±30% so enemies don't all fire in
// sync. Only the timing of shots varies between runs; the choreography does
// not, so a wave can be learned.
func jitter(base int) int {
	variance := base * 30 / 100
	if variance == 0 {
		return base
	}
	return base - variance + rand.Intn(2*variance)
}

// nextShot returns the ticks until e fires again. Enemies from the wave
// spawner carry their level-scaled interval; boss escorts fall back on the
// default for their role.
func nextShot(e *types.Enemy, fallback int) int {
	if e.ShootEvery > 0 {
		return jitter(e.ShootEvery)
	}
	return jitter(fallback)
}

// --- Enemy AI ---

// Formation holding and diving.
const (
	holdShootInterval = 150 // ~5 s between shots for a static holding its slot
	holdSwayAmplitude = 20  // px a holding formation sways either side of its slots
	holdSwayPeriod    = 120 // ticks per full sway (~4 s)
	holdSwayRampTicks = 30  // ease the sway in so arriving on a slot never jumps
	diveSpeed         = 2   // px/tick once released from formation
)

func tickEnemyAI(s *types.GameState) {
	dive := difficultyOf(s).DiveSpeed
	for i := range s.Enemies {
		e := &s.Enemies[i]

		if e.EntryTick < e.EntryDur {
			advanceEntry(e)
			continue // nobody shoots before reaching their slot
		}

		if e.Hold {
			tickHolding(s, e)
			continue
		}

		switch e.Type {
		case "static":
			// Static drifts down silently — only patrol enemies and holding
			// formations shoot, so the bullet volume stays readable.
			if e.Diving {
				e.Y += dive
			} else {
				e.Y += staticEnemySpeed
			}

		case "patrol":
			vy := patrolEnemySpeed
			if e.Diving {
				vy = dive
			}
			patrolStep(s, e, vy)
		}
	}
}

// advanceEntry moves an enemy one tick along its entry path: a quadratic
// Bézier from EntryFrom through EntryCtrl onto its slot, eased out so it
// settles into formation instead of stopping dead.
func advanceEntry(e *types.Enemy) {
	e.EntryTick++
	t := float64(e.EntryTick) / float64(e.EntryDur)
	t = 1 - (1-t)*(1-t)
	u := 1 - t
	e.X = round(u*u*float64(e.EntryFromX) + 2*u*t*float64(e.EntryCtrlX) + t*t*float64(e.SlotX))
	e.Y = round(u*u*float64(e.EntryFromY) + 2*u*t*float64(e.EntryCtrlY) + t*t*float64(e.SlotY))
}

// tickHolding keeps an enemy on its formation slot until it is released.
func tickHolding(s *types.GameState, e *types.Enemy) {
	e.HoldTick++

	switch e.Type {
	case "static":
		// The formation sways as one: the phase comes from the wave clock,
		// so every member moves in step regardless of when it arrived.
		ramp := math.Min(1, float64(e.HoldTick)/holdSwayRampTicks)
		phase := 2 * math.Pi * float64(s.WaveTick) / holdSwayPeriod
		e.X = e.SlotX + round(ramp*holdSwayAmplitude*math.Sin(phase))
		e.Y = e.SlotY

		// A holding static never reaches the bottom, so it has to be a
		// threat some other way: a single slow straight shot.
		e.ShootTimer--
		if e.ShootTimer <= 0 {
			e.ShootTimer = nextShot(e, holdShootInterval)
			s.EnemyBullets = append(s.EnemyBullets, types.EnemyBullet{
				X: float64(e.X), Y: float64(e.Y + enemySize/2), DX: 0, DY: enemyBulletSpeed,
			})
		}

	case "patrol":
		patrolStep(s, e, 0) // zigzag around the slot without drifting down
	}

	if e.ReleaseTimer > 0 {
		e.ReleaseTimer--
		if e.ReleaseTimer == 0 {
			e.Hold = false
			e.Diving = true
			e.SpawnX = e.X
		}
	}
}

// patrolStep zigzags a patrol around SpawnX, moves it down by vy and fires
// its three-way shot when due.
func patrolStep(s *types.GameState, e *types.Enemy, vy int) {
	// Patrol: zigzag horizontally + drift down
	e.X += e.PatternDir * patrolHorizontalSpeed
	e.Y += vy

	// Reverse at amplitude bounds
	if e.X > e.SpawnX+patrolAmplitude || e.X >= gameWidth-20 {
		e.PatternDir = -1
	} else if e.X < e.SpawnX-patrolAmplitude || e.X <= 20 {
		e.PatternDir = 1
	}

	// Shoot 3 bullets: straight + 2 diagonals
	e.ShootTimer--
	if e.ShootTimer <= 0 {
		e.ShootTimer = nextShot(e, patrolShootInterval)
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

// --- Sparks (transient visual effects) ---

func tickSparks(s *types.GameState) {
	if len(s.Sparks) == 0 {
		return
	}
	keep := s.Sparks[:0]
	for _, sp := range s.Sparks {
		sp.TTL--
		if sp.TTL > 0 {
			keep = append(keep, sp)
		}
	}
	s.Sparks = keep
}

// --- Player bullets vs enemy bullets ---

func tickBulletsVsBullets(s *types.GameState) {
	if len(s.Bullets) == 0 || len(s.EnemyBullets) == 0 {
		return
	}
	pbHit := make(map[int]bool)
	ebHit := make(map[int]bool)
	for pbi, pb := range s.Bullets {
		for ebi, eb := range s.EnemyBullets {
			if ebHit[ebi] {
				continue
			}
			if bulletHitBullet(pb.X, pb.Y, eb.X, eb.Y) {
				pbHit[pbi] = true
				ebHit[ebi] = true
				if s.Points != nil {
					s.Points[pb.OwnerID] += pointsPerBulletShot
				}
				s.Sparks = append(s.Sparks, types.Spark{
					X:    (float64(pb.X) + eb.X) / 2,
					Y:    (float64(pb.Y) + eb.Y) / 2,
					TTL:  sparkLifeTicks,
					Life: sparkLifeTicks,
					Kind: "bullet",
				})
				break
			}
		}
	}
	if len(pbHit) > 0 {
		var keep []types.Bullet
		for i, b := range s.Bullets {
			if !pbHit[i] {
				keep = append(keep, b)
			}
		}
		s.Bullets = keep
	}
	if len(ebHit) > 0 {
		var keep []types.EnemyBullet
		for i, b := range s.EnemyBullets {
			if !ebHit[i] {
				keep = append(keep, b)
			}
		}
		s.EnemyBullets = keep
	}
}

// bulletHitBullet performs AABB overlap between a player bullet and an enemy bullet
// using the visual sizes from the exported constants.
func bulletHitBullet(pbX, pbY int, ebX, ebY float64) bool {
	pbw2 := float64(BulletWidth) / 2
	pbh2 := float64(BulletHeight) / 2
	ebw2 := float64(EnemyBulletWidth) / 2
	ebh2 := float64(EnemyBulletHeight) / 2
	px, py := float64(pbX), float64(pbY)
	return !(px+pbw2 < ebX-ebw2 || px-pbw2 > ebX+ebw2 ||
		py+pbh2 < ebY-ebh2 || py-pbh2 > ebY+ebh2)
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
				if s.KillStreaks != nil {
					s.KillStreaks[b.OwnerID]++
					if s.BestStreaks != nil && s.KillStreaks[b.OwnerID] > s.BestStreaks[b.OwnerID] {
						s.BestStreaks[b.OwnerID] = s.KillStreaks[b.OwnerID]
					}
				}
				maybeDropPowerUp(s, e, b.OwnerID)
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
	speed := difficultyOf(s).BulletSpeed

	for _, eb := range s.EnemyBullets {
		eb.X += eb.DX * speed
		eb.Y += eb.DY * speed

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
			if enemyBulletHitPlayer(eb.X, eb.Y, float64(px), float64(s.Players[pi].Y)) {
				hitPlayer = true
				hurtPlayer(s, &s.Players[pi])
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
			if enemyHitPlayer(e.X, e.Y, s.Players[pi].X, s.Players[pi].Y) {
				hitPlayer = true
				hurtPlayer(s, &s.Players[pi])
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

// hurtPlayer applies one hit to p. A shield absorbs it and loses a charge,
// with a short grace so a single volley can't strip every charge at once;
// without one, the player loses a life.
func hurtPlayer(s *types.GameState, p *types.Player) {
	if p.ShieldCharges > 0 {
		p.ShieldCharges--
		p.InvincibleTimer = max(p.InvincibleTimer, shieldHitGraceTicks)
		return
	}
	killPlayer(s, p)
}

// killPlayer handles a player losing a life: clear power-ups + streak. If the player has lives left, they respawn instantly at their
// starting position with a temporary invincibility window.
func killPlayer(s *types.GameState, p *types.Player) {
	p.Lives--
	p.Direction = 0
	p.DirectionY = 0
	p.DoubleShot = false
	p.SpeedBoost = false
	p.ShieldCharges = 0
	if s.KillStreaks != nil {
		s.KillStreaks[p.ID] = 0
	}
	if s.Deaths != nil {
		s.Deaths[p.ID]++
	}
	if p.Lives > 0 {
		p.Alive = true
		p.X = p.SpawnX
		p.Y = p.SpawnY
		p.InvincibleTimer = invincibleTicks
	} else {
		p.Alive = false
		p.InvincibleTimer = 0
	}
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
	hurtPlayer(s, target)
}

// tickInvincibility decrements each player's invincibility timer. Respawn
// is now instant (handled in killPlayer) so this is the only remaining
// lifecycle work between ticks.
func tickInvincibility(s *types.GameState) {
	for i := range s.Players {
		if s.Players[i].InvincibleTimer > 0 {
			s.Players[i].InvincibleTimer--
		}
	}
}

// --- Power-ups ---

// maybeDropPowerUp gives a killed enemy a small chance to drop a small
// bonus. Kill streaks no longer guarantee drops: bonuses are meant to be
// rare, and the strong ones are placed in the levels on carriers.
func maybeDropPowerUp(s *types.GameState, e types.Enemy, ownerID string) {
	if rand.Intn(100) >= powerUpDropPct {
		return
	}
	spawnPowerUp(s, e.X, e.Y, randomSmallBonus())
}

func randomSmallBonus() string {
	return smallBonuses[rand.Intn(len(smallBonuses))]
}

// spawnPowerUp adds a falling power-up of the given kind.
func spawnPowerUp(s *types.GameState, x, y int, kind string) {
	s.PowerUps = append(s.PowerUps, types.PowerUp{X: x, Y: y, Kind: kind})
}

// tickPowerUps moves power-ups downward and handles pickup by alive players.
func tickPowerUps(s *types.GameState) {
	if len(s.PowerUps) == 0 {
		return
	}
	var remaining []types.PowerUp
	for _, pu := range s.PowerUps {
		pu.Y += powerUpFallSpeed
		if pu.Y > gameHeight {
			continue
		}
		picked := false
		for pi := range s.Players {
			if !s.Players[pi].Alive {
				continue
			}
			if powerUpHitsPlayer(pu.X, pu.Y, s.Players[pi].X, s.Players[pi].Y) {
				applyPowerUp(s, &s.Players[pi], pu.Kind)
				picked = true
				break
			}
		}
		if !picked {
			remaining = append(remaining, pu)
		}
	}
	s.PowerUps = remaining
}

func powerUpHitsPlayer(pux, puy, px, py int) bool {
	half := PowerUpSize / 2
	pw2 := playerWidth / 2
	ph2 := playerHeight / 2
	return !(pux+half < px-pw2 || pux-half > px+pw2 || puy+half < py-ph2 || puy-half > py+ph2)
}

// applyPowerUp grants the effect of `kind` to player `p`.
func applyPowerUp(s *types.GameState, p *types.Player, kind string) {
	switch kind {
	case "extra_life":
		p.Lives++
	case "double_shot":
		// Already firing double: the pickup still pays out.
		if p.DoubleShot && s.Points != nil {
			s.Points[p.ID] += pointsBonusValue
		}
		p.DoubleShot = true
	case "speed_boost":
		if p.SpeedBoost && s.Points != nil {
			s.Points[p.ID] += pointsBonusValue
		}
		p.SpeedBoost = true
	case "shield":
		p.ShieldCharges = shieldMaxCharges // a new shield tops the old one back up
	case "points_bonus":
		if s.Points != nil {
			s.Points[p.ID] += pointsBonusValue
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
