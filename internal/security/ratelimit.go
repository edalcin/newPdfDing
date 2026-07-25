package security

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginMaxFailures = 5
	loginLockout     = 30 * time.Minute
)

type ipState struct {
	failures int
	lockedAt time.Time
}

// LoginThrottle tracks failed-login attempts per IP and enforces a
// 30-minute lockout after 5 consecutive failures (ver 08-seguranca.md,
// "Rate limiting"). State is in-memory only and reset on process restart —
// an accepted, documented limitation, not a gap to fix.
type LoginThrottle struct {
	mu         sync.Mutex
	states     map[string]*ipState
	trustProxy bool
}

// NewLoginThrottle creates a throttle. trustProxy should be cfg.TrustProxyHeaders.
func NewLoginThrottle(trustProxy bool) *LoginThrottle {
	t := &LoginThrottle{states: make(map[string]*ipState), trustProxy: trustProxy}
	go t.sweepLoop()
	return t
}

// Allow reports whether the request's IP is not currently locked out.
func (t *LoginThrottle) Allow(r *http.Request) bool {
	ip := t.realIP(r)
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.states[ip]
	if !ok || st.failures < loginMaxFailures {
		return true
	}
	return time.Since(st.lockedAt) >= loginLockout
}

// RecordFailure increments the failure counter for the request's IP. The
// 5th consecutive failure starts the lockout.
func (t *LoginThrottle) RecordFailure(r *http.Request) {
	ip := t.realIP(r)
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.states[ip]
	if !ok {
		st = &ipState{}
		t.states[ip] = st
	} else if st.failures >= loginMaxFailures && time.Since(st.lockedAt) >= loginLockout {
		st.failures = 0 // previous lockout expired — start counting fresh
	}
	st.failures++
	if st.failures >= loginMaxFailures {
		st.lockedAt = time.Now()
	}
}

// RecordSuccess clears the failure counter for the request's IP.
func (t *LoginThrottle) RecordSuccess(r *http.Request) {
	ip := t.realIP(r)
	t.mu.Lock()
	delete(t.states, ip)
	t.mu.Unlock()
}

// realIP returns the client IP, honouring X-Forwarded-For when trustProxy
// is enabled (ver 08-seguranca.md / 07-docker-ci-deploy.md, TRUST_PROXY_HEADERS).
func (t *LoginThrottle) realIP(r *http.Request) string {
	if t.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// sweepLoop purges expired lockouts every minute to bound memory growth.
func (t *LoginThrottle) sweepLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		t.mu.Lock()
		for ip, st := range t.states {
			if st.failures < loginMaxFailures || time.Since(st.lockedAt) >= loginLockout {
				delete(t.states, ip)
			}
		}
		t.mu.Unlock()
	}
}
