package matchmaking

import "time"

// Observability for the queue. The admin queue page needs more than a 0/1
// size to explain what happened to a player: whether they were paired, gave
// up at the deadline, or dropped mid-wait, and how long each took.
//
// Deliberately read-only: nothing here touches the waiting player's
// connection. A ping probe from the admin handler would be a second writer on
// that conn, which is the concurrent-write race removed in #17.
//
// All state in this file is guarded by mu (declared in queue.go). Helpers
// named *Locked assume the caller already holds it.

// maxEvents bounds the in-memory event log. The queue is a single slot, so
// transitions are low-volume; 200 covers a long session and still renders as
// one page.
const maxEvents = 200

// EventKind labels what happened to a player in the queue.
type EventKind string

const (
	EventEnqueued     EventKind = "enqueued"
	EventMatched      EventKind = "matched"
	EventTimeout      EventKind = "timeout"
	EventDisconnected EventKind = "disconnected"
	// EventPairRace is a departure that lost the race with pairing: the player
	// was already matched, so the departure was discarded.
	EventPairRace EventKind = "pair-race"
	// EventSuperseded is a waiting player displaced by a newer connection
	// using the same playerID — typically a second browser tab sharing a
	// stored playerId, or a refresh reconnecting before the old socket died.
	EventSuperseded EventKind = "superseded"
)

// Event is one queue transition.
type Event struct {
	At         time.Time `json:"at"`
	Kind       EventKind `json:"kind"`
	PlayerID   string    `json:"playerId"`
	Difficulty string    `json:"difficulty,omitempty"`
	MatchID    string    `json:"matchId,omitempty"`
	WaitedMs   int64     `json:"waitedMs"`
	Note       string    `json:"note,omitempty"`
}

// Counters are lifetime totals since process start.
type Counters struct {
	// Arrivals counts every player who entered the queue.
	Arrivals int64 `json:"arrivals"`
	// Pairs counts successful pairings; each one starts a match for 2 players.
	Pairs int64 `json:"pairs"`
	// TimedOut counts players whose wait deadline expired without a match.
	TimedOut int64 `json:"timedOut"`
	// Disconnected counts players who dropped while waiting.
	Disconnected int64 `json:"disconnected"`
	// PairRaces counts departures that lost the race with pairing. A handful is
	// normal; a lot of them means players are giving up exactly as they match.
	PairRaces int64 `json:"pairRaces"`
	// Superseded counts waiting players replaced by a newer connection reusing
	// the same playerID. Anything above the occasional refresh means clients
	// are sharing one playerId across tabs.
	Superseded int64 `json:"superseded"`
}

// WaitStats summarises how long the waiting player sat before being paired.
type WaitStats struct {
	LastMs  int64 `json:"lastMs"`
	AvgMs   int64 `json:"avgMs"`
	MaxMs   int64 `json:"maxMs"`
	Samples int64 `json:"samples"`
}

// WaitingInfo describes the player holding one difficulty's slot.
type WaitingInfo struct {
	Difficulty string    `json:"difficulty"`
	PlayerID   string    `json:"playerId"`
	Since      time.Time `json:"since"`
	WaitedMs   int64     `json:"waitedMs"`
	// RemainingMs is how long until this player's wait deadline expires.
	RemainingMs int64 `json:"remainingMs"`
}

// Status is the full queue state for the admin page and /api/queue.
type Status struct {
	Waiting        []WaitingInfo `json:"waiting"` // occupied slots, easy → hard
	Counters       Counters      `json:"counters"`
	Wait           WaitStats     `json:"wait"`
	Events         []Event       `json:"events"` // newest first
	TimeoutSeconds int           `json:"timeoutSeconds"`
}

var (
	counters   Counters
	events     []Event
	waitSumMs  int64
	waitMaxMs  int64
	waitLastMs int64
	waitCount  int64
)

// recordLocked appends one event to the log, evicting the oldest when full.
// It does not touch counters; call sites update those explicitly so a single
// pairing (which logs two events) can't double-count.
func recordLocked(e Event) {
	if len(events) < maxEvents {
		events = append(events, e)
		return
	}
	copy(events, events[1:])
	events[maxEvents-1] = e
}

// noteWaitLocked folds one wait-to-match duration into the running stats.
func noteWaitLocked(ms int64) {
	if ms < 0 {
		ms = 0
	}
	waitLastMs = ms
	waitSumMs += ms
	waitCount++
	if ms > waitMaxMs {
		waitMaxMs = ms
	}
}

// Snapshot returns the current queue state. Events are ordered newest first.
func Snapshot() Status {
	mu.Lock()
	defer mu.Unlock()

	s := Status{
		Counters:       counters,
		TimeoutSeconds: int(QueueTimeout / time.Second),
		Wait: WaitStats{
			LastMs:  waitLastMs,
			MaxMs:   waitMaxMs,
			Samples: waitCount,
		},
	}
	if waitCount > 0 {
		s.Wait.AvgMs = waitSumMs / waitCount
	}

	now := time.Now()
	s.Waiting = []WaitingInfo{}
	for _, d := range Difficulties {
		w := waiting[d]
		if w == nil {
			continue
		}
		waited := now.Sub(w.since).Milliseconds()
		s.Waiting = append(s.Waiting, WaitingInfo{
			Difficulty:  d,
			PlayerID:    w.playerID,
			Since:       w.since,
			WaitedMs:    waited,
			RemainingMs: max(0, QueueTimeout.Milliseconds()-waited),
		})
	}

	s.Events = make([]Event, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		s.Events = append(s.Events, events[i])
	}
	return s
}
