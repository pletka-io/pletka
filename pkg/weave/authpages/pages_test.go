package authpages

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// newTestRouter mounts the authpages routes behind a real renderer + i18n
// manager (in-memory backend) so LoginPage/LocalLoginPage can exercise their
// full render path, not just the pre-render redirects.
func newTestRouter(t *testing.T, ssoLoginURL string) *chi.Mux {
	t.Helper()

	i18nMgr, err := i18n.New(i18n.Config{
		DefaultLanguage:  "en",
		FallbackLanguage: "en",
		Storage:          backend.NewMemoryBackend(),
		Languages: []i18n.Language{
			{Code: "en", Name: "English", EnglishName: "English", Direction: "ltr", Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	renderer, err := weavetemplates.NewRenderer(func(names ...string) template.HTML {
		return ""
	}, i18nMgr, weavetemplates.AnalyticsConfig{})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	r := chi.NewRouter()
	Mount(r, Host{
		Templates:   renderer,
		I18n:        i18nMgr,
		SSOLoginURL: ssoLoginURL,
	})
	return r
}

func TestLoginPage_SSOButton(t *testing.T) {
	// safeRedirect resolves a bare "/login" (no next/redirect, no referer) to
	// the "/profile" fallback, not "". "next=/" is the genuine no-destination
	// case here (isSafeReturnPath allows "/", but ssoLoginHTML deliberately
	// does not forward it — same "skip the landing page" idea safeRedirect
	// already applies to the Referer fallback), so it's the plain-href case.
	router := newTestRouter(t, "/auth/oidc/login")

	req := httptest.NewRequest(http.MethodGet, "/login?next=/", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `href="/auth/oidc/login"`) {
		t.Fatalf("body missing plain SSO login href: %s", body)
	}
	if strings.Contains(body, `id="login-form"`) {
		t.Fatalf("body should not contain password login form when SSO is enabled: %s", body)
	}
}

// TestLoginPage_SSOButton_ForwardsNext covers the cross-task integration gap:
// the OIDC login route reads "next" and carries it through the auth
// round-trip (state cookie), so the SSO button must forward the caller's
// post-login destination instead of always linking to the bare SSO URL.
func TestLoginPage_SSOButton_ForwardsNext(t *testing.T) {
	router := newTestRouter(t, "/auth/oidc/login")

	req := httptest.NewRequest(http.MethodGet, "/login?next=/models/x", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `href="/auth/oidc/login?next=%2Fmodels%2Fx"`) {
		t.Fatalf("body missing next-forwarding SSO login href: %s", body)
	}
}

func TestLoginPage_StandaloneUnchanged(t *testing.T) {
	router := newTestRouter(t, "")

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if body := rr.Body.String(); !strings.Contains(body, `id="login-form"`) {
		t.Fatalf("body missing password login form: %s", body)
	}
}

func TestLoginLocalPage_AlwaysPasswordForm(t *testing.T) {
	router := newTestRouter(t, "/auth/oidc/login")

	req := httptest.NewRequest(http.MethodGet, "/login/local", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if body := rr.Body.String(); !strings.Contains(body, `id="login-form"`) {
		t.Fatalf("body missing password login form: %s", body)
	}
}

// TestIsSafeReturnPath covers the backslash/control-character rejections
// added alongside oidcauth's safeRelative (pkg/weave/oidcauth/state.go in
// pletka-platform) — browsers normalize "\" to "/" during URL parsing, so
// "/\evil.com" can resolve as a protocol-relative escape despite starting
// with a single "/", and raw control characters enable similar
// parser-mismatch bypasses.
func TestIsSafeReturnPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"plain relative path", "/models/x", true},
		{"root", "/", true},
		{"empty", "", false},
		{"not rooted", "models/x", false},
		{"protocol-relative escape", "//evil.com", false},
		{"backslash escape", "/\\evil.com", false},
		{"tab control character", "/a\tb", false},
		{"login path excluded", "/login", false},
		{"logout path excluded", "/logout", false},
		{"register path excluded", "/register", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafeReturnPath(tt.path); got != tt.want {
				t.Errorf("isSafeReturnPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestRegisterPageRedirectsWhenRegistrationDisabled(t *testing.T) {
	handler := &Handler{registrationEnabled: false}

	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	rr := httptest.NewRecorder()

	handler.RegisterPage(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusFound)
	}
	if got, want := rr.Header().Get("Location"), "/login"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestAuthHTMLUsesCurrentBrandClasses(t *testing.T) {
	retiredPrimary := "zellij-" + "primary"
	retiredSecondary := "zellij-" + "secondary"
	for name, body := range map[string]string{
		"login":    string(loginHTML("/profile", true)),
		"register": string(registerHTML()),
	} {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(body, retiredPrimary) || strings.Contains(body, retiredSecondary) {
				t.Fatalf("%s auth HTML uses retired zellij brand classes", name)
			}
			if !strings.Contains(body, "pletka-primary") {
				t.Fatalf("%s auth HTML does not include current pletka brand classes", name)
			}
		})
	}
}
