package game

import (
	"slices"
	"strings"
	"testing"

	"space-invaders-coop/backend-go/internal/types"
)

func TestEnemyDropsAreRareAndSmall(t *testing.T) {
	s := newWaveState("")
	s.KillStreaks["p1"] = 5 // a streak milestone no longer guarantees anything
	const kills = 20000
	for i := 0; i < kills; i++ {
		maybeDropPowerUp(s, types.Enemy{X: 100, Y: 100, Type: "patrol"}, "p1")
	}
	pct := float64(len(s.PowerUps)) * 100 / kills
	if pct < 4 || pct > 6 {
		t.Fatalf("%.2f%% of kills dropped a bonus, want about %d%%", pct, powerUpDropPct)
	}
	for _, pu := range s.PowerUps {
		if !slices.Contains(smallBonuses, pu.Kind) {
			t.Fatalf("an enemy dropped %q; firepower and lives only come from carriers and bosses", pu.Kind)
		}
	}
}

func TestBonusesLastUntilALifeIsLost(t *testing.T) {
	s := newWaveState("")
	p := &s.Players[0]
	applyPowerUp(s, p, "double_shot")
	applyPowerUp(s, p, "speed_boost")
	if !p.DoubleShot || !p.SpeedBoost {
		t.Fatal("bonuses should be active after pickup")
	}

	applyPowerUp(s, p, "double_shot")
	if s.Points["p1"] != pointsBonusValue {
		t.Fatalf("a second double shot should pay %d points, got %d", pointsBonusValue, s.Points["p1"])
	}

	hurtPlayer(s, p)
	if p.Lives != initialLives-1 || p.DoubleShot || p.SpeedBoost {
		t.Fatalf("losing a life should cost the bonuses: %+v", *p)
	}
}

func TestShieldAbsorbsThreeHits(t *testing.T) {
	s := newWaveState("")
	p := &s.Players[0]
	applyPowerUp(s, p, "shield")
	for hit := 1; hit <= shieldMaxCharges; hit++ {
		p.InvincibleTimer = 0
		hurtPlayer(s, p)
		if p.Lives != initialLives || p.ShieldCharges != shieldMaxCharges-hit {
			t.Fatalf("hit %d: lives %d charges %d", hit, p.Lives, p.ShieldCharges)
		}
		if p.InvincibleTimer < shieldHitGraceTicks {
			t.Fatal("an absorbed hit should grant a short grace")
		}
	}
	p.InvincibleTimer = 0
	hurtPlayer(s, p)
	if p.Lives != initialLives-1 {
		t.Fatal("the fourth hit should cost a life")
	}

	applyPowerUp(s, p, "shield")
	p.ShieldCharges = 1
	applyPowerUp(s, p, "shield")
	if p.ShieldCharges != shieldMaxCharges {
		t.Fatal("a new shield should top the old one back up")
	}
}

func TestEscapedEnemyHitsTheShieldFirst(t *testing.T) {
	s := newWaveState("")
	p := &s.Players[0]
	applyPowerUp(s, p, "shield")
	damageRandomPlayer(s)
	if p.Lives != initialLives || p.ShieldCharges != shieldMaxCharges-1 {
		t.Fatalf("an escaped enemy should cost a shield charge, not a life: %+v", *p)
	}
}

func TestBossDropsALifeAndAShield(t *testing.T) {
	s := newWaveState("")
	spawnBoss(s, BossSentinel, 0)
	onBossKilled(s, "p1")
	var kinds []string
	for _, pu := range s.PowerUps {
		kinds = append(kinds, pu.Kind)
	}
	if strings.Join(kinds, ",") != "extra_life,shield" {
		t.Fatalf("boss dropped %v", kinds)
	}
}

func TestInvalidCarriersAreRejected(t *testing.T) {
	wave := func(carrier string) string {
		return `{"waves":[{"name":"w","groups":[{"kind":"static","count":1}],"carriers":[` + carrier + `]}]}`
	}
	cases := map[string]struct{ json, want string }{
		"unknown kind":      {wave(`{"kind":"comet"}`), "unknown carrier"},
		"missing group":     {wave(`{"kind":"courier","group":2}`), "does not exist"},
		"courier from top":  {wave(`{"kind":"courier","from":"top"}`), "left or right"},
		"courier too low":   {wave(`{"kind":"courier","y":400}`), "formation band"},
		"asteroid altitude": {wave(`{"kind":"asteroid","y":100}`), "only applies to a courier"},
		"unknown drop":      {wave(`{"kind":"asteroid","drop":"laser"}`), "unknown drop"},
		"typo in carrier":   {wave(`{"kind":"asteroid","dealy":10}`), "unknown field"},
		"negative delay":    {wave(`{"kind":"asteroid","delay":-1}`), "negative"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseLevelJSON([]byte(c.json))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error mentioning %q", err, c.want)
			}
		})
	}
	def, err := ParseLevelJSON([]byte(wave(`{"kind":"courier"}`)))
	if err != nil {
		t.Fatal(err)
	}
	if c := def.Waves[0].Carriers[0]; c.Group != 1 || c.Drop != "double_shot" {
		t.Fatalf("courier defaults: %+v", c)
	}
}

func TestCarrierLaunchesWithItsGroup(t *testing.T) {
	installLevel(t, "t-carrier.json", `{"waves":[{"name":"w","groups":[
		{"kind":"static","count":1,"entry":"appear"},
		{"kind":"static","count":1,"y":150,"entry":"appear","after":"cleared"}
	],"carriers":[{"kind":"courier","group":2,"delay":10,"from":"left","y":100}]}]}`)
	s := newWaveState("t-carrier.json")
	for i := 0; i < 50; i++ {
		tickWaveSpawning(s)
	}
	if len(s.Carriers) != 0 {
		t.Fatal("the courier must wait for group 2")
	}
	s.Enemies = nil // clear group 1: group 2 starts on the next tick
	for i := 0; i < 11; i++ {
		tickWaveSpawning(s)
	}
	if len(s.Carriers) != 1 {
		t.Fatal("the courier should launch 10 ticks after group 2 starts")
	}
	for i := 0; i < 100; i++ {
		tickWaveSpawning(s)
	}
	if len(s.Carriers) != 1 {
		t.Fatal("a carrier launches once per wave")
	}
}

func TestCourierCrossesAndLeaves(t *testing.T) {
	s := newWaveState("")
	s.Carriers = []types.Carrier{newCarrier(&CarrierDefinition{Kind: CarrierCourier, From: "left", Y: 100, Drop: "double_shot"})}
	lastX := s.Carriers[0].X
	for tick := 0; len(s.Carriers) > 0; tick++ {
		if tick > 400 {
			t.Fatal("courier never left the screen")
		}
		tickCarriers(s)
		if len(s.Carriers) == 0 {
			break
		}
		k := s.Carriers[0]
		if k.X <= lastX || k.Y < 100-courierBob || k.Y > 100+courierBob {
			t.Fatalf("courier should fly right at its altitude, got (%d,%d)", k.X, k.Y)
		}
		lastX = k.X
	}
}

func TestAsteroidsComeFromRandomEdgesAndLeave(t *testing.T) {
	s := newWaveState("")
	edges := map[string]bool{}
	for i := 0; i < 300; i++ {
		k := newCarrier(&CarrierDefinition{Kind: CarrierAsteroid})
		switch {
		case k.X < 0:
			edges["left"] = true
		case k.X > gameWidth:
			edges["right"] = true
		case k.Y < 0:
			edges["top"] = true
		default:
			t.Fatalf("asteroid spawned on screen at (%d,%d)", k.X, k.Y)
		}
		if !slices.Contains(smallBonuses, k.Drop) {
			t.Fatalf("an unscripted asteroid should carry a small bonus, got %q", k.Drop)
		}
		s.Carriers = append(s.Carriers, k)
	}
	if len(edges) != 3 {
		t.Fatalf("asteroids should enter from every edge, saw %v", edges)
	}
	for i := 0; i < 1000 && len(s.Carriers) > 0; i++ {
		tickCarriers(s)
	}
	if len(s.Carriers) != 0 {
		t.Fatalf("%d asteroids never left the screen", len(s.Carriers))
	}
}

func TestShootingACarrierReleasesItsBonus(t *testing.T) {
	s := newWaveState("")
	s.Carriers = []types.Carrier{{Kind: "courier", X: 300, Y: 100, HP: courierHP, Drop: "double_shot"}}
	s.Bullets = []types.Bullet{{X: 300, Y: 105, OwnerID: "p1"}}
	tickCarrierHits(s)
	if len(s.Bullets) != 0 || len(s.Carriers) != 1 || s.Carriers[0].HP != 1 || len(s.PowerUps) != 0 {
		t.Fatalf("first hit should cost 1 hp and the bullet: carriers %+v bullets %d", s.Carriers, len(s.Bullets))
	}
	s.Bullets = []types.Bullet{{X: 310, Y: 100, OwnerID: "p1"}, {X: 700, Y: 100, OwnerID: "p1"}}
	tickCarrierHits(s)
	if len(s.Carriers) != 0 || len(s.PowerUps) != 1 || s.PowerUps[0].Kind != "double_shot" {
		t.Fatalf("second hit should bring it down and drop its bonus: %+v", s.PowerUps)
	}
	if s.Points["p1"] != pointsPerCarrier || len(s.Bullets) != 1 {
		t.Fatalf("points %d, bullets left %d", s.Points["p1"], len(s.Bullets))
	}
}

func TestCarriersDontHoldUpTheWave(t *testing.T) {
	installLevel(t, "t-carrier-end.json", `{"waves":[{"name":"w","groups":[{"kind":"static","count":1,"entry":"appear"}],
		"carriers":[{"kind":"asteroid"}]}]}`)
	s := newWaveState("t-carrier-end.json")
	tickWaveSpawning(s)
	s.Enemies = nil
	tickWaveSpawning(s)
	if len(s.Carriers) != 1 || s.WaveCooldown == 0 {
		t.Fatalf("the wave should end with the asteroid still flying: carriers %d cooldown %d", len(s.Carriers), s.WaveCooldown)
	}
}
