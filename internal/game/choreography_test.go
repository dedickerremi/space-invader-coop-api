package game

import (
	"testing"

	"space-invaders-coop/backend-go/internal/types"
)

// installLevel parses js and makes it loadable as name for the test.
func installLevel(t *testing.T, name, js string) *LevelDefinition {
	t.Helper()
	def, err := ParseLevelJSON([]byte(js))
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	levelsMu.Lock()
	levelCache[name] = def
	levelsMu.Unlock()
	t.Cleanup(func() {
		levelsMu.Lock()
		delete(levelCache, name)
		levelsMu.Unlock()
	})
	return def
}

func newWaveState(level string) *types.GameState {
	return &types.GameState{
		LevelName:   level,
		WaveNumber:  1,
		Started:     true,
		Players:     []types.Player{{ID: "p1", Lives: initialLives, Alive: true, X: 400, Y: playerY}},
		Points:      map[string]int{"p1": 0},
		Kills:       map[string]int{"p1": 0},
		KillStreaks: map[string]int{"p1": 0},
		Deaths:      map[string]int{"p1": 0},
		BestStreaks: map[string]int{"p1": 0},
	}
}

// step runs one tick of wave spawning and enemy movement.
func step(s *types.GameState) {
	tickWaveSpawning(s)
	tickEnemyAI(s)
}

func TestStaggerMakesACascade(t *testing.T) {
	installLevel(t, "t-stagger.json", groupLevel(`{"kind":"static","count":3,"stagger":5,"entry":"appear"}`))
	s := newWaveState("t-stagger.json")
	counts := []int{}
	for tick := 0; tick <= 10; tick++ {
		tickWaveSpawning(s)
		counts = append(counts, len(s.Enemies))
	}
	want := []int{1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 3}
	for i := range want {
		if counts[i] != want[i] {
			t.Fatalf("enemies per tick = %v, want %v", counts, want)
		}
	}
	// Default order is formation order: left to right.
	if s.Enemies[0].X >= s.Enemies[1].X || s.Enemies[1].X >= s.Enemies[2].X {
		t.Fatalf("cascade should run left to right: %+v", s.Enemies)
	}
}

func TestChainedGroups(t *testing.T) {
	installLevel(t, "t-chain.json", `{"waves":[{"name":"w","groups":[
		{"kind":"static","count":2,"entry":"appear"},
		{"kind":"static","count":1,"y":200,"entry":"appear","after":"spawned","delay":10},
		{"kind":"patrol","count":1,"y":150,"entry":"appear","after":"cleared","delay":20}
	]}]}`)
	s := newWaveState("t-chain.json")

	for i := 0; i < 10; i++ {
		tickWaveSpawning(s)
	}
	if len(s.Enemies) != 2 {
		t.Fatalf("group 2 must wait 10 ticks after group 1 spawned, have %d enemies", len(s.Enemies))
	}
	tickWaveSpawning(s) // tick 10
	if len(s.Enemies) != 3 || s.Enemies[2].Group != 2 {
		t.Fatalf("group 2 should spawn on tick 10: %+v", s.Enemies)
	}

	for i := 0; i < 100; i++ {
		tickWaveSpawning(s)
	}
	if len(s.Enemies) != 3 {
		t.Fatal("group 3 must not start while group 2 is alive")
	}

	// Kill group 2; group 3 follows 20 ticks later, even with group 1 alive.
	s.Enemies = s.Enemies[:2]
	for i := 0; i < 20; i++ {
		tickWaveSpawning(s)
	}
	if len(s.Enemies) != 2 {
		t.Fatal("group 3 spawned before its delay")
	}
	tickWaveSpawning(s)
	if len(s.Enemies) != 3 || s.Enemies[2].Type != "patrol" {
		t.Fatalf("group 3 should spawn 20 ticks after group 2 cleared: %+v", s.Enemies)
	}
}

func TestWaveEndsWhenEveryGroupIsDone(t *testing.T) {
	installLevel(t, "t-end.json", `{"waves":[
		{"name":"a","groups":[
			{"kind":"static","count":1,"entry":"appear"},
			{"kind":"static","count":1,"entry":"appear","after":"cleared"}]},
		{"name":"b","groups":[{"kind":"static","count":1}]}
	]}`)
	s := newWaveState("t-end.json")
	tickWaveSpawning(s)
	s.Enemies = nil // kill group 1
	for i := 0; i < 5; i++ {
		tickWaveSpawning(s)
	}
	if s.WaveCooldown != 0 {
		t.Fatal("wave ended while a chained group had not spawned yet")
	}
	if len(s.Enemies) != 1 {
		t.Fatalf("group 2 should have spawned, have %d", len(s.Enemies))
	}
	s.Enemies = nil
	tickWaveSpawning(s)
	if s.WaveCooldown != waveCooldownTicks {
		t.Fatalf("wave should end once every group is spawned and gone, cooldown=%d", s.WaveCooldown)
	}
	for s.WaveNumber == 1 {
		tickWaveSpawning(s)
	}
	if s.WaveName != "b" || len(s.WaveGroups) != 1 {
		t.Fatalf("second wave should start with fresh group progress: %q %+v", s.WaveName, s.WaveGroups)
	}
}

func TestEntryPathsLandOnTheSlot(t *testing.T) {
	for _, entry := range []string{"top", "left", "right"} {
		t.Run(entry, func(t *testing.T) {
			name := "t-entry-" + entry + ".json"
			installLevel(t, name, groupLevel(`{"kind":"patrol","count":1,"x":600,"y":100,"behavior":"hold","entry":"`+entry+`"}`))
			s := newWaveState(name)
			tickWaveSpawning(s)
			e := &s.Enemies[0]
			if e.X == e.SlotX && e.Y == e.SlotY {
				t.Fatal("enemy should start off its slot")
			}
			maxY := e.Y
			for e.EntryTick < e.EntryDur {
				tickEnemyAI(s)
				maxY = max(maxY, e.Y)
				if len(s.EnemyBullets) > 0 {
					t.Fatal("an entering enemy fired")
				}
			}
			if e.X != 600 || e.Y != 100 {
				t.Fatalf("entry ended at (%d,%d), want the slot (600,100)", e.X, e.Y)
			}
			if maxY >= playerYMin {
				t.Fatalf("entry path dipped to y=%d, into the players' zone", maxY)
			}
		})
	}
}

func TestHoldSwayReleaseAndDive(t *testing.T) {
	installLevel(t, "t-hold.json", groupLevel(`{"kind":"static","count":1,"y":100,"entry":"appear","behavior":"hold","release":200}`))
	s := newWaveState("t-hold.json")
	tickWaveSpawning(s)
	e := &s.Enemies[0]

	for i := 0; i < 199; i++ {
		step(s)
		if e.Y != 100 {
			t.Fatalf("holding enemy moved vertically to %d", e.Y)
		}
		if d := e.X - e.SlotX; d < -holdSwayAmplitude || d > holdSwayAmplitude {
			t.Fatalf("sway %d px exceeds the amplitude", d)
		}
	}
	if len(s.EnemyBullets) == 0 {
		t.Fatal("a holding static should have fired during 200 ticks")
	}
	step(s)
	if e.Hold || !e.Diving {
		t.Fatalf("enemy should be released after 200 holding ticks: %+v", *e)
	}
	y := e.Y
	step(s)
	if e.Y-y != diveSpeed {
		t.Fatalf("released enemy should dive at %d px/tick, moved %d", diveSpeed, e.Y-y)
	}
}

func TestHoldWithoutReleaseStaysUntilKilled(t *testing.T) {
	installLevel(t, "t-forever.json", groupLevel(`{"kind":"patrol","count":1,"y":120,"entry":"appear","behavior":"hold"}`))
	s := newWaveState("t-forever.json")
	for i := 0; i < 2000; i++ {
		step(s)
	}
	e := s.Enemies[0]
	if !e.Hold || e.Y != 120 || e.X < e.SlotX-patrolAmplitude-patrolHorizontalSpeed || e.X > e.SlotX+patrolAmplitude+patrolHorizontalSpeed {
		t.Fatalf("patrol holder should zigzag on its row forever: %+v", e)
	}
}

func TestLegacyEnemiesStillDescend(t *testing.T) {
	installLevel(t, "t-legacy.json", `{"waves":[{"name":"w","rows":["S"]}]}`)
	s := newWaveState("t-legacy.json")
	tickWaveSpawning(s)
	if e := s.Enemies[0]; e.X != 400 || e.Y != defaultSpawnY {
		t.Fatalf("legacy enemy should appear at (400,%d), got (%d,%d)", defaultSpawnY, e.X, e.Y)
	}
	tickEnemyAI(s)
	if s.Enemies[0].Y != defaultSpawnY+staticEnemySpeed {
		t.Fatal("legacy static should drift down from its first tick")
	}
}

func TestBossHPOverride(t *testing.T) {
	installLevel(t, "t-boss.json", `{"waves":[{"name":"w","groups":[{"kind":"static","count":1}]}],"boss":{"kind":"sentinel","hp":7}}`)
	s := newWaveState("t-boss.json")
	tickWaveSpawning(s)
	s.Enemies = nil
	for i := 0; i < waveCooldownTicks+2 && s.Boss == nil; i++ {
		tickWaveSpawning(s)
	}
	if s.Boss == nil || s.Boss.MaxHP != 7 || s.Boss.HP != 7 {
		t.Fatalf("boss should spawn with the level's hp 7: %+v", s.Boss)
	}
}

func TestFireRateScalesShooting(t *testing.T) {
	installLevel(t, "t-fire.json", `{"fireRate":2,"waves":[{"name":"w","groups":[
		{"kind":"patrol","count":1,"entry":"appear"},
		{"kind":"static","count":1,"y":150,"entry":"appear","behavior":"hold"}]}]}`)
	s := newWaveState("t-fire.json")
	tickWaveSpawning(s)
	if p, h := s.Enemies[0].ShootEvery, s.Enemies[1].ShootEvery; p != patrolShootInterval/2 || h != holdShootInterval/2 {
		t.Fatalf("fireRate 2 should halve shot intervals: patrol %d, holder %d", p, h)
	}
	if _, err := ParseLevelJSON([]byte(`{"fireRate":5,"waves":[{"name":"w","groups":[{"kind":"static","count":1}]}]}`)); err == nil {
		t.Fatal("an out-of-range fireRate should be rejected")
	}
}
