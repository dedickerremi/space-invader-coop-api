// Package auth integrates Clerk session-token verification and upserts
// authenticated users into the local `users` table.
//
// Design:
//   - Verification is best-effort: when CLERK_SECRET_KEY is unset we operate
//     in guest-only mode (no tokens ever validate, everyone is a guest).
//   - Every successful verification upserts the user so downstream code
//     can rely on the row existing for FK references.
//   - Failures (invalid token, expired, misconfigured) never abort the
//     caller — they return ("", nil) and the caller treats the session
//     as a guest.
package auth

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	clerkuser "github.com/clerk/clerk-sdk-go/v2/user"
)

var enabled bool

// Init configures the Clerk SDK from CLERK_SECRET_KEY. Safe to call with
// an empty env: the package will silently operate in guest-only mode.
func Init() {
	key := os.Getenv("CLERK_SECRET_KEY")
	if key == "" {
		fmt.Println("[AUTH] CLERK_SECRET_KEY not set — auth disabled (guest-only mode)")
		return
	}
	clerk.SetKey(key)
	enabled = true
	fmt.Println("[AUTH] Clerk configured")
}

// Enabled reports whether Clerk is configured.
func Enabled() bool {
	return enabled
}

// VerifiedUser is the minimal identity we extract from a Clerk session token.
type VerifiedUser struct {
	UserID      string
	DisplayName string
}

// VerifyToken validates a Clerk session JWT. On success it returns the
// user id + a best-effort display name (falling back to the user id).
// On any failure it returns (nil, err) and the caller should fall back
// to guest mode without surfacing the error to the client.
func VerifyToken(ctx context.Context, token string) (*VerifiedUser, error) {
	if !enabled {
		return nil, fmt.Errorf("auth not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("empty token")
	}

	claims, err := jwt.Verify(ctx, &jwt.VerifyParams{Token: token})
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("token missing sub claim")
	}

	return &VerifiedUser{
		UserID:      claims.Subject,
		DisplayName: displayNameFromClaims(claims),
	}, nil
}

// FetchProfile queries the Clerk API for the user's canonical profile.
// This is a network call — use sparingly (on first sign-in or when the
// local display name is stale), not on every WebSocket message.
func FetchProfile(ctx context.Context, userID string) (string, error) {
	if !enabled {
		return "", fmt.Errorf("auth not configured")
	}
	u, err := clerkuser.Get(ctx, userID)
	if err != nil {
		return "", err
	}
	if u.Username != nil && *u.Username != "" {
		return *u.Username, nil
	}
	first := ""
	last := ""
	if u.FirstName != nil {
		first = *u.FirstName
	}
	if u.LastName != nil {
		last = *u.LastName
	}
	full := strings.TrimSpace(first + " " + last)
	if full != "" {
		return full, nil
	}
	return userID, nil
}

// displayNameFromClaims pulls the best available name off a verified
// Clerk session claim set. Clerk tokens don't always include a display
// name, so callers that need an authoritative value should call
// FetchProfile once and cache the result.
func displayNameFromClaims(c *clerk.SessionClaims) string {
	// Session claims have a minimal shape; Subject is always populated.
	return c.Subject
}
