package audit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	want := Cursor{
		OccurredAt: time.Date(2026, 7, 15, 18, 30, 0, 123456789, time.FixedZone("test", -3*60*60)),
		ID:         uuid.MustParse("01981c38-277b-7a31-a350-d9fcb545f8ce"),
	}
	encoded := EncodeCursor(want)
	got, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if !got.OccurredAt.Equal(want.OccurredAt) || got.ID != want.ID {
		t.Fatalf("DecodeCursor(EncodeCursor(cursor)) = %#v, want %#v", got, want)
	}
}

func TestCursorRejectsMalformedAndOversizedValues(t *testing.T) {
	for _, value := range []string{"%%%", "bm90LWEtdmFsaWQtY3Vyc29y", strings.Repeat("a", 257)} {
		if _, err := DecodeCursor(value); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("DecodeCursor(%q) error = %v, want %v", value, err, ErrInvalidCursor)
		}
	}
}

func TestNewEventUsesSafeDefaults(t *testing.T) {
	at := time.Date(2026, 7, 15, 12, 0, 0, 0, time.FixedZone("local", -3*60*60))
	correlationID := uuid.New()
	event, err := NewEvent("security.session_started", OutcomeSuccess, "login_completed", at, correlationID)
	if err != nil {
		t.Fatal(err)
	}
	if event.ID == uuid.Nil || event.Version != 1 || event.Source != "api" {
		t.Fatalf("unexpected event defaults: %#v", event)
	}
	if event.OccurredAt.Location() != time.UTC || !event.OccurredAt.Equal(at) {
		t.Fatalf("OccurredAt = %v, want UTC equivalent of %v", event.OccurredAt, at)
	}
	if event.Details == nil {
		t.Fatal("Details must be a non-nil empty object")
	}
}
