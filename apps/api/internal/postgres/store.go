package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kaeffea/lice/apps/api/internal/audit"
	"github.com/kaeffea/lice/apps/api/internal/auth"
)

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("database URL is invalid")
	}
	config.MaxConns = 12
	config.MinConns = 1
	config.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("could not initialize database pool")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("database is unavailable")
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close()                         { s.pool.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) CreateLoginTransaction(ctx context.Context, transaction auth.LoginTransaction) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		WITH removable AS (
			SELECT id
			FROM identity.login_transactions
			WHERE expires_at <= $1 OR consumed_at IS NOT NULL
			ORDER BY expires_at
			LIMIT 128
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM identity.login_transactions AS transaction
		USING removable
		WHERE transaction.id = removable.id`, transaction.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.login_transactions (
			id, state_digest, nonce_digest, browser_binding_digest,
			pkce_verifier_ciphertext, crypto_key_id, created_at, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		transaction.ID, transaction.StateDigest, transaction.NonceDigest,
		transaction.BrowserBindingDigest, transaction.PKCEVerifierCiphertext,
		transaction.CryptoKeyID, transaction.CreatedAt, transaction.ExpiresAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ConsumeLoginTransaction(ctx context.Context, stateDigest, bindingDigest []byte, now time.Time) (auth.LoginTransaction, error) {
	var transaction auth.LoginTransaction
	err := s.pool.QueryRow(ctx, `
		UPDATE identity.login_transactions
		SET consumed_at = $3
		WHERE state_digest = $1
		  AND browser_binding_digest = $2
		  AND consumed_at IS NULL
		  AND expires_at > $3
		RETURNING id, state_digest, nonce_digest, browser_binding_digest,
		          pkce_verifier_ciphertext, crypto_key_id, created_at, expires_at, consumed_at`,
		stateDigest, bindingDigest, now,
	).Scan(
		&transaction.ID, &transaction.StateDigest, &transaction.NonceDigest,
		&transaction.BrowserBindingDigest, &transaction.PKCEVerifierCiphertext,
		&transaction.CryptoKeyID, &transaction.CreatedAt, &transaction.ExpiresAt,
		&transaction.ConsumedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.LoginTransaction{}, auth.ErrInvalidLogin
	}
	return transaction, err
}

func (s *Store) FindExternalIdentity(ctx context.Context, issuer, subject string) (auth.ExternalIdentity, error) {
	var identity auth.ExternalIdentity
	err := s.pool.QueryRow(ctx, `
		SELECT id, principal_id, issuer, subject
		FROM identity.external_identities
		WHERE issuer = $1 AND subject = $2`, issuer, subject,
	).Scan(&identity.ID, &identity.PrincipalID, &identity.Issuer, &identity.Subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ExternalIdentity{}, auth.ErrIdentityUnknown
	}
	return identity, err
}

func (s *Store) CreateSession(ctx context.Context, session auth.Session, digest []byte, event audit.Event) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.web_sessions (
			id, session_digest, principal_id, external_identity_id,
			created_at, last_seen_at, idle_expires_at, absolute_expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		session.ID, digest, session.PrincipalID, session.ExternalIdentityID,
		session.CreatedAt, session.LastSeenAt, session.IdleExpiresAt, session.AbsoluteExpiresAt,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.external_identities
		SET last_authenticated_at = $2
		WHERE id = $1`, session.ExternalIdentityID, session.CreatedAt,
	); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ResolveSession(ctx context.Context, digest []byte, now time.Time, idleTTL time.Duration, correlationID uuid.UUID) (auth.Session, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return auth.Session{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var session auth.Session
	var principalStatus string
	var endedAt pgtype.Timestamptz
	var endReason pgtype.Text
	err = tx.QueryRow(ctx, `
		SELECT s.id, s.principal_id, s.external_identity_id, s.created_at,
		       s.last_seen_at, s.idle_expires_at, s.absolute_expires_at,
		       s.ended_at, s.end_reason, p.status, p.display_name
		FROM identity.web_sessions s
		JOIN identity.principals p ON p.id = s.principal_id
		WHERE s.session_digest = $1
		FOR UPDATE OF s`, digest,
	).Scan(
		&session.ID, &session.PrincipalID, &session.ExternalIdentityID, &session.CreatedAt,
		&session.LastSeenAt, &session.IdleExpiresAt, &session.AbsoluteExpiresAt,
		&endedAt, &endReason, &principalStatus, &session.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Session{}, auth.ErrSessionNotFound
	}
	if err != nil {
		return auth.Session{}, err
	}
	if endedAt.Valid {
		if endReason.String == "idle_timeout" || endReason.String == "absolute_timeout" {
			return auth.Session{}, auth.ErrSessionExpired
		}
		return auth.Session{}, auth.ErrSessionNotFound
	}
	if principalStatus != "active" {
		if err := endLockedSession(ctx, tx, session, now, "revoked", "principal_disabled", correlationID); err != nil {
			return auth.Session{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return auth.Session{}, err
		}
		return auth.Session{}, auth.ErrSessionNotFound
	}

	reason := sessionExpirationReason(now, session.IdleExpiresAt, session.AbsoluteExpiresAt)
	if reason != "" {
		if err := endLockedSession(ctx, tx, session, now, reason, reason, correlationID); err != nil {
			return auth.Session{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return auth.Session{}, err
		}
		return auth.Session{}, auth.ErrSessionExpired
	}

	newIdleExpiry := nextIdleExpiry(now, idleTTL, session.AbsoluteExpiresAt)
	if _, err := tx.Exec(ctx, `
		UPDATE identity.web_sessions
		SET last_seen_at = $2, idle_expires_at = $3
		WHERE id = $1`, session.ID, now, newIdleExpiry,
	); err != nil {
		return auth.Session{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return auth.Session{}, err
	}
	session.LastSeenAt = now
	session.IdleExpiresAt = newIdleExpiry
	return session, nil
}

func (s *Store) EndSession(ctx context.Context, digest []byte, now time.Time, correlationID uuid.UUID) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var session auth.Session
	var endedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `
		SELECT id, principal_id, external_identity_id, created_at, last_seen_at,
		       idle_expires_at, absolute_expires_at, ended_at
		FROM identity.web_sessions
		WHERE session_digest = $1
		FOR UPDATE`, digest,
	).Scan(
		&session.ID, &session.PrincipalID, &session.ExternalIdentityID, &session.CreatedAt,
		&session.LastSeenAt, &session.IdleExpiresAt, &session.AbsoluteExpiresAt, &endedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if endedAt.Valid {
		return false, nil
	}
	if err := endLockedSession(ctx, tx, session, now, "logout", "user_logout", correlationID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) HasPlatformRole(ctx context.Context, principalID uuid.UUID, role string, now time.Time) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM platform.role_grants AS role_grant
			JOIN identity.principals AS principal ON principal.id = role_grant.principal_id
			WHERE role_grant.principal_id = $1 AND role_grant.role_code = $2
			  AND role_grant.revoked_at IS NULL
			  AND role_grant.valid_from <= $3
			  AND (role_grant.valid_until IS NULL OR role_grant.valid_until > $3)
			  AND principal.status = 'active'
		)`, principalID, role, now,
	).Scan(&allowed)
	return allowed, err
}

func (s *Store) AuthorizePlatformSession(ctx context.Context, session auth.Session, now time.Time, correlationID uuid.UUID) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var endedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `
		SELECT ended_at
		FROM identity.web_sessions
		WHERE id = $1 AND principal_id = $2
		FOR UPDATE`, session.ID, session.PrincipalID,
	).Scan(&endedAt)
	if errors.Is(err, pgx.ErrNoRows) || endedAt.Valid {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var allowed bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM platform.role_grants AS role_grant
			JOIN identity.principals AS principal ON principal.id = role_grant.principal_id
			WHERE role_grant.principal_id = $1
			  AND role_grant.role_code = $2
			  AND role_grant.revoked_at IS NULL
			  AND role_grant.valid_from <= $3
			  AND (role_grant.valid_until IS NULL OR role_grant.valid_until > $3)
			  AND principal.status = 'active'
		)`, session.PrincipalID, auth.PlatformOperatorRole, now,
	).Scan(&allowed); err != nil {
		return false, err
	}
	if allowed {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := endLockedSession(ctx, tx, session, now, "revoked", "platform_grant_missing", correlationID); err != nil {
		return false, err
	}
	event, err := audit.NewEvent("security.access_denied", audit.OutcomeDenied, "platform_grant_missing", now, correlationID)
	if err != nil {
		return false, err
	}
	event.ActorPrincipalID = &session.PrincipalID
	event.ActorSessionID = &session.ID
	event.ActorRole = auth.PlatformOperatorRole
	event.ResourceType = "platform_console"
	if err := insertAudit(ctx, tx, event); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return false, nil
}

func (s *Store) RecordAudit(ctx context.Context, event audit.Event) error {
	return insertAudit(ctx, s.pool, event)
}

func (s *Store) ListAuditEvents(ctx context.Context, cursor audit.Cursor, limit int, filter audit.Filter) (audit.Page, error) {
	if limit < 1 || limit > 100 {
		return audit.Page{}, errors.New("audit page limit is invalid")
	}
	query := `
		SELECT e.id, e.event_type, e.event_version, e.occurred_at,
		       e.actor_principal_id, p.display_name, e.actor_session_id,
		       e.actor_role, e.source, e.outcome, e.reason_code, e.correlation_id,
		       e.resource_type, e.resource_id, e.details
		FROM audit.events e
		LEFT JOIN identity.principals p ON p.id = e.actor_principal_id`
	args := []any{}
	conditions := make([]string, 0, 5)
	if !filter.Since.IsZero() {
		args = append(args, filter.Since)
		conditions = append(conditions, fmt.Sprintf("e.occurred_at >= $%d", len(args)))
	}
	if filter.EventType != "" {
		args = append(args, filter.EventType)
		conditions = append(conditions, fmt.Sprintf("e.event_type = $%d", len(args)))
	}
	if filter.Outcome != "" {
		args = append(args, filter.Outcome)
		conditions = append(conditions, fmt.Sprintf("e.outcome = $%d", len(args)))
	}
	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		position := len(args)
		conditions = append(conditions, fmt.Sprintf("(e.event_type ILIKE $%[1]d OR COALESCE(e.reason_code, '') ILIKE $%[1]d OR COALESCE(p.display_name, '') ILIKE $%[1]d OR e.correlation_id::text ILIKE $%[1]d)", position))
	}
	if cursor.ID != uuid.Nil {
		args = append(args, cursor.OccurredAt, cursor.ID)
		conditions = append(conditions, fmt.Sprintf("(e.occurred_at, e.id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	if len(conditions) != 0 {
		query += ` WHERE ` + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY e.occurred_at DESC, e.id DESC LIMIT $` + fmt.Sprint(len(args)+1)
	args = append(args, limit+1)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return audit.Page{}, err
	}
	defer rows.Close()
	events := make([]audit.Event, 0, limit+1)
	for rows.Next() {
		event, err := scanAudit(rows)
		if err != nil {
			return audit.Page{}, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return audit.Page{}, err
	}
	page := audit.Page{Events: events}
	if len(events) > limit {
		page.Events = events[:limit]
		last := page.Events[len(page.Events)-1]
		page.NextCursor = audit.EncodeCursor(audit.Cursor{OccurredAt: last.OccurredAt, ID: last.ID})
	}
	return page, nil
}

func (s *Store) GetAuditEvent(ctx context.Context, id uuid.UUID) (audit.Event, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT e.id, e.event_type, e.event_version, e.occurred_at,
		       e.actor_principal_id, p.display_name, e.actor_session_id,
		       e.actor_role, e.source, e.outcome, e.reason_code, e.correlation_id,
		       e.resource_type, e.resource_id, e.details
		FROM audit.events e
		LEFT JOIN identity.principals p ON p.id = e.actor_principal_id
		WHERE e.id = $1`, id)
	event, err := scanAudit(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return audit.Event{}, audit.ErrNotFound
	}
	return event, err
}

func (s *Store) BootstrapIdentity(ctx context.Context, issuer, subject, displayName string, grant bool, now time.Time, correlationID uuid.UUID) (auth.BootstrapResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return auth.BootstrapResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, hashtextextended($2, 0)))`, issuer, subject); err != nil {
		return auth.BootstrapResult{}, err
	}

	var result auth.BootstrapResult
	err = tx.QueryRow(ctx, `
		SELECT e.principal_id, e.id
		FROM identity.external_identities e
		WHERE e.issuer = $1 AND e.subject = $2`, issuer, subject,
	).Scan(&result.PrincipalID, &result.IdentityID)
	if errors.Is(err, pgx.ErrNoRows) {
		result.PrincipalID = uuid.Must(uuid.NewV7())
		result.IdentityID = uuid.Must(uuid.NewV7())
		if _, err := tx.Exec(ctx, `
			INSERT INTO identity.principals (id, display_name, status, created_at)
			VALUES ($1,$2,'active',$3)`, result.PrincipalID, displayName, now); err != nil {
			return auth.BootstrapResult{}, err
		}
		result.PrincipalMade = true
		if _, err := tx.Exec(ctx, `
			INSERT INTO identity.external_identities (id, principal_id, issuer, subject, created_at)
			VALUES ($1,$2,$3,$4,$5)`, result.IdentityID, result.PrincipalID, issuer, subject, now); err != nil {
			return auth.BootstrapResult{}, err
		}
		result.IdentityMade = true
		event := mustAudit("security.identity_linked", audit.OutcomeSuccess, "bootstrap_demo", now, correlationID)
		event.ResourceType = "external_identity"
		event.ResourceID = &result.IdentityID
		event.Source = "admin"
		if err := insertAudit(ctx, tx, event); err != nil {
			return auth.BootstrapResult{}, err
		}
	} else if err != nil {
		return auth.BootstrapResult{}, err
	}

	if grant {
		grantID := uuid.Must(uuid.NewV7())
		err = tx.QueryRow(ctx, `
			INSERT INTO platform.role_grants (id, principal_id, role_code, valid_from, created_at)
			VALUES ($1,$2,$3,$4,$4)
			ON CONFLICT (principal_id, role_code) WHERE revoked_at IS NULL DO NOTHING
			RETURNING id`, grantID, result.PrincipalID, auth.PlatformOperatorRole, now,
		).Scan(&grantID)
		if err == nil {
			result.GrantMade = true
			event := mustAudit("security.grant_created", audit.OutcomeSuccess, "bootstrap_demo", now, correlationID)
			event.ResourceType = "role_grant"
			event.ResourceID = &grantID
			event.Source = "admin"
			if err := insertAudit(ctx, tx, event); err != nil {
				return auth.BootstrapResult{}, err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return auth.BootstrapResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return auth.BootstrapResult{}, err
	}
	return result, nil
}

type rowScanner interface {
	Scan(...any) error
}

type auditExecer interface {
	Exec(context.Context, string, ...any) (pgconnCommandTag, error)
}

// pgx.Tx and pgxpool.Pool expose compatible Exec methods; this narrow alias keeps
// the audit insert usable in both transaction and pool contexts.
type pgconnCommandTag = pgconn.CommandTag

func insertAudit(ctx context.Context, executor auditExecer, event audit.Event) error {
	details := event.Details
	if details == nil {
		details = map[string]any{}
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode audit context: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO audit.events (
			id, event_type, event_version, occurred_at,
			actor_principal_id, actor_session_id, actor_role, source,
			outcome, reason_code, correlation_id,
			resource_type, resource_id, details
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,NULLIF($12,''),$13,$14)`,
		event.ID, event.Type, event.Version, event.OccurredAt,
		event.ActorPrincipalID, event.ActorSessionID, nullableString(event.ActorRole), event.Source,
		event.Outcome, event.ReasonCode, event.CorrelationID,
		event.ResourceType, event.ResourceID, encoded,
	)
	return err
}

func endLockedSession(ctx context.Context, tx pgx.Tx, session auth.Session, now time.Time, endReason, auditReason string, correlationID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE identity.web_sessions
		SET ended_at = $2, end_reason = $3
		WHERE id = $1 AND ended_at IS NULL`, session.ID, now, endReason,
	); err != nil {
		return err
	}
	eventType := "security.session_ended"
	if endReason == "idle_timeout" || endReason == "absolute_timeout" {
		eventType = "security.session_expired"
	}
	event := mustAudit(eventType, audit.OutcomeSuccess, auditReason, now, correlationID)
	event.ActorPrincipalID = &session.PrincipalID
	event.ActorSessionID = &session.ID
	event.ResourceType = "web_session"
	event.ResourceID = &session.ID
	return insertAudit(ctx, tx, event)
}

func mustAudit(eventType, outcome, reason string, now time.Time, correlationID uuid.UUID) audit.Event {
	event, err := audit.NewEvent(eventType, outcome, reason, now, correlationID)
	if err != nil {
		panic(err)
	}
	return event
}

func scanAudit(scanner rowScanner) (audit.Event, error) {
	var event audit.Event
	var actorPrincipal, actorSession, resourceID pgtype.UUID
	var actorName, actorRole, reason, resourceType pgtype.Text
	var details []byte
	err := scanner.Scan(
		&event.ID, &event.Type, &event.Version, &event.OccurredAt,
		&actorPrincipal, &actorName, &actorSession, &actorRole,
		&event.Source, &event.Outcome, &reason, &event.CorrelationID,
		&resourceType, &resourceID, &details,
	)
	if err != nil {
		return audit.Event{}, err
	}
	if actorPrincipal.Valid {
		value := uuid.UUID(actorPrincipal.Bytes)
		event.ActorPrincipalID = &value
	}
	if actorSession.Valid {
		value := uuid.UUID(actorSession.Bytes)
		event.ActorSessionID = &value
	}
	if resourceID.Valid {
		value := uuid.UUID(resourceID.Bytes)
		event.ResourceID = &value
	}
	if actorName.Valid {
		event.ActorDisplayName = actorName.String
	}
	if actorRole.Valid {
		event.ActorRole = actorRole.String
	}
	if reason.Valid {
		event.ReasonCode = reason.String
	}
	if resourceType.Valid {
		event.ResourceType = resourceType.String
	}
	if err := json.Unmarshal(details, &event.Details); err != nil {
		return audit.Event{}, fmt.Errorf("decode audit context: %w", err)
	}
	return event, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func sessionExpirationReason(now, idleExpiresAt, absoluteExpiresAt time.Time) string {
	if !now.Before(absoluteExpiresAt) {
		return "absolute_timeout"
	}
	if !now.Before(idleExpiresAt) {
		return "idle_timeout"
	}
	return ""
}

func nextIdleExpiry(now time.Time, idleTTL time.Duration, absoluteExpiresAt time.Time) time.Time {
	next := now.Add(idleTTL)
	if next.After(absoluteExpiresAt) {
		return absoluteExpiresAt
	}
	return next
}
