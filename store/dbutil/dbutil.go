// Package dbutil owns small helpers that bridge sqlc-generated nullable types
// and the domain layer.
package dbutil

import (
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Ptr returns a pointer to v.
func Ptr[T any](v T) *T {
	return &v
}

// Deref returns the pointed-to value, or the zero value of T when p is nil.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// DerefOr returns *p, or def when p is nil.
func DerefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// SliceOrEmpty returns *p, or nil when p is nil.
func SliceOrEmpty[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

// EmptyToNil maps "" to nil and any non-empty string to *string.
func EmptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// TrimEmptyToNil trims s and maps "" to nil.
func TrimEmptyToNil(s string) *string {
	s = strings.TrimSpace(s)
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

// TrimNullable trims *p and maps nil or empty results to nil.
func TrimNullable(p *string) *string {
	if p == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*p)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// Int32Ptr returns a pointer to int32(v).
func Int32Ptr(v int) *int32 {
	i := int32(v)
	return &i
}

// DerefInt32 returns int(*p), or 0 when p is nil.
func DerefInt32(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

// DerefInt32ToIntPtr converts *int32 to *int while preserving nil.
func DerefInt32ToIntPtr(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
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
