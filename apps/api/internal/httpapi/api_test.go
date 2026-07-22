package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaeffea/lice/apps/api/internal/audit"
	"github.com/kaeffea/lice/apps/api/internal/auth"
	"github.com/kaeffea/lice/apps/api/internal/clock"
	"github.com/kaeffea/lice/apps/api/internal/config"
	"github.com/kaeffea/lice/apps/api/internal/cryptoutil"
)

type apiStore struct {
	transaction *auth.LoginTransaction
	consumed    bool
	identity    auth.ExternalIdentity
	identityErr error
	created     *auth.Session
	resolved    auth.Session
	resolveErr  error
	hasRole     bool
	endCalls    int
	recorded    []audit.Event
	createCalls int
}

func (s *apiStore) CreateLoginTransaction(_ context.Context, transaction auth.LoginTransaction) error {
	s.createCalls++
	copy := transaction
	s.transaction = &copy
	return nil
}

func (s *apiStore) ConsumeLoginTransaction(_ context.Context, stateDigest, bindingDigest []byte, now time.Time) (auth.LoginTransaction, error) {
	if s.transaction == nil || s.consumed || !cryptoutil.EqualDigest(stateDigest, s.transaction.StateDigest) || !cryptoutil.EqualDigest(bindingDigest, s.transaction.BrowserBindingDigest) {
		return auth.LoginTransaction{}, auth.ErrInvalidLogin
	}
	s.consumed = true
	copy := *s.transaction
	copy.ConsumedAt = &now
	return copy, nil
}

func (s *apiStore) FindExternalIdentity(context.Context, string, string) (auth.ExternalIdentity, error) {
	if s.identityErr != nil {
		return auth.ExternalIdentity{}, s.identityErr
	}
	return s.identity, nil
}

func (s *apiStore) CreateSession(_ context.Context, session auth.Session, _ []byte, _ audit.Event) error {
	copy := session
	s.created = &copy
	return nil
}

func (s *apiStore) ResolveSession(context.Context, []byte, time.Time, time.Duration, uuid.UUID) (auth.Session, error) {
	return s.resolved, s.resolveErr
}

func (s *apiStore) EndSession(context.Context, []byte, time.Time, uuid.UUID) (bool, error) {
	s.endCalls++
	return true, nil
}

func (s *apiStore) HasPlatformRole(context.Context, uuid.UUID, string, time.Time) (bool, error) {
	return s.hasRole, nil
}

func (s *apiStore) AuthorizePlatformSession(_ context.Context, session auth.Session, at time.Time, correlationID uuid.UUID) (bool, error) {
	if s.hasRole {
		return true, nil
	}
	event, err := audit.NewEvent("security.access_denied", audit.OutcomeDenied, "platform_grant_missing", at, correlationID)
	if err != nil {
		return false, err
	}
	event.ActorPrincipalID = &session.PrincipalID
	event.ActorSessionID = &session.ID
	s.recorded = append(s.recorded, event)
	return false, nil
}

func (s *apiStore) RecordAudit(_ context.Context, event audit.Event) error {
	s.recorded = append(s.recorded, event)
	return nil
}

func (s *apiStore) ListAuditEvents(context.Context, audit.Cursor, int, audit.Filter) (audit.Page, error) {
	return audit.Page{}, nil
}

func (s *apiStore) GetAuditEvent(context.Context, uuid.UUID) (audit.Event, error) {
	return audit.Event{}, audit.ErrNotFound
}

func (s *apiStore) BootstrapIdentity(context.Context, string, string, string, bool, time.Time, uuid.UUID) (auth.BootstrapResult, error) {
	return auth.BootstrapResult{}, nil
}

type apiProvider struct {
	state        string
	nonce        string
	authorizeErr error
	verified     auth.VerifiedIdentity
	exchangeErr  error
	logoutURL    string
}

func (p *apiProvider) AuthorizationURL(_ context.Context, state, nonce, _ string) (string, error) {
	p.state, p.nonce = state, nonce
	if p.authorizeErr != nil {
		return "", p.authorizeErr
	}
	return "https://identity.example/authorize", nil
}

func (p *apiProvider) ExchangeAndVerify(context.Context, string, string) (auth.VerifiedIdentity, error) {
	if p.exchangeErr != nil {
		return auth.VerifiedIdentity{}, p.exchangeErr
	}
	return p.verified, nil
}

func (p *apiProvider) LogoutURL(context.Context, string) (string, error) {
	return p.logoutURL, nil
}

type apiPinger struct{}

func (apiPinger) Ping(context.Context) error { return nil }

type apiHarness struct {
	api      *API
	handler  http.Handler
	service  *auth.Service
	store    *apiStore
	provider *apiProvider
	config   config.Config
	logs     *bytes.Buffer
}

func newAPIHarness(t *testing.T) apiHarness {
	t.Helper()
	store := &apiStore{
		identity: auth.ExternalIdentity{
			ID:          uuid.New(),
			PrincipalID: uuid.New(),
			Issuer:      "https://identity.example/realms/lice",
			Subject:     "linked-subject",
		},
		hasRole: true,
	}
	provider := &apiProvider{logoutURL: "https://identity.example/logout?client_id=lice-web&post_logout_redirect_uri=http%3A%2F%2Flice.localhost%3A8080%2Facesso%2Fsessao-encerrada"}
	cipher, _ := cryptoutil.NewCipher("test-v1", bytes.Repeat([]byte{1}, 32))
	digester, _ := cryptoutil.NewDigester(bytes.Repeat([]byte{2}, 32))
	csrf, _ := cryptoutil.NewCSRF(bytes.Repeat([]byte{3}, 32))
	service, err := auth.NewService(store, provider, clock.Fixed{Time: time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)}, cipher, digester, csrf, auth.ServiceConfig{
		LoginTTL:           5 * time.Minute,
		SessionIdleTTL:     30 * time.Minute,
		SessionAbsoluteTTL: 8 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	publicURL, _ := url.Parse("http://lice.localhost:8080")
	cfg := config.Config{Environment: "local", PublicURL: publicURL, LoginTTL: 5 * time.Minute}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	api, err := New(service, apiPinger{}, cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	return apiHarness{api: api, handler: api.Handler(), service: service, store: store, provider: provider, config: cfg, logs: logs}
}

func TestLoginCallbackUsesBoundTransactionAndSessionCookie(t *testing.T) {
	harness := newAPIHarness(t)
	loginRequest := httptest.NewRequest(http.MethodGet, "http://lice.localhost:8080/api/v1/auth/login", nil)
	loginResponse := httptest.NewRecorder()
	harness.handler.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusFound || loginResponse.Header().Get("Location") != "https://identity.example/authorize" {
		t.Fatalf("login response = %d %q", loginResponse.Code, loginResponse.Header().Get("Location"))
	}
	var bindingCookie *http.Cookie
	for _, cookie := range loginResponse.Result().Cookies() {
		if cookie.Name == harness.config.LoginCookieName() {
			bindingCookie = cookie
		}
	}
	if bindingCookie == nil || !bindingCookie.HttpOnly || bindingCookie.Path != "/api/v1/auth/callback" || bindingCookie.Value == "" {
		t.Fatalf("unsafe login binding cookie: %#v", bindingCookie)
	}
	harness.provider.verified = auth.VerifiedIdentity{
		Issuer: harness.store.identity.Issuer, Subject: harness.store.identity.Subject, Nonce: harness.provider.nonce,
	}
	callbackRequest := httptest.NewRequest(http.MethodGet, "http://lice.localhost:8080/api/v1/auth/callback?state="+url.QueryEscape(harness.provider.state)+"&code=one-time-code", nil)
	callbackRequest.AddCookie(bindingCookie)
	callbackResponse := httptest.NewRecorder()
	harness.handler.ServeHTTP(callbackResponse, callbackRequest)
	if callbackResponse.Code != http.StatusSeeOther || callbackResponse.Header().Get("Location") != "http://lice.localhost:8080/controle" {
		t.Fatalf("callback response = %d %q", callbackResponse.Code, callbackResponse.Header().Get("Location"))
	}
	var sessionCookie *http.Cookie
	for _, cookie := range callbackResponse.Result().Cookies() {
		if cookie.Name == harness.config.SessionCookieName() {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.Value == "" || sessionCookie.MaxAge != 0 || !sessionCookie.Expires.IsZero() {
		t.Fatalf("session cookie is missing or persistent: %#v", sessionCookie)
	}
	if callbackResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("callback Cache-Control = %q", callbackResponse.Header().Get("Cache-Control"))
	}
}

func TestProductionLoginCookieSatisfiesHostPrefixRequirements(t *testing.T) {
	harness := newAPIHarness(t)
	harness.api.config.Environment = "production"
	request := httptest.NewRequest(http.MethodGet, "https://lice.example/api/v1/auth/login", nil)
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("login response = %d", response.Code)
	}
	var binding *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "__Host-lice_oidc_tx" {
			binding = cookie
		}
	}
	if binding == nil || binding.Path != "/" || !binding.Secure || !binding.HttpOnly || binding.Domain != "" {
		t.Fatalf("production binding cookie violates __Host- requirements: %#v", binding)
	}
}

func TestLoginRateLimitRunsBeforeCreatingTransaction(t *testing.T) {
	harness := newAPIHarness(t)
	for attempt := 1; attempt <= loginRatePerClient+1; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "http://lice.localhost:8080/api/v1/auth/login", nil)
		request.RemoteAddr = "203.0.113.10:4567"
		response := httptest.NewRecorder()
		harness.handler.ServeHTTP(response, request)
		if attempt <= loginRatePerClient && response.Code != http.StatusFound {
			t.Fatalf("attempt %d response = %d", attempt, response.Code)
		}
		if attempt == loginRatePerClient+1 {
			if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
				t.Fatalf("limited response = %d Retry-After %q", response.Code, response.Header().Get("Retry-After"))
			}
		}
	}
	if harness.store.createCalls != loginRatePerClient {
		t.Fatalf("login transactions created = %d, want %d", harness.store.createCalls, loginRatePerClient)
	}
}

func TestLoginProviderFailureUsesVisualErrorRoute(t *testing.T) {
	harness := newAPIHarness(t)
	harness.provider.authorizeErr = auth.ErrProviderUnavailable
	request := httptest.NewRequest(http.MethodGet, "http://lice.localhost:8080/api/v1/auth/login", nil)
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "http://lice.localhost:8080/acesso/servico-indisponivel" {
		t.Fatalf("provider failure response = %d %q", response.Code, response.Header().Get("Location"))
	}
	if harness.store.createCalls != 0 {
		t.Fatal("provider failure persisted a login transaction")
	}
}

func TestCallbackUsesClosedErrorRoutes(t *testing.T) {
	tests := []struct {
		name        string
		prepare     func(*apiHarness)
		want        string
		withBinding bool
	}{
		{name: "missing binding", want: "http://lice.localhost:8080/acesso/fluxo-invalido"},
		{name: "unknown identity", withBinding: true, want: "http://lice.localhost:8080/acesso/negado", prepare: func(h *apiHarness) { h.store.identityErr = auth.ErrIdentityUnknown }},
		{name: "provider unavailable", withBinding: true, want: "http://lice.localhost:8080/acesso/servico-indisponivel", prepare: func(h *apiHarness) { h.provider.exchangeErr = auth.ErrProviderUnavailable }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newAPIHarness(t)
			if test.prepare != nil {
				test.prepare(&harness)
			}
			request := httptest.NewRequest(http.MethodGet, "http://lice.localhost:8080/api/v1/auth/callback?state=invalid&code=secret", nil)
			if test.withBinding {
				loginRequest := httptest.NewRequest(http.MethodGet, "http://lice.localhost:8080/api/v1/auth/login", nil)
				loginResponse := httptest.NewRecorder()
				harness.handler.ServeHTTP(loginResponse, loginRequest)
				binding := loginResponse.Result().Cookies()[0]
				harness.provider.verified = auth.VerifiedIdentity{Issuer: harness.store.identity.Issuer, Subject: harness.store.identity.Subject, Nonce: harness.provider.nonce}
				request = httptest.NewRequest(http.MethodGet, "http://lice.localhost:8080/api/v1/auth/callback?state="+url.QueryEscape(harness.provider.state)+"&code=secret", nil)
				request.AddCookie(binding)
			}
			response := httptest.NewRecorder()
			harness.handler.ServeHTTP(response, request)
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != test.want {
				t.Fatalf("callback response = %d %q, want redirect %q", response.Code, response.Header().Get("Location"), test.want)
			}
		})
	}
}

func TestProtectedErrorsAreNeverCacheableAndLogsRedactQueryAndCookies(t *testing.T) {
	harness := newAPIHarness(t)
	request := httptest.NewRequest(http.MethodGet, "http://lice.localhost:8080/api/v1/session?token=TOP-SECRET-QUERY", nil)
	request.Header.Set("Cookie", "lice_session=TOP-SECRET-COOKIE")
	harness.store.resolveErr = auth.ErrSessionNotFound
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("protected error = %d Cache-Control %q", response.Code, response.Header().Get("Cache-Control"))
	}
	logged := harness.logs.String()
	if strings.Contains(logged, "TOP-SECRET-QUERY") || strings.Contains(logged, "TOP-SECRET-COOKIE") || strings.Contains(logged, "?token=") {
		t.Fatalf("request log exposed URL query or cookie: %s", logged)
	}
}

func TestProtectedResponsePublishesRefreshedIdleDeadline(t *testing.T) {
	harness := newAPIHarness(t)
	idleDeadline := time.Date(2026, 7, 15, 18, 30, 0, 123, time.UTC)
	harness.store.resolved = auth.Session{
		ID: uuid.New(), PrincipalID: harness.store.identity.PrincipalID, DisplayName: "Operador",
		CreatedAt:         time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC),
		IdleExpiresAt:     idleDeadline,
		AbsoluteExpiresAt: time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC),
	}
	request := httptest.NewRequest(http.MethodGet, "http://lice.localhost:8080/api/v1/session", nil)
	request.AddCookie(&http.Cookie{Name: harness.config.SessionCookieName(), Value: "session-token"})
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("session response = %d", response.Code)
	}
	if got := response.Header().Get(sessionIdleExpiresHeader); got != idleDeadline.Format(time.RFC3339Nano) {
		t.Fatalf("idle deadline header = %q", got)
	}
}

func TestLogoutRequiresOriginAndCSRFThenClearsLocalSession(t *testing.T) {
	harness := newAPIHarness(t)
	harness.store.resolved = auth.Session{
		ID: uuid.New(), PrincipalID: harness.store.identity.PrincipalID, DisplayName: "Operador",
		CreatedAt: time.Now().Add(-time.Hour), IdleExpiresAt: time.Now().Add(time.Minute), AbsoluteExpiresAt: time.Now().Add(time.Hour),
	}
	csrfToken := harness.service.CSRFToken(harness.store.resolved)
	form := url.Values{"csrf_token": {csrfToken}}
	request := httptest.NewRequest(http.MethodPost, "http://lice.localhost:8080/api/v1/auth/logout", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://lice.localhost:8080")
	request.AddCookie(&http.Cookie{Name: harness.config.SessionCookieName(), Value: "raw-session-token"})
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != harness.provider.logoutURL || harness.store.endCalls != 1 {
		t.Fatalf("logout response = %d %q, end calls = %d", response.Code, response.Header().Get("Location"), harness.store.endCalls)
	}
	cleared := false
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == harness.config.SessionCookieName() && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout did not clear the local session cookie")
	}
	if strings.Contains(response.Header().Get("Location"), "id_token_hint") {
		t.Fatal("logout redirect exposed an ID token hint")
	}
}

func TestLogoutRejectsMissingCSRFAndCrossOriginRequests(t *testing.T) {
	for _, test := range []struct {
		name         string
		origin       string
		csrf         string
		wantStatus   int
		wantLocation string
	}{
		{
			name: "missing csrf", origin: "http://lice.localhost:8080",
			wantStatus:   http.StatusSeeOther,
			wantLocation: "http://lice.localhost:8080/acesso/fluxo-invalido",
		},
		{name: "cross origin", origin: "https://attacker.example", csrf: "anything", wantStatus: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newAPIHarness(t)
			harness.store.resolved = auth.Session{ID: uuid.New(), PrincipalID: harness.store.identity.PrincipalID}
			body := "csrf_token=" + url.QueryEscape(test.csrf)
			request := httptest.NewRequest(http.MethodPost, "http://lice.localhost:8080/api/v1/auth/logout", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", test.origin)
			request.AddCookie(&http.Cookie{Name: harness.config.SessionCookieName(), Value: "token"})
			response := httptest.NewRecorder()
			harness.handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus || response.Header().Get("Location") != test.wantLocation || harness.store.endCalls != 0 {
				t.Fatalf("logout response = %d %q, end calls = %d", response.Code, response.Header().Get("Location"), harness.store.endCalls)
			}
		})
	}
}

func TestLogoutKeepsCookieWhenSessionRevocationCannotBeEstablished(t *testing.T) {
	harness := newAPIHarness(t)
	harness.store.resolveErr = errors.New("database unavailable")
	request := httptest.NewRequest(http.MethodPost, "http://lice.localhost:8080/api/v1/auth/logout", nil)
	request.Header.Set("Origin", "http://lice.localhost:8080")
	request.AddCookie(&http.Cookie{Name: harness.config.SessionCookieName(), Value: "raw-session-token"})
	response := httptest.NewRecorder()

	harness.handler.ServeHTTP(response, request)

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "http://lice.localhost:8080/acesso/servico-indisponivel" {
		t.Fatalf("logout response = %d %q", response.Code, response.Header().Get("Location"))
	}
	if harness.store.endCalls != 0 {
		t.Fatalf("end calls = %d, want 0", harness.store.endCalls)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == harness.config.SessionCookieName() {
			t.Fatal("logout cleared the cookie without establishing server-side revocation")
		}
	}
}
