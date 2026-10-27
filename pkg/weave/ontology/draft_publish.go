package ontology

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/ontologydraft"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	writesql "github.com/pletka-io/pletka/pkg/database/writesql/ontologydraft"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// Publish freezes one expected draft revision through the same body writer,
// seal and dependency publisher used by imports. Failure rolls back catalog,
// namespaces, content, policy and receipt; the editable draft is preserved.
func (s *DraftService) Publish(ctx context.Context, ref DraftReference, revision int64, request DraftPublishRequest) (out DraftPublication, err error) {
	defer func() { err = mapDraftError(err) }()
	if err := s.validateRef(ref); err != nil {
		return out, err
	}
	if err := request.validate(); err != nil {
		return out, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	r := &draftReader{queries: readsql.New(tx), ref: ref}
	defer func() { r.closed = true; err = closeDraftTx(ctx, tx, err) }()
	if err := lockDraft(ctx, writesql.New(tx), ref, revision); err != nil {
		return out, err
	}
	q := sqlcgen.New(tx)
	existing, err := q.WeaveOntologyDraftPublication(ctx, sqlcgen.WeaveOntologyDraftPublicationParams{OntologyID: string(ref.owner), DraftID: string(ref.id), Revision: revision})
	if err == nil {
		var previous DraftPublishRequest
		if err := json.Unmarshal(existing.Request, &previous); err != nil {
			return out, err
		}
		if previous != request {
			return out, apierror.Conflict("draft revision already published with a different request; edit the draft or change active policy separately")
		}
		out = DraftPublication{OntologyID: ref.owner, DraftID: ref.id, Revision: revision, VersionID: existing.VersionID, Digest: existing.Digest}
		return out, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	snapshot, err := r.Snapshot(ctx)
	if err != nil {
		return out, err
	}
	input, err := compileOntologyDraft(string(ref.owner), snapshot.Document, request)
	if err != nil {
		return out, err
	}
	d := snapshot.Document.Ontology
	if _, err := q.WeaveUpdateOntology(ctx, sqlcgen.WeaveUpdateOntologyParams{ID: string(ref.owner), Prefix: d.Prefix, Namespace: d.Namespace, Name: d.Name, Description: marshalTranslations(d.Description), FamilyID: d.FamilyID, OntologyType: string(d.Type), ExtendsOntologyID: d.ExtendsOntologyID, HomepageUrl: nullableDraftString(d.HomepageURL), SourceUrl: nullableDraftString(d.SourceURL)}); err != nil {
		return out, err
	}
	store := &postgresStore{pool: s.pool, queries: q}
	version, err := store.writeImportedVersion(ctx, tx, input, "")
	if err != nil {
		return out, err
	}
	if err := publishImportedOntology(ctx, q, version, input.Publication); err != nil {
		return out, err
	}
	digest, err := q.WeaveOntologyDraftPublishedDigest(ctx, sqlcgen.WeaveOntologyDraftPublishedDigestParams{OntologyID: string(ref.owner), VersionID: version.ID})
	if err != nil {
		return out, err
	}
	data, err := json.Marshal(request)
	if err != nil {
		return out, err
	}
	if err := q.WeaveRecordOntologyDraftPublication(ctx, sqlcgen.WeaveRecordOntologyDraftPublicationParams{OntologyID: string(ref.owner), DraftID: string(ref.id), Revision: revision, VersionID: version.ID, Digest: digest, Request: data}); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, domain.Event{Type: domain.EventOntologyVersionImported, EntityID: version.ID})
	}
	return DraftPublication{OntologyID: ref.owner, DraftID: ref.id, Revision: revision, VersionID: version.ID, Digest: digest, Created: true}, nil
}
func nullableDraftString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
