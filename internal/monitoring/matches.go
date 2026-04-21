package monitoring

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"space-invaders-coop/backend-go/internal/db"
)

type matchRow struct {
	ID           string
	Mode         string
	LevelName    string
	WaveReached  int
	Duration     int
	Outcome      string
	Country      string
	Platform     string
	UserAgent    string
	BossesKilled []string
	EndedAt      time.Time
	Participants []participantRow
}

type participantRow struct {
	UserID      *string
	DisplayName string
	Points      int
	Kills       int
	Deaths      int
	BestStreak  int
}

// HandleMatchesList renders a paginated table of recent matches.
func (s *Server) HandleMatchesList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	pool := db.Pool()
	if pool == nil {
		fmt.Fprint(w, matchesPageShell("Matches", `<p class="empty">Database not configured.</p>`))
		return
	}

	outcomeFilter := r.URL.Query().Get("outcome")
	modeFilter := r.URL.Query().Get("mode")
	limit := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 500 {
		limit = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	where, args := buildMatchesFilter(outcomeFilter, modeFilter)
	rows, err := pool.Query(ctx, `
		SELECT id, mode, COALESCE(level_name, ''), wave_reached, duration_seconds,
		       COALESCE(outcome, ''), COALESCE(country, ''), COALESCE(platform, ''),
		       COALESCE(user_agent, ''), COALESCE(bosses_killed, '{}'), ended_at
		FROM match_summaries
		`+where+`
		ORDER BY ended_at DESC
		LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		if isMissingRelation(err) {
			fmt.Fprint(w, matchesPageShell("Matches", `<p class="empty">match_summaries not created yet (run migrations).</p>`))
			return
		}
		fmt.Fprint(w, matchesPageShell("Matches", fmt.Sprintf(`<p class="empty">Query failed: %s</p>`, html.EscapeString(err.Error()))))
		return
	}
	defer rows.Close()

	var list []matchRow
	for rows.Next() {
		var m matchRow
		if err := rows.Scan(&m.ID, &m.Mode, &m.LevelName, &m.WaveReached, &m.Duration,
			&m.Outcome, &m.Country, &m.Platform, &m.UserAgent, &m.BossesKilled, &m.EndedAt); err != nil {
			continue
		}
		list = append(list, m)
	}

	ids := make([]string, 0, len(list))
	for _, m := range list {
		ids = append(ids, m.ID)
	}
	partByMatch := loadParticipants(ctx, ids)
	for i := range list {
		list[i].Participants = partByMatch[list[i].ID]
	}

	fmt.Fprint(w, matchesPageShell("Matches", renderMatchesList(list, outcomeFilter, modeFilter, limit)))
}

func buildMatchesFilter(outcome, mode string) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	i := 1
	if outcome == "victory" || outcome == "defeat" || outcome == "abandoned" {
		clauses = append(clauses, fmt.Sprintf("outcome = $%d", i))
		args = append(args, outcome)
		i++
	}
	if mode == "solo" || mode == "coop" {
		clauses = append(clauses, fmt.Sprintf("mode = $%d", i))
		args = append(args, mode)
		i++
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func loadParticipants(ctx context.Context, matchIDs []string) map[string][]participantRow {
	out := make(map[string][]participantRow)
	if len(matchIDs) == 0 {
		return out
	}
	pool := db.Pool()
	rows, err := pool.Query(ctx, `
		SELECT match_id::text, user_id, display_name, points, kills, COALESCE(deaths, 0), best_streak
		FROM match_participants
		WHERE match_id = ANY($1)
		ORDER BY points DESC`, matchIDs)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var mid string
		var p participantRow
		if err := rows.Scan(&mid, &p.UserID, &p.DisplayName, &p.Points, &p.Kills, &p.Deaths, &p.BestStreak); err != nil {
			continue
		}
		out[mid] = append(out[mid], p)
	}
	return out
}

func renderMatchesList(list []matchRow, outcomeFilter, modeFilter string, limit int) string {
	var b strings.Builder

	b.WriteString(`<form class="filters" method="get">`)
	b.WriteString(`<label>Outcome <select name="outcome" onchange="this.form.submit()">`)
	for _, opt := range []string{"", "victory", "defeat", "abandoned"} {
		sel := ""
		if opt == outcomeFilter {
			sel = " selected"
		}
		label := opt
		if label == "" {
			label = "All"
		}
		fmt.Fprintf(&b, `<option value="%s"%s>%s</option>`, opt, sel, label)
	}
	b.WriteString(`</select></label>`)
	b.WriteString(`<label>Mode <select name="mode" onchange="this.form.submit()">`)
	for _, opt := range []string{"", "solo", "coop"} {
		sel := ""
		if opt == modeFilter {
			sel = " selected"
		}
		label := opt
		if label == "" {
			label = "All"
		}
		fmt.Fprintf(&b, `<option value="%s"%s>%s</option>`, opt, sel, label)
	}
	b.WriteString(`</select></label>`)
	fmt.Fprintf(&b, `<label>Limit <input type="number" name="limit" min="1" max="500" value="%d" /></label>`, limit)
	b.WriteString(`<button type="submit">Apply</button>`)
	b.WriteString(`</form>`)

	if len(list) == 0 {
		b.WriteString(`<p class="empty">No matches recorded yet.</p>`)
		return b.String()
	}

	b.WriteString(`<table><thead><tr>
  <th>Ended</th><th>Mode</th><th>Level</th><th>Wave</th><th>Duration</th>
  <th>Outcome</th><th>Country</th><th>Platform</th><th>Bosses</th><th>Players</th>
</tr></thead><tbody>`)
	for _, m := range list {
		outcome := m.Outcome
		if outcome == "" {
			outcome = "—"
		}
		country := m.Country
		if country == "" {
			country = "—"
		}
		platform := m.Platform
		if platform == "" {
			platform = "—"
		}
		bosses := "—"
		if len(m.BossesKilled) > 0 {
			bosses = strings.Join(m.BossesKilled, ", ")
		}

		var playersBuf strings.Builder
		for i, p := range m.Participants {
			if i > 0 {
				playersBuf.WriteString("<br>")
			}
			tag := "guest"
			if p.UserID != nil && *p.UserID != "" {
				tag = html.EscapeString(*p.UserID)
			}
			fmt.Fprintf(&playersBuf, `<span class="player">%s <em>(%s)</em> — %d pts, %d kills, %d deaths, streak %d</span>`,
				html.EscapeString(p.DisplayName), tag, p.Points, p.Kills, p.Deaths, p.BestStreak)
		}
		if playersBuf.Len() == 0 {
			playersBuf.WriteString(`<span class="empty">No participants</span>`)
		}

		fmt.Fprintf(&b, `<tr class="outcome-%s">
  <td>%s</td>
  <td>%s</td>
  <td class="mono">%s</td>
  <td>%d</td>
  <td>%ds</td>
  <td><span class="badge %s">%s</span></td>
  <td>%s</td>
  <td>%s</td>
  <td class="mono">%s</td>
  <td class="players">%s</td>
</tr>`,
			html.EscapeString(outcome),
			m.EndedAt.UTC().Format("2006-01-02 15:04"),
			html.EscapeString(m.Mode),
			html.EscapeString(m.LevelName),
			m.WaveReached,
			m.Duration,
			html.EscapeString(outcome), html.EscapeString(outcome),
			html.EscapeString(country),
			html.EscapeString(platform),
			html.EscapeString(bosses),
			playersBuf.String())
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func matchesPageShell(title, body string) string {
	return `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="UTF-8"><title>` + html.EscapeString(title) + ` - Space Invaders Coop</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: 'JetBrains Mono', ui-monospace, monospace; background: #0a0a0f; color: #e0e0e0; padding: 2rem; line-height: 1.6; }
  .container { max-width: 1400px; margin: 0 auto; }
  header { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 1.5rem; }
  h1 { color: #00ff88; text-transform: uppercase; letter-spacing: 0.25em; font-size: 1.25rem; }
  a.back { color: #00aaff; text-decoration: none; font-size: 0.875rem; }
  a.back:hover { text-decoration: underline; }
  form.filters { display: flex; gap: 1rem; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; background: #15151d; padding: 0.75rem 1rem; border-radius: 6px; }
  form.filters label { color: #888; font-size: 0.75rem; text-transform: uppercase; display: flex; gap: 0.5rem; align-items: center; }
  form.filters select, form.filters input { background: #0a0a0f; color: #e0e0e0; border: 1px solid #333; border-radius: 4px; padding: 0.25rem 0.5rem; font-family: inherit; font-size: 0.875rem; }
  form.filters button { background: #00ff88; color: #000; border: 0; border-radius: 4px; padding: 0.35rem 1rem; font-family: inherit; font-weight: bold; cursor: pointer; text-transform: uppercase; font-size: 0.75rem; }
  table { width: 100%; border-collapse: collapse; background: #1a1a24; border: 2px solid #333; border-radius: 8px; overflow: hidden; }
  th, td { padding: 0.75rem 1rem; text-align: left; border-bottom: 1px solid #222; vertical-align: top; font-size: 0.8125rem; }
  th { color: #888; font-size: 0.75rem; text-transform: uppercase; background: #15151d; }
  .mono { color: #888; word-break: break-all; }
  .empty { text-align: center; color: #666; padding: 1.5rem; }
  .badge { display: inline-block; padding: 0.15rem 0.6rem; border-radius: 4px; font-size: 0.7rem; font-weight: bold; text-transform: uppercase; }
  .badge.victory { background: #00ff88; color: #000; }
  .badge.defeat { background: #ff4444; color: #fff; }
  .badge.abandoned { background: #ffaa00; color: #000; }
  .badge { background: #444; color: #ccc; }
  .players .player { display: inline-block; line-height: 1.4; font-size: 0.75rem; }
  .players em { color: #666; font-style: normal; }
</style></head><body>
<div class="container">
  <header>
    <h1>` + html.EscapeString(title) + `</h1>
    <a class="back" href="/">&larr; Back to dashboard</a>
  </header>
  ` + body + `
</div></body></html>`
}

func isMissingRelation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01"
	}
	return strings.Contains(strings.ToLower(err.Error()), "does not exist")
}
