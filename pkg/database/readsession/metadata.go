package readsession

import (
	"context"
	"encoding/json"
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

// ContentSection is storage evidence for one owned archive section, including
// an empty section. SHA256 hashes PostgreSQL row values, not portable Git bytes.
type ContentSection struct {
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

// ContentReceipt certifies complete immutable owned content under contract 1.
// It does not certify an assembler, serializer or restore implementation.
type ContentReceipt struct{ sections map[string]ContentSection }

// Section returns a detached section value; absent and empty are distinct.
func (r ContentReceipt) Section(name string) (ContentSection, bool) {
	section, ok := r.sections[name]
	return section, ok
}

// Content requires the storage-verified complete owned-content receipt on this
// bound session. Unverified releases must be rebuilt, never filled from live data.
func (s *Session) Content(ctx context.Context) (ContentReceipt, error) {
	q, err := s.ArchiveQueries()
	if err != nil {
		return ContentReceipt{}, err
	}
	raw, err := q.ReleaseContentReceipt(ctx, archive.ReleaseContentReceiptParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if errors.Is(err, pgx.ErrNoRows) {
		return ContentReceipt{}, &NotInReleaseError{ProjectID: s.projectID, Version: s.scope.Version(), EntityID: "complete owned content"}
	}
	if err != nil {
		return ContentReceipt{}, fmt.Errorf("read content receipt: %w", err)
	}
	var document struct {
		FormatVersion int                       `json:"format_version"`
		Sections      map[string]ContentSection `json:"sections"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return ContentReceipt{}, fmt.Errorf("decode content receipt: %w", err)
	}
	if document.FormatVersion != 1 || len(document.Sections) != 22 {
		return ContentReceipt{}, errors.New("unsupported content receipt")
	}
	return ContentReceipt{sections: document.Sections}, nil
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
