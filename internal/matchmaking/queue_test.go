package matchmaking

import (
	"testing"

	"github.com/gorilla/websocket"

	"space-invaders-coop/backend-go/internal/types"
)

func resetQueue(t *testing.T) {
	t.Helper()
	mu.Lock()
	waiting = map[string]*waitingPlayer{}
	counters = Counters{}
	events = nil
	mu.Unlock()
}

func TestPlayersOnlyPairAtTheSameDifficulty(t *testing.T) {
	resetQueue(t)
	_, waitingEasy, _, _ := Enqueue("a", types.DifficultyEasy, new(websocket.Conn))
	_, waitingHard, _, _ := Enqueue("b", types.DifficultyHard, new(websocket.Conn))
	if !waitingEasy || !waitingHard {
		t.Fatal("easy and hard players must not be paired")
	}
	if got := WaitingByDifficulty(); got[types.DifficultyEasy] != 1 || got[types.DifficultyHard] != 1 || got[types.DifficultyMedium] != 0 {
		t.Fatalf("waiting by difficulty = %v", got)
	}

	mid, isWaiting, _, _ := Enqueue("c", types.DifficultyHard, new(websocket.Conn))
	if isWaiting || mid == "" {
		t.Fatal("a second hard player should pair with the waiting one")
	}
	if QueueSize() != 1 {
		t.Fatalf("only the easy player should still wait, queue size %d", QueueSize())
	}
	snap := Snapshot()
	if len(snap.Waiting) != 1 || snap.Waiting[0].Difficulty != types.DifficultyEasy || snap.Waiting[0].PlayerID != "a" {
		t.Fatalf("snapshot waiting = %+v", snap.Waiting)
	}
}

func TestUnknownDifficultyQueuesAsEasy(t *testing.T) {
	resetQueue(t)
	Enqueue("a", "nightmare", new(websocket.Conn))
	if _, isWaiting, _, _ := Enqueue("b", types.DifficultyEasy, new(websocket.Conn)); isWaiting {
		t.Fatal("an unknown difficulty should queue as easy")
	}
}

func TestRequeueSupersedesAcrossDifficulties(t *testing.T) {
	resetQueue(t)
	_, _, _, cancelled := Enqueue("a", types.DifficultyEasy, new(websocket.Conn))
	newConn := new(websocket.Conn)
	_, isWaiting, _, _ := Enqueue("a", types.DifficultyHard, newConn)
	select {
	case <-cancelled:
	default:
		t.Fatal("the old connection should be released")
	}
	if !isWaiting {
		t.Fatal("the player must never be paired with themselves")
	}
	if got := WaitingByDifficulty(); got[types.DifficultyEasy] != 0 || got[types.DifficultyHard] != 1 {
		t.Fatalf("the player should now wait on hard only: %v", got)
	}
	if Dequeue("a", new(websocket.Conn), EventTimeout) {
		t.Fatal("a stale connection must not evict the new one")
	}
	if !Dequeue("a", newConn, EventDisconnected) || QueueSize() != 0 {
		t.Fatal("the live connection should leave the queue")
	}
}
