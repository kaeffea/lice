package postgres

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaeffea/lice/apps/api/internal/audit"
	"github.com/kaeffea/lice/apps/api/internal/auth"
)

func integrationStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("LICE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("LICE_TEST_DATABASE_URL is not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open integration store: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

func TestIntegrationLoginTransactionIsBoundExpiringAndSingleUse(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	transaction := auth.LoginTransaction{
		ID: uuid.Must(uuid.NewV7()), StateDigest: bytes.Repeat([]byte{1}, 32),
		NonceDigest: bytes.Repeat([]byte{2}, 32), BrowserBindingDigest: bytes.Repeat([]byte{3}, 32),
		PKCEVerifierCiphertext: bytes.Repeat([]byte{4}, 48), CryptoKeyID: "integration-v1",
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := store.CreateLoginTransaction(ctx, transaction); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM identity.login_transactions WHERE id = $1`, transaction.ID)
	})
	if _, err := store.ConsumeLoginTransaction(ctx, transaction.StateDigest, bytes.Repeat([]byte{9}, 32), now); !errors.Is(err, auth.ErrInvalidLogin) {
		t.Fatalf("wrong binding error = %v", err)
	}

	var winners atomic.Int32
	var unexpected atomic.Int32
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := store.ConsumeLoginTransaction(ctx, transaction.StateDigest, transaction.BrowserBindingDigest, now)
			switch {
			case err == nil:
				winners.Add(1)
			case !errors.Is(err, auth.ErrInvalidLogin):
				unexpected.Add(1)
			}
		}()
	}
	group.Wait()
	if winners.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("consume winners=%d unexpected=%d", winners.Load(), unexpected.Load())
	}

	expired := transaction
	expired.ID = uuid.Must(uuid.NewV7())
	expired.StateDigest = bytes.Repeat([]byte{5}, 32)
	expired.CreatedAt = now.Add(-time.Minute)
	expired.ExpiresAt = now
	if err := store.CreateLoginTransaction(ctx, expired); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM identity.login_transactions WHERE id = $1`, expired.ID)
	})
	if _, err := store.ConsumeLoginTransaction(ctx, expired.StateDigest, expired.BrowserBindingDigest, now); !errors.Is(err, auth.ErrInvalidLogin) {
		t.Fatalf("closed expiry deadline error = %v", err)
	}
}

func TestIntegrationCreateSessionRollsBackWhenAuditFails(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	principalID, identityID := insertIntegrationIdentity(t, store)
	now := time.Now().UTC()
	session := auth.Session{
		ID: uuid.Must(uuid.NewV7()), PrincipalID: principalID, ExternalIdentityID: identityID,
		CreatedAt: now, LastSeenAt: now, IdleExpiresAt: now.Add(time.Minute),
		AbsoluteExpiresAt: now.Add(time.Hour),
	}
	invalidEvent := audit.Event{
		ID: uuid.Must(uuid.NewV7()), Type: "INVALID", Version: 1, OccurredAt: now,
		Source: "api", Outcome: audit.OutcomeSuccess, CorrelationID: uuid.New(),
	}
	if err := store.CreateSession(ctx, session, bytes.Repeat([]byte{6}, 32), invalidEvent); err == nil {
		t.Fatal("CreateSession accepted an invalid audit event")
	}
	var sessions int
	var lastAuthenticated *time.Time
	if err := store.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM identity.web_sessions WHERE id = $1),
			(SELECT last_authenticated_at FROM identity.external_identities WHERE id = $2)
	`, session.ID, identityID).Scan(&sessions, &lastAuthenticated); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 || lastAuthenticated != nil {
		t.Fatalf("failed audit left session=%d last_authenticated_at=%v", sessions, lastAuthenticated)
	}
}

func TestIntegrationRevokedGrantEndsSessionAndAuditsOnce(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	principalID, identityID := insertIntegrationIdentity(t, store)
	now := time.Now().UTC()
	grantID := uuid.Must(uuid.NewV7())
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO platform.role_grants (id, principal_id, role_code, valid_from, created_at)
		VALUES ($1, $2, $3, $4, $4)
	`, grantID, principalID, auth.PlatformOperatorRole, now); err != nil {
		t.Fatal(err)
	}
	session := auth.Session{
		ID: uuid.Must(uuid.NewV7()), PrincipalID: principalID, ExternalIdentityID: identityID,
		CreatedAt: now, LastSeenAt: now, IdleExpiresAt: now.Add(time.Minute),
		AbsoluteExpiresAt: now.Add(time.Hour),
	}
	started, err := audit.NewEvent("security.session_started", audit.OutcomeSuccess, "integration_test", now, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	started.ActorPrincipalID = &principalID
	started.ActorSessionID = &session.ID
	if err := store.CreateSession(ctx, session, bytes.Repeat([]byte{7}, 32), started); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE platform.role_grants SET revoked_at = $2 WHERE id = $1`, grantID, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	var group sync.WaitGroup
	var allowed atomic.Int32
	var failures atomic.Int32
	errorChannel := make(chan error, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			ok, err := store.AuthorizePlatformSession(ctx, session, now.Add(2*time.Second), uuid.New())
			if err != nil {
				failures.Add(1)
				errorChannel <- err
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	group.Wait()
	close(errorChannel)
	if allowed.Load() != 0 || failures.Load() != 0 {
		t.Fatalf("authorization allowed=%d failures=%d first_error=%v", allowed.Load(), failures.Load(), <-errorChannel)
	}
	var endedReason string
	if err := store.pool.QueryRow(ctx, `SELECT end_reason FROM identity.web_sessions WHERE id = $1`, session.ID).Scan(&endedReason); err != nil {
		t.Fatal(err)
	}
	var denials int
	if err := store.pool.QueryRow(ctx, `
		SELECT count(*) FROM audit.events
		WHERE actor_session_id = $1 AND event_type = 'security.access_denied'
	`, session.ID).Scan(&denials); err != nil {
		t.Fatal(err)
	}
	if endedReason != "revoked" || denials != 1 {
		t.Fatalf("ended reason=%q access-denied events=%d", endedReason, denials)
	}
}

func insertIntegrationIdentity(t *testing.T, store *Store) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	principalID := uuid.Must(uuid.NewV7())
	identityID := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO identity.principals (id, display_name, status, created_at)
		VALUES ($1, 'Integration test principal', 'active', $2)
	`, principalID, now); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		_, _ = store.pool.Exec(cleanup, `DELETE FROM audit.events WHERE actor_principal_id = $1`, principalID)
		_, _ = store.pool.Exec(cleanup, `DELETE FROM platform.role_grants WHERE principal_id = $1`, principalID)
		_, _ = store.pool.Exec(cleanup, `DELETE FROM identity.web_sessions WHERE principal_id = $1`, principalID)
		_, _ = store.pool.Exec(cleanup, `DELETE FROM identity.external_identities WHERE principal_id = $1`, principalID)
		_, _ = store.pool.Exec(cleanup, `DELETE FROM identity.principals WHERE id = $1`, principalID)
	})
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO identity.external_identities (id, principal_id, issuer, subject, created_at)
		VALUES ($1, $2, 'https://integration.invalid/realms/lice', $3, $4)
	`, identityID, principalID, identityID.String(), now); err != nil {
		t.Fatal(err)
	}
	return principalID, identityID
}
