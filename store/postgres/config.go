package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Config controls PostgreSQL connectivity for Pletka stores and migrations.
type Config struct {
	URL            string
	Host           string
	Port           string
	Database       string
	User           string
	Password       string
	SSLMode        string
	MaxConnections int32
	MinConnections int32
	ConnectTimeout time.Duration
}

// ConnString returns a pgx-compatible PostgreSQL connection string.
func (c Config) ConnString() string {
	if c.URL != "" {
		return c.URL
	}

	parts := []string{
		"host=" + quoteConnValue(defaultString(c.Host, "localhost")),
		"port=" + quoteConnValue(defaultString(c.Port, "5432")),
		"dbname=" + quoteConnValue(defaultString(c.Database, "pletka")),
		"user=" + quoteConnValue(defaultString(c.User, "postgres")),
		"sslmode=" + quoteConnValue(defaultString(c.SSLMode, "disable")),
	}
	if c.Password != "" {
		parts = append(parts, "password="+quoteConnValue(c.Password))
	}
	if c.ConnectTimeout > 0 {
		parts = append(parts, fmt.Sprintf("connect_timeout=%d", int(c.ConnectTimeout.Seconds())))
	}
	return strings.Join(parts, " ")
}

// OpenPool creates a pgx pool and verifies the connection.
func OpenPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.ConnString())
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	if cfg.MaxConnections > 0 {
		poolConfig.MaxConns = cfg.MaxConnections
	}
	if cfg.MinConnections > 0 {
		poolConfig.MinConns = cfg.MinConnections
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

// OpenSQL creates a database/sql connection for goose migrations.
func OpenSQL(ctx context.Context, cfg Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.ConnString())
	if err != nil {
		return nil, fmt.Errorf("open postgres sql db: %w", err)
	}
	if cfg.MaxConnections > 0 {
		db.SetMaxOpenConns(int(cfg.MaxConnections))
		db.SetMaxIdleConns(int(cfg.MaxConnections))
	}
	if cfg.MinConnections > 0 {
		db.SetMaxIdleConns(int(cfg.MinConnections))
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func quoteConnValue(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n\r'\\") {
		return value
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}
