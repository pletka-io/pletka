// Package dbutil owns small helpers that bridge sqlc-generated nullable types
// and the domain layer.
package dbutil

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// EmptyToNil maps "" to nil and any non-empty string to *string.
func EmptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// NilToEmpty returns *p, or "" when p is nil.
func NilToEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// TimestamptzToTimePtr returns &ts.Time when valid, nil otherwise.
func TimestamptzToTimePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	return &ts.Time
}

// TimePtrToTimestamptz returns a valid pgtype.Timestamptz when t is non-nil,
// and an invalid value that writes SQL NULL otherwise.
func TimePtrToTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
