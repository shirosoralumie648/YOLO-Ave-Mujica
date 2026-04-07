package auth

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type fixedWindowBucket struct {
	windowStart time.Time
	count       int
}

type fixedWindowLimiter struct {
	limit   int
	window  time.Duration
	now     func() time.Time
	mu      sync.Mutex
	buckets map[string]fixedWindowBucket
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{
		limit:   limit,
		window:  window,
		now:     time.Now,
		buckets: map[string]fixedWindowBucket{},
	}
}

func (l *fixedWindowLimiter) allow(key string) (bool, time.Duration) {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return true, 0
	}

	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, ok := l.buckets[key]
	if !ok || !now.Before(bucket.windowStart.Add(l.window)) {
		l.buckets[key] = fixedWindowBucket{
			windowStart: now,
			count:       1,
		}
		return true, 0
	}

	if bucket.count >= l.limit {
		retryAfter := bucket.windowStart.Add(l.window).Sub(now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}

	bucket.count++
	l.buckets[key] = bucket
	return true, 0
}

// FixedWindowRateLimitMiddleware applies a per-client fixed-window rate limit to
// the wrapped handler. Bearer tokens are used as the client key when present;
// otherwise the middleware falls back to forwarded or remote IP identity.
func FixedWindowRateLimitMiddleware(limit int, window time.Duration) func(http.Handler) http.Handler {
	limiter := newFixedWindowLimiter(limit, window)
	return func(next http.Handler) http.Handler {
		if limit <= 0 || window <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := limiter.allow(rateLimitKey(r))
			if !allowed {
				seconds := int(math.Ceil(retryAfter.Seconds()))
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func rateLimitKey(r *http.Request) string {
	if r == nil {
		return "ip:unknown"
	}
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return "bearer:" + token
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if client := strings.TrimSpace(parts[0]); client != "" {
			return "ip:" + client
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-Ip")); realIP != "" {
		return "ip:" + realIP
	}
	if host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil && host != "" {
		return "ip:" + host
	}
	if remoteAddr := strings.TrimSpace(r.RemoteAddr); remoteAddr != "" {
		return "ip:" + remoteAddr
	}
	return "ip:unknown"
}
