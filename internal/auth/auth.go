// Package auth authenticates requests via OIDC and restricts access to a
// fixed set of allowed email addresses (SPEC.md section 2: personal-use app,
// single owner or a small fixed set of users).
//
// glidepath is server-rendered (no SPA/fetch layer to attach a bearer token
// to), so login is a browser-redirect Authorization Code + PKCE flow ending
// in an HttpOnly session cookie — not the bearer-token-per-request pattern
// this author's other OIDC-protected apps (kondate, korpus) use for their
// SPA+API frontends. The session cookie holds the raw ID token JWT, verified
// against the issuer's JWKS on every request; since the token is already a
// signed, self-verifying credential, this needs no session store or extra
// signing/encryption dependency (SPEC.md 7: minimal dependencies).
//
// The shared homelab IdP (Ory Hydra) does not include an email claim on
// issued ID tokens — its login-consent app sets the Hydra subject to the
// user's email, so the allowlist check below compares against the "sub"
// claim, not "email".
package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/ShotaKitazawa/glidepath/internal/config"
)

// idTokenVerifier is the subset of *oidc.IDTokenVerifier this package
// depends on, so tests can inject a fake without a network call.
type idTokenVerifier interface {
	Verify(ctx context.Context, rawIDToken string) (*gooidc.IDToken, error)
}

// sessionCookieName holds the raw ID token JWT once login succeeds.
const sessionCookieName = "glidepath_session"

// Middleware authenticates incoming requests and rejects any user whose
// verified "sub" claim (the allowlisted IdP subject — see package doc) is
// not in the OIDC_ALLOWED_EMAILS allowlist.
type Middleware struct {
	disabled bool

	verifier    idTokenVerifier
	oauth2Cfg   *oauth2.Config
	allowedSubs map[string]struct{}
}

// New builds the auth middleware from cfg. When cfg.DisableOIDC is true,
// all requests are let through unauthenticated; this must only be used for
// local development (wired via the --disable-oidc flag / mise run dev).
func New(ctx context.Context, cfg config.Config) (*Middleware, error) {
	if cfg.DisableOIDC {
		slog.Warn("OIDC authentication is DISABLED; do not use this mode outside local development")
		return &Middleware{disabled: true}, nil
	}

	provider, err := gooidc.NewProvider(ctx, cfg.OIDCIssuer)
	if err != nil {
		return nil, err
	}
	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.OIDCClientID})

	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		RedirectURL:  cfg.OIDCRedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID},
	}

	return &Middleware{
		verifier:    verifier,
		oauth2Cfg:   oauth2Cfg,
		allowedSubs: parseAllowedEmails(cfg.OIDCAllowedEmails),
	}, nil
}

func parseAllowedEmails(raw string) map[string]struct{} {
	allowed := map[string]struct{}{}
	for _, email := range strings.Split(raw, ",") {
		email = strings.TrimSpace(email)
		if email != "" {
			allowed[email] = struct{}{}
		}
	}
	return allowed
}

// unauthenticatedPaths must stay reachable without a session: /healthz for
// Kubernetes probes, and the login flow's own endpoints (wrapping those
// would make login itself unreachable).
var unauthenticatedPaths = map[string]struct{}{
	"/healthz":  {},
	"/login":    {},
	"/callback": {},
	"/logout":   {},
}

// Wrap enforces authentication on next: unauthenticated for
// unauthenticatedPaths, otherwise requires a valid, allowlisted session
// cookie (see verifySession), redirecting to /login when absent or invalid.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	if m.disabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := unauthenticatedPaths[r.URL.Path]; ok {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if _, _, ok := m.verifyAllowedSession(r.Context(), cookie.Value); !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// verifyAllowedSession verifies rawIDToken and checks its Subject (the "sub"
// claim — see package doc for why "sub" rather than "email") against the
// allowlist, returning that subject and the token's expiry on success.
// Shared by Wrap and the callback handler (routes.go), which needs the same
// check right after exchanging the authorization code (and needs expiry to
// set the session cookie's lifetime).
func (m *Middleware) verifyAllowedSession(ctx context.Context, rawIDToken string) (sub string, expiry time.Time, ok bool) {
	idToken, err := m.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", time.Time{}, false
	}
	if _, allowed := m.allowedSubs[idToken.Subject]; !allowed {
		return "", time.Time{}, false
	}
	return idToken.Subject, idToken.Expiry, true
}
