package field

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/publication"
)

// LangResolver returns the caller's preferred UI language for a request.
type LangResolver func(r *http.Request) string

// Handler exposes the field slice's read endpoints.
type Handler struct {
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
	// i18n walks LocalizedText nodes inside the row payload (origin_label)
	// — schema-driven UI rule (.claude/rules/ui-patterns.md).
	i18n i18n.Manager
	// pub derives per-row publication state (draft/published/modified/new).
	// Optional; nil leaves the field unset.
	pub *publication.Reader
}

// NewHandler constructs a Handler. nil log → slog.Default; nil lang → "en".
func NewHandler(svc *Service, log *slog.Logger, languages []formschema.LanguageInfo, lang LangResolver, i18nMgr i18n.Manager) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, log: log, languages: languages, lang: lang, i18n: i18nMgr}
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

// fieldListItem mirrors the legacy handler's flattened JSON shape so the
// migration is JSON-shape-compatible for frontend consumers.
type fieldListItem struct {
	ID                string               `json:"id"`
	SemanticID        string               `json:"semantic_id,omitempty"`
	SystemName        string               `json:"system_name,omitempty"`
	UIName            domain.Translations  `json:"ui_name,omitempty"`
	Description       domain.Translations  `json:"description,omitempty"`
	Status            string               `json:"status"`
	ProjectID         string               `json:"project_id"`
	Deprecated        bool                 `json:"deprecated"`
	Owned             bool                 `json:"owned"`
	CanDeprecate      bool                 `json:"can_deprecate"`
	CanActivate       bool                 `json:"can_activate"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
	OntologyScope     string               `json:"ontology_scope,omitempty"`
	ScopeClassCode    string               `json:"scope_class_code,omitempty"`
	ExpectedValueType string               `json:"expected_value_type,omitempty"`
	PathElements      []domain.PathElement `json:"path_elements,omitempty"`
	CategoryID        string               `json:"category_id,omitempty"`
	// Usage counts (batch-computed for the page): models/collections using this
	// field in this project, the number of OTHER projects that use it, and an
	// in-use flag that gates the delete action.
	ModelCount        int           `json:"model_count,omitempty"`
	CollectionCount   int           `json:"collection_count,omitempty"`
	OtherProjectCount int           `json:"other_project_count,omitempty"`
	InUse             bool          `json:"in_use,omitempty"`
	Origin            domain.Origin `json:"origin"`
	OriginKind        string        `json:"origin_kind"`
	// OriginLabel is a LocalizedText so EntityListBadge.svelte renders
	// via tr() with no domain wording in the frontend.
	OriginLabel i18n.LocalizedText `json:"origin_label,omitempty"`
}

func flattenField(f *domain.Field, projectID string) fieldListItem {
	scope, scopeCode := "", ""
	if f.OntologyScope.LocalName != "" {
		scope = f.OntologyScope.PrefixedName()
		scopeCode = f.OntologyScope.ShortCode()
	}
	owned := f.ProjectID == projectID
	return fieldListItem{
		ID:                f.ID,
		SemanticID:        f.SemanticID,
		SystemName:        f.SystemName,
		UIName:            f.UIName,
		Description:       f.Description,
		Status:            string(f.Status),
		ProjectID:         f.ProjectID,
		Deprecated:        f.Deprecated,
		Owned:             owned,
		CanDeprecate:      owned && !f.Deprecated,
		CanActivate:       owned && f.Deprecated,
		CreatedAt:         f.CreatedAt,
		UpdatedAt:         f.UpdatedAt,
		OntologyScope:     scope,
		ScopeClassCode:    scopeCode,
		ExpectedValueType: f.ExpectedValueType,
		PathElements:      f.PathElements,
	}
}

// List handles GET /projects/{projectID}/fields. Paginated + searchable
// list rendered as fieldListItem JSON.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
		return
	}

	search := r.URL.Query().Get("search")
	scopeFilter := parseScopeFilter(r.URL.Query().Get("ontology_scope"))
	categoryFilter := parseScopeFilter(r.URL.Query().Get("category_id"))
	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "ui_name"
	}
	sortDir := r.URL.Query().Get("sort_dir")

	page := 1
	perPage := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && v > 0 {
		perPage = v
		if perPage > 1000 {
			perPage = 1000
		}
	}

	opts := []domain.QueryOption{
		domain.WithOrderBy(sortBy, sortDir == "desc"),
		domain.WithSearchColumns("ui_name", "description"),
	}
	if search != "" {
		opts = append(opts, domain.WithSearch(search))
	}
	// Scope + category filters are post-fetch (matches the model +
	// collection pattern). When a filter is active we pull a wide
	// window from SQL and slice in-memory so totals reflect the
	// filtered set; without filter we paginate at SQL level for cheap
	// row counts.
	hasPostFilter := len(scopeFilter) > 0 || len(categoryFilter) > 0
	if hasPostFilter {
		opts = append(opts, domain.WithLimit(10000))
	} else {
		opts = append(opts, domain.WithLimit(perPage), domain.WithOffset((page-1)*perPage))
	}

	fields, total, err := h.svc.List(r.Context(), projectID, opts...)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	// Resolve base-override category assignments once so we can both
	// post-filter and surface the category on each row.
	var categoryByField map[string]string
	if hasPostFilter || true {
		assignments, aErr := h.svc.ListBaseFieldCategoryAssignments(r.Context(), projectID)
		if aErr != nil {
			h.log.Warn("load field category assignments", "project", projectID, "err", aErr)
		} else {
			categoryByField = assignments
		}
	}

	if hasPostFilter {
		filtered := fields[:0]
		for _, f := range fields {
			if len(scopeFilter) > 0 && !scopeFilter[f.OntologyScope.PrefixedName()] {
				continue
			}
			if len(categoryFilter) > 0 {
				cat := categoryByField[f.ID]
				if cat == "" || !categoryFilter[cat] {
					continue
				}
			}
			filtered = append(filtered, f)
		}
		fields = filtered
		total = int64(len(fields))
		// Page-slice the filtered set.
		start := (page - 1) * perPage
		if start > len(fields) {
			start = len(fields)
		}
		end := start + perPage
		if end > len(fields) {
			end = len(fields)
		}
		fields = fields[start:end]
	}

	// Receipt-adopted fields — task 3a. List fetched in full (no
	// pagination) since adoptions are typically a small set; merged into
	// the items slice below and total adjusted.
	adoptedFields, adoptedOrigins, aErr := h.svc.ListAdoptedExplicit(r.Context(), projectID)
	if aErr != nil {
		h.writeServiceError(w, aErr)
		return
	}
	// Reference-adopted fields — task 3b. Fields living in another
	// project that this project's overrides reference via field_id.
	referenceAdoptedFields, refErr := h.svc.ListAdoptedByReference(r.Context(), projectID)
	if refErr != nil {
		h.writeServiceError(w, refErr)
		return
	}
	referencedIDs := make(map[string]bool, len(referenceAdoptedFields))
	for _, f := range referenceAdoptedFields {
		referencedIDs[f.ID] = true
	}
	originFilter := parseOriginKinds(r.URL.Query().Get("origin_kind"))

	items := make([]fieldListItem, 0, len(fields)+len(adoptedFields))
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		row := flattenField(f, projectID)
		if categoryByField != nil {
			row.CategoryID = categoryByField[f.ID]
		}
		row.Origin = domain.OwnOrigin()
		row.OriginKind = string(row.Origin.Kind)
		row.OriginLabel = originListLabel(row.Origin, false)
		if len(originFilter) > 0 && !originFilter[row.Origin.Kind] {
			continue
		}
		seen[f.ID] = true
		items = append(items, row)
	}
	// Append receipt-adopted, dedup on entity ID (local wins per Q4).
	// Receipt rows whose ID is not in the reference set get the
	// "not yet referenced" Q10 note.
	for _, f := range adoptedFields {
		if seen[f.ID] {
			continue
		}
		origin := adoptedOrigins[f.ID]
		if origin.Kind == "" {
			origin = domain.AdoptedOrigin(f.ProjectID, f.ID)
		}
		if len(originFilter) > 0 && !originFilter[origin.Kind] {
			continue
		}
		row := flattenField(f, projectID)
		if categoryByField != nil {
			row.CategoryID = categoryByField[f.ID]
		}
		row.Origin = origin
		row.OriginKind = string(origin.Kind)
		row.OriginLabel = originListLabel(origin, !referencedIDs[f.ID])
		seen[f.ID] = true
		items = append(items, row)
		total++
	}
	// Append reference-adopted, dedup against own + receipt-adopted —
	// explicit-dominates per Q4.
	for _, f := range referenceAdoptedFields {
		if seen[f.ID] {
			continue
		}
		origin := domain.Origin{
			Kind:            domain.OriginAdoptedReference,
			SourceProjectID: f.ProjectID,
			SourceEntityID:  f.ID,
		}
		if len(originFilter) > 0 && !originFilter[origin.Kind] {
			continue
		}
		row := flattenField(f, projectID)
		if categoryByField != nil {
			row.CategoryID = categoryByField[f.ID]
		}
		row.Origin = origin
		row.OriginKind = string(origin.Kind)
		row.OriginLabel = originListLabel(origin, false)
		items = append(items, row)
		total++
	}

	// Attach per-row usage counts in one batch query (no N+1). Best-effort:
	// a failure logs and leaves the counts zero rather than failing the list.
	if len(items) > 0 {
		ids := make([]string, 0, len(items))
		for i := range items {
			ids = append(ids, items[i].ID)
		}
		if counts, cErr := h.svc.BatchUsageCounts(r.Context(), projectID, ids); cErr != nil {
			h.log.Warn("field usage counts", "project", projectID, "err", cErr)
		} else {
			for i := range items {
				if c, ok := counts[items[i].ID]; ok {
					items[i].ModelCount = c.ModelCount
					items[i].CollectionCount = c.CollectionCount
					items[i].OtherProjectCount = c.OtherProjectCount
					items[i].InUse = c.InUse
				}
			}
		}
		publication.Attach(r.Context(), h.pub, h.log, projectID, "field", ids, func(i int) *string { return &items[i].Status })
	}

	// Walk-resolve LocalizedText nodes (origin_label) per current lang.
	if h.i18n != nil {
		h.i18n.Resolve(&items, h.lang(r))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"fields":   items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// parseOriginKinds accepts the four-state list filter values (Q5):
// own, forked, adopted, adopted_reference.
func parseOriginKinds(raw string) map[domain.OriginKind]bool {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := map[domain.OriginKind]bool{}
	for _, part := range strings.Split(raw, ",") {
		switch kind := domain.OriginKind(strings.TrimSpace(part)); kind {
		case domain.OriginOwn, domain.OriginForked, domain.OriginAdopted, domain.OriginAdoptedReference:
			out[kind] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// originListLabel returns the row-badge label as a LocalizedText —
// mirror of model + collection list labels. notReferenced applies only
// to OriginAdopted (Q10 'not yet referenced').
func originListLabel(origin domain.Origin, notReferenced bool) i18n.LocalizedText {
	source := origin.SourceProjectID
	if origin.SourceEntityID != "" {
		if source != "" {
			source += " · "
		}
		source += origin.SourceEntityID
	}
	switch origin.Kind {
	case domain.OriginForked:
		if source == "" {
			return i18n.L("common.origin_state.adapted", "Adapted")
		}
		return i18n.LF("common.origin_state.adapted_from", "Adapted from {source}", map[string]string{"source": source})
	case domain.OriginAdopted:
		if notReferenced {
			if source == "" {
				return i18n.L("common.origin_state.adopted_unused", "Adopted · not yet referenced")
			}
			return i18n.LF("common.origin_state.adopted_unused_from", "Adopted from {source} · not yet referenced", map[string]string{"source": source})
		}
		if source == "" {
			return i18n.L("common.origin_state.adopted", "Adopted")
		}
		return i18n.LF("common.origin_state.adopted_from", "Adopted from {source}", map[string]string{"source": source})
	case domain.OriginAdoptedReference:
		if source == "" {
			return i18n.L("common.origin_state.adopted_reference", "Adopted (by reference)")
		}
		return i18n.LF("common.origin_state.adopted_reference_from", "Adopted by reference from {source}", map[string]string{"source": source})
	case domain.OriginInherited:
		if source == "" {
			return i18n.L("common.origin_state.inherited", "Inherited")
		}
		return i18n.LF("common.origin_state.inherited_from", "Inherited from {source}", map[string]string{"source": source})
	case domain.OriginOwn:
		return i18n.L("common.origin_state.own", "Own")
	default:
		return i18n.LocalizedText{}
	}
}

// parseScopeFilter splits a comma-joined ontology_scope query param
// into a set keyed by prefixed name (e.g. "crm:E21_Person"). Empty
// returns nil so the post-fetch filter short-circuits.
func parseScopeFilter(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out[p] = true
		}
	}
	return out
}

// OntologyScopesFilter handles GET /projects/{projectID}/fields/filters/ontology-scopes.
// Returns the distinct ontology scopes present on this project's
// fields, in the FilterOption shape the entity-list schema expects.
func (h *Handler) OntologyScopesFilter(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		apierror.Write(w, apierror.BadRequest("project ID is required"))
		return
	}
	fields, _, err := h.svc.List(r.Context(), projectID, domain.WithLimit(10000))
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	seen := map[string]struct{}{}
	scopes := make([]string, 0)
	for _, f := range fields {
		s := f.OntologyScope.PrefixedName()
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		scopes = append(scopes, s)
	}
	options := make([]map[string]any, 0, len(scopes))
	for _, s := range scopes {
		options = append(options, map[string]any{
			"value": s,
			"label": map[string]string{"en": s},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options})
}

// CategoriesFilter handles GET /projects/{projectID}/fields/filters/categories.
// Returns the distinct categories assigned to base field overrides in
// the project, in the {"options":[...]} envelope.
func (h *Handler) CategoriesFilter(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		apierror.Write(w, apierror.BadRequest("project ID is required"))
		return
	}
	cats, err := h.svc.ListBaseFieldCategories(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	options := make([]map[string]any, 0, len(cats))
	for _, c := range cats {
		options = append(options, map[string]any{
			"value": c.ID,
			"label": c.UIName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options})
}

// Detail handles GET /projects/{projectID}/fields/api/{fieldID}.
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if projectID == "" || fieldID == "" {
		http.Error(w, "Project ID and Field ID are required", http.StatusBadRequest)
		return
	}

	field, err := h.svc.Get(r.Context(), projectID, fieldID)
	if err != nil {
		if IsNotFound(err) {
			http.Error(w, "Field not found", http.StatusNotFound)
			return
		}
		h.writeServiceError(w, err)
		return
	}
	if field == nil {
		http.Error(w, "Field not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, field)
}

// ModelRefs handles GET /projects/{projectID}/fields/{fieldID}/models.
// Returns models that reference this field via weave_field_overrides.
func (h *Handler) ModelRefs(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if fieldID == "" {
		http.Error(w, "Field ID is required", http.StatusBadRequest)
		return
	}

	refs, err := h.svc.GetModels(r.Context(), projectID, fieldID)
	if err != nil {
		h.log.Error("get field models", "field_id", fieldID, "err", err)
		// Match legacy behaviour: degrade to empty slice on error.
		refs = []domain.FieldModelRef{}
	}

	writeJSON(w, http.StatusOK, refs)
}

// CollectionRefs handles GET /projects/{projectID}/fields/{fieldID}/collections.
func (h *Handler) CollectionRefs(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if fieldID == "" {
		http.Error(w, "Field ID is required", http.StatusBadRequest)
		return
	}

	refs, err := h.svc.GetCollections(r.Context(), projectID, fieldID)
	if err != nil {
		h.log.Error("get field collections", "field_id", fieldID, "err", err)
		refs = []domain.FieldCollectionRef{}
	}

	writeJSON(w, http.StatusOK, refs)
}

// Stats handles GET /projects/{projectID}/fields/{fieldID}/stats.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	report, err := h.svc.Stats(r.Context(), projectID, fieldID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// ---------------------------------------------------------------------------
// Write endpoints
// ---------------------------------------------------------------------------

// fieldWriteBody is the shared JSON body for Create + Update. PUT
// treats every populated field as a partial change; POST treats them
// as full create state.
type fieldWriteBody struct {
	UIName            domain.Translations  `json:"ui_name"`
	Description       domain.Translations  `json:"description,omitempty"`
	SystemName        string               `json:"system_name,omitempty"`
	SemanticID        string               `json:"semantic_id,omitempty"`
	Status            string               `json:"status,omitempty"`
	OntologyScope     domain.PathElement   `json:"ontology_scope"`
	OntologyPath      []domain.PathElement `json:"ontology_path"`
	ExpectedValueType string               `json:"expected_value_type,omitempty"`

	// Override-shaped — written into the base override row.
	CategoryID               *string   `json:"category_id,omitempty"`
	SetValue                 *string   `json:"set_value,omitempty"`
	ExpectedResourceModels   *[]string `json:"expected_resource_models,omitempty"`
	ExpectedCollectionModels *[]string `json:"expected_collection_models,omitempty"`
}

// Create handles POST /projects/{projectID}/fields.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var body fieldWriteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	in := CreateInput{
		UIName:                body.UIName,
		Description:           body.Description,
		SystemName:            body.SystemName,
		SemanticID:            body.SemanticID,
		Status:                domain.Status(body.Status),
		OntologyScope:         body.OntologyScope,
		OntologyPath:          body.OntologyPath,
		ExpectedValueType:     body.ExpectedValueType,
		CategoryID:            derefString(body.CategoryID),
		SetValue:              derefString(body.SetValue),
		ExpectedModelIDs:      derefStringSlice(body.ExpectedResourceModels),
		ExpectedCollectionIDs: derefStringSlice(body.ExpectedCollectionModels),
	}

	field, err := h.svc.Create(r.Context(), projectID, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, field)
}

// Update handles PUT /projects/{projectID}/fields/{fieldID}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")

	var body fieldWriteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	in := UpdateInput{}
	if len(body.UIName) > 0 {
		ui := body.UIName
		in.UIName = &ui
	}
	if len(body.Description) > 0 {
		desc := body.Description
		in.Description = &desc
	}
	if body.SystemName != "" {
		s := body.SystemName
		in.SystemName = &s
	}
	if body.Status != "" {
		st := domain.Status(body.Status)
		in.Status = &st
	}
	if body.OntologyScope.LocalName != "" {
		sc := body.OntologyScope
		in.OntologyScope = &sc
	}
	if len(body.OntologyPath) > 0 {
		in.OntologyPath = body.OntologyPath
	}
	if body.ExpectedValueType != "" {
		v := body.ExpectedValueType
		in.ExpectedValueType = &v
	}
	if body.CategoryID != nil {
		c := *body.CategoryID
		in.CategoryID = &c
	}
	if body.SetValue != nil {
		v := *body.SetValue
		in.SetValue = &v
	}
	if body.ExpectedResourceModels != nil {
		v := append([]string(nil), (*body.ExpectedResourceModels)...)
		in.ExpectedModelIDs = &v
	}
	if body.ExpectedCollectionModels != nil {
		v := append([]string(nil), (*body.ExpectedCollectionModels)...)
		in.ExpectedCollectionIDs = &v
	}

	field, err := h.svc.Update(r.Context(), projectID, fieldID, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, field)
}

// Delete handles DELETE /projects/{projectID}/fields/{fieldID}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if err := h.svc.Delete(r.Context(), projectID, fieldID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Deprecate handles POST /projects/{projectID}/fields/{fieldID}/deprecate.
func (h *Handler) Deprecate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if err := h.svc.Deprecate(r.Context(), projectID, fieldID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Activate handles POST /projects/{projectID}/fields/{fieldID}/activate.
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	fieldID := chi.URLParam(r, "fieldID")
	if err := h.svc.Activate(r.Context(), projectID, fieldID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefStringSlice(v *[]string) []string {
	if v == nil {
		return nil
	}
	return append([]string(nil), (*v)...)
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	if IsNotFound(err) {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	// In-use conflict carries slice-specific counts the frontend reads to steer
	// the user to Deprecate — mirrors the model + collection handlers. Without
	// this branch it fell through to a generic 500 and the UI showed nothing
	// useful ("delete did nothing").
	var inUseErr *ErrEntityInUse
	if errors.As(err, &inUseErr) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":             string(apierror.CodeInUse),
			"error":            "field_in_use",
			"model_count":      inUseErr.Usage.ModelCount,
			"collection_count": inUseErr.Usage.CollectionCount,
			"message":          "This field is used by other models or collections. Deprecate it instead, or repoint those references first.",
		})
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("field handler error", "err", err)
	}
	apierror.Write(w, ae)
}

// ---------------------------------------------------------------------------
// Local HTTP helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError forwards to apierror.Write — see pkg/weave/apierror.
func writeError(w http.ResponseWriter, status int, msg string) {
	apierror.Write(w, &apierror.Error{Status: status, Message: msg})
}
