package projectread

import (
	"context"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/database/readsession"
)

func TestPageLoadingReturnsNoPartialRowsOnFailure(t *testing.T) {
	failure := errors.New("section query failed")
	for _, mode := range []string{"query", "unloaded", "oversized", "rows", "bytes", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			limits := DefaultOptions().Limits
			limits.PageSize = 2
			if mode == "rows" {
				limits.MaxRows = 2
			}
			if mode == "bytes" {
				limits.MaxBytes = 1
			}
			b := budget{limits: limits}
			calls := 0
			got, err := pages(ctx, &b, func(context.Context, readsession.Page) ([]string, error) {
				calls++
				if calls == 1 {
					return []string{"first", "second"}, nil
				}
				switch mode {
				case "query":
					return nil, failure
				case "unloaded":
					return nil, nil
				case "oversized":
					return []string{"a", "b", "c"}, nil
				case "cancel":
					cancel()
					return []string{"a", "b"}, nil
				default:
					return []string{"a"}, nil
				}
			})
			if err == nil || got != nil {
				t.Fatalf("partial success: %v %v", got, err)
			}
			if mode == "query" && !errors.Is(err, failure) {
				t.Fatal("query error lost")
			}
		})
	}
}

func TestAssemblerRejectsUninitializedInputs(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("unlimited zero options")
	}
	a, err := New(DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []*readsession.Session{nil, {}} {
		fragment, err := a.Entity(t.Context(), s, "field", "id")
		if err == nil {
			t.Fatal("entity accepted zero session")
		}
		if _, err := fragment.Root(); err == nil {
			t.Fatal("entity returned partial value")
		}
		value, err := a.Project(t.Context(), s)
		if err == nil {
			t.Fatal("zero session accepted")
		}
		if _, err := value.Root(); err == nil {
			t.Fatal("invalid session returned partial value")
		}
	}
}
