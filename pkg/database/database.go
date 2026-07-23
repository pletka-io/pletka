package database

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Settings contains the connection parameters needed to open the app database.
type Settings struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Name     string `toml:"name"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	SSLMode  string `toml:"ssl_mode"`
}

// WithEnvOverrides returns a copy with PLETKA_DB_* environment variables applied.
// This allows per-worktree DB targets without modifying the config file.
func (s Settings) WithEnvOverrides() Settings {
	if v := os.Getenv("PLETKA_DB_HOST"); v != "" {
		s.Host = v
	}
	if v := os.Getenv("PLETKA_DB_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			s.Port = p
		}
	}
	if v := os.Getenv("PLETKA_DB_NAME"); v != "" {
		s.Name = v
	}
	if v := os.Getenv("PLETKA_DB_USER"); v != "" {
		s.User = v
	}
	if v := os.Getenv("PLETKA_DB_PASSWORD"); v != "" {
		s.Password = v
	}
	if v := os.Getenv("PLETKA_DB_SSLMODE"); v != "" {
		s.SSLMode = v
	}
	return s
}

// PgxConnString returns a pgx-compatible keyword/value connection string.
// The keyword form (not a postgres:// URL) is used deliberately: a URL
// cannot express a unix-socket directory host (/var/run/postgresql —
// net/url rejects both the raw slashes and %2F escapes), while the
// keyword form handles TCP hosts and socket directories alike. Every
// consumer feeds pgx (sql.Open("pgx", …) / pgxpool.New), which accepts
// both forms.
func (s Settings) PgxConnString() string {
	quote := func(v string) string {
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		quote(s.Host), s.Port, quote(s.User), quote(s.Password), quote(s.Name), quote(s.SSLMode))
}

// DefaultSettings returns development defaults for the weave database.
func DefaultSettings() Settings {
	return Settings{
		Host:     "localhost",
		Port:     5433,
		Name:     "pletka_weave",
		User:     "postgres",
		Password: "pw123",
		SSLMode:  "disable",
	}
}
