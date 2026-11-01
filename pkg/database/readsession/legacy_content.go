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
	value, err := s.LegacyInterpretation(ctx)
	if err != nil {
		return ontologyevidence.Artifact{}, err
	}
	return value.Artifact()
}

// LegacyInterpretation returns one validated detached capability. The operation's
// preparer shares exact source documents and bodies; closed sessions cannot use it.
func (s *Session) LegacyInterpretation(ctx context.Context) (ontologyevidence.Prepared, error) {
	if err := s.Check(ctx); err != nil {
		return ontologyevidence.Prepared{}, err
	}
	if s.interpretation != nil {
		return *s.interpretation, nil
	}
	receipt, err := s.LegacyContent(ctx)
	if err != nil {
		return ontologyevidence.Prepared{}, err
	}
	q, err := s.ArchiveQueries()
	if err != nil {
		return ontologyevidence.Prepared{}, err
	}
	r, err := q.ReleaseLegacyOntologySnapshot(ctx, archive.ReleaseLegacyOntologySnapshotParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if err != nil {
		return ontologyevidence.Prepared{}, err
	}
	value, err := s.lifetime.interpretations.Prepare(ontologyevidence.Artifact{JSON: r.Document, Digest: r.Digest})
	if err != nil {
		return ontologyevidence.Prepared{}, err
	}
	ref, err := value.Reference()
	if err != nil {
		return ontologyevidence.Prepared{}, err
	}
	if ref != receipt.Ontology() {
		return ontologyevidence.Prepared{}, fmt.Errorf("legacy ontology evidence differs from bound release")
	}
	s.interpretation = &value
	return value, nil
}
