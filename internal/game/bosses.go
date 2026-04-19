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
