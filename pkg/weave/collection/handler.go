package collection

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
	"github.com/pletka-io/pletka/pkg/weave/publication"
)

type LangResolver func(r *http.Request) string

type Handler struct {
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
	// i18n walks LocalizedText nodes inside list-row payloads (origin_label
	// is the first such node) — schema-driven UI rule, see
	// .claude/rules/ui-patterns.md.
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

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
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
	if sc := strings.TrimSpace(r.URL.Query().Get("scope_class")); sc != "" {
		opts = append(opts, domain.WithFilter("scope_class", sc))
	}
	if cat := strings.TrimSpace(r.URL.Query().Get("category_id")); cat != "" {
		opts = append(opts, domain.WithFilter("category_id", cat))
	}

	collections, _, err := h.svc.List(r.Context(), projectID, opts...)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	forkedOrigins, err := h.svc.ForkOriginsForProject(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	// Receipt-adopted collections — UNION with local rows. Task 3a.
	adoptedCollections, adoptedOrigins, err := h.svc.ListAdoptedExplicit(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	// Reference-adopted collections — task 3b. Collections referenced
	// via part_of_collection_id by overrides in this project but living
	// in another project.
	referenceAdoptedCollections, err := h.svc.ListAdoptedByReference(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	referencedIDs := make(map[string]bool, len(referenceAdoptedCollections))
	for _, c := range referenceAdoptedCollections {
		referencedIDs[c.ID] = true
	}

	type item struct {
		ID                       string              `json:"id"`
		SemanticID               string              `json:"semantic_id,omitempty"`
		SystemName               string              `json:"system_name,omitempty"`
		UIName                   domain.Translations `json:"ui_name,omitempty"`
		Description              domain.Translations `json:"description,omitempty"`
		Status                   string              `json:"status"`
		ProjectID                string              `json:"project_id"`
		OriginalProjectID        string              `json:"original_project_id,omitempty"`
		Deprecated               bool                `json:"deprecated"`
		Owned                    bool                `json:"owned"`
		CanDeprecate             bool                `json:"can_deprecate"`
		CanActivate              bool                `json:"can_activate"`
		FieldCount               int                 `json:"field_count,omitempty"`
		OntologyScope            string              `json:"ontology_scope,omitempty"`
		ScopeClassCode           string              `json:"scope_class_code,omitempty"`
		DefaultCategoryID        string              `json:"default_category_id,omitempty"`
		CollectionNumber         int                 `json:"collection_number,omitempty"`
		CanonicalCollectionOrder int                 `json:"canonical_collection_order,omitempty"`
		Origin                   domain.Origin       `json:"origin"`
		OriginKind               string              `json:"origin_kind"`
		// OriginLabel is a LocalizedText so EntityListBadge.svelte can
		// render via tr() with no domain wording baked into the frontend.
		// Resolved by h.i18n.Resolve before encode.
		OriginLabel i18n.LocalizedText `json:"origin_label,omitempty"`
	}
	items := make([]item, 0, len(collections)+len(adoptedCollections))
	rowFor := func(c *domain.Collection, origin domain.Origin, notReferenced bool) item {
		row := item{
			ID:                       c.ID,
			SemanticID:               c.SemanticID,
			SystemName:               c.SystemName,
			UIName:                   c.UIName,
			Description:              c.Description,
			Status:                   string(c.Status),
			ProjectID:                c.ProjectID,
			Deprecated:               c.Deprecated,
			OntologyScope:            c.OntologyScope.PrefixedName(),
			ScopeClassCode:           c.OntologyScope.ShortCode(),
			CollectionNumber:         c.CollectionNumber,
			CanonicalCollectionOrder: c.CanonicalCollectionOrder,
			Origin:                   origin,
			OriginKind:               string(origin.Kind),
			OriginLabel:              originListLabel(origin, notReferenced),
		}
		if c.DefaultCategoryID != nil {
			row.DefaultCategoryID = *c.DefaultCategoryID
		}
		row.Owned = c.ProjectID == projectID
		row.CanDeprecate = row.Owned && !c.Deprecated
		row.CanActivate = row.Owned && c.Deprecated
		if origin.Kind == domain.OriginForked {
			row.OriginalProjectID = origin.SourceProjectID
		}
		return row
	}
	seenIDs := make(map[string]bool, len(collections))
	for _, c := range collections {
		origin := domain.OwnOrigin()
		if forked, ok := forkedOrigins[c.ID]; ok {
			origin = forked
		}
		if len(originFilter) > 0 && !originFilter[origin.Kind] {
			continue
		}
		seenIDs[c.ID] = true
		items = append(items, rowFor(c, origin, false))
	}
	// Append receipt-adopted collections, dedup against local rows.
	// Local wins per Q4 (explicit-dominates). Receipt rows whose ID is
	// not in the reference set get the "not yet referenced" Q10 note.
	for _, c := range adoptedCollections {
		if seenIDs[c.ID] {
			continue
		}
		origin := adoptedOrigins[c.ID]
		if origin.Kind == "" {
			origin = domain.AdoptedOrigin(c.ProjectID, c.ID)
		}
		if len(originFilter) > 0 && !originFilter[origin.Kind] {
			continue
		}
		seenIDs[c.ID] = true
		items = append(items, rowFor(c, origin, !referencedIDs[c.ID]))
	}
	// Append reference-adopted collections — task 3b. Same dedup; any
	// row already tagged Own/Adapted/AdoptedExplicit keeps its stronger
	// state.
	for _, c := range referenceAdoptedCollections {
		if seenIDs[c.ID] {
			continue
		}
		origin := domain.Origin{
			Kind:            domain.OriginAdoptedReference,
			SourceProjectID: c.ProjectID,
			SourceEntityID:  c.ID,
		}
		if len(originFilter) > 0 && !originFilter[origin.Kind] {
			continue
		}
		items = append(items, rowFor(c, origin, false))
	}
	total := int64(len(items))
	items = paginateItems(items, page, perPage)

	// Attach field-composition counts for the page in one batch query.
	if len(items) > 0 {
		ids := make([]string, 0, len(items))
		for i := range items {
			ids = append(ids, items[i].ID)
		}
		if counts, cErr := h.svc.BatchCompositionCounts(r.Context(), projectID, ids); cErr != nil {
			h.log.Warn("collection composition counts", "project", projectID, "err", cErr)
		} else {
			for i := range items {
				if c, ok := counts[items[i].ID]; ok {
					items[i].FieldCount = c.FieldCount
				}
			}
		}
		publication.Attach(r.Context(), h.pub, h.log, projectID, "collection", ids, func(i int) *string { return &items[i].Status })
	}

	// Walk-resolve LocalizedText nodes inside the row payload so the
	// wire format matches the schema-driven UI contract (Translations
	// map per row, frontend picks via tr()).
	if h.i18n != nil {
		h.i18n.Resolve(&items, h.lang(r))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"collections": items,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
	})
}

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

// originListLabel returns the row-badge label as a LocalizedText.
// notReferenced is only meaningful for OriginAdopted (Q10 'not yet
// referenced' surfaces when a receipt exists but no override in the
// current project points at the entity yet). Mirror of
// model.originListLabel — kept in sync.
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
	collectionID := chi.URLParam(r, "collectionID")
	c, err := h.svc.Get(r.Context(), projectID, collectionID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	if c == nil {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	report, err := h.svc.Stats(r.Context(), projectID, collectionID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) Fork(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	if projectID == "" || collectionID == "" {
		http.Error(w, "Project ID and Collection ID are required", http.StatusBadRequest)
		return
	}

	collection, err := h.svc.ForkFromSource(r.Context(), projectID, collectionID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":  collection.ID,
		"url": "/projects/" + projectID + "/collections/" + collection.ID,
	})
}

// ScopeClassesOptions returns the distinct ontology scope classes used
// by collections in the project, in the {"options":[...]} envelope
// expected by the entity-list lazy-options loader.
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

// CategoriesOptions returns the distinct categories assigned to
// collections in the project, in the {"options":[...]} envelope.
func (h *Handler) CategoriesOptions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	cats, err := h.svc.CategoriesUsed(r.Context(), projectID)
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

// ---------------------------------------------------------------------------
// Override editing
// ---------------------------------------------------------------------------

func (h *Handler) ListOverrides(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	rows, err := h.svc.ListOverrides(r.Context(), projectID, collectionID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entity_type": "collection",
		"entity_id":   collectionID,
		"project_id":  projectID,
		"overrides":   rows,
	})
}

func (h *Handler) SaveOverrides(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")

	var body struct {
		CommitMessage string                 `json:"commit_message"`
		Overrides     []domain.FieldOverride `json:"overrides"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	saved, diff, err := h.svc.SaveOverrides(r.Context(), projectID, collectionID, body.Overrides, body.CommitMessage)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entity_type": "collection",
		"entity_id":   collectionID,
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

type collectionWriteBody struct {
	UIName                   domain.Translations `json:"ui_name"`
	Description              domain.Translations `json:"description,omitempty"`
	SystemName               string              `json:"system_name,omitempty"`
	Status                   string              `json:"status,omitempty"`
	OntologyScope            any                 `json:"ontology_scope,omitempty"`
	DefaultCategoryID        *string             `json:"default_category_id,omitempty"`
	CollectionNumber         *int                `json:"collection_number,omitempty"`
	CanonicalCollectionOrder *int                `json:"canonical_collection_order,omitempty"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var body collectionWriteBody
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
	if body.DefaultCategoryID != nil {
		in.DefaultCategoryID = *body.DefaultCategoryID
	}
	c, err := h.svc.Create(r.Context(), projectID, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	var body collectionWriteBody
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
	if body.DefaultCategoryID != nil {
		in.DefaultCategoryID = body.DefaultCategoryID
	}
	if body.CollectionNumber != nil {
		in.CollectionNumber = body.CollectionNumber
	}
	if body.CanonicalCollectionOrder != nil {
		in.CanonicalCollectionOrder = body.CanonicalCollectionOrder
	}
	c, err := h.svc.Update(r.Context(), projectID, collectionID, in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	if err := h.svc.Delete(r.Context(), projectID, collectionID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Deprecate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	if err := h.svc.Deprecate(r.Context(), projectID, collectionID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	collectionID := chi.URLParam(r, "collectionID")
	if err := h.svc.Activate(r.Context(), projectID, collectionID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	var inUseErr *ErrEntityInUse
	if errors.As(err, &inUseErr) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":          string(apierror.CodeInUse),
			"error":         "collection_in_use",
			"field_count":   inUseErr.Usage.FieldCount,
			"field_samples": inUseErr.Usage.FieldSamples,
			"message":       "Fields reference this collection as a value target. Deprecate or repoint those fields first.",
		})
		return
	}
	var setupErr *ErrSetupIncomplete
	if errors.As(err, &setupErr) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":         "no_ontologies",
			"error":        "no_ontologies",
			"error_code":   "NO_ONTOLOGIES",
			"message":      "No ontologies configured for this project. Configure at least one before creating collections.",
			"settings_url": "/projects/" + setupErr.ProjectID + "/settings-v2#ontology",
		})
		return
	}
	if IsNotFound(err) {
		apierror.Write(w, apierror.NotFound(err.Error()))
		return
	}
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("collection handler error", "err", err)
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
