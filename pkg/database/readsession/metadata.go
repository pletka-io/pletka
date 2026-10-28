package readsession

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
)

// ReleaseMetadata is detached evidence for capture contract 1: concept namespace
// and model types only. It does not certify complete project content.
type ReleaseMetadata struct {
	rawNamespace       *string
	effectiveNamespace string
	modelTypes         map[string]string
}

// ConceptNamespace preserves null versus an authored empty or whitespace value.
func (m ReleaseMetadata) ConceptNamespace() *string {
	if m.rawNamespace == nil {
		return nil
	}
	value := *m.rawNamespace
	return &value
}

// EffectiveConceptNamespace is the normalized namespace/default frozen at the cut.
func (m ReleaseMetadata) EffectiveConceptNamespace() string { return m.effectiveNamespace }

// ModelType returns false for a model outside this project's captured membership.
func (m ReleaseMetadata) ModelType(id string) (string, bool) {
	value, ok := m.modelTypes[id]
	return value, ok
}

// Metadata requires explicit capture evidence on the bound release. In particular,
// an old model archive's default "core" is not evidence of its original type.
func (s *Session) Metadata(ctx context.Context) (ReleaseMetadata, error) {
	q, err := s.ArchiveQueries()
	if err != nil {
		return ReleaseMetadata{}, err
	}
	ns, err := q.ReleaseConceptNamespace(ctx, archive.ReleaseConceptNamespaceParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReleaseMetadata{}, &NotInReleaseError{ProjectID: s.projectID, Version: s.scope.Version(), EntityID: "captured project metadata"}
	}
	if err != nil {
		return ReleaseMetadata{}, fmt.Errorf("read captured concept namespace: %w", err)
	}
	rows, err := q.ReleaseModelTypes(ctx, archive.ReleaseModelTypesParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if err != nil {
		return ReleaseMetadata{}, fmt.Errorf("read captured model types: %w", err)
	}
	m := ReleaseMetadata{rawNamespace: ns.ConceptNamespace, effectiveNamespace: ns.EffectiveNamespace, modelTypes: make(map[string]string, len(rows))}
	for _, row := range rows {
		m.modelTypes[row.ID] = row.ModelType
	}
	return m, nil
}
