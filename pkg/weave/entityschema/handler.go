package entityschema

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pletka-io/pletka/pkg/auth"
	pkgdomain "github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	"github.com/pletka-io/pletka/pkg/weave/organization"
	weaveproject "github.com/pletka-io/pletka/pkg/weave/project"
)

type LangResolver func(*http.Request) string

type Host struct {
	Logger        *slog.Logger
	Weave         pkgdomain.WeaveStore
	Organizations *organization.Service
	Languages     []formschema.LanguageInfo
	LangResolver  LangResolver
	I18n          i18n.Manager
}

func (h Host) Validate() error {
	var missing []string
	if h.Weave == nil {
		missing = append(missing, "Weave")
	}
	if h.Organizations == nil {
		missing = append(missing, "Organizations")
	}
	if len(missing) > 0 {
		return fmt.Errorf("entityschema host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

type Handler struct {
	logger       *slog.Logger
	weave        pkgdomain.WeaveStore
	orgs         *organization.Service
	languages    []formschema.LanguageInfo
	langResolver LangResolver
	i18n         i18n.Manager
	schemas      *schemaregistry.SchemaRegistry
}

func NewHandler(
	logger *slog.Logger,
	weaveStore pkgdomain.WeaveStore,
	orgs *organization.Service,
	languages []formschema.LanguageInfo,
	langResolver LangResolver,
	i18nMgr i18n.Manager,
) *Handler {
	h := &Handler{
		logger:       logger,
		weave:        weaveStore,
		orgs:         orgs,
		languages:    languages,
		langResolver: langResolver,
		i18n:         i18nMgr,
	}
	h.schemas = h.defaultSchemaRegistry()
	return h
}

func Mount(r chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	NewHandler(host.Logger, host.Weave, host.Organizations, host.Languages, host.LangResolver, host.I18n).Mount(r)
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/projects/form-schema/project", h.ProjectFormSchema)
	r.With(auth.WithProjectVersionContext).Get("/projects/{projectID:[A-Z0-9]+}/entity-list-schema/{entityType}", auth.WrapProjectRead(h.weave.Projects(), h.EntityListSchema))
	r.With(auth.WithProjectVersionContext).Get("/projects/{projectID:[A-Z0-9]+}/form-schema/{entityType}", auth.WrapProjectRead(h.weave.Projects(), h.FormSchema))
	r.With(auth.WithProjectVersionContext).Get("/projects/{projectID:[A-Z0-9]+}/models/options", auth.WrapProjectRead(h.weave.Projects(), h.ProjectModelOptions))
	r.With(auth.WithProjectVersionContext).Get("/projects/{projectID:[A-Z0-9]+}/collections/options", auth.WrapProjectRead(h.weave.Projects(), h.ProjectCollectionOptions))
}

func (h *Handler) EntityListSchema(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	entityType := chi.URLParam(r, "entityType")
	if projectID == "" || entityType == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Project ID and entity type are required")
		return
	}

	lang := h.currentLang(r)

	provider, ok := h.schemas.EntityListProvider(entityType)
	if !ok {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", fmt.Sprintf("Unknown entity type: %s", entityType))
		return
	}
	schema, err := provider.BuildEntityListSchema(r.Context(), schemaregistry.EntityListRequest{
		ProjectID:  projectID,
		EntityType: entityType,
		Lang:       lang,
		Languages:  h.languages,
		Query:      r.URL.Query(),
	})
	if err != nil {
		h.logger.Error("failed to build entity list schema", "project_id", projectID, "entity_type", entityType, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "Failed to build schema")
		return
	}
	if schema == nil {
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "Failed to build schema")
		return
	}

	if project := auth.ProjectFromContext(r.Context()); project != nil {
		if auth.ProjectVersionFromContext(r.Context()) != "" {
			schema.StripEditActions()
		}
		if !auth.FromContext(r.Context()).Can(auth.ProjectEdit, auth.ProjectResource(project), nil) {
			schema.StripEditActions()
		}
	} else {
		schema.StripEditActions()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(schema)
}

func (h *Handler) ProjectFormSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = formschema.ModeCreate
	}
	entityID := r.URL.Query().Get("entity_id")
	lang := h.currentLang(r)

	var existing *formschema.ProjectInput
	if (mode == formschema.ModeEdit || mode == formschema.ModeView) && entityID != "" {
		p, err := h.weave.Projects().GetByID(ctx, entityID)
		if err != nil {
			h.logger.Error("failed to load project for form schema", "id", entityID, "err", err)
		} else if p != nil {
			existing = &formschema.ProjectInput{
				ID:          p.ID,
				SystemName:  p.SystemName,
				UIName:      p.UIName,
				Description: p.Description,
			}
		}
	}

	// Delegate to the project slice's canonical form-schema builder
	// (per pkg/weave/ module-shape ADR — slices own their schemas).
	schema := weaveproject.BuildFormSchema(mode, existing, lang, h.languages)
	if mode == formschema.ModeCreate {
		h.attachProjectOwnerField(r, schema)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(schema); err != nil {
		h.logger.Error("failed to encode project form schema", "error", err)
	}
}

func (h *Handler) attachProjectOwnerField(r *http.Request, schema *formschema.FormSchema) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil || schema == nil || len(schema.Sections) == 0 {
		return
	}
	options := []formschema.SelectOption{
		{
			Value: principal.ActorID,
			Label: pkglangOrFallback(fmt.Sprintf("Personal · %s", strings.TrimSpace(principal.DisplayName)), "Personal workspace"),
		},
	}
	seen := map[string]struct{}{principal.ActorID: {}}
	if h.orgs != nil {
		orgIDs := projectCreateOrgIDs(auth.FromContext(r.Context()))
		if len(orgIDs) > 0 {
			items, err := h.orgs.ListByIDs(r.Context(), orgIDs)
			if err != nil {
				h.logger.Debug("load org owner options failed", "err", err)
			} else {
				slices.SortFunc(items, func(a, b organization.BrowseItem) int {
					if cmp := strings.Compare(strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName)); cmp != 0 {
						return cmp
					}
					return strings.Compare(a.Slug, b.Slug)
				})
				for _, item := range items {
					if _, ok := seen[item.ID]; ok {
						continue
					}
					seen[item.ID] = struct{}{}
					options = append(options, formschema.SelectOption{
						Value: item.ID,
						Label: pkglangOrFallback(fmt.Sprintf("Organization · %s", strings.TrimSpace(item.DisplayName)), item.Slug),
					})
				}
			}
		}
	}
	field := formschema.FieldDef{
		Name:     "owner_id",
		Widget:   formschema.WidgetSearchSelect,
		Required: true,
		Readonly: len(options) == 1,
		Label:    i18n.L("entityschema.project_owner.label", "Owner"),
		Help:     i18n.L("entityschema.project_owner.help", "Choose whether this project lives in your personal workspace or an organization you manage."),
		Value:    principal.ActorID,
		Options:  options,
	}
	fields := schema.Sections[0].Fields
	insertAt := len(fields)
	for i, candidate := range fields {
		if candidate.Name == "description" {
			insertAt = i
			break
		}
	}
	fields = append(fields, formschema.FieldDef{})
	copy(fields[insertAt+1:], fields[insertAt:])
	fields[insertAt] = field
	schema.Sections[0].Fields = fields
}

func projectCreateOrgIDs(snap *auth.AuthSnapshot) []string {
	if snap == nil {
		return nil
	}
	ids := make([]string, 0, len(snap.Roles))
	for key := range snap.Roles {
		if !strings.HasPrefix(key, "org:") {
			continue
		}
		orgID := strings.TrimPrefix(key, "org:")
		// snap.Can short-circuits true for a super-admin regardless of the
		// org's own role (see AuthSnapshot.Can), so a super-admin org
		// membership row of any role — even "member" — is included here.
		// Intentional: a super-admin can create projects in any org.
		if snap.Can(auth.OrgProjectCreate, auth.OrgResourceByID(orgID), nil) {
			ids = append(ids, orgID)
		}
	}
	return ids
}

func pkglangOrFallback(label, fallback string) pkgdomain.Translations {
	if strings.TrimSpace(label) == "" {
		label = fallback
	}
	return pkgdomain.Translations{"en": label}
}

func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	entityType := chi.URLParam(r, "entityType")
	if projectID == "" || entityType == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Project ID and entity type are required")
		return
	}

	role := "viewer"
	if project := auth.ProjectFromContext(ctx); project != nil {
		if effective := auth.FromContext(ctx).EffectiveRole(auth.ProjectResource(project)); effective != "" {
			role = effective
		}
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = formschema.ModeCreate
	}
	entityID := r.URL.Query().Get("entity_id")
	lang := h.currentLang(r)

	provider, ok := h.schemas.FormProvider(entityType)
	if !ok {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", fmt.Sprintf("Unknown entity type: %s", entityType))
		return
	}
	schema, err := provider.BuildFormSchema(ctx, schemaregistry.FormRequest{
		ProjectID:  projectID,
		EntityType: entityType,
		Mode:       mode,
		EntityID:   entityID,
		Lang:       lang,
		Languages:  h.languages,
		Query:      r.URL.Query(),
	})
	if err != nil {
		h.logger.Error("failed to build form schema", "project_id", projectID, "entity_type", entityType, "mode", mode, "entity_id", entityID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "Failed to build schema")
		return
	}

	if schema == nil {
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "Failed to build schema")
		return
	}

	schema = schema.FilterForRole(role)
	if h.i18n != nil {
		// Resolve every i18n.LocalizedText embedded in the schema with
		// bundle lookups for the current language. Raw
		// domain.Translations literals pass through; mixed mode is fine
		// during the migration.
		h.i18n.Resolve(schema, lang)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(schema)
}

func (h *Handler) ProjectModelOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Project ID is required")
		return
	}

	chain, err := h.weave.Projects().ProjectChain(ctx, projectID)
	if err != nil || len(chain) == 0 {
		chain = []string{projectID}
	}
	seen := map[string]bool{}
	opts := make([]formschema.SelectOption, 0)
	for _, pid := range chain {
		modelsList, listErr := h.weave.Models().ListOptions(ctx, pid)
		if listErr != nil {
			h.logger.Warn("list models for chain segment", "project", projectID, "ancestor", pid, "err", listErr)
			continue
		}
		var sourceLabel string
		if pid != projectID {
			sourceLabel = h.projectLabel(ctx, pid)
		}
		for _, m := range modelsList {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			opt := formschema.SelectOption{Value: m.ID, Label: m.UIName, Status: m.Status, SemanticID: m.SemanticID}
			if pid != projectID {
				opt.SourceProjectID = pid
				opt.SourceProjectLabel = sourceLabel
			}
			opts = append(opts, opt)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(opts); err != nil {
		h.logger.Error("failed to encode model options response", "err", err)
	}
}

func (h *Handler) ProjectCollectionOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Project ID is required")
		return
	}

	chain, err := h.weave.Projects().ProjectChain(ctx, projectID)
	if err != nil || len(chain) == 0 {
		chain = []string{projectID}
	}
	seen := map[string]bool{}
	opts := make([]formschema.SelectOption, 0)
	for _, pid := range chain {
		collections, listErr := h.weave.Collections().ListOptions(ctx, pid)
		if listErr != nil {
			h.logger.Warn("list collections for chain segment", "project", projectID, "ancestor", pid, "err", listErr)
			continue
		}
		var sourceLabel string
		if pid != projectID {
			sourceLabel = h.projectLabel(ctx, pid)
		}
		for _, c := range collections {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			opt := formschema.SelectOption{Value: c.ID, Label: c.UIName, Status: c.Status, SemanticID: c.SemanticID}
			if pid != projectID {
				opt.SourceProjectID = pid
				opt.SourceProjectLabel = sourceLabel
			}
			opts = append(opts, opt)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(opts); err != nil {
		h.logger.Error("failed to encode collection options response", "err", err)
	}
}

// projectLabel resolves the friendly UI name for an ancestor project id,
// used to give chain-walked picker options a readable provenance label
// instead of a raw project id. Returns "" if the project cannot be loaded;
// callers fall back to the id.
func (h *Handler) projectLabel(ctx context.Context, projectID string) string {
	project, err := h.weave.Projects().GetByID(ctx, projectID)
	if err != nil || project == nil {
		return ""
	}
	return project.UIName.Get("en", project.ID)
}

func (h *Handler) currentLang(r *http.Request) string {
	if h.langResolver != nil {
		if lang := h.langResolver(r); lang != "" {
			return lang
		}
	}
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	return "en"
}
