package dbutil

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestStringPointerHelpers(t *testing.T) {
	if Deref(Ptr("hello")) != "hello" {
		t.Fatalf("Deref(Ptr(hello)) != hello")
	}
	var nilString *string
	if Deref(nilString) != "" {
		t.Fatalf("Deref(nil string) != empty string")
	}
	if DerefOr(nilString, "fallback") != "fallback" {
		t.Fatalf("DerefOr(nil, fallback) != fallback")
	}
	values := []string{"a", "b"}
	if got := SliceOrEmpty(&values); len(got) != 2 || got[0] != "a" {
		t.Fatalf("SliceOrEmpty(values) = %#v", got)
	}
	if got := SliceOrEmpty[string](nil); got != nil {
		t.Fatalf("SliceOrEmpty(nil) = %#v", got)
	}
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
	if TrimEmptyToNil("  ") != nil {
		t.Fatalf("TrimEmptyToNil(spaces) != nil")
	}
	trimmed := TrimEmptyToNil(" value ")
	if trimmed == nil || *trimmed != "value" {
		t.Fatalf("TrimEmptyToNil(value) = %#v", trimmed)
	}
	if TrimNullable(nil) != nil {
		t.Fatalf("TrimNullable(nil) != nil")
	}
	if TrimNullable(Ptr("  ")) != nil {
		t.Fatalf("TrimNullable(spaces) != nil")
	}
	trimmedPtr := TrimNullable(Ptr(" value "))
	if trimmedPtr == nil || *trimmedPtr != "value" {
		t.Fatalf("TrimNullable(value) = %#v", trimmedPtr)
	}
}

func TestInt32Helpers(t *testing.T) {
	if got := Int32Ptr(12); got == nil || *got != 12 {
		t.Fatalf("Int32Ptr(12) = %#v", got)
	}
	if DerefInt32(nil) != 0 {
		t.Fatalf("DerefInt32(nil) != 0")
	}
	value := int32(12)
	if DerefInt32(&value) != 12 {
		t.Fatalf("DerefInt32(value) != 12")
	}
	if DerefInt32ToIntPtr(nil) != nil {
		t.Fatalf("DerefInt32ToIntPtr(nil) != nil")
	}
	intPtr := DerefInt32ToIntPtr(&value)
	if intPtr == nil || *intPtr != 12 {
		t.Fatalf("DerefInt32ToIntPtr(value) = %#v", intPtr)
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
