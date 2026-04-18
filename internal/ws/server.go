package ws

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"space-invaders-coop/backend-go/internal/game"
	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/types"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Grace period before ending a match when a player disconnects.
// This allows React StrictMode re-mounts to reconnect without killing the match.
const disconnectGracePeriod = 800 * time.Millisecond

// Server is the WebSocket server.
type Server struct {
	Hub *Hub
}

// NewServer creates a new WebSocket server.
func NewServer(hub *Hub) *Server {
	return &Server{Hub: hub}
}

// HandleConnection handles a single WebSocket connection.
func (s *Server) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	u := r.URL.Query()
	token := u.Get("token")
	matchID := u.Get("matchId")
	playerID := u.Get("playerId")
	mode := u.Get("mode")
	if mode != "solo" && mode != "coop" {
		mode = "coop"
	}

	fmt.Printf("[WS] New connection: token=%s..., matchId=%s, playerId=%s, mode=%s\n",
		trunc(token, 20), matchID, playerID, mode)

	if token == "" || matchID == "" || playerID == "" {
		s.Hub.Send(conn, types.ErrorMessage{Type: "ERROR", Reason: "Missing token, matchId or playerId"})
		time.Sleep(200 * time.Millisecond)
		return
	}

	if !match.RegisterToken(token, matchID, playerID, mode) {
		s.Hub.Send(conn, types.ErrorMessage{Type: "ERROR", Reason: "Cannot join match (full or limit reached)"})
		time.Sleep(200 * time.Millisecond)
		return
	}

	m := match.GetMatch(matchID)
	if m == nil {
		s.Hub.Send(conn, types.ErrorMessage{Type: "ERROR", Reason: "Match not found"})
		time.Sleep(200 * time.Millisecond)
		return
	}

	s.Hub.Register(matchID, playerID, conn)
	game.AddPlayer(matchID, playerID)
	capacity := 2
	if mode == "solo" {
		capacity = 1
	}
	fmt.Printf("[WS] Player %s connected to %s (%d/%d)\n", playerID, matchID, game.GetPlayerCount(matchID), capacity)

	// Use SendSafe so the WELCOME write doesn't race with game loop broadcasts
	s.Hub.SendSafe(matchID, playerID, types.WelcomeMessage{Type: "WELCOME", PlayerID: playerID, MatchID: matchID, Mode: mode})

	// Read loop
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			fmt.Printf("[WS] Read error for %s in %s: %v\n", playerID, matchID, err)
			break
		}
		result := HandleMessage(matchID, playerID, raw)
		switch result.Action {
		case "exit":
			s.Hub.BroadcastToMatch(matchID, types.MatchEndedMessage{Type: "MATCH_ENDED", Reason: "Player left the game"})
			s.Hub.CloseMatch(matchID)
			goto done
		case "pong":
			// Respond to PING immediately with PONG (echo timestamp)
			s.Hub.SendSafe(matchID, playerID, types.PongMessage{Type: "PONG", Timestamp: result.Timestamp})
		}
	}
done:

	// On disconnect — only clean up if this connection is still the active one.
	remaining := s.Hub.Unregister(matchID, playerID, conn)
	if remaining == -1 {
		// Stale goroutine: a newer connection has already replaced this one. Skip cleanup.
		fmt.Printf("[WS] Stale connection for %s in %s — skipping cleanup\n", playerID, matchID)
		return
	}

	if remaining > 0 {
		// Another player is still connected. Wait a grace period before ending the match,
		// in case this player reconnects (React StrictMode double-mount).
		fmt.Printf("[WS] Player %s left %s, waiting %.0fms grace period (remaining=%d)\n",
			playerID, matchID, disconnectGracePeriod.Seconds()*1000, remaining)
		time.Sleep(disconnectGracePeriod)

		// After grace period, check if the player has reconnected
		if s.Hub.HasPlayer(matchID, playerID) {
			fmt.Printf("[WS] Player %s reconnected to %s during grace period — cancel cleanup\n", playerID, matchID)
			// Player reconnected, no need to end the match.
			// But we still need to remove the player from game state if they were re-added
			// (AddPlayer is idempotent, so the player was never removed from game state)
			return
		}

		// Player did not reconnect — end the match for the remaining player
		fmt.Printf("[WS] Grace period expired for %s in %s — ending match\n", playerID, matchID)
		s.Hub.BroadcastToMatch(matchID, types.MatchEndedMessage{Type: "MATCH_ENDED", Reason: "Opponent disconnected"})
		s.Hub.CloseMatch(matchID)
	}

	game.RemovePlayer(matchID, playerID)
	fmt.Printf("[WS] Player %s disconnected from %s (%d/%d)\n", playerID, matchID, game.GetPlayerCount(matchID), capacity)
	if game.GetPlayerCount(matchID) == 0 {
		match.RemoveMatch(matchID)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
