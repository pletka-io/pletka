package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	weavepkg "github.com/pletka-io/pletka/pkg/weave"
)

type queriesProvider interface {
	Queries() *sqlcgen.Queries
}

type poolProvider interface {
	Pool() *pgxpool.Pool
}

type Handler struct {
	weave  domain.WeaveStore
	logger *slog.Logger
}

type Host struct {
	Weave  domain.WeaveStore
	Logger *slog.Logger
}

func (h Host) Validate() error {
	if h.Weave == nil {
		return fmt.Errorf("search host missing required dependencies: Weave")
	}
	return nil
}

func NewHandler(weave domain.WeaveStore, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		weave:  weave,
		logger: logger.With("handler", "weave-search"),
	}
}

func Mount(r chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	NewHandler(host.Weave, host.Logger).Mount(r)
}

func (h *Handler) Mount(r chi.Router) {
	r.With(weaveauth.WithProjectVersionContext).Get("/api/v1/projects/{projectID}/search", h.EntitySearch)
	r.With(weaveauth.WithProjectVersionContext).Get("/api/v1/projects/{projectID}/path-suggestions", h.PathSuggestionsHandler)
}

// EntitySearch handles GET /api/v1/projects/{projectID}/search?type=field&q=...
func (h *Handler) EntitySearch(w http.ResponseWriter, r *http.Request) {
	if searchUnavailableInReleaseMode(w, r) {
		return
	}
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		writeAPIError(w, "missing project id", http.StatusBadRequest)
		return
	}

	params := parseSearchParams(r, projectID)

	q, ok := h.queries()
	if !ok {
		writeAPIError(w, "search not available", http.StatusInternalServerError)
		return
	}

	switch params.Type {
	case "field":
		h.searchFields(w, r, q, params)
	case "collection":
		h.searchCollections(w, r, q, params)
	default:
		writeAPIError(w, "type must be field or collection", http.StatusBadRequest)
	}
}

func (h *Handler) queries() (*sqlcgen.Queries, bool) {
	qp, ok := h.weave.(queriesProvider)
	if !ok {
		return nil, false
	}

	return qp.Queries(), true
}

func (h *Handler) pool() (*pgxpool.Pool, bool) {
	pp, ok := h.weave.(poolProvider)
	if !ok {
		return nil, false
	}

	return pp.Pool(), true
}

func searchUnavailableInReleaseMode(w http.ResponseWriter, r *http.Request) bool {
	if weaveauth.ProjectVersionFromContext(r.Context()) == "" {
		return false
	}
	writeAPIError(w, "not found", http.StatusNotFound)
	return true
}

func (h *Handler) searchFields(w http.ResponseWriter, r *http.Request, q *sqlcgen.Queries, params domain.SearchParams) {
	ctx := r.Context()

	pool, ok := h.pool()
	if !ok {
		writeAPIError(w, "search not available", http.StatusInternalServerError)
		return
	}

	targets, err := resolveProjectTargets(ctx, pool, params.ProjectID, params.Scope)
	if err != nil {
		h.logger.Error("resolve project targets failed", "err", err, "project_id", params.ProjectID)
		writeAPIError(w, "search query failed", http.StatusInternalServerError)
		return
	}

	linked := h.linkedSet(ctx, q, "model", params.TargetID)
	adoptedOrigins, err := weavepkg.AdoptionOriginsForProject(ctx, h.weave, params.ProjectID, "field")
	if err != nil {
		h.logger.Warn("load field adoption origins failed", "err", err, "project_id", params.ProjectID)
		adoptedOrigins = map[string]domain.Origin{}
	}
	forkedOrigins, err := weavepkg.ForkOriginsForProject(ctx, h.weave, params.ProjectID, "field")
	if err != nil {
		h.logger.Warn("load field fork origins failed", "err", err, "project_id", params.ProjectID)
		forkedOrigins = map[string]domain.Origin{}
	}

	candidates := make([]fieldSearchCandidate, 0)
	for _, target := range targets {
		targetParams := params
		targetFieldIDs, err := resolveTargetPathFieldIDs(ctx, pool, target, targetParams)
		if err != nil {
			h.logger.Error("resolve target path field ids failed", "err", err, "project_id", params.ProjectID, "target_id", target.ProjectID, "target_version", target.Version)
			writeAPIError(w, "search query failed", http.StatusInternalServerError)
			return
		}
		if targetFieldIDs != nil && len(targetFieldIDs) == 0 {
			continue
		}
		if targetFieldIDs != nil {
			targetParams.PathFieldIDs = targetFieldIDs
		} else {
			targetParams.PathFieldIDs = []string{}
		}

		var targetRows []fieldSearchCandidate
		if strings.TrimSpace(target.Version) == "" {
			rows, err := q.WeaveSearchFields(ctx, sqlcgen.WeaveSearchFieldsParams{
				SortBy:            "relevance",
				ResultOffset:      0,
				ResultLimit:       5000,
				ProjectID:         target.ProjectID,
				Scope:             "project",
				Search:            targetParams.Query,
				PathLocalName:     targetParams.PathLocalName,
				PathPrefix:        targetParams.PathPrefix,
				Category:          targetParams.Category,
				ExpectedValueType: targetParams.ExpectedValue,
				OntologyClass:     targetParams.OntologyClass,
				OntologyPrefix:    targetParams.OntologyPrefix,
				PathFieldIds:      targetParams.PathFieldIDs,
			})
			if err != nil {
				h.logger.Error("search live fields query failed", "err", err, "project_id", target.ProjectID)
				writeAPIError(w, "search query failed", http.StatusInternalServerError)
				return
			}
			targetRows = make([]fieldSearchCandidate, 0, len(rows))
			for _, row := range rows {
				targetRows = append(targetRows, liveFieldCandidateFromRow(params.ProjectID, row))
			}
		} else {
			targetRows, err = h.searchArchivedFields(ctx, pool, target, targetParams, params.ProjectID)
			if err != nil {
				h.logger.Error("search archived fields query failed", "err", err, "project_id", target.ProjectID, "version", target.Version)
				writeAPIError(w, "search query failed", http.StatusInternalServerError)
				return
			}
		}
		candidates = append(candidates, targetRows...)
	}

	refMap, err := loadFieldExpectedRefsForCandidates(ctx, q, pool, params.ProjectID, candidates)
	if err != nil {
		h.logger.Warn("load field expected refs failed", "err", err, "project_id", params.ProjectID)
	}

	items := buildFieldSearchResults(candidates, refMap, linked, adoptedOrigins, forkedOrigins, params.ProjectID)
	sortSearchResults(items, params.Sort)
	total := len(items)
	items = paginateSearchResults(items, params.Offset, params.Limit)

	writeJSON(w, http.StatusOK, domain.SearchResponse{
		Items:      items,
		TotalCount: total,
	})
}

type fieldExpectedRefs struct {
	resourceModels      []string
	collectionModels    []string
	conceptLists        []string
	resourceModelRefs   []domain.EntityRef
	collectionModelRefs []domain.EntityRef
	conceptListRefs     []domain.EntityRef
}

func loadFieldExpectedRefs(ctx context.Context, q *sqlcgen.Queries, projectID string, rows []sqlcgen.WeaveSearchFieldsRow) (map[string]fieldExpectedRefs, error) {
	if len(rows) == 0 {
		return map[string]fieldExpectedRefs{}, nil
	}

	fieldIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		fieldIDs = append(fieldIDs, row.ID)
	}

	baseRows, err := q.WeaveListBaseOverridesForFields(ctx, sqlcgen.WeaveListBaseOverridesForFieldsParams{
		ProjectID: projectID,
		Column2:   fieldIDs,
	})
	if err != nil {
		return nil, err
	}
	if len(baseRows) == 0 {
		return map[string]fieldExpectedRefs{}, nil
	}

	overrideToField := make(map[int64]string, len(baseRows))
	overrideIDs := make([]int64, 0, len(baseRows))
	for _, row := range baseRows {
		overrideToField[row.ID] = row.FieldID
		overrideIDs = append(overrideIDs, row.ID)
	}

	refRows, err := q.WeaveListRefsForOverrides(ctx, overrideIDs)
	if err != nil {
		return nil, err
	}

	names := loadEntityRefNames(ctx, q, refRows)
	out := make(map[string]fieldExpectedRefs, len(baseRows))
	for _, row := range refRows {
		fieldID := overrideToField[row.OverrideID]
		current := out[fieldID]
		url := expectedRefURL(projectID, row.RefType, row.TargetID, row.SemanticID)
		entityRef := domain.EntityRef{
			ID:         row.TargetID,
			SemanticID: row.SemanticID,
			Name:       names[row.TargetID],
			URL:        url,
		}
		switch row.RefType {
		case "resource_model":
			current.resourceModels = append(current.resourceModels, row.TargetID)
			current.resourceModelRefs = append(current.resourceModelRefs, entityRef)
		case "collection_model":
			current.collectionModels = append(current.collectionModels, row.TargetID)
			current.collectionModelRefs = append(current.collectionModelRefs, entityRef)
		case "concept_list":
			current.conceptLists = append(current.conceptLists, row.TargetID)
			current.conceptListRefs = append(current.conceptListRefs, entityRef)
		}
		out[fieldID] = current
	}
	return out, nil
}

func expectedRefURL(projectID, refType, targetID, semanticID string) string {
	if refType == "concept_list" {
		return domain.EntityURLForRefType(projectID, refType, targetID)
	}
	sid := domain.ParseSemanticID(semanticID)
	if url := sid.URL(); url != "" {
		return url
	}
	return domain.EntityURLForRefType(projectID, refType, targetID)
}

func loadEntityRefNames(ctx context.Context, q *sqlcgen.Queries, refRows []sqlcgen.WeaveOverrideRef) map[string]domain.Translations {
	modelIDs := make(map[string]struct{})
	collectionIDs := make(map[string]struct{})
	conceptListIDs := make(map[string]struct{})

	for _, row := range refRows {
		switch row.RefType {
		case "resource_model":
			modelIDs[row.TargetID] = struct{}{}
		case "collection_model":
			collectionIDs[row.TargetID] = struct{}{}
		case "concept_list":
			conceptListIDs[row.TargetID] = struct{}{}
		}
	}

	names := make(map[string]domain.Translations, len(modelIDs)+len(collectionIDs))

	if len(modelIDs) > 0 {
		rows, err := q.WeaveGetModelNamesByIDs(ctx, mapKeys(modelIDs))
		if err == nil {
			for _, row := range rows {
				names[row.ID] = parseTranslations(row.UiName)
			}
		}
	}

	if len(collectionIDs) > 0 {
		rows, err := q.WeaveGetCollectionNamesByIDs(ctx, mapKeys(collectionIDs))
		if err == nil {
			for _, row := range rows {
				names[row.ID] = parseTranslations(row.UiName)
			}
		}
	}
	if len(conceptListIDs) > 0 {
		rows, err := q.WeaveGetConceptListNamesByIDs(ctx, mapKeys(conceptListIDs))
		if err == nil {
			for _, row := range rows {
				name := parseTranslations(row.UiName)
				names[row.ID] = name
				if row.SemanticID != nil {
					names[*row.SemanticID] = name
				}
			}
		}
	}

	return names
}

func (h *Handler) searchCollections(w http.ResponseWriter, r *http.Request, q *sqlcgen.Queries, params domain.SearchParams) {
	ctx := r.Context()

	pool, ok := h.pool()
	if !ok {
		writeAPIError(w, "search not available", http.StatusInternalServerError)
		return
	}

	targets, err := resolveProjectTargets(ctx, pool, params.ProjectID, params.Scope)
	if err != nil {
		h.logger.Error("resolve project targets failed", "err", err, "project_id", params.ProjectID)
		writeAPIError(w, "search query failed", http.StatusInternalServerError)
		return
	}

	linked := h.linkedSet(ctx, q, "collection", params.TargetID)
	adoptedOrigins, err := weavepkg.AdoptionOriginsForProject(ctx, h.weave, params.ProjectID, "collection")
	if err != nil {
		h.logger.Warn("load collection adoption origins failed", "err", err, "project_id", params.ProjectID)
		adoptedOrigins = map[string]domain.Origin{}
	}
	forkedOrigins, err := weavepkg.ForkOriginsForProject(ctx, h.weave, params.ProjectID, "collection")
	if err != nil {
		h.logger.Warn("load collection fork origins failed", "err", err, "project_id", params.ProjectID)
		forkedOrigins = map[string]domain.Origin{}
	}

	candidates := make([]collectionSearchCandidate, 0)
	for _, target := range targets {
		targetParams := params
		targetFieldIDs, err := resolveTargetPathFieldIDs(ctx, pool, target, targetParams)
		if err != nil {
			h.logger.Error("resolve target path field ids failed", "err", err, "project_id", params.ProjectID, "target_id", target.ProjectID, "target_version", target.Version)
			writeAPIError(w, "search query failed", http.StatusInternalServerError)
			return
		}
		if targetFieldIDs != nil && len(targetFieldIDs) == 0 {
			continue
		}
		if targetFieldIDs != nil {
			targetParams.PathFieldIDs = targetFieldIDs
		} else {
			targetParams.PathFieldIDs = []string{}
		}

		var targetRows []collectionSearchCandidate
		if strings.TrimSpace(target.Version) == "" {
			rows, err := q.WeaveSearchCollections(ctx, sqlcgen.WeaveSearchCollectionsParams{
				SortBy:         "relevance",
				ResultOffset:   0,
				ResultLimit:    5000,
				ProjectID:      target.ProjectID,
				Scope:          "project",
				Search:         targetParams.Query,
				PathLocalName:  targetParams.PathLocalName,
				PathPrefix:     targetParams.PathPrefix,
				OntologyClass:  targetParams.OntologyClass,
				OntologyPrefix: targetParams.OntologyPrefix,
				PathFieldIds:   targetParams.PathFieldIDs,
			})
			if err != nil {
				h.logger.Error("search live collections query failed", "err", err, "project_id", target.ProjectID)
				writeAPIError(w, "search query failed", http.StatusInternalServerError)
				return
			}
			targetRows = make([]collectionSearchCandidate, 0, len(rows))
			for _, row := range rows {
				targetRows = append(targetRows, liveCollectionCandidateFromRow(params.ProjectID, row))
			}
		} else {
			targetRows, err = h.searchArchivedCollections(ctx, pool, target, targetParams, params.ProjectID)
			if err != nil {
				h.logger.Error("search archived collections query failed", "err", err, "project_id", target.ProjectID, "version", target.Version)
				writeAPIError(w, "search query failed", http.StatusInternalServerError)
				return
			}
		}
		candidates = append(candidates, targetRows...)
	}

	items := buildCollectionSearchResults(candidates, linked, adoptedOrigins, forkedOrigins, params.ProjectID)
	sortSearchResults(items, params.Sort)
	total := len(items)
	items = paginateSearchResults(items, params.Offset, params.Limit)

	writeJSON(w, http.StatusOK, domain.SearchResponse{
		Items:      items,
		TotalCount: total,
	})
}

func (h *Handler) linkedSet(ctx context.Context, q *sqlcgen.Queries, entityType, targetID string) map[string]bool {
	if targetID == "" {
		return nil
	}

	ids, err := q.WeaveGetLinkedFieldIDs(ctx, sqlcgen.WeaveGetLinkedFieldIDsParams{
		EntityType: entityType,
		EntityID:   targetID,
	})
	if err != nil {
		h.logger.Warn("failed to load linked field ids", "err", err, "target_id", targetID)
		return nil
	}

	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}

	return set
}

func parseSearchParams(r *http.Request, projectID string) domain.SearchParams {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	scope := normalizeScope(r.URL.Query().Get("scope"))

	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "relevance"
	}

	pathDepth, _ := strconv.Atoi(r.URL.Query().Get("path_depth"))

	ontologyClassRaw := r.URL.Query().Get("ontology_class")
	var ontologyClass, ontologyPrefix string
	if idx := strings.IndexByte(ontologyClassRaw, ':'); idx >= 0 {
		ontologyPrefix = ontologyClassRaw[:idx]
		ontologyClass = ontologyClassRaw[idx+1:]
	} else {
		ontologyClass = ontologyClassRaw
	}

	pathContainsRaw := r.URL.Query().Get("path_contains")
	var pathStartsWith, pathLocalName, pathPrefix string
	if strings.Contains(pathContainsRaw, "->") {
		pathStartsWith = pathContainsRaw
	} else {
		if idx := strings.IndexByte(pathContainsRaw, ':'); idx >= 0 {
			pathPrefix = pathContainsRaw[:idx]
			pathLocalName = pathContainsRaw[idx+1:]
		} else {
			pathLocalName = pathContainsRaw
		}
	}

	if v := r.URL.Query().Get("path_starts_with"); v != "" {
		pathStartsWith = v
	}

	return domain.SearchParams{
		ProjectID:      projectID,
		Type:           r.URL.Query().Get("type"),
		Query:          r.URL.Query().Get("q"),
		Scope:          scope,
		PathPrefix:     pathPrefix,
		PathLocalName:  pathLocalName,
		PathStartsWith: pathStartsWith,
		PathEndsWith:   r.URL.Query().Get("path_ends_with"),
		PathDepth:      pathDepth,
		OntologyClass:  ontologyClass,
		OntologyPrefix: ontologyPrefix,
		Category:       r.URL.Query().Get("category"),
		ExpectedValue:  r.URL.Query().Get("expected_value_type"),
		TargetID:       r.URL.Query().Get("target_id"),
		Sort:           sort,
		Limit:          limit,
		Offset:         offset,
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parseTranslations(data []byte) domain.Translations {
	if len(data) == 0 {
		return nil
	}

	var t domain.Translations
	if err := json.Unmarshal(data, &t); err != nil {
		return nil
	}

	return t
}

func parsePathElements(data []byte) []domain.PathElement {
	if len(data) == 0 {
		return nil
	}

	var elements []domain.PathElement
	if err := json.Unmarshal(data, &elements); err != nil {
		return nil
	}

	return elements
}

func parsePathElement(data []byte) *domain.PathElement {
	if len(data) == 0 {
		return nil
	}

	var scope domain.PathElement
	if err := json.Unmarshal(data, &scope); err != nil {
		return nil
	}

	return &scope
}

func mapKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func toInt(v any) int {
	switch n := v.(type) {
	case nil:
		return 0
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case *int32:
		if n == nil {
			return 0
		}
		return int(*n)
	case *int64:
		if n == nil {
			return 0
		}
		return int(*n)
	default:
		return 0
	}
}

type fieldSearchCandidate struct {
	ID                string
	SemanticID        *string
	SystemName        *string
	UIName            []byte
	Description       []byte
	OntologyScope     []byte
	OntologyPath      *string
	PathElements      []byte
	ExpectedValueType *string
	ProjectID         string
	AdoptionCount     int
	IsLocal           bool
	Version           string
}

type collectionSearchCandidate struct {
	ID            string
	SystemName    *string
	UIName        []byte
	Description   []byte
	OntologyScope []byte
	ProjectID     string
	AdoptionCount int
	IsLocal       bool
	Version       string
}

func liveFieldCandidateFromRow(rootProjectID string, row sqlcgen.WeaveSearchFieldsRow) fieldSearchCandidate {
	isLocal := row.ProjectID == rootProjectID
	return fieldSearchCandidate{
		ID:                row.ID,
		SemanticID:        row.SemanticID,
		SystemName:        row.SystemName,
		UIName:            row.UiName,
		Description:       row.Description,
		OntologyScope:     row.OntologyScope,
		OntologyPath:      row.OntologyPath,
		PathElements:      row.PathElements,
		ExpectedValueType: row.ExpectedValueType,
		ProjectID:         row.ProjectID,
		AdoptionCount:     toInt(row.AdoptionCount),
		IsLocal:           isLocal,
	}
}

func liveCollectionCandidateFromRow(rootProjectID string, row sqlcgen.WeaveSearchCollectionsRow) collectionSearchCandidate {
	isLocal := row.ProjectID == rootProjectID
	return collectionSearchCandidate{
		ID:            row.ID,
		SystemName:    row.SystemName,
		UIName:        row.UiName,
		Description:   row.Description,
		OntologyScope: row.OntologyScope,
		ProjectID:     row.ProjectID,
		AdoptionCount: toInt(row.AdoptionCount),
		IsLocal:       isLocal,
	}
}

func buildFieldSearchResults(candidates []fieldSearchCandidate, refMap map[string]fieldExpectedRefs, linked map[string]bool, adoptedOrigins, forkedOrigins map[string]domain.Origin, rootProjectID string) []domain.SearchResult {
	seen := make(map[string]domain.SearchResult)
	order := make([]string, 0, len(candidates))
	for _, row := range candidates {
		sid := derefStr(row.SemanticID)
		refs := refMap[fieldRefKey(row.Version, row.ID)]
		result := domain.SearchResult{
			ID:                          row.ID,
			SemanticID:                  sid,
			SystemName:                  derefStr(row.SystemName),
			UIName:                      parseTranslations(row.UIName),
			Description:                 parseTranslations(row.Description),
			OntologyScope:               parsePathElement(row.OntologyScope),
			OntologyPath:                derefStr(row.OntologyPath),
			PathElements:                parsePathElements(row.PathElements),
			ExpectedValueType:           derefStr(row.ExpectedValueType),
			ExpectedResourceModels:      refs.resourceModels,
			ExpectedCollectionModels:    refs.collectionModels,
			ExpectedConceptLists:        refs.conceptLists,
			ExpectedResourceModelRefs:   refs.resourceModelRefs,
			ExpectedCollectionModelRefs: refs.collectionModelRefs,
			ExpectedConceptListRefs:     refs.conceptListRefs,
			ProjectID:                   row.ProjectID,
			Origin:                      weavepkg.ResolveReuseOrigin(forkedOrigins, adoptedOrigins, rootProjectID, row.ProjectID, domain.ParseSemanticID(sid).ID()),
			IsLinked:                    linked[row.ID],
			AdoptionCount:               row.AdoptionCount,
			URL:                         domain.ParseSemanticID(sid).URL(),
		}
		if existing, ok := seen[row.ID]; ok {
			if existing.ProjectID == rootProjectID || !row.IsLocal {
				continue
			}
		} else {
			order = append(order, row.ID)
		}
		seen[row.ID] = result
	}

	items := make([]domain.SearchResult, 0, len(order))
	for _, id := range order {
		items = append(items, seen[id])
	}
	return items
}

func buildCollectionSearchResults(candidates []collectionSearchCandidate, linked map[string]bool, adoptedOrigins, forkedOrigins map[string]domain.Origin, rootProjectID string) []domain.SearchResult {
	seen := make(map[string]domain.SearchResult)
	order := make([]string, 0, len(candidates))
	for _, row := range candidates {
		sid := domain.ParseSemanticID(row.ID)
		result := domain.SearchResult{
			ID:            row.ID,
			SemanticID:    row.ID,
			SystemName:    derefStr(row.SystemName),
			UIName:        parseTranslations(row.UIName),
			Description:   parseTranslations(row.Description),
			OntologyScope: parsePathElement(row.OntologyScope),
			ProjectID:     row.ProjectID,
			Origin:        weavepkg.ResolveReuseOrigin(forkedOrigins, adoptedOrigins, rootProjectID, row.ProjectID, sid.ID()),
			IsLinked:      linked[row.ID],
			AdoptionCount: row.AdoptionCount,
			URL:           sid.URL(),
		}
		if existing, ok := seen[row.ID]; ok {
			if existing.ProjectID == rootProjectID || !row.IsLocal {
				continue
			}
		} else {
			order = append(order, row.ID)
		}
		seen[row.ID] = result
	}

	items := make([]domain.SearchResult, 0, len(order))
	for _, id := range order {
		items = append(items, seen[id])
	}
	return items
}

func sortSearchResults(items []domain.SearchResult, sortBy string) {
	nameFor := func(item domain.SearchResult) string {
		if item.UIName == nil {
			return ""
		}
		if v := item.UIName["en"]; v != "" {
			return strings.ToLower(v)
		}
		for _, v := range item.UIName {
			return strings.ToLower(v)
		}
		return ""
	}
	isLocal := func(item domain.SearchResult) bool {
		return item.Origin.Kind == domain.OriginOwn || item.Origin.Kind == domain.OriginForked
	}

	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		switch sortBy {
		case "adoption":
			if left.AdoptionCount != right.AdoptionCount {
				return left.AdoptionCount > right.AdoptionCount
			}
			return left.SemanticID < right.SemanticID
		case "name":
			ln, rn := nameFor(left), nameFor(right)
			if ln != rn {
				return ln < rn
			}
			return left.SemanticID < right.SemanticID
		default:
			if isLocal(left) != isLocal(right) {
				return isLocal(left)
			}
			if left.AdoptionCount != right.AdoptionCount {
				return left.AdoptionCount > right.AdoptionCount
			}
			return left.SemanticID < right.SemanticID
		}
	})
}

func paginateSearchResults(items []domain.SearchResult, offset, limit int) []domain.SearchResult {
	if offset >= len(items) {
		return []domain.SearchResult{}
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + limit
	if limit <= 0 || end > len(items) {
		end = len(items)
	}
	return append([]domain.SearchResult(nil), items[offset:end]...)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeAPIError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   message,
		"success": false,
	})
}
