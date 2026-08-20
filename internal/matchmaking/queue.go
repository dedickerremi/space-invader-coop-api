package matchmaking

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// QueueTimeout is how long a player waits for an opponent before giving up.
// Owned here so the WS wait loop and the admin queue page agree on the
// deadline shown to operators.
const QueueTimeout = 30 * time.Second

type waitingPlayer struct {
	playerID string
	conn     *websocket.Conn
	matched  chan string   // receives the generated matchID
	cancel   chan struct{} // closed when a newer connection takes this slot
	since    time.Time
}

func newWaiter(playerID string, conn *websocket.Conn, now time.Time) *waitingPlayer {
	return &waitingPlayer{
		playerID: playerID,
		conn:     conn,
		matched:  make(chan string, 1),
		cancel:   make(chan struct{}),
		since:    now,
	}
}

// mu guards waiting plus all observability state in observe.go.
var (
	mu      sync.Mutex
	waiting *waitingPlayer
)

// Enqueue adds a player to the queue or pairs them with the waiting player.
// If isWaiting is true, the caller must receive from the returned notify channel
// to get the assigned matchID, and must abandon the wait if superseded is
// closed. If isWaiting is false, matchID is ready immediately.
func Enqueue(playerID string, conn *websocket.Conn) (matchID string, isWaiting bool, notify <-chan string, superseded <-chan struct{}) {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	counters.Arrivals++

	// The same playerID is already holding the slot: two browser tabs sharing
	// one stored playerId, or a refresh that reconnected before the old socket
	// was reaped. Pairing them would match the player with themselves — both
	// connections collapse onto one key in the hub and one entity in game
	// state, so the match reports 1/2 connected and never starts. Hand the
	// slot to the newest connection instead and release the old one.
	if waiting != nil && waiting.playerID == playerID {
		counters.Superseded++
		recordLocked(Event{
			At: now, Kind: EventSuperseded, PlayerID: playerID,
			WaitedMs: now.Sub(waiting.since).Milliseconds(),
			Note:     "same playerId re-queued — newest connection takes the slot",
		})
		close(waiting.cancel)
		w := newWaiter(playerID, conn, now)
		waiting = w
		return "", true, w.matched, w.cancel
	}

	if waiting == nil {
		w := newWaiter(playerID, conn, now)
		waiting = w
		recordLocked(Event{At: now, Kind: EventEnqueued, PlayerID: playerID})
		return "", true, w.matched, w.cancel
	}

	// Pair them.
	mid := fmt.Sprintf("q-%d", now.UnixNano())
	waited := now.Sub(waiting.since).Milliseconds()
	waiting.matched <- mid
	counters.Pairs++
	noteWaitLocked(waited)
	recordLocked(Event{
		At: now, Kind: EventMatched, PlayerID: waiting.playerID, MatchID: mid,
		WaitedMs: waited, Note: "paired with " + playerID,
	})
	recordLocked(Event{
		At: now, Kind: EventMatched, PlayerID: playerID, MatchID: mid,
		Note: "paired with " + waiting.playerID,
	})
	waiting = nil
	return mid, false, nil, nil
}

// Dequeue removes a player from the queue if their connection is the one
// holding the slot. reason should be EventTimeout or EventDisconnected so the
// queue page can tell the two apart.
//
// The conn is matched as well as the playerID: a superseded goroutine must
// never be able to evict the newer connection that replaced it.
//
// A false return means this connection no longer held the slot — either
// Enqueue paired it first (the matchID is already buffered on notify) or a
// newer connection took over.
func Dequeue(playerID string, conn *websocket.Conn, reason EventKind) bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()

	if waiting != nil && waiting.playerID == playerID && waiting.conn == conn {
		waited := now.Sub(waiting.since).Milliseconds()
		waiting = nil
		switch reason {
		case EventTimeout:
			counters.TimedOut++
		case EventDisconnected:
			counters.Disconnected++
		}
		recordLocked(Event{At: now, Kind: reason, PlayerID: playerID, WaitedMs: waited})
		return true
	}

	counters.PairRaces++
	recordLocked(Event{
		At: now, Kind: EventPairRace, PlayerID: playerID,
		Note: string(reason) + " on a connection that no longer held the slot (already paired, or superseded)",
	})
	return false
}

// QueueSize returns 0 or 1.
func QueueSize() int {
	mu.Lock()
	defer mu.Unlock()
	if waiting != nil {
		return 1
	}
	return 0
}
