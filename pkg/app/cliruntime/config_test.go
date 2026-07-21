package cliruntime

import (
	"testing"

	"github.com/spf13/viper"
)

func TestDatabaseSettingsFromViperReadsKeys(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("database.host", "db.example")
	viper.Set("database.port", 5432)
	viper.Set("database.name", "pletka_weave")
	viper.Set("database.user", "pletka")

	s := DatabaseSettingsFromViper()
	if s.Host != "db.example" || s.Port != 5432 || s.Name != "pletka_weave" || s.User != "pletka" {
		t.Fatalf("unexpected settings: %+v", s)
	}
}

func TestDatabaseSettingsFromViperInstanceDerivesName(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	// Full connection (host/port/credentials) supplied by config, but a
	// stale database.name that --instance must override.
	viper.Set("database.host", "db.example")
	viper.Set("database.port", 5432)
	viper.Set("database.name", "pletka_weave")
	viper.Set("database.user", "pletka")
	viper.Set("instance", "prod")

	s := DatabaseSettingsFromViper()
	if s.Name != "pletka_prod" {
		t.Fatalf("instance=prod should derive name pletka_prod, got %q", s.Name)
	}
	// Everything else still comes from config.
	if s.Host != "db.example" || s.Port != 5432 || s.User != "pletka" {
		t.Fatalf("instance override must not disturb the rest: %+v", s)
	}
}

func TestDatabaseSettingsFromViperNoInstanceKeepsName(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("database.name", "pletka_weave")

	if got := DatabaseSettingsFromViper().Name; got != "pletka_weave" {
		t.Fatalf("without --instance the configured name must stand, got %q", got)
	}
}
