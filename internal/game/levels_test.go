package game

import (
	"strings"
	"testing"
)

// groupLevel wraps a single group into a one-wave level.
func groupLevel(group string) string {
	return `{"waves":[{"name":"w","groups":[` + group + `]}]}`
}

func parseGroup(t *testing.T, group string) GroupDefinition {
	t.Helper()
	def, err := ParseLevelJSON([]byte(groupLevel(group)))
	if err != nil {
		t.Fatalf("parse %s: %v", group, err)
	}
	return def.Waves[0].Groups[0]
}

func slotsEqual(t *testing.T, got []Slot, want ...Slot) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d slots %v, want %v", len(got), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slot %d = %v, want %v (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestLegacyRowsKeepTheirBehaviour(t *testing.T) {
	data, err := levelFiles.ReadFile("levels/level1.json")
	if err != nil {
		t.Fatal(err)
	}
	def, err := ParseLevelJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := def.EnemyCount(); got != 189 {
		t.Fatalf("level1.json has %d enemies, want the original 189", got)
	}
	// Wave 1, row 0 is "..S...S...S..": 13 columns over 40–760 px.
	g := def.Waves[0].Groups[0]
	if g.Entry != EntryAppear || g.Trigger != TriggerAt || g.At != 0 || g.Stagger != 0 {
		t.Fatalf("legacy row should appear in place at once, got %+v", g)
	}
	slotsEqual(t, g.Slots, Slot{160, 20}, Slot{400, 20}, Slot{640, 20})
	// Rows are rowDelay (30) ticks apart; wave 3 row 2 is the third row.
	for _, g := range def.Waves[2].Groups {
		if g.Kind == EnemyPatrol && g.At != 2*30 {
			t.Fatalf("patrol row should start at tick 60, got %d", g.At)
		}
	}
}

func TestFormations(t *testing.T) {
	t.Run("line is centred on x", func(t *testing.T) {
		g := parseGroup(t, `{"kind":"static","count":3,"x":400,"y":80}`)
		slotsEqual(t, g.Slots, Slot{344, 80}, Slot{400, 80}, Slot{456, 80})
	})
	t.Run("column stacks downward", func(t *testing.T) {
		g := parseGroup(t, `{"kind":"static","count":3,"formation":"column","x":100,"y":60}`)
		slotsEqual(t, g.Slots, Slot{100, 60}, Slot{100, 104}, Slot{100, 148})
	})
	t.Run("grid centres a short last row", func(t *testing.T) {
		g := parseGroup(t, `{"kind":"static","count":5,"formation":"grid","cols":3,"x":400,"y":60}`)
		slotsEqual(t, g.Slots,
			Slot{344, 60}, Slot{400, 60}, Slot{456, 60},
			Slot{372, 104}, Slot{428, 104})
	})
	t.Run("v leads at the front", func(t *testing.T) {
		g := parseGroup(t, `{"kind":"static","count":5,"formation":"v","x":400,"y":80}`)
		slotsEqual(t, g.Slots,
			Slot{400, 120}, Slot{344, 100}, Slot{456, 100}, Slot{288, 80}, Slot{512, 80})
	})
	t.Run("arc bulges toward the players in the middle", func(t *testing.T) {
		g := parseGroup(t, `{"kind":"static","count":3,"formation":"arc","x":400,"y":80}`)
		slotsEqual(t, g.Slots, Slot{344, 80}, Slot{400, 120}, Slot{456, 80})
	})
	t.Run("diagonal descends to the right, mirror flips it", func(t *testing.T) {
		g := parseGroup(t, `{"kind":"static","count":3,"formation":"diagonal","x":400,"y":80}`)
		slotsEqual(t, g.Slots, Slot{344, 80}, Slot{400, 102}, Slot{456, 124})
		m := parseGroup(t, `{"kind":"static","count":3,"formation":"diagonal","x":400,"y":80,"mirror":true}`)
		slotsEqual(t, m.Slots, Slot{456, 80}, Slot{400, 102}, Slot{344, 124})
	})
}

func TestSpawnOrder(t *testing.T) {
	cases := map[string][]Slot{
		"right-to-left": {{456, 80}, {400, 80}, {344, 80}},
		"center-out":    {{400, 80}, {344, 80}, {456, 80}},
		"edges-in":      {{344, 80}, {456, 80}, {400, 80}},
	}
	for order, want := range cases {
		t.Run(order, func(t *testing.T) {
			g := parseGroup(t, `{"kind":"static","count":3,"order":"`+order+`"}`)
			slotsEqual(t, g.Slots, want...)
		})
	}
}

func TestGroupDefaults(t *testing.T) {
	g := parseGroup(t, `{"kind":"patrol","count":1}`)
	if g.Entry != EntryTop || g.Trigger != TriggerAt || g.Hold || g.Slots[0] != (Slot{400, 80}) {
		t.Fatalf("unexpected defaults: %+v", g)
	}
	h := parseGroup(t, `{"kind":"static","count":2,"behavior":"hold","release":90,"entry":"left"}`)
	if !h.Hold || h.Release != 90 || h.Entry != EntryLeft {
		t.Fatalf("hold group not parsed: %+v", h)
	}
}

func TestInvalidLevelsAreRejected(t *testing.T) {
	cases := map[string]struct{ json, want string }{
		"typo in a field":      {groupLevel(`{"kind":"static","count":3,"stager":5}`), "unknown field"},
		"unknown kind":         {groupLevel(`{"kind":"boss","count":1}`), "unknown kind"},
		"empty group":          {groupLevel(`{"kind":"static","count":0}`), "count must be"},
		"unknown formation":    {groupLevel(`{"kind":"static","count":2,"formation":"star"}`), "unknown formation"},
		"unknown entry":        {groupLevel(`{"kind":"static","count":2,"entry":"bottom"}`), "unknown entry"},
		"unknown order":        {groupLevel(`{"kind":"static","count":2,"order":"random"}`), "unknown order"},
		"overlapping members":  {groupLevel(`{"kind":"static","count":3,"spacing":20}`), "overlap"},
		"off screen":           {groupLevel(`{"kind":"static","count":15}`), "off screen"},
		"too low":              {groupLevel(`{"kind":"static","count":1,"y":340}`), "formation band"},
		"release on descender": {groupLevel(`{"kind":"static","count":1,"release":30}`), "release only applies"},
		"cols outside grid":    {groupLevel(`{"kind":"static","count":2,"cols":2}`), "cols only applies"},
		"delay without after":  {groupLevel(`{"kind":"static","count":1,"delay":10}`), "delay only applies"},
		"first group chained":  {groupLevel(`{"kind":"static","count":1,"after":"cleared"}`), "nothing to chain on"},
		"at and after": {groupLevel(`{"kind":"static","count":1},{"kind":"static","count":1,"at":5,"after":"spawned"}`),
			"exclusive"},
		"fixed time after a chain": {groupLevel(`{"kind":"static","count":1},{"kind":"static","count":1,"after":"cleared"},{"kind":"static","count":1,"at":90}`),
			"cannot follow a chained group"},
		"rows and groups": {`{"waves":[{"name":"w","rows":["S"],"groups":[{"kind":"static","count":1}]}]}`,
			"either rows or groups"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseLevelJSON([]byte(c.json))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got error %v, want one mentioning %q", err, c.want)
			}
		})
	}
}

func TestPeakEnemies(t *testing.T) {
	def, err := ParseLevelJSON([]byte(`{"waves":[{"name":"w","groups":[
		{"kind":"static","count":4,"x":200},
		{"kind":"static","count":4,"x":600,"at":30},
		{"kind":"static","count":3,"y":150,"after":"cleared"},
		{"kind":"static","count":5,"y":200,"after":"spawned"}
	]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	// Groups 1+2 overlap (8). Group 3 waits for group 2 to die, but group 1
	// may still be alive: 4+3. Group 4 follows group 3's spawn: 4+3+5 = 12.
	if got := def.Waves[0].PeakEnemies(); got != 12 {
		t.Fatalf("PeakEnemies = %d, want 12", got)
	}
}
