package serverruntime

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/database"
)

// TestProductionRejectsDefaultDBPassword verifies that a production config
// carrying the dev-default database password fails boot instead of silently
// connecting with a known credential.
func TestProductionRejectsDefaultDBPassword(t *testing.T) {
	err := validateProductionConfig(Config{
		Env:      "production",
		Database: database.Settings{Password: "pw123"},
	})
	if err == nil {
		t.Fatal("production with pw123 must be rejected")
	}
}

// TestProductionRejectsEmptyDBPassword verifies that a production config
// with no password set fails boot.
func TestProductionRejectsEmptyDBPassword(t *testing.T) {
	err := validateProductionConfig(Config{
		Env:      "production",
		Database: database.Settings{Password: ""},
	})
	if err == nil {
		t.Fatal("production with empty password must be rejected")
	}
}

// TestProductionAllowsRealDBPassword verifies that a production config with
// a non-default password boots normally.
func TestProductionAllowsRealDBPassword(t *testing.T) {
	if err := validateProductionConfig(Config{
		Env:      "production",
		Database: database.Settings{Password: "a-real-secret"},
	}); err != nil {
		t.Fatalf("production with a real secret must be allowed: %v", err)
	}
}

// TestDevelopmentAllowsDefaultDBPassword verifies that non-production
// environments may keep using the dev-default database password.
func TestDevelopmentAllowsDefaultDBPassword(t *testing.T) {
	if err := validateProductionConfig(Config{
		Env:      "development",
		Database: database.Settings{Password: "pw123"},
	}); err != nil {
		t.Fatalf("dev must allow the default: %v", err)
	}
}
