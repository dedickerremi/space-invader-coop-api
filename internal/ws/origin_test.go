package ws

import (
	"net/http"
	"testing"
)

func req(origin string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, "/ws", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestOriginAllowed(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		origin  string
		want    bool
	}{
		{"empty allowlist permits anything", nil, "https://evil.example", true},
		{"absent Origin is not a browser", []string{"https://game.example"}, "", true},
		{"exact match", []string{"https://game.example"}, "https://game.example", true},
		{"match ignores case", []string{"https://game.example"}, "HTTPS://GAME.EXAMPLE", true},
		{"match ignores path", []string{"https://game.example"}, "https://game.example", true},
		{"unlisted origin", []string{"https://game.example"}, "https://evil.example", false},
		{"scheme must match", []string{"https://game.example"}, "http://game.example", false},
		{"port is part of the origin", []string{"https://game.example"}, "https://game.example:8443", false},

		{"wildcard subdomain", []string{"https://*.vercel.app"}, "https://preview-abc.vercel.app", true},
		{"wildcard nested subdomain", []string{"https://*.vercel.app"}, "https://a.b.vercel.app", true},
		{"wildcard does not match the bare domain", []string{"https://*.vercel.app"}, "https://vercel.app", false},
		{"wildcard suffix keeps its dot", []string{"https://*.vercel.app"}, "https://evilvercel.app", false},
		{"wildcard still pins the scheme", []string{"https://*.vercel.app"}, "http://preview.vercel.app", false},

		{"unparseable origin", []string{"https://game.example"}, "://nope", false},
		{"origin without host", []string{"https://game.example"}, "https://", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowedOrigins = tt.allowed
			if got := originAllowed(req(tt.origin)); got != tt.want {
				t.Errorf("originAllowed(%q) with allowlist %v = %v, want %v",
					tt.origin, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	got := parseAllowedOrigins("  https://A.example , ,https://b.example,  ")
	want := []string{"https://a.example", "https://b.example"}
	if len(got) != len(want) {
		t.Fatalf("parseAllowedOrigins = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
		}
	}
	if len(parseAllowedOrigins("")) != 0 {
		t.Error("empty env should yield an empty allowlist")
	}
}
