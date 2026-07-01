package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginAttemptWindow tracks failed login attempts in a sliding time window.
type loginAttemptWindow struct {
	hits     []time.Time
	blocked  bool
	blockEnd time.Time
}

// loginRateLimiter is a small, in-memory, per-key sliding-window limiter used
// to slow down brute-force login attempts. It is intentionally simple: it is
// process-local, has no persistence, and resets on backend restart.
type loginRateLimiter struct {
	mu          sync.Mutex
	perKey      map[string]*loginAttemptWindow
	window      time.Duration
	maxPerKey   int
	maxPerIP    int
	blockFor    time.Duration
	idleEvict   time.Duration
	lastSweepAt time.Time
}

func newLoginRateLimiter(maxPerKey, maxPerIP int, window, blockFor time.Duration) *loginRateLimiter {
	if maxPerKey <= 0 {
		maxPerKey = 10
	}
	if maxPerIP <= 0 {
		maxPerIP = maxPerKey * 5
	}
	if window <= 0 {
		window = time.Minute
	}
	if blockFor <= 0 {
		blockFor = 5 * time.Minute
	}
	return &loginRateLimiter{
		perKey:    make(map[string]*loginAttemptWindow),
		window:    window,
		maxPerKey: maxPerKey,
		maxPerIP:  maxPerIP,
		blockFor:  blockFor,
		idleEvict: 30 * time.Minute,
	}
}

// recordFailure records a failed login attempt for (ip, loginKey) and returns
// true if the caller is now over the limit and should be blocked.
func (l *loginRateLimiter) recordFailure(ip, loginKey string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked(time.Now())

	key := ip + "|" + strings.ToLower(strings.TrimSpace(loginKey))
	w := l.perKey[key]
	if w == nil {
		w = &loginAttemptWindow{}
		l.perKey[key] = w
	}
	now := time.Now()
	if w.blocked && now.Before(w.blockEnd) {
		return true
	}
	if w.blocked && !now.Before(w.blockEnd) {
		w.blocked = false
		w.hits = w.hits[:0]
	}
	w.hits = append(w.hits, now)
	// Drop hits outside the window.
	cutoff := now.Add(-l.window)
	pruned := w.hits[:0]
	for _, t := range w.hits {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	w.hits = pruned
	if len(w.hits) >= l.maxPerKey {
		w.blocked = true
		w.blockEnd = now.Add(l.blockFor)
		return true
	}
	// Also enforce per-IP ceiling (sum across all login keys for this IP).
	ipCount := 0
	for k, other := range l.perKey {
		if !strings.HasPrefix(k, ip+"|") {
			continue
		}
		for _, t := range other.hits {
			if t.After(cutoff) {
				ipCount++
			}
		}
	}
	if ipCount >= l.maxPerIP {
		w.blocked = true
		w.blockEnd = now.Add(l.blockFor)
		return true
	}
	return false
}

// recordSuccess clears the limiter state for a given key.
func (l *loginRateLimiter) recordSuccess(ip, loginKey string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := ip + "|" + strings.ToLower(strings.TrimSpace(loginKey))
	delete(l.perKey, key)
}

// isBlocked reports whether the key is currently blocked without recording a
// new failure. Useful to short-circuit before looking up the user.
func (l *loginRateLimiter) isBlocked(ip, loginKey string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked(time.Now())
	key := ip + "|" + strings.ToLower(strings.TrimSpace(loginKey))
	w, ok := l.perKey[key]
	if !ok {
		return false
	}
	now := time.Now()
	if w.blocked && now.Before(w.blockEnd) {
		return true
	}
	if w.blocked && !now.Before(w.blockEnd) {
		w.blocked = false
		w.hits = w.hits[:0]
	}
	return false
}

// sweepLocked prunes idle entries to keep the map bounded.
func (l *loginRateLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweepAt) < 5*time.Minute {
		return
	}
	l.lastSweepAt = now
	cutoff := now.Add(-l.idleEvict)
	for k, w := range l.perKey {
		if w.blocked && now.Before(w.blockEnd) {
			continue
		}
		if len(w.hits) == 0 {
			delete(l.perKey, k)
			continue
		}
		last := w.hits[len(w.hits)-1]
		if last.Before(cutoff) {
			delete(l.perKey, k)
		}
	}
}

// clientIP extracts the best-effort IP address for a request. It honors a
// trusted proxy's X-Forwarded-For first hop, otherwise falls back to the
// peer address. Host is stripped.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := strings.IndexByte(xff, ','); comma >= 0 {
			xff = xff[:comma]
		}
		xff = strings.TrimSpace(xff)
		if xff != "" {
			return xff
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}