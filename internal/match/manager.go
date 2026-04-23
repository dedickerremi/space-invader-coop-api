package match

import (
	"fmt"
	"sync"
	"time"

	"space-invaders-coop/backend-go/internal/types"
)

const maxMatches = 50

var (
	mu           sync.RWMutex
	matches      = make(map[string]*types.Match)
	tokenToPlayer = make(map[string]struct{ MatchID, PlayerID string })

	finalizerMu sync.RWMutex
	finalizer   func(*types.MatchSnapshot)
)

// SetFinalizer registers a callback that is fired once per match when it
// ends (game over / victory / abandoned). The stats package wires this at
// boot. match is the bottom of the dependency tree so callers can import
// match without creating cycles.
func SetFinalizer(f func(*types.MatchSnapshot)) {
	finalizerMu.Lock()
	defer finalizerMu.Unlock()
	finalizer = f
}

func fireFinalizer(snap *types.MatchSnapshot) {
	finalizerMu.RLock()
	f := finalizer
	finalizerMu.RUnlock()
	if f == nil || snap == nil {
		return
	}
	go f(snap)
}

func createInitialState() types.GameState {
	return types.GameState{
		Players:           nil,
		Bullets:           nil,
		Enemies:           nil,
		Lives:             0,
		Points:            nil,
		Kills:             nil,
		WaveNumber:        0,
		Started:           false,
		Paused:            false,
		PausedBy:          nil,
		GameOver:          false,
		GameOverSummary:   nil,
		NextWaveCountdown: 0,
	}
}

// CanCreateMatch returns true if we can create a new match.
func CanCreateMatch() bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(matches) < maxMatches
}

// GetActiveMatchCount returns the number of active matches.
func GetActiveMatchCount() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(matches)
}

// RegisterToken registers a token for a player in a match, creating the match if needed.
// mode should be "solo" or "coop" (defaults to "coop" if empty).
func RegisterToken(token, matchID, playerID, mode string) bool {
	if mode == "" {
		mode = "coop"
	}
	mu.Lock()
	defer mu.Unlock()

	if _, exists := matches[matchID]; !exists {
		if len(matches) >= maxMatches {
			fmt.Println("[MATCH] Cannot create match: max limit reached")
			return false
		}
		m := &types.Match{
			MatchID:   matchID,
			PlayerIDs: nil,
			Tokens:    make(map[string]string),
			State:     createInitialState(),
			CreatedAt: time.Now().UnixMilli(),
			Mode:      mode,
		}
		matches[matchID] = m
		fmt.Printf("[MATCH] Created %s (mode=%s)\n", matchID, mode)
	}

	m := matches[matchID]
	capacity := 2
	if m.Mode == "solo" {
		capacity = 1
	}
	if len(m.PlayerIDs) >= capacity {
		found := false
		for _, id := range m.PlayerIDs {
			if id == playerID {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("[MATCH] Match %s is full\n", matchID)
			return false
		}
	}

	found := false
	for _, id := range m.PlayerIDs {
		if id == playerID {
			found = true
			break
		}
	}
	if !found {
		m.PlayerIDs = append(m.PlayerIDs, playerID)
	}
	m.Tokens[playerID] = token
	tokenToPlayer[token] = struct{ MatchID, PlayerID string }{matchID, playerID}
	fmt.Printf("[MATCH] Registered player %s in %s (%d/%d)\n", playerID, matchID, len(m.PlayerIDs), capacity)
	return true
}

// SetMetadataIfEmpty stamps client metadata onto the match the first time
// a field is set. Later joiners do not overwrite — first-joiner wins, so
// the match row reflects who started the session.
func SetMetadataIfEmpty(matchID string, meta types.MatchMetadata) {
	mu.Lock()
	defer mu.Unlock()
	m, ok := matches[matchID]
	if !ok {
		return
	}
	if m.Metadata.UserAgent != "" || m.Metadata.IPHash != "" {
		return
	}
	m.Metadata = meta
}

// SetMatchCountry fills in the country code for a match if it isn't
// already set. Called asynchronously after a best-effort GeoIP lookup so
// it shouldn't block the handshake.
func SetMatchCountry(matchID, country string) {
	if country == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	m, ok := matches[matchID]
	if !ok || m.Metadata.Country != "" {
		return
	}
	m.Metadata.Country = country
}

// MarkPersisted flags a match as already written to the DB so subsequent
// cleanup paths don't write a second row. Returns true if this call was
// the one to set the flag (caller owns the write).
func MarkPersisted(matchID string) bool {
	mu.Lock()
	defer mu.Unlock()
	m, ok := matches[matchID]
	if !ok || m.Persisted {
		return false
	}
	m.Persisted = true
	return true
}

// ValidateToken returns matchID and playerID for a token, or empty strings if invalid.
func ValidateToken(token string) (matchID, playerID string) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := tokenToPlayer[token]
	if !ok {
		return "", ""
	}
	return p.MatchID, p.PlayerID
}

// GetMatch returns the match for the given ID, or nil.
func GetMatch(matchID string) *types.Match {
	mu.RLock()
	defer mu.RUnlock()
	m, ok := matches[matchID]
	if !ok {
		return nil
	}
	return m
}

// RemoveMatch removes a match and cleans up token lookups. If the match
// was never persisted (game-over / victory did not fire the finalizer),
// an "abandoned" snapshot is dispatched before the match is dropped.
func RemoveMatch(matchID string) {
	mu.Lock()
	m, ok := matches[matchID]
	if !ok {
		mu.Unlock()
		return
	}
	var abandoned *types.MatchSnapshot
	if !m.Persisted {
		abandoned = buildSnapshotLocked(m, "abandoned")
		m.Persisted = true
	}
	for _, token := range m.Tokens {
		delete(tokenToPlayer, token)
	}
	delete(matches, matchID)
	mu.Unlock()
	fmt.Printf("[MATCH] Removed %s\n", matchID)
	if abandoned != nil {
		fireFinalizer(abandoned)
	}
}

// Snapshot builds a MatchSnapshot for persistence under the match lock.
// Returns nil if the match is unknown. Safe to call from the game loop
// before the match is removed.
func Snapshot(matchID, outcome string) *types.MatchSnapshot {
	mu.RLock()
	m, ok := matches[matchID]
	mu.RUnlock()
	if !ok {
		return nil
	}
	return buildSnapshotLocked(m, outcome)
}

// FireFinalizer dispatches the snapshot to the registered callback.
// Exported so callers (game loop) can build + fire without touching the
// package-private state directly.
func FireFinalizer(snap *types.MatchSnapshot) {
	fireFinalizer(snap)
}

func buildSnapshotLocked(m *types.Match, outcome string) *types.MatchSnapshot {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	participants := make([]types.ParticipantSnapshot, 0, len(m.State.Players))
	for _, p := range m.State.Players {
		participants = append(participants, types.ParticipantSnapshot{
			UserID:      p.UserID,
			DisplayName: p.DisplayName,
			Points:      m.State.Points[p.ID],
			Kills:       m.State.Kills[p.ID],
			Deaths:      m.State.Deaths[p.ID],
			BestStreak:  m.State.BestStreaks[p.ID],
		})
	}

	bosses := make([]string, len(m.State.BossesKilled))
	copy(bosses, m.State.BossesKilled)

	return &types.MatchSnapshot{
		MatchID:      m.MatchID,
		Mode:         m.Mode,
		Outcome:      outcome,
		StartedAt:    m.CreatedAt,
		EndedAt:      time.Now().UnixMilli(),
		LevelName:    m.State.LevelName,
		WaveReached:  m.State.WaveNumber,
		BossesKilled: bosses,
		Metadata:     m.Metadata,
		Participants: participants,
	}
}

// GetAllMatches returns a snapshot of all matches (caller must not mutate).
func GetAllMatches() []*types.Match {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]*types.Match, 0, len(matches))
	for _, m := range matches {
		out = append(out, m)
	}
	return out
}
