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
		dirStr, dirYStr := "nil", "nil"
		if msg.Dir != nil {
			dirStr = fmt.Sprintf("%d", *msg.Dir)
		}
		if msg.DirY != nil {
			dirYStr = fmt.Sprintf("%d", *msg.DirY)
		}
		fmt.Printf("[WS] MOVE from %s: dir=%s dirY=%s\n", playerID, dirStr, dirYStr)
		game.SetPlayerDirection(matchID, playerID, msg.Dir, msg.DirY)
	case "STOP":
		fmt.Printf("[WS] STOP from %s\n", playerID)
		zero := 0
		game.SetPlayerDirection(matchID, playerID, &zero, &zero)
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
		if msg.Dir == nil && msg.DirY == nil {
			return false
		}
		if msg.Dir != nil && *msg.Dir != -1 && *msg.Dir != 0 && *msg.Dir != 1 {
			return false
		}
		if msg.DirY != nil && *msg.DirY != -1 && *msg.DirY != 0 && *msg.DirY != 1 {
			return false
		}
		return true
	case "STOP", "SHOOT", "PAUSE", "RESUME", "EXIT":
		return true
	case "PING":
		return msg.Timestamp != nil
	default:
		return false
	}
}
