package dbutil

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestPtr_Deref_RoundTrip(t *testing.T) {
	if got := Deref(Ptr("hello")); got != "hello" {
		t.Errorf("Deref(Ptr(\"hello\")) = %q; want hello", got)
	}
	if got := Deref(Ptr(42)); got != 42 {
		t.Errorf("Deref(Ptr(42)) = %d; want 42", got)
	}
	if got := Deref(Ptr(true)); !got {
		t.Errorf("Deref(Ptr(true)) = false; want true")
	}
}

func TestDeref_NilReturnsZero(t *testing.T) {
	var sp *string
	if got := Deref(sp); got != "" {
		t.Errorf("Deref(nil *string) = %q; want empty", got)
	}
	var ip *int
	if got := Deref(ip); got != 0 {
		t.Errorf("Deref(nil *int) = %d; want 0", got)
	}
}

func TestDerefOr(t *testing.T) {
	if got := DerefOr(Ptr(5), 999); got != 5 {
		t.Errorf("DerefOr(Ptr(5), 999) = %d; want 5", got)
	}
	var ip *int
	if got := DerefOr(ip, 999); got != 999 {
		t.Errorf("DerefOr(nil, 999) = %d; want 999", got)
	}
}

func TestSliceOrEmpty(t *testing.T) {
	in := []string{"a", "b"}
	if got := SliceOrEmpty(&in); len(got) != 2 || got[0] != "a" {
		t.Errorf("SliceOrEmpty = %v; want [a b]", got)
	}
	var sp *[]string
	if got := SliceOrEmpty(sp); got != nil {
		t.Errorf("SliceOrEmpty(nil) = %v; want nil", got)
	}
}

func TestEmptyToNil(t *testing.T) {
	if got := EmptyToNil(""); got != nil {
		t.Errorf("EmptyToNil(\"\") = %v; want nil", got)
	}
	if got := EmptyToNil("hi"); got == nil || *got != "hi" {
		t.Errorf("EmptyToNil(\"hi\") = %v; want *hi", got)
	}
	// Whitespace must NOT be trimmed.
	if got := EmptyToNil("  "); got == nil || *got != "  " {
		t.Errorf("EmptyToNil(\"  \") = %v; want *\"  \" (whitespace preserved)", got)
	}
}

func TestTrimEmptyToNil(t *testing.T) {
	cases := []struct {
		in   string
		want *string
	}{
		{"", nil},
		{"   ", nil},
		{"\t\n", nil},
		{"hi", Ptr("hi")},
		{"  hi  ", Ptr("hi")},
	}
	for _, tc := range cases {
		got := TrimEmptyToNil(tc.in)
		switch {
		case got == nil && tc.want == nil:
		case got != nil && tc.want != nil && *got == *tc.want:
		default:
			t.Errorf("TrimEmptyToNil(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

func TestNilToEmpty(t *testing.T) {
	var sp *string
	if got := NilToEmpty(sp); got != "" {
		t.Errorf("NilToEmpty(nil) = %q; want empty", got)
	}
	if got := NilToEmpty(Ptr("hi")); got != "hi" {
		t.Errorf("NilToEmpty(Ptr(hi)) = %q; want hi", got)
	}
}

func TestInt32Helpers(t *testing.T) {
	if got := *Int32Ptr(7); got != int32(7) {
		t.Errorf("Int32Ptr(7) = %d; want 7", got)
	}
	if got := DerefInt32(nil); got != 0 {
		t.Errorf("DerefInt32(nil) = %d; want 0", got)
	}
	v := int32(42)
	if got := DerefInt32(&v); got != 42 {
		t.Errorf("DerefInt32(&42) = %d; want 42", got)
	}
	if got := DerefInt32ToIntPtr(nil); got != nil {
		t.Errorf("DerefInt32ToIntPtr(nil) = %v; want nil", got)
	}
	if got := DerefInt32ToIntPtr(&v); got == nil || *got != 42 {
		t.Errorf("DerefInt32ToIntPtr(&42) = %v; want *42", got)
	}
}

func TestTimestamptzRoundTrip(t *testing.T) {
	if got := TimestamptzToTimePtr(pgtype.Timestamptz{Valid: false}); got != nil {
		t.Errorf("invalid Timestamptz -> %v; want nil", got)
	}
	now := time.Date(2026, 5, 10, 9, 30, 0, 0, time.UTC)
	ts := TimePtrToTimestamptz(&now)
	if !ts.Valid || !ts.Time.Equal(now) {
		t.Errorf("TimePtrToTimestamptz(&now) = %+v; want valid + matching", ts)
	}
	if got := TimestamptzToTimePtr(ts); got == nil || !got.Equal(now) {
		t.Errorf("round-trip lost data: %v vs %v", got, now)
	}
	if got := TimePtrToTimestamptz(nil); got.Valid {
		t.Errorf("TimePtrToTimestamptz(nil) = %+v; want invalid", got)
	}
}
