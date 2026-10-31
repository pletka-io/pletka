//go:build integration

package vocabulary_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

func TestConceptNamespaceDoesNotDefaultOnReadFailure(t *testing.T) {
	svc := vocabulary.NewService(testdb.Pool(t), nil)
	if ns, err := vocabulary.TestConceptNamespace(svc, t.Context(), "nonexistent-namespace-project"); !errors.Is(err, pgx.ErrNoRows) || ns != "" {
		t.Fatalf("missing project = %q %v", ns, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if ns, err := vocabulary.TestConceptNamespace(svc, ctx, "SRD"); !errors.Is(err, context.Canceled) || ns != "" {
		t.Fatalf("canceled lookup = %q %v", ns, err)
	}
}
