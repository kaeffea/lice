package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginLimiterResetsWithoutGrowingClientMap(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	limiter := newLoginRateLimiter()
	limiter.now = func() time.Time { return now }
	limiter.perClient = 2
	limiter.globalMax = 10
	limiter.maxClients = 2

	if allowed, _ := limiter.allow("one"); !allowed {
		t.Fatal("first request was denied")
	}
	if allowed, _ := limiter.allow("one"); !allowed {
		t.Fatal("second request was denied")
	}
	if allowed, retry := limiter.allow("one"); allowed || retry <= 0 {
		t.Fatalf("third request = (%v, %v), want limited", allowed, retry)
	}
	if allowed, _ := limiter.allow("two"); !allowed {
		t.Fatal("second client was denied")
	}
	if allowed, _ := limiter.allow("three"); allowed {
		t.Fatal("new client was accepted after the bounded map filled")
	}

	now = now.Add(limiter.window)
	if allowed, _ := limiter.allow("three"); !allowed {
		t.Fatal("expired windows were not pruned")
	}
	if len(limiter.clients) > limiter.maxClients {
		t.Fatalf("client map grew to %d entries", len(limiter.clients))
	}
}

func TestLoginClientKeyTrustsSanitizedHeaderOnlyFromPrivateProxy(t *testing.T) {
	request := httptest.NewRequest("GET", "http://lice.localhost/api/v1/auth/login", nil)
	request.RemoteAddr = "172.20.0.4:1234"
	request.Header.Set("X-Lice-Client-IP", "198.51.100.7")
	if got := loginClientKey(request); got != "198.51.100.7" {
		t.Fatalf("private proxy key = %q", got)
	}

	request.RemoteAddr = "203.0.113.9:1234"
	if got := loginClientKey(request); got != "203.0.113.9" {
		t.Fatalf("public peer trusted a forwarded header: %q", got)
	}
}
