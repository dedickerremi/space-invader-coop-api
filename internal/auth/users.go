package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"space-invaders-coop/backend-go/internal/db"
)

// UpsertUser ensures a `users` row exists for the given Clerk user id.
// On first insert it queries Clerk for a canonical display name; on
// subsequent calls it only bumps updated_at so the Clerk API is not
// hit on every WebSocket reconnect.
func UpsertUser(ctx context.Context, userID string) (displayName string, err error) {
	if userID == "" {
		return "", fmt.Errorf("empty userID")
	}
	pool := db.Pool()
	if pool == nil {
		return "", fmt.Errorf("db not configured")
	}

	err = pool.QueryRow(ctx,
		`SELECT display_name FROM users WHERE id = $1`, userID,
	).Scan(&displayName)

	if err == nil {
		// Existing user — best-effort bump.
		_, _ = pool.Exec(ctx,
			`UPDATE users SET updated_at = now() WHERE id = $1`, userID)
		return displayName, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lookup user: %w", err)
	}

	// First time we see this user — try Clerk for a better display name.
	displayName = userID
	if name, ferr := FetchProfile(ctx, userID); ferr == nil && name != "" {
		displayName = name
	}
	if _, ierr := pool.Exec(ctx,
		`INSERT INTO users (id, display_name) VALUES ($1, $2)
		 ON CONFLICT (id) DO NOTHING`,
		userID, displayName); ierr != nil {
		return "", fmt.Errorf("insert user: %w", ierr)
	}
	return displayName, nil
}
