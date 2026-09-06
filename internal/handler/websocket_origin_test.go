package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The upgrader used to accept every origin unconditionally, so any page a
// player visited could open a socket to a game. These pin the replacement:
// allowlisted origins connect, everything else is refused, and clients that
// send no Origin at all (native apps, curl, uptime probes) still work.
func TestWebSocketCheckOrigin(t *testing.T) {
	allowed := []string{
		"https://miniapp-production-f267.up.railway.app",
		"http://localhost:3000",
	}
	check := newUpgrader(allowed).CheckOrigin

	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{"allowlisted production origin", "https://miniapp-production-f267.up.railway.app", true},
		{"allowlisted localhost", "http://localhost:3000", true},
		{"case-insensitive host", "HTTPS://MiniApp-Production-F267.up.railway.app", true},
		{"absent origin (non-browser client)", "", true},
		{"unrelated site", "https://evil.example", false},
		{"scheme mismatch", "http://miniapp-production-f267.up.railway.app", false},
		{"port mismatch", "http://localhost:3001", false},
		// A prefix/suffix match would let an attacker register a lookalike
		// domain; only whole-string equality is acceptable here.
		{"suffix lookalike", "https://evil-miniapp-production-f267.up.railway.app", false},
		{"prefix lookalike", "https://miniapp-production-f267.up.railway.app.evil.example", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws/game?type=REGULAR", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if got := check(req); got != tc.want {
				t.Fatalf("CheckOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

// An empty allowlist must not degrade into "allow everything" — a
// misconfigured ALLOWED_ORIGINS should fail closed for browsers.
func TestWebSocketCheckOriginEmptyAllowlistFailsClosed(t *testing.T) {
	check := newUpgrader(nil).CheckOrigin
	req := httptest.NewRequest(http.MethodGet, "/ws/game", nil)
	req.Header.Set("Origin", "https://anything.example")
	if check(req) {
		t.Fatal("empty allowlist accepted a browser origin; it must fail closed")
	}
}
