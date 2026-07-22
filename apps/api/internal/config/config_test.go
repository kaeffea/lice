package config

import (
	"strings"
	"testing"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("LICE_ENVIRONMENT", "local")
	t.Setenv("LICE_HTTP_ADDR", ":9090")
	t.Setenv("LICE_DATABASE_URL", "postgres://runtime:secret@postgres:5432/lice")
	t.Setenv("LICE_PUBLIC_URL", "http://lice.localhost:8080")
	t.Setenv("LICE_OIDC_ISSUER", "http://auth.lice.localhost:8080/realms/lice")
	t.Setenv("LICE_OIDC_CLIENT_ID", "lice-web")
	t.Setenv("LICE_OIDC_CLIENT_SECRET", "client-secret")
	t.Setenv("LICE_SESSION_HASH_KEY", strings.Repeat("01", 32))
	t.Setenv("LICE_LOGIN_ENCRYPTION_KEY", strings.Repeat("02", 32))
	t.Setenv("LICE_CSRF_KEY", strings.Repeat("03", 32))
}

func TestLoadAcceptsExplicitLocalConfiguration(t *testing.T) {
	setValidEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.SecureCookies() {
		t.Fatalf("unexpected local HTTP configuration: %#v", cfg)
	}
	if len(cfg.SessionHashKey) != 32 || len(cfg.LoginEncryptionKey) != 32 || len(cfg.CSRFKey) != 32 {
		t.Fatal("hexadecimal keys were not decoded to 32 bytes")
	}
	if cfg.CallbackURL() != "http://lice.localhost:8080/api/v1/auth/callback" {
		t.Fatalf("CallbackURL() = %q", cfg.CallbackURL())
	}
	if cfg.PostLogoutURL() != "http://lice.localhost:8080/acesso/sessao-encerrada" {
		t.Fatalf("PostLogoutURL() = %q", cfg.PostLogoutURL())
	}
}

func TestLoadFailsClosedWithoutExplicitEnvironment(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("LICE_ENVIRONMENT", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted an empty LICE_ENVIRONMENT")
	}
}

func TestLoadRejectsHTTPInProduction(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("LICE_ENVIRONMENT", "production")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted HTTP URLs in production")
	}
}

func TestLoadRequiresExactly64HexadecimalCharactersForEveryKey(t *testing.T) {
	for _, keyName := range []string{"LICE_SESSION_HASH_KEY", "LICE_LOGIN_ENCRYPTION_KEY", "LICE_CSRF_KEY"} {
		t.Run(keyName, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(keyName, strings.Repeat("g", 64))
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted a non-hexadecimal %s", keyName)
			}
		})
	}
}

func TestProductionUsesHostPrefixedCookies(t *testing.T) {
	cfg := Config{Environment: "production"}
	if cfg.SessionCookieName() != "__Host-lice_session" || cfg.LoginCookieName() != "__Host-lice_oidc_tx" {
		t.Fatalf("unexpected production cookie names: %q, %q", cfg.SessionCookieName(), cfg.LoginCookieName())
	}
	if cfg.LoginCookiePath() != "/" {
		t.Fatalf("__Host- login cookie path = %q, want /", cfg.LoginCookiePath())
	}
}

func TestLocalLoginCookieKeepsCallbackOnlyPath(t *testing.T) {
	cfg := Config{Environment: "local"}
	if cfg.LoginCookiePath() != "/api/v1/auth/callback" {
		t.Fatalf("local login cookie path = %q", cfg.LoginCookiePath())
	}
}
