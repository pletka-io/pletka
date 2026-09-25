package vocabulary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

type LangResolver func(r *http.Request) string

type Handler struct {
	svc       *Service
	projects  auth.ProjectReader
	logger    *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
}

// canReadProject checks the request's caller has read access to projectID. Used
// by the "global" (non-project-scoped) routes to gate resources that belong to
// a project, so those routes are not cross-tenant readable.
func (h *Handler) canReadProject(ctx context.Context, projectID string) bool {
	if projectID == "" || h.projects == nil {
		return false
	}
	p, err := h.projects.GetByID(ctx, projectID)
	if err != nil || p == nil {
		return false
	}
	return auth.FromContext(ctx).Can(auth.ProjectRead, auth.ProjectResource(p), nil)
}

type conceptListBody struct {
	SystemName    string              `json:"system_name,omitempty"`
	UIName        domain.Translations `json:"ui_name,omitempty"`
	Description   domain.Translations `json:"description,omitempty"`
	Status        string              `json:"status,omitempty"`
	VocabularyID  string              `json:"vocabulary_id,omitempty"`
	ParentTermURI string              `json:"parent_term_uri,omitempty"`
	ListTypeURI   string              `json:"list_type_uri,omitempty"`
}

type conceptListEntryBody struct {
	VocabularyEntryID  string `json:"vocabulary_entry_id"`
	VocabularyEntryURI string `json:"vocabulary_entry_uri,omitempty"`
}

type conceptListEntryUpdateBody struct {
	CustomLabel domain.Translations `json:"custom_label,omitempty"`
}

type conceptListEntryReorderBody struct {
	EntryIDs []string `json:"entry_ids"`
}

// NewHandler builds the vocabulary HTTP handler.
func NewHandler(svc *Service, projects auth.ProjectReader, logger *slog.Logger, languages []formschema.LanguageInfo, lang LangResolver) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, projects: projects, logger: logger, languages: languages, lang: lang}
}

func (h *Handler) ListGlobalVocabularies(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListGlobalVocabularies(r.Context())
	if err != nil {
		h.logger.Error("list global vocabularies failed", "err", err)
		apierror.Write(w, apierror.Internal())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) AdminVocabularyEntityListSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildAdminVocabularyEntityListSchema(h.requestLang(r), h.languages))
}

func (h *Handler) AdminVocabularyData(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListAdminVocabularies(r.Context())
	if err != nil {
		h.logger.Error("list admin vocabularies failed", "err", err)
		apierror.Write(w, apierror.Internal())
		return
	}
	items = filterAdminVocabularies(
		items,
		r.URL.Query().Get("search"),
		r.URL.Query().Get("status"),
		r.URL.Query().Get("connector_type"),
	)
	sortAdminVocabularies(items, r.URL.Query().Get("sort_by"), r.URL.Query().Get("sort_dir") == "desc")
	total := len(items)
	page, perPage := paginationParams(r)
	items = paginate(items, page, perPage)
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *Handler) ListProjectVocabularies(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	items, err := h.svc.ListProjectVocabularies(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list project vocabularies failed", "err", err, "project_id", projectID)
		apierror.Write(w, apierror.Internal())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) ListProjectConceptLists(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	items, err := h.svc.ListProjectConceptLists(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list concept lists failed", "err", err, "project_id", projectID)
		apierror.Write(w, apierror.Internal())
		return
	}
	items = filterConceptLists(
		items,
		r.URL.Query().Get("search"),
		r.URL.Query().Get("status"),
		r.URL.Query().Get("vocabulary_id"),
	)
	sortConceptLists(items, r.URL.Query().Get("sort_by"), r.URL.Query().Get("sort_dir") == "desc")
	total := len(items)
	page, perPage := paginationParams(r)
	items = paginate(items, page, perPage)
	writeJSON(w, http.StatusOK, map[string]any{
		"items":         items,
		"concept_lists": items,
		"total":         total,
		"page":          page,
		"per_page":      perPage,
	})
}

func (h *Handler) ConceptListVocabularyFilterOptions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	items, err := h.svc.ListProjectConceptLists(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list concept list vocabulary filters failed", "err", err, "project_id", projectID)
		apierror.Write(w, apierror.Internal())
		return
	}
	options := conceptListVocabularyFilterOptions(items)
	writeJSON(w, http.StatusOK, map[string]any{"options": options})
}

func (h *Handler) ConceptListCreateFormSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	vocabs, err := h.svc.ListProjectVocabularies(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, "list vocabularies for concept list form failed", err)
		return
	}
	schema := BuildConceptListFormSchema(formschema.ModeCreate, nil, vocabs, projectID, h.requestLang(r), h.languages)
	writeJSON(w, http.StatusOK, schema)
}

func (h *Handler) ConceptListEditFormSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	item, err := h.svc.GetProjectConceptList(r.Context(), projectID, listID)
	if err != nil {
		h.writeServiceError(w, "get concept list for form failed", err)
		return
	}
	if item == nil {
		apierror.Write(w, apierror.NotFound("concept list not found"))
		return
	}
	vocabs, err := h.svc.ListProjectVocabularies(r.Context(), projectID)
	if err != nil {
		h.writeServiceError(w, "list vocabularies for concept list form failed", err)
		return
	}
	schema := BuildConceptListFormSchema(formschema.ModeEdit, item, vocabs, projectID, h.requestLang(r), h.languages)
	writeJSON(w, http.StatusOK, schema)
}

func (h *Handler) GetProjectConceptList(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	item, err := h.svc.GetProjectConceptList(r.Context(), projectID, listID)
	if err != nil {
		h.logger.Error("get concept list failed", "err", err, "project_id", projectID, "list_id", listID)
		apierror.Write(w, apierror.Internal())
		return
	}
	if item == nil {
		apierror.Write(w, apierror.NotFound("concept list not found"))
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateProjectConceptList(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var body conceptListBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	item, err := h.svc.CreateConceptList(r.Context(), projectID, conceptListInputFromBody(body))
	if err != nil {
		h.writeServiceError(w, "create concept list failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) UpdateProjectConceptList(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	var body conceptListBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	item, err := h.svc.UpdateConceptList(r.Context(), projectID, listID, conceptListInputFromBody(body))
	if err != nil {
		h.writeServiceError(w, "update concept list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) DeleteProjectConceptList(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	if err := h.svc.DeleteConceptList(r.Context(), projectID, listID); err != nil {
		h.writeServiceError(w, "delete concept list failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) AddProjectConceptListEntry(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	var body conceptListEntryBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	item, err := h.svc.AddConceptListEntry(r.Context(), projectID, listID, body.VocabularyEntryID, body.VocabularyEntryURI)
	if err != nil {
		h.writeServiceError(w, "add concept list entry failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// CreateProjectConceptListTerm authors a local (hand-typed) concept and adds
// it to the list — no remote authority required.
func (h *Handler) CreateProjectConceptListTerm(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	var in CreateTermInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	item, err := h.svc.CreateLocalTerm(r.Context(), projectID, listID, in)
	if err != nil {
		h.writeServiceError(w, "create local term failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

type conceptBroaderBody struct {
	BroaderID string `json:"broader_id"`
	Position  int    `json:"position,omitempty"`
}

type conceptListSealBody struct {
	IsClosed bool `json:"is_closed"`
}

// SealProjectConceptList marks a list sealed/unsealed (complete membership).
func (h *Handler) SealProjectConceptList(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	var body conceptListSealBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	if err := h.svc.SetListClosed(r.Context(), projectID, listID, body.IsClosed); err != nil {
		h.writeServiceError(w, "seal concept list failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddConceptBroader records a broader/narrower edge for a term, scoped to the
// list in the route (the service enforces project/list ownership).
func (h *Handler) AddConceptBroader(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	conceptID := chi.URLParam(r, "conceptID")
	var body conceptBroaderBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	edge := domain.ConceptBroaderEdge{ConceptID: conceptID, BroaderID: body.BroaderID, Position: body.Position}
	out, err := h.svc.AddBroader(r.Context(), projectID, listID, edge)
	if err != nil {
		h.writeServiceError(w, "add broader edge failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// ListConceptBroader lists a term's broader concepts within the list.
func (h *Handler) ListConceptBroader(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	conceptID := chi.URLParam(r, "conceptID")
	edges, err := h.svc.ListBroader(r.Context(), projectID, listID, conceptID)
	if err != nil {
		h.writeServiceError(w, "list broader edges failed", err)
		return
	}
	writeJSON(w, http.StatusOK, edges)
}

// RemoveConceptBroader deletes a broader edge, scoped to project/list/term.
func (h *Handler) RemoveConceptBroader(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	conceptID := chi.URLParam(r, "conceptID")
	edgeID := chi.URLParam(r, "edgeID")
	if err := h.svc.RemoveBroader(r.Context(), projectID, listID, conceptID, edgeID); err != nil {
		h.writeServiceError(w, "remove broader edge failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateProjectConceptListEntry(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	entryID := chi.URLParam(r, "entryID")
	var body conceptListEntryUpdateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	item, err := h.svc.UpdateConceptListEntry(r.Context(), projectID, listID, entryID, body.CustomLabel)
	if err != nil {
		h.writeServiceError(w, "update concept list entry failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) ReorderProjectConceptListEntries(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	var body conceptListEntryReorderBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	items, err := h.svc.ReorderConceptListEntries(r.Context(), projectID, listID, body.EntryIDs)
	if err != nil {
		h.writeServiceError(w, "reorder concept list entries failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) RemoveProjectConceptListEntry(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	entryID := chi.URLParam(r, "entryID")
	if err := h.svc.RemoveConceptListEntry(r.Context(), projectID, listID, entryID); err != nil {
		h.writeServiceError(w, "remove concept list entry failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SearchConceptListEntries(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "listID")
	// This route is not project-scoped in its path; enforce that the caller can
	// read the list's project so it is not cross-tenant readable.
	projID, err := h.svc.ConceptListProjectID(r.Context(), listID)
	if err != nil {
		apierror.Write(w, apierror.Internal())
		return
	}
	if projID == "" || !h.canReadProject(r.Context(), projID) {
		apierror.Write(w, apierror.NotFound("concept list not found"))
		return
	}
	items, err := h.svc.SearchConceptListEntries(r.Context(), listID, r.URL.Query().Get("q"), h.requestLang(r), requestLimit(r))
	if err != nil {
		h.logger.Error("search concept list entries failed", "err", err, "list_id", listID)
		apierror.Write(w, apierror.Internal())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) SearchConceptListSourceEntries(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	items, degraded, err := h.svc.SearchConceptListSourceEntries(r.Context(), projectID, listID, r.URL.Query().Get("q"), h.requestLang(r), requestLimit(r))
	if err != nil {
		h.writeServiceError(w, "search concept list source entries failed", err)
		return
	}
	body := map[string]any{"items": items, "total": len(items)}
	if degraded {
		// A failed remote lookup and a genuine no-match are otherwise the same
		// 200 with an empty list, so neither a browser's network tab nor a
		// developer without server-log access can tell them apart. The key is
		// absent (not false) when nothing degraded, so a reader cannot mistake
		// every other connector for a degraded one.
		body["degraded"] = true
	}
	writeJSON(w, http.StatusOK, body)
}

func (h *Handler) SearchVocabularyEntries(w http.ResponseWriter, r *http.Request) {
	vocabularyID := chi.URLParam(r, "vocabularyID")
	// A project-scoped vocabulary is only searchable by readers of its project;
	// global (shared-authority) vocabularies stay open.
	projID, found, err := h.svc.VocabularyProjectID(r.Context(), vocabularyID)
	if err != nil {
		apierror.Write(w, apierror.Internal())
		return
	}
	if !found {
		apierror.Write(w, apierror.NotFound("vocabulary not found"))
		return
	}
	if projID != "" && !h.canReadProject(r.Context(), projID) {
		apierror.Write(w, apierror.NotFound("vocabulary not found"))
		return
	}
	items, degraded, err := h.svc.SearchVocabularyEntriesDegradable(r.Context(), vocabularyID, r.URL.Query().Get("q"), h.requestLang(r), requestLimit(r), "")
	if err != nil {
		h.logger.Error("search vocabulary entries failed", "err", err, "vocabulary_id", vocabularyID)
		apierror.Write(w, apierror.Internal())
		return
	}
	body := map[string]any{"items": items, "total": len(items)}
	if degraded {
		// A failed remote lookup and a genuine no-match are otherwise the same
		// 200 with an empty list, so neither a browser's network tab nor a
		// developer without server-log access can tell them apart.
		body["degraded"] = true
	}
	writeJSON(w, http.StatusOK, body)
}

func (h *Handler) ResolveEntry(w http.ResponseWriter, r *http.Request) {
	// ResolveEntry upserts a cached entry and can trigger an outbound connector
	// fetch, so it must not be driven by anonymous callers (SSRF / cache-write).
	if auth.FromContext(r.Context()).IsAnonymous {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	item, err := h.svc.ResolveEntry(r.Context(), r.URL.Query().Get("uri"), h.requestLang(r))
	if err != nil {
		h.logger.Error("resolve vocabulary entry failed", "err", err, "uri", r.URL.Query().Get("uri"))
		apierror.Write(w, apierror.Internal())
		return
	}
	if item == nil {
		apierror.Write(w, apierror.NotFound("vocabulary entry not found"))
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) writeServiceError(w http.ResponseWriter, message string, err error) {
	h.logger.Error(message, "err", err)
	apierror.Write(w, apierror.FromError(err))
}

func conceptListInputFromBody(body conceptListBody) ConceptListInput {
	return ConceptListInput{
		SystemName:   body.SystemName,
		UIName:       body.UIName,
		Description:  body.Description,
		Status:       body.Status,
		VocabularyID: body.VocabularyID,
		ListTypeURI:  firstNonEmpty(body.ParentTermURI, body.ListTypeURI),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (h *Handler) requestLang(r *http.Request) string {
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	return h.lang(r)
}

func requestLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		return 50
	}
	return limit
}

func paginationParams(r *http.Request) (int, int) {
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
	return page, perPage
}

func filterConceptLists(items []ConceptListView, query, statusRaw, vocabularyRaw string) []ConceptListView {
	query = strings.ToLower(strings.TrimSpace(query))
	statuses := parseCSVSet(statusRaw)
	vocabularies := parseCSVSet(vocabularyRaw)
	if query == "" && len(statuses) == 0 && len(vocabularies) == 0 {
		return items
	}
	out := items[:0]
	for _, item := range items {
		if len(statuses) > 0 && !statuses[item.Status] {
			continue
		}
		if len(vocabularies) > 0 && (item.VocabularyID == nil || !vocabularies[*item.VocabularyID]) {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.ID,
				item.SemanticID,
				item.SystemName,
				item.Status,
				item.VocabularyLabel,
				item.ListTypeLabel,
				item.ListTypeURI,
				translationsText(item.UIName),
				translationsText(item.Description),
			}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func filterAdminVocabularies(items []AdminVocabularyView, query, statusRaw, connectorRaw string) []AdminVocabularyView {
	query = strings.ToLower(strings.TrimSpace(query))
	statuses := parseCSVSet(statusRaw)
	connectors := parseCSVSet(connectorRaw)
	if query == "" && len(statuses) == 0 && len(connectors) == 0 {
		return items
	}
	out := items[:0]
	for _, item := range items {
		if len(statuses) > 0 && !statuses[item.Status] {
			continue
		}
		if len(connectors) > 0 && !connectors[item.ConnectorType] {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.ID,
				item.SemanticID,
				item.SystemName,
				item.Status,
				item.ConnectorType,
				item.BaseURI,
				translationsText(item.UIName),
				translationsText(item.Description),
			}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func parseCSVSet(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimSpace(part)
		if value != "" {
			out[value] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func conceptListVocabularyFilterOptions(items []ConceptListView) []formschema.FilterOption {
	type option struct {
		value string
		label string
	}
	seen := map[string]option{}
	for _, item := range items {
		if item.VocabularyID == nil || strings.TrimSpace(*item.VocabularyID) == "" {
			continue
		}
		label := strings.TrimSpace(item.VocabularyLabel)
		if label == "" {
			label = *item.VocabularyID
		}
		seen[*item.VocabularyID] = option{value: *item.VocabularyID, label: label}
	}
	values := make([]option, 0, len(seen))
	for _, opt := range seen {
		values = append(values, opt)
	}
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].label == values[j].label {
			return values[i].value < values[j].value
		}
		return values[i].label < values[j].label
	})
	out := make([]formschema.FilterOption, 0, len(values))
	for _, opt := range values {
		out = append(out, formschema.FilterOption{
			Value: opt.value,
			Label: domain.Translations{"en": opt.label},
		})
	}
	return out
}

func sortConceptLists(items []ConceptListView, sortBy string, desc bool) {
	if strings.TrimSpace(sortBy) == "" {
		sortBy = "ui_name"
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := conceptListSortValue(items[i], sortBy), conceptListSortValue(items[j], sortBy)
		if left == right {
			left, right = items[i].SemanticID, items[j].SemanticID
		}
		if desc {
			return left > right
		}
		return left < right
	})
}

func sortAdminVocabularies(items []AdminVocabularyView, sortBy string, desc bool) {
	if strings.TrimSpace(sortBy) == "" {
		sortBy = "ui_name"
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := adminVocabularySortValue(items[i], sortBy), adminVocabularySortValue(items[j], sortBy)
		if left == right {
			left, right = items[i].ID, items[j].ID
		}
		if desc {
			return left > right
		}
		return left < right
	})
}

func conceptListSortValue(item ConceptListView, sortBy string) string {
	switch sortBy {
	case "semantic_id":
		return item.SemanticID
	case "system_name":
		return item.SystemName
	case "status":
		return item.Status
	case "vocabulary":
		return item.VocabularyLabel
	case "entries":
		return leftPadInt(item.EntryCount)
	case "bound_fields":
		return leftPadInt(item.BoundFieldCount)
	default:
		return translationsText(item.UIName)
	}
}

func adminVocabularySortValue(item AdminVocabularyView, sortBy string) string {
	switch sortBy {
	case "system_name":
		return item.SystemName
	case "connector_type":
		return item.ConnectorType
	case "entries":
		return leftPadInt(item.EntryCount)
	case "projects":
		return leftPadInt(item.ProjectCount)
	default:
		return translationsText(item.UIName)
	}
}

func paginate[T any](items []T, page, perPage int) []T {
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

func translationsText(value map[string]string) string {
	if len(value) == 0 {
		return ""
	}
	parts := make([]string, 0, len(value))
	for _, text := range value {
		parts = append(parts, text)
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func leftPadInt(value int) string {
	return strconv.FormatInt(int64(value+1000000000), 10)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ExportConceptListSKOS streams a concept list as SKOS Turtle (#3599). The
// route is project-scoped; the caller must be able to read the project. The
// document is rendered into a buffer first so a mid-render error still returns
// a clean error response rather than a truncated body.
func (h *Handler) ExportConceptListSKOS(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	listID := chi.URLParam(r, "listID")
	if !h.canReadProject(r.Context(), projectID) {
		apierror.Write(w, apierror.NotFound("concept list not found"))
		return
	}
	var buf bytes.Buffer
	if err := h.svc.RenderConceptListSKOS(r.Context(), projectID, listID, &buf); err != nil {
		h.writeServiceError(w, "render concept list SKOS failed", err)
		return
	}
	w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", listID+".ttl"))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
