package monitoring

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"space-invaders-coop/backend-go/internal/matchmaking"
)

// HandleQueue renders the matchmaking queue page: who holds the single queue
// slot right now, lifetime outcome counters, and the recent transition log.
func (s *Server) HandleQueue(w http.ResponseWriter, r *http.Request) {
	snap := matchmaking.Snapshot()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, matchesPageShell("Matchmaking Queue", renderQueuePage(snap, time.Now())))
}

// HandleAPIQueue serves the same snapshot as JSON.
func (s *Server) HandleAPIQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(matchmaking.Snapshot())
}

// formatMillis renders a millisecond duration. Queue waits span single-digit
// milliseconds (both players already in flight) to the full 30 s deadline, so
// sub-second values keep ms precision.
func formatMillis(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	if ms < 60000 {
		return fmt.Sprintf("%.1f s", float64(ms)/1000.0)
	}
	return fmt.Sprintf("%dm %ds", ms/60000, (ms%60000)/1000)
}

// queueEventColor maps an event kind to its badge colour. Anything that stops
// a player from reaching a match reads warm; healthy transitions read cool.
func queueEventColor(k matchmaking.EventKind) string {
	switch k {
	case matchmaking.EventMatched:
		return "#00ff88"
	case matchmaking.EventEnqueued:
		return "#00aaff"
	case matchmaking.EventTimeout:
		return "#ffaa00"
	case matchmaking.EventDisconnected:
		return "#ff9500"
	case matchmaking.EventPairRace:
		return "#c08cff"
	default:
		return "#888"
	}
}

func statCard(title, value, color, sub string) string {
	if color == "" {
		color = "#00ff88"
	}
	subHTML := ""
	if sub != "" {
		subHTML = `<div class="sub">` + sub + `</div>`
	}
	return `<div class="stat-card"><h2>` + html.EscapeString(title) + `</h2>` +
		`<div class="value" style="color:` + color + `">` + value + `</div>` + subHTML + `</div>`
}

func renderQueuePage(snap matchmaking.Status, now time.Time) string {
	var b strings.Builder

	b.WriteString(`<div class="stats-grid">`)

	// The queue slot itself. Showing the deadline countdown makes a stuck
	// waiter obvious without reading logs.
	if snap.Waiting != nil {
		b.WriteString(statCard("Queue Slot", "Waiting", "#ffaa00",
			`<span class="mono">`+html.EscapeString(snap.Waiting.PlayerID)+`</span><br>`+
				`waiting `+formatMillis(snap.Waiting.WaitedMs)+
				` · expires in `+formatMillis(snap.Waiting.RemainingMs)))
	} else {
		b.WriteString(statCard("Queue Slot", "Empty", "#666",
			fmt.Sprintf("capacity 1 · %ds wait deadline", snap.TimeoutSeconds)))
	}

	c := snap.Counters
	b.WriteString(statCard("Arrivals", strconv.FormatInt(c.Arrivals, 10), "#e0e0e0",
		"players who entered the queue"))

	matchedPlayers := c.Pairs * 2
	rate := "—"
	rateColor := "#888"
	if c.Arrivals > 0 {
		pct := matchedPlayers * 100 / c.Arrivals
		rate = strconv.FormatInt(pct, 10) + "%"
		switch {
		case pct >= 80:
			rateColor = "#00ff88"
		case pct >= 50:
			rateColor = "#ffd84d"
		default:
			rateColor = "#ff4444"
		}
	}
	b.WriteString(statCard("Pairs Formed", strconv.FormatInt(c.Pairs, 10), "#00ff88",
		fmt.Sprintf("%d players matched", matchedPlayers)))
	b.WriteString(statCard("Match Rate", rate, rateColor,
		"arrivals that reached a match"))

	timeoutColor := "#666"
	if c.TimedOut > 0 {
		timeoutColor = "#ffaa00"
	}
	b.WriteString(statCard("Timed Out", strconv.FormatInt(c.TimedOut, 10), timeoutColor,
		"no opponent before deadline"))

	discColor := "#666"
	if c.Disconnected > 0 {
		discColor = "#ff9500"
	}
	b.WriteString(statCard("Disconnected", strconv.FormatInt(c.Disconnected, 10), discColor,
		"dropped while waiting"))

	raceColor := "#666"
	if c.PairRaces > 0 {
		raceColor = "#c08cff"
	}
	b.WriteString(statCard("Pair Races", strconv.FormatInt(c.PairRaces, 10), raceColor,
		"left the queue just as they matched"))

	waitSub := "no pairings yet"
	if snap.Wait.Samples > 0 {
		waitSub = fmt.Sprintf("avg %s · max %s · n=%d",
			formatMillis(snap.Wait.AvgMs), formatMillis(snap.Wait.MaxMs), snap.Wait.Samples)
	}
	lastWait := "—"
	if snap.Wait.Samples > 0 {
		lastWait = formatMillis(snap.Wait.LastMs)
	}
	b.WriteString(statCard("Last Wait To Match", lastWait, "#00aaff", waitSub))

	b.WriteString(`</div>`)

	// A waiter sitting near the full deadline is the shape of "nobody else is
	// online", which is worth distinguishing from a queue that is broken.
	if snap.Waiting != nil && snap.Waiting.WaitedMs > 10000 {
		fmt.Fprintf(&b, `<div class="banner"><strong>%s has been waiting %s.</strong> `+
			`With a single-slot queue that just means no second player has arrived yet — `+
			`they will get QUEUE_TIMEOUT in %s.</div>`,
			html.EscapeString(snap.Waiting.PlayerID),
			formatMillis(snap.Waiting.WaitedMs),
			formatMillis(snap.Waiting.RemainingMs))
	}

	b.WriteString(`<form class="filters" onsubmit="return false">
  <label><input type="checkbox" id="auto" checked> Auto-refresh 3s</label>
  <label>Snapshot <span class="mono" id="taken">` + now.UTC().Format("15:04:05") + ` UTC</span></label>
  <button type="button" onclick="location.reload()">Refresh now</button>
  <label><a class="back" href="/api/queue">&rarr; JSON</a></label>
</form>`)

	b.WriteString(`<table><thead><tr>
  <th>Time (UTC)</th><th>Age</th><th>Event</th><th>Player</th><th>Match</th><th>Waited</th><th>Detail</th>
</tr></thead><tbody>`)

	if len(snap.Events) == 0 {
		b.WriteString(`<tr><td colspan="7" class="empty">No queue activity since boot.</td></tr>`)
	}
	for _, e := range snap.Events {
		waited := "—"
		if e.WaitedMs > 0 {
			waited = formatMillis(e.WaitedMs)
		}
		matchID := "—"
		if e.MatchID != "" {
			matchID = html.EscapeString(e.MatchID)
		}
		note := "—"
		if e.Note != "" {
			note = html.EscapeString(e.Note)
		}
		age := now.Sub(e.At)
		if age < 0 {
			age = 0
		}
		fmt.Fprintf(&b, `<tr>
  <td class="mono">%s</td>
  <td style="color:#666">%s ago</td>
  <td><span class="badge" style="background:%s;color:#000">%s</span></td>
  <td class="mono">%s</td>
  <td class="mono">%s</td>
  <td>%s</td>
  <td style="color:#999">%s</td>
</tr>`,
			e.At.UTC().Format("15:04:05.000"),
			formatMillis(age.Milliseconds()),
			queueEventColor(e.Kind),
			html.EscapeString(string(e.Kind)),
			html.EscapeString(e.PlayerID),
			matchID,
			waited,
			note)
	}
	b.WriteString(`</tbody></table>`)

	// Auto-refresh is opt-out: reading a 200-row event log while the page
	// reloads under you is worse than a stale snapshot.
	b.WriteString(`<script>
(function () {
  var box = document.getElementById('auto');
  var timer = null;
  function sync() {
    if (timer) { clearInterval(timer); timer = null; }
    if (box.checked) { timer = setInterval(function () { location.reload(); }, 3000); }
  }
  box.addEventListener('change', sync);
  sync();
})();
</script>`)

	return b.String()
}
