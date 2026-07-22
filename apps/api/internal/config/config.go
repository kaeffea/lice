package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultHTTPAddr = ":8080"
	keyBytes        = 32
)

type Config struct {
	Environment        string
	HTTPAddr           string
	DatabaseURL        string
	PublicURL          *url.URL
	OIDCIssuer         string
	OIDCClientID       string
	OIDCClientSecret   string
	SessionHashKey     []byte
	LoginEncryptionKey []byte
	CSRFKey            []byte
	LoginTTL           time.Duration
	SessionIdleTTL     time.Duration
	SessionAbsoluteTTL time.Duration
}

type BootstrapConfig struct {
	Environment string
	DatabaseURL string
	OIDCIssuer  string
}

func Load() (Config, error) {
	var cfg Config
	cfg.Environment = strings.TrimSpace(os.Getenv("LICE_ENVIRONMENT"))
	if cfg.Environment != "local" && cfg.Environment != "production" {
		return Config{}, errors.New("LICE_ENVIRONMENT must be explicitly set to local or production")
	}
	cfg.HTTPAddr = valueOr("LICE_HTTP_ADDR", defaultHTTPAddr)
	cfg.DatabaseURL = strings.TrimSpace(os.Getenv("LICE_DATABASE_URL"))
	cfg.OIDCIssuer = strings.TrimSpace(os.Getenv("LICE_OIDC_ISSUER"))
	cfg.OIDCClientID = strings.TrimSpace(os.Getenv("LICE_OIDC_CLIENT_ID"))
	cfg.OIDCClientSecret = os.Getenv("LICE_OIDC_CLIENT_SECRET")
	cfg.LoginTTL = 5 * time.Minute
	cfg.SessionIdleTTL = 30 * time.Minute
	cfg.SessionAbsoluteTTL = 8 * time.Hour

	missing := make([]string, 0, 5)
	for name, value := range map[string]string{
		"LICE_DATABASE_URL":       cfg.DatabaseURL,
		"LICE_PUBLIC_URL":         strings.TrimSpace(os.Getenv("LICE_PUBLIC_URL")),
		"LICE_OIDC_ISSUER":        cfg.OIDCIssuer,
		"LICE_OIDC_CLIENT_ID":     cfg.OIDCClientID,
		"LICE_OIDC_CLIENT_SECRET": cfg.OIDCClientSecret,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return Config{}, fmt.Errorf("required configuration is missing: %s", strings.Join(missing, ", "))
	}

	publicURL, err := parseAbsoluteURL("LICE_PUBLIC_URL", os.Getenv("LICE_PUBLIC_URL"), cfg.Environment == "local")
	if err != nil {
		return Config{}, err
	}
	issuer, err := parseAbsoluteURL("LICE_OIDC_ISSUER", cfg.OIDCIssuer, cfg.Environment == "local")
	if err != nil {
		return Config{}, err
	}
	cfg.PublicURL = publicURL
	cfg.OIDCIssuer = issuer.String()

	if cfg.SessionHashKey, err = parseHexKey("LICE_SESSION_HASH_KEY"); err != nil {
		return Config{}, err
	}
	if cfg.LoginEncryptionKey, err = parseHexKey("LICE_LOGIN_ENCRYPTION_KEY"); err != nil {
		return Config{}, err
	}
	if cfg.CSRFKey, err = parseHexKey("LICE_CSRF_KEY"); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadBootstrap() (BootstrapConfig, error) {
	cfg := BootstrapConfig{
		Environment: strings.TrimSpace(os.Getenv("LICE_ENVIRONMENT")),
		DatabaseURL: strings.TrimSpace(os.Getenv("LICE_DATABASE_URL")),
		OIDCIssuer:  strings.TrimSpace(os.Getenv("LICE_OIDC_ISSUER")),
	}
	if cfg.DatabaseURL == "" || cfg.OIDCIssuer == "" {
		return BootstrapConfig{}, errors.New("LICE_DATABASE_URL and LICE_OIDC_ISSUER are required")
	}
	if cfg.Environment != "local" && cfg.Environment != "production" {
		return BootstrapConfig{}, errors.New("LICE_ENVIRONMENT must be explicitly set to local or production")
	}
	if _, err := parseAbsoluteURL("LICE_OIDC_ISSUER", cfg.OIDCIssuer, cfg.Environment == "local"); err != nil {
		return BootstrapConfig{}, err
	}
	return cfg, nil
}

func (c Config) CallbackURL() string {
	return strings.TrimRight(c.PublicURL.String(), "/") + "/api/v1/auth/callback"
}

func (c Config) PostLogoutURL() string {
	return strings.TrimRight(c.PublicURL.String(), "/") + "/acesso/sessao-encerrada"
}
func (c Config) LoginSuccessURL() string {
	return strings.TrimRight(c.PublicURL.String(), "/") + "/controle"
}
func (c Config) AccessDeniedURL() string {
	return strings.TrimRight(c.PublicURL.String(), "/") + "/acesso/negado"
}
func (c Config) ServiceUnavailableURL() string {
	return strings.TrimRight(c.PublicURL.String(), "/") + "/acesso/servico-indisponivel"
}
func (c Config) InvalidFlowURL() string {
	return strings.TrimRight(c.PublicURL.String(), "/") + "/acesso/fluxo-invalido"
}
func (c Config) SessionExpiredURL() string {
	return strings.TrimRight(c.PublicURL.String(), "/") + "/acesso/sessao-expirada"
}
func (c Config) SecureCookies() bool { return c.Environment != "local" }

func (c Config) SessionCookieName() string {
	if c.SecureCookies() {
		return "__Host-lice_session"
	}
	return "lice_session"
}

func (c Config) LoginCookieName() string {
	if c.SecureCookies() {
		return "__Host-lice_oidc_tx"
	}
	return "lice_oidc_tx"
}

func (c Config) LoginCookiePath() string {
	if c.SecureCookies() {
		return "/"
	}
	return "/api/v1/auth/callback"
}

func parseHexKey(name string) ([]byte, error) {
	encoded := strings.TrimSpace(os.Getenv(name))
	if len(encoded) != keyBytes*2 {
		return nil, fmt.Errorf("%s must contain exactly 64 hexadecimal characters", name)
	}
	key, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%s must contain exactly 64 hexadecimal characters", name)
	}
	return key, nil
}

func parseAbsoluteURL(name, raw string, allowHTTP bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%s must be an absolute URL without credentials, query, or fragment", name)
	}
	if parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http") {
		return nil, fmt.Errorf("%s must use HTTPS", name)
	}
	return parsed, nil
}

func valueOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
