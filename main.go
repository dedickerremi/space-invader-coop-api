package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"space-invaders-coop/backend-go/internal/game"
	"space-invaders-coop/backend-go/internal/monitoring"
	"space-invaders-coop/backend-go/internal/ws"
)

func main() {
	wsPort := getEnvInt("WS_PORT", 3001)
	monitoringPort := getEnvInt("MONITORING_PORT", 3002)

	fmt.Println("=================================")
	fmt.Println(" Space Invaders Coop - Backend (Go)")
	fmt.Println("=================================")

	hub := ws.NewHub()
	wsServer := ws.NewServer(hub)

	// Start game loop in background (broadcasts state via hub)
	go game.StartLoop(hub)

	// WebSocket HTTP server: single route, upgrade to WS
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		wsServer.HandleConnection(w, r)
	})
	go func() {
		addr := ":" + strconv.Itoa(wsPort)
		fmt.Printf("[SERVER] WebSocket server ready on port %s\n", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("[SERVER] WebSocket server: %v", err)
		}
	}()

	// Monitoring HTTP server (blocking)
	mon := monitoring.NewServer(hub, monitoringPort)
	mon.Run()
}

func getEnvInt(key string, defaultVal int) int {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}
