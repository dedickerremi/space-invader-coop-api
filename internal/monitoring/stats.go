package monitoring

import (
	"time"

	"space-invaders-coop/backend-go/internal/game"
	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/ws"
)

const maxMatches = 10

// MatchInfo is one match's live stats for the dashboard.
type MatchInfo struct {
	MatchID          string `json:"matchId"`
	PlayerCount      int    `json:"playerCount"`
	ConnectedPlayers int    `json:"connectedPlayers"`
	Started          bool   `json:"started"`
	Paused           bool   `json:"paused"`
	CreatedAt        int64  `json:"createdAt"`
	Age              int64  `json:"age"` // seconds
}

// ServerStats is the full live-dashboard response.
type ServerStats struct {
	ActiveMatches int              `json:"activeMatches"`
	MaxMatches    int              `json:"maxMatches"`
	TotalPlayers  int              `json:"totalPlayers"`
	Matches       []MatchInfo      `json:"matches"`
	Loop          game.LoopMetrics `json:"loop"`
}

// GetServerStats returns current in-memory server statistics.
func GetServerStats(hub *ws.Hub) ServerStats {
	all := match.GetAllMatches()
	totalPlayers := 0
	matches := make([]MatchInfo, 0, len(all))
	now := time.Now().UnixMilli()
	for _, m := range all {
		connected := hub.GetConnectedCount(m.MatchID)
		totalPlayers += connected
		age := (now - m.CreatedAt) / 1000
		matches = append(matches, MatchInfo{
			MatchID:          m.MatchID,
			PlayerCount:      len(m.State.Players),
			ConnectedPlayers: connected,
			Started:          m.State.Started,
			Paused:           m.State.Paused,
			CreatedAt:        m.CreatedAt,
			Age:              age,
		})
	}
	return ServerStats{
		ActiveMatches: match.GetActiveMatchCount(),
		MaxMatches:    maxMatches,
		TotalPlayers:  totalPlayers,
		Matches:       matches,
		Loop:          game.GetMetrics(),
	}
}
