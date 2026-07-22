package audit

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	OutcomeSuccess = "success"
	OutcomeDenied  = "denied"
	OutcomeFailure = "failure"
)

var (
	ErrInvalidCursor = errors.New("invalid audit cursor")
	ErrNotFound      = errors.New("audit event not found")
)

type Event struct {
	ID               uuid.UUID      `json:"id"`
	Type             string         `json:"type"`
	Version          int16          `json:"version"`
	OccurredAt       time.Time      `json:"occurred_at"`
	ActorPrincipalID *uuid.UUID     `json:"actor_principal_id,omitempty"`
	ActorDisplayName string         `json:"actor_display_name,omitempty"`
	ActorSessionID   *uuid.UUID     `json:"actor_session_id,omitempty"`
	ActorRole        string         `json:"actor_role,omitempty"`
	Source           string         `json:"source"`
	Outcome          string         `json:"outcome"`
	ReasonCode       string         `json:"reason_code,omitempty"`
	CorrelationID    uuid.UUID      `json:"correlation_id"`
	ResourceType     string         `json:"resource_type,omitempty"`
	ResourceID       *uuid.UUID     `json:"resource_id,omitempty"`
	Details          map[string]any `json:"details"`
}

type Cursor struct {
	OccurredAt time.Time
	ID         uuid.UUID
}

type Page struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type Filter struct {
	Since     time.Time
	EventType string
	Outcome   string
	Query     string
}

func EncodeCursor(cursor Cursor) string {
	payload := strconv.FormatInt(cursor.OccurredAt.UTC().UnixNano(), 10) + "." + cursor.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func DecodeCursor(encoded string) (Cursor, error) {
	if encoded == "" {
		return Cursor{}, nil
	}
	if len(encoded) > 256 {
		return Cursor{}, ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 2 {
		return Cursor{}, ErrInvalidCursor
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	return Cursor{OccurredAt: time.Unix(0, nanos).UTC(), ID: id}, nil
}

func NewEvent(eventType, outcome, reason string, at time.Time, correlationID uuid.UUID) (Event, error) {
	if eventType == "" || correlationID == uuid.Nil {
		return Event{}, fmt.Errorf("event type and correlation id are required")
	}
	return Event{
		ID:            uuid.Must(uuid.NewV7()),
		Type:          eventType,
		Version:       1,
		OccurredAt:    at.UTC(),
		Source:        "api",
		Outcome:       outcome,
		ReasonCode:    reason,
		CorrelationID: correlationID,
		Details:       map[string]any{},
	}, nil
}
