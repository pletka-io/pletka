package project

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

const directFieldsID = "__direct__"

type overrideEditorResponse struct {
	EntityType string                   `json:"entity_type"`
	EntityID   string                   `json:"entity_id"`
	ProjectID  string                   `json:"project_id"`
	Version    int                      `json:"version"`
	Scope      overrideEditorScope      `json:"scope"`
	Categories []overrideEditorCategory `json:"categories"`
	// AvailableCategories is the project's full category roster (used
	// or not). One source of truth for the move-to-category dropdown,
	// the field/collection sidebar selects, and the state's name-cache
	// used by ensureCategory. Saves the frontend a separate fetch.
	AvailableCategories []overrideEditorCategoryRef `json:"available_categories"`
	Available           overrideEditorAvailable     `json:"available"`
	Capabilities        overrideEditorCapabilities  `json:"capabilities"`
	// Fingerprint is the content hash of the entity's pattern as loaded
	// (see override.Service.EntityFingerprint). The editor carries it
	// with its draft and round-trips it on save so a stale save can be
	// refused with a 409 instead of silently overwriting newer work.
	Fingerprint string `json:"fingerprint"`
}

// overrideEditorCategoryRef is the lightweight category descriptor used
// by every dropdown in the editor. Position/items live on
// overrideEditorCategory; this struct only carries identity + label.
type overrideEditorCategoryRef struct {
	ID             string              `json:"id"`
	SemanticID     string              `json:"semantic_id,omitempty"`
	Name           domain.Translations `json:"name,omitempty"`
	Status         string              `json:"status,omitempty"`
	CanonicalOrder int                 `json:"canonical_order,omitempty"`
}

type overrideEditorScope struct {
	Prefix    string `json:"prefix,omitempty"`
	LocalName string `json:"local_name"`
	Display   string `json:"display"`
}

type overrideEditorAvailable struct {
	SearchURL                       string `json:"search_url"`
	PathSuggestionsURL              string `json:"path_suggestions_url"`
	AdoptCollectionURL              string `json:"adopt_collection_url,omitempty"`
	FieldSidebarSchemaURL           string `json:"field_sidebar_schema_url,omitempty"`
	CollectionGroupSidebarSchemaURL string `json:"collection_group_sidebar_schema_url,omitempty"`
}

type overrideEditorCapabilities struct {
	CanAddField      bool `json:"can_add_field"`
	CanAddCollection bool `json:"can_add_collection"`
	CanReorder       bool `json:"can_reorder"`
	CanEditOverrides bool `json:"can_edit_overrides"`
	CanHideFields    bool `json:"can_hide_fields"`
}

type overrideEditorCategory struct {
	CategoryID   string               `json:"category_id"`
	SemanticID   string               `json:"semantic_id,omitempty"`
	CategoryName domain.Translations  `json:"category_name"`
	Position     int                  `json:"position"`
	Items        []overrideEditorItem `json:"items"`
}

type overrideEditorItem struct {
	Widget           string                `json:"widget"`
	ID               string                `json:"id"`
	SemanticID       string                `json:"semantic_id,omitempty"`
	Name             domain.Translations   `json:"name,omitempty"`
	Position         int                   `json:"position"`
	FieldCount       int                   `json:"field_count"`
	SharedPathPrefix []domain.PathElement  `json:"shared_path_prefix,omitempty"`
	// Placement: collection-group constraints; nil =
	// defaults (optional, 0..unbounded, visible). Model editor only.
	Placement *domain.CollectionPlacement `json:"placement,omitempty"`
	Fields    []overrideEditorField       `json:"fields"`
}

type overrideEditorField struct {
	FieldID            string               `json:"field_id"`
	OverrideID         int64                `json:"override_id"`
	Position           int                  `json:"position"`
	DisplayName        domain.Translations  `json:"display_name"`
	Description        domain.Translations  `json:"description,omitempty"`
	OntologyPath       string               `json:"ontology_path,omitempty"`
	PathElements       []domain.PathElement `json:"path_elements,omitempty"`
	CategoryID         string               `json:"category_id,omitempty"`
	PartOfCollectionID string               `json:"part_of_collection_id,omitempty"`
	ExpectedValueType  string               `json:"expected_value_type,omitempty"`
	// ExpectedResourceModels / ExpectedCollectionModels are the editable
	// payload (string IDs) — what save sends back to the backend. Pickers
	// bind these directly.
	ExpectedResourceModels   []string `json:"expected_resource_models,omitempty"`
	ExpectedCollectionModels []string `json:"expected_collection_models,omitempty"`
	ExpectedConceptLists     []string `json:"expected_concept_lists,omitempty"`
	// ExpectedResourceModelRefs / ExpectedCollectionModelRefs carry the
	// resolved {id, semantic_id, name} for display in chips. Frontend
	// reads these to render labels alongside ULIDs; save flow ignores
	// them and consumes only the bare-ID arrays above.
	ExpectedResourceModelRefs   []domain.EntityRef `json:"expected_resource_model_refs,omitempty"`
	ExpectedCollectionModelRefs []domain.EntityRef `json:"expected_collection_model_refs,omitempty"`
	ExpectedConceptListRefs     []domain.EntityRef `json:"expected_concept_list_refs,omitempty"`
	SetValue                    string             `json:"set_value,omitempty"`
	IsRequired                  bool               `json:"is_required"`
	MinOccurs                   int                `json:"min_occurs"`
	MaxOccurs                   *int               `json:"max_occurs,omitempty"`
	IsHidden                    bool               `json:"is_hidden"`
	Visibility                  string             `json:"visibility,omitempty"`
}

func (h *Handler) ModelOverrides(w http.ResponseWriter, r *http.Request) {
	if denyReleaseEditorSurface(w, r) {
		return
	}
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	if projectID == "" || modelID == "" {
		writeError(w, http.StatusBadRequest, "projectID and modelID are required")
		return
	}

	model, err := h.weave.Models().GetByID(ctx, modelID)
	if err != nil || model == nil {
		writeError(w, http.StatusNotFound, "model not found")
		return
	}
	project, err := h.svc.Get(ctx, projectID)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	view, err := h.weave.ModelView(ctx, modelID, projectID)
	if err != nil {
		h.log.Error("build model override editor payload", "project_id", projectID, "model_id", modelID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to build override editor payload")
		return
	}

	catSemIDs, collSemIDs := h.semIDMaps(ctx, projectID)
	availableCats := h.availableCategories(ctx, projectID)

	fingerprint, err := h.overrides.EntityFingerprint(ctx, "model", modelID)
	if err != nil {
		h.log.Error("compute model override fingerprint", "project_id", projectID, "model_id", modelID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to build override editor payload")
		return
	}

	resp := overrideEditorResponse{
		EntityType:          "model",
		EntityID:            modelID,
		ProjectID:           projectID,
		Version:             1,
		Scope:               toEditorScope(model.OntologyScope),
		Categories:          h.modelOverrideCategories(view.Categories, catSemIDs, collSemIDs),
		AvailableCategories: availableCats,
		Available: overrideEditorAvailable{
			SearchURL:                       fmt.Sprintf("/api/v1/projects/%s/search", projectID),
			PathSuggestionsURL:              fmt.Sprintf("/api/v1/projects/%s/path-suggestions", projectID),
			AdoptCollectionURL:              fmt.Sprintf("/projects/%s/models/%s/composition/adopt-collection", projectID, modelID),
			FieldSidebarSchemaURL:           fmt.Sprintf("/projects/%s/composition/sidebar-schema/field", projectID),
			CollectionGroupSidebarSchemaURL: fmt.Sprintf("/projects/%s/composition/sidebar-schema/collection-group", projectID),
		},
		Capabilities: h.overrideCapabilities(ctx, project, model.ProjectID == projectID, true),
		Fingerprint:  fingerprint,
	}

	writeJSON(w, http.StatusOK, resp)
}

// overrideCapabilities assembles the capability flags shared by the model
// and collection override editors. All flags mirror the caller's edit
// permission except CanAddCollection, which is additionally gated by
// supportsAddCollection: a collection can't nest another collection, so
// its editor always passes supportsAddCollection=false and gets
// CanAddCollection=false regardless of edit permission.
//
// CanHideFields previously diverged between the two editors
// (CollectionOverrides hard-coded false), so the collection hide checkbox
// never rendered. Routing both through this one helper keeps them in sync.
//
// Takes the loaded project (not projectID/visibility) so CanEdit can build
// an auth.ProjectResource that carries OrgID and org-inherited roles resolve.
//
// owned is false when the model or collection belongs to another project
// (e.g. an adopted one opened under the adopting project): the editor is
// then read-only, matching saveOverrides, which only accepts the owner.
func (h *Handler) overrideCapabilities(ctx context.Context, p *domain.Project, owned, supportsAddCollection bool) overrideEditorCapabilities {
	canEdit := owned && h.svc.CanEdit(ctx, p)
	canAddCollection := false
	if supportsAddCollection {
		canAddCollection = canEdit
	}
	return overrideEditorCapabilities{
		CanAddField:      canEdit,
		CanAddCollection: canAddCollection,
		CanReorder:       canEdit,
		CanEditOverrides: canEdit,
		CanHideFields:    canEdit,
	}
}

func (h *Handler) CollectionOverrides(w http.ResponseWriter, r *http.Request) {
	if denyReleaseEditorSurface(w, r) {
		return
	}
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	if projectID == "" || collectionID == "" {
		writeError(w, http.StatusBadRequest, "projectID and collectionID are required")
		return
	}

	collection, err := h.weave.Collections().GetByID(ctx, collectionID)
	if err != nil || collection == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	project, err := h.svc.Get(ctx, projectID)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	sourceProjectID := projectID
	if collection.ProjectID != "" {
		sourceProjectID = collection.ProjectID
	}

	fields, err := h.weave.CollectionView(ctx, collectionID, sourceProjectID)
	if err != nil {
		h.log.Error(
			"build collection override editor payload",
			"project_id", projectID,
			"source_project_id", sourceProjectID,
			"collection_id", collectionID,
			"err", err,
		)
		writeError(w, http.StatusInternalServerError, "failed to build override editor payload")
		return
	}

	categories, err := h.weave.WeaveCategories().List(ctx, domain.WithProjectID(sourceProjectID))
	if err != nil {
		h.log.Error(
			"list categories for collection override payload",
			"project_id", projectID,
			"source_project_id", sourceProjectID,
			"err", err,
		)
		writeError(w, http.StatusInternalServerError, "failed to build override editor payload")
		return
	}

	fingerprint, err := h.overrides.EntityFingerprint(ctx, "collection", collectionID)
	if err != nil {
		h.log.Error("compute collection override fingerprint", "project_id", projectID, "collection_id", collectionID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to build override editor payload")
		return
	}

	resp := overrideEditorResponse{
		EntityType:          "collection",
		EntityID:            collectionID,
		ProjectID:           projectID,
		Version:             1,
		Scope:               toEditorScope(collection.OntologyScope),
		Categories:          h.collectionOverrideCategories(ctx, projectID, fields, categories),
		AvailableCategories: refsFromCategories(categories),
		Available: overrideEditorAvailable{
			SearchURL:                       fmt.Sprintf("/api/v1/projects/%s/search", projectID),
			PathSuggestionsURL:              fmt.Sprintf("/api/v1/projects/%s/path-suggestions", projectID),
			FieldSidebarSchemaURL:           fmt.Sprintf("/projects/%s/composition/sidebar-schema/field", projectID),
			CollectionGroupSidebarSchemaURL: fmt.Sprintf("/projects/%s/composition/sidebar-schema/collection-group", projectID),
		},
		Capabilities: h.overrideCapabilities(ctx, project, collection.ProjectID == projectID, false),
		Fingerprint:  fingerprint,
	}

	writeJSON(w, http.StatusOK, resp)
}

func denyReleaseEditorSurface(w http.ResponseWriter, r *http.Request) bool {
	if weaveauth.ProjectVersionFromContext(r.Context()) == "" {
		return false
	}
	writeError(w, http.StatusNotFound, "not found")
	return true
}

func toEditorScope(scope domain.PathElement) overrideEditorScope {
	display := scope.LocalName
	if scope.Prefix != "" {
		display = scope.Prefix + ":" + scope.LocalName
	}
	return overrideEditorScope{
		Prefix:    scope.Prefix,
		LocalName: scope.LocalName,
		Display:   display,
	}
}

func (h *Handler) modelOverrideCategories(
	categories []domain.CategoryGroup,
	catSemIDs map[string]string,
	collSemIDs map[string]string,
) []overrideEditorCategory {
	out := make([]overrideEditorCategory, 0, len(categories))
	for _, cat := range categories {
		items := make([]overrideEditorItem, 0, len(cat.Collections))
		for _, coll := range cat.Collections {
			items = append(items, overrideEditorItem{
				Widget:           collectionWidget(coll.ID),
				ID:               coll.ID,
				SemanticID:       collSemIDs[coll.ID],
				Name:             coll.Name,
				Position:         coll.Position,
				FieldCount:       len(coll.Fields),
				SharedPathPrefix: coll.SharedPathPrefix,
				Placement:        coll.Placement,
				Fields:           convertEditorFields(coll.Fields),
			})
		}
		categoryID := denormalizeEditorCategoryID(cat.ID)
		out = append(out, overrideEditorCategory{
			CategoryID:   categoryID,
			SemanticID:   catSemIDs[cat.ID],
			CategoryName: editorCategoryName(categoryID, cat.Name),
			Position:     cat.Position,
			Items:        items,
		})
	}
	return out
}

// availableCategories returns the project's full category roster as the
// lightweight ref shape consumed by every dropdown in the editor. Falls
// back silently to an empty list on error — the editor still renders
// the in-tree categories, just without the unused ones.
func (h *Handler) availableCategories(ctx context.Context, projectID string) []overrideEditorCategoryRef {
	cats, err := h.weave.WeaveCategories().List(ctx, domain.WithProjectID(projectID))
	if err != nil {
		return nil
	}
	return refsFromCategories(cats)
}

// refsFromCategories converts a domain Category slice to the lightweight
// ref shape. Falls back to system_name when UIName is empty so the
// dropdown label is never blank.
func refsFromCategories(cats []*domain.Category) []overrideEditorCategoryRef {
	maxOrder := 0
	out := make([]overrideEditorCategoryRef, 0, len(cats)+1)
	for _, c := range cats {
		if c == nil {
			continue
		}
		if c.CanonicalOrder > maxOrder {
			maxOrder = c.CanonicalOrder
		}
		name := c.UIName
		if name == nil || name.Get("en", "") == "" {
			if c.SystemName != "" {
				name = domain.Translations{"en": c.SystemName}
			}
		}
		out = append(out, overrideEditorCategoryRef{
			ID:             c.ID,
			SemanticID:     c.SemanticID,
			Name:           name,
			Status:         string(c.Status),
			CanonicalOrder: c.CanonicalOrder,
		})
	}
	out = append(out, overrideEditorCategoryRef{
		ID:             overrideEditorUncategorizedID,
		Name:           domain.Translations{"en": "Uncategorized"},
		CanonicalOrder: maxOrder + 1,
	})
	return out
}

// semIDMaps loads {category_id → semantic_id} and {collection_id → semantic_id}
// for the given project. Failures are non-fatal — the editor falls back to
// rendering raw IDs in the UI rather than blocking the page.
func (h *Handler) semIDMaps(ctx context.Context, projectID string) (map[string]string, map[string]string) {
	catSem := map[string]string{}
	collSem := map[string]string{}
	if cats, err := h.weave.WeaveCategories().List(ctx, domain.WithProjectID(projectID)); err == nil {
		for _, c := range cats {
			if c == nil {
				continue
			}
			catSem[c.ID] = c.SemanticID
		}
	}
	if colls, _, err := h.weave.Collections().List(ctx, domain.WithProjectID(projectID)); err == nil {
		for _, c := range colls {
			if c == nil {
				continue
			}
			collSem[c.ID] = c.SemanticID
		}
	}
	return catSem, collSem
}

func (h *Handler) collectionOverrideCategories(
	ctx context.Context,
	projectID string,
	fields []domain.ResolvedField,
	categories []*domain.Category,
) []overrideEditorCategory {
	type catInfo struct {
		name  domain.Translations
		semID string
		order int
	}
	catMap := make(map[string]catInfo, len(categories))
	for _, cat := range categories {
		if cat == nil {
			continue
		}
		catMap[cat.ID] = catInfo{name: cat.UIName, semID: cat.SemanticID, order: cat.CanonicalOrder}
	}
	_ = ctx
	_ = projectID

	grouped := make(map[string][]domain.ResolvedField)
	for _, f := range fields {
		grouped[f.CategoryID] = append(grouped[f.CategoryID], f)
	}

	out := make([]overrideEditorCategory, 0, len(grouped))
	for catID, catFields := range grouped {
		editorCategoryID := denormalizeEditorCategoryID(catID)
		name := domain.Translations{"en": catID}
		position := 0
		semID := ""
		if info, ok := catMap[catID]; ok {
			name = info.name
			position = info.order
			semID = info.semID
		}
		// direct fields share a category, not an ontology root — never
		// hoist a shared prefix here (follow-up), same fix
		// as resolve.go's buildModelView for the "__direct__" bucket.
		out = append(out, overrideEditorCategory{
			CategoryID:   editorCategoryID,
			SemanticID:   semID,
			CategoryName: editorCategoryName(editorCategoryID, name),
			Position:     position,
			Items: []overrideEditorItem{{
				Widget:     "field-group",
				ID:         directFieldsID,
				Name:       domain.Translations{"en": "Direct Fields"},
				Position:   1,
				FieldCount: len(catFields),
				Fields:     convertEditorFields(catFields),
			}},
		})
	}

	// preserve category ordering
	sortEditorCategories(out)
	return out
}

func convertEditorFields(fields []domain.ResolvedField) []overrideEditorField {
	out := make([]overrideEditorField, 0, len(fields))
	for _, f := range fields {
		out = append(out, overrideEditorField{
			FieldID:                     f.ID,
			OverrideID:                  f.OverrideID,
			Position:                    f.Position,
			DisplayName:                 f.DisplayName,
			Description:                 f.Description,
			OntologyPath:                f.OntologyPath,
			PathElements:                f.PathElements,
			CategoryID:                  denormalizeEditorCategoryID(f.CategoryID),
			PartOfCollectionID:          f.PartOfCollectionID,
			ExpectedValueType:           f.ExpectedValueType,
			ExpectedResourceModels:      entityRefIDs(f.ResourceModels),
			ExpectedCollectionModels:    entityRefIDs(f.CollectionModels),
			ExpectedConceptLists:        entityRefIDs(f.ConceptLists),
			ExpectedResourceModelRefs:   f.ResourceModels,
			ExpectedCollectionModelRefs: f.CollectionModels,
			ExpectedConceptListRefs:     f.ConceptLists,
			SetValue:                    f.SetValue,
			IsRequired:                  f.IsRequired,
			MinOccurs:                   f.MinOccurs,
			MaxOccurs:                   f.MaxOccurs,
			IsHidden:                    f.IsHidden,
			Visibility:                  f.Visibility,
		})
	}
	return out
}

func entityRefIDs(refs []domain.EntityRef) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.ID)
	}
	return out
}

func collectionWidget(collectionID string) string {
	if collectionID == directFieldsID || collectionID == "" {
		return "field-group"
	}
	return "collection-group"
}

func sortEditorCategories(categories []overrideEditorCategory) {
	for i := 0; i < len(categories); i++ {
		for j := i + 1; j < len(categories); j++ {
			if categories[j].Position < categories[i].Position {
				categories[i], categories[j] = categories[j], categories[i]
			}
		}
	}
}

func editorCategoryName(categoryID string, name domain.Translations) domain.Translations {
	if categoryID == overrideEditorUncategorizedID {
		if name == nil || name.Get("en", "") == "" {
			return domain.Translations{"en": "Uncategorized"}
		}
	}
	return name
}
