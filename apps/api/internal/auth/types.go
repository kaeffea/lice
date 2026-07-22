package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kaeffea/lice/apps/api/internal/audit"
)

var (
	ErrAccessDenied        = errors.New("access denied")
	ErrAuditUnavailable    = errors.New("security audit unavailable")
	ErrIdentityUnknown     = errors.New("identity unknown")
	ErrInvalidLogin        = errors.New("invalid login transaction")
	ErrProviderUnavailable = errors.New("identity provider unavailable")
	ErrSessionExpired      = errors.New("session expired")
	ErrSessionNotFound     = errors.New("session not found")
)

const PlatformOperatorRole = "platform_operator"

type LoginTransaction struct {
	ID                     uuid.UUID
	StateDigest            []byte
	NonceDigest            []byte
	BrowserBindingDigest   []byte
	PKCEVerifierCiphertext []byte
	CryptoKeyID            string
	CreatedAt              time.Time
	ExpiresAt              time.Time
	ConsumedAt             *time.Time
}

type ExternalIdentity struct {
	ID          uuid.UUID
	PrincipalID uuid.UUID
	Issuer      string
	Subject     string
}

type Session struct {
	ID                 uuid.UUID
	PrincipalID        uuid.UUID
	ExternalIdentityID uuid.UUID
	CreatedAt          time.Time
	LastSeenAt         time.Time
	IdleExpiresAt      time.Time
	AbsoluteExpiresAt  time.Time
	DisplayName        string
}

type VerifiedIdentity struct {
	Issuer  string
	Subject string
	Nonce   string
}

type LoginStart struct {
	AuthorizationURL string
	BrowserBinding   string
}

type SessionStart struct {
	SessionToken string
	Session      Session
}

type BootstrapResult struct {
	PrincipalID   uuid.UUID
	IdentityID    uuid.UUID
	PrincipalMade bool
	IdentityMade  bool
	GrantMade     bool
}

type Store interface {
	CreateLoginTransaction(context.Context, LoginTransaction) error
	ConsumeLoginTransaction(context.Context, []byte, []byte, time.Time) (LoginTransaction, error)
	FindExternalIdentity(context.Context, string, string) (ExternalIdentity, error)
	CreateSession(context.Context, Session, []byte, audit.Event) error
	ResolveSession(context.Context, []byte, time.Time, time.Duration, uuid.UUID) (Session, error)
	EndSession(context.Context, []byte, time.Time, uuid.UUID) (bool, error)
	HasPlatformRole(context.Context, uuid.UUID, string, time.Time) (bool, error)
	AuthorizePlatformSession(context.Context, Session, time.Time, uuid.UUID) (bool, error)
	RecordAudit(context.Context, audit.Event) error
	ListAuditEvents(context.Context, audit.Cursor, int, audit.Filter) (audit.Page, error)
	GetAuditEvent(context.Context, uuid.UUID) (audit.Event, error)
	BootstrapIdentity(context.Context, string, string, string, bool, time.Time, uuid.UUID) (BootstrapResult, error)
}

type Provider interface {
	AuthorizationURL(context.Context, string, string, string) (string, error)
	ExchangeAndVerify(context.Context, string, string) (VerifiedIdentity, error)
	LogoutURL(context.Context, string) (string, error)
}
