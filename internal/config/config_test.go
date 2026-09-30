package config

import "testing"

func TestLoad_DisableOIDC_OnlyRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "/tmp/glidepath.db")

	cfg, err := Load(":8080", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "/tmp/glidepath.db" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(":8080", true); err == nil {
		t.Fatal("expected error when DATABASE_URL is unset")
	}
}

func TestLoad_OIDCEnabled_RequiresOIDCVars(t *testing.T) {
	t.Setenv("DATABASE_URL", "/tmp/glidepath.db")
	t.Setenv("OIDC_ISSUER", "")
	t.Setenv("OIDC_AUDIENCE", "")
	t.Setenv("OIDC_CLIENT_ID", "")
	t.Setenv("OIDC_ALLOWED_EMAILS", "")

	if _, err := Load(":8080", false); err == nil {
		t.Fatal("expected error when OIDC is enabled but OIDC_* vars are unset")
	}

	t.Setenv("OIDC_ISSUER", "https://issuer.example")
	t.Setenv("OIDC_AUDIENCE", "aud")
	t.Setenv("OIDC_CLIENT_ID", "client")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("OIDC_REDIRECT_URL", "https://app.example/callback")
	t.Setenv("OIDC_ALLOWED_EMAILS", "me@example.com")

	cfg, err := Load(":8080", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCIssuer != "https://issuer.example" {
		t.Errorf("OIDCIssuer = %q", cfg.OIDCIssuer)
	}
	if cfg.OIDCClientSecret != "secret" {
		t.Errorf("OIDCClientSecret = %q", cfg.OIDCClientSecret)
	}
	if cfg.OIDCRedirectURL != "https://app.example/callback" {
		t.Errorf("OIDCRedirectURL = %q", cfg.OIDCRedirectURL)
	}
}
