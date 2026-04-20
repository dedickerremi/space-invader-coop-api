package game

import (
	"math"
	"math/rand"

	"space-invaders-coop/backend-go/internal/types"
)

// --- Boss registry ---

const (
	BossSentinel = "sentinel"
	BossWarden   = "warden"
	BossCitadel  = "citadel"
	BossNexus    = "nexus"
)

// bossArenaYMin / bossArenaYMax bound the boss to the upper portion of the
// canvas so it can drift vertically but never reach the player.
const (
	bossArenaXMin = 80
	bossArenaXMax = gameWidth - 80
	bossArenaYMin = 50
	bossArenaYMax = 320
)

// bossStats holds the per-kind tunable stats. Adding a new boss kind = add
// a case here + a tick handler below.
type bossStats struct {
	maxHP         int
	pointsOnKill  int
	hitboxW       int
	hitboxH       int
	powerUpDrops  int // guaranteed power-ups spawned when killed
}

func statsFor(kind string) bossStats {
	switch kind {
	case BossSentinel:
		return bossStats{maxHP: 30, pointsOnKill: 2000, hitboxW: 80, hitboxH: 60, powerUpDrops: 2}
	case BossWarden:
		return bossStats{maxHP: 50, pointsOnKill: 3000, hitboxW: 100, hitboxH: 70, powerUpDrops: 3}
	case BossCitadel:
		return bossStats{maxHP: 80, pointsOnKill: 4000, hitboxW: 110, hitboxH: 80, powerUpDrops: 3}
	case BossNexus:
		return bossStats{maxHP: 120, pointsOnKill: 5000, hitboxW: 130, hitboxH: 90, powerUpDrops: 4}
	}
	return bossStats{maxHP: 30, pointsOnKill: 1000, hitboxW: 80, hitboxH: 60, powerUpDrops: 1}
}

// --- Spawn & naming ---

// spawnBoss creates a Boss of the given kind and writes it into the game
// state. Caller is responsible for clearing/pausing the regular wave loop.
func spawnBoss(s *types.GameState, kind string) {
	stats := statsFor(kind)
	s.Boss = &types.Boss{
		Kind:        kind,
		X:           gameWidth / 2,
		Y:           120,
		HP:          stats.maxHP,
		MaxHP:       stats.maxHP,
		Phase:       1,
		PatternDir:  1,
		AttackState: 0,
		AttackTimer: 60, // 2s grace before first attack burst
	}
	if kind == BossWarden {
		s.Boss.SummonTimer = wardenFirstSummonDelay
	}
	s.WaveName = bossDisplayName(kind)
}

// bossDisplayName is the human-readable name shown in the HUD + wave banner.
func bossDisplayName(kind string) string {
	switch kind {
	case BossSentinel:
		return "Sentinel"
	case BossWarden:
		return "Warden"
	case BossCitadel:
		return "Citadel"
	case BossNexus:
		return "Nexus"
	}
	return "Boss"
}

// --- Per-tick dispatcher ---

func tickBoss(s *types.GameState) {
	if s.Boss == nil {
		return
	}
	s.Boss.PatternTick++
	switch s.Boss.Kind {
	case BossSentinel:
		tickSentinel(s)
	case BossWarden:
		tickWarden(s)
	case BossCitadel:
		tickCitadel(s)
	}
}

// --- Sentinel (level 2) ---
//
// Behavior:
//   - Slow horizontal zigzag between bossArenaXMin..bossArenaXMax.
//   - Vertical bob around Y ≈ 120 (amplitude 25, period ~4s).
//   - Attack cycle: after a 2s rest, fires 5 aimed bullets spaced 12 ticks
//     apart (≈2s total), then rests 3s, repeats.

const (
	sentinelSpeedX        = 1.5
	sentinelBobAmplitude  = 25.0
	sentinelBobPeriodTick = 120
	sentinelBobCenterY    = 120.0

	sentinelBurstShots     = 5
	sentinelBurstInterval  = 12 // ticks between shots in a burst
	sentinelRestTicks      = 90 // 3s rest between bursts
	sentinelBulletSpeed    = 4.5
)

func tickSentinel(s *types.GameState) {
	b := s.Boss

	// --- Movement ---
	b.X += float64(b.PatternDir) * sentinelSpeedX
	if b.X >= bossArenaXMax {
		b.X = bossArenaXMax
		b.PatternDir = -1
	} else if b.X <= bossArenaXMin {
		b.X = bossArenaXMin
		b.PatternDir = 1
	}
	b.Y = sentinelBobCenterY + sentinelBobAmplitude*math.Sin(
		2*math.Pi*float64(b.PatternTick)/float64(sentinelBobPeriodTick),
	)

	// --- Attack cycle ---
	b.AttackTimer--
	if b.AttackTimer > 0 {
		return
	}
	switch b.AttackState {
	case 0: // rest ended, start firing a burst
		b.AttackState = 1
		b.AttackShotsLeft = sentinelBurstShots
		fallthrough
	case 1: // fire one shot
		fireAimedBullet(s, b.X, b.Y, sentinelBulletSpeed)
		b.AttackShotsLeft--
		if b.AttackShotsLeft <= 0 {
			b.AttackState = 0
			b.AttackTimer = sentinelRestTicks
		} else {
			b.AttackTimer = sentinelBurstInterval
		}
	}
}

// --- Warden (level 3) ---
//
// Behavior:
//   - Wider, slower horizontal sweep than Sentinel; shallow vertical bob.
//   - Periodically summons up to 4 patrol escorts from its side ports.
//     Each summon drops 2 escorts (one per side), cooldown 5s. New summons
//     are skipped if 4+ patrol enemies are already alive on the canvas.
//   - Attack cycle: 3 aimed shots spaced 14 ticks, then rests 2.5s.

const (
	wardenSpeedX           = 1.2
	wardenBobAmplitude     = 15.0
	wardenBobPeriodTick    = 150
	wardenBobCenterY       = 130.0
	wardenBulletSpeed      = 4.5
	wardenBurstShots       = 3
	wardenBurstInterval    = 14
	wardenRestTicks        = 75 // 2.5s between bursts

	wardenFirstSummonDelay = 90  // 3s before first escort wave
	wardenSummonInterval   = 150 // 5s between summon attempts
	wardenMaxEscorts       = 4
	wardenEscortOffsetX    = 60
	wardenEscortOffsetY    = 40
)

func tickWarden(s *types.GameState) {
	b := s.Boss

	// --- Movement (wider sweep, shallower bob) ---
	b.X += float64(b.PatternDir) * wardenSpeedX
	if b.X >= bossArenaXMax {
		b.X = bossArenaXMax
		b.PatternDir = -1
	} else if b.X <= bossArenaXMin {
		b.X = bossArenaXMin
		b.PatternDir = 1
	}
	b.Y = wardenBobCenterY + wardenBobAmplitude*math.Sin(
		2*math.Pi*float64(b.PatternTick)/float64(wardenBobPeriodTick),
	)

	// --- Escort summons (independent of the aimed-shot cycle) ---
	if b.SummonTimer > 0 {
		b.SummonTimer--
	}
	if b.SummonTimer <= 0 {
		if countPatrolEnemies(s) < wardenMaxEscorts {
			spawnWardenEscorts(s, b)
		}
		b.SummonTimer = wardenSummonInterval
	}

	// --- Attack cycle (aimed shots) ---
	b.AttackTimer--
	if b.AttackTimer > 0 {
		return
	}
	switch b.AttackState {
	case 0:
		b.AttackState = 1
		b.AttackShotsLeft = wardenBurstShots
		fallthrough
	case 1:
		fireAimedBullet(s, b.X, b.Y, wardenBulletSpeed)
		b.AttackShotsLeft--
		if b.AttackShotsLeft <= 0 {
			b.AttackState = 0
			b.AttackTimer = wardenRestTicks
		} else {
			b.AttackTimer = wardenBurstInterval
		}
	}
}

// spawnWardenEscorts drops two patrol enemies — one from each side port of
// the boss. They act like normal patrol enemies from that point on.
func spawnWardenEscorts(s *types.GameState, b *types.Boss) {
	portY := int(b.Y) + wardenEscortOffsetY
	for _, side := range []int{-1, 1} {
		x := int(b.X) + side*wardenEscortOffsetX
		s.Enemies = append(s.Enemies, types.Enemy{
			X:          x,
			Y:          portY,
			Type:       string(EnemyPatrol),
			SpawnX:     x,
			PatternDir: side,
			ShootTimer: randomShootDelay(EnemyPatrol),
		})
	}
}

// countPatrolEnemies returns how many patrol-type enemies are alive. During
// a boss fight no regular-wave enemies spawn, so this count = escort count.
func countPatrolEnemies(s *types.GameState) int {
	n := 0
	for i := range s.Enemies {
		if s.Enemies[i].Type == string(EnemyPatrol) {
			n++
		}
	}
	return n
}

// --- Citadel (level 4) ---
//
// Behavior:
//   - Slow horizontal drift in a tight central arena with a gentle bob.
//   - Hard shield cycle: 10s invulnerable, 5s open. When shielded, player
//     bullets bounce off with sparks (handled in tickBossCollision).
//   - Fires a 3-comet fan aimed at the nearest player every 45 ticks in
//     both phases — vulnerable window is short, so the player has to dodge
//     AND dps at the same time.

const (
	citadelSpeedX         = 0.8
	citadelBobAmplitude   = 10.0
	citadelBobPeriodTick  = 180
	citadelBobCenterY     = 140.0
	citadelArenaXMin      = 150
	citadelArenaXMax      = gameWidth - 150

	citadelShieldedTicks  = 300 // 10s shielded
	citadelVulnerableTicks = 150 // 5s open
	citadelCycleTicks     = citadelShieldedTicks + citadelVulnerableTicks

	citadelCometSpeed     = 5.5
	citadelSpreadCount    = 3
	citadelSpreadDeg      = 18.0 // total fan half-angle in degrees
	citadelShotInterval   = 45
)

func tickCitadel(s *types.GameState) {
	b := s.Boss

	// --- Movement ---
	b.X += float64(b.PatternDir) * citadelSpeedX
	if b.X >= citadelArenaXMax {
		b.X = citadelArenaXMax
		b.PatternDir = -1
	} else if b.X <= citadelArenaXMin {
		b.X = citadelArenaXMin
		b.PatternDir = 1
	}
	b.Y = citadelBobCenterY + citadelBobAmplitude*math.Sin(
		2*math.Pi*float64(b.PatternTick)/float64(citadelBobPeriodTick),
	)

	// --- Shield cycle (derived from PatternTick — no extra state needed) ---
	cyclePos := b.PatternTick % citadelCycleTicks
	b.ShieldActive = cyclePos < citadelShieldedTicks

	// --- Attack: spread of comets, same cadence in both phases ---
	b.AttackTimer--
	if b.AttackTimer > 0 {
		return
	}
	fireCometSpread(s, b.X, b.Y, citadelCometSpeed, citadelSpreadCount, citadelSpreadDeg)
	b.AttackTimer = citadelShotInterval
}

// fireCometSpread emits `count` comet bullets in a symmetric fan around the
// aim vector toward the nearest alive player. `spreadDeg` is the half-angle
// (degrees) — bullets are evenly distributed between -spreadDeg and
// +spreadDeg. Comets use kind="comet" for frontend trail rendering.
func fireCometSpread(s *types.GameState, bx, by, speed float64, count int, spreadDeg float64) {
	target := nearestAlivePlayer(s, bx, by)
	if target == nil {
		return
	}
	dx := float64(target.X) - bx
	dy := float64(target.Y) - by
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		dist = 1
	}
	baseAngle := math.Atan2(dy, dx)
	spreadRad := spreadDeg * math.Pi / 180.0

	for i := 0; i < count; i++ {
		var offset float64
		if count == 1 {
			offset = 0
		} else {
			// Distribute evenly across [-spreadRad, +spreadRad].
			offset = -spreadRad + 2*spreadRad*float64(i)/float64(count-1)
		}
		a := baseAngle + offset
		s.EnemyBullets = append(s.EnemyBullets, types.EnemyBullet{
			X:    bx,
			Y:    by,
			DX:   math.Cos(a) * speed,
			DY:   math.Sin(a) * speed,
			Kind: "comet",
		})
	}
}

// fireAimedBullet shoots a bullet from (bx, by) toward the nearest alive
// player. Uses kind="aimed" so the frontend can tint it.
func fireAimedBullet(s *types.GameState, bx, by, speed float64) {
	target := nearestAlivePlayer(s, bx, by)
	if target == nil {
		return
	}
	dx := float64(target.X) - bx
	dy := float64(target.Y) - by
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		dist = 1
	}
	s.EnemyBullets = append(s.EnemyBullets, types.EnemyBullet{
		X:    bx,
		Y:    by,
		DX:   dx / dist * speed,
		DY:   dy / dist * speed,
		Kind: "aimed",
	})
}

func nearestAlivePlayer(s *types.GameState, x, y float64) *types.Player {
	var best *types.Player
	bestDist := math.Inf(1)
	for i := range s.Players {
		p := &s.Players[i]
		if !p.Alive {
			continue
		}
		d := math.Hypot(float64(p.X)-x, float64(p.Y)-y)
		if d < bestDist {
			bestDist = d
			best = p
		}
	}
	return best
}

// --- Player bullet collision ---

// tickBossCollision resolves player bullets vs the active boss. Consumes
// each bullet that overlaps the boss hitbox, subtracts HP (or bounces off
// the shield with a visible spark when ShieldActive), and handles death.
func tickBossCollision(s *types.GameState) {
	if s.Boss == nil || len(s.Bullets) == 0 {
		return
	}
	stats := statsFor(s.Boss.Kind)
	halfW := float64(stats.hitboxW) / 2
	halfH := float64(stats.hitboxH) / 2

	keep := s.Bullets[:0]
	for _, b := range s.Bullets {
		if s.Boss == nil {
			keep = append(keep, b)
			continue
		}
		bx, by := float64(b.X), float64(b.Y)
		if math.Abs(bx-s.Boss.X) > halfW || math.Abs(by-s.Boss.Y) > halfH {
			keep = append(keep, b)
			continue
		}
		// Hit.
		if s.Boss.ShieldActive {
			// Shield bounce: consume the bullet, drop a spark, no HP loss.
			s.Sparks = append(s.Sparks, types.Spark{
				X: bx, Y: by, TTL: sparkLifeTicks, Life: sparkLifeTicks, Kind: "bullet",
			})
			continue
		}
		s.Boss.HP--
		if s.Boss.HP <= 0 {
			onBossKilled(s, b.OwnerID)
		}
	}
	s.Bullets = keep
}

// onBossKilled awards team points, spawns guaranteed power-ups, clears the
// boss + its residual bullets, and kicks off the post-boss cooldown.
func onBossKilled(s *types.GameState, killerID string) {
	if s.Boss == nil {
		return
	}
	stats := statsFor(s.Boss.Kind)
	bx, by := int(s.Boss.X), int(s.Boss.Y)

	// Points: full bounty to the killer, and half split across other alive
	// players so coop feels like a team effort.
	if s.Points != nil {
		s.Points[killerID] += stats.pointsOnKill
		teamBonus := stats.pointsOnKill / 2
		others := 0
		for _, p := range s.Players {
			if p.ID != killerID {
				others++
			}
		}
		if others > 0 {
			per := teamBonus / others
			for _, p := range s.Players {
				if p.ID != killerID {
					s.Points[p.ID] += per
				}
			}
		}
	}

	// Guaranteed drops — always include an extra_life, then fill with
	// impactful boosts.
	pool := []string{"extra_life", "shield", "double_shot", "speed_boost"}
	for i := 0; i < stats.powerUpDrops; i++ {
		var kind string
		if i == 0 {
			kind = "extra_life"
		} else {
			kind = pool[rand.Intn(len(pool))]
		}
		s.PowerUps = append(s.PowerUps, types.PowerUp{
			X: bx + (i-stats.powerUpDrops/2)*40, Y: by, Kind: kind,
		})
	}

	s.Boss = nil
	s.EnemyBullets = s.EnemyBullets[:0]
	s.BossDefeated = true
	s.WaveCleared = true
	s.WaveCooldown = waveCooldownTicks
	s.NextWaveCountdown = waveCooldownTicks
}
