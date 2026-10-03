package http

import (
	"log/slog"
	"net"
	stdhttp "net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"opsflow/backend/internal/service"
)

type tokenBucket struct {
	tokens   float64
	updated  time.Time
	lastSeen time.Time
}

type ipRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	now     func() time.Time
}

func newIPRateLimiter() *ipRateLimiter {
	return &ipRateLimiter{buckets: make(map[string]*tokenBucket), now: time.Now}
}

func (l *ipRateLimiter) middleware(logger *slog.Logger) func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			strict := r.Method == stdhttp.MethodPost && r.URL.Path == "/auth/dev-login"
			key := clientIP(r) + ":api"
			capacity, refillPerSecond := 100.0, 10.0
			if strict {
				key = clientIP(r) + ":dev-login"
				capacity, refillPerSecond = 5, 1.0/12.0
			}
			allowed, retryAfter := l.allow(key, capacity, refillPerSecond)
			if !allowed {
				seconds := int(retryAfter.Seconds()) + 1
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				writeError(w, r, service.NewAppError(service.KindRateLimited, "too many requests", map[string]any{
					"retry_after_seconds": seconds,
				}), logger)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (l *ipRateLimiter) allow(key string, capacity, refillPerSecond float64) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, exists := l.buckets[key]
	if !exists {
		bucket = &tokenBucket{tokens: capacity, updated: now}
		l.buckets[key] = bucket
	}
	elapsed := now.Sub(bucket.updated).Seconds()
	if elapsed > 0 {
		bucket.tokens += elapsed * refillPerSecond
		if bucket.tokens > capacity {
			bucket.tokens = capacity
		}
		bucket.updated = now
	}
	bucket.lastSeen = now
	if len(l.buckets) > 4096 {
		cutoff := now.Add(-10 * time.Minute)
		for bucketKey, candidate := range l.buckets {
			if candidate.lastSeen.Before(cutoff) {
				delete(l.buckets, bucketKey)
			}
		}
	}
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true, 0
	}
	wait := time.Duration((1 - bucket.tokens) / refillPerSecond * float64(time.Second))
	return false, wait
}

func clientIP(r *stdhttp.Request) string {
	address, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return address
	}
	if strings.TrimSpace(r.RemoteAddr) == "" {
		return "unknown"
	}
	return r.RemoteAddr
}
