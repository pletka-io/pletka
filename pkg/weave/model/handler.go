package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	"github.com/pletka-io/pletka/pkg/weave/publication"
)

// LangResolver returns the caller's preferred UI language for a request.
type LangResolver func(r *http.Request) string

type Handler struct {
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
	// i18n walks LocalizedText nodes inside list-row payloads (origin_label
	// is the first such node) so the wire shape ends up as a Translations
	// map the frontend can pick from with tr() — schema-driven UI rule
	// (see .claude/rules/ui-patterns.md).
	i18n i18n.Manager
	// pub derives per-row publication state. Optional; nil leaves it unset.
	pub *publication.Reader
}

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
// Reads
// ---------------------------------------------------------------------------

// List handles GET /projects/{projectID}/models. Paginated + searchable.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "project ID is required")
		return
	}

	search := r.URL.Query().Get("search")
	originFilter := parseOriginKinds(r.URL.Query().Get("origin_kind"))
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
		domain.WithLimit(10000),
	}
	if search != "" {
		opts = append(opts, domain.WithSearch(search))
	}
	if mt := strings.TrimSpace(r.URL.Query().Get("model_type")); mt != "" {
		opts = append(opts, domain.WithFilter("model_type", mt))
	}
	if sc := strings.TrimSpace(r.URL.Query().Get("scope_class")); sc != "" {
		opts = append(opts, domain.WithFilter("scope_class", sc))
	}

	models, _, err := h.svc.List(r.Context(), projectID, opts...)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	forkedOrigins, err := h.svc.ForkOriginsForProject(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	// Receipt-adopted models — UNION with local rows so the list shows
	// every model in the project's working set, not just the locally
	// authored ones. Per Q4 the four-state origin discriminates them
	// from Owned + Adapted + Inherited.
	adoptedModels, adoptedOrigins, err := h.svc.ListAdoptedExplicit(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	// Reference-adopted models — task 3b. Any cross-project model
	// referenced by an override in this project that isn't already
	// surfaced as Own/Adapted/Receipt-adopted.
	referenceAdoptedModels, err := h.svc.ListAdoptedByReference(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	// Intersect receipt set against reference set so we can flag
	// receipt-only-not-referenced rows ("not yet referenced" note —
	// Q10 of the adopt/adapt rollout). A model in the receipt set
	// that's ALSO in the reference set is in active use; one missing
	// from the reference set is intent-only.
	referencedIDs := make(map[string]bool, len(referenceAdoptedModels))
	for _, m := range referenceAdoptedModels {
		referencedIDs[m.ID] = true
	}

	type item struct {
		ID                string              `json:"id"`
		SemanticID        string              `json:"semantic_id,omitempty"`
		SystemName        string              `json:"system_name,omitempty"`
		UIName            domain.Translations `json:"ui_name,omitempty"`
		Description       domain.Translations `json:"description,omitempty"`
		Status            string              `json:"status"`
		ProjectID         string              `json:"project_id"`
		OriginalProjectID string              `json:"original_project_id,omitempty"`
		Deprecated        bool                `json:"deprecated"`
		Owned             bool                `json:"owned"`
		CanDeprecate      bool                `json:"can_deprecate"`
		CanActivate       bool                `json:"can_activate"`
		ModelType         string              `json:"model_type"`
		OntologyScope     string              `json:"ontology_scope,omitempty"`
		ScopeClassCode    string              `json:"scope_class_code,omitempty"`
		// Composition counts (batch): fields/categories/collections the model
		// is composed of. Restores the declared-but-blank list badges.
		FieldCount      int           `json:"field_count,omitempty"`
		CategoryCount   int           `json:"category_count,omitempty"`
		CollectionCount int           `json:"collection_count,omitempty"`
		InUse           bool          `json:"in_use,omitempty"`
		Origin          domain.Origin `json:"origin"`
		OriginKind      string        `json:"origin_kind"`
		// OriginLabel is a LocalizedText so the schema-driven badge in
		// EntityListBadge.svelte can render via tr() without baking any
		// "Adapted"/"Forked" wording into the frontend. Resolved by the
		// h.i18n.Resolve walk before encode (see below).
		OriginLabel i18n.LocalizedText `json:"origin_label,omitempty"`
	}
	items := make([]item, 0, len(models)+len(adoptedModels))
	rowFor := func(m *domain.Model, origin domain.Origin, notReferenced bool) item {
		row := item{
			ID:             m.ID,
			SemanticID:     m.SemanticID,
			SystemName:     m.SystemName,
			UIName:         m.UIName,
			Description:    m.Description,
			Status:         string(m.Status),
			ProjectID:      m.ProjectID,
			Deprecated:     m.Deprecated,
			ModelType:      m.ModelType,
			OntologyScope:  m.OntologyScope.PrefixedName(),
			ScopeClassCode: m.OntologyScope.ShortCode(),
			Origin:         origin,
			OriginKind:     string(origin.Kind),
			OriginLabel:    originListLabel(origin, notReferenced),
		}
		row.Owned, row.CanDeprecate, row.CanActivate = ownedActionFlags(m, projectID)
		if origin.Kind == domain.OriginForked {
			row.OriginalProjectID = origin.SourceProjectID
		}
		return row
	}
	seenModelIDs := make(map[string]bool, len(models))
	for _, m := range models {
		origin := domain.OwnOrigin()
		if forked, ok := forkedOrigins[m.ID]; ok {
			origin = forked
		}
		if len(originFilter) > 0 && !originFilter[origin.Kind] {
			continue
		}
		seenModelIDs[m.ID] = true
		items = append(items, rowFor(m, origin, false))
	}
	// Append receipt-adopted models. Skip any whose ID already showed
	// up in the local list — a model can be local AND have a receipt
	// row from a previous adoption flow; explicit-dominates resolution
	// keeps the local copy as the canonical entry.
	for _, m := range adoptedModels {
		if seenModelIDs[m.ID] {
			continue
		}
		origin := adoptedOrigins[m.ID]
		if origin.Kind == "" {
			origin = domain.AdoptedOrigin(m.ProjectID, m.ID)
		}
		if len(originFilter) > 0 && !originFilter[origin.Kind] {
			continue
		}
		seenModelIDs[m.ID] = true
		// Receipt-only-not-referenced rows get the 'not yet referenced'
		// label (Q10). A receipt that's also in the reference set is in
		// active use — render the standard 'Adopted from X' label.
		items = append(items, rowFor(m, origin, !referencedIDs[m.ID]))
	}
	// Append reference-adopted models. Explicit-dominates per Q4: if a
	// row is already surfaced as Own/Adapted (local) or Adopted explicit
	// (receipt) it stays in its stronger state; only previously-unseen
	// IDs get the adopted_reference label.
	for _, m := range referenceAdoptedModels {
		if seenModelIDs[m.ID] {
			continue
		}
		origin := domain.Origin{
			Kind:            domain.OriginAdoptedReference,
			SourceProjectID: m.ProjectID,
			SourceEntityID:  m.ID,
		}
		if len(originFilter) > 0 && !originFilter[origin.Kind] {
			continue
		}
		items = append(items, rowFor(m, origin, false))
	}
	total := int64(len(items))
	items = paginateItems(items, page, perPage)

	// Attach composition counts for the page in one batch query (no N+1).
	if len(items) > 0 {
		ids := make([]string, 0, len(items))
		for i := range items {
			ids = append(ids, items[i].ID)
		}
		if counts, cErr := h.svc.BatchCompositionCounts(r.Context(), projectID, ids); cErr != nil {
			h.log.Warn("model composition counts", "project", projectID, "err", cErr)
		} else {
			for i := range items {
				if c, ok := counts[items[i].ID]; ok {
					items[i].FieldCount = c.FieldCount
					items[i].CategoryCount = c.CategoryCount
					items[i].CollectionCount = c.CollectionCount
				}
			}
		}
		// Referenced-by in-use flag — gates the list delete row-action, matching
		// the detail-page kebab (weave_override_refs value-target usage).
		if inUse, uErr := h.svc.BatchInUse(r.Context(), projectID, ids); uErr != nil {
			h.log.Warn("model in-use counts", "project", projectID, "err", uErr)
		} else {
			for i := range items {
				if inUse[items[i].ID] {
					items[i].InUse = true
				}
			}
		}
		publication.Attach(r.Context(), h.pub, h.log, projectID, "model", ids, func(i int) *string { return &items[i].Status })
	}

	// Resolve every LocalizedText inside the row payload (origin_label is
	// the only one today; mixed-mode tolerates any future additions). The
	// walker mutates Translations maps in place, so the encoded JSON ships
	// the same {lang: value} shape the frontend already reads via tr().
	if h.i18n != nil {
		h.i18n.Resolve(&items, h.lang(r))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"models":   items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// parseOriginKinds accepts the four-state list filter values (Q5):
// own, forked, adopted, adopted_reference. Returns nil when the param
// is missing — caller treats nil as "no filter, show everything".
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

// ownedActionFlags reports which owner-only row actions apply to a model
// from the viewing project's perspective. A project may mutate only models
// it owns (model.ProjectID == projectID) — adopted/reference rows are
// read-only, so their edit/delete/deprecate/activate actions are hidden.
func ownedActionFlags(m *domain.Model, projectID string) (owned, canDeprecate, canActivate bool) {
	owned = m.ProjectID == projectID
	return owned, owned && !m.Deprecated, owned && m.Deprecated
}

// originListLabel returns the row-badge label as a LocalizedText so
// every locale can resolve via bundle lookup (i18n keys live under
// "common.origin.*"). Empty Translations on the LocalizedText means
// "render nothing" — the badge widget hides when there's no text.
//
// Origin kinds:
//   - own → no badge
//   - adopted → "Adopted from <source>" (Q4: explicit/by-reference is a
//     follow-up surface on top of the same key family)
//   - forked → "Adapted from <source>" (the Q7 verb rename lands here
//     in i18n values; the OriginKind wire value stays "forked" until
//     a deliberate breaking-change pass)
//   - inherited → "Inherited from <source>"
//
// originListLabel composes the row-badge label. notReferenced is only
// meaningful for OriginAdopted (Q10 "not yet referenced" surfaces when
// a receipt exists but no override in the current project points at
// the entity yet) — every other state ignores it.
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

func paginateItems[T any](items []T, page, perPage int) []T {
	if perPage <= 0 {
		return items
	}
	start := (page - 1) * perPage
	if start < 0 || start >= len(items) {
		return []T{}
	}
	end := start + perPage
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	m, err := h.svc.Get(r.Context(), projectID, modelID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	if m == nil {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "Model not found")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	report, err := h.svc.Stats(r.Context(), projectID, modelID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) Adopt(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	if err := h.svc.AdoptSource(r.Context(), projectID, modelID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handler) Fork(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	if projectID == "" || modelID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Project ID and Model ID are required")
		return
	}
	model, err := h.svc.ForkFromSource(r.Context(), projectID, modelID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":  model.ID,
		"url": "/projects/" + projectID + "/models/" + model.ID,
	})
}

// ScopeClassesOptions returns the distinct ontology scope classes used
// by models in the project, in the {"options":[...]} envelope expected
// by the entity-list lazy-options loader. Members + readers see the
// same set.
func (h *Handler) ScopeClassesOptions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	classes, err := h.svc.ScopeClasses(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	options := make([]map[string]any, 0, len(classes))
	for _, c := range classes {
		options = append(options, map[string]any{
			"value": c,
			"label": map[string]string{"en": c},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options})
}

// ---------------------------------------------------------------------------
// Override editing
// ---------------------------------------------------------------------------

// ListOverrides handles GET /projects/{projectID}/models/{modelID}/overrides.
// Returns the flat list of model-context override rows. Frontend
// groups into the nested category→items shape per the override
// editor design doc.
func (h *Handler) ListOverrides(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	rows, err := h.svc.ListOverrides(r.Context(), projectID, modelID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entity_type": "model",
		"entity_id":   modelID,
		"project_id":  projectID,
		"overrides":   rows,
	})
}

// SaveOverrides handles PUT /projects/{projectID}/models/{modelID}/overrides.
// Accepts a flat list of override rows + commit_message; delegates to
// override.Service.SaveForEntity which computes a diff against the
// pre-replace state, atomically replaces, and emits per-row changelog
// entries.
func (h *Handler) SaveOverrides(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")

	var body struct {
		CommitMessage string                 `json:"commit_message"`
		Overrides     []domain.FieldOverride `json:"overrides"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	saved, diff, err := h.svc.SaveOverrides(r.Context(), projectID, modelID, body.Overrides, body.CommitMessage)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entity_type": "model",
		"entity_id":   modelID,
		"project_id":  projectID,
		"overrides":   saved,
		"diff": map[string]any{
			"added":   len(diff.Added),
			"removed": len(diff.Removed),
			"changed": len(diff.Changed),
		},
	})
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

type modelWriteBody struct {
	UIName        domain.Translations `json:"ui_name"`
	Description   domain.Translations `json:"description,omitempty"`
	SystemName    string              `json:"system_name,omitempty"`
	Status        string              `json:"status,omitempty"`
	OntologyScope any                 `json:"ontology_scope,omitempty"`
	ModelType     *string             `json:"model_type,omitempty"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var body modelWriteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	scope, scopeErr := parseOntologyScopeInput(body.OntologyScope)
	if scopeErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "validation error",
			"errors":  map[string][]string{"ontology_scope": {scopeErr.Error()}},
			"message": "Please correct the highlighted fields and try again.",
		})
		return
	}
	in := CreateInput{
		UIName:        body.UIName,
		Description:   body.Description,
		SystemName:    body.SystemName,
		OntologyScope: scope,
	}
	if body.ModelType != nil {
		in.ModelType = *body.ModelType
	}
	m, err := h.svc.Create(r.Context(), projectID, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	var body modelWriteBody
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
	if body.OntologyScope != nil {
		scope, scopeErr := parseOntologyScopeInput(body.OntologyScope)
		if scopeErr != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":   "validation error",
				"errors":  map[string][]string{"ontology_scope": {scopeErr.Error()}},
				"message": "Please correct the highlighted fields and try again.",
			})
			return
		}
		in.OntologyScope = &scope
	}
	if body.ModelType != nil {
		in.ModelType = body.ModelType
	}
	m, err := h.svc.Update(r.Context(), projectID, modelID, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	if err := h.svc.Delete(r.Context(), projectID, modelID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Deprecate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	if err := h.svc.Deprecate(r.Context(), projectID, modelID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	modelID := chi.URLParam(r, "modelID")
	if err := h.svc.Activate(r.Context(), projectID, modelID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

// writeServiceError translates a service error to its HTTP shape.
// Common cases route through apierror; the in-use + setup-incomplete
// envelopes carry slice-specific fields the frontend reads, so they
// stay inline.
func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	var inUseErr *ErrEntityInUse
	if errors.As(err, &inUseErr) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":          string(apierror.CodeInUse),
			"error":         "model_in_use",
			"field_count":   inUseErr.Usage.FieldCount,
			"field_samples": inUseErr.Usage.FieldSamples,
			"message":       "Fields reference this model as a value target. Deprecate or repoint those fields first.",
		})
		return
	}
	var setupErr *ErrSetupIncomplete
	if errors.As(err, &setupErr) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":         "no_ontologies",
			"error":        "no_ontologies",
			"error_code":   "NO_ONTOLOGIES",
			"message":      "No ontologies configured for this project. Configure at least one before creating models.",
			"settings_url": "/projects/" + setupErr.ProjectID + "/settings#ontology",
		})
		return
	}
	if IsNotFound(err) {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("model handler error", "err", err)
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

// parsePrefixedClass parses "crm:E21_Person" into a PathElement{Type:"class"}.
func parsePrefixedClass(s string) (domain.PathElement, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return domain.PathElement{}, nil
	}
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return domain.PathElement{}, fmt.Errorf("ontology_scope must be in prefix:LocalName form (got %q)", s)
	}
	return domain.PathElement{
		Type:      "class",
		Prefix:    parts[0],
		LocalName: parts[1],
	}, nil
}

func parseOntologyScopeInput(raw any) (domain.PathElement, error) {
	switch v := raw.(type) {
	case nil:
		return domain.PathElement{}, nil
	case map[string]any:
		prefix, _ := v["prefix"].(string)
		localName, _ := v["local_name"].(string)
		uri, _ := v["uri"].(string)
		typ, _ := v["type"].(string)
		classCode, _ := v["class_code"].(string)
		if typ == "" {
			typ = "class"
		}
		if prefix == "" || localName == "" {
			return domain.PathElement{}, fmt.Errorf("ontology scope requires prefix and local_name")
		}
		if uri == "" {
			uri = prefix + ":" + localName
		}
		if classCode == "" {
			classCode = domain.DeriveClassCode(localName)
		}
		return domain.PathElement{
			Type:      typ,
			Prefix:    prefix,
			LocalName: localName,
			URI:       uri,
			ClassCode: classCode,
		}, nil
	default:
		return domain.PathElement{}, fmt.Errorf("ontology_scope must be a PathElement object")
	}
}
