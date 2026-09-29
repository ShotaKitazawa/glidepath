package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
)

// fakeVerifier maps a raw token string directly to a canned *oidc.IDToken
// (or an error), avoiding any network call to a real IdP.
type fakeVerifier struct {
	tokens map[string]*gooidc.IDToken
}

func (f *fakeVerifier) Verify(_ context.Context, rawIDToken string) (*gooidc.IDToken, error) {
	tok, ok := f.tokens[rawIDToken]
	if !ok {
		return nil, errInvalidToken
	}
	return tok, nil
}

var errInvalidToken = &testError{"invalid token"}

type testError struct{ s string }

func (e *testError) Error() string { return e.s }

func newTestMiddleware() *Middleware {
	future := time.Now().Add(time.Hour)
	return &Middleware{
		verifier: &fakeVerifier{tokens: map[string]*gooidc.IDToken{
			"valid-allowed":     {Subject: "me@example.com", Expiry: future},
			"valid-not-allowed": {Subject: "stranger@example.com", Expiry: future},
		}},
		allowedSubs: map[string]struct{}{"me@example.com": {}},
	}
}

func TestWrap_UnauthenticatedPathsAlwaysPassThrough(t *testing.T) {
	m := newTestMiddleware()
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for path := range unauthenticatedPaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("path %s: status = %d, want %d (no session cookie set)", path, rec.Code, http.StatusOK)
		}
	}
}

func TestWrap_ProtectedPath(t *testing.T) {
	cases := []struct {
		name       string
		cookie     *http.Cookie
		wantCalled bool
	}{
		{"no cookie", nil, false},
		{"unknown token", &http.Cookie{Name: sessionCookieName, Value: "garbage"}, false},
		{"valid but not allowlisted", &http.Cookie{Name: sessionCookieName, Value: "valid-not-allowed"}, false},
		{"valid and allowlisted", &http.Cookie{Name: sessionCookieName, Value: "valid-allowed"}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestMiddleware()
			called := false
			handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.cookie != nil {
				req.AddCookie(c.cookie)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if called != c.wantCalled {
				t.Errorf("next called = %v, want %v (status = %d)", called, c.wantCalled, rec.Code)
			}
			if !c.wantCalled && rec.Code != http.StatusFound {
				t.Errorf("status = %d, want %d (redirect to /login)", rec.Code, http.StatusFound)
			}
		})
	}
}

func TestParseAllowedEmails(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"single", "a@example.com", []string{"a@example.com"}},
		{"multiple with whitespace", " a@example.com , b@example.com", []string{"a@example.com", "b@example.com"}},
		{"blank entries dropped", "a@example.com,,", []string{"a@example.com"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseAllowedEmails(c.raw)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for _, email := range c.want {
				if _, ok := got[email]; !ok {
					t.Errorf("missing %q in %v", email, got)
				}
			}
		})
	}
}
