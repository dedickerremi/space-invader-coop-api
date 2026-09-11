// Command level-preview prints what each campaign level asks of the players:
// how many enemies, how many at once, how much firepower, and how every wave
// is choreographed. It reads the level files from the repo, so it is the way
// to review a level without playing it.
//
//	go run ./cmd/level-preview                   # both campaigns
//	go run ./cmd/level-preview solo              # one campaign
//	go run ./cmd/level-preview coop-level3.json  # one level
package main

import (
	"fmt"
	"os"
	"strings"

	"space-invaders-coop/backend-go/internal/game"
)

func main() {
	var names []string
	switch arg := strings.Join(os.Args[1:], ""); {
	case arg == "":
		names = append(game.CampaignLevels("solo"), game.CampaignLevels("coop")...)
	case arg == "solo" || arg == "coop":
		names = game.CampaignLevels(arg)
	default:
		names = []string{arg}
	}

	for _, name := range names {
		def, err := load(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			os.Exit(1)
		}
		printLevel(name, def)
	}
}

// load parses a level without the loader's per-wave log lines.
func load(name string) (*game.LevelDefinition, error) {
	stdout := os.Stdout
	if null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		os.Stdout = null
		defer null.Close()
	}
	defer func() { os.Stdout = stdout }()
	return game.LoadLevel(name)
}

func printLevel(name string, def *game.LevelDefinition) {
	peak := 0
	for _, w := range def.Waves {
		peak = max(peak, w.PeakEnemies())
	}
	boss := "no boss"
	if def.BossKind != "" {
		boss = "boss " + def.BossKind
		if def.BossHP > 0 {
			boss += fmt.Sprintf(" (%d hp)", def.BossHP)
		}
	}
	fmt.Printf("\n%s — %s\n", name, def.Title)
	fmt.Printf("  %d enemies · up to %d on screen · pressure %.1f bullets/s · fire rate ×%.2f · %s\n",
		def.EnemyCount(), peak, def.Pressure(), def.FireRate, boss)

	for _, w := range def.Waves {
		fmt.Printf("  %d. %-14s %3d enemies, up to %2d at once, %.1f bullets/s\n",
			w.Number, w.Name, w.EnemyCount(), w.PeakEnemies(), w.PeakFirepower()*def.FireRate)
		for _, g := range w.Groups {
			fmt.Printf("       %2d %-6s %-8s %-6s %-22s %s\n",
				len(g.Slots), g.Kind, g.Formation, g.Entry, behavior(g), timing(g))
		}
	}
}

func behavior(g game.GroupDefinition) string {
	switch {
	case g.Hold && g.Release > 0:
		return fmt.Sprintf("hold, dive after %.1fs", float64(g.Release)/30)
	case g.Hold:
		return "hold until killed"
	}
	return "descend"
}

func timing(g game.GroupDefinition) string {
	var when string
	switch g.Trigger {
	case game.TriggerSpawned:
		when = fmt.Sprintf("%.1fs after previous spawned", float64(g.Delay)/30)
	case game.TriggerCleared:
		when = fmt.Sprintf("%.1fs after previous cleared", float64(g.Delay)/30)
	default:
		when = fmt.Sprintf("at %.1fs", float64(g.At)/30)
	}
	if g.Stagger > 0 && len(g.Slots) > 1 {
		when += fmt.Sprintf(", one every %.2fs", float64(g.Stagger)/30)
	}
	return when
}
