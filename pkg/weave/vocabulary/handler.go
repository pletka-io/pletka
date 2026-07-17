package vocabulary

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

type LangResolver func(r *http.Request) string

type Handler struct {
	svc       *Service
	logger    *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
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

func NewHandler(svc *Service, logger *slog.Logger, languages []formschema.LanguageInfo, lang LangResolver) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, logger: logger, languages: languages, lang: lang}
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
	items, err := h.svc.SearchConceptListSourceEntries(r.Context(), projectID, listID, r.URL.Query().Get("q"), h.requestLang(r), requestLimit(r))
	if err != nil {
		h.writeServiceError(w, "search concept list source entries failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) SearchVocabularyEntries(w http.ResponseWriter, r *http.Request) {
	vocabularyID := chi.URLParam(r, "vocabularyID")
	items, err := h.svc.SearchVocabularyEntries(r.Context(), vocabularyID, r.URL.Query().Get("q"), h.requestLang(r), requestLimit(r))
	if err != nil {
		h.logger.Error("search vocabulary entries failed", "err", err, "vocabulary_id", vocabularyID)
		apierror.Write(w, apierror.Internal())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) ResolveEntry(w http.ResponseWriter, r *http.Request) {
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
