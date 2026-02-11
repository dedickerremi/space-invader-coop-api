package ws

import (
	"encoding/json"
	"fmt"

	"space-invaders-coop/backend-go/internal/game"
	"space-invaders-coop/backend-go/internal/types"
)

// HandleResult is the result of handling a message.
type HandleResult struct {
	Action    string  // "none", "exit", or "pong"
	Timestamp float64 // for "pong": echoed client timestamp
}

// HandleMessage parses and handles a client message. Returns action "exit" if client requested exit.
func HandleMessage(matchID, playerID string, raw []byte) HandleResult {
	var msg types.ClientMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		fmt.Printf("[WS] Invalid JSON from %s: %v\n", playerID, err)
		return HandleResult{Action: "none"}
	}
	if !isValidMessage(&msg) {
		fmt.Printf("[WS] Invalid message from %s: %s\n", playerID, string(raw))
		return HandleResult{Action: "none"}
	}
	switch msg.Type {
	case "MOVE":
		dir := 0
		if msg.Dir != nil {
			dir = *msg.Dir
		}
		game.SetPlayerDirection(matchID, playerID, dir)
	case "STOP":
		game.SetPlayerDirection(matchID, playerID, 0)
	case "SHOOT":
		game.PlayerShoot(matchID, playerID)
	case "PAUSE":
		game.PauseGame(matchID, playerID)
	case "RESUME":
		game.ResumeGame(matchID, playerID)
	case "EXIT":
		return HandleResult{Action: "exit"}
	case "PING":
		return HandleResult{Action: "pong", Timestamp: *msg.Timestamp}
	}
	return HandleResult{Action: "none"}
}

func isValidMessage(msg *types.ClientMessage) bool {
	if msg == nil {
		return false
	}
	switch msg.Type {
	case "MOVE":
		return msg.Dir != nil && (*msg.Dir == -1 || *msg.Dir == 1)
	case "STOP", "SHOOT", "PAUSE", "RESUME", "EXIT":
		return true
	case "PING":
		return msg.Timestamp != nil
	default:
		return false
	}
}
