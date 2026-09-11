package game

import (
	"testing"

	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/types"
)

func TestEasyIsTheCampaignAsDesigned(t *testing.T) {
	d := difficulties[types.DifficultyEasy]
	if d.FireRate != 1 || d.BulletSpeed != 1 || d.HoldTime != 1 || d.BossHP != 1 || d.DiveSpeed != diveSpeed || d.Lives != initialLives {
		t.Fatalf("easy must not change the campaign: %+v", d)
	}
	for _, name := range []string{types.DifficultyEasy, types.DifficultyMedium, types.DifficultyHard} {
		if _, ok := difficulties[name]; !ok {
			t.Fatalf("no modifiers for %q", name)
		}
	}
}

func TestHarderDifficultiesPressHarder(t *testing.T) {
	e, m, h := difficulties[types.DifficultyEasy], difficulties[types.DifficultyMedium], difficulties[types.DifficultyHard]
	if !(e.FireRate < m.FireRate && m.FireRate < h.FireRate) ||
		!(e.BulletSpeed < m.BulletSpeed && m.BulletSpeed < h.BulletSpeed) ||
		!(e.HoldTime > m.HoldTime && m.HoldTime > h.HoldTime) ||
		!(e.BossHP < m.BossHP && m.BossHP < h.BossHP) ||
		h.Lives > e.Lives || h.DiveSpeed < e.DiveSpeed {
		t.Fatalf("difficulties should ramp: easy %+v, medium %+v, hard %+v", e, m, h)
	}
}

func TestHardScalesTheWave(t *testing.T) {
	installLevel(t, "t-hard.json", `{"waves":[{"name":"w","groups":[
		{"kind":"patrol","count":1,"entry":"appear"},
		{"kind":"static","count":1,"y":150,"entry":"appear","behavior":"hold","release":200}]}]}`)
	hard := difficulties[types.DifficultyHard]

	s := newWaveState("t-hard.json")
	s.Difficulty = types.DifficultyHard
	tickWaveSpawning(s)
	patrol, holder := s.Enemies[0], s.Enemies[1]
	if want := round(patrolShootInterval / hard.FireRate); patrol.ShootEvery != want {
		t.Fatalf("hard patrol shoots every %d ticks, want %d", patrol.ShootEvery, want)
	}
	if want := round(200 * hard.HoldTime); holder.ReleaseTimer != want {
		t.Fatalf("hard formation holds %d ticks, want %d", holder.ReleaseTimer, want)
	}

	// Released enemies dive faster.
	s.Enemies = []types.Enemy{{Type: "static", Y: 100, Diving: true}}
	tickEnemyAI(s)
	if s.Enemies[0].Y != 100+hard.DiveSpeed {
		t.Fatalf("hard dive moved to %d", s.Enemies[0].Y)
	}

	// Enemy bullets fly faster, boss ones included.
	s.EnemyBullets = []types.EnemyBullet{{X: 400, Y: 100, DX: 0, DY: enemyBulletSpeed}}
	tickEnemyBullets(s)
	if got, want := s.EnemyBullets[0].Y, 100+enemyBulletSpeed*hard.BulletSpeed; got != want {
		t.Fatalf("hard bullet at y=%.2f, want %.2f", got, want)
	}

	// Bosses have more health, on top of a level's own override.
	spawnBoss(s, BossSentinel, 20)
	if want := round(20 * hard.BossHP); s.Boss.MaxHP != want {
		t.Fatalf("hard boss hp %d, want %d", s.Boss.MaxHP, want)
	}
}

func TestDifficultySetsStartingLives(t *testing.T) {
	for name, d := range difficulties {
		t.Run(name, func(t *testing.T) {
			matchID, playerID := "t-lives-"+name, "p-lives-"+name
			if !match.JoinMatch(matchID, playerID, "solo", name) {
				t.Fatal("could not create the match")
			}
			t.Cleanup(func() { match.RemoveMatch(matchID) })
			p := AddPlayer(matchID, playerID)
			if p == nil || p.Lives != d.Lives {
				t.Fatalf("player starts with %+v lives, want %d", p, d.Lives)
			}
			if st := GetState(matchID); st.Difficulty != name || !st.Started {
				t.Fatalf("state difficulty %q started %v", st.Difficulty, st.Started)
			}
		})
	}
}
