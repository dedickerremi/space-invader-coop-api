package match

import (
	"strings"
	"testing"
	"time"
)

func TestNewSessionAssignsIdentity(t *testing.T) {
	coop, err := NewSession("coop")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if !strings.HasPrefix(coop.PlayerID, "p_") || !strings.HasPrefix(coop.Token, "t_") {
		t.Errorf("unexpected ids: token=%q player=%q", coop.Token, coop.PlayerID)
	}
	if coop.MatchID != "" {
		t.Errorf("coop session should have no match until the matchmaker pairs it, got %q", coop.MatchID)
	}

	solo, err := NewSession("solo")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if solo.MatchID == "" {
		t.Error("solo session should get a match id immediately")
	}
}

func TestNewSessionRejectsCallerSuppliedMode(t *testing.T) {
	s, err := NewSession("admin")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if s.Mode != "coop" {
		t.Errorf("unknown mode should fall back to coop, got %q", s.Mode)
	}
}

func TestSessionsAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		s, err := NewSession("coop")
		if err != nil {
			t.Fatalf("NewSession: %v", err)
		}
		if seen[s.Token] || seen[s.PlayerID] {
			t.Fatalf("collision on iteration %d", i)
		}
		seen[s.Token], seen[s.PlayerID] = true, true
	}
}

func TestLookupSession(t *testing.T) {
	s, _ := NewSession("coop")

	if got := LookupSession(s.Token); got == nil || got.PlayerID != s.PlayerID {
		t.Error("a freshly minted token should resolve")
	}
	if LookupSession("t_deadbeef") != nil {
		t.Error("an unknown token must not resolve — this is the whole guard")
	}
	if LookupSession("") != nil {
		t.Error("an empty token must not resolve")
	}
}

func TestLookupSessionRejectsExpired(t *testing.T) {
	s, _ := NewSession("coop")

	sessMu.Lock()
	sessions[s.Token].CreatedAt = time.Now().Add(-sessionTTL - time.Second)
	sessMu.Unlock()

	if LookupSession(s.Token) != nil {
		t.Error("an aged-out session must not resolve")
	}

	sessMu.Lock()
	_, still := sessions[s.Token]
	sessMu.Unlock()
	if still {
		t.Error("looking up an expired session should drop it")
	}
}

func TestLookupSessionReturnsCopy(t *testing.T) {
	s, _ := NewSession("coop")
	got := LookupSession(s.Token)
	got.PlayerID = "p_tampered"

	if again := LookupSession(s.Token); again.PlayerID == "p_tampered" {
		t.Error("callers must not be able to mutate the stored session")
	}
}

func TestBindSessionMatch(t *testing.T) {
	s, _ := NewSession("coop")
	BindSessionMatch(s.Token, "m_abc")

	got := LookupSession(s.Token)
	if got == nil || got.MatchID != "m_abc" {
		t.Fatalf("bind did not stick: %+v", got)
	}

	// Binding must not panic or create anything for an unknown token.
	BindSessionMatch("t_nope", "m_abc")
	if LookupSession("t_nope") != nil {
		t.Error("binding an unknown token must not create a session")
	}
}

func TestDropSessionsForMatch(t *testing.T) {
	a, _ := NewSession("coop")
	b, _ := NewSession("coop")
	other, _ := NewSession("coop")
	BindSessionMatch(a.Token, "m_done")
	BindSessionMatch(b.Token, "m_done")
	BindSessionMatch(other.Token, "m_live")

	DropSessionsForMatch("m_done")

	if LookupSession(a.Token) != nil || LookupSession(b.Token) != nil {
		t.Error("sessions for the finished match should be gone")
	}
	if LookupSession(other.Token) == nil {
		t.Error("an unrelated match's session must survive")
	}

	// An empty match id would otherwise match every unbound coop session.
	unbound, _ := NewSession("coop")
	DropSessionsForMatch("")
	if LookupSession(unbound.Token) == nil {
		t.Error("dropping the empty match id must not wipe unbound sessions")
	}
}
