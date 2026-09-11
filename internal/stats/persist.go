package stats

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"space-invaders-coop/backend-go/internal/db"
	"space-invaders-coop/backend-go/internal/types"
)

// FinalizeMatch writes a finished match to Postgres. Safe to call from a
// goroutine — noop if the pool is not configured. Errors are logged and
// swallowed: persistence is best-effort and must never interfere with
// gameplay.
func FinalizeMatch(snap *types.MatchSnapshot) {
	if snap == nil {
		return
	}
	// Loadgen self-identifies via platform=loadgen in the WS query string;
	// those matches are stress-test noise and would otherwise pile up as
	// "abandoned" rows in match_summaries.
	if snap.Metadata.Platform == "loadgen" {
		fmt.Printf("[STATS] Skipping persistence for loadgen match %s\n", snap.MatchID)
		return
	}
	pool := db.Pool()
	if pool == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	durSec := int((snap.EndedAt - snap.StartedAt) / 1000)
	if durSec < 0 {
		durSec = 0
	}
	gameOver := snap.Outcome == "defeat" || snap.Outcome == "victory"

	bosses := snap.BossesKilled
	if bosses == nil {
		bosses = []string{}
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		fmt.Printf("[STATS] begin tx failed for %s: %v\n", snap.MatchID, err)
		return
	}
	defer tx.Rollback(ctx)

	var summaryID string
	err = tx.QueryRow(ctx, `
		INSERT INTO match_summaries (
			mode, level_name, wave_reached, duration_seconds, game_over,
			started_at, outcome, bosses_killed,
			user_agent, platform, locale, country, ip_hash,
			ended_at, difficulty
		) VALUES ($1,$2,$3,$4,$5,to_timestamp($6::bigint / 1000.0),$7,$8,$9,$10,$11,$12,$13,to_timestamp($14::bigint / 1000.0),$15)
		RETURNING id`,
		snap.Mode, snap.LevelName, snap.WaveReached, durSec, gameOver,
		snap.StartedAt, snap.Outcome, bosses,
		nullIfEmpty(snap.Metadata.UserAgent),
		nullIfEmpty(snap.Metadata.Platform),
		nullIfEmpty(snap.Metadata.Locale),
		nullIfEmpty(snap.Metadata.Country),
		nullIfEmpty(snap.Metadata.IPHash),
		snap.EndedAt,
		nullIfEmpty(snap.Difficulty),
	).Scan(&summaryID)
	if err != nil {
		fmt.Printf("[STATS] insert summary failed for %s: %v\n", snap.MatchID, err)
		return
	}

	for _, p := range snap.Participants {
		if _, err := tx.Exec(ctx, `
			INSERT INTO match_participants (match_id, user_id, display_name, points, kills, best_streak, deaths)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			summaryID, nullIfEmpty(p.UserID), p.DisplayName, p.Points, p.Kills, p.BestStreak, p.Deaths,
		); err != nil {
			fmt.Printf("[STATS] insert participant failed for %s: %v\n", snap.MatchID, err)
			return
		}

		if p.UserID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO player_stats (
				user_id, games_played, total_kills, total_points,
				best_streak, best_wave, best_single_match_points, updated_at
			) VALUES ($1, 1, $2, $3, $4, $5, $3, now())
			ON CONFLICT (user_id) DO UPDATE SET
				games_played             = player_stats.games_played + 1,
				total_kills              = player_stats.total_kills + EXCLUDED.total_kills,
				total_points             = player_stats.total_points + EXCLUDED.total_points,
				best_streak              = GREATEST(player_stats.best_streak, EXCLUDED.best_streak),
				best_wave                = GREATEST(player_stats.best_wave, EXCLUDED.best_wave),
				best_single_match_points = GREATEST(player_stats.best_single_match_points, EXCLUDED.best_single_match_points),
				updated_at               = now()`,
			p.UserID, p.Kills, p.Points, p.BestStreak, snap.WaveReached,
		); err != nil {
			fmt.Printf("[STATS] upsert player_stats failed for %s (%s): %v\n", snap.MatchID, p.UserID, err)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Printf("[STATS] commit failed for %s: %v\n", snap.MatchID, err)
		return
	}
	fmt.Printf("[STATS] Persisted match %s (outcome=%s, players=%d)\n", snap.MatchID, snap.Outcome, len(snap.Participants))
}

// HashIP returns a hex SHA-256 of ip salted with IP_HASH_SALT. Empty
// input or missing salt returns "".
func HashIP(ip string) string {
	if ip == "" {
		return ""
	}
	salt := os.Getenv("IP_HASH_SALT")
	if salt == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(salt + "|" + ip))
	return hex.EncodeToString(sum[:])
}

// LookupCountry returns the ISO 3166-1 alpha-2 country code for ip, or
// "" if lookup fails / is skipped. Uses ipapi.co's free tier (no key,
// ~1000 req/day). Best-effort: 2s timeout, never errors up.
func LookupCountry(ctx context.Context, ip string) string {
	if ip == "" {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsLoopback() || parsed.IsPrivate() {
		return ""
	}
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	url := "https://ipapi.co/" + ip + "/country/"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "space-invaders-coop/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	buf, err := io.ReadAll(io.LimitReader(resp.Body, 16))
	if err != nil {
		return ""
	}
	code := strings.TrimSpace(string(buf))
	if len(code) != 2 {
		return ""
	}
	return strings.ToUpper(code)
}

// LookupCountryJSON is an alternative endpoint used if the plain-text
// form ever changes response shape. Kept unexported for now; the simple
// text endpoint above is sufficient.
func lookupCountryJSON(ctx context.Context, ip string) string {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://ipapi.co/"+ip+"/json/", nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var payload struct {
		CountryCode string `json:"country_code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}
	return strings.ToUpper(payload.CountryCode)
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
