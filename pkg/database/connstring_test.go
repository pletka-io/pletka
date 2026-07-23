package database

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestPgxConnStringParsesForTCPAndSocketHosts guards the keyword/value DSN
// form: a postgres:// URL cannot express a unix-socket directory host, which
// broke every socket connection (only at first query — pools connect lazily).
func TestPgxConnStringParsesForTCPAndSocketHosts(t *testing.T) {
	cases := []Settings{
		{Host: "localhost", Port: 5433, Name: "pletka_weave", User: "postgres", Password: "pw123", SSLMode: "disable"},
		{Host: "/var/run/postgresql", Port: 5432, Name: "pletka_alpha", User: "postgres", Password: "", SSLMode: "disable"},
		{Host: "db.example.com", Port: 5432, Name: "x", User: "u", Password: "p'w\\d", SSLMode: "require"},
	}
	for _, s := range cases {
		cfg, err := pgx.ParseConfig(s.PgxConnString())
		if err != nil {
			t.Errorf("ParseConfig(%q): %v", s.PgxConnString(), err)
			continue
		}
		if cfg.Host != s.Host {
			t.Errorf("host: want %q, got %q", s.Host, cfg.Host)
		}
		if cfg.Database != s.Name {
			t.Errorf("dbname: want %q, got %q", s.Name, cfg.Database)
		}
		if cfg.Password != s.Password {
			t.Errorf("password: want %q, got %q", s.Password, cfg.Password)
		}
	}
}
