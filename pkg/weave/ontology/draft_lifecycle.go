package ontology

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

const draftImportRequirement = "import"

// Start creates the ontology's single draft without changing live catalog,
// namespace policy, active selection or any released rows.
func (s *DraftService) Start(ctx context.Context, owner ontologybody.OntologyID, document DraftDocument) (DraftSnapshot, error) {
	return s.start(ctx, owner, document, nil)
}
func (s *DraftService) start(ctx context.Context, owner ontologybody.OntologyID, document DraftDocument, base *string) (out DraftSnapshot, err error) {
	defer func() { err = mapDraftError(err) }()
	ref, err := NewDraftReference(owner, DraftID(ids.GenerateULID()))
	if err != nil {
		return out, err
	}
	if err := s.validateRef(ref); err != nil {
		return out, err
	}
	if err := document.validate(); err != nil {
		return out, err
	}
	data, err := json.Marshal(document)
	if err != nil {
		return out, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer func() { err = closeDraftTx(ctx, tx, err) }()
	q := sqlcgen.New(tx)
	if _, err := q.WeaveGetOntologyByID(ctx, string(owner)); errors.Is(err, pgx.ErrNoRows) {
		return out, apierror.NotFound("ontology not found")
	} else if err != nil {
		return out, err
	}
	if err := q.WeaveCreateOntologyDraft(ctx, sqlcgen.WeaveCreateOntologyDraftParams{ID: string(ref.id), OntologyID: string(owner), BaseVersionID: base, Document: data}); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	return DraftSnapshot{OntologyID: owner, ID: ref.id, BaseVersionID: base, Revision: 1, Document: document}, nil
}

// Current discovers the current draft identity. Further reads/edits bind that
// explicit reference; a missing draft never falls back to active version rows.
func (s *DraftService) Current(ctx context.Context, owner ontologybody.OntologyID) (DraftSnapshot, error) {
	if s == nil || s.pool == nil || strings.TrimSpace(string(owner)) == "" {
		return DraftSnapshot{}, draftInvalid("ontology", "ontology identity required")
	}
	id, err := sqlcgen.New(s.pool).WeaveCurrentOntologyDraft(ctx, string(owner))
	if errors.Is(err, pgx.ErrNoRows) {
		return DraftSnapshot{}, apierror.NotFound("ontology draft not found")
	}
	if err != nil {
		return DraftSnapshot{}, err
	}
	ref, err := NewDraftReference(owner, DraftID(id))
	if err != nil {
		return DraftSnapshot{}, err
	}
	var out DraftSnapshot
	err = s.Read(ctx, ref, func(r DraftReader) error { var err error; out, err = r.Snapshot(ctx); return err })
	return out, err
}

// StartFromRelease copies frozen source inputs and pins into a new draft. The
// base artifact remains retained; there is no implicit certification of legacy rows.
func (s *DraftService) StartFromRelease(ctx context.Context, owner ontologybody.OntologyID, version ontologybody.VersionID) (DraftSnapshot, error) {
	ref, err := ontologyrelease.NewReference(owner, version)
	if err != nil {
		return DraftSnapshot{}, err
	}
	if s == nil || s.pool == nil {
		return DraftSnapshot{}, draftInvalid("draft", "draft service required")
	}
	var document DraftDocument
	err = ontologyrelease.NewFactory(s.pool).Read(ctx, ref, func(r ontologyrelease.Reader) error {
		artifact, err := r.Artifact(ctx)
		if err != nil {
			return err
		}
		doc, err := artifact.Decode()
		if err != nil {
			return err
		}
		var body struct {
			Ontology DraftOntology `json:"ontology"`
			Version  struct {
				RDF        string   `json:"rdf_content"`
				Filename   string   `json:"original_filename"`
				Compatible []string `json:"compatible_base_versions"`
			} `json:"version"`
			Companions []DraftSource    `json:"companions"`
			Bindings   []DraftNamespace `json:"namespace_bindings"`
		}
		if err := json.Unmarshal([]byte(doc.BodyJSON), &body); err != nil {
			return err
		}
		if body.Version.Filename == "" {
			body.Version.Filename = body.Ontology.Prefix + ".rdf"
		}
		document = DraftDocument{FormatVersion: 1, Ontology: body.Ontology, Primary: DraftSource{Filename: body.Version.Filename, Content: body.Version.RDF}, Companions: body.Companions, Bindings: body.Bindings, CompatibleBaseVersions: body.Version.Compatible, Publication: ontologyrelease.Options{Pins: map[string]string{}}}
		for _, pin := range doc.Pins {
			document.Publication.Pins[pin.OntologyID] = pin.VersionID
		}
		for _, requirement := range doc.Requirements {
			if requirement.Disposition != "external" {
				continue
			}
			if requirement.Kind == draftImportRequirement {
				document.Publication.ExternalImports = append(document.Publication.ExternalImports, requirement.Name)
			}
			if requirement.Kind == "reference" {
				document.Publication.ExternalReferences = append(document.Publication.ExternalReferences, requirement.Name)
			}
		}
		return nil
	})
	if err != nil {
		return DraftSnapshot{}, err
	}
	base := string(version)
	return s.start(ctx, owner, document, &base)
}
