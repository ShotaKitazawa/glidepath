package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func newTestMiddlewareWithFlow() *Middleware {
	m := newTestMiddleware()
	m.oauth2Cfg = &oauth2.Config{
		ClientID:    "test-client",
		RedirectURL: "https://glidepath.example/callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://idp.example/oauth2/auth",
			TokenURL: "https://idp.example/oauth2/token",
		},
		Scopes: []string{"openid"},
	}
	return m
}

func TestLogin_SetsFlowCookieAndRedirectsWithPKCE(t *testing.T) {
	m := newTestMiddlewareWithFlow()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	m.login(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("invalid Location header: %v", err)
	}
	q := loc.Query()
	for _, want := range []struct{ key, value string }{
		{"response_type", "code"},
		{"client_id", "test-client"},
		{"redirect_uri", "https://glidepath.example/callback"},
		{"code_challenge_method", "S256"},
	} {
		if got := q.Get(want.key); got != want.value {
			t.Errorf("query %s = %q, want %q (full URL: %s)", want.key, got, want.value, loc)
		}
	}
	if q.Get("state") == "" {
		t.Error("expected a non-empty state param")
	}
	if q.Get("code_challenge") == "" {
		t.Error("expected a non-empty code_challenge param")
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != flowCookieName {
		t.Fatalf("expected exactly one %s cookie, got %v", flowCookieName, cookies)
	}
	state, verifier, ok := strings.Cut(cookies[0].Value, "|")
	if !ok || state == "" || verifier == "" {
		t.Fatalf("flow cookie value %q did not round-trip to non-empty state|verifier", cookies[0].Value)
	}
	if state != q.Get("state") {
		t.Errorf("cookie state %q does not match AuthCodeURL state %q", state, q.Get("state"))
	}
}

func TestCallback_MissingFlowCookieRejected(t *testing.T) {
	m := newTestMiddlewareWithFlow()
	req := httptest.NewRequest(http.MethodGet, "/callback?state=x&code=y", nil)
	rec := httptest.NewRecorder()

	m.callback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCallback_StateMismatchRejected(t *testing.T) {
	m := newTestMiddlewareWithFlow()
	req := httptest.NewRequest(http.MethodGet, "/callback?state=wrong&code=y", nil)
	req.AddCookie(&http.Cookie{Name: flowCookieName, Value: "expected-state|some-verifier"})
	rec := httptest.NewRecorder()

	m.callback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLogout_ClearsSessionCookie(t *testing.T) {
	m := newTestMiddlewareWithFlow()
	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	rec := httptest.NewRecorder()

	m.logout(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("expected a cleared %s cookie (MaxAge < 0), got %v", sessionCookieName, cookies)
	}
}
