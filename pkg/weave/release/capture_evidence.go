package release

import (
	"context"
	"errors"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/database/releasedescriptor"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Only the two lifecycle entry points construct this policy. Content consumers
// cannot request a permissive lookup mode or substitute a live ontology reader.
type cutEvidence interface {
	capture(context.Context, *sqlcgen.Queries, string, string, releasedescriptor.Hosts) error
	finalize(context.Context, *sqlcgen.Queries, string, string) error
}

type publishedEvidence struct{}
type legacyEvidence struct{}

func (publishedEvidence) capture(ctx context.Context, q *sqlcgen.Queries, project, version string, hosts releasedescriptor.Hosts) error {
	if err := ontologyrelease.CaptureProject(ctx, q, project, version); err != nil {
		var invalid interface{ ValidationFields() map[string][]string }
		if errors.As(err, &invalid) {
			return &ErrValidation{Fields: map[string][]string{"ontologies": {err.Error()}}}
		}
		return fmt.Errorf("capture release ontology dependencies: %w", err)
	}
	return releasedescriptor.Capture(ctx, q, project, version, hosts)
}

func (publishedEvidence) finalize(ctx context.Context, q *sqlcgen.Queries, project, version string) error {
	return q.WeaveFinalizeReleaseCapture(ctx, sqlcgen.WeaveFinalizeReleaseCaptureParams{ProjectID: project, Version: version})
}

func (legacyEvidence) capture(ctx context.Context, q *sqlcgen.Queries, project, version string, hosts releasedescriptor.Hosts) error {
	origin, err := ontologysnapshot.NewOrigin(project, version)
	if err != nil {
		return err
	}
	a, err := ontologysnapshot.Capture(ctx, q, origin)
	if err != nil {
		return err
	}
	return releasedescriptor.CaptureLegacy(ctx, q, project, version, hosts, a)
}

func (legacyEvidence) finalize(ctx context.Context, q *sqlcgen.Queries, project, version string) error {
	return q.WeaveFinalizeLegacyReleaseCapture(ctx, sqlcgen.WeaveFinalizeLegacyReleaseCaptureParams{ProjectID: project, Version: version})
}
