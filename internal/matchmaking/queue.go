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
	matched  chan string // receives the generated matchID
	since    time.Time
}

// mu guards waiting plus all observability state in observe.go.
var (
	mu      sync.Mutex
	waiting *waitingPlayer
)

// Enqueue adds a player to the queue or pairs them with the waiting player.
// If isWaiting is true, the caller must receive from the returned notify channel
// to get the assigned matchID. If isWaiting is false, matchID is ready immediately.
func Enqueue(playerID string, conn *websocket.Conn) (matchID string, isWaiting bool, notify <-chan string) {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	counters.Arrivals++

	if waiting == nil {
		ch := make(chan string, 1)
		waiting = &waitingPlayer{playerID: playerID, conn: conn, matched: ch, since: now}
		recordLocked(Event{At: now, Kind: EventEnqueued, PlayerID: playerID})
		return "", true, ch
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
	return mid, false, nil
}

// Dequeue removes a player from the queue if they are the one waiting. reason
// should be EventTimeout or EventDisconnected so the queue page can tell the
// two apart.
//
// A false return means the player no longer held the slot because Enqueue
// paired them first — their matchID is already buffered on the notify channel.
// That race is counted separately so a spike in it stays visible.
func Dequeue(playerID string, reason EventKind) bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()

	if waiting != nil && waiting.playerID == playerID {
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
		Note: string(reason) + " lost the race with pairing — player was already matched",
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
