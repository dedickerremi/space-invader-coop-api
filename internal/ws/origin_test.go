package ws

import (
	"net/http"
	"slices"
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
	saved := allowedOrigins
	t.Cleanup(func() { allowedOrigins = saved })

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
		{"match ignores path", []string{"https://game.example"}, "https://game.example/play?x=1", true},
		{"match ignores the default port", []string{"https://game.example"}, "https://game.example:443", true},
		{"unlisted origin", []string{"https://game.example"}, "https://evil.example", false},
		{"scheme must match", []string{"https://game.example"}, "http://game.example", false},
		{"port is part of the origin", []string{"https://game.example"}, "https://game.example:8443", false},
		{"path cannot smuggle an allowed host", []string{"https://game.example"}, "https://evil.example/https://game.example", false},

		{"wildcard subdomain", []string{"https://*.dedickerremi.dev"}, "https://spaceinvader.dedickerremi.dev", true},
		{"wildcard nested subdomain", []string{"https://*.dedickerremi.dev"}, "https://a.b.dedickerremi.dev", true},
		{"wildcard does not match the bare domain", []string{"https://*.dedickerremi.dev"}, "https://dedickerremi.dev", false},
		{"wildcard suffix keeps its dot", []string{"https://*.dedickerremi.dev"}, "https://evildedickerremi.dev", false},
		{"wildcard still pins the scheme", []string{"https://*.dedickerremi.dev"}, "http://spaceinvader.dedickerremi.dev", false},
		{"wildcard is not fooled by a path", []string{"https://*.dedickerremi.dev"}, "https://evil.example/.dedickerremi.dev", false},

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
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty env", "", nil},
		{"trims, lowercases and skips blanks",
			"  https://A.example , ,https://b.example,  ",
			[]string{"https://a.example", "https://b.example"}},
		{"drops path and trailing slash",
			"https://game.example/,https://game.example/play",
			[]string{"https://game.example", "https://game.example"}},
		{"drops default ports only",
			"https://a.example:443,http://b.example:80,https://c.example:8443",
			[]string{"https://a.example", "http://b.example", "https://c.example:8443"}},
		{"keeps wildcards",
			"https://*.dedickerremi.dev/",
			[]string{"https://*.dedickerremi.dev"}},
		{"ignores entries that are not origins",
			"game.example,https://ok.example,://bad",
			[]string{"https://ok.example"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseAllowedOrigins(tt.raw); !slices.Equal(got, tt.want) {
				t.Errorf("parseAllowedOrigins(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
