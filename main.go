package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"space-invaders-coop/backend-go/internal/auth"
	"space-invaders-coop/backend-go/internal/db"
	"space-invaders-coop/backend-go/internal/game"
	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/monitoring"
	"space-invaders-coop/backend-go/internal/stats"
	"space-invaders-coop/backend-go/internal/ws"
)

// Version / BuildTime are injected at build time via -ldflags "-X
// main.Version=<git sha> -X main.BuildTime=<iso8601>". See deploy.sh.
// The defaults keep /api/version honest when the binary was built
// without ldflags (local `go run`, tests).
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	port := getEnvInt("PORT", 0)
	wsPort := getEnvInt("WS_PORT", 3001)
	monitoringPort := getEnvInt("MONITORING_PORT", 3002)

	fmt.Println("=================================")
	fmt.Println(" Space Invaders Coop - Backend (Go)")
	fmt.Printf(" Version %s (built %s)\n", Version, BuildTime)
	fmt.Println("=================================")

	hub := ws.NewHub()
	wsServer := ws.NewServer(hub)
	mon := monitoring.NewServer(hub, monitoringPort)
	monitoring.SetBuildInfo(Version, BuildTime)

	ctx := context.Background()
	if err := db.Init(ctx); err != nil {
		log.Fatalf("[DB] init failed: %v", err)
	}
	defer db.Close()
	auth.Init()
	if pool := db.Pool(); pool != nil {
		game.UseDB(pool)
	}
	if err := game.SeedLevels(ctx); err != nil {
		log.Printf("[LEVELS] seed warning: %v", err)
	}

	// Register the match-end persistence callback. Fires once per match
	// (game over / victory / abandoned) from the game loop or the match
	// removal path. Best-effort: swallows DB errors.
	match.SetFinalizer(stats.FinalizeMatch)

	// Start game loop in background (broadcasts state via hub)
	go game.StartLoop(hub)
	match.StartSessionReaper()

	handleVersion := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"commit":    Version,
			"buildTime": BuildTime,
		})
	}

	handleGameMeta := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(map[string]int{
			"gameWidth":         game.GameWidth,
			"gameHeight":        game.GameHeight,
			"playerXMin":        game.PlayerXMin,
			"playerXMax":        game.PlayerXMax,
			"playerYMin":        game.PlayerYMin,
			"playerYMax":        game.PlayerYMax,
			"playerY":           game.PlayerY,
			"playerWidth":       game.PlayerWidth,
			"playerHeight":      game.PlayerHeight,
			"playerSpeed":       game.PlayerSpeed,
			"bulletSpeed":       game.BulletSpeed,
			"bulletWidth":       game.BulletWidth,
			"bulletHeight":      game.BulletHeight,
			"enemyBulletWidth":  game.EnemyBulletWidth,
			"enemyBulletHeight": game.EnemyBulletHeight,
			"enemySize":         game.EnemySize,
			"patrolSize":        game.PatrolSize,
			"powerUpSize":       game.PowerUpSize,
			"initialLives":      game.InitialLives,
		})
	}

	if port > 0 {
		// Production: single PORT (Fly, Railway, Koyeb, etc.)
		mux := http.NewServeMux()
		mux.HandleFunc("/api/session", wsServer.HandleSession)
		mux.HandleFunc("/api/version", handleVersion)
		mux.HandleFunc("/api/game-meta", handleGameMeta)
		mux.HandleFunc("/api/online", mon.HandleAPIOnline)
		mux.HandleFunc("/api/stats", monitoring.BasicAuth(mon.HandleAPIStats))
		mux.HandleFunc("/api/db-status", monitoring.BasicAuth(mon.HandleAPIDBStatus))
		mux.HandleFunc("/api/levels", monitoring.BasicAuth(monitoring.HandleLevelsList))
		mux.HandleFunc("/api/levels/", monitoring.BasicAuth(monitoring.HandleLevelByName))
		mux.HandleFunc("/api/levels/reload", monitoring.BasicAuth(monitoring.HandleLevelsReload))
		mux.HandleFunc("/editor", monitoring.BasicAuth(mon.HandleEditor))
		mux.HandleFunc("/users", monitoring.BasicAuth(mon.HandleUsersList))
		mux.HandleFunc("/users/", monitoring.BasicAuth(mon.HandleUserDetail))
		mux.HandleFunc("/matches", monitoring.BasicAuth(mon.HandleMatchesList))
		mux.HandleFunc("/queue", monitoring.BasicAuth(mon.HandleQueue))
		mux.HandleFunc("/api/queue", monitoring.BasicAuth(mon.HandleAPIQueue))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
				wsServer.HandleConnection(w, r)
				return
			}
			monitoring.BasicAuth(mon.HandleDashboard)(w, r)
		})
		addr := ":" + strconv.Itoa(port)
		fmt.Printf("[SERVER] Single port mode: WebSocket + monitoring on %s\n", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("[SERVER] %v", err)
		}
		return
	}

	// Development: two servers (WS on WS_PORT, monitoring on MONITORING_PORT)
	http.HandleFunc("/api/session", wsServer.HandleSession)
	http.HandleFunc("/api/version", handleVersion)
	http.HandleFunc("/api/game-meta", handleGameMeta)
	http.HandleFunc("/api/online", mon.HandleAPIOnline)
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
