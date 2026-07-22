package postgres

import (
	"testing"
	"time"
)

func TestSessionExpirationReasonUsesClosedDeadlines(t *testing.T) {
	base := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	idle := base.Add(30 * time.Minute)
	absolute := base.Add(8 * time.Hour)
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{name: "before idle", now: idle.Add(-time.Nanosecond), want: ""},
		{name: "exact idle", now: idle, want: "idle_timeout"},
		{name: "after idle", now: idle.Add(time.Second), want: "idle_timeout"},
		{name: "exact absolute takes precedence", now: absolute, want: "absolute_timeout"},
		{name: "after absolute", now: absolute.Add(time.Second), want: "absolute_timeout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sessionExpirationReason(test.now, idle, absolute); got != test.want {
				t.Fatalf("sessionExpirationReason() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNextIdleExpiryNeverExceedsAbsoluteDeadline(t *testing.T) {
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	absolute := now.Add(10 * time.Minute)
	if got := nextIdleExpiry(now, 30*time.Minute, absolute); !got.Equal(absolute) {
		t.Fatalf("nextIdleExpiry() = %v, want absolute deadline %v", got, absolute)
	}
}
