package actoradmin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

type Handler struct {
	svc       *Service
	languages []formschema.LanguageInfo
	lang      LangResolver
}

type LangResolver func(*http.Request) string

func NewHandler(svc *Service, languages []formschema.LanguageInfo, lang LangResolver) *Handler {
	if lang == nil {
		lang = func(*http.Request) string { return "en" }
	}
	return &Handler{svc: svc, languages: languages, lang: lang}
}

// Mount registers the super-admin-gated routes. Wrap the call site with
// RequireSuperAdmin (see routes.go); self-profile endpoints mount via
// MountSelf without that gate, since they're scoped to the caller's
// own actor row.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/admin/users", func(r chi.Router) {
		r.Get("/entity-list-schema", h.UserEntityListSchema)
		r.Get("/data", h.ListUsersData)
		r.Get("/form-schema", h.UserFormSchema)
		r.Post("/", h.CreateUser)
		r.Put("/{id}", h.UpdateUser)
		r.Get("/{id}/password-form-schema", h.UserPasswordFormSchema)
		r.Post("/{id}/password", h.SetUserPassword)
	})

	r.Route("/admin/institutions", func(r chi.Router) {
		r.Get("/entity-list-schema", h.InstitutionEntityListSchema)
		r.Get("/data", h.ListInstitutionsData)
		r.Get("/form-schema", h.InstitutionFormSchema)
		r.Get("/options", h.InstitutionOptions)
		r.Post("/", h.CreateInstitution)
		r.Put("/{id}", h.UpdateInstitution)
	})
}

// MountSelf registers the self-edit profile endpoints. Each handler
// reads the actor ID from the auth context and rejects anonymous
// callers with 401, so no router-level admin gate is needed.
func (h *Handler) MountSelf(r chi.Router) {
	r.Get("/me/form-schema", h.SelfProfileFormSchema)
	r.Put("/me", h.UpdateSelf)
	r.Get("/me/password-form-schema", h.PasswordFormSchema)
	r.Post("/me/password", h.ChangePassword)
}

func (h *Handler) UserEntityListSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildUserEntityListSchema(h.lang(r), h.languages))
}

func (h *Handler) InstitutionEntityListSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BuildInstitutionEntityListSchema(h.lang(r), h.languages))
}

func (h *Handler) ListUsersData(w http.ResponseWriter, r *http.Request) {
	page, perPage, ok := parsePaging(w, r)
	if !ok {
		return
	}
	out, err := h.svc.BrowseUsers(r.Context(), UserBrowseInput{
		Search:  r.URL.Query().Get("search"),
		Role:    r.URL.Query().Get("role"),
		SortBy:  r.URL.Query().Get("sort_by"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to list users"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out.Items, "total": out.Total})
}

func (h *Handler) ListInstitutionsData(w http.ResponseWriter, r *http.Request) {
	page, perPage, ok := parsePaging(w, r)
	if !ok {
		return
	}
	out, err := h.svc.BrowseInstitutions(r.Context(), InstitutionBrowseInput{
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort_by"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to list institutions"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out.Items, "total": out.Total})
}

func (h *Handler) UserFormSchema(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = formschema.ModeCreate
	}
	if mode == formschema.ModeCreate {
		writeJSON(w, http.StatusOK, BuildUserFormSchema(formschema.ModeCreate, nil, h.lang(r), h.languages))
		return
	}
	id := r.URL.Query().Get("entity_id")
	if id == "" {
		apierror.Write(w, apierror.BadRequest("entity_id required"))
		return
	}
	user, err := h.svc.GetUser(r.Context(), id)
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to load user"))
		return
	}
	if user == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, BuildUserFormSchema(formschema.ModeEdit, user, h.lang(r), h.languages))
}

func (h *Handler) InstitutionFormSchema(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = formschema.ModeCreate
	}
	if mode == formschema.ModeCreate {
		writeJSON(w, http.StatusOK, BuildInstitutionFormSchema(formschema.ModeCreate, nil, h.lang(r), h.languages))
		return
	}
	id := r.URL.Query().Get("entity_id")
	if id == "" {
		apierror.Write(w, apierror.BadRequest("entity_id required"))
		return
	}
	institution, err := h.svc.GetInstitution(r.Context(), id)
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to load institution"))
		return
	}
	if institution == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, BuildInstitutionFormSchema(formschema.ModeEdit, institution, h.lang(r), h.languages))
}

func (h *Handler) InstitutionOptions(w http.ResponseWriter, r *http.Request) {
	opts, err := h.svc.InstitutionOptions(r.Context())
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to load institution options"))
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

// SelfProfileFormSchema returns the form schema for the logged-in
// user's own profile. Anonymous callers get 401 — there's nothing to
// schema.
func (h *Handler) SelfProfileFormSchema(w http.ResponseWriter, r *http.Request) {
	snap := weaveauth.FromContext(r.Context())
	if snap == nil || snap.IsAnonymous || snap.ActorID == "" {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	current, err := h.svc.GetUser(r.Context(), snap.ActorID)
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to load profile"))
		return
	}
	if current == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, BuildSelfProfileFormSchema(current, h.lang(r), h.languages))
}

// PasswordFormSchema returns the schema for the change-password form.
// Anonymous callers get 401.
func (h *Handler) PasswordFormSchema(w http.ResponseWriter, r *http.Request) {
	snap := weaveauth.FromContext(r.Context())
	if snap == nil || snap.IsAnonymous || snap.ActorID == "" {
		apierror.Write(w, apierror.Unauthorized())
		return
	}
	writeJSON(w, http.StatusOK, BuildPasswordFormSchema(h.lang(r), h.languages))
}

// ChangePassword verifies the supplied current password and writes a
// new bcrypt hash on match. 401 for anonymous callers, 422 for
// validation errors (mismatch, length, wrong current), 500 for
// unexpected failures.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	snap := weaveauth.FromContext(r.Context())
	if snap == nil || snap.IsAnonymous || snap.ActorID == "" {
		apierror.Write(w, apierror.Unauthorized())
		return
	}

	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid request body"))
		return
	}

	errs := map[string][]string{}
	if strings.TrimSpace(body.CurrentPassword) == "" {
		errs["current_password"] = append(errs["current_password"], "current password is required")
	}
	if len(body.NewPassword) < 12 {
		errs["new_password"] = append(errs["new_password"], "new password must be at least 12 characters")
	}
	if body.ConfirmPassword != body.NewPassword {
		errs["confirm_password"] = append(errs["confirm_password"], "passwords do not match")
	}
	if len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}

	if err := h.svc.ChangePassword(r.Context(), snap.ActorID, body.CurrentPassword, body.NewPassword); err != nil {
		if errors.Is(err, ErrPasswordIncorrect) {
			writeValidationErrors(w, map[string][]string{"current_password": {"current password is incorrect"}})
			return
		}
		apierror.Write(w, apierror.InternalWith("failed to change password"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// UpdateSelf writes a self-edit. Identity-bearing fields
// (slug/email/role/parent_id) are intentionally not accepted here —
// admin-only edits go through PUT /admin/users/{id}.
func (h *Handler) UpdateSelf(w http.ResponseWriter, r *http.Request) {
	snap := weaveauth.FromContext(r.Context())
	if snap == nil || snap.IsAnonymous || snap.ActorID == "" {
		apierror.Write(w, apierror.Unauthorized())
		return
	}

	var body struct {
		DisplayName string  `json:"display_name"`
		Country     *string `json:"country"`
		Website     *string `json:"website"`
		Orcid       *string `json:"orcid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid request body"))
		return
	}

	if strings.TrimSpace(body.DisplayName) == "" {
		writeValidationErrors(w, map[string][]string{"display_name": {"name is required"}})
		return
	}

	row, err := h.svc.UpdateSelf(r.Context(), snap.ActorID, SelfProfileInput{
		DisplayName: body.DisplayName,
		Country:     body.Country,
		Website:     body.Website,
		Orcid:       body.Orcid,
	})
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to update profile"))
		return
	}
	if row == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": row.ID})
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		DisplayName string  `json:"display_name"`
		Slug        string  `json:"slug"`
		Email       *string `json:"email"`
		Role        string  `json:"role"`
		ParentID    *string `json:"parent_id"`
		FirstName   *string `json:"first_name"`
		LastName    *string `json:"last_name"`
		Country     *string `json:"country"`
		Website     *string `json:"website"`
		Orcid       *string `json:"orcid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid request body"))
		return
	}
	if errs := validateUserInput(body.DisplayName, body.Slug, body.Role); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}
	row, err := h.svc.UpdateUser(r.Context(), id, UserEditInput{
		DisplayName: body.DisplayName,
		Slug:        body.Slug,
		Email:       body.Email,
		Role:        body.Role,
		ParentID:    body.ParentID,
		FirstName:   body.FirstName,
		LastName:    body.LastName,
		Country:     body.Country,
		Website:     body.Website,
		Orcid:       body.Orcid,
	})
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to update user"))
		return
	}
	if row == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": row.ID})
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName string  `json:"display_name"`
		Slug        string  `json:"slug"`
		Email       *string `json:"email"`
		Role        string  `json:"role"`
		ParentID    *string `json:"parent_id"`
		FirstName   *string `json:"first_name"`
		LastName    *string `json:"last_name"`
		Country     *string `json:"country"`
		Website     *string `json:"website"`
		Orcid       *string `json:"orcid"`
		Password    *string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid request body"))
		return
	}
	if errs := validateUserInput(body.DisplayName, body.Slug, body.Role); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}
	if body.Password != nil && strings.TrimSpace(*body.Password) != "" && len(*body.Password) < 12 {
		writeValidationErrors(w, map[string][]string{"password": {"password must be at least 12 characters"}})
		return
	}
	row, generatedPassword, err := h.svc.CreateUser(r.Context(), UserCreateInput{
		DisplayName: body.DisplayName,
		Slug:        body.Slug,
		Email:       body.Email,
		Role:        body.Role,
		ParentID:    body.ParentID,
		FirstName:   body.FirstName,
		LastName:    body.LastName,
		Country:     body.Country,
		Website:     body.Website,
		Orcid:       body.Orcid,
		Password:    body.Password,
	})
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to create user"))
		return
	}
	resp := map[string]any{"id": row.ID}
	// Surface a generated password once so the admin can share it. Empty
	// when the admin supplied the password themselves.
	if generatedPassword != "" {
		resp["generated_password"] = generatedPassword
	}
	writeJSON(w, http.StatusCreated, resp)
}

// UserPasswordFormSchema returns the admin set-password form for one user.
func (h *Handler) UserPasswordFormSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		apierror.Write(w, apierror.BadRequest("user id is required"))
		return
	}
	lang := h.lang(r)
	schema := BuildAdminResetPasswordFormSchema(id, lang, h.languages)
	writeJSON(w, http.StatusOK, schema)
}

// SetUserPassword sets a user's password with admin authority (no current
// password required). Super-admin gated at the route level.
func (h *Handler) SetUserPassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		apierror.Write(w, apierror.BadRequest("user id is required"))
		return
	}
	var body struct {
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid request body"))
		return
	}
	if len(body.NewPassword) < 12 {
		writeValidationErrors(w, map[string][]string{"new_password": {"new password must be at least 12 characters"}})
		return
	}
	if err := h.svc.SetPassword(r.Context(), id, body.NewPassword); err != nil {
		apierror.Write(w, apierror.InternalWith("failed to set password"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) CreateInstitution(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName string  `json:"display_name"`
		Slug        string  `json:"slug"`
		Acronym     *string `json:"acronym"`
		Country     *string `json:"country"`
		Website     *string `json:"website"`
		Visibility  string  `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid request body"))
		return
	}
	if errs := validateInstitutionInput(body.DisplayName, body.Slug); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}
	row, err := h.svc.CreateInstitution(r.Context(), InstitutionCreateInput{
		DisplayName: body.DisplayName,
		Slug:        body.Slug,
		Acronym:     body.Acronym,
		Country:     body.Country,
		Website:     body.Website,
		Visibility:  body.Visibility,
	})
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to create institution"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": row.ID})
}

func (h *Handler) UpdateInstitution(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		DisplayName string  `json:"display_name"`
		Slug        string  `json:"slug"`
		Acronym     *string `json:"acronym"`
		Country     *string `json:"country"`
		Website     *string `json:"website"`
		Visibility  string  `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid request body"))
		return
	}
	if errs := validateInstitutionInput(body.DisplayName, body.Slug); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}
	row, err := h.svc.UpdateInstitution(r.Context(), id, InstitutionEditInput{
		DisplayName: body.DisplayName,
		Slug:        body.Slug,
		Acronym:     body.Acronym,
		Country:     body.Country,
		Website:     body.Website,
		Visibility:  body.Visibility,
	})
	if err != nil {
		apierror.Write(w, apierror.InternalWith("failed to update institution"))
		return
	}
	if row == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": row.ID})
}

func parsePaging(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	page := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			apierror.Write(w, apierror.BadRequest("page must be a positive integer"))
			return 0, 0, false
		}
		page = parsed
	}
	perPage := 25
	if raw := strings.TrimSpace(r.URL.Query().Get("per_page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			apierror.Write(w, apierror.BadRequest("per_page must be a positive integer"))
			return 0, 0, false
		}
		perPage = parsed
	}
	return page, perPage, true
}

func validateUserInput(displayName, slug, role string) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(displayName) == "" {
		errs["display_name"] = append(errs["display_name"], "Display name is required")
	}
	if strings.TrimSpace(slug) == "" {
		errs["slug"] = append(errs["slug"], "Username / slug is required")
	} else if err := weaveauth.ValidateSlug(strings.TrimSpace(slug)); err != nil {
		errs["slug"] = append(errs["slug"], err.Error())
	}
	switch strings.TrimSpace(role) {
	case "contributor", "admin", "super_admin":
	default:
		errs["role"] = append(errs["role"], "Role must be contributor, admin, or super_admin")
	}
	return errs
}

func validateInstitutionInput(displayName, slug string) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(displayName) == "" {
		errs["display_name"] = append(errs["display_name"], "Display name is required")
	}
	if strings.TrimSpace(slug) == "" {
		errs["slug"] = append(errs["slug"], "Slug is required")
	} else if err := weaveauth.ValidateSlug(strings.TrimSpace(slug)); err != nil {
		errs["slug"] = append(errs["slug"], err.Error())
	}
	return errs
}

// writeValidationErrors forwards to apierror.Write — see pkg/weave/apierror.
func writeValidationErrors(w http.ResponseWriter, errors map[string][]string) {
	apierror.Write(w, apierror.Validation(errors))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
