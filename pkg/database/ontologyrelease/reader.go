package ontologyrelease

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	readsql "github.com/pletka-io/pletka/pkg/database/readsql/ontologyrelease"
)

// Reader is bound to a published owner/version. Dependency accepts only an owner
// in its recorded closure; it cannot select a version or consult active policy.
type Reader interface {
	Artifact(context.Context) (Artifact, error)
	Dependency(context.Context, ontologybody.OntologyID) (Artifact, error)
}
type reader struct {
	queries readsql.Querier
	ref     Reference
	closed  bool
}

func (r *reader) Artifact(ctx context.Context) (Artifact, error) { return r.read(ctx, r.ref) }
func (r *reader) read(ctx context.Context, ref Reference) (Artifact, error) {
	if r.closed {
		return Artifact{}, fmt.Errorf("ontology release reader is closed")
	}
	row, err := r.queries.OntologyRelease(ctx, readsql.OntologyReleaseParams{OntologyID: string(ref.body.OntologyID()), VersionID: string(ref.body.VersionID())})
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, &missingError{owner: string(ref.body.OntologyID()), version: string(ref.body.VersionID())}
	}
	if err != nil {
		return Artifact{}, err
	}
	artifact := Artifact{JSON: row.Artifact, Digest: row.Digest}
	doc, err := artifact.Decode()
	if err != nil {
		return Artifact{}, err
	}
	if doc.OntologyID != string(ref.body.OntologyID()) || doc.VersionID != string(ref.body.VersionID()) {
		return Artifact{}, fmt.Errorf("ontology release reader ownership mismatch")
	}
	return artifact, nil
}
func (r *reader) Dependency(ctx context.Context, owner ontologybody.OntologyID) (Artifact, error) {
	root, err := r.Artifact(ctx)
	if err != nil {
		return Artifact{}, err
	}
	doc, err := root.Decode()
	if err != nil {
		return Artifact{}, err
	}
	for _, pin := range doc.Pins {
		if pin.OntologyID == string(owner) {
			ref, err := NewReference(owner, ontologybody.VersionID(pin.VersionID))
			if err != nil {
				return Artifact{}, err
			}
			artifact, err := r.read(ctx, ref)
			if err != nil {
				return Artifact{}, err
			}
			if artifact.Digest != pin.Digest {
				return Artifact{}, fmt.Errorf("ontology dependency digest mismatch")
			}
			return artifact, nil
		}
	}
	return Artifact{}, &missingError{owner: string(owner), version: "not captured"}
}
