package monitoring

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"space-invaders-coop/backend-go/internal/db"
)

type userListRow struct {
	ID          string
	DisplayName string
	CreatedAt   time.Time
	Games       int64
	BestPoints  int64
}

type userDetail struct {
	ID          string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	HasStats    bool
	Games       int64
	TotalKills  int64
	TotalPoints int64
	BestStreak  int64
	BestWave    int64
	BestPoints  int64
	Recent      []userMatchRow
}

type userMatchRow struct {
	EndedAt    time.Time
	Mode       string
	LevelName  string
	Wave       int64
	Duration   int64
	GameOver   bool
	Points     int64
	Kills      int64
	BestStreak int64
}

// HandleUsersList renders an HTML table of authenticated users with a
// quick summary of their aggregated stats.
func (s *Server) HandleUsersList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	pool := db.Pool()
	if pool == nil {
		fmt.Fprint(w, usersPageShell("Users", `<p class="empty">Database not configured.</p>`))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT u.id, u.display_name, u.created_at,
		       COALESCE(ps.games_played, 0),
		       COALESCE(ps.best_single_match_points, 0)
		FROM users u
		LEFT JOIN player_stats ps ON ps.user_id = u.id
		ORDER BY u.created_at DESC
		LIMIT 200`)
	if err != nil {
		fmt.Fprint(w, usersPageShell("Users", fmt.Sprintf(`<p class="empty">Query failed: %s</p>`, html.EscapeString(err.Error()))))
		return
	}
	defer rows.Close()

	var list []userListRow
	for rows.Next() {
		var u userListRow
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.CreatedAt, &u.Games, &u.BestPoints); err != nil {
			continue
		}
		list = append(list, u)
	}

	var body strings.Builder
	body.WriteString(`<table><thead><tr>
  <th>Display name</th><th>Clerk ID</th><th>Games</th><th>Best score</th><th>Signed up</th>
</tr></thead><tbody>`)
	if len(list) == 0 {
		body.WriteString(`<tr><td colspan="5" class="empty">No authenticated users yet.</td></tr>`)
	}
	for _, u := range list {
		body.WriteString(fmt.Sprintf(`<tr>
  <td><a href="/users/%s">%s</a></td>
  <td class="mono">%s</td>
  <td>%d</td>
  <td>%d</td>
  <td>%s</td>
</tr>`,
			html.EscapeString(u.ID),
			html.EscapeString(u.DisplayName),
			html.EscapeString(u.ID),
			u.Games,
			u.BestPoints,
			u.CreatedAt.UTC().Format("2006-01-02 15:04")))
	}
	body.WriteString(`</tbody></table>`)
	fmt.Fprint(w, usersPageShell("Users", body.String()))
}

// HandleUserDetail renders per-user stats + recent matches.
func (s *Server) HandleUserDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	userID := strings.TrimPrefix(r.URL.Path, "/users/")
	if userID == "" || strings.Contains(userID, "/") {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	pool := db.Pool()
	if pool == nil {
		fmt.Fprint(w, usersPageShell("User", `<p class="empty">Database not configured.</p>`))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	detail, err := loadUserDetail(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, usersPageShell("User", fmt.Sprintf(`<p class="empty">Query failed: %s</p>`, html.EscapeString(err.Error()))))
		return
	}

	fmt.Fprint(w, usersPageShell("User: "+detail.DisplayName, renderUserDetail(detail)))
}

func loadUserDetail(ctx context.Context, userID string) (*userDetail, error) {
	pool := db.Pool()
	d := &userDetail{ID: userID}

	if err := pool.QueryRow(ctx,
		`SELECT display_name, created_at, updated_at FROM users WHERE id = $1`, userID,
	).Scan(&d.DisplayName, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}

	err := pool.QueryRow(ctx, `
		SELECT games_played, total_kills, total_points, best_streak, best_wave, best_single_match_points
		FROM player_stats WHERE user_id = $1`, userID,
	).Scan(&d.Games, &d.TotalKills, &d.TotalPoints, &d.BestStreak, &d.BestWave, &d.BestPoints)
	if err == nil {
		d.HasStats = true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT ms.ended_at, ms.mode, COALESCE(ms.level_name, ''), ms.wave_reached,
		       ms.duration_seconds, ms.game_over, mp.points, mp.kills, mp.best_streak
		FROM match_participants mp
		JOIN match_summaries ms ON ms.id = mp.match_id
		WHERE mp.user_id = $1
		ORDER BY ms.ended_at DESC
		LIMIT 20`, userID)
	if err != nil {
		return d, nil
	}
	defer rows.Close()
	for rows.Next() {
		var m userMatchRow
		if err := rows.Scan(&m.EndedAt, &m.Mode, &m.LevelName, &m.Wave, &m.Duration, &m.GameOver, &m.Points, &m.Kills, &m.BestStreak); err != nil {
			continue
		}
		d.Recent = append(d.Recent, m)
	}
	return d, nil
}

func renderUserDetail(d *userDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="user-head">
  <h2>%s</h2>
  <div class="meta mono">%s</div>
  <div class="meta">Signed up %s · Last seen %s</div>
</div>`,
		html.EscapeString(d.DisplayName),
		html.EscapeString(d.ID),
		d.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		d.UpdatedAt.UTC().Format("2006-01-02 15:04 UTC"))

	b.WriteString(`<div class="stats-grid">`)
	if d.HasStats {
		stats := []struct {
			label string
			value int64
		}{
			{"Games played", d.Games},
			{"Total kills", d.TotalKills},
			{"Total points", d.TotalPoints},
			{"Best streak", d.BestStreak},
			{"Best wave", d.BestWave},
			{"Best single-match points", d.BestPoints},
		}
		for _, s := range stats {
			fmt.Fprintf(&b, `<div class="stat-card"><h3>%s</h3><div class="value">%d</div></div>`, html.EscapeString(s.label), s.value)
		}
	} else {
		b.WriteString(`<div class="stat-card full"><h3>Aggregated stats</h3><div class="empty">No finished matches recorded yet.</div></div>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`<section class="recent"><h2>Recent matches</h2>`)
	if len(d.Recent) == 0 {
		b.WriteString(`<p class="empty">No matches yet.</p></section>`)
		return b.String()
	}
	b.WriteString(`<table><thead><tr>
  <th>Ended</th><th>Mode</th><th>Level</th><th>Wave</th><th>Duration</th><th>Points</th><th>Kills</th><th>Streak</th><th>Outcome</th>
</tr></thead><tbody>`)
	for _, m := range d.Recent {
		outcome := "Quit"
		if m.GameOver {
			outcome = "Game over"
		}
		fmt.Fprintf(&b, `<tr>
  <td>%s</td><td>%s</td><td class="mono">%s</td><td>%d</td><td>%ds</td><td>%d</td><td>%d</td><td>%d</td><td>%s</td>
</tr>`,
			m.EndedAt.UTC().Format("2006-01-02 15:04"),
			html.EscapeString(m.Mode),
			html.EscapeString(m.LevelName),
			m.Wave, m.Duration, m.Points, m.Kills, m.BestStreak, outcome)
	}
	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func usersPageShell(title, body string) string {
	return `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="UTF-8"><title>` + html.EscapeString(title) + ` - Space Invaders Coop</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: 'JetBrains Mono', ui-monospace, monospace; background: #0a0a0f; color: #e0e0e0; padding: 2rem; line-height: 1.6; }
  .container { max-width: 1200px; margin: 0 auto; }
  header { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 1.5rem; }
  h1 { color: #00ff88; text-transform: uppercase; letter-spacing: 0.25em; font-size: 1.25rem; }
  a.back { color: #00aaff; text-decoration: none; font-size: 0.875rem; }
  a.back:hover { text-decoration: underline; }
  table { width: 100%; border-collapse: collapse; background: #1a1a24; border: 2px solid #333; border-radius: 8px; overflow: hidden; }
  th, td { padding: 0.75rem 1rem; text-align: left; border-bottom: 1px solid #222; vertical-align: top; }
  th { color: #888; font-size: 0.75rem; text-transform: uppercase; background: #15151d; }
  td a { color: #00aaff; text-decoration: none; }
  td a:hover { text-decoration: underline; }
  .mono { font-family: inherit; color: #888; font-size: 0.8125rem; word-break: break-all; }
  .empty { text-align: center; color: #666; padding: 1.5rem; }
  .user-head { background: #1a1a24; border: 2px solid #333; border-radius: 8px; padding: 1.25rem 1.5rem; margin-bottom: 1.5rem; }
  .user-head h2 { color: #00ff88; margin-bottom: 0.5rem; }
  .user-head .meta { color: #888; font-size: 0.875rem; }
  .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 1.5rem; }
  .stat-card { background: #1a1a24; border: 2px solid #333; border-radius: 8px; padding: 1rem 1.25rem; }
  .stat-card.full { grid-column: 1 / -1; }
  .stat-card h3 { color: #888; font-size: 0.75rem; text-transform: uppercase; margin-bottom: 0.5rem; }
  .stat-card .value { color: #00ff88; font-size: 1.75rem; font-weight: bold; }
  section.recent h2 { color: #00ff88; font-size: 0.875rem; text-transform: uppercase; letter-spacing: 0.2em; margin-bottom: 0.75rem; }
</style></head><body>
<div class="container">
  <header>
    <h1>` + html.EscapeString(title) + `</h1>
    <a class="back" href="/">&larr; Back to dashboard</a>
  </header>
  ` + body + `
</div></body></html>`
}
