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

	fmt.Printf("[WS] New connection: token=%s..., matchId=%s, playerId=%s\n",
		trunc(token, 20), matchID, playerID)

	if token == "" || matchID == "" || playerID == "" {
		s.Hub.Send(conn, types.ErrorMessage{Type: "ERROR", Reason: "Missing token, matchId or playerId"})
		time.Sleep(200 * time.Millisecond)
		return
	}

	if !match.RegisterToken(token, matchID, playerID) {
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
	fmt.Printf("[WS] Player %s connected to %s (%d/2)\n", playerID, matchID, game.GetPlayerCount(matchID))

	s.Hub.Send(conn, types.WelcomeMessage{Type: "WELCOME", PlayerID: playerID, MatchID: matchID})

	// Read loop
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		result := HandleMessage(matchID, playerID, raw)
		if result.Action == "exit" {
			s.Hub.BroadcastToMatch(matchID, types.MatchEndedMessage{Type: "MATCH_ENDED", Reason: "Player left the game"})
			s.Hub.CloseMatch(matchID)
			break
		}
	}

	// On disconnect
	s.Hub.Unregister(matchID, playerID)
	remaining := s.Hub.GetConnectedCount(matchID)
	if remaining > 0 {
		s.Hub.BroadcastToMatch(matchID, types.MatchEndedMessage{Type: "MATCH_ENDED", Reason: "Opponent disconnected"})
		s.Hub.CloseMatch(matchID)
	}
	game.RemovePlayer(matchID, playerID)
	fmt.Printf("[WS] Player %s disconnected from %s (%d/2)\n", playerID, matchID, game.GetPlayerCount(matchID))
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
