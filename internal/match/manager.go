package match

import (
	"fmt"
	"sync"
	"time"

	"space-invaders-coop/backend-go/internal/types"
)

const maxMatches = 10

var (
	mu           sync.RWMutex
	matches      = make(map[string]*types.Match)
	tokenToPlayer = make(map[string]struct{ MatchID, PlayerID string })
)

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

// RemoveMatch removes a match and cleans up token lookups.
func RemoveMatch(matchID string) {
	mu.Lock()
	defer mu.Unlock()
	m, ok := matches[matchID]
	if !ok {
		return
	}
	for _, token := range m.Tokens {
		delete(tokenToPlayer, token)
	}
	delete(matches, matchID)
	fmt.Printf("[MATCH] Removed %s\n", matchID)
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
