package matchmaking

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type waitingPlayer struct {
	playerID string
	conn     *websocket.Conn
	matched  chan string // receives the generated matchID
}

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

	if waiting == nil {
		ch := make(chan string, 1)
		waiting = &waitingPlayer{playerID: playerID, conn: conn, matched: ch}
		return "", true, ch
	}

	// Pair them.
	mid := fmt.Sprintf("q-%d", time.Now().UnixNano())
	waiting.matched <- mid
	waiting = nil
	return mid, false, nil
}

// Dequeue removes a player from the queue if they are the one waiting.
func Dequeue(playerID string) bool {
	mu.Lock()
	defer mu.Unlock()
	if waiting != nil && waiting.playerID == playerID {
		waiting = nil
		return true
	}
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
