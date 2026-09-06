package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimitRule is one bucket: at most Limit requests per Window.
type RateLimitRule struct {
	Limit  int
	Window time.Duration
}

// Unlimited reports whether the rule is disabled (limit <= 0), which is how an
// operator switches a bucket off from the environment without a deploy.
func (r RateLimitRule) Unlimited() bool { return r.Limit <= 0 }

// RateLimit throttles a route to rule.Limit requests per rule.Window.
//
// IDENTITY. Authenticated requests are keyed on the user id, unauthenticated
// ones on the client IP. Keying on the user wherever possible is not a nicety
// here: Ethiopian mobile carriers put large numbers of subscribers behind
// carrier-grade NAT, so many genuine players share one public address. A
// per-IP limit on a player-facing endpoint would throttle a whole carrier the
// moment one person was busy. So this middleware must be registered AFTER
// AuthMiddleware on protected routes, which is where the user id comes from.
//
// STORAGE. A Redis fixed-window counter (INCR, EXPIRE on first hit), so the
// limit holds across restarts and across instances if the service is ever
// scaled out. Redis is already a hard startup dependency for this service.
//
// FAILURE MODE. Fails OPEN. If Redis errors the request is allowed and the
// problem logged — a rate limiter that turns a Redis blip into a total outage
// is worse than the abuse it prevents. The tradeoff is that abuse is
// unthrottled during a Redis outage, which is the right way round for a game
// people are mid-round in.
func RateLimit(rdb *redis.Client, bucket string, rule RateLimitRule) gin.HandlerFunc {
	if rule.Unlimited() {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		identity := "ip:" + c.ClientIP()
		if v, ok := c.Get(UserIDKey); ok {
			if s := fmt.Sprintf("%v", v); s != "" {
				identity = "u:" + s
			}
		}

		// Fixed window: the key carries the window index, so it rolls over on
		// its own and expiry is a backstop rather than the mechanism.
		windowIdx := time.Now().UnixNano() / int64(rule.Window)
		key := fmt.Sprintf("rl:%s:%s:%d", bucket, identity, windowIdx)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
		defer cancel()

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			log.Printf("[ratelimit] %s: redis error, allowing request: %v", bucket, err)
			c.Next()
			return
		}
		if count == 1 {
			// Best-effort: a missed expiry only means the key lingers until the
			// next window, and the window index already moved on.
			if err := rdb.Expire(ctx, key, rule.Window+time.Second).Err(); err != nil {
				log.Printf("[ratelimit] %s: could not set expiry: %v", bucket, err)
			}
		}

		if count > int64(rule.Limit) {
			retryAfter := int(rule.Window.Seconds())
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Header("X-RateLimit-Limit", strconv.Itoa(rule.Limit))
			c.Header("X-RateLimit-Remaining", "0")
			log.Printf("[ratelimit] %s: %s exceeded %d/%s", bucket, identity, rule.Limit, rule.Window)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "too many requests, please slow down",
				"retry_after": retryAfter,
			})
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(rule.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(rule.Limit-int(count)))
		c.Next()
	}
}
