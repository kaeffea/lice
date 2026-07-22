package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kaeffea/lice/apps/api/internal/audit"
	"github.com/kaeffea/lice/apps/api/internal/clock"
	"github.com/kaeffea/lice/apps/api/internal/cryptoutil"
	"golang.org/x/oauth2"
)

type ServiceConfig struct {
	LoginTTL           time.Duration
	SessionIdleTTL     time.Duration
	SessionAbsoluteTTL time.Duration
}

type Service struct {
	store    Store
	provider Provider
	clock    clock.Clock
	cipher   *cryptoutil.Cipher
	digester *cryptoutil.Digester
	csrf     *cryptoutil.CSRF
	config   ServiceConfig
}

func NewService(store Store, provider Provider, clk clock.Clock, cipher *cryptoutil.Cipher, digester *cryptoutil.Digester, csrf *cryptoutil.CSRF, config ServiceConfig) (*Service, error) {
	if store == nil || provider == nil || clk == nil || cipher == nil || digester == nil || csrf == nil {
		return nil, errors.New("all authentication service dependencies are required")
	}
	if config.LoginTTL <= 0 || config.SessionIdleTTL <= 0 || config.SessionAbsoluteTTL <= config.SessionIdleTTL {
		return nil, errors.New("authentication durations are invalid")
	}
	return &Service{store: store, provider: provider, clock: clk, cipher: cipher, digester: digester, csrf: csrf, config: config}, nil
}

func (s *Service) BeginLogin(ctx context.Context, correlationID uuid.UUID) (LoginStart, error) {
	now := s.clock.Now().UTC()
	state, err := cryptoutil.RandomToken()
	if err != nil {
		return LoginStart{}, err
	}
	nonce, err := cryptoutil.RandomToken()
	if err != nil {
		return LoginStart{}, err
	}
	browserBinding, err := cryptoutil.RandomToken()
	if err != nil {
		return LoginStart{}, err
	}
	verifier := oauth2.GenerateVerifier()
	transactionID, err := uuid.NewV7()
	if err != nil {
		return LoginStart{}, fmt.Errorf("create login transaction id: %w", err)
	}
	ciphertext, err := s.cipher.Seal([]byte(verifier), []byte(transactionID.String()))
	if err != nil {
		return LoginStart{}, err
	}
	stateDigest := cryptoutil.Digest(state)
	nonceDigest := cryptoutil.Digest(nonce)
	bindingDigest := cryptoutil.Digest(browserBinding)
	tx := LoginTransaction{
		ID:                     transactionID,
		StateDigest:            stateDigest[:],
		NonceDigest:            nonceDigest[:],
		BrowserBindingDigest:   bindingDigest[:],
		PKCEVerifierCiphertext: ciphertext,
		CryptoKeyID:            s.cipher.KeyID(),
		CreatedAt:              now,
		ExpiresAt:              now.Add(s.config.LoginTTL),
	}
	authorizationURL, err := s.provider.AuthorizationURL(ctx, state, nonce, oauth2.S256ChallengeFromVerifier(verifier))
	if err != nil {
		return LoginStart{}, err
	}
	if err := s.store.CreateLoginTransaction(ctx, tx); err != nil {
		return LoginStart{}, fmt.Errorf("persist login transaction: %w", err)
	}
	_ = correlationID // correlation starts at the request boundary; no audit event is emitted before authentication.
	return LoginStart{AuthorizationURL: authorizationURL, BrowserBinding: browserBinding}, nil
}

func (s *Service) CompleteLogin(ctx context.Context, state, browserBinding, code string, correlationID uuid.UUID) (SessionStart, error) {
	if state == "" || browserBinding == "" || code == "" || len(state) > 512 || len(browserBinding) > 512 || len(code) > 4096 {
		return SessionStart{}, ErrInvalidLogin
	}
	now := s.clock.Now().UTC()
	stateDigest := cryptoutil.Digest(state)
	bindingDigest := cryptoutil.Digest(browserBinding)
	transaction, err := s.store.ConsumeLoginTransaction(ctx, stateDigest[:], bindingDigest[:], now)
	if err != nil {
		return SessionStart{}, ErrInvalidLogin
	}
	if transaction.CryptoKeyID != s.cipher.KeyID() {
		return SessionStart{}, ErrInvalidLogin
	}
	verifier, err := s.cipher.Open(transaction.PKCEVerifierCiphertext, []byte(transaction.ID.String()))
	if err != nil {
		return SessionStart{}, ErrInvalidLogin
	}
	verified, err := s.provider.ExchangeAndVerify(ctx, code, string(verifier))
	if err != nil {
		reason := "callback_rejected"
		if errors.Is(err, ErrProviderUnavailable) {
			reason = "provider_unavailable"
		}
		if auditErr := s.recordAnonymous(ctx, "security.login_rejected", audit.OutcomeFailure, reason, now, correlationID); auditErr != nil {
			return SessionStart{}, auditErr
		}
		return SessionStart{}, err
	}
	nonceDigest := cryptoutil.Digest(verified.Nonce)
	if !cryptoutil.EqualDigest(transaction.NonceDigest, nonceDigest[:]) {
		if auditErr := s.recordAnonymous(ctx, "security.login_rejected", audit.OutcomeDenied, "nonce_mismatch", now, correlationID); auditErr != nil {
			return SessionStart{}, auditErr
		}
		return SessionStart{}, ErrInvalidLogin
	}
	identity, err := s.store.FindExternalIdentity(ctx, verified.Issuer, verified.Subject)
	if err != nil {
		if errors.Is(err, ErrIdentityUnknown) {
			if auditErr := s.recordAnonymous(ctx, "security.login_rejected", audit.OutcomeDenied, "identity_unknown", now, correlationID); auditErr != nil {
				return SessionStart{}, auditErr
			}
		}
		return SessionStart{}, err
	}
	allowed, err := s.store.HasPlatformRole(ctx, identity.PrincipalID, PlatformOperatorRole, now)
	if err != nil {
		return SessionStart{}, err
	}
	if !allowed {
		event, eventErr := audit.NewEvent("security.access_denied", audit.OutcomeDenied, "platform_grant_missing", now, correlationID)
		if eventErr != nil {
			return SessionStart{}, eventErr
		}
		event.ActorPrincipalID = &identity.PrincipalID
		event.ResourceType = "platform_console"
		if err := s.store.RecordAudit(ctx, event); err != nil {
			return SessionStart{}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
		}
		return SessionStart{}, ErrAccessDenied
	}
	token, err := cryptoutil.RandomToken()
	if err != nil {
		return SessionStart{}, err
	}
	tokenDigest := s.digester.Digest(token)
	sessionID, err := uuid.NewV7()
	if err != nil {
		return SessionStart{}, fmt.Errorf("create session id: %w", err)
	}
	session := Session{
		ID:                 sessionID,
		PrincipalID:        identity.PrincipalID,
		ExternalIdentityID: identity.ID,
		CreatedAt:          now,
		LastSeenAt:         now,
		IdleExpiresAt:      now.Add(s.config.SessionIdleTTL),
		AbsoluteExpiresAt:  now.Add(s.config.SessionAbsoluteTTL),
	}
	event, err := audit.NewEvent("security.session_started", audit.OutcomeSuccess, "login_completed", now, correlationID)
	if err != nil {
		return SessionStart{}, err
	}
	event.ActorPrincipalID = &session.PrincipalID
	event.ActorSessionID = &session.ID
	event.ActorRole = PlatformOperatorRole
	event.ResourceType = "web_session"
	event.ResourceID = &session.ID
	if err := s.store.CreateSession(ctx, session, tokenDigest[:], event); err != nil {
		return SessionStart{}, fmt.Errorf("create session: %w", err)
	}
	return SessionStart{SessionToken: token, Session: session}, nil
}

func (s *Service) RejectProviderCallback(ctx context.Context, state, browserBinding string, correlationID uuid.UUID) error {
	if state == "" || browserBinding == "" || len(state) > 512 || len(browserBinding) > 512 {
		return ErrInvalidLogin
	}
	now := s.clock.Now().UTC()
	stateDigest := cryptoutil.Digest(state)
	bindingDigest := cryptoutil.Digest(browserBinding)
	if _, err := s.store.ConsumeLoginTransaction(ctx, stateDigest[:], bindingDigest[:], now); err != nil {
		return ErrInvalidLogin
	}
	return s.recordAnonymous(ctx, "security.login_rejected", audit.OutcomeDenied, "provider_access_denied", now, correlationID)
}

func (s *Service) Authenticate(ctx context.Context, rawToken string, correlationID uuid.UUID) (Session, error) {
	if rawToken == "" || len(rawToken) > 512 {
		return Session{}, ErrSessionNotFound
	}
	digest := s.digester.Digest(rawToken)
	return s.store.ResolveSession(ctx, digest[:], s.clock.Now().UTC(), s.config.SessionIdleTTL, correlationID)
}

func (s *Service) AuthorizePlatform(ctx context.Context, session Session, correlationID uuid.UUID) error {
	allowed, err := s.store.AuthorizePlatformSession(ctx, session, s.clock.Now().UTC(), correlationID)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}
	return ErrAccessDenied
}

func (s *Service) EndSession(ctx context.Context, rawToken string, correlationID uuid.UUID) (bool, error) {
	if rawToken == "" || len(rawToken) > 512 {
		return false, nil
	}
	digest := s.digester.Digest(rawToken)
	return s.store.EndSession(ctx, digest[:], s.clock.Now().UTC(), correlationID)
}

func (s *Service) CSRFToken(session Session) string { return s.csrf.Token(session.ID.String()) }

func (s *Service) ValidCSRF(session Session, presented string) bool {
	return s.csrf.Valid(session.ID.String(), presented)
}

func (s *Service) LogoutURL(ctx context.Context, postLogoutRedirect string) (string, error) {
	return s.provider.LogoutURL(ctx, postLogoutRedirect)
}

func (s *Service) ListAuditEvents(ctx context.Context, cursor audit.Cursor, limit int, period, eventType, outcome, query string) (audit.Page, error) {
	duration := map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}[period]
	if duration == 0 {
		return audit.Page{}, errors.New("invalid audit period")
	}
	filter := audit.Filter{
		Since:     s.clock.Now().UTC().Add(-duration),
		EventType: eventType,
		Outcome:   outcome,
		Query:     query,
	}
	return s.store.ListAuditEvents(ctx, cursor, limit, filter)
}

func (s *Service) GetAuditEvent(ctx context.Context, id uuid.UUID) (audit.Event, error) {
	return s.store.GetAuditEvent(ctx, id)
}

func (s *Service) recordAnonymous(ctx context.Context, eventType, outcome, reason string, at time.Time, correlationID uuid.UUID) error {
	event, err := audit.NewEvent(eventType, outcome, reason, at, correlationID)
	if err != nil {
		return err
	}
	if err := s.store.RecordAudit(ctx, event); err != nil {
		return fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	return nil
}
