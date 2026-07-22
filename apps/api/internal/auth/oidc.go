package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCConfig struct {
	Issuer                string
	ClientID              string
	ClientSecret          string
	RedirectURL           string
	PostLogoutRedirectURL string
	HTTPClient            *http.Client
	AllowHTTP             bool
}

type OIDCClient struct {
	config OIDCConfig
	mu     sync.Mutex
	ready  bool
	oauth  oauth2.Config
	verify *oidc.IDTokenVerifier
	logout string
}

func NewOIDCClient(config OIDCConfig) *OIDCClient {
	return &OIDCClient{config: config}
}

func (c *OIDCClient) AuthorizationURL(ctx context.Context, state, nonce, challenge string) (string, error) {
	if err := c.ensure(ctx); err != nil {
		return "", ErrProviderUnavailable
	}
	return c.oauth.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("prompt", "login"),
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	), nil
}

func (c *OIDCClient) ExchangeAndVerify(ctx context.Context, code, verifier string) (VerifiedIdentity, error) {
	if err := c.ensure(ctx); err != nil {
		return VerifiedIdentity{}, ErrProviderUnavailable
	}
	if c.config.HTTPClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, c.config.HTTPClient)
	}
	token, err := c.oauth.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return VerifiedIdentity{}, classifyTokenExchangeError(err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return VerifiedIdentity{}, ErrInvalidLogin
	}
	idToken, err := c.verify.Verify(ctx, rawIDToken)
	if err != nil {
		return VerifiedIdentity{}, ErrInvalidLogin
	}
	var claims struct {
		Nonce           string `json:"nonce"`
		AuthorizedParty string `json:"azp"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return VerifiedIdentity{}, ErrInvalidLogin
	}
	if !validAuthorizedParty(idToken.Audience, claims.AuthorizedParty, c.config.ClientID) {
		return VerifiedIdentity{}, ErrInvalidLogin
	}
	return VerifiedIdentity{
		Issuer:  idToken.Issuer,
		Subject: idToken.Subject,
		Nonce:   claims.Nonce,
	}, nil
}

func classifyTokenExchangeError(err error) error {
	var responseError *oauth2.RetrieveError
	if errors.As(err, &responseError) {
		switch responseError.ErrorCode {
		case "invalid_grant", "invalid_request", "access_denied":
			return ErrInvalidLogin
		}
	}
	return ErrProviderUnavailable
}

func validAuthorizedParty(audience []string, authorizedParty, clientID string) bool {
	audienceContainsClient := false
	for _, value := range audience {
		if value == clientID {
			audienceContainsClient = true
			break
		}
	}
	if !audienceContainsClient {
		return false
	}
	if authorizedParty != "" && authorizedParty != clientID {
		return false
	}
	return len(audience) == 1 || authorizedParty == clientID
}

func (c *OIDCClient) LogoutURL(ctx context.Context, postLogoutRedirect string) (string, error) {
	if err := c.ensure(ctx); err != nil {
		return "", ErrProviderUnavailable
	}
	if c.logout == "" {
		return "", nil
	}
	redirect := c.config.PostLogoutRedirectURL
	if postLogoutRedirect != "" {
		redirect = postLogoutRedirect
	}
	endpoint, err := url.Parse(c.logout)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && !(c.config.AllowHTTP && endpoint.Scheme == "http")) {
		return "", ErrProviderUnavailable
	}
	query := endpoint.Query()
	query.Set("client_id", c.config.ClientID)
	query.Set("post_logout_redirect_uri", redirect)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (c *OIDCClient) ensure(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ready {
		return nil
	}
	if c.config.HTTPClient != nil {
		ctx = oidc.ClientContext(ctx, c.config.HTTPClient)
	}
	provider, err := oidc.NewProvider(ctx, c.config.Issuer)
	if err != nil {
		return errors.New("OIDC discovery failed")
	}
	var metadata struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return errors.New("OIDC metadata is invalid")
	}
	providerEndpoint := provider.Endpoint()
	if !validProviderEndpoint(providerEndpoint.AuthURL, c.config.AllowHTTP) || !validProviderEndpoint(providerEndpoint.TokenURL, c.config.AllowHTTP) {
		return errors.New("OIDC endpoints are invalid")
	}
	c.oauth = oauth2.Config{
		ClientID:     c.config.ClientID,
		ClientSecret: c.config.ClientSecret,
		Endpoint:     providerEndpoint,
		RedirectURL:  c.config.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID},
	}
	c.verify = provider.Verifier(&oidc.Config{
		ClientID:             c.config.ClientID,
		SupportedSigningAlgs: []string{oidc.RS256},
	})
	c.logout = metadata.EndSessionEndpoint
	c.ready = true
	return nil
}

func validProviderEndpoint(raw string, allowHTTP bool) bool {
	endpoint, err := url.Parse(raw)
	return err == nil && endpoint.Host != "" && (endpoint.Scheme == "https" || (allowHTTP && endpoint.Scheme == "http"))
}
