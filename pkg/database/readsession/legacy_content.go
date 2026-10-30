package readsession

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/releasecontent"
)

// LegacyContentReceipt is detached contract 2 structural evidence. It does not
// certify semantic resolution or convert to the verified contract.
type LegacyContentReceipt = releasecontent.Legacy

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
	return releasecontent.DecodeLegacy(raw, s.projectID, s.scope.Version())
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
func (s *Session) LegacyOntology(ctx context.Context) (ontologyevidence.Artifact, error) {
	if _, err := s.LegacyContent(ctx); err != nil {
		return ontologyevidence.Artifact{}, err
	}
	q, err := s.ArchiveQueries()
	if err != nil {
		return ontologyevidence.Artifact{}, err
	}
	r, err := q.ReleaseLegacyOntologySnapshot(ctx, archive.ReleaseLegacyOntologySnapshotParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if err != nil {
		return ontologyevidence.Artifact{}, err
	}
	a := ontologyevidence.Artifact{JSON: r.Document, Digest: r.Digest}
	d, err := a.Decode()
	if err != nil {
		return ontologyevidence.Artifact{}, err
	}
	if d.ProjectID != s.projectID || d.Version != s.scope.Version() {
		return ontologyevidence.Artifact{}, fmt.Errorf("legacy ontology evidence differs from bound release")
	}
	return a, nil
}
