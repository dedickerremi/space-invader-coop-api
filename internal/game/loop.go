package game

import (
	"fmt"
	"time"

	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/types"
)

const tickRate = 30 // Hz
const tickInterval = time.Second / tickRate

// Broadcaster is used to send state to clients (e.g. WebSocket hub).
type Broadcaster interface {
	BroadcastToMatch(matchID string, msg interface{})
}

var (
	loopRunning bool
	stopCh      chan struct{}
)

// StartLoop starts the game loop. It ticks all matches and broadcasts state via b.
func StartLoop(b Broadcaster) {
	if loopRunning {
		return
	}
	loopRunning = true
	stopCh = make(chan struct{})
	fmt.Printf("[GAME] Starting game loop at %d Hz\n", tickRate)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			loopRunning = false
			fmt.Println("[GAME] Game loop stopped")
			return
		case <-ticker.C:
			tickStart := time.Now()
			var broadcastDur time.Duration
			for _, m := range match.GetAllMatches() {
				Tick(m.MatchID)
				state := GetState(m.MatchID)
				if state != nil {
					bcStart := time.Now()
					b.BroadcastToMatch(m.MatchID, types.StateMessage{Type: "STATE", State: *state})
					broadcastDur += time.Since(bcStart)
					if state.GameOver && match.MarkPersisted(m.MatchID) {
						outcome := "defeat"
						if state.Victory {
							outcome = "victory"
						}
						if snap := match.Snapshot(m.MatchID, outcome); snap != nil {
							match.FireFinalizer(snap)
						}
					}
				}
			}
			RecordTick(time.Since(tickStart))
			RecordBroadcast(broadcastDur)
		}
	}
}

// StopLoop stops the game loop.
func StopLoop() {
	if stopCh != nil {
		close(stopCh)
	}
}
