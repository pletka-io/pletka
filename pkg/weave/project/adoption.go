package project

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func (h *Handler) Adoptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if strings.TrimSpace(projectID) == "" {
		writeError(w, r, http.StatusBadRequest, "projectID is required")
		return
	}

	opts := []domain.QueryOption{domain.WithProjectID(projectID)}
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		opts = append(opts, domain.WithVersion(version))
	}
	for _, key := range []string{"context_entity_type", "context_entity_id", "entity_type", "source_project_id", "source_entity_id"} {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			opts = append(opts, domain.WithFilter(key, value))
		}
	}

	adoptions, err := h.weave.Adoptions().List(ctx, opts...)
	if err != nil {
		h.log.Error("list adoptions failed", "project_id", projectID, "err", err)
		apierror.Write(w, r, apierror.InternalWith("failed to list adoptions", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"adoptions": adoptions,
	})
}

func buildAdoptionsFromOverrideCategories(
	projectID string,
	contextEntityType string,
	contextEntityID string,
	categories []overrideEditorCategory,
	createdByID *string,
) []domain.Adoption {
	adoptions := make([]domain.Adoption, 0)
	seen := map[string]struct{}{}

	for _, category := range categories {
		for _, item := range category.Items {
			switch item.Widget {
			case "collection-group":
				source := domain.ParseSemanticID(item.ID)
				if source.Valid() && source.ProjectID != "" && source.ProjectID != projectID {
					adoption := domain.Adoption{
						ProjectID:         projectID,
						ContextEntityType: contextEntityType,
						ContextEntityID:   contextEntityID,
						EntityType:        "collection",
						SourceProjectID:   source.ProjectID,
						SourceEntityID:    source.ID(),
						CreatedByID:       createdByID,
					}
					key := adoptionReceiptKey(adoption)
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						adoptions = append(adoptions, adoption)
					}
				}
			default:
				for _, field := range item.Fields {
					source := domain.ParseSemanticID(field.FieldID)
					if !source.Valid() || source.ProjectID == "" || source.ProjectID == projectID {
						continue
					}
					adoption := domain.Adoption{
						ProjectID:         projectID,
						ContextEntityType: contextEntityType,
						ContextEntityID:   contextEntityID,
						EntityType:        "field",
						SourceProjectID:   source.ProjectID,
						SourceEntityID:    source.ID(),
						CreatedByID:       createdByID,
					}
					key := adoptionReceiptKey(adoption)
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
					adoptions = append(adoptions, adoption)
				}
			}
		}
	}

	return adoptions
}

func adoptionReceiptKey(a domain.Adoption) string {
	return strings.Join([]string{
		a.ContextEntityType,
		a.ContextEntityID,
		a.EntityType,
		a.SourceProjectID,
		a.SourceEntityID,
		a.SourceVersion,
	}, "|")
}

type adoptableOption struct {
	Value             string              `json:"value"`
	Label             domain.Translations `json:"label"`
	SemanticID        string              `json:"semantic_id,omitempty"`
	SourceProjectID   string              `json:"source_project_id"`
	SourceProjectName string              `json:"source_project_name,omitempty"`
	OntologyScope     string              `json:"ontology_scope,omitempty"`
	// SourceVersion is the release this candidate was read from. It is
	// echoed back on adopt and stored, so a receipt records what was
	// actually adopted rather than leaving it to be inferred later.
	SourceVersion string `json:"source_version"`
}

// Narrow readers the adoption picker needs, declared here and satisfied by
// the model, collection and release services. The project slice names what
// it needs rather than depending on those packages (see
// docs-oss/architecture/slices.md, "Reader-interface pattern").
//
// ListAt takes a scope rather than reading whatever version the request
// carries, because the picker is reading ANOTHER project: the ancestor's own
// release is the right version, and the adopter's is not. That is also the
// scoped-reader shape the read model is converging on, so these interfaces
// survive the model and collection conversions unchanged.

// ModelVersionLister lists an ancestor's models at a named scope.
type ModelVersionLister interface {
	ListAt(ctx context.Context, scope weaveauth.ReadScope, projectID string, opts ...domain.QueryOption) ([]*domain.Model, int64, error)
}

// CollectionVersionLister lists an ancestor's collections at a named scope.
type CollectionVersionLister interface {
	ListAt(ctx context.Context, scope weaveauth.ReadScope, projectID string, opts ...domain.QueryOption) ([]*domain.Collection, int64, error)
}

// ReleaseLister lists a project's releases, newest first. The picker
// filters the withdrawn ones out itself: a release archived after it was cut
// is not something to adopt from, and ArchivedAt is the only signal.
type ReleaseLister interface {
	ListByProject(ctx context.Context, projectID string) ([]release.Release, error)
}

// adoptableSource is one ancestor a curator can adopt from, with the
// releases available to choose between.
type adoptableSource struct {
	ProjectID       string   `json:"project_id"`
	ProjectName     string   `json:"project_name,omitempty"`
	Versions        []string `json:"versions"`
	SelectedVersion string   `json:"selected_version,omitempty"`
	// Unreleased says this ancestor has published nothing, so it offers no
	// candidates. The picker shows the reason rather than an empty group,
	// because "nothing here" and "nothing published yet" are different and
	// only the second tells the curator what to do about it.
	Unreleased bool `json:"unreleased,omitempty"`
}

// liveReleaseVersions returns a project's non-withdrawn release versions,
// newest first. A release archived after it was cut is excluded: LA 0.2.0
// was archived the same day ("cut prematurely"), and adopting from a
// retracted release is worse than not adopting at all.
func (h *Handler) liveReleaseVersions(ctx context.Context, projectID string) ([]string, error) {
	rels, err := h.releases.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rels))
	for _, rel := range rels {
		if rel.ArchivedAt != nil {
			continue
		}
		out = append(out, rel.Version)
	}
	return out, nil
}

func containsString(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

// AdoptableEntities handles
//
//	GET /projects/{projectID}/adoptable/{entityType}
//
// Returns candidate models or collections from the project's ancestor
// chain that aren't already local to the project and haven't been
// adopted yet. Drives the Adopt-existing side panel.
func (h *Handler) AdoptableEntities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	entityType := chi.URLParam(r, "entityType")
	if projectID == "" || entityType == "" {
		writeError(w, r, http.StatusBadRequest, "projectID and entityType are required")
		return
	}
	switch entityType {
	case "model", "collection":
	default:
		writeError(w, r, http.StatusBadRequest, "entityType must be model or collection")
		return
	}

	chain, err := h.weave.Projects().ProjectChain(ctx, projectID)
	if err != nil || len(chain) == 0 {
		chain = []string{projectID}
	}

	existingAdoptions, err := h.weave.Adoptions().List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
		domain.WithFilter("entity_type", entityType),
	)
	if err != nil {
		h.log.Error("list adoptions for adoptable options", "project_id", projectID, "entity_type", entityType, "err", err)
		apierror.Write(w, r, apierror.InternalWith("failed to load existing adoptions", err))
		return
	}
	adoptedKey := func(sourceProjectID, sourceEntityID string) string {
		return sourceProjectID + "|" + sourceEntityID
	}
	adopted := make(map[string]bool, len(existingAdoptions))
	for _, a := range existingAdoptions {
		adopted[adoptedKey(a.SourceProjectID, a.SourceEntityID)] = true
	}

	projectNames := make(map[string]string, len(chain))
	for _, pid := range chain {
		if p, perr := h.weave.Projects().GetByID(ctx, pid); perr == nil && p != nil {
			projectNames[pid] = p.UIName.Get("en", p.ID)
		}
	}

	// Which release to read each ancestor at. A curator may deliberately
	// want an older, stable release rather than the newest, so the choice is
	// theirs: ?source_project_id=X&source_version=Y pins one ancestor, and
	// everything else defaults to its latest live release.
	wantProject := strings.TrimSpace(r.URL.Query().Get("source_project_id"))
	wantVersion := strings.TrimSpace(r.URL.Query().Get("source_version"))

	opts := make([]adoptableOption, 0)
	sources := make([]adoptableSource, 0)
	seen := map[string]bool{}
	for _, pid := range chain {
		if pid == projectID {
			// Skip own project — adopting your own row is a no-op.
			continue
		}

		live, lerr := h.liveReleaseVersions(ctx, pid)
		if lerr != nil {
			h.log.Warn("list releases for adoptable options", "project_id", projectID, "ancestor", pid, "err", lerr)
			continue
		}
		src := adoptableSource{ProjectID: pid, ProjectName: projectNames[pid], Versions: live}
		if len(live) == 0 {
			// Nothing published, so nothing to adopt. Said explicitly: an
			// empty group reads as "no candidates", which does not tell the
			// curator that the fix is for the source project to publish.
			src.Unreleased = true
			sources = append(sources, src)
			continue
		}

		version := live[0] // newest live release
		if wantProject == pid && wantVersion != "" {
			if !containsString(live, wantVersion) {
				writeError(w, r, http.StatusBadRequest,
					fmt.Sprintf("%s has no live release %s", pid, wantVersion))
				return
			}
			version = wantVersion
		}
		src.SelectedVersion = version
		sources = append(sources, src)

		scope := weaveauth.Release(version)
		switch entityType {
		case "model":
			models, _, merr := h.models.ListAt(ctx, scope, pid)
			if merr != nil {
				h.log.Warn("list models for adoptable options", "project_id", projectID, "ancestor", pid, "version", version, "err", merr)
				continue
			}
			for _, m := range models {
				if seen[m.ID] || adopted[adoptedKey(pid, m.ID)] {
					continue
				}
				seen[m.ID] = true
				opts = append(opts, adoptableOption{
					Value: m.ID, Label: m.UIName, SemanticID: m.SemanticID,
					SourceProjectID: pid, SourceProjectName: projectNames[pid],
					OntologyScope: m.OntologyScope.PrefixedName(), SourceVersion: version,
				})
			}
		case "collection":
			collections, _, cerr := h.collections.ListAt(ctx, scope, pid)
			if cerr != nil {
				h.log.Warn("list collections for adoptable options", "project_id", projectID, "ancestor", pid, "version", version, "err", cerr)
				continue
			}
			for _, c := range collections {
				if seen[c.ID] || adopted[adoptedKey(pid, c.ID)] {
					continue
				}
				seen[c.ID] = true
				opts = append(opts, adoptableOption{
					Value: c.ID, Label: c.UIName, SemanticID: c.SemanticID,
					SourceProjectID: pid, SourceProjectName: projectNames[pid],
					OntologyScope: c.OntologyScope.PrefixedName(), SourceVersion: version,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"options": opts, "sources": sources})
}

// entityInRelease reports whether the source entity is carried by that
// release. The release existing is not enough: an entity created after a
// release was cut is absent from its archive, and pinning to it would name
// a version that never contained the thing adopted.
func (h *Handler) entityInRelease(ctx context.Context, entityType, projectID, entityID, version string) (bool, error) {
	switch entityType {
	case "model":
		m, err := h.weave.Models().GetByIDVersion(ctx, projectID, entityID, version)
		return m != nil, err
	case "collection":
		c, err := h.weave.Collections().GetByIDVersion(ctx, projectID, entityID, version)
		return c != nil, err
	}
	return false, fmt.Errorf("unsupported entity type %q", entityType)
}

// CreateProjectAdoption handles
//
//	POST /projects/{projectID}/adoptions
//
// Appends an adoption receipt for a single source entity. Reads
// existing adoptions, appends the new row, and writes back via
// ReplaceForContext (the AdoptionStore's append semantics). Drives the
// Adopt-existing side-panel submit.
func (h *Handler) CreateProjectAdoption(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeError(w, r, http.StatusBadRequest, "projectID is required")
		return
	}

	project, err := h.svc.Get(ctx, projectID)
	if err != nil || project == nil {
		writeError(w, r, http.StatusNotFound, "project not found")
		return
	}
	if !h.svc.CanEdit(ctx, project) {
		writeEditDenied(w, r)
		return
	}

	var body struct {
		EntityType      string `json:"entity_type"`
		SourceProjectID string `json:"source_project_id"`
		SourceEntityID  string `json:"source_entity_id"`
		// SourceVersion is the source's release this adoption is taken
		// from. The picker sends back whatever it listed the candidate at.
		SourceVersion string `json:"source_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	body.EntityType = strings.TrimSpace(body.EntityType)
	body.SourceProjectID = strings.TrimSpace(body.SourceProjectID)
	body.SourceEntityID = strings.TrimSpace(body.SourceEntityID)
	body.SourceVersion = strings.TrimSpace(body.SourceVersion)
	if body.EntityType == "" || body.SourceProjectID == "" || body.SourceEntityID == "" {
		writeError(w, r, http.StatusBadRequest, "entity_type, source_project_id, and source_entity_id are required")
		return
	}
	switch body.EntityType {
	case "model", "collection":
	default:
		writeError(w, r, http.StatusBadRequest, "entity_type must be model or collection")
		return
	}
	// Adoption is against a release, never against a draft. Without this a
	// project can take a dependency on content that has never been
	// published -- which is how 32 adoptions on production came to point at
	// entities no release contains, with nothing to pin them to.
	//
	// The version must be named, must be a live release of the source, and
	// must actually carry the entity. A caller sending a version the picker
	// never offered is the case this exists to refuse.
	if body.SourceProjectID != projectID {
		live, lerr := h.liveReleaseVersions(ctx, body.SourceProjectID)
		if lerr != nil {
			h.log.Error("list releases for adopt", "source", body.SourceProjectID, "err", lerr)
			apierror.Write(w, r, apierror.InternalWith("failed to check the source's releases", lerr))
			return
		}
		switch {
		case len(live) == 0:
			writeError(w, r, http.StatusConflict,
				fmt.Sprintf("%s has published no release yet, so there is nothing to adopt from", body.SourceProjectID))
			return
		case body.SourceVersion == "":
			writeError(w, r, http.StatusBadRequest,
				"source_version is required: an adoption records the release it was taken from")
			return
		case !containsString(live, body.SourceVersion):
			writeError(w, r, http.StatusBadRequest,
				fmt.Sprintf("%s has no live release %s", body.SourceProjectID, body.SourceVersion))
			return
		}
		if ok, cerr := h.entityInRelease(ctx, body.EntityType, body.SourceProjectID, body.SourceEntityID, body.SourceVersion); cerr != nil {
			h.log.Error("check entity in release", "source", body.SourceProjectID, "err", cerr)
			apierror.Write(w, r, apierror.InternalWith("failed to check the source release", cerr))
			return
		} else if !ok {
			writeError(w, r, http.StatusConflict,
				fmt.Sprintf("%s is not part of %s %s", body.SourceEntityID, body.SourceProjectID, body.SourceVersion))
			return
		}
	}

	if body.SourceProjectID == projectID {
		writeError(w, r, http.StatusBadRequest, "cannot adopt own entity")
		return
	}

	var actorID *string
	if snap := weaveauth.FromContext(ctx); snap != nil && snap.ActorID != "" {
		id := snap.ActorID
		actorID = &id
	}

	existing, err := h.weave.Adoptions().List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
	)
	if err != nil {
		h.log.Error("list existing adoptions", "project_id", projectID, "err", err)
		apierror.Write(w, r, apierror.InternalWith("failed to load existing adoptions", err))
		return
	}
	newRow := domain.Adoption{
		ProjectID:         projectID,
		ContextEntityType: "project",
		ContextEntityID:   projectID,
		EntityType:        body.EntityType,
		SourceProjectID:   body.SourceProjectID,
		SourceEntityID:    body.SourceEntityID,
		SourceVersion:     body.SourceVersion,
		CreatedByID:       actorID,
	}
	key := adoptionReceiptKey(newRow)
	for _, a := range existing {
		if adoptionReceiptKey(a) == key {
			writeError(w, r, http.StatusConflict, "already adopted")
			return
		}
	}
	merged := append(existing[:0:0], existing...)
	merged = append(merged, newRow)
	if err := h.weave.Adoptions().ReplaceForContext(ctx, projectID, "project", projectID, merged); err != nil {
		h.log.Error("replace adoptions", "project_id", projectID, "err", err)
		apierror.Write(w, r, apierror.InternalWith(fmt.Sprintf("failed to record adoption: %v", err), err))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"adoption": newRow,
	})
}
