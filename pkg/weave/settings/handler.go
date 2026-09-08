package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

// Handler exposes the project-settings-v2 surface as a vertical slice.
//
// All endpoints take a {projectID} URL parameter. Reads gate on
// auth.ProjectRead; writes gate on auth.ProjectEdit. Schema construction
// stays in pkg/formschema — this handler only composes those builders
// with the project store + auth snapshot.
type Handler struct {
	weave     domain.WeaveStore
	store     Store
	log       *slog.Logger
	languages []formschema.LanguageInfo
}

// NewHandler builds a Handler. nil log → slog.Default.
func NewHandler(weave domain.WeaveStore, store Store, languages []formschema.LanguageInfo, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{weave: weave, store: store, log: log, languages: languages}
}

// loadProjectAndGate fetches the project and verifies the caller holds
// the requested capability against it. Returns the loaded project and
// resource on success. On failure it writes an HTTP error to w and
// returns (nil, nil, false); callers should return immediately.
func (h *Handler) loadProjectAndGate(ctx context.Context, w http.ResponseWriter, projectID string, cap auth.Capability) (*domain.Project, *auth.Resource, bool) {
	project, err := h.weave.Projects().GetByID(ctx, projectID)
	if err != nil || project == nil {
		apierror.Write(w, apierror.NotFound("Project not found"))
		return nil, nil, false
	}

	res := auth.ProjectResource(project)
	snap := auth.FromContext(ctx)
	if !snap.Can(cap, res, nil) {
		if snap.IsAnonymous {
			apierror.Write(w, apierror.Unauthorized())
		} else {
			apierror.Write(w, apierror.Forbidden("Access denied"))
		}
		return nil, nil, false
	}
	return project, &res, true
}

// PageSchema returns the settings page schema with capability-filtered
// sections. GET /projects/{projectID}/settings/schema.
//
// Gated on ProjectEdit. Settings is an admin surface; even read-only
// inspection of the settings shell (sections, warnings) leaks
// project structure to anon viewers, so the gate matches the
// settings page route itself (project_pages.go).
func (h *Handler) PageSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")

	project, res, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}

	resolved, err := h.weave.Projects().ResolvedOntologyVersions(ctx, project.ID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		h.log.Error("failed to resolve ontology versions for setup state", "project", project.ID, "err", err)
	}
	setup := formschema.ProjectSetupState{HasOntology: len(resolved) > 0}

	schema := formschema.BuildSettingsSchema(project, auth.FromContext(ctx), *res, setup, auth.ProjectVersionFromContext(ctx))

	writeJSON(w, http.StatusOK, schema)
}

// FormSchema returns the form schema for a single settings section.
// GET /projects/{projectID}/settings/form-schema/{section}.
func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	section := chi.URLParam(r, "section")

	// Section gate. Today every section is ProjectEdit; structure leaves
	// room for stricter caps (e.g. project.delete) per section without
	// touching the URL space.
	sectionGate := map[string]auth.Capability{
		"general":                auth.ProjectEdit,
		"about":                  auth.ProjectEdit,
		"ontology":               auth.ProjectEdit,
		"ontology-parent-add":    auth.ProjectEdit,
		"ontology-parent-source": auth.ProjectEdit,
		"vocabularies":           auth.ProjectEdit,
		"autocomplete":           auth.ProjectEdit,
	}
	gate, known := sectionGate[section]
	if !known {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "Unknown settings section: "+section)
		return
	}
	// Release-version snapshots stay edit-gated too: settings is
	// admin-only regardless of draft vs frozen release. MakeReadOnly()
	// below disables write affordances on the rendered form for
	// release viewers.

	project, _, ok := h.loadProjectAndGate(ctx, w, projectID, gate)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}

	var schema *formschema.FormSchema
	switch section {
	case "general":
		snap := auth.FromContext(ctx)
		schema = formschema.BuildGeneralSettingsSchema(project, snap != nil && snap.IsSuperAdmin, lang, h.languages)
	case "about":
		schema = formschema.BuildAboutSettingsSchema(project, lang, h.languages)
	case "vocabularies":
		vocabState, vocabErr := h.store.VocabularySettingsState(ctx, project.ID)
		if vocabErr != nil {
			h.log.Error("failed to load vocabulary settings", "project_id", project.ID, "err", vocabErr)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load vocabulary settings")
			return
		}
		schema = formschema.BuildVocabularySettingsSchema(project.ID, vocabularySelectOptions(vocabState.Options), vocabState.Selected, vocabState.Enforce, lang, h.languages)
	case "autocomplete":
		schema = formschema.BuildOntologyProbeSchema(lang, h.languages)
	case "ontology":
		allProjects, _, listErr := h.weave.Projects().List(ctx)
		if listErr != nil {
			allProjects = nil
		}
		// Pre-populate the "Also inherit from child weaves" multi-select
		// with the current secondary parents that are children of the
		// primary parent.
		var existingChildParents []string
		if project.ParentProjectID != nil && *project.ParentProjectID != "" {
			links, lErr := h.weave.ProjectInheritances().List(ctx, project.ID)
			if lErr == nil {
				children, cErr := h.weave.Projects().ListChildren(ctx, *project.ParentProjectID)
				if cErr == nil {
					childSet := make(map[string]bool, len(children))
					for _, c := range children {
						childSet[c.ID] = true
					}
					for _, l := range links {
						if l.IsPrimary {
							continue
						}
						if childSet[l.ParentProjectID] {
							existingChildParents = append(existingChildParents, l.ParentProjectID)
						}
					}
				}
			}
		}
		schema = formschema.BuildOntologySettingsSchema(project, allProjects, existingChildParents, lang, h.languages)
	case "ontology-parent-add":
		allProjects, _, listErr := h.weave.Projects().List(ctx)
		if listErr != nil {
			allProjects = nil
		}
		existing, listErr := h.weave.ProjectInheritances().List(ctx, project.ID)
		if listErr != nil {
			existing = nil
		}
		schema = formschema.BuildProjectInheritanceCreateSchema(project, allProjects, existing, lang, h.languages)
	case "ontology-parent-source":
		parentID := strings.TrimSpace(r.URL.Query().Get("parent_id"))
		if parentID == "" {
			errresp.Error(w, r, http.StatusBadRequest, "bad_request", "parent_id is required")
			return
		}
		links, listErr := h.weave.ProjectInheritances().List(ctx, project.ID)
		if listErr != nil {
			h.log.Error("list inheritances for source form", "project_id", project.ID, "err", listErr)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load parent dependency")
			return
		}
		var link *domain.ProjectInheritance
		for i := range links {
			if links[i].ParentProjectID == parentID {
				link = &links[i]
				break
			}
		}
		if link == nil {
			errresp.Error(w, r, http.StatusNotFound, "not_found", "parent dependency not found")
			return
		}
		parent, _ := h.weave.Projects().GetByID(ctx, parentID)
		releaseOptions, relErr := h.releaseOptionsForProject(ctx, parentID)
		if relErr != nil {
			h.log.Error("list parent releases for source form", "project_id", project.ID, "parent_id", parentID, "err", relErr)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load parent releases")
			return
		}
		schema = formschema.BuildProjectInheritanceSourceSchema(project.ID, *link, parent, releaseOptions, lang, h.languages)
	}
	if auth.ProjectVersionFromContext(ctx) != "" && schema != nil {
		schema.MakeReadOnly()
	}

	writeJSON(w, http.StatusOK, schema)
}

// ListSchema returns the list schema for settings panes that are driven by
// ListManager rather than a form renderer.
//
// Gated on ProjectEdit — same reasoning as PageSchema (settings is an
// admin surface, even structural reads leak).
func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	section := chi.URLParam(r, "section")

	project, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}

	switch section {
	case "ontology-parents":
		writeJSON(w, http.StatusOK, formschema.BuildProjectInheritanceListSchema(project.ID, lang, h.languages, auth.ProjectVersionFromContext(ctx)))
	default:
		errresp.Error(w, r, http.StatusNotFound, "not_found", "No list schema for section: "+section)
	}
}

// PaneSchema returns a composite pane schema for sections that need
// multiple panels. Only "ontology" is supported today.
// GET /projects/{projectID}/settings/pane-schema/{section}.
//
// Gated on ProjectEdit — same reasoning as PageSchema. The pane
// itself is read-only for non-editors via the URL gate above (no
// mutation URLs emitted by PaneView when canEdit is false), but the
// pane shell shouldn't be reachable at all for anon viewers.
func (h *Handler) PaneSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	section := chi.URLParam(r, "section")

	gate := auth.ProjectEdit
	project, _, ok := h.loadProjectAndGate(ctx, w, projectID, gate)
	if !ok {
		return
	}

	var schema *formschema.CompositePaneSchema
	switch section {
	case "ontology":
		schema = formschema.BuildOntologyPaneSchema(project.ID, auth.ProjectVersionFromContext(ctx))
	default:
		errresp.Error(w, r, http.StatusNotFound, "not_found", "No pane for section: "+section)
		return
	}

	writeJSON(w, http.StatusOK, schema)
}

// UpdateGeneral handles JSON updates to the general project settings
// (UIName + Description + Visibility). PUT /projects/{projectID}/settings/general.
func (h *Handler) UpdateGeneral(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")

	project, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}

	var body struct {
		UIName      domain.Translations `json:"ui_name"`
		Description domain.Translations `json:"description"`
		Visibility  string              `json:"visibility"`
		IsCoreWeave *bool               `json:"is_core_weave,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationErrors(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}

	errs := map[string][]string{}
	if body.UIName.Get("en") == "" {
		errs["ui_name"] = []string{"English name is required"}
	}
	switch body.Visibility {
	case "public", "internal", "private":
		// valid
	case "":
		errs["visibility"] = []string{"Visibility is required"}
	default:
		errs["visibility"] = []string{"Visibility must be public, internal, or private"}
	}
	if len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}

	if body.UIName != nil {
		project.UIName = body.UIName
	}
	if body.Description != nil {
		project.Description = body.Description
	}
	project.Visibility = body.Visibility
	// is_core_weave is super-admin only — silently ignore the field
	// when a non-super-admin sends it, so the form keeps working for
	// regular owners.
	if body.IsCoreWeave != nil {
		if snap := auth.FromContext(ctx); snap != nil && snap.IsSuperAdmin {
			project.IsCoreWeave = *body.IsCoreWeave
		}
	}

	if err := h.weave.Projects().Update(ctx, project); err != nil {
		h.log.Error("failed to update project", "error", err, "project_id", projectID)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to save settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// UpdateAbout handles JSON updates to the project's About section
// (license + readme + topics + base_url). PUT
// /projects/{projectID}/settings/about. Identity, access control, and
// ontology-link state are untouched.
func (h *Handler) UpdateAbout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")

	_, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}

	var body struct {
		License string              `json:"license"`
		README  domain.Translations `json:"readme"`
		// Topics accepts either an array of strings or a single
		// comma-separated string (the v1 widget posts the latter).
		Topics  json.RawMessage `json:"topics"`
		BaseURL string          `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationErrors(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}

	topics, terr := parseTopics(body.Topics)
	if terr != nil {
		writeValidationErrors(w, map[string][]string{"topics": {terr.Error()}})
		return
	}
	if body.README == nil {
		body.README = domain.Translations{}
	}

	updated, err := h.weave.Projects().UpdateAbout(ctx, projectID, domain.ProjectAboutUpdate{
		License: body.License,
		README:  body.README,
		Topics:  topics,
		BaseURL: body.BaseURL,
	})
	if err != nil {
		h.log.Error("failed to update project about", "error", err, "project_id", projectID)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to save settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"license":  updated.License,
		"readme":   updated.README,
		"topics":   updated.Topics,
		"base_url": updated.BaseURL,
	})
}

func (h *Handler) UpdateVocabularies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")

	if _, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit); !ok {
		return
	}

	var body struct {
		VocabularyIDs       []string `json:"vocabulary_ids"`
		EnforceConceptLists bool     `json:"enforce_concept_lists"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationErrors(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}

	allowed, err := h.store.GlobalVocabularyIDs(ctx)
	if err != nil {
		h.log.Error("load global vocabularies for settings update", "project_id", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to validate vocabularies")
		return
	}
	selected := make([]string, 0, len(body.VocabularyIDs))
	seen := map[string]bool{}
	for _, id := range body.VocabularyIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		if !allowed[id] {
			writeValidationErrors(w, map[string][]string{"vocabulary_ids": {"Choose vocabularies from the global vocabulary catalogue."}})
			return
		}
		seen[id] = true
		selected = append(selected, id)
	}

	if err := h.store.UpdateVocabularySettings(ctx, projectID, selected, body.EnforceConceptLists); err != nil {
		h.log.Error("save vocabulary settings", "project_id", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to save vocabulary settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// UpdateOntology handles JSON updates to the ontology settings.
// Accepts {"parent_project_id": string|null}. Cycle detection walks the
// ancestor chain (bounded depth 10); if the current project appears,
// the update is rejected with 422. No rows are copied on link —
// inheritance is resolved live by ResolvedOntologyVersions.
// PUT /projects/{projectID}/settings/ontology.
func (h *Handler) UpdateOntology(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")

	project, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}

	var body struct {
		ParentProjectID        *string  `json:"parent_project_id"`
		AdditionalChildParents []string `json:"additional_child_parents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationErrors(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}

	if body.ParentProjectID == nil || *body.ParentProjectID == "" {
		project.ParentProjectID = nil
	} else {
		if *body.ParentProjectID == projectID {
			writeValidationErrors(w, map[string][]string{"parent_project_id": {"project cannot be its own parent"}})
			return
		}
		creates, cerr := createsParentCycle(ctx, h.weave.Projects(), projectID, *body.ParentProjectID)
		if cerr != nil {
			h.log.Error("cycle detection failed", "error", cerr, "project_id", projectID)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to validate parent chain")
			return
		}
		if creates {
			writeValidationErrors(w, map[string][]string{"parent_project_id": {"creates cycle"}})
			return
		}
		project.ParentProjectID = body.ParentProjectID
	}

	if err := h.weave.Projects().Update(ctx, project); err != nil {
		h.log.Error("failed to update project", "error", err, "project_id", projectID)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to save settings")
		return
	}

	// Sync the "Also inherit from child weaves" multi-select against
	// weave_project_inheritance — but only diff within the set of the
	// chosen primary parent's children so de-selecting one doesn't drop
	// unrelated inheritance rows.
	if project.ParentProjectID != nil && *project.ParentProjectID != "" {
		if err := h.syncChildWeaveInheritances(ctx, projectID, *project.ParentProjectID, body.AdditionalChildParents); err != nil {
			h.log.Error("sync child-weave inheritances", "error", err, "project_id", projectID)
		}
	}

	refreshed, gerr := h.weave.Projects().GetByID(ctx, projectID)
	if gerr != nil || refreshed == nil {
		refreshed = project
	}
	writeJSON(w, http.StatusOK, refreshed)
}

// syncChildWeaveInheritances reconciles the secondary inheritance rows
// that point at children of the project's primary parent against the
// `additional_child_parents` multi-select. Rows whose parent is NOT a
// child of the current primary are left untouched — they belong to a
// different inheritance branch (e.g. a previous primary). Drives the
// PUT side of the form.
func (h *Handler) syncChildWeaveInheritances(ctx context.Context, projectID, primaryParentID string, requested []string) error {
	children, err := h.weave.Projects().ListChildren(ctx, primaryParentID)
	if err != nil {
		return fmt.Errorf("list children of %s: %w", primaryParentID, err)
	}
	childSet := make(map[string]bool, len(children))
	for _, c := range children {
		if c.ID == projectID {
			continue
		}
		childSet[c.ID] = true
	}
	requestedSet := make(map[string]bool, len(requested))
	for _, id := range requested {
		id = strings.TrimSpace(id)
		if id == "" || id == projectID {
			continue
		}
		if !childSet[id] {
			continue
		}
		requestedSet[id] = true
	}
	links, err := h.weave.ProjectInheritances().List(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list project inheritances: %w", err)
	}
	existing := make(map[string]bool, len(links))
	for _, l := range links {
		if l.IsPrimary {
			continue
		}
		existing[l.ParentProjectID] = true
		if childSet[l.ParentProjectID] && !requestedSet[l.ParentProjectID] {
			if rmErr := h.weave.ProjectInheritances().Remove(ctx, projectID, l.ParentProjectID); rmErr != nil {
				h.log.Warn("remove child-weave inheritance", "project", projectID, "parent", l.ParentProjectID, "err", rmErr)
			}
		}
	}
	for id := range requestedSet {
		if existing[id] {
			continue
		}
		if err := h.weave.ProjectInheritances().Add(ctx, domain.ProjectInheritance{
			ProjectID:       projectID,
			ParentProjectID: id,
			IsPrimary:       false,
			SourceMode:      domain.DependencySourceDraft,
		}); err != nil {
			h.log.Warn("add child-weave inheritance", "project", projectID, "parent", id, "err", err)
		}
	}
	return nil
}

// ChildWeaveOptions handles
// GET /projects/{projectID}/settings/ontology/child-weave-options
//
//	?parent_project_id={id}
//
// Returns the children of parent_project_id (excluding the project
// itself) for the additional_child_parents multi-select on the ontology
// settings form.
func (h *Handler) ChildWeaveOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	parentID := strings.TrimSpace(r.URL.Query().Get("parent_project_id"))
	if parentID == "" {
		writeJSON(w, http.StatusOK, []formschema.SelectOption{})
		return
	}
	children, err := h.weave.Projects().ListChildren(ctx, parentID)
	if err != nil {
		h.log.Error("list children for child-weave options", "project_id", projectID, "parent_id", parentID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to list child weaves")
		return
	}
	opts := make([]formschema.SelectOption, 0, len(children))
	for _, c := range children {
		if c.ID == projectID {
			continue
		}
		label := fmt.Sprintf("%s (%s)", c.UIName.Get("en", c.ID), c.ID)
		opts = append(opts, formschema.SelectOption{
			Value: c.ID,
			Label: domain.Translations{"en": label},
		})
	}
	writeJSON(w, http.StatusOK, opts)
}

func (h *Handler) ListInheritance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")

	project, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectRead)
	if !ok {
		return
	}

	var (
		links []domain.ProjectInheritance
		err   error
	)
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		links, err = h.weave.ProjectInheritances().ListVersion(ctx, projectID, version)
	} else {
		links, err = h.weave.ProjectInheritances().List(ctx, projectID)
	}
	if err != nil {
		h.log.Error("list inheritances", "project_id", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load inheritances")
		return
	}

	parents := make(map[string]*domain.Project, len(links))
	for _, link := range links {
		if parent, perr := h.weave.Projects().GetByID(ctx, link.ParentProjectID); perr == nil && parent != nil {
			parents[link.ParentProjectID] = parent
		}
	}
	categoryCountsByParent := map[string]int{}
	adoptionOpts := []domain.QueryOption{
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
		domain.WithFilter("entity_type", "category"),
	}
	if version := auth.ProjectVersionFromContext(ctx); version != "" {
		adoptionOpts = append(adoptionOpts, domain.WithVersion(version))
	}
	if adoptions, aerr := h.weave.Adoptions().List(ctx, adoptionOpts...); aerr == nil {
		for _, adoption := range adoptions {
			if strings.TrimSpace(adoption.SourceProjectID) == "" {
				continue
			}
			categoryCountsByParent[adoption.SourceProjectID]++
		}
	} else {
		h.log.Warn("list inheritance adoptions", "project_id", projectID, "err", aerr)
	}

	items := make([]map[string]any, 0, len(links))
	for _, link := range links {
		label := link.ParentProjectID
		subtitleParts := []string{link.ParentProjectID}
		if parent := parents[link.ParentProjectID]; parent != nil {
			label = parent.UIName.Get("en", parent.ID)
			subtitleParts = []string{parent.ID, parent.Visibility}
		}
		copiedCount := categoryCountsByParent[link.ParentProjectID]
		if copiedCount > 0 {
			subtitleParts = append(subtitleParts, fmt.Sprintf("%d copied %s", copiedCount, pluralize(copiedCount, "category", "categories")))
		}
		primaryLabel := ""
		if link.IsPrimary {
			primaryLabel = "Primary"
		}
		sourceMode := normalizeSourceMode(link.SourceMode)
		sourceLabel := "Draft"
		sourceDetail := "Follows live draft"
		if sourceMode == domain.DependencySourceRelease {
			sourceLabel = "Pinned"
			sourceDetail = "Pinned release"
			if strings.TrimSpace(link.SourceVersion) != "" {
				sourceDetail = fmt.Sprintf("Pinned · %s", strings.TrimSpace(link.SourceVersion))
			}
		}
		subtitleParts = append(subtitleParts, sourceDetail)
		copiedLabel := ""
		if copiedCount > 0 {
			copiedLabel = fmt.Sprintf("%d copied", copiedCount)
		}
		items = append(items, map[string]any{
			"id":                      link.ParentProjectID,
			"label":                   label,
			"subtitle":                strings.Join(subtitleParts, " · "),
			"source_mode":             string(sourceMode),
			"source_version":          strings.TrimSpace(link.SourceVersion),
			"source_label":            sourceLabel,
			"is_primary":              link.IsPrimary,
			"primary_label":           primaryLabel,
			"precedence_label":        ordinalLabel(link.CanonicalOrder + 1),
			"copied_categories_label": copiedLabel,
			"canonical_order":         link.CanonicalOrder,
			"adopted_at":              link.AdoptedAt,
			"_readonly":               auth.ProjectVersionFromContext(ctx) != "",
		})
	}
	writeJSON(w, http.StatusOK, items)

	_ = project
}

func (h *Handler) CreateInheritance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")

	project, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit)
	if !ok {
		return
	}

	var body struct {
		ParentProjectID string `json:"parent_project_id"`
		IsPrimary       bool   `json:"is_primary"`
		SourceMode      string `json:"source_mode"`
		SourceVersion   string `json:"source_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationErrors(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}
	body.ParentProjectID = strings.TrimSpace(body.ParentProjectID)
	if body.ParentProjectID == "" {
		writeValidationErrors(w, map[string][]string{"parent_project_id": {"Parent project is required"}})
		return
	}
	if body.ParentProjectID == projectID {
		writeValidationErrors(w, map[string][]string{"parent_project_id": {"project cannot be its own parent"}})
		return
	}
	sourceMode := normalizeSourceMode(domain.DependencySourceMode(strings.TrimSpace(body.SourceMode)))
	sourceVersion := strings.TrimSpace(body.SourceVersion)
	if _, _, ok := h.loadProjectAndGate(ctx, w, body.ParentProjectID, auth.ProjectRead); !ok {
		return
	}
	if fieldErrs := h.validateInheritanceSource(ctx, body.ParentProjectID, sourceMode, sourceVersion); len(fieldErrs) > 0 {
		writeValidationErrors(w, fieldErrs)
		return
	}

	existing, err := h.weave.ProjectInheritances().List(ctx, projectID)
	if err != nil {
		h.log.Error("list existing inheritances", "project_id", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to validate inheritance graph")
		return
	}
	for _, link := range existing {
		if link.ParentProjectID == body.ParentProjectID {
			writeValidationErrors(w, map[string][]string{"parent_project_id": {"parent project already linked"}})
			return
		}
	}
	creates, cerr := createsInheritanceCycle(ctx, h.weave.ProjectInheritances(), projectID, body.ParentProjectID)
	if cerr != nil {
		h.log.Error("inheritance cycle detection failed", "project_id", projectID, "candidate_parent", body.ParentProjectID, "err", cerr)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to validate parent chain")
		return
	}
	if creates {
		writeValidationErrors(w, map[string][]string{"parent_project_id": {"creates cycle"}})
		return
	}

	primary := body.IsPrimary || len(existing) == 0
	link := domain.ProjectInheritance{
		ProjectID:       projectID,
		ParentProjectID: body.ParentProjectID,
		IsPrimary:       primary,
		CanonicalOrder:  len(existing),
		SourceMode:      sourceMode,
		SourceVersion:   sourceVersion,
	}
	if err := h.weave.ProjectInheritances().Add(ctx, link); err != nil {
		h.log.Error("add inheritance", "project_id", projectID, "parent_id", body.ParentProjectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to save inheritance")
		return
	}
	if primary {
		if err := h.weave.ProjectInheritances().SetPrimary(ctx, projectID, body.ParentProjectID); err != nil {
			h.log.Error("set primary inheritance", "project_id", projectID, "parent_id", body.ParentProjectID, "err", err)
			errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to save inheritance")
			return
		}
	}
	if err := h.copyParentCategories(ctx, project, body.ParentProjectID); err != nil {
		_ = h.weave.ProjectInheritances().Remove(ctx, projectID, body.ParentProjectID)
		h.log.Error("copy parent categories", "project_id", projectID, "parent_id", body.ParentProjectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to copy parent categories")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true})
}

func (h *Handler) InheritanceReleaseOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if _, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit); !ok {
		return
	}
	parentID := strings.TrimSpace(r.URL.Query().Get("parent_project_id"))
	if parentID == "" {
		writeJSON(w, http.StatusOK, []formschema.SelectOption{})
		return
	}
	if _, _, ok := h.loadProjectAndGate(ctx, w, parentID, auth.ProjectRead); !ok {
		return
	}
	options, err := h.releaseOptionsForProject(ctx, parentID)
	if err != nil {
		h.log.Error("list parent release options", "project_id", projectID, "parent_id", parentID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load parent releases")
		return
	}
	writeJSON(w, http.StatusOK, options)
}

func (h *Handler) UpdateInheritanceSource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	parentID := strings.TrimSpace(chi.URLParam(r, "parentID"))
	if parentID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Parent project ID is required")
		return
	}
	if _, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit); !ok {
		return
	}
	links, err := h.weave.ProjectInheritances().List(ctx, projectID)
	if err != nil {
		h.log.Error("list inheritances for source update", "project_id", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to load parent dependencies")
		return
	}
	var existing *domain.ProjectInheritance
	for i := range links {
		if links[i].ParentProjectID == parentID {
			existing = &links[i]
			break
		}
	}
	if existing == nil {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "parent dependency not found")
		return
	}

	var body struct {
		SourceMode    string `json:"source_mode"`
		SourceVersion string `json:"source_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationErrors(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}
	sourceMode := normalizeSourceMode(domain.DependencySourceMode(strings.TrimSpace(body.SourceMode)))
	sourceVersion := strings.TrimSpace(body.SourceVersion)
	if fieldErrs := h.validateInheritanceSource(ctx, parentID, sourceMode, sourceVersion); len(fieldErrs) > 0 {
		writeValidationErrors(w, fieldErrs)
		return
	}
	updated := *existing
	updated.SourceMode = sourceMode
	updated.SourceVersion = sourceVersion
	if err := h.weave.ProjectInheritances().Add(ctx, updated); err != nil {
		h.log.Error("update inheritance source", "project_id", projectID, "parent_id", parentID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to update dependency source")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handler) DeleteInheritance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	parentID := strings.TrimSpace(chi.URLParam(r, "parentID"))
	if parentID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Parent project ID is required")
		return
	}
	if _, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit); !ok {
		return
	}
	if blockers, err := h.receiptOnlyAdoptionRemovalBlockers(ctx, projectID, parentID); err != nil {
		h.log.Error("check receipt-only adoptions before parent removal", "project_id", projectID, "parent_id", parentID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to validate parent removal")
		return
	} else if len(blockers) > 0 {
		writeValidationErrors(w, map[string][]string{
			"parent_project_id": {fmt.Sprintf(
				"cannot remove parent while adopted items from %s are still receipt-only: %s; fork them into local project material first",
				parentID,
				strings.Join(blockers, ", "),
			)},
		})
		return
	}
	if err := h.cleanupRemovedParentCategories(ctx, projectID, parentID); err != nil {
		h.log.Error("cleanup copied parent categories", "project_id", projectID, "parent_id", parentID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to clean up copied parent categories")
		return
	}
	if err := h.weave.ProjectInheritances().Remove(ctx, projectID, parentID); err != nil {
		h.log.Error("remove inheritance", "project_id", projectID, "parent_id", parentID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to remove inheritance")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SetPrimaryInheritance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	parentID := strings.TrimSpace(chi.URLParam(r, "parentID"))
	if parentID == "" {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "Parent project ID is required")
		return
	}
	if _, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit); !ok {
		return
	}
	if err := h.weave.ProjectInheritances().SetPrimary(ctx, projectID, parentID); err != nil {
		h.log.Error("set primary inheritance", "project_id", projectID, "parent_id", parentID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to update primary inheritance")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handler) ReorderInheritance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := chi.URLParam(r, "projectID")
	if _, _, ok := h.loadProjectAndGate(ctx, w, projectID, auth.ProjectEdit); !ok {
		return
	}
	var body struct {
		ParentProjectIDs []string `json:"parent_project_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidationErrors(w, map[string][]string{"body": {"invalid JSON body"}})
		return
	}
	if err := h.weave.ProjectInheritances().Reorder(ctx, projectID, body.ParentProjectIDs); err != nil {
		h.log.Error("reorder inheritances", "project_id", projectID, "err", err)
		errresp.Error(w, r, http.StatusInternalServerError, "internal", "failed to reorder inheritances")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func createsInheritanceCycle(ctx context.Context, store domain.ProjectInheritanceStore, projectID, candidateParentID string) (bool, error) {
	visited := map[string]bool{candidateParentID: true}
	queue := []string{candidateParentID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == projectID {
			return true, nil
		}
		parents, err := store.List(ctx, current)
		if err != nil {
			return false, err
		}
		for _, parent := range parents {
			if !visited[parent.ParentProjectID] {
				visited[parent.ParentProjectID] = true
				queue = append(queue, parent.ParentProjectID)
			}
		}
	}
	return false, nil
}

func ordinalLabel(n int) string {
	if n <= 0 {
		return ""
	}
	if n%100 >= 11 && n%100 <= 13 {
		return fmt.Sprintf("%dth", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	default:
		return fmt.Sprintf("%dth", n)
	}
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func normalizeSourceMode(mode domain.DependencySourceMode) domain.DependencySourceMode {
	switch mode {
	case domain.DependencySourceRelease:
		return domain.DependencySourceRelease
	default:
		return domain.DependencySourceDraft
	}
}

func (h *Handler) validateInheritanceSource(ctx context.Context, parentProjectID string, sourceMode domain.DependencySourceMode, sourceVersion string) map[string][]string {
	errs := map[string][]string{}
	switch normalizeSourceMode(sourceMode) {
	case domain.DependencySourceDraft:
		if strings.TrimSpace(sourceVersion) != "" {
			errs["source_version"] = []string{"draft dependencies cannot specify a parent release"}
		}
	case domain.DependencySourceRelease:
		if strings.TrimSpace(sourceVersion) == "" {
			errs["source_version"] = []string{"parent release is required when pinning a dependency"}
			return errs
		}
		versions, err := h.releaseVersions(ctx, parentProjectID)
		if err != nil {
			h.log.Error("validate parent release selector", "parent_id", parentProjectID, "err", err)
			return map[string][]string{"source_version": {"failed to load parent releases"}}
		}
		found := false
		for _, version := range versions {
			if version == sourceVersion {
				found = true
				break
			}
		}
		if !found {
			archived, aerr := h.store.ReleaseArchived(ctx, parentProjectID, sourceVersion)
			if aerr != nil {
				h.log.Error("check archived parent release", "parent_id", parentProjectID, "err", aerr)
				return map[string][]string{"source_version": {"failed to load parent releases"}}
			}
			if archived {
				errs["source_version"] = []string{"selected parent release is archived and can no longer be pinned"}
			} else {
				errs["source_version"] = []string{"selected parent release does not exist"}
			}
		}
	}
	return errs
}

func (h *Handler) releaseOptionsForProject(ctx context.Context, projectID string) ([]formschema.SelectOption, error) {
	versions, err := h.releaseVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	options := make([]formschema.SelectOption, 0, len(versions))
	for _, version := range versions {
		options = append(options, formschema.SelectOption{
			Value: version,
			Label: domain.Translations{"en": version},
		})
	}
	return options, nil
}

func (h *Handler) releaseVersions(ctx context.Context, projectID string) ([]string, error) {
	return h.store.ReleaseVersions(ctx, projectID)
}

func (h *Handler) copyParentCategories(ctx context.Context, project *domain.Project, parentProjectID string) error {
	parentCategories, err := h.weave.WeaveCategories().List(ctx, domain.WithProjectID(parentProjectID))
	if err != nil {
		return fmt.Errorf("list parent categories: %w", err)
	}
	if len(parentCategories) == 0 {
		return nil
	}
	childCategories, err := h.weave.WeaveCategories().List(ctx, domain.WithProjectID(project.ID))
	if err != nil {
		return fmt.Errorf("list child categories: %w", err)
	}
	existingBySystemName := make(map[string]bool, len(childCategories))
	maxOrder := -1
	for _, cat := range childCategories {
		existingBySystemName[cat.SystemName] = true
		if cat.CanonicalOrder > maxOrder {
			maxOrder = cat.CanonicalOrder
		}
	}
	sort.Slice(parentCategories, func(i, j int) bool {
		if parentCategories[i].CanonicalOrder != parentCategories[j].CanonicalOrder {
			return parentCategories[i].CanonicalOrder < parentCategories[j].CanonicalOrder
		}
		return parentCategories[i].SystemName < parentCategories[j].SystemName
	})

	createdAdoptions := make([]domain.Adoption, 0)
	createdCategoryIDs := make([]string, 0)
	cleanupCreated := func() {
		for _, id := range createdCategoryIDs {
			_ = h.weave.WeaveCategories().Delete(ctx, id)
		}
	}
	principal := auth.PrincipalFromContext(ctx)
	var createdByID *string
	if principal != nil && principal.ActorID != "" {
		createdByID = &principal.ActorID
	}
	for _, source := range parentCategories {
		if existingBySystemName[source.SystemName] {
			continue
		}
		maxOrder++
		next, err := h.weave.AllocateEntityNumber(ctx, project.ID, "category")
		if err != nil {
			return fmt.Errorf("allocate copied category number for %s: %w", source.ID, err)
		}
		semID := fmt.Sprintf("%s.CAT.%d", project.ID, next)
		// Set BOTH ID and SemanticID. postgresStore.Create mints a ULID
		// when ID is empty, which produced 20 stray ULID-shaped category
		// rows on TPZ (parent-inheritance copy) — every other category
		// uses the semantic ID as its primary key (~97% of rows). Keeping
		// ID == SemanticID matches the drafts handler path and keeps
		// adoption receipts + override remap consistent.
		created := &domain.Category{
			Entity: domain.Entity{
				ID:          semID,
				SemanticID:  semID,
				SystemName:  source.SystemName,
				UIName:      source.UIName,
				Description: source.Description,
				Status:      source.Status,
				ProjectID:   project.ID,
			},
			CanonicalOrder: maxOrder,
		}
		if err := h.weave.WeaveCategories().Create(ctx, created); err != nil {
			cleanupCreated()
			return fmt.Errorf("create copied category %s: %w", source.ID, err)
		}
		createdCategoryIDs = append(createdCategoryIDs, created.ID)
		existingBySystemName[created.SystemName] = true
		createdAdoptions = append(createdAdoptions, domain.Adoption{
			ProjectID:         project.ID,
			ContextEntityType: "project",
			ContextEntityID:   project.ID,
			EntityType:        "category",
			SourceProjectID:   source.ProjectID,
			SourceEntityID:    source.ID,
			CreatedByID:       createdByID,
		})
	}
	if len(createdAdoptions) == 0 {
		return nil
	}
	existingAdoptions, err := h.weave.Adoptions().List(ctx,
		domain.WithProjectID(project.ID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", project.ID),
	)
	if err != nil {
		cleanupCreated()
		return fmt.Errorf("list existing project adoptions: %w", err)
	}
	merged := append(existingAdoptions[:0:0], existingAdoptions...)
	merged = append(merged, createdAdoptions...)
	if err := h.weave.Adoptions().ReplaceForContext(ctx, project.ID, "project", project.ID, merged); err != nil {
		cleanupCreated()
		return fmt.Errorf("record category adoptions: %w", err)
	}
	return nil
}

func (h *Handler) receiptOnlyAdoptionRemovalBlockers(ctx context.Context, projectID, parentProjectID string) ([]string, error) {
	if h.weave == nil || strings.TrimSpace(projectID) == "" || strings.TrimSpace(parentProjectID) == "" {
		return nil, nil
	}

	adoptions, err := h.weave.Adoptions().List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
		domain.WithFilter("source_project_id", parentProjectID),
	)
	if err != nil {
		return nil, fmt.Errorf("list project-level adoptions: %w", err)
	}
	if len(adoptions) == 0 {
		return nil, nil
	}

	forks, err := h.weave.Forks().List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("source_project_id", parentProjectID),
	)
	if err != nil {
		return nil, fmt.Errorf("list project forks for parent removal: %w", err)
	}
	forkedSources := make(map[string]bool, len(forks))
	for _, fork := range forks {
		key := fork.EntityType + "|" + fork.SourceEntityID
		forkedSources[key] = true
	}

	blockers := make([]string, 0)
	seen := make(map[string]bool)
	for _, adoption := range adoptions {
		switch adoption.EntityType {
		case "category":
			continue
		case "model", "collection":
			key := adoption.EntityType + "|" + adoption.SourceEntityID
			if forkedSources[key] {
				continue
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			blockers = append(blockers, strings.ToUpper(adoption.EntityType[:1])+adoption.EntityType[1:]+" "+adoption.SourceEntityID)
		default:
			if seen[adoption.EntityType+"|"+adoption.SourceEntityID] {
				continue
			}
			seen[adoption.EntityType+"|"+adoption.SourceEntityID] = true
			blockers = append(blockers, adoption.EntityType+" "+adoption.SourceEntityID)
		}
	}
	sort.Strings(blockers)
	return blockers, nil
}

func (h *Handler) cleanupRemovedParentCategories(ctx context.Context, projectID, parentProjectID string) error {
	if h.weave == nil || strings.TrimSpace(projectID) == "" || strings.TrimSpace(parentProjectID) == "" {
		return nil
	}

	projectAdoptions, err := h.weave.Adoptions().List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
	)
	if err != nil {
		return fmt.Errorf("list project adoptions: %w", err)
	}
	if len(projectAdoptions) == 0 {
		return nil
	}

	categoryCounts, err := h.weave.WeaveCategories().ListWithCounts(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list project categories with counts: %w", err)
	}
	localBySystemName := make(map[string]domain.WeaveCategoryWithCounts, len(categoryCounts))
	for _, category := range categoryCounts {
		localBySystemName[category.SystemName] = category
	}

	removeKeys := make(map[string]bool)
	deleteIDs := make([]string, 0)
	for _, adoption := range projectAdoptions {
		if adoption.ContextEntityType != "project" || adoption.ContextEntityID != projectID || adoption.EntityType != "category" || adoption.SourceProjectID != parentProjectID {
			continue
		}
		sourceCategory, err := h.weave.WeaveCategories().GetByIdentifier(ctx, adoption.SourceEntityID, adoption.SourceProjectID)
		if err != nil {
			return fmt.Errorf("resolve adopted category source %s/%s: %w", adoption.SourceProjectID, adoption.SourceEntityID, err)
		}
		if sourceCategory == nil || strings.TrimSpace(sourceCategory.SystemName) == "" {
			removeKeys[adoptionCleanupKey(adoption)] = true
			continue
		}
		localCategory, ok := localBySystemName[sourceCategory.SystemName]
		if !ok {
			removeKeys[adoptionCleanupKey(adoption)] = true
			continue
		}
		removeKeys[adoptionCleanupKey(adoption)] = true
		if localCategory.FieldCount == 0 && localCategory.ModelFieldCount == 0 && localCategory.CollectionFieldCount == 0 {
			deleteIDs = append(deleteIDs, localCategory.ID)
		}
	}
	if len(removeKeys) == 0 {
		return nil
	}

	kept := make([]domain.Adoption, 0, len(projectAdoptions))
	for _, adoption := range projectAdoptions {
		if removeKeys[adoptionCleanupKey(adoption)] {
			continue
		}
		kept = append(kept, adoption)
	}
	if err := h.weave.Adoptions().ReplaceForContext(ctx, projectID, "project", projectID, kept); err != nil {
		return fmt.Errorf("detach copied parent categories: %w", err)
	}

	seenDelete := make(map[string]bool, len(deleteIDs))
	for _, id := range deleteIDs {
		if id == "" || seenDelete[id] {
			continue
		}
		seenDelete[id] = true
		if err := h.weave.WeaveCategories().Delete(ctx, id); err != nil {
			return fmt.Errorf("delete unused copied category %s: %w", id, err)
		}
	}
	return nil
}

func adoptionCleanupKey(a domain.Adoption) string {
	return strings.Join([]string{
		a.ContextEntityType,
		a.ContextEntityID,
		a.EntityType,
		a.SourceProjectID,
		a.SourceEntityID,
		a.SourceVersion,
	}, "|")
}

// writeJSON serializes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeValidationErrors writes a 422 with the standard schema-driven weave
// validation envelope.
// writeValidationErrors forwards to apierror.Write — see pkg/weave/apierror.
func writeValidationErrors(w http.ResponseWriter, fields map[string][]string) {
	apierror.Write(w, apierror.Validation(fields))
}

// errInvalidJSON is the canonical error for body parse failures. Held as
// a package-level value so tests can identify it without string
// comparison; not currently used by handlers but kept for future
// service-layer extraction.
var errInvalidJSON = errors.New("invalid JSON body")

// parseTopics accepts either ["topic1","topic2"] (JSON array of strings)
// or "topic1, topic2" (a single comma-separated string — what the v1
// text widget posts) and returns a normalised []string. nil / empty
// input returns an empty slice.
func parseTopics(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}

	var asArray []string
	if err := json.Unmarshal(raw, &asArray); err == nil {
		return cleanTopics(asArray), nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if asString == "" {
			return []string{}, nil
		}
		parts := strings.Split(asString, ",")
		return cleanTopics(parts), nil
	}

	return nil, errors.New("topics must be an array of strings or a comma-separated string")
}

func vocabularySelectOptions(options []VocabularySettingsOption) []formschema.SelectOption {
	out := make([]formschema.SelectOption, 0, len(options))
	for _, option := range options {
		out = append(out, formschema.SelectOption{
			Value:       option.ID,
			Label:       option.Label,
			Description: option.Description,
			Status:      option.Status,
		})
	}
	return out
}

// cleanTopics trims whitespace and drops empties.
func cleanTopics(in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}
