package ontology

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
	parsedontology "github.com/pletka-io/pletka/pkg/weave/ontology/rdf"
)

// Handler exposes the slice's HTTP surface. Read-only at this stage
// (Phase A step 3c) — admin CRUD endpoints land in Phase B.
type Handler struct {
	svc          *Service
	log          *slog.Logger
	languages    []formschema.LanguageInfo
	langResolver func(*http.Request) string
}

// NewHandler wires a Handler. nil log → slog.Default.
func NewHandler(svc *Service, log *slog.Logger, languages []formschema.LanguageInfo, langResolver func(*http.Request) string) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, log: log, languages: languages, langResolver: langResolver}
}

// ---------------------------------------------------------------------------
// Family read endpoints
// ---------------------------------------------------------------------------

func (h *Handler) ListFamilies(w http.ResponseWriter, r *http.Request) {
	families, err := h.svc.ListFamilies(r.Context())
	if err != nil {
		h.log.Error("list families", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list families")
		return
	}
	writeJSON(w, http.StatusOK, families)
}

func (h *Handler) OptionsFamilies(w http.ResponseWriter, r *http.Request) {
	families, err := h.svc.ListFamilies(r.Context())
	if err != nil {
		h.log.Error("list family options", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list families")
		return
	}
	opts := make([]formschema.SelectOption, 0, len(families))
	for _, family := range families {
		opts = append(opts, formschema.SelectOption{
			Value: family.ID,
			Label: domain.Translations{"en": family.Name},
		})
	}
	writeJSON(w, http.StatusOK, opts)
}

func (h *Handler) ListFamiliesData(w http.ResponseWriter, r *http.Request) {
	page, perPage, ok := parsePaging(w, r)
	if !ok {
		return
	}
	out, err := h.svc.BrowseFamilies(r.Context(), FamilyBrowseInput{
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort_by"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		h.log.Error("browse families", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to browse families")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out.Items,
		"total": out.Total,
	})
}

func (h *Handler) GetFamily(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "familyID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "family id required")
		return
	}
	f, err := h.svc.GetFamily(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, r, err, "family")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// ---------------------------------------------------------------------------
// Ontology read endpoints
// ---------------------------------------------------------------------------

func (h *Handler) ListOntologies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if q := r.URL.Query().Get("q"); q != "" {
		limit := parseLimit(r, 50)
		out, err := h.svc.SearchOntologies(ctx, q, limit)
		if err != nil {
			h.log.Error("search ontologies", "err", err)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to search ontologies")
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if familyID := r.URL.Query().Get("family_id"); familyID != "" {
		out, err := h.svc.ListOntologiesByFamily(ctx, familyID)
		if err != nil {
			h.log.Error("list ontologies by family", "err", err)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list ontologies")
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	out, err := h.svc.ListOntologies(ctx)
	if err != nil {
		h.log.Error("list ontologies", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list ontologies")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) OptionsOntologies(w http.ResponseWriter, r *http.Request) {
	ontologies, err := h.svc.ListOntologies(r.Context())
	if err != nil {
		h.log.Error("list ontology options", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list ontologies")
		return
	}
	filterType := strings.TrimSpace(r.URL.Query().Get("ontology_type"))
	opts := make([]formschema.SelectOption, 0, len(ontologies))
	for _, ontology := range ontologies {
		if filterType != "" && string(ontology.OntologyType) != filterType {
			continue
		}
		label := ontology.Name
		if ontology.Prefix != "" {
			label = ontology.Prefix + " - " + ontology.Name
		}
		opts = append(opts, formschema.SelectOption{
			Value:  ontology.ID,
			Label:  domain.Translations{"en": label},
			Status: string(ontology.OntologyType),
		})
	}
	writeJSON(w, http.StatusOK, opts)
}

func (h *Handler) ListOntologiesData(w http.ResponseWriter, r *http.Request) {
	page, perPage, ok := parsePaging(w, r)
	if !ok {
		return
	}
	out, err := h.svc.BrowseOntologies(r.Context(), OntologyBrowseInput{
		Search:       r.URL.Query().Get("search"),
		FamilyID:     r.URL.Query().Get("family_id"),
		OntologyType: r.URL.Query().Get("ontology_type"),
		SortBy:       r.URL.Query().Get("sort_by"),
		Page:         page,
		PerPage:      perPage,
	})
	if err != nil {
		h.log.Error("browse ontologies", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to browse ontologies")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out.Items,
		"total": out.Total,
	})
}

func (h *Handler) GetOntology(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ontologyID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "ontology id required")
		return
	}
	o, err := h.svc.GetOntology(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, r, err, "ontology")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) ListExtensions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ontologyID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "ontology id required")
		return
	}
	out, err := h.svc.ListOntologyExtensions(r.Context(), id)
	if err != nil {
		h.log.Error("list extensions", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list extensions")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Version read endpoints
// ---------------------------------------------------------------------------

func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ontologyID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "ontology id required")
		return
	}
	out, err := h.svc.VersionListWithUsage(r.Context(), id)
	if err != nil {
		h.log.Error("list versions", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list versions")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "versionID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "version id required")
		return
	}
	v, err := h.svc.GetVersion(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, r, err, "version")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// ---------------------------------------------------------------------------
// Class / property read endpoints (version-scoped)
// ---------------------------------------------------------------------------

func (h *Handler) ListClasses(w http.ResponseWriter, r *http.Request) {
	versionID := chi.URLParam(r, "versionID")
	if versionID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "version id required")
		return
	}
	if q := r.URL.Query().Get("q"); q != "" {
		limit := parseLimit(r, 50)
		out, err := h.svc.SearchClasses(r.Context(), versionID, q, limit)
		if err != nil {
			h.log.Error("search classes", "err", err)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to search classes")
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	out, err := h.svc.ListClasses(r.Context(), versionID)
	if err != nil {
		h.log.Error("list classes", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list classes")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) ListProperties(w http.ResponseWriter, r *http.Request) {
	versionID := chi.URLParam(r, "versionID")
	if versionID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "version id required")
		return
	}
	if q := r.URL.Query().Get("q"); q != "" {
		limit := parseLimit(r, 50)
		out, err := h.svc.SearchProperties(r.Context(), versionID, q, limit)
		if err != nil {
			h.log.Error("search properties", "err", err)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to search properties")
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	out, err := h.svc.ListProperties(r.Context(), versionID)
	if err != nil {
		h.log.Error("list properties", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list properties")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Autocomplete + ontology-labels endpoints (Phase D).
//
// Path-builder + label maps drive the OntologyPathBuilder Svelte widget
// and the frontend ontology-labels client. The endpoints keep their
// stable URLs (/api/v1/ontology/autocomplete and
// /api/projects/{projectID}/ontology-labels) so the frontend doesn't
// need to migrate.
// ---------------------------------------------------------------------------

func (h *Handler) Autocomplete(w http.ResponseWriter, r *http.Request) {
	var req autocomplete.Request
	if !decodeJSON(w, r, &req) {
		return
	}
	// Set the source override and super-admin flag so DispatchEngine can honour
	// per-request mode overrides when AllowPerRequestOverride is configured.
	req.Source = r.URL.Query().Get("source")
	if snap := auth.FromContext(r.Context()); snap != nil {
		req.SuperAdmin = snap.IsSuperAdmin
	}
	suggestions, err := h.svc.GetSuggestions(r.Context(), req)
	if err != nil {
		h.log.Error("autocomplete", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "autocomplete failed")
		return
	}
	if suggestions == nil {
		suggestions = []autocomplete.Suggestion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}

func (h *Handler) OntologyLabels(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "project id required")
		return
	}
	lang := h.lang(r)
	out, err := h.svc.OntologyLabels(r.Context(), projectID, lang)
	if err != nil {
		h.log.Error("ontology labels", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "labels lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Form / list schema endpoints — drive the entity-list + entity-form
// Svelte islands in the admin UI.
// ---------------------------------------------------------------------------

func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildOntologyListSchema(h.lang(r), h.languages))
}

func (h *Handler) EntityListSchema(w http.ResponseWriter, r *http.Request) {
	familyOptions := h.familyFilterOptions(r.Context())
	writeJSON(w, http.StatusOK, BuildOntologyEntityListSchema(h.lang(r), h.languages, familyOptions))
}

func (h *Handler) FamilyListSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildFamilyListSchema(h.lang(r), h.languages))
}

func (h *Handler) FamilyEntityListSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildFamilyEntityListSchema(h.lang(r), h.languages))
}

func (h *Handler) VersionListSchema(w http.ResponseWriter, r *http.Request) {
	ontologyID := chi.URLParam(r, "ontologyID")
	if ontologyID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "ontology id required")
		return
	}
	writeJSON(w, http.StatusOK, BuildVersionListSchema(ontologyID, h.lang(r), h.languages))
}

func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = formschema.ModeCreate
	}
	var existing *domain.Ontology
	if (mode == formschema.ModeEdit || mode == formschema.ModeView) && r.URL.Query().Get("entity_id") != "" {
		o, err := h.svc.GetOntology(r.Context(), r.URL.Query().Get("entity_id"))
		if err == nil {
			existing = o
		}
	}
	writeJSON(w, http.StatusOK, BuildOntologyFormSchema(mode, existing, h.lang(r), h.languages))
}

func (h *Handler) FamilyFormSchema(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = formschema.ModeCreate
	}
	var existing *domain.OntologyFamily
	if (mode == formschema.ModeEdit || mode == formschema.ModeView) && r.URL.Query().Get("entity_id") != "" {
		f, err := h.svc.GetFamily(r.Context(), r.URL.Query().Get("entity_id"))
		if err == nil {
			existing = f
		}
	}
	// Load every family so the parent_family_id select can list them as
	// options. Failure here degrades to an empty list (root-only) rather
	// than 500'ing the form schema endpoint.
	families, err := h.svc.ListFamilies(r.Context())
	if err != nil {
		h.log.Warn("ontology: list families for form schema", "err", err)
		families = nil
	}
	writeJSON(w, http.StatusOK, BuildFamilyFormSchema(mode, existing, families, h.lang(r), h.languages))
}

func (h *Handler) VersionFormSchema(w http.ResponseWriter, r *http.Request) {
	var existing *domain.OntologyVersion
	if r.URL.Query().Get("entity_id") != "" {
		v, err := h.svc.GetVersion(r.Context(), r.URL.Query().Get("entity_id"))
		if err == nil {
			existing = v
		}
	}
	writeJSON(w, http.StatusOK, BuildVersionFormSchema(existing, h.lang(r), h.languages))
}

func (h *Handler) AdminLandingPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := h.lang(r)
	model, err := h.svc.FamilyLandingPageModel(r.Context(), lang)
	if err != nil {
		h.log.Error("admin ontology landing page schema", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to build ontology page schema")
		return
	}
	writeJSON(w, http.StatusOK, BuildAdminFamilyLandingPageSchema(model, lang, h.languages))
}

func (h *Handler) AdminFamilyPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := h.lang(r)
	familyID := chi.URLParam(r, "familyID")
	family, err := h.svc.GetFamily(r.Context(), familyID)
	if err != nil {
		h.writeServiceError(w, r, err, "family")
		return
	}
	model, err := h.svc.FamilyDetailPageModel(r.Context(), family.Slug, r.URL.Query().Get("tab"), lang)
	if err != nil {
		h.writeServiceError(w, r, err, "family")
		return
	}
	writeJSON(w, http.StatusOK, BuildAdminFamilyDetailPageSchema(model, lang, h.languages))
}

func (h *Handler) AdminOntologyPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := h.lang(r)
	model, err := h.svc.OntologyDetailPageModelByID(r.Context(), chi.URLParam(r, "ontologyID"), lang)
	if err != nil {
		h.writeServiceError(w, r, err, "ontology")
		return
	}
	writeJSON(w, http.StatusOK, BuildOntologyDetailPageSchema(model, lang, h.languages, true))
}

func (h *Handler) AdminVersionPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := h.lang(r)
	model, err := h.svc.VersionDetailPageModelByID(r.Context(), chi.URLParam(r, "versionID"), r.URL.Query().Get("tab"), lang)
	if err != nil {
		h.writeServiceError(w, r, err, "version")
		return
	}
	writeJSON(w, http.StatusOK, BuildVersionDetailPageSchema(model, lang, h.languages, true))
}

func (h *Handler) AdminImportVersionPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := h.lang(r)
	model, err := h.svc.OntologyDetailPageModelByID(r.Context(), chi.URLParam(r, "ontologyID"), lang)
	if err != nil {
		h.writeServiceError(w, r, err, "ontology")
		return
	}
	writeJSON(w, http.StatusOK, BuildAdminImportVersionPageSchema(model, lang, h.languages))
}

func (h *Handler) ProbeImportVersion(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	input, ok := decodeImportUpload(w, r, chi.URLParam(r, "ontologyID"))
	if !ok {
		return
	}
	probe, _, err := h.svc.ProbeImportVersion(r.Context(), input)
	if err != nil {
		h.writeServiceError(w, r, err, "version import probe")
		return
	}
	writeJSON(w, http.StatusOK, probe)
}

func (h *Handler) ImportVersionUpload(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	input, ok := decodeImportUpload(w, r, chi.URLParam(r, "ontologyID"))
	if !ok {
		return
	}
	probe, importInput, err := h.svc.ProbeImportVersion(r.Context(), input)
	if err != nil {
		h.writeServiceError(w, r, err, "version import")
		return
	}
	if !probe.CanImport {
		if len(probe.MissingNamespaces) > 0 {
			writeValidationError(w, fieldError("rdf_file", "RDF file references namespaces without prefix bindings: "+formatMissingNamespacesForValidation(probe.MissingNamespaces)))
			return
		}
		writeValidationError(w, fieldError("rdf_file", "RDF file did not contain importable classes or properties"))
		return
	}
	version, err := h.svc.ImportVersionWithOptions(r.Context(), importInput, ImportVersionOptions{})
	if err != nil {
		h.writeServiceError(w, r, err, "version import")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"probe":   probe,
		"version": version,
	})
}

func formatMissingNamespacesForValidation(missing []parsedontology.MissingNamespace) string {
	parts := make([]string, 0, len(missing))
	for _, item := range missing {
		if item.Namespace == "" {
			continue
		}
		parts = append(parts, item.Namespace)
	}
	if len(parts) == 0 {
		return "unknown namespace"
	}
	return strings.Join(parts, ", ")
}

func (h *Handler) lang(r *http.Request) string {
	if h.langResolver != nil {
		if l := h.langResolver(r); l != "" {
			return l
		}
	}
	if l := r.URL.Query().Get("lang"); l != "" {
		return l
	}
	return "en"
}

const ontologyImportMaxBytes int64 = 25 << 20

func decodeImportUpload(w http.ResponseWriter, r *http.Request, ontologyID string) (ImportUploadInput, bool) {
	if ontologyID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "ontology id required")
		return ImportUploadInput{}, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, ontologyImportMaxBytes)
	if err := r.ParseMultipartForm(ontologyImportMaxBytes); err != nil {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "invalid multipart upload")
		return ImportUploadInput{}, false
	}
	file, header, err := r.FormFile("rdf_file")
	if err != nil {
		writeValidationError(w, fieldError("rdf_file", "required"))
		return ImportUploadInput{}, false
	}
	defer file.Close() //nolint:errcheck
	content, err := io.ReadAll(file)
	if err != nil {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "failed to read RDF file")
		return ImportUploadInput{}, false
	}
	if len(content) == 0 {
		writeValidationError(w, fieldError("rdf_file", "empty file"))
		return ImportUploadInput{}, false
	}
	return ImportUploadInput{
		OntologyID:             ontologyID,
		Filename:               header.Filename,
		Content:                content,
		VersionString:          strings.TrimSpace(r.FormValue("version_string")),
		CompatibleBaseVersions: parseImportCSV(r.FormValue("compatible_base_versions")),
		SetActive:              parseImportBool(r.FormValue("set_active")),
	}, true
}

func parseImportCSV(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseImportBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Family writes (super-admin only)
// ---------------------------------------------------------------------------

type familyPayload struct {
	Slug           string              `json:"slug"`
	Name           string              `json:"name"`
	Description    domain.Translations `json:"description,omitempty"`
	ParentFamilyID *string             `json:"parent_family_id,omitempty"`
	HomepageURL    string              `json:"homepage_url,omitempty"`
	Icon           string              `json:"icon,omitempty"`
	DisplayOrder   int64               `json:"display_order,omitempty"`
}

func (h *Handler) CreateFamily(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	var p familyPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	if err := validateFamily(p); err != nil {
		writeValidationError(w, err)
		return
	}
	f, err := h.svc.CreateFamily(r.Context(), CreateFamilyInput{
		Slug:           p.Slug,
		Name:           p.Name,
		Description:    p.Description,
		ParentFamilyID: p.ParentFamilyID,
		HomepageURL:    p.HomepageURL,
		Icon:           p.Icon,
		DisplayOrder:   p.DisplayOrder,
	})
	if err != nil {
		h.writeServiceError(w, r, err, "family")
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (h *Handler) UpdateFamily(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "familyID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "family id required")
		return
	}
	var p familyPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	if err := validateFamily(p); err != nil {
		writeValidationError(w, err)
		return
	}
	f, err := h.svc.UpdateFamily(r.Context(), id, UpdateFamilyInput{
		Slug:           p.Slug,
		Name:           p.Name,
		Description:    p.Description,
		ParentFamilyID: p.ParentFamilyID,
		HomepageURL:    p.HomepageURL,
		Icon:           p.Icon,
		DisplayOrder:   p.DisplayOrder,
	})
	if err != nil {
		h.writeServiceError(w, r, err, "family")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *Handler) DeleteFamily(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "familyID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "family id required")
		return
	}
	if err := h.svc.DeleteFamily(r.Context(), id); err != nil {
		h.writeServiceError(w, r, err, "family")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Ontology writes (super-admin only)
// ---------------------------------------------------------------------------

type ontologyPayload struct {
	Prefix            string              `json:"prefix"`
	Namespace         string              `json:"namespace"`
	Name              string              `json:"name"`
	Description       domain.Translations `json:"description,omitempty"`
	FamilyID          *string             `json:"family_id,omitempty"`
	OntologyType      domain.OntologyType `json:"ontology_type,omitempty"`
	ExtendsOntologyID *string             `json:"extends_ontology_id,omitempty"`
	HomepageURL       string              `json:"homepage_url,omitempty"`
	SourceURL         string              `json:"source_url,omitempty"`
}

func (h *Handler) CreateOntology(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	var p ontologyPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	if err := validateOntology(p); err != nil {
		writeValidationError(w, err)
		return
	}
	createdBy := actorIDFromCtx(r)
	o, err := h.svc.CreateOntology(r.Context(), CreateOntologyInput{
		Prefix:            p.Prefix,
		Namespace:         p.Namespace,
		Name:              p.Name,
		Description:       p.Description,
		FamilyID:          p.FamilyID,
		OntologyType:      p.OntologyType,
		ExtendsOntologyID: p.ExtendsOntologyID,
		HomepageURL:       p.HomepageURL,
		SourceURL:         p.SourceURL,
		CreatedByID:       createdBy,
	})
	if err != nil {
		h.writeServiceError(w, r, err, "ontology")
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (h *Handler) UpdateOntology(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "ontologyID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "ontology id required")
		return
	}
	var p ontologyPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	if err := validateOntology(p); err != nil {
		writeValidationError(w, err)
		return
	}
	o, err := h.svc.UpdateOntology(r.Context(), id, UpdateOntologyInput{
		Prefix:            p.Prefix,
		Namespace:         p.Namespace,
		Name:              p.Name,
		Description:       p.Description,
		FamilyID:          p.FamilyID,
		OntologyType:      p.OntologyType,
		ExtendsOntologyID: p.ExtendsOntologyID,
		HomepageURL:       p.HomepageURL,
		SourceURL:         p.SourceURL,
	})
	if err != nil {
		h.writeServiceError(w, r, err, "ontology")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) DeleteOntology(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "ontologyID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "ontology id required")
		return
	}
	if err := h.svc.DeleteOntology(r.Context(), id); err != nil {
		h.writeServiceError(w, r, err, "ontology")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Version writes (super-admin only) — admin metadata edit, lifecycle, and
// browser RDF upload. The CLI `ontology-import import-v2` uses the same
// service/import path for manifest-driven bulk imports.
// ---------------------------------------------------------------------------

type versionMetadataPayload struct {
	VersionString              string              `json:"version_string"`
	CompatibleBaseVersions     []string            `json:"compatible_base_versions,omitempty"`
	CompatibleBaseVersionsText *string             `json:"compatible_base_versions_text,omitempty"`
	OntologyURI                string              `json:"ontology_uri,omitempty"`
	VersionIRI                 string              `json:"version_iri,omitempty"`
	VersionInfo                domain.Translations `json:"version_info,omitempty"`
	ImportedOntologies         []string            `json:"imported_ontologies,omitempty"`
	ImportedOntologiesText     *string             `json:"imported_ontologies_text,omitempty"`
	OntologyLabel              domain.Translations `json:"ontology_label,omitempty"`
	OntologyComment            domain.Translations `json:"ontology_comment,omitempty"`
	OntologyMetadata           json.RawMessage     `json:"ontology_metadata,omitempty"`
}

func (h *Handler) UpdateVersion(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "versionID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "version id required")
		return
	}
	var p versionMetadataPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	p.VersionString = strings.TrimSpace(p.VersionString)
	if p.VersionString == "" {
		writeValidationError(w, fieldError("version_string", "required"))
		return
	}
	existing, err := h.svc.GetVersion(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, r, err, "version")
		return
	}
	if p.CompatibleBaseVersionsText != nil {
		p.CompatibleBaseVersions = parseImportCSV(*p.CompatibleBaseVersionsText)
	} else if p.CompatibleBaseVersions == nil {
		p.CompatibleBaseVersions = existing.CompatibleBaseVersions
	}
	if p.ImportedOntologiesText != nil {
		p.ImportedOntologies = parseImportCSV(*p.ImportedOntologiesText)
	} else if p.ImportedOntologies == nil {
		p.ImportedOntologies = existing.ImportedOntologies
	}
	if p.VersionInfo == nil {
		p.VersionInfo = existing.VersionInfo
	}
	if p.OntologyLabel == nil {
		p.OntologyLabel = existing.OntologyLabel
	}
	if p.OntologyComment == nil {
		p.OntologyComment = existing.OntologyComment
	}
	if p.OntologyMetadata == nil {
		p.OntologyMetadata = existing.OntologyMetadata
	}
	v, err := h.svc.UpdateVersionMetadata(r.Context(), id, UpdateVersionMetadataInput{
		VersionString:          p.VersionString,
		CompatibleBaseVersions: p.CompatibleBaseVersions,
		OntologyURI:            p.OntologyURI,
		VersionIRI:             p.VersionIRI,
		VersionInfo:            p.VersionInfo,
		ImportedOntologies:     p.ImportedOntologies,
		OntologyLabel:          p.OntologyLabel,
		OntologyComment:        p.OntologyComment,
		OntologyMetadata:       p.OntologyMetadata,
	})
	if err != nil {
		h.writeServiceError(w, r, err, "version")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) SetActiveVersion(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	ontologyID := chi.URLParam(r, "ontologyID")
	versionID := chi.URLParam(r, "versionID")
	if ontologyID == "" || versionID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "ontology id and version id required")
		return
	}
	if err := h.svc.SetActiveVersion(r.Context(), ontologyID, versionID); err != nil {
		h.writeServiceError(w, r, err, "version")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	id := chi.URLParam(r, "versionID")
	if id == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "version id required")
		return
	}
	if err := h.svc.DeleteVersion(r.Context(), id); err != nil {
		h.writeServiceError(w, r, err, "version")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Admin autocomplete-cache endpoints
// ---------------------------------------------------------------------------

// DropAutocompleteCache handles POST /admin/autocomplete-cache/drop.
// Drops all live entries from the index cache and returns 204.
func (h *Handler) DropAutocompleteCache(w http.ResponseWriter, r *http.Request) {
	h.svc.DropAutocompleteCache()
	w.WriteHeader(http.StatusNoContent)
}

// AutocompleteCacheStats handles GET /admin/autocomplete-cache/stats.
// Returns a JSON array of cache entry snapshots.
func (h *Handler) AutocompleteCacheStats(w http.ResponseWriter, r *http.Request) {
	stats := h.svc.AutocompleteCacheStats()
	if stats == nil {
		stats = []autocomplete.EntryStat{}
	}
	writeJSON(w, http.StatusOK, stats)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parseLimit(r *http.Request, def int) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func parsePaging(w http.ResponseWriter, r *http.Request) (page int, perPage int, ok bool) {
	page = 1
	perPage = 25
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			errresp.Error(w, r, http.StatusBadRequest, "bad_request", "page must be a positive integer")
			return 0, 0, false
		}
		page = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("per_page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			errresp.Error(w, r, http.StatusBadRequest, "bad_request", "per_page must be a positive integer")
			return 0, 0, false
		}
		perPage = parsed
	}
	return page, perPage, true
}

func (h *Handler) familyFilterOptions(ctx context.Context) []formschema.FilterOption {
	families, err := h.svc.ListFamilies(ctx)
	if err != nil {
		return nil
	}
	options := make([]formschema.FilterOption, 0, len(families))
	for _, f := range families {
		options = append(options, formschema.FilterOption{
			Value: f.ID,
			Label: domain.Translations{"en": f.Name},
		})
	}
	return options
}

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error, label string) {
	if errors.Is(err, ErrNotFound) {
		errresp.Error(w, r, http.StatusNotFound, "not_found", label+" not found")
		return
	}
	if errors.Is(err, ErrCycle) {
		writeValidationError(w, fieldError("parent_family_id", "cannot set a parent that would create a cycle"))
		return
	}
	if errors.Is(err, ErrInUse) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   err.Error(),
			"message": label + " is in use; cannot delete",
		})
		return
	}
	h.log.Error(label+" error", "err", err)
	errresp.Error(w, r, http.StatusInternalServerError, "internal", "internal error")
}

// requireSuperAdmin gates write endpoints on auth.AuthSnapshot.IsSuperAdmin.
// Returns false (and writes 403) when the caller is not a super admin.
// The request must have already passed an auth-loading middleware that
// puts an AuthSnapshot on the context.
func (h *Handler) requireSuperAdmin(w http.ResponseWriter, r *http.Request) bool {
	snap := auth.FromContext(r.Context())
	if snap == nil || !snap.IsSuperAdmin {
		errresp.Error(w, r, http.StatusForbidden, "forbidden", "super_admin required")
		return false
	}
	return true
}

// actorIDFromCtx pulls the current caller's actor ID off the context,
// returning "" when no principal is set (anonymous / unauthenticated).
// Used to stamp CreatedByID on new ontology rows.
func actorIDFromCtx(r *http.Request) string {
	if p := auth.PrincipalFromContext(r.Context()); p != nil {
		return p.ActorID
	}
	return ""
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// fieldError builds a single-field validation error in the standard
// shape: {"errors": {"<field>": ["<msg>"]}}.
func fieldError(field, msg string) map[string][]string {
	return map[string][]string{field: {msg}}
}

func writeValidationError(w http.ResponseWriter, errs map[string][]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(map[string]any{"errors": errs}) // best-effort: headers already sent, write failure isn't actionable
}

func validateFamily(p familyPayload) map[string][]string {
	errs := map[string][]string{}
	if p.Slug == "" {
		errs["slug"] = []string{"required"}
	}
	if p.Name == "" {
		errs["name"] = []string{"required"}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func validateOntology(p ontologyPayload) map[string][]string {
	errs := map[string][]string{}
	if p.Prefix == "" {
		errs["prefix"] = []string{"required"}
	}
	if p.Namespace == "" {
		errs["namespace"] = []string{"required"}
	}
	if p.Name == "" {
		errs["name"] = []string{"required"}
	}
	if p.OntologyType != "" && p.OntologyType != domain.OntologyTypeBase && p.OntologyType != domain.OntologyTypeExtension {
		errs["ontology_type"] = []string{"must be 'base' or 'extension'"}
	}
	if p.OntologyType == domain.OntologyTypeExtension && (p.ExtendsOntologyID == nil || *p.ExtendsOntologyID == "") {
		errs["extends_ontology_id"] = []string{"required when ontology_type is 'extension'"}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
