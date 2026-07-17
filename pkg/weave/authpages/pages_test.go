package authpages

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
