package ontology

import (
	"context"

	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
)

// RestoreArtifacts restores exact published content without reparsing source
// RDF or changing current namespace/catalog policy. IDs are verified, never skipped.
func (s *Service) RestoreArtifacts(ctx context.Context, artifacts []ontologyrelease.Artifact) error {
	return s.store.RestoreArtifacts(ctx, artifacts)
}
func (s *postgresStore) RestoreArtifacts(ctx context.Context, artifacts []ontologyrelease.Artifact) error {
	return ontologyrelease.Restore(ctx, s.pool, artifacts)
}
