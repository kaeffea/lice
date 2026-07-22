package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kaeffea/lice/apps/api/internal/config"
	"github.com/kaeffea/lice/apps/api/internal/postgres"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 || arguments[0] != "bootstrap-demo" {
		return errors.New("usage: lice-admin bootstrap-demo --operator-subject SUBJECT --viewer-subject SUBJECT")
	}
	flags := flag.NewFlagSet("bootstrap-demo", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	operatorSubject := flags.String("operator-subject", "", "OIDC subject for the demo platform operator")
	viewerSubject := flags.String("viewer-subject", "", "OIDC subject for the demo viewer")
	if err := flags.Parse(arguments[1:]); err != nil {
		return errors.New("bootstrap arguments are invalid")
	}
	operator := strings.TrimSpace(*operatorSubject)
	viewer := strings.TrimSpace(*viewerSubject)
	if operator == "" || viewer == "" || len(operator) > 255 || len(viewer) > 255 {
		return errors.New("operator-subject and viewer-subject are required and must contain at most 255 bytes")
	}
	if operator == viewer {
		return errors.New("operator-subject and viewer-subject must be distinct")
	}
	cfg, err := config.LoadBootstrap()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	now := time.Now().UTC()
	correlationID := uuid.Must(uuid.NewV7())
	operatorResult, err := store.BootstrapIdentity(ctx, cfg.OIDCIssuer, operator, "Operador de demonstração", true, now, correlationID)
	if err != nil {
		return errors.New("could not bootstrap the demo operator")
	}
	viewerResult, err := store.BootstrapIdentity(ctx, cfg.OIDCIssuer, viewer, "Observador de demonstração", false, now, correlationID)
	if err != nil {
		return errors.New("could not bootstrap the demo viewer")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"operator": map[string]any{
			"principal_id":  operatorResult.PrincipalID,
			"identity_id":   operatorResult.IdentityID,
			"grant_created": operatorResult.GrantMade,
		},
		"viewer": map[string]any{
			"principal_id": viewerResult.PrincipalID,
			"identity_id":  viewerResult.IdentityID,
		},
	})
}
