package auth_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
)

func TestFromContext_Missing(t *testing.T) {
	got := auth.FromContext(context.Background())
	if got == nil {
		t.Fatal("FromContext should never return nil; got nil")
	}
	if !got.IsAnonymous {
		t.Error("missing-snapshot fallback should be anonymous")
	}
}

func TestFromContext_Present(t *testing.T) {
	want := &auth.AuthSnapshot{ActorID: "u1"}
	ctx := auth.WithSnapshot(context.Background(), want)
	got := auth.FromContext(ctx)
	if got != want {
		t.Errorf("FromContext: got %p want %p", got, want)
	}
}
