package auth_test

import (
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
)

func TestValidateSlug_Reserved(t *testing.T) {
	for _, bad := range []string{"admin", "api", "login", "users", "orgs"} {
		err := auth.ValidateSlug(bad)
		if !errors.Is(err, auth.ErrSlugReserved) {
			t.Errorf("ValidateSlug(%q): want ErrSlugReserved, got %v", bad, err)
		}
	}
}

func TestValidateSlug_Format(t *testing.T) {
	bad := []string{"", "A", "has spaces", "bad!chars", "-leading", "trailing-",
		"UPPER", "dou--ble", "x"} // "x" too short
	for _, s := range bad {
		if err := auth.ValidateSlug(s); err == nil {
			t.Errorf("ValidateSlug(%q): expected error, got nil", s)
		}
	}
}

func TestValidateSlug_Valid(t *testing.T) {
	good := []string{"ab", "delving", "my-org", "some_user", "actor123", "a1b2c3"}
	for _, s := range good {
		if err := auth.ValidateSlug(s); err != nil {
			t.Errorf("ValidateSlug(%q): unexpected error %v", s, err)
		}
	}
}
