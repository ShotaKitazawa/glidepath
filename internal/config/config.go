// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
)

// Config holds all runtime configuration. OIDC_* fields are only populated
// (and required) when DisableOIDC is false.
type Config struct {
	Addr            string
	DatabaseURL     string
	AnthropicAPIKey string
	DisableOIDC     bool
	OIDCIssuer      string
	// OIDCAudience is not consulted by the current cookie-session login flow
	// (an ID token's aud is always the client_id, not a separate resource
	// audience) — reserved for a future /mcp endpoint doing RFC 9728
	// resource-audience checks on access tokens, mirroring korpus.
	OIDCAudience      string
	OIDCClientID      string
	OIDCClientSecret  string
	OIDCRedirectURL   string
	OIDCAllowedEmails string
}

// Load reads Config from the environment. DATABASE_URL is always required;
// the OIDC_* variables are required unless disableOIDC is set (local dev).
func Load(addr string, disableOIDC bool) (Config, error) {
	cfg := Config{Addr: addr, DisableOIDC: disableOIDC}

	dbURL, err := mustGetenv("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}
	cfg.DatabaseURL = dbURL

	cfg.AnthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")

	if disableOIDC {
		return cfg, nil
	}

	issuer, err := mustGetenv("OIDC_ISSUER")
	if err != nil {
		return Config{}, err
	}
	audience, err := mustGetenv("OIDC_AUDIENCE")
	if err != nil {
		return Config{}, err
	}
	clientID, err := mustGetenv("OIDC_CLIENT_ID")
	if err != nil {
		return Config{}, err
	}
	clientSecret, err := mustGetenv("OIDC_CLIENT_SECRET")
	if err != nil {
		return Config{}, err
	}
	redirectURL, err := mustGetenv("OIDC_REDIRECT_URL")
	if err != nil {
		return Config{}, err
	}
	allowedEmails, err := mustGetenv("OIDC_ALLOWED_EMAILS")
	if err != nil {
		return Config{}, err
	}
	cfg.OIDCIssuer = issuer
	cfg.OIDCAudience = audience
	cfg.OIDCClientID = clientID
	cfg.OIDCClientSecret = clientSecret
	cfg.OIDCRedirectURL = redirectURL
	cfg.OIDCAllowedEmails = allowedEmails

	return cfg, nil
}

func mustGetenv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return v, nil
}
