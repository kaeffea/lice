package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kaeffea/lice/apps/api/internal/audit"
	"github.com/kaeffea/lice/apps/api/internal/auth"
	"github.com/kaeffea/lice/apps/api/internal/config"
)

type Pinger interface {
	Ping(context.Context) error
}

type API struct {
	service *auth.Service
	pinger  Pinger
	config  config.Config
	logger  *slog.Logger
	logins  *loginRateLimiter
}

type contextKey string

const correlationKey contextKey = "correlation_id"

const sessionIdleExpiresHeader = "X-Lice-Session-Idle-Expires-At"

var auditEventTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_.]{2,127}$`)

func New(service *auth.Service, pinger Pinger, cfg config.Config, logger *slog.Logger) (*API, error) {
	if service == nil || pinger == nil || logger == nil || cfg.PublicURL == nil {
		return nil, errors.New("HTTP API dependencies are required")
	}
	return &API{service: service, pinger: pinger, config: cfg, logger: logger, logins: newLoginRateLimiter()}, nil
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", a.live)
	mux.HandleFunc("GET /health/ready", a.ready)
	mux.HandleFunc("GET /api/v1/auth/login", a.login)
	mux.HandleFunc("GET /api/v1/auth/callback", a.callback)
	mux.HandleFunc("GET /api/v1/session", a.session)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)
	mux.HandleFunc("GET /api/v1/platform/audit-events", a.auditEvents)
	mux.HandleFunc("GET /api/v1/platform/audit-events/{event_id}", a.auditEvent)

	var handler http.Handler = mux
	crossOrigin := http.NewCrossOriginProtection()
	crossOrigin.AddTrustedOrigin(a.publicOrigin())
	handler = crossOrigin.Handler(handler)
	handler = a.apiNoStore(handler)
	handler = a.securityHeaders(handler)
	handler = a.recoverPanics(handler)
	handler = a.requestLog(handler)
	handler = a.correlations(handler)
	return handler
}

func (a *API) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.pinger.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if allowed, retryAfter := a.logins.allow(loginClientKey(r)); !allowed {
		seconds := int((retryAfter + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeError(w, http.StatusTooManyRequests, "login_rate_limited")
		return
	}
	start, err := a.service.BeginLogin(r.Context(), correlationID(r.Context()))
	if err != nil {
		http.Redirect(w, r, a.config.ServiceUnavailableURL(), http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.config.LoginCookieName(),
		Value:    start.BrowserBinding,
		Path:     a.config.LoginCookiePath(),
		MaxAge:   int(a.config.LoginTTL.Seconds()),
		HttpOnly: true,
		Secure:   a.config.SecureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, start.AuthorizationURL, http.StatusFound)
}

func (a *API) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	bindingCookie, err := r.Cookie(a.config.LoginCookieName())
	if err != nil {
		a.clearLoginCookie(w)
		http.Redirect(w, r, a.config.InvalidFlowURL(), http.StatusSeeOther)
		return
	}
	if r.URL.Query().Get("error") != "" {
		a.clearLoginCookie(w)
		destination := a.config.AccessDeniedURL()
		if err := a.service.RejectProviderCallback(r.Context(), r.URL.Query().Get("state"), bindingCookie.Value, correlationID(r.Context())); err != nil {
			if errors.Is(err, auth.ErrInvalidLogin) {
				destination = a.config.InvalidFlowURL()
			} else {
				destination = a.config.ServiceUnavailableURL()
			}
		}
		http.Redirect(w, r, destination, http.StatusSeeOther)
		return
	}
	started, err := a.service.CompleteLogin(
		r.Context(),
		r.URL.Query().Get("state"),
		bindingCookie.Value,
		r.URL.Query().Get("code"),
		correlationID(r.Context()),
	)
	a.clearLoginCookie(w)
	if err != nil {
		destination := a.config.ServiceUnavailableURL()
		switch {
		case errors.Is(err, auth.ErrInvalidLogin):
			destination = a.config.InvalidFlowURL()
		case errors.Is(err, auth.ErrIdentityUnknown), errors.Is(err, auth.ErrAccessDenied):
			destination = a.config.AccessDeniedURL()
		}
		http.Redirect(w, r, destination, http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.config.SessionCookieName(),
		Value:    started.SessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.config.SecureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, a.config.LoginSuccessURL(), http.StatusSeeOther)
}

func (a *API) session(w http.ResponseWriter, r *http.Request) {
	session, ok := a.requirePlatform(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"principal": map[string]string{
			"id":           session.PrincipalID.String(),
			"display_name": session.DisplayName,
		},
		"role": auth.PlatformOperatorRole,
		"session": map[string]string{
			"started_at":          session.CreatedAt.UTC().Format(time.RFC3339Nano),
			"idle_expires_at":     session.IdleExpiresAt.UTC().Format(time.RFC3339Nano),
			"absolute_expires_at": session.AbsoluteExpiresAt.UTC().Format(time.RFC3339Nano),
		},
		"csrf_token": a.service.CSRFToken(session),
	})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != a.publicOrigin() {
		writeError(w, http.StatusForbidden, "invalid_origin")
		return
	}
	cookie, err := r.Cookie(a.config.SessionCookieName())
	if err != nil {
		a.clearSessionCookie(w)
		http.Redirect(w, r, a.config.PostLogoutURL(), http.StatusSeeOther)
		return
	}
	session, err := a.service.Authenticate(r.Context(), cookie.Value, correlationID(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrSessionNotFound):
			a.clearSessionCookie(w)
			http.Redirect(w, r, a.config.PostLogoutURL(), http.StatusSeeOther)
		case errors.Is(err, auth.ErrSessionExpired):
			a.clearSessionCookie(w)
			http.Redirect(w, r, a.config.SessionExpiredURL(), http.StatusSeeOther)
		default:
			// Keep the cookie when revocation could not be established. A retry can
			// then end the server-side session instead of only hiding it locally.
			http.Redirect(w, r, a.config.ServiceUnavailableURL(), http.StatusSeeOther)
		}
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, a.config.InvalidFlowURL(), http.StatusSeeOther)
		return
	}
	presented := r.Header.Get("X-CSRF-Token")
	if presented == "" {
		presented = r.PostForm.Get("csrf_token")
	}
	if !a.service.ValidCSRF(session, presented) {
		http.Redirect(w, r, a.config.InvalidFlowURL(), http.StatusSeeOther)
		return
	}
	if _, err := a.service.EndSession(r.Context(), cookie.Value, correlationID(r.Context())); err != nil {
		http.Redirect(w, r, a.config.ServiceUnavailableURL(), http.StatusSeeOther)
		return
	}
	a.clearSessionCookie(w)
	destination := a.config.PostLogoutURL()
	if providerURL, err := a.service.LogoutURL(r.Context(), destination); err == nil && providerURL != "" {
		destination = providerURL
	}
	http.Redirect(w, r, destination, http.StatusSeeOther)
}

func (a *API) auditEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requirePlatform(w, r); !ok {
		return
	}
	query := r.URL.Query()
	period := query.Get("period")
	if period == "" {
		period = "24h"
	}
	if period != "24h" && period != "7d" && period != "30d" {
		writeError(w, http.StatusBadRequest, "invalid_period")
		return
	}
	eventType := strings.TrimSpace(query.Get("event_type"))
	if eventType != "" && !auditEventTypePattern.MatchString(eventType) {
		writeError(w, http.StatusBadRequest, "invalid_event_type")
		return
	}
	result := query.Get("result")
	if result != "" && result != audit.OutcomeSuccess && result != audit.OutcomeDenied && result != audit.OutcomeFailure {
		writeError(w, http.StatusBadRequest, "invalid_result")
		return
	}
	search := strings.TrimSpace(query.Get("q"))
	if len(search) > 120 {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	limit := 25
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = parsed
	}
	cursor, err := audit.DecodeCursor(query.Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	page, err := a.service.ListAuditEvents(r.Context(), cursor, limit, period, eventType, result, search)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable")
		return
	}
	items := make([]auditEventResponse, 0, len(page.Events))
	for _, event := range page.Events {
		items = append(items, presentAuditEvent(event))
	}
	var nextCursor any
	if page.NextCursor != "" {
		nextCursor = page.NextCursor
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nextCursor})
}

func (a *API) auditEvent(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requirePlatform(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("event_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event_id")
		return
	}
	event, err := a.service.GetAuditEvent(r.Context(), id)
	if errors.Is(err, audit.ErrNotFound) {
		writeError(w, http.StatusNotFound, "audit_event_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit_unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, presentAuditEvent(event))
}

func (a *API) requirePlatform(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	cookie, err := r.Cookie(a.config.SessionCookieName())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not_authenticated")
		return auth.Session{}, false
	}
	session, err := a.service.Authenticate(r.Context(), cookie.Value, correlationID(r.Context()))
	if err != nil {
		a.clearSessionCookie(w)
		writeError(w, http.StatusUnauthorized, authenticationErrorCode(err))
		return auth.Session{}, false
	}
	if err := a.service.AuthorizePlatform(r.Context(), session, correlationID(r.Context())); err != nil {
		if errors.Is(err, auth.ErrAccessDenied) {
			a.clearSessionCookie(w)
			writeError(w, http.StatusForbidden, "access_denied")
		} else {
			writeError(w, http.StatusServiceUnavailable, "authorization_unavailable")
		}
		return auth.Session{}, false
	}
	w.Header().Set(sessionIdleExpiresHeader, session.IdleExpiresAt.UTC().Format(time.RFC3339Nano))
	return session, true
}

func (a *API) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: a.config.SessionCookieName(), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.config.SecureCookies(), SameSite: http.SameSiteLaxMode,
	})
}

func (a *API) clearLoginCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: a.config.LoginCookieName(), Value: "", Path: a.config.LoginCookiePath(), MaxAge: -1,
		HttpOnly: true, Secure: a.config.SecureCookies(), SameSite: http.SameSiteLaxMode,
	})
}

func (a *API) publicOrigin() string {
	return a.config.PublicURL.Scheme + "://" + a.config.PublicURL.Host
}

func (a *API) correlations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.Must(uuid.NewV7())
		w.Header().Set("X-Correlation-ID", id.String())
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), correlationKey, id)))
	})
}

func (a *API) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		response := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(response, r)
		a.logger.InfoContext(r.Context(), "request completed",
			"method", r.Method,
			"route", safeRoute(r.Pattern),
			"status", response.status,
			"duration_ms", time.Since(started).Milliseconds(),
			"correlation_id", correlationID(r.Context()).String(),
		)
	})
}

func (a *API) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if a.config.SecureCookies() {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) apiNoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				a.logger.ErrorContext(r.Context(), "request panic", "correlation_id", correlationID(r.Context()).String())
				writeError(w, http.StatusInternalServerError, "internal_error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

type auditEventResponse struct {
	ID            string              `json:"id"`
	EventType     string              `json:"event_type"`
	OccurredAt    string              `json:"occurred_at"`
	Actor         *auditActorResponse `json:"actor"`
	Role          *string             `json:"role"`
	Context       string              `json:"context"`
	Result        string              `json:"result"`
	ReasonCode    *string             `json:"reason_code"`
	CorrelationID string              `json:"correlation_id"`
	Source        string              `json:"source"`
}

type auditActorResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

func presentAuditEvent(event audit.Event) auditEventResponse {
	response := auditEventResponse{
		ID:            event.ID.String(),
		EventType:     event.Type,
		OccurredAt:    event.OccurredAt.UTC().Format(time.RFC3339Nano),
		Context:       auditContext(event),
		Result:        event.Outcome,
		CorrelationID: event.CorrelationID.String(),
		Source:        event.Source,
	}
	if event.ActorPrincipalID != nil {
		response.Actor = &auditActorResponse{ID: event.ActorPrincipalID.String(), DisplayName: event.ActorDisplayName}
	}
	if event.ActorRole != "" {
		role := event.ActorRole
		response.Role = &role
	}
	if event.ReasonCode != "" {
		reason := event.ReasonCode
		response.ReasonCode = &reason
	}
	return response
}

func auditContext(event audit.Event) string {
	labels := map[string]string{
		"security.session_started": "Sessão web",
		"security.session_ended":   "Sessão web",
		"security.session_expired": "Sessão web",
		"security.login_rejected":  "Fluxo de entrada",
		"security.access_denied":   "Console de operação",
		"security.identity_linked": "Identidade externa",
		"security.grant_created":   "Concessão de acesso",
	}
	if label := labels[event.Type]; label != "" {
		return label
	}
	if event.ResourceType != "" {
		return event.ResourceType
	}
	return "Segurança da plataforma"
}

func authenticationErrorCode(err error) string {
	if errors.Is(err, auth.ErrSessionExpired) {
		return "session_expired"
	}
	return "not_authenticated"
}

func correlationID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(correlationKey).(uuid.UUID)
	return id
}

func safeRoute(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if len(pattern) > 160 {
		return pattern[:160]
	}
	return pattern
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
