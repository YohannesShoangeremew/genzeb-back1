package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// testRedis returns a client against the dev Redis, skipping if unreachable so
// the suite still runs on a machine without it.
func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{
		Addr:     env("REDIS_HOST", "127.0.0.1") + ":" + env("REDIS_PORT", "6379"),
		Password: os.Getenv("REDIS_PASSWORD"),
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("no redis reachable (%v) — skipping", err)
	}
	return rdb
}

// newTestRouter wires one rate-limited route. If userID is non-empty the
// request is treated as authenticated, mirroring AuthMiddleware having run
// before the limiter.
func newTestRouter(rdb *redis.Client, bucket string, rule RateLimitRule, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/x", func(c *gin.Context) {
		if userID != "" {
			c.Set(UserIDKey, userID)
		}
		c.Next()
	}, RateLimit(rdb, bucket, rule), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func do(r *gin.Engine, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.RemoteAddr = ip + ":12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The Nth+1 request in a window is refused, and the refusal tells the caller
// when to come back.
func TestRateLimitBlocksOverLimit(t *testing.T) {
	rdb := testRedis(t)
	bucket := "test-block-" + uuid.NewString()
	r := newTestRouter(rdb, bucket, RateLimitRule{Limit: 3, Window: time.Minute}, "")

	for i := 1; i <= 3; i++ {
		if w := do(r, "10.1.1.1"); w.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, w.Code)
		}
	}
	w := do(r, "10.1.1.1")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request: got %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 response is missing Retry-After")
	}
}

// Different callers must not share a bucket — otherwise one busy player
// throttles everyone.
func TestRateLimitIsolatesIdentities(t *testing.T) {
	rdb := testRedis(t)
	bucket := "test-isolate-" + uuid.NewString()
	r := newTestRouter(rdb, bucket, RateLimitRule{Limit: 2, Window: time.Minute}, "")

	for i := 0; i < 2; i++ {
		do(r, "10.2.2.2")
	}
	if w := do(r, "10.2.2.2"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("first IP should be limited, got %d", w.Code)
	}
	if w := do(r, "10.3.3.3"); w.Code != http.StatusOK {
		t.Fatalf("second IP must have its own bucket, got %d", w.Code)
	}
}

// This is the property that keeps carrier NAT from breaking the game: two
// users arriving from the SAME IP are limited separately once authenticated.
func TestRateLimitKeysOnUserNotIPWhenAuthenticated(t *testing.T) {
	rdb := testRedis(t)
	bucket := "test-user-" + uuid.NewString()
	sharedIP := "10.4.4.4"

	userA := newTestRouter(rdb, bucket, RateLimitRule{Limit: 2, Window: time.Minute}, "user-aaa")
	userB := newTestRouter(rdb, bucket, RateLimitRule{Limit: 2, Window: time.Minute}, "user-bbb")

	for i := 0; i < 2; i++ {
		do(userA, sharedIP)
	}
	if w := do(userA, sharedIP); w.Code != http.StatusTooManyRequests {
		t.Fatalf("user A should be limited, got %d", w.Code)
	}
	// Same IP, different user — must be unaffected.
	if w := do(userB, sharedIP); w.Code != http.StatusOK {
		t.Fatalf("user B shares an IP with a limited user but must have its own bucket, got %d", w.Code)
	}
}

// A new window starts a fresh allowance.
func TestRateLimitWindowRollover(t *testing.T) {
	rdb := testRedis(t)
	bucket := "test-roll-" + uuid.NewString()
	r := newTestRouter(rdb, bucket, RateLimitRule{Limit: 1, Window: time.Second}, "")

	if w := do(r, "10.5.5.5"); w.Code != http.StatusOK {
		t.Fatalf("first request: got %d", w.Code)
	}
	if w := do(r, "10.5.5.5"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("second request in window: got %d, want 429", w.Code)
	}
	time.Sleep(1100 * time.Millisecond)
	if w := do(r, "10.5.5.5"); w.Code != http.StatusOK {
		t.Fatalf("after the window rolled over: got %d, want 200", w.Code)
	}
}

// A Redis outage must not take the API down with it. Abuse going unthrottled
// for the duration is the lesser harm versus refusing every player mid-round.
func TestRateLimitFailsOpenWhenRedisUnavailable(t *testing.T) {
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}) // nothing listening
	r := newTestRouter(dead, "test-dead", RateLimitRule{Limit: 1, Window: time.Minute}, "")

	for i := 1; i <= 3; i++ {
		if w := do(r, "10.6.6.6"); w.Code != http.StatusOK {
			t.Fatalf("request %d with redis down: got %d, want 200 (fail open)", i, w.Code)
		}
	}
}

// Limit 0 disables the bucket, so an operator can switch one off from the
// environment without a deploy.
func TestRateLimitZeroLimitDisables(t *testing.T) {
	rdb := testRedis(t)
	r := newTestRouter(rdb, "test-off-"+uuid.NewString(), RateLimitRule{Limit: 0, Window: time.Minute}, "")
	for i := 1; i <= 25; i++ {
		if w := do(r, "10.7.7.7"); w.Code != http.StatusOK {
			t.Fatalf("request %d with limit disabled: got %d, want 200", i, w.Code)
		}
	}
}

// The headers a client needs to back off politely.
func TestRateLimitReportsRemaining(t *testing.T) {
	rdb := testRedis(t)
	r := newTestRouter(rdb, "test-hdr-"+uuid.NewString(), RateLimitRule{Limit: 3, Window: time.Minute}, "")
	for i, want := range []string{"2", "1", "0"} {
		w := do(r, "10.8.8.8")
		if got := w.Header().Get("X-RateLimit-Remaining"); got != want {
			t.Errorf("request %d: X-RateLimit-Remaining = %q, want %q", i+1, got, want)
		}
	}
}
