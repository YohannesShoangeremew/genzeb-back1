package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Every per-IP rate limit rests on c.ClientIP() being something the caller
// cannot choose. Gin's default is to trust ALL proxies, in which case
// ClientIP() returns the left-most X-Forwarded-For entry — set by whoever sent
// the request. An attacker varying that header per request gets a fresh bucket
// each time and the limiter does nothing.
//
// With a trusted list, gin walks X-Forwarded-For from the RIGHT and stops at
// the first hop that is not a trusted proxy. These pin that behaviour.
func newIPEchoRouter(t *testing.T, trusted []string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := r.SetTrustedProxies(trusted); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	r.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	return r
}

func clientIP(r *gin.Engine, remoteAddr, xff string) string {
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Body.String()
}

func TestClientIPIgnoresSpoofedForwardedForFromUntrustedPeer(t *testing.T) {
	r := newIPEchoRouter(t, resolveTrustedProxies())

	// A direct caller from a public address claiming to be someone else. The
	// peer is not a trusted proxy, so its header must be disregarded entirely.
	if got := clientIP(r, "203.0.113.9:5555", "1.2.3.4"); got != "203.0.113.9" {
		t.Errorf("spoofed XFF from an untrusted peer was honoured: ClientIP = %q, want 203.0.113.9", got)
	}

	// The same attacker rotating the header must not get a new identity —
	// this is exactly the rate-limit bypass.
	first := clientIP(r, "203.0.113.9:5555", "9.9.9.9")
	second := clientIP(r, "203.0.113.9:5555", "8.8.8.8")
	if first != second {
		t.Errorf("rotating X-Forwarded-For changed the identity (%q then %q) — per-IP limits would be bypassable", first, second)
	}
}

func TestClientIPTrustsForwardedForFromPlatformProxy(t *testing.T) {
	r := newIPEchoRouter(t, resolveTrustedProxies())

	// The real deployment shape: the platform edge reaches the container over
	// private networking and appends the caller's address.
	if got := clientIP(r, "10.1.2.3:443", "203.0.113.7"); got != "203.0.113.7" {
		t.Errorf("ClientIP = %q, want the real client 203.0.113.7", got)
	}

	// A caller that pre-seeds the header before the trusted proxy appends the
	// real address: gin takes the right-most untrusted hop, so the forged
	// left-hand entry is ignored.
	if got := clientIP(r, "10.1.2.3:443", "1.2.3.4, 203.0.113.7"); got != "203.0.113.7" {
		t.Errorf("ClientIP = %q, want 203.0.113.7 — the forged left-hand entry must not win", got)
	}
}

// Guard the default list itself: losing the private ranges would silently
// collapse every caller into one bucket behind the platform proxy.
func TestResolveTrustedProxiesCoversPrivateRanges(t *testing.T) {
	got := resolveTrustedProxies()
	for _, want := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fd00::/8"} {
		found := false
		for _, p := range got {
			if p == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("default trusted proxies missing %s: %v", want, got)
		}
	}
}

func TestResolveTrustedProxiesEnvOverride(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", " 172.20.0.0/16 , 10.9.0.0/16 ")
	got := resolveTrustedProxies()
	if len(got) != 2 || got[0] != "172.20.0.0/16" || got[1] != "10.9.0.0/16" {
		t.Fatalf("env override = %v, want [172.20.0.0/16 10.9.0.0/16]", got)
	}
}
