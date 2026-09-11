package game

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

//go:embed levels/*.json
var levelFiles embed.FS

// --- Enemy kinds ---

type EnemyKind string

const (
	EnemyStatic EnemyKind = "static"
	EnemyPatrol EnemyKind = "patrol"
)

// Entry is how a group's members travel to their formation slots.
type Entry string

const (
	EntryAppear Entry = "appear" // materialise on the slot (the legacy row behaviour)
	EntryTop    Entry = "top"    // drop in from above the screen
	EntryLeft   Entry = "left"   // swoop in from the left edge
	EntryRight  Entry = "right"  // swoop in from the right edge
)

// Trigger decides when a group starts spawning.
type Trigger string

const (
	TriggerAt      Trigger = "at"      // a fixed wave tick
	TriggerSpawned Trigger = "spawned" // Delay ticks after the previous group finished spawning
	TriggerCleared Trigger = "cleared" // Delay ticks after the previous group was destroyed
)

// Slot is a formation position: an enemy centre, in px.
type Slot struct{ X, Y int }

// GroupDefinition is one choreographed spawn group ("escouade").
type GroupDefinition struct {
	Kind      EnemyKind
	Formation string // layout name, for display; "row" for a legacy grid row
	Slots     []Slot // in spawn order
	Entry     Entry
	Stagger   int // ticks between consecutive members
	Trigger   Trigger
	At        int  // TriggerAt: wave tick the group starts on
	Delay     int  // TriggerSpawned / TriggerCleared: ticks after the event
	Hold      bool // keep the slot instead of descending
	Release   int  // holding ticks before the member dives; 0 = hold until killed
}

// --- Wave definition ---

type WaveDefinition struct {
	Number int
	Name   string
	Groups []GroupDefinition
}

// EnemyCount returns how many enemies the wave spawns in total.
func (w WaveDefinition) EnemyCount() int {
	n := 0
	for _, g := range w.Groups {
		n += len(g.Slots)
	}
	return n
}

// PeakEnemies is an upper bound on how many of the wave's enemies can be on
// screen at once. It assumes the players kill nothing until a "cleared"
// trigger forces them to, so it is the number a wave is designed around, not
// a typical one. A group is only known to be gone once a later group was
// triggered by its destruction.
func (w WaveDefinition) PeakEnemies() int {
	gone := make([]bool, len(w.Groups))
	peak := 0
	for k, g := range w.Groups {
		if k > 0 && g.Trigger == TriggerCleared {
			gone[k-1] = true
		}
		sum := 0
		for j := 0; j <= k; j++ {
			if !gone[j] {
				sum += len(w.Groups[j].Slots)
			}
		}
		if sum > peak {
			peak = sum
		}
	}
	return peak
}

// PeakFirepower is the enemy bullets per second the wave can put out at its
// fullest, counted like PeakEnemies: patrols fire three bullets per
// interval, holding statics one, descending statics none (their threat is
// reaching the bottom). It ignores the level's fire rate.
func (w WaveDefinition) PeakFirepower() float64 {
	gone := make([]bool, len(w.Groups))
	peak := 0.0
	for k, g := range w.Groups {
		if k > 0 && g.Trigger == TriggerCleared {
			gone[k-1] = true
		}
		bps := 0.0
		for j := 0; j <= k; j++ {
			if gone[j] {
				continue
			}
			n := float64(len(w.Groups[j].Slots))
			switch {
			case w.Groups[j].Kind == EnemyPatrol:
				bps += n * 3 * 30 / patrolShootInterval
			case w.Groups[j].Hold:
				bps += n * 30 / holdShootInterval
			}
		}
		peak = max(peak, bps)
	}
	return peak
}

// --- Level definition ---

type LevelDefinition struct {
	Title    string // display name; the file name stays the storage key
	Waves    []WaveDefinition
	BossKind string  // "" = no boss at the end of this level
	BossHP   int     // 0 = the boss kind's default
	FireRate float64 // multiplies how often the level's enemies shoot (1 = default)
}

// Pressure is the level's sustained firepower: the average over its waves of
// PeakFirepower, scaled by the fire rate. It is the number the campaigns ramp
// on from one sector to the next.
func (l *LevelDefinition) Pressure() float64 {
	total := 0.0
	for _, w := range l.Waves {
		total += w.PeakFirepower()
	}
	return total / float64(len(l.Waves)) * l.FireRate
}

// EnemyCount returns how many regular enemies the level spawns (boss excluded).
func (l *LevelDefinition) EnemyCount() int {
	n := 0
	for _, w := range l.Waves {
		n += w.EnemyCount()
	}
	return n
}

// --- JSON schema ---

type jsonLevel struct {
	Title    string     `json:"title"`
	FireRate float64    `json:"fireRate"` // default 1; 1.5 = enemies shoot 50% more often
	Waves    []jsonWave `json:"waves"`
	Boss     *jsonBoss  `json:"boss,omitempty"`
}

type jsonWave struct {
	Name string `json:"name"`

	// Legacy grid format: every row spawns at once at the top of the screen.
	RowDelay int      `json:"rowDelay"` // ticks between rows (default 30)
	Rows     []string `json:"rows"`

	// Choreographed format.
	Groups []jsonGroup `json:"groups"`
}

type jsonGroup struct {
	Kind      string `json:"kind"`      // "static" | "patrol"
	Count     int    `json:"count"`     // members
	Formation string `json:"formation"` // line | column | grid | v | arc | diagonal (default line)
	Cols      int    `json:"cols"`      // grid only (default: squarest fit)
	Spacing   int    `json:"spacing"`   // px between neighbours (default 56)
	X         int    `json:"x"`         // formation centre (default mid-screen)
	Y         int    `json:"y"`         // formation top edge (default 80)
	Mirror    bool   `json:"mirror"`    // flip horizontally around X
	Entry     string `json:"entry"`     // top | left | right | appear (default top)
	Order     string `json:"order"`     // member spawn order (default: formation order)
	Stagger   int    `json:"stagger"`   // ticks between members
	At        int    `json:"at"`        // start tick, when not chained with "after"
	After     string `json:"after"`     // "spawned" | "cleared": chain on the previous group
	Delay     int    `json:"delay"`     // ticks after the "after" event
	Behavior  string `json:"behavior"`  // descend | hold (default descend)
	Release   int    `json:"release"`   // hold only: holding ticks before diving
}

type jsonBoss struct {
	Kind string `json:"kind"`
	HP   int    `json:"hp,omitempty"`
}

const (
	defaultRowDelay = 30
	defaultSpawnY   = 20
	gridMargin      = 40

	defaultGroupX  = gameWidth / 2
	defaultGroupY  = 80
	defaultSpacing = 56
	rowSpacing     = 44 // vertical gap between grid rows and column members
	vRise          = 20 // vertical step between the ranks of a V
	arcDepth       = 40 // how far the middle of an arc bulges toward the players
	diagonalStep   = 22 // vertical step between diagonal members
	maxGroupSize   = 24
	minFireRate    = 0.25
	maxFireRate    = 3.0

	// Formation slots stay in the top half: the players can move up to
	// playerYMin, and a formation sitting on top of them is unreadable.
	slotMinY = 40
	slotMaxY = 300
)

// LoadLevel loads and parses a level JSON file (disk first, embed fallback).
func LoadLevel(filename string) (*LevelDefinition, error) {
	data, err := ReadLevelFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read level file %s: %w", filename, err)
	}
	return ParseLevelJSON(data)
}

// ParseLevelJSON parses and validates level JSON data. Unknown fields are
// rejected so a typo ("stager") fails loudly instead of silently tuning
// nothing.
func ParseLevelJSON(data []byte) (*LevelDefinition, error) {
	var raw jsonLevel
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid level JSON: %w", err)
	}

	if len(raw.Waves) == 0 {
		return nil, fmt.Errorf("level has no waves")
	}

	level := &LevelDefinition{Title: raw.Title, FireRate: 1}
	if raw.FireRate != 0 {
		// Difficulty is tuned per level through this and the composition of
		// the waves; the patterns themselves stay identical run to run.
		if raw.FireRate < minFireRate || raw.FireRate > maxFireRate {
			return nil, fmt.Errorf("fireRate must be between %g and %g, got %g", minFireRate, maxFireRate, raw.FireRate)
		}
		level.FireRate = raw.FireRate
	}

	for i, jw := range raw.Waves {
		wave := WaveDefinition{Number: len(level.Waves) + 1, Name: jw.Name}

		switch {
		case len(jw.Groups) > 0 && len(jw.Rows) > 0:
			return nil, fmt.Errorf("wave %d (%q): use either rows or groups, not both", i+1, jw.Name)
		case len(jw.Groups) > 0:
			for gi, jg := range jw.Groups {
				g, err := buildGroup(jg, gi, jw.Groups)
				if err != nil {
					return nil, fmt.Errorf("wave %d (%q) group %d: %w", i+1, jw.Name, gi+1, err)
				}
				wave.Groups = append(wave.Groups, g)
			}
		default:
			wave.Groups = rowsToGroups(jw)
		}

		if wave.EnemyCount() > 0 {
			level.Waves = append(level.Waves, wave)
		}
	}

	if len(level.Waves) == 0 {
		return nil, fmt.Errorf("no valid waves in level")
	}

	if raw.Boss != nil {
		if raw.Boss.HP < 0 {
			return nil, fmt.Errorf("boss hp must be positive")
		}
		level.BossKind = raw.Boss.Kind
		level.BossHP = raw.Boss.HP
	}

	fmt.Printf("[LEVELS] Loaded %d waves, %d enemies (boss=%q)\n", len(level.Waves), level.EnemyCount(), level.BossKind)
	for _, w := range level.Waves {
		fmt.Printf("[LEVELS]   Wave %d (%s): %d enemies in %d groups\n", w.Number, w.Name, w.EnemyCount(), len(w.Groups))
	}

	return level, nil
}

// rowsToGroups converts a legacy grid wave. Each row becomes one group per
// enemy kind, appearing in place at the top of the screen on the row's tick,
// which is exactly what the grid format did before groups existed.
func rowsToGroups(jw jsonWave) []GroupDefinition {
	rowDelay := jw.RowDelay
	if rowDelay <= 0 {
		rowDelay = defaultRowDelay
	}
	usableWidth := gameWidth - 2*gridMargin

	var groups []GroupDefinition
	for rowIdx, row := range jw.Rows {
		cols := len(row)
		if cols == 0 {
			continue
		}
		byKind := map[EnemyKind][]Slot{}
		for colIdx, ch := range row {
			var kind EnemyKind
			switch ch {
			case 'S', 's':
				kind = EnemyStatic
			case 'P', 'p':
				kind = EnemyPatrol
			default:
				continue
			}
			x := gameWidth / 2
			if cols > 1 {
				x = gridMargin + (colIdx * usableWidth / (cols - 1))
			}
			byKind[kind] = append(byKind[kind], Slot{X: x, Y: defaultSpawnY})
		}
		for _, kind := range []EnemyKind{EnemyStatic, EnemyPatrol} {
			if slots := byKind[kind]; len(slots) > 0 {
				groups = append(groups, GroupDefinition{
					Kind:      kind,
					Formation: "row",
					Slots:     slots,
					Entry:     EntryAppear,
					Trigger:   TriggerAt,
					At:        rowIdx * rowDelay,
				})
			}
		}
	}
	return groups
}

func buildGroup(jg jsonGroup, index int, all []jsonGroup) (GroupDefinition, error) {
	g := GroupDefinition{
		Stagger: jg.Stagger,
		Release: jg.Release,
		Delay:   jg.Delay,
	}

	switch EnemyKind(jg.Kind) {
	case EnemyStatic, EnemyPatrol:
		g.Kind = EnemyKind(jg.Kind)
	default:
		return g, fmt.Errorf("unknown kind %q (static, patrol)", jg.Kind)
	}

	if jg.Count < 1 || jg.Count > maxGroupSize {
		return g, fmt.Errorf("count must be between 1 and %d, got %d", maxGroupSize, jg.Count)
	}
	if jg.Stagger < 0 || jg.Delay < 0 || jg.At < 0 || jg.Release < 0 {
		return g, fmt.Errorf("stagger, delay, at and release cannot be negative")
	}

	switch Entry(jg.Entry) {
	case "":
		g.Entry = EntryTop
	case EntryTop, EntryLeft, EntryRight, EntryAppear:
		g.Entry = Entry(jg.Entry)
	default:
		return g, fmt.Errorf("unknown entry %q (top, left, right, appear)", jg.Entry)
	}

	switch jg.Behavior {
	case "", "descend":
	case "hold":
		g.Hold = true
	default:
		return g, fmt.Errorf("unknown behavior %q (descend, hold)", jg.Behavior)
	}
	if jg.Release > 0 && !g.Hold {
		return g, fmt.Errorf("release only applies to behavior \"hold\"")
	}

	switch jg.After {
	case "":
		g.Trigger = TriggerAt
		g.At = jg.At
		if jg.Delay != 0 {
			return g, fmt.Errorf("delay only applies with \"after\"; use \"at\" for a fixed start")
		}
		// Time-triggered groups come first. Once a wave chains on events,
		// a fixed-time group could start before the chain reaches it, and the
		// on-screen budget (PeakEnemies) would no longer hold.
		for j := 0; j < index; j++ {
			if all[j].After != "" {
				return g, fmt.Errorf("a group with \"at\" cannot follow a chained group")
			}
		}
	case "spawned":
		g.Trigger = TriggerSpawned
	case "cleared":
		g.Trigger = TriggerCleared
	default:
		return g, fmt.Errorf("unknown after %q (spawned, cleared)", jg.After)
	}
	if g.Trigger != TriggerAt {
		if index == 0 {
			return g, fmt.Errorf("the first group has nothing to chain on; use \"at\"")
		}
		if jg.At != 0 {
			return g, fmt.Errorf("\"at\" and \"after\" are exclusive")
		}
	}

	slots, err := formationSlots(jg)
	if err != nil {
		return g, err
	}
	for i, s := range slots {
		if s.X < enemySize || s.X > gameWidth-enemySize {
			return g, fmt.Errorf("member %d at x=%d is off screen", i+1, s.X)
		}
		if s.Y < slotMinY || s.Y > slotMaxY {
			return g, fmt.Errorf("member %d at y=%d is outside the formation band %d–%d", i+1, s.Y, slotMinY, slotMaxY)
		}
		for j := 0; j < i; j++ {
			if abs(s.X-slots[j].X) < enemySize && abs(s.Y-slots[j].Y) < enemySize {
				return g, fmt.Errorf("members %d and %d overlap; increase spacing", j+1, i+1)
			}
		}
	}

	ordered, err := orderSlots(slots, jg.Order, centreX(jg))
	if err != nil {
		return g, err
	}
	g.Slots = ordered
	g.Formation = jg.Formation
	if g.Formation == "" {
		g.Formation = "line"
	}
	return g, nil
}

func centreX(jg jsonGroup) int {
	if jg.X == 0 {
		return defaultGroupX
	}
	return jg.X
}

// formationSlots lays out a group's members in formation order.
func formationSlots(jg jsonGroup) ([]Slot, error) {
	n := jg.Count
	x0 := float64(centreX(jg))
	y0 := jg.Y
	if y0 == 0 {
		y0 = defaultGroupY
	}
	sp := float64(jg.Spacing)
	if jg.Spacing == 0 {
		sp = defaultSpacing
	}
	if jg.Cols != 0 && jg.Formation != "grid" {
		return nil, fmt.Errorf("cols only applies to formation \"grid\"")
	}

	// offset of member i on a centred row of m members
	across := func(i, m int) float64 { return (float64(i) - float64(m-1)/2) * sp }

	var slots []Slot
	switch jg.Formation {
	case "", "line":
		for i := 0; i < n; i++ {
			slots = append(slots, Slot{X: round(x0 + across(i, n)), Y: y0})
		}
	case "column":
		for i := 0; i < n; i++ {
			slots = append(slots, Slot{X: round(x0), Y: y0 + i*rowSpacing})
		}
	case "grid":
		cols := jg.Cols
		if cols <= 0 {
			cols = int(math.Ceil(math.Sqrt(float64(n))))
		}
		if cols > n {
			cols = n
		}
		for i := 0; i < n; i++ {
			row, col := i/cols, i%cols
			inRow := cols
			if last := n - row*cols; last < cols {
				inRow = last // centre a short last row
			}
			slots = append(slots, Slot{X: round(x0 + across(col, inRow)), Y: y0 + row*rowSpacing})
		}
	case "v":
		// Leader at the front (lowest), ranks trailing back toward the top.
		ranks := n / 2
		for i := 0; i < n; i++ {
			rank := (i + 1) / 2
			side := 1.0
			if i%2 == 1 {
				side = -1
			}
			slots = append(slots, Slot{X: round(x0 + side*float64(rank)*sp), Y: y0 + (ranks-rank)*vRise})
		}
	case "arc":
		half := float64(n-1) / 2 * sp
		for i := 0; i < n; i++ {
			dx := across(i, n)
			bulge := 1.0
			if half > 0 {
				bulge = 1 - (dx/half)*(dx/half)
			}
			slots = append(slots, Slot{X: round(x0 + dx), Y: y0 + round(arcDepth*bulge)})
		}
	case "diagonal":
		for i := 0; i < n; i++ {
			slots = append(slots, Slot{X: round(x0 + across(i, n)), Y: y0 + i*diagonalStep})
		}
	default:
		return nil, fmt.Errorf("unknown formation %q (line, column, grid, v, arc, diagonal)", jg.Formation)
	}

	if jg.Mirror {
		for i := range slots {
			slots[i].X = round(2*x0) - slots[i].X
		}
	}
	return slots, nil
}

// orderSlots returns the slots in the order members spawn. With a stagger,
// this is the direction the cascade runs in.
func orderSlots(slots []Slot, order string, cx int) ([]Slot, error) {
	out := append([]Slot(nil), slots...)
	var less func(a, b Slot) bool
	switch order {
	case "":
		return out, nil
	case "left-to-right":
		less = func(a, b Slot) bool { return a.X < b.X || (a.X == b.X && a.Y < b.Y) }
	case "right-to-left":
		less = func(a, b Slot) bool { return a.X > b.X || (a.X == b.X && a.Y < b.Y) }
	case "center-out":
		less = func(a, b Slot) bool { return abs(a.X-cx) < abs(b.X-cx) }
	case "edges-in":
		less = func(a, b Slot) bool { return abs(a.X-cx) > abs(b.X-cx) }
	case "top-down":
		less = func(a, b Slot) bool { return a.Y < b.Y || (a.Y == b.Y && a.X < b.X) }
	default:
		return nil, fmt.Errorf("unknown order %q (left-to-right, right-to-left, center-out, edges-in, top-down)", order)
	}
	sort.SliceStable(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out, nil
}

func round(f float64) int { return int(math.Round(f)) }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
