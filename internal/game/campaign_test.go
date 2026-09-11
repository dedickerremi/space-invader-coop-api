package game

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// campaignBrief is the design brief each campaign is tuned against: target
// enemies per level, and the most that may be on screen at once.
var campaignBrief = map[string]struct {
	perLevel []int
	peak     int
}{
	"solo": {perLevel: []int{60, 75, 85, 100, 110}, peak: 12},
	"coop": {perLevel: []int{90, 115, 130, 150, 170}, peak: 18},
}

const budgetTolerance = 10 // percent either side of a level's target

// maxCarriersPerLevel keeps scripted bonuses occasional.
const maxCarriersPerLevel = 4

func carriersIn(def *LevelDefinition) []CarrierDefinition {
	var all []CarrierDefinition
	for _, w := range def.Waves {
		all = append(all, w.Carriers...)
	}
	return all
}

var campaignBosses = []string{"", BossSentinel, BossWarden, BossCitadel, BossNexus}

func loadCampaign(t *testing.T, mode string) ([]string, []*LevelDefinition) {
	t.Helper()
	names := CampaignLevels(mode)
	var defs []*LevelDefinition
	for _, n := range names {
		if !strings.HasPrefix(n, mode+"-") {
			t.Fatalf("%s campaign fell back to legacy level %s", mode, n)
		}
		data, err := levelFiles.ReadFile("levels/" + n)
		if err != nil {
			t.Fatal(err)
		}
		def, err := ParseLevelJSON(data)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		defs = append(defs, def)
	}
	return names, defs
}

func TestCampaignsFollowTheirBrief(t *testing.T) {
	for mode, brief := range campaignBrief {
		t.Run(mode, func(t *testing.T) {
			names, defs := loadCampaign(t, mode)
			if len(defs) != len(brief.perLevel) {
				t.Fatalf("%s campaign has %d levels %v, want %d", mode, len(defs), names, len(brief.perLevel))
			}
			for i, def := range defs {
				target := brief.perLevel[i]
				lo, hi := target*(100-budgetTolerance)/100, target*(100+budgetTolerance)/100
				if n := def.EnemyCount(); n < lo || n > hi {
					t.Errorf("%s: %d enemies, want %d–%d (target %d)", names[i], n, lo, hi, target)
				}
				if def.Title == "" {
					t.Errorf("%s: missing title", names[i])
				}
				// Bonuses are rare but placed: every level offers its
				// firepower once, on a courier the players have to shoot down.
				carriers := carriersIn(def)
				if len(carriers) > maxCarriersPerLevel {
					t.Errorf("%s: %d carriers, keep it to %d", names[i], len(carriers), maxCarriersPerLevel)
				}
				if !slices.ContainsFunc(carriers, func(c CarrierDefinition) bool {
					return c.Kind == CarrierCourier && c.Drop == "double_shot"
				}) {
					t.Errorf("%s: no courier carrying double_shot", names[i])
				}
				if def.BossKind != campaignBosses[i] {
					t.Errorf("%s: boss %q, want %q", names[i], def.BossKind, campaignBosses[i])
				}
				for _, w := range def.Waves {
					if p := w.PeakEnemies(); p > brief.peak {
						t.Errorf("%s wave %d (%s): up to %d enemies on screen, cap is %d", names[i], w.Number, w.Name, p, brief.peak)
					}
					checkCoexistence(t, names[i], w)
				}
			}

			// Players should lose, learn the patterns and come back, so each
			// sector must press harder than the last — not just last longer.
			prev := 0.0
			for i, def := range defs {
				p := def.Pressure()
				t.Logf("%s: pressure %.2f bullets/s (fireRate %.2f)", names[i], p, def.FireRate)
				if i > 0 && p < prev*(1+minPressureRamp) {
					t.Errorf("%s: pressure %.2f is not %.0f%% above the previous sector's %.2f", names[i], p, minPressureRamp*100, prev)
				}
				prev = p
			}
		})
	}
}

// minPressureRamp is how much harder each sector must be than the one before.
const minPressureRamp = 0.10

// reach is how far a group's members move sideways from their slots.
func reach(g GroupDefinition) int {
	switch {
	case g.Kind == EnemyPatrol:
		return patrolAmplitude
	case g.Hold:
		return holdSwayAmplitude
	}
	return 0
}

// checkCoexistence flags groups that can be on screen together and would
// collide: slots that overlap once sideways movement is accounted for, or a
// descending group that starts above a holding one and would fall through
// it. It is conservative — a descending group is checked at its slot even if
// it has long moved on — which keeps formations clearly apart.
func checkCoexistence(t *testing.T, level string, w WaveDefinition) {
	t.Helper()
	gone := make([]bool, len(w.Groups))
	for k, gk := range w.Groups {
		if k > 0 && gk.Trigger == TriggerCleared {
			gone[k-1] = true
		}
		for j := 0; j < k; j++ {
			if gone[j] {
				continue
			}
			gj := w.Groups[j]
			width := enemySize + reach(gj) + reach(gk)
			if problem := collide(gj, gk, width); problem != "" {
				t.Errorf("%s wave %d (%s): groups %d and %d %s", level, w.Number, w.Name, j+1, k+1, problem)
			}
		}
	}
}

func collide(a, b GroupDefinition, width int) string {
	for _, p := range a.Slots {
		for _, q := range b.Slots {
			if abs(p.X-q.X) >= width {
				continue
			}
			if abs(p.Y-q.Y) < enemySize {
				return fmt.Sprintf("overlap at (%d,%d) / (%d,%d)", p.X, p.Y, q.X, q.Y)
			}
			if a.Hold != b.Hold {
				hold, fall := p, q
				if !a.Hold {
					hold, fall = q, p
				}
				if fall.Y < hold.Y {
					return fmt.Sprintf("descends from (%d,%d) through the formation at (%d,%d)", fall.X, fall.Y, hold.X, hold.Y)
				}
			}
		}
	}
	return ""
}

// TestCampaignsPlayThrough plays every campaign start to finish with a bot
// that shoots each enemy a second after it settles, and checks nothing
// stalls: every wave ends, every boss appears, and the last level is a win.
func TestCampaignsPlayThrough(t *testing.T) {
	const reaction = 30           // ticks an enemy survives once settled
	const bossFight = 20 * 30     // ticks the bot takes to down a boss
	const maxTicks = 60 * 60 * 30 // an hour of game time: anything longer is a stall

	for mode := range campaignBrief {
		t.Run(mode, func(t *testing.T) {
			names := CampaignLevels(mode)
			scripted := 0
			for _, n := range names {
				scripted += len(carriersIn(GetLevelByName(n)))
			}
			s := newWaveState(names[0])
			var bosses []string
			levelStart, bossTicks := 0, 0
			level := s.LevelName

			tick := 0
			for ; tick < maxTicks && !s.Victory; tick++ {
				tickWaveSpawning(s)
				tickEnemyAI(s)

				kept := s.Enemies[:0]
				for _, e := range s.Enemies {
					settled := e.EntryTick >= e.EntryDur
					dead := settled && (e.Hold && e.HoldTick >= reaction || !e.Hold && e.Y >= e.SlotY+reaction)
					if !dead {
						kept = append(kept, e)
					}
				}
				s.Enemies = kept

				if s.Boss != nil {
					if bossTicks == 0 {
						bosses = append(bosses, s.Boss.Kind)
					}
					if bossTicks++; bossTicks >= bossFight {
						onBossKilled(s, "p1")
						bossTicks = 0
					}
				}

				if s.LevelName != level || s.Victory {
					t.Logf("%s cleared in %s", level, time.Duration(tick-levelStart)*time.Second/30)
					level, levelStart = s.LevelName, tick
				}
			}
			if !s.Victory {
				t.Fatalf("%s campaign stalled at %s wave %d after %d ticks", mode, s.LevelName, s.WaveNumber, tick)
			}
			// Carriers are never ticked here, so every launched one is still listed.
			if len(s.Carriers) != scripted {
				t.Fatalf("%d of %d scripted carriers launched", len(s.Carriers), scripted)
			}
			want := campaignBosses[1:]
			if strings.Join(bosses, ",") != strings.Join(want, ",") {
				t.Fatalf("bosses fought %v, want %v", bosses, want)
			}
		})
	}
}
