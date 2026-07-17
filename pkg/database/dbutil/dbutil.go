// Package dbutil owns the small helpers that bridge sqlc-generated
// types and the domain layer: generic pointer round-trips, the
// string-empty/nil idiom, and pgtype <-> stdlib time conversions.
//
// Why one package: every weave slice was carrying its own copy of
// strPtr / boolPtr / derefStrPtr / timestamptzToTimePtr — 9 dupes of
// the same 4 lines, drift in subtle ways (some trimmed, some didn't).
// One source of truth keeps the contract honest.
//
// Why under pkg/database: most callers are sqlc store layers that
// already import pkg/database/sqlcgen; one more sibling package adds
// no friction. The pgtype converters specifically only make sense in
// a DB-adjacent package, and keeping the generic helpers next to them
// avoids splitting "the helpers I need when writing a store" across
// two packages.
package dbutil

import (
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Generic pointer helpers (Go 1.18+ generics)
// ---------------------------------------------------------------------------

// Ptr returns a pointer to v. The standard "I need *string but have
// string" one-liner that, before generics, every package reimplemented
// per type (strPtr / boolPtr / intPtr).
//
//	field.SetValue = dbutil.Ptr("default")
//	row.IsActive   = dbutil.Ptr(true)
func Ptr[T any](v T) *T {
	return &v
}

// Deref returns the pointed-to value, or the zero value of T when p is
// nil. Use when you want a safe scalar read out of a nullable column.
//
//	systemName := dbutil.Deref(row.SystemName) // "" if NULL
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// DerefOr returns *p, or def when p is nil. Used when zero-of-T isn't
// the right default (e.g. nil position should be 999, not 0).
func DerefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// SliceOrEmpty returns *p, or an empty slice when p is nil. Avoids
// chains of `if v != nil { for _, x := range *v {...} }` boilerplate.
func SliceOrEmpty[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

// ---------------------------------------------------------------------------
// String idioms
// ---------------------------------------------------------------------------

// EmptyToNil maps "" to nil and any non-empty string to *string.
// The default contract weave stores use to honour SQL NULL semantics
// for string columns.
func EmptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// TrimEmptyToNil trims s of leading/trailing whitespace and then maps
// "" to nil. Use for user-input strings that should not store
// whitespace-only values.
func TrimEmptyToNil(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// NilToEmpty returns *p, or "" when p is nil. Inverse of EmptyToNil
// for read paths that want a string for templating / JSON.
func NilToEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// TrimNullable normalises a *string by trimming whitespace and
// returning nil when the result is empty. Use on input fields that
// arrive as *string but should not store whitespace-only or empty
// values (TrimEmptyToNil is the same idiom for non-pointer inputs).
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

// ---------------------------------------------------------------------------
// int32 ↔ int (sqlc emits int32 for integer columns)
// ---------------------------------------------------------------------------

// Int32Ptr returns a pointer to int32(v). Convenience for sqlc params
// that take *int32 when the caller naturally has an int.
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

// DerefInt32ToIntPtr converts a *int32 to *int while preserving nil.
// Used where the wire shape stays *int but sqlc emits *int32 (e.g.
// nullable max_occurs where nil semantically means "unbounded" and
// must round-trip).
func DerefInt32ToIntPtr(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

// ---------------------------------------------------------------------------
// pgtype.Timestamptz ↔ *time.Time
// ---------------------------------------------------------------------------

// TimestamptzToTimePtr returns &ts.Time when valid, nil otherwise.
func TimestamptzToTimePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	return &ts.Time
}

// TimePtrToTimestamptz returns a valid pgtype.Timestamptz when t is
// non-nil, an invalid (NULL) one otherwise.
func TimePtrToTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
