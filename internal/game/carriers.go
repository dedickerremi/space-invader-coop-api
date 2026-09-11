package game

import (
	"fmt"
	"math"
	"math/rand"
	"slices"

	"space-invaders-coop/backend-go/internal/types"
)

// Carriers deliver bonuses. They are scripted into waves like groups — a
// level decides when and what — but where they come from and how they drift
// can be random. Unlike enemies they never shoot, never hurt, and never hold
// up the end of a wave: shoot one down before it leaves the screen and its
// bonus drops.

type CarrierKind string

const (
	CarrierAsteroid CarrierKind = "asteroid" // small rock drifting across the screen: a small bonus
	CarrierCourier  CarrierKind = "courier"  // the goblin's ship crossing the top: firepower
)

// CarrierDefinition is one scripted carrier in a wave.
type CarrierDefinition struct {
	Kind  CarrierKind
	Group int    // launches when this group (1-based) starts...
	Delay int    // ...plus this many ticks
	From  string // "left" | "right" | "top" (asteroid only) | "" = random
	Y     int    // courier altitude; 0 = random within the formation band
	Drop  string // bonus released; "" on an asteroid = a random small bonus
}

type jsonCarrier struct {
	Kind  string `json:"kind"`  // "asteroid" | "courier"
	Group int    `json:"group"` // group whose start launches it (default 1)
	Delay int    `json:"delay"` // ticks after that group starts
	From  string `json:"from"`  // left | right | top (asteroid) | "" = random
	Y     int    `json:"y"`     // courier only: altitude (default random)
	Drop  string `json:"drop"`  // bonus (default: courier double_shot, asteroid random small)
}

const (
	asteroidHP       = 3
	asteroidSize     = 36 // hitbox, px
	asteroidMinSpeed = 1.2
	asteroidMaxSpeed = 2.2

	courierHP        = 2
	courierWidth     = 44 // hitbox, px
	courierHeight    = 24
	courierSpeed     = 3.5
	courierBob       = 10 // px either side of its altitude
	courierBobPeriod = 60 // ticks

	pointsPerCarrier = 150
	carrierMargin    = 60 // px beyond the screen edge before a carrier is gone
)

func buildCarrier(jc jsonCarrier, groups int) (CarrierDefinition, error) {
	c := CarrierDefinition{
		Kind:  CarrierKind(jc.Kind),
		Group: jc.Group,
		Delay: jc.Delay,
		From:  jc.From,
		Y:     jc.Y,
		Drop:  jc.Drop,
	}
	if c.Group == 0 {
		c.Group = 1
	}
	if c.Group < 1 || c.Group > groups {
		return c, fmt.Errorf("group %d does not exist (the wave has %d)", c.Group, groups)
	}
	if c.Delay < 0 {
		return c, fmt.Errorf("delay cannot be negative")
	}
	if c.Drop != "" && !slices.Contains(powerUpKinds, c.Drop) {
		return c, fmt.Errorf("unknown drop %q", c.Drop)
	}

	switch c.Kind {
	case CarrierCourier:
		if c.From != "" && c.From != "left" && c.From != "right" {
			return c, fmt.Errorf("a courier crosses from left or right, not %q", c.From)
		}
		if c.Y != 0 && (c.Y < slotMinY || c.Y > slotMaxY) {
			return c, fmt.Errorf("courier altitude %d is outside the formation band %d–%d", c.Y, slotMinY, slotMaxY)
		}
		if c.Drop == "" {
			c.Drop = "double_shot"
		}
	case CarrierAsteroid:
		if c.From != "" && c.From != "left" && c.From != "right" && c.From != "top" {
			return c, fmt.Errorf("an asteroid comes from left, right or top, not %q", c.From)
		}
		if c.Y != 0 {
			return c, fmt.Errorf("y only applies to a courier")
		}
	default:
		return c, fmt.Errorf("unknown carrier %q (asteroid, courier)", jc.Kind)
	}
	return c, nil
}

// launchCarriers starts every carrier of the wave whose group has started
// and whose delay has run out. Called from the wave spawner each tick.
func launchCarriers(s *types.GameState, wave *WaveDefinition) {
	for ci := range wave.Carriers {
		c := &wave.Carriers[ci]
		if s.WaveCarriers[ci] {
			continue
		}
		g := s.WaveGroups[c.Group-1]
		if g.Started && s.WaveTick >= g.StartTick+c.Delay {
			s.WaveCarriers[ci] = true
			s.Carriers = append(s.Carriers, newCarrier(c))
		}
	}
}

// newCarrier places a carrier at the edge it enters from, picking whatever
// the level left open — side, altitude, heading — at random.
func newCarrier(c *CarrierDefinition) types.Carrier {
	k := types.Carrier{Kind: string(c.Kind), Drop: c.Drop}
	from := c.From

	switch c.Kind {
	case CarrierCourier:
		k.HP = courierHP
		if from == "" {
			from = []string{"left", "right"}[rand.Intn(2)]
		}
		k.BaseY = float64(c.Y)
		if c.Y == 0 {
			k.BaseY = float64(60 + rand.Intn(101)) // 60–160
		}
		k.FY = k.BaseY
		if from == "left" {
			k.FX, k.VX = -courierWidth, courierSpeed
		} else {
			k.FX, k.VX = gameWidth+courierWidth, -courierSpeed
		}

	case CarrierAsteroid:
		k.HP = asteroidHP
		if k.Drop == "" {
			k.Drop = randomSmallBonus()
		}
		if from == "" {
			from = []string{"left", "right", "top"}[rand.Intn(3)]
		}
		speed := asteroidMinSpeed + rand.Float64()*(asteroidMaxSpeed-asteroidMinSpeed)
		switch from {
		case "top":
			k.FX, k.FY = float64(120+rand.Intn(561)), -asteroidSize // x 120–680
			k.VX = (rand.Float64()*2 - 1) * speed * 0.6
			k.VY = speed
		default:
			// Enter high on a side and drift down across the screen.
			k.FY = float64(40 + rand.Intn(181)) // 40–220
			k.VX = speed
			k.VY = speed * (0.3 + rand.Float64()*0.4)
			k.FX = -asteroidSize
			if from == "right" {
				k.FX, k.VX = gameWidth+asteroidSize, -k.VX
			}
		}
	}
	k.X, k.Y = round(k.FX), round(k.FY)
	return k
}

// tickCarriers moves carriers and drops the ones that left the screen,
// bonus and all.
func tickCarriers(s *types.GameState) {
	kept := s.Carriers[:0]
	for _, k := range s.Carriers {
		k.Age++
		k.FX += k.VX
		if k.Kind == string(CarrierCourier) {
			k.FY = k.BaseY + courierBob*math.Sin(2*math.Pi*float64(k.Age)/courierBobPeriod)
		} else {
			k.FY += k.VY
		}
		k.X, k.Y = round(k.FX), round(k.FY)
		if k.X < -carrierMargin || k.X > gameWidth+carrierMargin || k.Y > gameHeight+carrierMargin {
			continue
		}
		kept = append(kept, k)
	}
	s.Carriers = kept
}

// tickCarrierHits lets player bullets chip at carriers. The one that brings
// a carrier down scores it and releases its bonus where it broke.
func tickCarrierHits(s *types.GameState) {
	if len(s.Carriers) == 0 || len(s.Bullets) == 0 {
		return
	}
	spent := make(map[int]bool)
	kept := s.Carriers[:0]
	for _, k := range s.Carriers {
		w, h := asteroidSize, asteroidSize
		if k.Kind == string(CarrierCourier) {
			w, h = courierWidth, courierHeight
		}
		for bi, b := range s.Bullets {
			if spent[bi] || k.HP <= 0 {
				continue
			}
			if abs(b.X-k.X) <= (w+BulletWidth)/2 && abs(b.Y-k.Y) <= (h+BulletHeight)/2 {
				spent[bi] = true
				k.HP--
				if k.HP == 0 {
					if s.Points != nil {
						s.Points[b.OwnerID] += pointsPerCarrier
					}
					spawnPowerUp(s, k.X, k.Y, k.Drop)
				}
			}
		}
		if k.HP > 0 {
			kept = append(kept, k)
		}
	}
	s.Carriers = kept

	if len(spent) > 0 {
		bullets := s.Bullets[:0]
		for bi, b := range s.Bullets {
			if !spent[bi] {
				bullets = append(bullets, b)
			}
		}
		s.Bullets = bullets
	}
}
