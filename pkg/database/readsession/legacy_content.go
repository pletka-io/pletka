package readsession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
)

// LegacyContentReceipt proves complete immutable structural content and binds
// the preserved ontology evidence. It does not certify semantic resolution.
type LegacyContentReceipt struct {
	content  ContentReceipt
	ontology ontologysnapshot.Reference
}

// Section distinguishes an explicitly captured empty section from an absent one.
func (r LegacyContentReceipt) Section(name string) (ContentSection, bool) {
	return r.content.Section(name)
}

// Ontology returns the exact project/release/digest binding of this receipt.
func (r LegacyContentReceipt) Ontology() ontologysnapshot.Reference { return r.ontology }

// LegacyContent reads only contract 2 evidence. Ordinary Content still requires
// contract 1; neither method substitutes another contract or current data.
func (s *Session) LegacyContent(ctx context.Context) (LegacyContentReceipt, error) {
	q, err := s.ArchiveQueries()
	if err != nil {
		return LegacyContentReceipt{}, err
	}
	raw, err := q.ReleaseLegacyContentReceipt(ctx, archive.ReleaseLegacyContentReceiptParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if errors.Is(err, pgx.ErrNoRows) {
		return LegacyContentReceipt{}, &NotInReleaseError{ProjectID: s.projectID, Version: s.scope.Version(), EntityID: "legacy complete owned content"}
	}
	if err != nil {
		return LegacyContentReceipt{}, fmt.Errorf("read legacy content receipt: %w", err)
	}
	var d struct {
		FormatVersion int                       `json:"format_version"`
		EvidenceKind  string                    `json:"evidence_kind"`
		Digest        string                    `json:"ontology_snapshot_digest"`
		Sections      map[string]ContentSection `json:"sections"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return LegacyContentReceipt{}, err
	}
	if d.FormatVersion != 2 || d.EvidenceKind != "legacy_ontology_snapshot" || len(d.Sections) != 22 {
		return LegacyContentReceipt{}, fmt.Errorf("unsupported legacy content receipt")
	}
	ref, err := ontologysnapshot.NewReference(s.projectID, s.scope.Version(), d.Digest)
	if err != nil {
		return LegacyContentReceipt{}, err
	}
	return LegacyContentReceipt{content: ContentReceipt{sections: d.Sections}, ontology: ref}, nil
}

// ContentContract reports the captured contract, never inferring it from a failed
// reader lookup. Zero denotes an unfinished historical cut, not legacy evidence.
func (s *Session) ContentContract(ctx context.Context) (int16, error) {
	q, err := s.ArchiveQueries()
	if err != nil {
		return 0, err
	}
	return q.ReleaseContentContract(ctx, archive.ReleaseContentContractParams{ProjectID: s.projectID, Version: s.scope.Version()})
}

// LegacyOntology returns detached evidence bound to this session's completed cut.
func (s *Session) LegacyOntology(ctx context.Context) (ontologysnapshot.Artifact, error) {
	if _, err := s.LegacyContent(ctx); err != nil {
		return ontologysnapshot.Artifact{}, err
	}
	q, err := s.ArchiveQueries()
	if err != nil {
		return ontologysnapshot.Artifact{}, err
	}
	r, err := q.ReleaseLegacyOntologySnapshot(ctx, archive.ReleaseLegacyOntologySnapshotParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if err != nil {
		return ontologysnapshot.Artifact{}, err
	}
	a := ontologysnapshot.Artifact{JSON: r.Document, Digest: r.Digest}
	d, err := a.Decode()
	if err != nil {
		return ontologysnapshot.Artifact{}, err
	}
	if d.ProjectID != s.projectID || d.Version != s.scope.Version() {
		return ontologysnapshot.Artifact{}, fmt.Errorf("legacy ontology evidence differs from bound release")
	}
	return a, nil
}
