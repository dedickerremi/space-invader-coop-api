package match

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// A Session is a WebSocket credential minted before the socket opens. It is
// the only thing the handshake trusts: the player and match ids come from
// here, never from the query string, so a client cannot name itself or walk
// into someone else's match by guessing an id.
//
// Sessions live in their own map with their own lock. Nothing here calls into
// the match manager while holding sessMu, so the two locks never nest.
type Session struct {
	Token     string
	PlayerID  string
	MatchID   string // empty until the matchmaker assigns one (coop)
	Mode      string
	CreatedAt time.Time
}

// sessionTTL bounds how long an unused session is honoured. It only has to
// cover the gap between "player pressed play" and "socket opened", plus a
// page reload — a match in progress keeps its session alive by holding the
// socket, and reconnects re-present the same token.
const sessionTTL = 15 * time.Minute

var (
	sessMu   sync.Mutex
	sessions = make(map[string]*Session)
)

func randomID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("randomID: %w", err)
	}
	return prefix + hex.EncodeToString(b), nil
}

// NewSession mints a credential for one player. mode must be "solo" or
// "coop"; solo sessions get their match id immediately because there is
// nobody to wait for, coop sessions get one from the matchmaker later.
func NewSession(mode string) (*Session, error) {
	if mode != "solo" && mode != "coop" {
		mode = "coop"
	}

	token, err := randomID("t_")
	if err != nil {
		return nil, err
	}
	playerID, err := randomID("p_")
	if err != nil {
		return nil, err
	}

	s := &Session{Token: token, PlayerID: playerID, Mode: mode, CreatedAt: time.Now()}
	if mode == "solo" {
		if s.MatchID, err = randomID("m_"); err != nil {
			return nil, err
		}
	}

	sessMu.Lock()
	sessions[token] = s
	sessMu.Unlock()
	return s, nil
}

// LookupSession returns the session for a token, or nil if it is unknown or
// has aged out. Returns a copy so callers cannot mutate the stored session.
func LookupSession(token string) *Session {
	sessMu.Lock()
	defer sessMu.Unlock()

	s, ok := sessions[token]
	if !ok {
		return nil
	}
	if time.Since(s.CreatedAt) > sessionTTL {
		delete(sessions, token)
		return nil
	}
	cp := *s
	return &cp
}

// BindSessionMatch records the match the matchmaker assigned to a coop
// session, so a reconnect with the same token lands back in the same match.
func BindSessionMatch(token, matchID string) {
	sessMu.Lock()
	defer sessMu.Unlock()
	if s, ok := sessions[token]; ok {
		s.MatchID = matchID
		s.CreatedAt = time.Now() // the session is in use; keep it from ageing out mid-match
	}
}

// DropSessionsForMatch removes every session pointing at a finished match.
func DropSessionsForMatch(matchID string) {
	if matchID == "" {
		return
	}
	sessMu.Lock()
	defer sessMu.Unlock()
	for token, s := range sessions {
		if s.MatchID == matchID {
			delete(sessions, token)
		}
	}
}

// SessionCount reports how many sessions are held, for the dashboard.
func SessionCount() int {
	sessMu.Lock()
	defer sessMu.Unlock()
	return len(sessions)
}

// StartSessionReaper drops aged-out sessions that never opened a socket.
// Without it an unused session would sit in the map until the process
// restarted, which is a slow leak on a long-lived machine.
func StartSessionReaper() {
	go func() {
		for range time.Tick(time.Minute) {
			cutoff := time.Now().Add(-sessionTTL)
			sessMu.Lock()
			for token, s := range sessions {
				if s.CreatedAt.Before(cutoff) {
					delete(sessions, token)
				}
			}
			sessMu.Unlock()
		}
	}()
}
