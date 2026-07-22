package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kaeffea/lice/apps/api/internal/auth"
	"github.com/kaeffea/lice/apps/api/internal/clock"
	"github.com/kaeffea/lice/apps/api/internal/config"
	"github.com/kaeffea/lice/apps/api/internal/cryptoutil"
	"github.com/kaeffea/lice/apps/api/internal/httpapi"
	"github.com/kaeffea/lice/apps/api/internal/postgres"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("API stopped", "error", safeError(err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := postgres.Open(startupContext, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	cipher, err := cryptoutil.NewCipher("login-v1", cfg.LoginEncryptionKey)
	if err != nil {
		return err
	}
	digester, err := cryptoutil.NewDigester(cfg.SessionHashKey)
	if err != nil {
		return err
	}
	csrf, err := cryptoutil.NewCSRF(cfg.CSRFKey)
	if err != nil {
		return err
	}
	provider := auth.NewOIDCClient(auth.OIDCConfig{
		Issuer:                cfg.OIDCIssuer,
		ClientID:              cfg.OIDCClientID,
		ClientSecret:          cfg.OIDCClientSecret,
		RedirectURL:           cfg.CallbackURL(),
		PostLogoutRedirectURL: cfg.PostLogoutURL(),
		HTTPClient:            &http.Client{Timeout: 10 * time.Second},
		AllowHTTP:             cfg.Environment == "local",
	})
	service, err := auth.NewService(store, provider, clock.System{}, cipher, digester, csrf, auth.ServiceConfig{
		LoginTTL:           cfg.LoginTTL,
		SessionIdleTTL:     cfg.SessionIdleTTL,
		SessionAbsoluteTTL: cfg.SessionAbsoluteTTL,
	})
	if err != nil {
		return err
	}
	api, err := httpapi.New(service, store, cfg, logger)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	shutdownContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownContext.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Warn("graceful shutdown did not finish", "error", safeError(err))
		}
	}()
	logger.Info("API listening", "address", cfg.HTTPAddr, "environment", cfg.Environment)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runHealthcheck() int {
	client := &http.Client{Timeout: 2 * time.Second}
	request, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8080/health/ready", nil)
	if err != nil {
		return 1
	}
	response, err := client.Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	// Application errors are intentionally terse and never include HTTP response
	// bodies, authorization codes, cookies, or provider tokens.
	return err.Error()
}
