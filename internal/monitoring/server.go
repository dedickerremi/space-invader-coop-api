package monitoring

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"

	"space-invaders-coop/backend-go/internal/db"
	"space-invaders-coop/backend-go/internal/stats"
	"space-invaders-coop/backend-go/internal/ws"
)

const (
	authUser = "admin"
	authPass = "sp@c31nv@d3r"
)

// BasicAuth wraps a handler with HTTP Basic authentication.
func BasicAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(user), []byte(authUser)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(authPass)) == 1
		if !ok || !userOK || !passOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="Space Invaders Dashboard"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// Server is the HTTP monitoring server.
type Server struct {
	hub  *ws.Hub
	port int
}

// NewServer creates a new monitoring server.
func NewServer(hub *ws.Hub, port int) *Server {
	return &Server{hub: hub, port: port}
}

// Run starts the HTTP server (blocking). Used when monitoring runs on its own port.
func (s *Server) Run() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", BasicAuth(s.HandleAPIStats))
	mux.HandleFunc("/api/db-status", BasicAuth(s.HandleAPIDBStatus))
	mux.HandleFunc("/api/levels", BasicAuth(HandleLevelsList))
	mux.HandleFunc("/api/levels/", BasicAuth(HandleLevelByName))
	mux.HandleFunc("/api/levels/reload", BasicAuth(HandleLevelsReload))
	mux.HandleFunc("/editor", BasicAuth(s.HandleEditor))
	mux.HandleFunc("/users", BasicAuth(s.HandleUsersList))
	mux.HandleFunc("/users/", BasicAuth(s.HandleUserDetail))
	mux.HandleFunc("/", BasicAuth(s.HandleDashboard))
	addr := ":" + strconv.Itoa(s.port)
	fmt.Printf("[MONITORING] Dashboard available at http://localhost%s\n", addr)
	_ = http.ListenAndServe(addr, mux)
}

// HandleAPIStats serves /api/stats JSON (exported for single-port mode).
func (s *Server) HandleAPIStats(w http.ResponseWriter, r *http.Request) {
	st := stats.GetServerStats(s.hub)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// HandleDashboard serves / dashboard HTML (exported for single-port mode).
func (s *Server) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	st := stats.GetServerStats(s.hub)
	health := db.Health(r.Context())
	counts, _ := db.GetCounts(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML(st, health, counts))
}

// HandleAPIDBStatus returns a JSON snapshot of the DB health + row counts.
func (s *Server) HandleAPIDBStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	health := db.Health(ctx)
	counts, _ := db.GetCounts(ctx)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"health": health,
		"counts": counts,
	})
}

func formatAge(sec int64) string {
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%dm %ds", sec/60, sec%60)
	}
	return fmt.Sprintf("%dh %dm", sec/3600, (sec%3600)/60)
}

func formatCount(n *int64) string {
	if n == nil {
		return "—"
	}
	return strconv.FormatInt(*n, 10)
}

func dbStatusBadge(h db.HealthStatus) (label, color string) {
	switch {
	case !h.Configured:
		return "Not configured", "#888"
	case h.Connected:
		return "Connected", "#00ff88"
	default:
		return "Error", "#ff4444"
	}
}

func dashboardHTML(st stats.ServerStats, health db.HealthStatus, counts db.Counts) string {
	tableRows := ""
	for _, m := range st.Matches {
		status := "Waiting"
		if m.Paused {
			status = "Paused"
		} else if m.Started {
			status = "Started"
		}
		badgeClass := "badge waiting"
		if status == "Paused" {
			badgeClass = "badge paused"
		} else if status == "Started" {
			badgeClass = "badge started"
		}
		tableRows += fmt.Sprintf(`<tr>
  <td class="match-id">%s</td>
  <td>%d / %d</td>
  <td><span class="%s">%s</span></td>
  <td>%s</td>
</tr>`,
			html.EscapeString(m.MatchID), m.ConnectedPlayers, m.PlayerCount,
			badgeClass, status, formatAge(m.Age))
	}
	if tableRows == "" {
		tableRows = `<tr><td colspan="4" class="empty">No active matches</td></tr>`
	}
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Space Invaders Coop - Server Status</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: 'JetBrains Mono', monospace; background: #0a0a0f; color: #e0e0e0; padding: 2rem; line-height: 1.6; }
    .container { max-width: 1200px; margin: 0 auto; }
    h1 { color: #00ff88; text-transform: uppercase; letter-spacing: 0.3em; margin-bottom: 2rem; }
    .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 1.5rem; margin-bottom: 2rem; }
    .stat-card { background: #1a1a24; border: 2px solid #333; border-radius: 8px; padding: 1.5rem; }
    .stat-card h2 { color: #888; font-size: 0.875rem; text-transform: uppercase; margin-bottom: 0.5rem; }
    .stat-card .value { color: #00ff88; font-size: 2rem; font-weight: bold; }
    .matches-table { background: #1a1a24; border: 2px solid #333; border-radius: 8px; overflow: hidden; }
    .matches-table h2 { padding: 1rem 1.5rem; border-bottom: 2px solid #333; color: #00ff88; }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 1rem 1.5rem; text-align: left; border-bottom: 1px solid #333; }
    th { color: #888; font-size: 0.75rem; text-transform: uppercase; }
    .match-id { color: #00aaff; font-size: 0.875rem; }
    .badge { display: inline-block; padding: 0.25rem 0.75rem; border-radius: 4px; font-size: 0.75rem; font-weight: bold; }
    .badge.started { background: #00ff88; color: #000; }
    .badge.waiting { background: #ffaa00; color: #000; }
    .badge.paused { background: #ff4444; color: #fff; }
    .empty { text-align: center; color: #666; padding: 2rem; }
  </style>
</head>
<body>
  <div class="container">
    <h1>Space Invaders Coop - Server Status</h1>
    <p style="margin-bottom:1.5rem">
      <a href="/editor" style="color:#00aaff">&rarr; Open Level Editor</a>
      &nbsp;·&nbsp;
      <a href="/users" style="color:#00aaff">&rarr; Browse Users</a>
    </p>
    <div class="stats-grid">
      <div class="stat-card"><h2>Active Matches</h2><div class="value">` + strconv.Itoa(st.ActiveMatches) + ` / ` + strconv.Itoa(st.MaxMatches) + `</div></div>
      <div class="stat-card"><h2>Total Players</h2><div class="value">` + strconv.Itoa(st.TotalPlayers) + `</div></div>
      <div class="stat-card"><h2>Database</h2><div class="value" style="color:` + func() string { _, c := dbStatusBadge(health); return c }() + `;font-size:1.25rem">` + func() string { l, _ := dbStatusBadge(health); return html.EscapeString(l) }() + `</div>` + func() string {
			if health.Error != "" {
				return `<div style="color:#ff8888;font-size:0.75rem;margin-top:0.5rem;word-break:break-word">` + html.EscapeString(health.Error) + `</div>`
			}
			return ""
		}() + `</div>
      <div class="stat-card"><h2>Levels (DB)</h2><div class="value">` + formatCount(counts.Levels) + `</div></div>
      <div class="stat-card"><h2>Games (DB)</h2><div class="value">` + formatCount(counts.Games) + `</div></div>
      <div class="stat-card"><h2>Users (DB)</h2><div class="value">` + formatCount(counts.Users) + `</div></div>
    </div>
    <div class="matches-table">
      <h2>Active Matches</h2>
      <table><thead><tr><th>Match ID</th><th>Players</th><th>Status</th><th>Age</th></tr></thead><tbody>` +
		tableRows +
		`</tbody></table>
    </div>
  </div>
  <script>setInterval(() => location.reload(), 10000);</script>
</body>
</html>`
}
