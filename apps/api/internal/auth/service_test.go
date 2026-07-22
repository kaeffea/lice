package auth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaeffea/lice/apps/api/internal/audit"
	"github.com/kaeffea/lice/apps/api/internal/clock"
	"github.com/kaeffea/lice/apps/api/internal/cryptoutil"
	"golang.org/x/oauth2"
)

var testNow = time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)

type testStore struct {
	loginTransaction *LoginTransaction
	consumed         bool
	identity         ExternalIdentity
	identityErr      error
	createdSession   *Session
	createdDigest    []byte
	createdEvent     *audit.Event
	resolvedSession  Session
	resolveErr       error
	hasRole          bool
	recordedEvents   []audit.Event
	recordErr        error
	listedFilter     audit.Filter
	endedDigest      []byte
	endedAt          time.Time
	endCalls         int
}

func (s *testStore) CreateLoginTransaction(_ context.Context, transaction LoginTransaction) error {
	copy := transaction
	s.loginTransaction = &copy
	return nil
}

func (s *testStore) ConsumeLoginTransaction(_ context.Context, stateDigest, bindingDigest []byte, now time.Time) (LoginTransaction, error) {
	if s.loginTransaction == nil || s.consumed || !cryptoutil.EqualDigest(stateDigest, s.loginTransaction.StateDigest) || !cryptoutil.EqualDigest(bindingDigest, s.loginTransaction.BrowserBindingDigest) || !now.Before(s.loginTransaction.ExpiresAt) {
		return LoginTransaction{}, ErrInvalidLogin
	}
	s.consumed = true
	copy := *s.loginTransaction
	copy.ConsumedAt = &now
	return copy, nil
}

func (s *testStore) FindExternalIdentity(context.Context, string, string) (ExternalIdentity, error) {
	if s.identityErr != nil {
		return ExternalIdentity{}, s.identityErr
	}
	return s.identity, nil
}

func (s *testStore) CreateSession(_ context.Context, session Session, digest []byte, event audit.Event) error {
	copy := session
	s.createdSession = &copy
	s.createdDigest = append([]byte(nil), digest...)
	eventCopy := event
	s.createdEvent = &eventCopy
	return nil
}

func (s *testStore) ResolveSession(context.Context, []byte, time.Time, time.Duration, uuid.UUID) (Session, error) {
	return s.resolvedSession, s.resolveErr
}

func (s *testStore) EndSession(_ context.Context, digest []byte, at time.Time, _ uuid.UUID) (bool, error) {
	s.endedDigest = append([]byte(nil), digest...)
	s.endedAt = at
	s.endCalls++
	return true, nil
}

func (s *testStore) HasPlatformRole(context.Context, uuid.UUID, string, time.Time) (bool, error) {
	return s.hasRole, nil
}

func (s *testStore) AuthorizePlatformSession(_ context.Context, session Session, at time.Time, correlationID uuid.UUID) (bool, error) {
	if s.hasRole {
		return true, nil
	}
	event, err := audit.NewEvent("security.access_denied", audit.OutcomeDenied, "platform_grant_missing", at, correlationID)
	if err != nil {
		return false, err
	}
	event.ActorPrincipalID = &session.PrincipalID
	event.ActorSessionID = &session.ID
	s.recordedEvents = append(s.recordedEvents, event)
	return false, nil
}

func (s *testStore) RecordAudit(_ context.Context, event audit.Event) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	s.recordedEvents = append(s.recordedEvents, event)
	return nil
}

func (s *testStore) ListAuditEvents(_ context.Context, _ audit.Cursor, _ int, filter audit.Filter) (audit.Page, error) {
	s.listedFilter = filter
	return audit.Page{}, nil
}

func (s *testStore) GetAuditEvent(context.Context, uuid.UUID) (audit.Event, error) {
	return audit.Event{}, audit.ErrNotFound
}

func (s *testStore) BootstrapIdentity(context.Context, string, string, string, bool, time.Time, uuid.UUID) (BootstrapResult, error) {
	return BootstrapResult{}, nil
}

type testProvider struct {
	state             string
	nonce             string
	challenge         string
	authorizationErr  error
	verified          VerifiedIdentity
	exchangeErr       error
	exchangedCode     string
	exchangedVerifier string
}

func (p *testProvider) AuthorizationURL(_ context.Context, state, nonce, challenge string) (string, error) {
	p.state, p.nonce, p.challenge = state, nonce, challenge
	if p.authorizationErr != nil {
		return "", p.authorizationErr
	}
	return "https://identity.example/authorize", nil
}

func (p *testProvider) ExchangeAndVerify(_ context.Context, code, verifier string) (VerifiedIdentity, error) {
	p.exchangedCode, p.exchangedVerifier = code, verifier
	if p.exchangeErr != nil {
		return VerifiedIdentity{}, p.exchangeErr
	}
	return p.verified, nil
}

func (p *testProvider) LogoutURL(context.Context, string) (string, error) { return "", nil }

func newTestService(t *testing.T) (*Service, *testStore, *testProvider, *cryptoutil.Cipher, *cryptoutil.Digester) {
	t.Helper()
	store := &testStore{
		identity: ExternalIdentity{
			ID:          uuid.MustParse("01981c38-277b-7a31-a350-d9fcb545f8ce"),
			PrincipalID: uuid.MustParse("01981c38-4f28-78a6-bb7c-60cf0fc7557d"),
			Issuer:      "https://identity.example/realms/lice",
			Subject:     "subject-1",
		},
		hasRole: true,
	}
	provider := &testProvider{}
	cipher, err := cryptoutil.NewCipher("test-v1", bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	digester, err := cryptoutil.NewDigester(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	csrf, err := cryptoutil.NewCSRF(bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store, provider, clock.Fixed{Time: testNow}, cipher, digester, csrf, ServiceConfig{
		LoginTTL:           5 * time.Minute,
		SessionIdleTTL:     30 * time.Minute,
		SessionAbsoluteTTL: 8 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, store, provider, cipher, digester
}

func TestBeginLoginPersistsOnlyDigestsAndEncryptedPKCE(t *testing.T) {
	service, store, provider, cipher, _ := newTestService(t)
	start, err := service.BeginLogin(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("BeginLogin() error = %v", err)
	}
	if store.loginTransaction == nil {
		t.Fatal("login transaction was not persisted")
	}
	stateDigest := cryptoutil.Digest(provider.state)
	bindingDigest := cryptoutil.Digest(start.BrowserBinding)
	nonceDigest := cryptoutil.Digest(provider.nonce)
	if !cryptoutil.EqualDigest(store.loginTransaction.StateDigest, stateDigest[:]) || !cryptoutil.EqualDigest(store.loginTransaction.BrowserBindingDigest, bindingDigest[:]) || !cryptoutil.EqualDigest(store.loginTransaction.NonceDigest, nonceDigest[:]) {
		t.Fatal("persisted transaction digests do not match generated secrets")
	}
	if !store.loginTransaction.ExpiresAt.Equal(testNow.Add(5 * time.Minute)) {
		t.Fatalf("transaction expiry = %v", store.loginTransaction.ExpiresAt)
	}
	verifier, err := cipher.Open(store.loginTransaction.PKCEVerifierCiphertext, []byte(store.loginTransaction.ID.String()))
	if err != nil {
		t.Fatalf("decrypt persisted verifier: %v", err)
	}
	if provider.challenge != oauth2.S256ChallengeFromVerifier(string(verifier)) {
		t.Fatal("authorization request challenge does not match encrypted verifier")
	}
	if bytes.Contains(store.loginTransaction.PKCEVerifierCiphertext, verifier) {
		t.Fatal("PKCE verifier appeared in persisted ciphertext")
	}
}

func TestBeginLoginDoesNotPersistWhenProviderIsUnavailable(t *testing.T) {
	service, store, provider, _, _ := newTestService(t)
	provider.authorizationErr = ErrProviderUnavailable
	if _, err := service.BeginLogin(context.Background(), uuid.New()); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("BeginLogin() error = %v", err)
	}
	if store.loginTransaction != nil {
		t.Fatal("provider failure left an orphan login transaction")
	}
}

func TestCompleteLoginCreatesOpaqueSessionAndPreventsReplay(t *testing.T) {
	service, store, provider, _, digester := newTestService(t)
	start, err := service.BeginLogin(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	provider.verified = VerifiedIdentity{Issuer: store.identity.Issuer, Subject: store.identity.Subject, Nonce: provider.nonce}
	correlationID := uuid.New()
	created, err := service.CompleteLogin(context.Background(), provider.state, start.BrowserBinding, "one-time-code", correlationID)
	if err != nil {
		t.Fatalf("CompleteLogin() error = %v", err)
	}
	if created.SessionToken == "" || store.createdSession == nil || store.createdEvent == nil {
		t.Fatal("session, token, or audit event was not created")
	}
	wantDigest := digester.Digest(created.SessionToken)
	if !cryptoutil.EqualDigest(store.createdDigest, wantDigest[:]) {
		t.Fatal("store did not receive the keyed session-token digest")
	}
	if string(store.createdDigest) == created.SessionToken {
		t.Fatal("raw session token was persisted")
	}
	if provider.exchangedCode != "one-time-code" || provider.exchangedVerifier == "" {
		t.Fatal("provider exchange did not receive code and PKCE verifier")
	}
	if !created.Session.IdleExpiresAt.Equal(testNow.Add(30*time.Minute)) || !created.Session.AbsoluteExpiresAt.Equal(testNow.Add(8*time.Hour)) {
		t.Fatalf("unexpected session deadlines: %#v", created.Session)
	}
	if store.createdEvent.Type != "security.session_started" || store.createdEvent.CorrelationID != correlationID || store.createdEvent.ActorSessionID == nil {
		t.Fatalf("unexpected session audit event: %#v", store.createdEvent)
	}
	if _, err := service.CompleteLogin(context.Background(), provider.state, start.BrowserBinding, "replayed-code", uuid.New()); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("replayed callback error = %v, want %v", err, ErrInvalidLogin)
	}
}

func TestCompleteLoginRejectsUnknownIdentityWithoutCreatingSession(t *testing.T) {
	service, store, provider, _, _ := newTestService(t)
	start, err := service.BeginLogin(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	provider.verified = VerifiedIdentity{Issuer: store.identity.Issuer, Subject: "unlinked", Nonce: provider.nonce}
	store.identityErr = ErrIdentityUnknown
	if _, err := service.CompleteLogin(context.Background(), provider.state, start.BrowserBinding, "code", uuid.New()); !errors.Is(err, ErrIdentityUnknown) {
		t.Fatalf("CompleteLogin() error = %v, want %v", err, ErrIdentityUnknown)
	}
	if store.createdSession != nil {
		t.Fatal("unknown identity received a session")
	}
	if len(store.recordedEvents) != 1 || store.recordedEvents[0].ReasonCode != "identity_unknown" || store.recordedEvents[0].ActorPrincipalID != nil {
		t.Fatalf("unexpected rejection audit event: %#v", store.recordedEvents)
	}
}

func TestCompleteLoginRejectsNonceMismatchBeforeIdentityLookup(t *testing.T) {
	service, store, provider, _, _ := newTestService(t)
	start, err := service.BeginLogin(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	provider.verified = VerifiedIdentity{Issuer: store.identity.Issuer, Subject: store.identity.Subject, Nonce: "wrong-nonce"}
	if _, err := service.CompleteLogin(context.Background(), provider.state, start.BrowserBinding, "code", uuid.New()); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("CompleteLogin() error = %v, want %v", err, ErrInvalidLogin)
	}
	if store.createdSession != nil || len(store.recordedEvents) != 1 || store.recordedEvents[0].ReasonCode != "nonce_mismatch" {
		t.Fatalf("nonce mismatch side effects are unsafe: session=%#v events=%#v", store.createdSession, store.recordedEvents)
	}
}

func TestCompleteLoginFailsClosedWhenRejectionCannotBeAudited(t *testing.T) {
	service, store, provider, _, _ := newTestService(t)
	start, err := service.BeginLogin(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	store.recordErr = errors.New("audit database unavailable")
	provider.verified = VerifiedIdentity{Issuer: store.identity.Issuer, Subject: store.identity.Subject, Nonce: "wrong-nonce"}
	if _, err := service.CompleteLogin(context.Background(), provider.state, start.BrowserBinding, "code", uuid.New()); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("CompleteLogin() error = %v, want %v", err, ErrAuditUnavailable)
	}
	if store.createdSession != nil {
		t.Fatal("login continued without a required rejection audit")
	}
}

func TestCompleteLoginRejectsKnownIdentityWithoutGrantBeforeCreatingSession(t *testing.T) {
	service, store, provider, _, _ := newTestService(t)
	store.hasRole = false
	start, err := service.BeginLogin(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	provider.verified = VerifiedIdentity{Issuer: store.identity.Issuer, Subject: store.identity.Subject, Nonce: provider.nonce}
	if _, err := service.CompleteLogin(context.Background(), provider.state, start.BrowserBinding, "code", uuid.New()); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("CompleteLogin() error = %v, want %v", err, ErrAccessDenied)
	}
	if store.createdSession != nil {
		t.Fatal("identity without a platform grant received a session")
	}
	if len(store.recordedEvents) != 1 || store.recordedEvents[0].Type != "security.access_denied" || store.recordedEvents[0].ReasonCode != "platform_grant_missing" || store.recordedEvents[0].ActorPrincipalID == nil {
		t.Fatalf("unexpected grant-denial event: %#v", store.recordedEvents)
	}
}

func TestProviderErrorConsumesBoundTransactionAndIsAudited(t *testing.T) {
	service, store, provider, _, _ := newTestService(t)
	start, err := service.BeginLogin(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RejectProviderCallback(context.Background(), provider.state, start.BrowserBinding, uuid.New()); err != nil {
		t.Fatalf("RejectProviderCallback() error = %v", err)
	}
	if !store.consumed || len(store.recordedEvents) != 1 || store.recordedEvents[0].ReasonCode != "provider_access_denied" {
		t.Fatalf("provider rejection did not consume and audit the transaction: consumed=%v events=%#v", store.consumed, store.recordedEvents)
	}
	if err := service.RejectProviderCallback(context.Background(), provider.state, start.BrowserBinding, uuid.New()); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("replayed provider error = %v, want %v", err, ErrInvalidLogin)
	}
}

func TestAuthorizePlatformDenialIsAudited(t *testing.T) {
	service, store, _, _, _ := newTestService(t)
	store.hasRole = false
	session := Session{ID: uuid.New(), PrincipalID: uuid.New()}
	correlationID := uuid.New()
	if err := service.AuthorizePlatform(context.Background(), session, correlationID); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("AuthorizePlatform() error = %v, want %v", err, ErrAccessDenied)
	}
	if len(store.recordedEvents) != 1 {
		t.Fatalf("recorded events = %d, want 1", len(store.recordedEvents))
	}
	event := store.recordedEvents[0]
	if event.Type != "security.access_denied" || event.Outcome != audit.OutcomeDenied || event.ActorPrincipalID == nil || *event.ActorPrincipalID != session.PrincipalID {
		t.Fatalf("unexpected access-denied event: %#v", event)
	}
}

func TestAuditPeriodUsesInjectedClock(t *testing.T) {
	service, store, _, _, _ := newTestService(t)
	if _, err := service.ListAuditEvents(context.Background(), audit.Cursor{}, 25, "7d", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if want := testNow.Add(-7 * 24 * time.Hour); !store.listedFilter.Since.Equal(want) {
		t.Fatalf("audit Since = %v, want %v", store.listedFilter.Since, want)
	}
	if _, err := service.ListAuditEvents(context.Background(), audit.Cursor{}, 25, "forever", "", "", ""); err == nil {
		t.Fatal("invalid audit period was accepted")
	}
}

func TestEndSessionHashesCookieAndUsesInjectedClock(t *testing.T) {
	service, store, _, _, digester := newTestService(t)
	ended, err := service.EndSession(context.Background(), "raw-session-cookie", uuid.New())
	if err != nil || !ended {
		t.Fatalf("EndSession() = (%v, %v), want (true, nil)", ended, err)
	}
	wantDigest := digester.Digest("raw-session-cookie")
	if !cryptoutil.EqualDigest(store.endedDigest, wantDigest[:]) {
		t.Fatal("EndSession did not pass a keyed cookie digest to the store")
	}
	if !store.endedAt.Equal(testNow) || store.endCalls != 1 {
		t.Fatalf("logout store call = (%v, %d), want (%v, 1)", store.endedAt, store.endCalls, testNow)
	}
}
