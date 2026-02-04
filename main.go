package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"space-invaders-coop/backend-go/internal/game"
	"space-invaders-coop/backend-go/internal/monitoring"
	"space-invaders-coop/backend-go/internal/ws"
)

func main() {
	port := getEnvInt("PORT", 0)
	wsPort := getEnvInt("WS_PORT", 3001)
	monitoringPort := getEnvInt("MONITORING_PORT", 3002)

	fmt.Println("=================================")
	fmt.Println(" Space Invaders Coop - Backend (Go)")
	fmt.Println("=================================")

	hub := ws.NewHub()
	wsServer := ws.NewServer(hub)
	mon := monitoring.NewServer(hub, monitoringPort)

	// Start game loop in background (broadcasts state via hub)
	go game.StartLoop(hub)

	if port > 0 {
		// Production: single PORT (Fly, Railway, Koyeb, etc.)
		mux := http.NewServeMux()
		mux.HandleFunc("/api/stats", mon.HandleAPIStats)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
				wsServer.HandleConnection(w, r)
				return
			}
			mon.HandleDashboard(w, r)
		})
		addr := ":" + strconv.Itoa(port)
		fmt.Printf("[SERVER] Single port mode: WebSocket + monitoring on %s\n", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("[SERVER] %v", err)
		}
		return
	}

	// Development: two servers (WS on WS_PORT, monitoring on MONITORING_PORT)
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
