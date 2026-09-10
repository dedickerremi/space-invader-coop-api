package ws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/ratelimit"
)

// sessionLimiter caps how many sessions one address can mint. This is the
// single front door to the game — a socket cannot be opened without coming
// through here first — so it is the one place a limit is worth having.
// Generous enough that a household behind one NAT address playing all evening
// never notices.
var sessionLimiter = ratelimit.New(30, time.Minute)

type sessionRequest struct {
	Mode string `json:"mode"`
}

type sessionResponse struct {
	Token    string `json:"token"`
	PlayerID string `json:"playerId"`
	MatchID  string `json:"matchId,omitempty"`
	Mode     string `json:"mode"`
}

// HandleSession mints a WebSocket credential. The client never chooses its own
// player id or match id; it gets what this server assigns and presents the
// token back on the socket.
//
// No CORS headers: the frontend proxies this from its own server, so the
// browser never calls it cross-origin.
func (s *Server) HandleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if ip := clientIPFromRequest(r); !sessionLimiter.Allow(ip) {
		fmt.Printf("[SESSION] Rate limited %s\n", ip)
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	var req sessionRequest
	// A missing or unparseable body is fine; mode defaults to coop.
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req)

	sess, err := match.NewSession(req.Mode)
	if err != nil {
		fmt.Printf("[SESSION] Mint failed: %v\n", err)
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}

	fmt.Printf("[SESSION] Minted %s for mode=%s\n", sess.PlayerID, sess.Mode)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sessionResponse{
		Token:    sess.Token,
		PlayerID: sess.PlayerID,
		MatchID:  sess.MatchID,
		Mode:     sess.Mode,
	})
}
