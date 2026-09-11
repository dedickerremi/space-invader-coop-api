package matchmaking

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"space-invaders-coop/backend-go/internal/types"
)

// QueueTimeout is how long a player waits for an opponent before giving up.
// Owned here so the WS wait loop and the admin queue page agree on the
// deadline shown to operators.
const QueueTimeout = 30 * time.Second

// Difficulties lists the queue's slots, in display order.
var Difficulties = []string{types.DifficultyEasy, types.DifficultyMedium, types.DifficultyHard}

type waitingPlayer struct {
	playerID   string
	difficulty string
	conn       *websocket.Conn
	matched    chan string   // receives the generated matchID
	cancel     chan struct{} // closed when a newer connection takes this slot
	since      time.Time
}

func newWaiter(playerID, difficulty string, conn *websocket.Conn, now time.Time) *waitingPlayer {
	return &waitingPlayer{
		playerID:   playerID,
		difficulty: difficulty,
		conn:       conn,
		matched:    make(chan string, 1),
		cancel:     make(chan struct{}),
		since:      now,
	}
}

// waiting holds at most one player per difficulty: a player is only ever
// paired with someone who picked the same difficulty.
//
// mu guards waiting plus all observability state in observe.go.
var (
	mu      sync.Mutex
	waiting = map[string]*waitingPlayer{}
)

// Enqueue adds a player to their difficulty's slot, or pairs them with the
// player already waiting there. If isWaiting is true, the caller must
// receive from the returned notify channel to get the assigned matchID, and
// must abandon the wait if superseded is closed. If isWaiting is false,
// matchID is ready immediately.
func Enqueue(playerID, difficulty string, conn *websocket.Conn) (matchID string, isWaiting bool, notify <-chan string, superseded <-chan struct{}) {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	counters.Arrivals++
	if !types.IsDifficulty(difficulty) {
		difficulty = types.DifficultyEasy
	}

	// The same playerID is already waiting — a refresh that reconnected
	// before the old socket was reaped, possibly after picking another
	// difficulty. Pairing them would match the player with themselves: both
	// connections collapse onto one key in the hub and one entity in game
	// state, so the match reports 1/2 connected and never starts. Release
	// the old connection; the new one queues normally.
	for d, w := range waiting {
		if w.playerID == playerID {
			counters.Superseded++
			recordLocked(Event{
				At: now, Kind: EventSuperseded, PlayerID: playerID, Difficulty: d,
				WaitedMs: now.Sub(w.since).Milliseconds(),
				Note:     "same playerId re-queued — newest connection takes over",
			})
			close(w.cancel)
			delete(waiting, d)
			break
		}
	}

	w := waiting[difficulty]
	if w == nil {
		nw := newWaiter(playerID, difficulty, conn, now)
		waiting[difficulty] = nw
		recordLocked(Event{At: now, Kind: EventEnqueued, PlayerID: playerID, Difficulty: difficulty})
		return "", true, nw.matched, nw.cancel
	}

	// Pair them.
	mid := fmt.Sprintf("q-%d", now.UnixNano())
	waited := now.Sub(w.since).Milliseconds()
	w.matched <- mid
	counters.Pairs++
	noteWaitLocked(waited)
	recordLocked(Event{
		At: now, Kind: EventMatched, PlayerID: w.playerID, Difficulty: difficulty, MatchID: mid,
		WaitedMs: waited, Note: "paired with " + playerID,
	})
	recordLocked(Event{
		At: now, Kind: EventMatched, PlayerID: playerID, Difficulty: difficulty, MatchID: mid,
		Note: "paired with " + w.playerID,
	})
	delete(waiting, difficulty)
	return mid, false, nil, nil
}

// Dequeue removes a player from the queue if their connection is the one
// holding a slot. reason should be EventTimeout or EventDisconnected so the
// queue page can tell the two apart.
//
// The conn is matched as well as the playerID: a superseded goroutine must
// never be able to evict the newer connection that replaced it.
//
// A false return means this connection no longer held a slot — either
// Enqueue paired it first (the matchID is already buffered on notify) or a
// newer connection took over.
func Dequeue(playerID string, conn *websocket.Conn, reason EventKind) bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()

	for d, w := range waiting {
		if w.playerID != playerID || w.conn != conn {
			continue
		}
		delete(waiting, d)
		switch reason {
		case EventTimeout:
			counters.TimedOut++
		case EventDisconnected:
			counters.Disconnected++
		}
		recordLocked(Event{At: now, Kind: reason, PlayerID: playerID, Difficulty: d, WaitedMs: now.Sub(w.since).Milliseconds()})
		return true
	}

	counters.PairRaces++
	recordLocked(Event{
		At: now, Kind: EventPairRace, PlayerID: playerID,
		Note: string(reason) + " on a connection that no longer held a slot (already paired, or superseded)",
	})
	return false
}

// QueueSize returns how many players are waiting, across difficulties.
func QueueSize() int {
	mu.Lock()
	defer mu.Unlock()
	return len(waiting)
}

// WaitingByDifficulty returns how many players wait at each difficulty (0 or
// 1). The lobby shows it so players can pick the difficulty where someone is
// already waiting instead of splitting a thin queue three ways.
func WaitingByDifficulty() map[string]int {
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]int, len(Difficulties))
	for _, d := range Difficulties {
		out[d] = 0
		if waiting[d] != nil {
			out[d] = 1
		}
	}
	return out
}
