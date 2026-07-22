package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginRateWindow     = time.Minute
	loginRatePerClient  = 12
	loginRateGlobal     = 300
	loginRateMaxClients = 4096
)

type rateWindow struct {
	start time.Time
	count int
}

type loginRateLimiter struct {
	mu         sync.Mutex
	now        func() time.Time
	window     time.Duration
	perClient  int
	globalMax  int
	maxClients int
	global     rateWindow
	clients    map[string]rateWindow
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		now:        time.Now,
		window:     loginRateWindow,
		perClient:  loginRatePerClient,
		globalMax:  loginRateGlobal,
		maxClients: loginRateMaxClients,
		clients:    make(map[string]rateWindow),
	}
}

func (l *loginRateLimiter) allow(client string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now().UTC()
	l.global = currentRateWindow(l.global, now, l.window)
	entry, known := l.clients[client]
	entry = currentRateWindow(entry, now, l.window)

	if !known && len(l.clients) >= l.maxClients {
		for key, candidate := range l.clients {
			if !candidate.start.IsZero() && !now.Before(candidate.start.Add(l.window)) {
				delete(l.clients, key)
			}
		}
		if len(l.clients) >= l.maxClients {
			return false, l.window
		}
	}
	if l.global.count >= l.globalMax {
		return false, remainingRateWindow(l.global, now, l.window)
	}
	if entry.count >= l.perClient {
		return false, remainingRateWindow(entry, now, l.window)
	}
	if l.global.start.IsZero() {
		l.global.start = now
	}
	if entry.start.IsZero() {
		entry.start = now
	}
	l.global.count++
	entry.count++
	l.clients[client] = entry
	return true, 0
}

func currentRateWindow(value rateWindow, now time.Time, window time.Duration) rateWindow {
	if value.start.IsZero() || !now.Before(value.start.Add(window)) {
		return rateWindow{}
	}
	return value
}

func remainingRateWindow(value rateWindow, now time.Time, window time.Duration) time.Duration {
	remaining := value.start.Add(window).Sub(now)
	if remaining <= 0 {
		return time.Second
	}
	return remaining
}

func loginClientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(strings.TrimSpace(host))
	if peer != nil && (peer.IsPrivate() || peer.IsLoopback()) {
		if forwarded := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Lice-Client-IP"))); forwarded != nil {
			return forwarded.String()
		}
	}
	if peer != nil {
		return peer.String()
	}
	return "unknown"
}
