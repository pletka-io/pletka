package projectread

import (
	"context"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func releasedEvidence(ctx context.Context, s *readsession.Session) (projectsnapshot.Evidence, error) {
	contract, err := s.ContentContract(ctx)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	d, err := s.Descriptors(ctx)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	switch contract {
	case 1:
		receipt, err := s.Content(ctx)
		if err != nil {
			return projectsnapshot.Evidence{}, err
		}
		reader, err := s.Ontologies(ctx)
		if err != nil {
			return projectsnapshot.Evidence{}, err
		}
		publications, err := reader.Artifacts(ctx)
		if err != nil {
			return projectsnapshot.Evidence{}, err
		}
		refs, err := reader.References(ctx)
		if err != nil {
			return projectsnapshot.Evidence{}, err
		}
		return projectsnapshot.VerifiedEvidence(receipt, d, publications, refs)
	case 2:
		receipt, err := s.LegacyContent(ctx)
		if err != nil {
			return projectsnapshot.Evidence{}, err
		}
		artifact, err := s.LegacyOntology(ctx)
		if err != nil {
			return projectsnapshot.Evidence{}, err
		}
		return projectsnapshot.LegacyEvidence(receipt, d, artifact)
	default:
		return projectsnapshot.Evidence{}, &readsession.NotInReleaseError{ProjectID: s.ProjectID(), Version: s.Scope().Version(), EntityID: "complete content contract"}
	}
}
