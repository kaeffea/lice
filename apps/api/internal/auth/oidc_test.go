package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOIDCRequestsMinimumScopeAndBuildsTokenlessLogout(t *testing.T) {
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/authorize",
			"token_endpoint":                        issuer + "/token",
			"jwks_uri":                              issuer + "/keys",
			"end_session_endpoint":                  issuer + "/logout",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	defer server.Close()
	issuer = server.URL

	client := NewOIDCClient(OIDCConfig{
		Issuer:                issuer,
		ClientID:              "lice-web",
		ClientSecret:          "secret",
		RedirectURL:           "http://lice.localhost/api/v1/auth/callback",
		PostLogoutRedirectURL: "http://lice.localhost/acesso/sessao-encerrada",
		HTTPClient:            server.Client(),
		AllowHTTP:             true,
	})
	authorizationURL, err := client.AuthorizationURL(context.Background(), "state", "nonce", "challenge")
	if err != nil {
		t.Fatalf("AuthorizationURL() error = %v", err)
	}
	authorization, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	query := authorization.Query()
	if query.Get("scope") != "openid" {
		t.Fatalf("scope = %q, want only openid", query.Get("scope"))
	}
	if query.Get("prompt") != "login" || query.Get("nonce") != "nonce" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization security parameters are incomplete: %v", query)
	}
	if query.Has("access_type") || query.Has("offline_access") {
		t.Fatalf("authorization requested an offline token: %v", query)
	}

	logoutURL, err := client.LogoutURL(context.Background(), "")
	if err != nil {
		t.Fatalf("LogoutURL() error = %v", err)
	}
	logout, err := url.Parse(logoutURL)
	if err != nil {
		t.Fatal(err)
	}
	logoutQuery := logout.Query()
	if logoutQuery.Get("client_id") != "lice-web" || logoutQuery.Get("post_logout_redirect_uri") != "http://lice.localhost/acesso/sessao-encerrada" {
		t.Fatalf("unexpected logout parameters: %v", logoutQuery)
	}
	if logoutQuery.Has("id_token_hint") || logoutQuery.Has("access_token") || logoutQuery.Has("refresh_token") || len(logoutQuery) != 2 {
		t.Fatalf("logout URL contains token-bearing or unexpected parameters: %v", logoutQuery)
	}
}

func TestOIDCTokenEndpointErrorsDistinguishInvalidFlowFromUnavailableProvider(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		errorCode string
		want      error
	}{
		{name: "expired authorization code", status: http.StatusBadRequest, errorCode: "invalid_grant", want: ErrInvalidLogin},
		{name: "provider failure", status: http.StatusServiceUnavailable, errorCode: "temporarily_unavailable", want: ErrProviderUnavailable},
		{name: "invalid client configuration", status: http.StatusUnauthorized, errorCode: "invalid_client", want: ErrProviderUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var issuer string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/.well-known/openid-configuration":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
						"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
						"id_token_signing_alg_values_supported": []string{"RS256"},
					})
				case "/token":
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(test.status)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": test.errorCode})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			issuer = server.URL
			client := NewOIDCClient(OIDCConfig{
				Issuer: issuer, ClientID: "lice-web", ClientSecret: "secret",
				RedirectURL: "http://lice.localhost/api/v1/auth/callback",
				HTTPClient:  server.Client(), AllowHTTP: true,
			})

			_, err := client.ExchangeAndVerify(context.Background(), "code", "verifier")
			if !errors.Is(err, test.want) {
				t.Fatalf("ExchangeAndVerify() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestValidAuthorizedParty(t *testing.T) {
	tests := []struct {
		name            string
		audience        []string
		authorizedParty string
		want            bool
	}{
		{name: "single audience without azp", audience: []string{"lice-web"}, want: true},
		{name: "single audience with matching azp", audience: []string{"lice-web"}, authorizedParty: "lice-web", want: true},
		{name: "multiple audiences with matching azp", audience: []string{"lice-web", "account"}, authorizedParty: "lice-web", want: true},
		{name: "multiple audiences without azp", audience: []string{"lice-web", "account"}, want: false},
		{name: "different azp", audience: []string{"lice-web", "other-client"}, authorizedParty: "other-client", want: false},
		{name: "client missing from audience", audience: []string{"other-client"}, authorizedParty: "lice-web", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validAuthorizedParty(test.audience, test.authorizedParty, "lice-web"); got != test.want {
				t.Fatalf("validAuthorizedParty() = %v, want %v", got, test.want)
			}
		})
	}
}
