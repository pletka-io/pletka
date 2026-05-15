package dbutil

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestStringPointerHelpers(t *testing.T) {
	if EmptyToNil("") != nil {
		t.Fatalf("EmptyToNil(\"\") != nil")
	}
	value := EmptyToNil("value")
	if value == nil || *value != "value" {
		t.Fatalf("EmptyToNil(value) = %#v", value)
	}
	if NilToEmpty(nil) != "" {
		t.Fatalf("NilToEmpty(nil) != empty string")
	}
	if NilToEmpty(value) != "value" {
		t.Fatalf("NilToEmpty(value) != value")
	}
}

func TestTimestamptzHelpers(t *testing.T) {
	when := time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)
	ts := TimePtrToTimestamptz(&when)
	if !ts.Valid || !ts.Time.Equal(when) {
		t.Fatalf("TimePtrToTimestamptz() = %#v", ts)
	}
	if TimePtrToTimestamptz(nil).Valid {
		t.Fatalf("TimePtrToTimestamptz(nil).Valid = true")
	}
	if got := TimestamptzToTimePtr(pgtype.Timestamptz{}); got != nil {
		t.Fatalf("TimestamptzToTimePtr(invalid) = %#v", got)
	}
	got := TimestamptzToTimePtr(ts)
	if got == nil || !got.Equal(when) {
		t.Fatalf("TimestamptzToTimePtr(valid) = %#v", got)
	}
}
