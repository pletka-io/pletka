package organization

import (
	"encoding/json"
	"errors"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

type LangResolver func(*http.Request) string

type Handler struct {
	svc       *Service
	log       *slog.Logger
	languages []formschema.LanguageInfo
	lang      LangResolver
}

func NewHandler(svc *Service, log *slog.Logger, languages []formschema.LanguageInfo, lang LangResolver) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, log: log, languages: languages, lang: lang}
}

func (h *Handler) OrgCreateFormSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildCreateFormSchema(h.lang(r), h.languages))
}

func (h *Handler) CreateSelf(w http.ResponseWriter, r *http.Request) {
	principal := weaveauth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var in CreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeValidationErrors(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}
	org, err := h.svc.CreateSelfOrganization(r.Context(), principal.ActorID, in)
	if err != nil {
		var validation *ErrValidation
		if errors.As(err, &validation) {
			writeValidationErrors(w, validation.Fields)
			return
		}
		h.log.Error("create organization failed", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to create organization")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":   org.Slug,
		"slug": org.Slug,
	})
}

func (h *Handler) EntityListSchema(w http.ResponseWriter, r *http.Request) {
	snap := weaveauth.FromContext(r.Context())
	canCreate := snap != nil && !snap.IsAnonymous
	writeJSON(w, http.StatusOK, BuildEntityListSchema(canCreate, h.lang(r), h.languages))
}

func (h *Handler) Data(w http.ResponseWriter, r *http.Request) {
	page := 1
	perPage := 30
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && v > 0 {
		perPage = v
	}
	includePrivate, readable := ReadableOrgIDs(r.Context())
	res, err := h.svc.Browse(r.Context(), BrowseInput{
		Search:         r.URL.Query().Get("search"),
		CountryCodes:   parseCountryCodes(r.URL.Query().Get("country")),
		SortBy:         defaultSort(r.URL.Query().Get("sort_by")),
		SortDesc:       r.URL.Query().Get("sort_dir") == "desc",
		Page:           page,
		PerPage:        perPage,
		IncludePrivate: includePrivate,
		ReadableOrgIDs: readable,
	})
	if err != nil {
		h.log.Error("browse organizations failed", "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load organizations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"organizations": res.Items,
		"total":         res.Total,
		"page":          page,
		"per_page":      perPage,
	})
}

func (h *Handler) SettingsSchema(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "not found")
		return
	}
	writeJSON(w, http.StatusOK, BuildSettingsSchema(org, h.lang(r), h.languages))
}

func (h *Handler) GeneralFormSchema(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "not found")
		return
	}
	writeJSON(w, http.StatusOK, BuildGeneralFormSchema(org, h.lang(r), h.languages))
}

func (h *Handler) UpdateGeneral(w http.ResponseWriter, r *http.Request) {
	org := weaveauth.OrgFromContext(r.Context())
	if org == nil {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "not found")
		return
	}
	var in UpdateGeneralInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeValidationErrors(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}
	updated, err := h.svc.UpdateGeneral(r.Context(), org, in)
	if err != nil {
		var validation *ErrValidation
		if errors.As(err, &validation) {
			writeValidationErrors(w, validation.Fields)
			return
		}
		h.log.Error("update organization failed", "slug", org.Slug, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to update organization")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":   updated.Slug,
		"slug": updated.Slug,
	})
}

func defaultSort(v string) string {
	if v == "" {
		return "display_name"
	}
	return v
}

// CountriesFilter handles GET /orgs/filters/countries — returns the
// distinct country codes that have at least one org visible to the
// caller, in the FilterOption shape the schema's lazy-load picks up.
// Labels are localised against the same CountryOptions table the
// general-settings form uses, so the dropdown always shows
// human-readable country names.
func (h *Handler) CountriesFilter(w http.ResponseWriter, r *http.Request) {
	includePrivate, readable := ReadableOrgIDs(r.Context())
	codes, err := h.svc.ListVisibleCountries(r.Context(), includePrivate, readable)
	if err != nil {
		h.log.Error("list org countries failed", "err", err)
		apierror.Write(w, apierror.InternalWith("failed to load countries"))
		return
	}
	lang := h.lang(r)
	labels := formschema.CountryLabels(lang)
	options := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		label := labels[code]
		if label == "" {
			label = code
		}
		options = append(options, map[string]any{
			"value": code,
			"label": map[string]string{lang: label},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options})
}

// parseCountryCodes splits the comma-joined country query param into
// a clean code slice. Empty / whitespace produces nil so the SQL's
// cardinality(@country_codes) = 0 short-circuit applies.
func parseCountryCodes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// writeValidationErrors forwards to apierror.Write — see pkg/weave/apierror.
func writeValidationErrors(w http.ResponseWriter, fields map[string][]string) {
	apierror.Write(w, apierror.Validation(fields))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func toTranslations(v string) domain.Translations {
	if v == "" {
		return nil
	}
	return domain.Translations{"en": v}
}
