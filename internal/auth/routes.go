package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

// flowCookieName holds the short-lived state+PKCE-verifier pair between
// /login and /callback. Its value is opaque and single-use, so it doesn't
// need signing — only state's exact round-trip match matters (CSRF check).
const flowCookieName = "glidepath_oauth_flow"

// RegisterRoutes wires the login flow's own endpoints onto mux. Must be
// called alongside handler.Register (both share the same mux, wrapped once
// by Wrap — see main.go) since these paths are exactly the ones Wrap lets
// through unauthenticated.
func (m *Middleware) RegisterRoutes(mux *http.ServeMux) {
	if m.disabled {
		return
	}
	mux.HandleFunc("GET /login", m.login)
	mux.HandleFunc("GET /callback", m.callback)
	mux.HandleFunc("GET /logout", m.logout)
}

// flowCookieMaxAge is generous (not the usual 5-10 minutes) because the
// full chain — glidepath -> Hydra -> Auth0 -> Google account picker ->
// possible 2FA -> Hydra consent -> back to glidepath — can take a while on
// an unfamiliar first login; a short window turned a slow-but-legitimate
// login into a confusing dead-end error in practice (found 2026-09).
const flowCookieMaxAge = 30 * 60

func (m *Middleware) login(w http.ResponseWriter, r *http.Request) {
	state := randomToken()
	verifier := oauth2.GenerateVerifier()

	http.SetCookie(w, &http.Cookie{
		Name:  flowCookieName,
		Value: state + "|" + verifier,
		Path:  "/",
		// Lax, not Strict: this cookie must survive the top-level
		// cross-site redirect back from the IdP's origin into /callback.
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Secure:   true,
		MaxAge:   flowCookieMaxAge,
	})

	http.Redirect(w, r, m.oauth2Cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (m *Middleware) callback(w http.ResponseWriter, r *http.Request) {
	// A missing/mismatched flow cookie only ever means "this login attempt
	// can't be trusted as-is" (expired, browser lost its cookie jar
	// mid-flow, or a stale/replayed callback link) — never a signal about
	// the user's identity. Restarting the login is always safe and always
	// the right next step, so redirect straight back into it instead of
	// dead-ending on a raw protocol error the user can't act on.
	flowCookie, err := r.Cookie(flowCookieName)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: flowCookieName, Path: "/", MaxAge: -1})

	state, verifier, ok := strings.Cut(flowCookie.Value, "|")
	if !ok || state != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	token, err := m.oauth2Cfg.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		http.Error(w, "code exchange failed", http.StatusBadGateway)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in token response", http.StatusBadGateway)
		return
	}

	_, expiry, ok := m.verifyAllowedSession(r.Context(), rawIDToken)
	if !ok {
		http.Error(w, "not authorized", http.StatusForbidden)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    rawIDToken,
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		HttpOnly: true,
		Secure:   true,
		Expires:  expiry,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (m *Middleware) logout(w http.ResponseWriter, r *http.Request) {
	// No RP-initiated logout call to the IdP: Hydra never remembers a
	// login/consent session for this deployment (remember_for: 0 in the
	// shared login-consent app), so there is nothing server-side to
	// invalidate beyond this cookie.
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand.Read failing means the system RNG is broken
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
