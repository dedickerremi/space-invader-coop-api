package game

import "space-invaders-coop/backend-go/internal/types"

// Difficulty scales how hard a solo campaign presses without changing what
// happens: the waves, formations and their timing stay identical, so a
// pattern learned on Easy still holds on Hard — it just comes faster and
// hits harder. The same scaling applies to the solo and coop campaigns.
type Difficulty struct {
	FireRate    float64 // multiplies the level's fire rate
	BulletSpeed float64 // multiplies enemy (and boss) bullet speed
	HoldTime    float64 // multiplies how long formations hold before diving
	DiveSpeed   int     // px/tick once released from formation
	BossHP      float64 // multiplies boss health
	Lives       int     // lives each player starts with
}

var difficulties = map[string]Difficulty{
	// The campaign as designed.
	types.DifficultyEasy: {FireRate: 1, BulletSpeed: 1, HoldTime: 1, DiveSpeed: diveSpeed, BossHP: 1, Lives: initialLives},
	// Same lives, noticeably more fire, formations break sooner.
	types.DifficultyMedium: {FireRate: 1.35, BulletSpeed: 1.15, HoldTime: 0.75, DiveSpeed: diveSpeed, BossHP: 1.4, Lives: initialLives},
	// One life less, and everything faster.
	types.DifficultyHard: {FireRate: 1.75, BulletSpeed: 1.3, HoldTime: 0.5, DiveSpeed: diveSpeed + 1, BossHP: 1.8, Lives: initialLives - 1},
}

// difficultyOf returns the modifiers for the match; anything unknown (a
// match created before difficulties existed) plays Easy.
func difficultyOf(s *types.GameState) Difficulty {
	if d, ok := difficulties[s.Difficulty]; ok {
		return d
	}
	return difficulties[types.DifficultyEasy]
}
