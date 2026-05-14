package postgres

import (
	"strings"
	"testing"
	"time"
)

func TestConfigConnStringUsesURL(t *testing.T) {
	cfg := Config{
		URL:      "postgres://example/db",
		Host:     "ignored",
		Database: "ignored",
	}

	if got := cfg.ConnString(); got != "postgres://example/db" {
		t.Fatalf("ConnString() = %q; want URL passthrough", got)
	}
}

func TestConfigConnStringFromFields(t *testing.T) {
	cfg := Config{
		Host:           "db",
		Port:           "6543",
		Database:       "pletka_test",
		User:           "pletka",
		Password:       "secret",
		SSLMode:        "require",
		ConnectTimeout: 15 * time.Second,
	}

	got := cfg.ConnString()
	for _, want := range []string{
		"host=db",
		"port=6543",
		"dbname=pletka_test",
		"user=pletka",
		"password=secret",
		"sslmode=require",
		"connect_timeout=15",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ConnString() = %q; want substring %q", got, want)
		}
	}
}
