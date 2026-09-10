package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"space-invaders-coop/backend-go/internal/auth"
	"space-invaders-coop/backend-go/internal/game"
	"space-invaders-coop/backend-go/internal/match"
	"space-invaders-coop/backend-go/internal/matchmaking"
	"space-invaders-coop/backend-go/internal/stats"
	"space-invaders-coop/backend-go/internal/types"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: originAllowed,
}

// allowedOrigins is the ALLOWED_ORIGINS env var split on commas, e.g.
// "https://invaders.dedickerremi.dev,https://*.vercel.app". Read once at
// startup; the process is restarted on config change.
var allowedOrigins = parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS"))

func parseAllowedOrigins(raw string) []string {
	var out []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.ToLower(strings.TrimSpace(o)); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// originAllowed reports whether a request may open a WebSocket. Without it
// gorilla accepts every Origin, so any page on the internet could open a
// socket to this backend using a visitor's browser and play as them.
//
// A missing Origin header is allowed. Only browsers send one, and the header
// exists to protect a browser user from a hostile page acting as them; a load
// generator or CLI client has no such user to protect, and blocking it would
// buy nothing an attacker could not sidestep by omitting the header from a
// non-browser client anyway.
//
// An empty allowlist also allows everything, matching the previous behaviour.
// That keeps a missing env var from taking the game offline on deploy —
// ALLOWED_ORIGINS must actually be set in production for this to bite.
func originAllowed(r *http.Request) bool {
	raw := strings.TrimSpace(r.Header.Get("Origin"))
	if raw == "" || len(allowedOrigins) == 0 {
		return true
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		log.Printf("[WS] Rejected unparseable Origin %q", raw)
		return false
	}
	origin := strings.ToLower(u.Scheme + "://" + u.Host)

	for _, a := range allowedOrigins {
		if a == origin {
			return true
		}
		// "https://*.vercel.app" matches any subdomain, which is how Vercel
		// addresses preview deployments. The dot is kept in the suffix so
		// "https://evilvercel.app" does not match "https://*.vercel.app".
		if i := strings.Index(a, "://*."); i != -1 {
			if strings.HasPrefix(origin, a[:i+3]) && strings.HasSuffix(origin, a[i+4:]) {
				return true
			}
		}
	}

	log.Printf("[WS] Rejected Origin %q (not in ALLOWED_ORIGINS)", origin)
	return false
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
	authToken := u.Get("authToken")
	matchID := u.Get("matchId")
	playerID := u.Get("playerId")
	mode := u.Get("mode")
	platform := u.Get("platform")
	if mode != "solo" && mode != "coop" {
		mode = "coop"
	}
	clientIP := clientIPFromRequest(r)

	fmt.Printf("[WS] New connection: token=%s..., authToken=%s, matchId=%s, playerId=%s, mode=%s\n",
		trunc(token, 20), boolLabel(authToken != "", "yes", "no"), matchID, playerID, mode)

	// Single reader goroutine for the whole connection lifetime. The queue
	// phase must select on queue events while still noticing disconnects —
	// but polling with SetReadDeadline poisons the connection: a gorilla
	// read timeout is permanent, and repeated reads on the failed connection
	// panic. So all reads go through this pump instead.
	handlerDone := make(chan struct{})
	defer close(handlerDone)
	type readResult struct {
		raw []byte
		err error
	}
	reads := make(chan readResult, 8)
	go func() {
		for {
			_, raw, err := conn.ReadMessage()
			select {
			case reads <- readResult{raw: raw, err: err}:
			case <-handlerDone:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	// Matchmaking queue: client sends matchId=queue (or empty) in coop mode.
	if mode == "coop" && (matchID == "" || matchID == "queue") {
		if token == "" || playerID == "" {
			s.Hub.Send(conn, types.ErrorMessage{Type: "ERROR", Reason: "Missing token or playerId"})
			time.Sleep(200 * time.Millisecond)
			return
		}

		genMatchID, isWaiting, notify, superseded := matchmaking.Enqueue(playerID, conn)
		if isWaiting {
			s.Hub.Send(conn, types.QueuedMessage{Type: "QUEUED", Position: 1})
			fmt.Printf("[QUEUE] Player %s waiting for opponent\n", playerID)

			queueTimer := time.NewTimer(matchmaking.QueueTimeout)
		waitLoop:
			for {
				select {
				case mid := <-notify:
					genMatchID = mid
					break waitLoop

				case <-superseded:
					// Another connection re-queued with this same playerID and
					// took the slot. Give it up rather than let the queue pair
					// this player with themselves.
					queueTimer.Stop()
					fmt.Printf("[QUEUE] Player %s superseded by a newer connection\n", playerID)
					s.Hub.Send(conn, types.ErrorMessage{
						Type:   "ERROR",
						Reason: "Already queued in another tab or window",
					})
					time.Sleep(200 * time.Millisecond)
					return

				case r := <-reads:
					if r.err != nil {
						queueTimer.Stop()
						fmt.Printf("[QUEUE] Player %s disconnected while waiting\n", playerID)
						matchmaking.Dequeue(playerID, conn, matchmaking.EventDisconnected)
						return
					}
					// Answer PINGs while queued so the client's connection
					// health checks keep passing; ignore everything else.
					var msg types.ClientMessage
					if json.Unmarshal(r.raw, &msg) == nil && msg.Type == "PING" && msg.Timestamp != nil {
						s.Hub.Send(conn, types.PongMessage{Type: "PONG", Timestamp: *msg.Timestamp})
					}

				case <-queueTimer.C:
					// Dequeue can lose a race with Enqueue pairing us: if it
					// returns false we were already matched and the notify is
					// guaranteed to be buffered (sent under the queue lock).
					if !matchmaking.Dequeue(playerID, conn, matchmaking.EventTimeout) {
						select {
						case mid := <-notify:
							genMatchID = mid
							break waitLoop
						default:
						}
					}
					fmt.Printf("[QUEUE] Player %s timed out waiting for opponent\n", playerID)
					s.Hub.Send(conn, types.QueueTimeoutMessage{Type: "QUEUE_TIMEOUT", Reason: "No opponent found"})
					return
				}
			}
			queueTimer.Stop()
		}

		fmt.Printf("[QUEUE] Player %s matched → %s\n", playerID, genMatchID)
		s.Hub.Send(conn, types.MatchFoundMessage{Type: "MATCH_FOUND", MatchID: genMatchID})
		matchID = genMatchID
	}

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

	match.SetMetadataIfEmpty(matchID, types.MatchMetadata{
		UserAgent: r.Header.Get("User-Agent"),
		Platform:  platform,
		Locale:    firstLocale(r.Header.Get("Accept-Language")),
		IPHash:    stats.HashIP(clientIP),
	})
	if clientIP != "" {
		go func(mid, ip string) {
			if country := stats.LookupCountry(context.Background(), ip); country != "" {
				match.SetMatchCountry(mid, country)
			}
		}(matchID, clientIP)
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

	// Best-effort Clerk auth: a failed verification leaves the player as a guest.
	if authToken != "" {
		authCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if vu, verr := auth.VerifyToken(authCtx, authToken); verr == nil && vu != nil {
			displayName, uerr := auth.UpsertUser(authCtx, vu.UserID)
			if uerr != nil {
				fmt.Printf("[WS] Upsert user %s failed: %v\n", vu.UserID, uerr)
				displayName = vu.DisplayName
			}
			game.SetPlayerAuth(matchID, playerID, vu.UserID, displayName)
			fmt.Printf("[WS] Player %s authenticated as %s (%s)\n", playerID, vu.UserID, displayName)
		} else if verr != nil {
			fmt.Printf("[WS] authToken rejected for %s: %v\n", playerID, verr)
		}
	}

	// Use SendSafe so the WELCOME write doesn't race with game loop broadcasts
	s.Hub.SendSafe(matchID, playerID, types.WelcomeMessage{Type: "WELCOME", PlayerID: playerID, MatchID: matchID, Mode: mode})

	// Read loop (messages come from the reader pump started above)
	for {
		r := <-reads
		if r.err != nil {
			fmt.Printf("[WS] Read error for %s in %s: %v\n", playerID, matchID, r.err)
			break
		}
		result := HandleMessage(matchID, playerID, r.raw)
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

func boolLabel(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

// clientIPFromRequest prefers Fly's Fly-Client-IP header (set by the
// Fly.io edge proxy), falling back to X-Forwarded-For then RemoteAddr.
func clientIPFromRequest(r *http.Request) string {
	if v := r.Header.Get("Fly-Client-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// firstLocale parses an Accept-Language header and returns the primary
// language tag (e.g. "en-US" from "en-US,en;q=0.9,fr;q=0.8"). Returns ""
// if the header is absent or malformed.
func firstLocale(header string) string {
	if header == "" {
		return ""
	}
	if i := strings.IndexByte(header, ','); i > 0 {
		header = header[:i]
	}
	if i := strings.IndexByte(header, ';'); i > 0 {
		header = header[:i]
	}
	return strings.TrimSpace(header)
}
