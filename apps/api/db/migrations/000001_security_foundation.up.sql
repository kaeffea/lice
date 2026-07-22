DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lice_runtime') THEN
        CREATE ROLE lice_runtime NOLOGIN;
    END IF;
END
$$;

CREATE SCHEMA identity;
CREATE SCHEMA platform;
CREATE SCHEMA audit;

REVOKE ALL ON SCHEMA identity, platform, audit FROM PUBLIC;

CREATE TABLE identity.principals (
    id uuid PRIMARY KEY,
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 200),
    status text NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL,
    disabled_at timestamptz,
    CONSTRAINT principals_disabled_state_check CHECK (
        (status = 'active' AND disabled_at IS NULL)
        OR (status = 'disabled' AND disabled_at IS NOT NULL)
    )
);

CREATE TABLE identity.external_identities (
    id uuid PRIMARY KEY,
    principal_id uuid NOT NULL REFERENCES identity.principals(id) ON DELETE RESTRICT,
    issuer text NOT NULL CHECK (char_length(issuer) BETWEEN 1 AND 2048),
    subject text NOT NULL CHECK (octet_length(subject) BETWEEN 1 AND 255),
    created_at timestamptz NOT NULL,
    last_authenticated_at timestamptz,
    CONSTRAINT external_identities_id_principal_key UNIQUE (id, principal_id),
    CONSTRAINT external_identities_issuer_subject_key UNIQUE (issuer, subject)
);

CREATE INDEX external_identities_principal_idx
    ON identity.external_identities (principal_id);

CREATE TABLE identity.login_transactions (
    id uuid PRIMARY KEY,
    state_digest bytea NOT NULL UNIQUE CHECK (octet_length(state_digest) = 32),
    nonce_digest bytea NOT NULL CHECK (octet_length(nonce_digest) = 32),
    browser_binding_digest bytea NOT NULL CHECK (octet_length(browser_binding_digest) = 32),
    pkce_verifier_ciphertext bytea NOT NULL CHECK (octet_length(pkce_verifier_ciphertext) > 28),
    crypto_key_id text NOT NULL CHECK (char_length(crypto_key_id) BETWEEN 1 AND 128),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    CONSTRAINT login_transactions_time_check CHECK (
        expires_at > created_at
        AND (consumed_at IS NULL OR consumed_at >= created_at)
    )
);

CREATE INDEX login_transactions_expiry_idx
    ON identity.login_transactions (expires_at)
    WHERE consumed_at IS NULL;

CREATE TABLE identity.web_sessions (
    id uuid PRIMARY KEY,
    session_digest bytea NOT NULL UNIQUE CHECK (octet_length(session_digest) = 32),
    principal_id uuid NOT NULL REFERENCES identity.principals(id) ON DELETE RESTRICT,
    external_identity_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    idle_expires_at timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    ended_at timestamptz,
    end_reason text CHECK (end_reason IN ('logout', 'idle_timeout', 'absolute_timeout', 'revoked')),
    CONSTRAINT web_sessions_identity_principal_fk
        FOREIGN KEY (external_identity_id, principal_id)
        REFERENCES identity.external_identities(id, principal_id) ON DELETE RESTRICT,
    CONSTRAINT web_sessions_time_check CHECK (
        last_seen_at >= created_at
        AND idle_expires_at > created_at
        AND absolute_expires_at > created_at
        AND idle_expires_at <= absolute_expires_at
        AND (
            (ended_at IS NULL AND end_reason IS NULL)
            OR (ended_at IS NOT NULL AND end_reason IS NOT NULL AND ended_at >= created_at)
        )
    )
);

CREATE INDEX web_sessions_principal_idx
    ON identity.web_sessions (principal_id);

CREATE INDEX web_sessions_expiry_idx
    ON identity.web_sessions (idle_expires_at, absolute_expires_at)
    WHERE ended_at IS NULL;

CREATE TABLE platform.role_grants (
    id uuid PRIMARY KEY,
    principal_id uuid NOT NULL REFERENCES identity.principals(id) ON DELETE RESTRICT,
    role_code text NOT NULL CHECK (role_code = 'platform_operator'),
    valid_from timestamptz NOT NULL,
    valid_until timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL,
    CONSTRAINT platform_role_grants_time_check CHECK (
        (valid_until IS NULL OR valid_until > valid_from)
        AND (revoked_at IS NULL OR revoked_at >= created_at)
    )
);

CREATE UNIQUE INDEX platform_role_grants_active_key
    ON platform.role_grants (principal_id, role_code)
    WHERE revoked_at IS NULL;

CREATE INDEX platform_role_grants_principal_idx
    ON platform.role_grants (principal_id, valid_from, valid_until);

CREATE TABLE audit.events (
    id uuid PRIMARY KEY,
    event_type text NOT NULL CHECK (event_type ~ '^[a-z][a-z0-9_.]{2,127}$'),
    event_version smallint NOT NULL CHECK (event_version > 0),
    occurred_at timestamptz NOT NULL,
    actor_principal_id uuid REFERENCES identity.principals(id) ON DELETE RESTRICT,
    actor_session_id uuid REFERENCES identity.web_sessions(id) ON DELETE RESTRICT,
    actor_role text CHECK (actor_role IS NULL OR actor_role = 'platform_operator'),
    source text NOT NULL CHECK (source IN ('api', 'admin', 'system')),
    outcome text NOT NULL CHECK (outcome IN ('success', 'denied', 'failure')),
    reason_code text CHECK (reason_code ~ '^[a-z][a-z0-9_.-]{1,127}$'),
    correlation_id uuid NOT NULL,
    resource_type text CHECK (resource_type IS NULL OR resource_type ~ '^[a-z][a-z0-9_.-]{1,127}$'),
    resource_id uuid,
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object')
);

CREATE INDEX audit_events_timeline_idx
    ON audit.events (occurred_at DESC, id DESC);

CREATE INDEX audit_events_actor_idx
    ON audit.events (actor_principal_id, occurred_at DESC);

CREATE INDEX audit_events_type_idx
    ON audit.events (event_type, occurred_at DESC);

GRANT USAGE ON SCHEMA identity, platform, audit TO lice_runtime;

GRANT SELECT ON identity.principals, identity.external_identities, platform.role_grants
    TO lice_runtime;
GRANT SELECT, INSERT ON identity.login_transactions, identity.web_sessions
    TO lice_runtime;
GRANT DELETE ON identity.login_transactions TO lice_runtime;
GRANT UPDATE (consumed_at) ON identity.login_transactions TO lice_runtime;
GRANT UPDATE (last_authenticated_at) ON identity.external_identities TO lice_runtime;
GRANT UPDATE (last_seen_at, idle_expires_at, ended_at, end_reason)
    ON identity.web_sessions TO lice_runtime;
GRANT SELECT, INSERT ON audit.events TO lice_runtime;

REVOKE UPDATE, DELETE, TRUNCATE ON audit.events FROM lice_runtime;
