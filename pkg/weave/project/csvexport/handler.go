package csvexport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

const exportListLimit = 10000

// validExportTypes lists all supported CSV export types.
var validExportTypes = map[string]bool{
	"fields":                     true,
	"models":                     true,
	"collections":                true,
	"categories":                 true,
	"model-field-overrides":      true,
	"collection-field-overrides": true,
	"base-field-overrides":       true,
	"ontologies":                 true,
}

// ExportTypes returns the list of valid export type names.
func ExportTypes() []string {
	return []string{
		"fields", "models", "collections", "categories",
		"model-field-overrides", "collection-field-overrides",
		"base-field-overrides", "ontologies",
	}
}

// ExportTypeInfo describes a CSV export for the downloads template.
type ExportTypeInfo struct {
	Type string // URL slug (e.g., "model-field-overrides")
	Name string // Display name
	Desc string // Short description
	Icon string // SVG path for the icon
}

// exportTypeInfos returns the export types with display metadata.
func exportTypeInfos() []ExportTypeInfo {
	return []ExportTypeInfo{
		{"fields", "Fields", "Field definitions with ontology paths and metadata", "M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"},
		{"models", "Models", "Model definitions with scope classes", "M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"},
		{"collections", "Collections", "Collection definitions with scope and ordering", "M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"},
		{"categories", "Categories", "Display categories with ordering", "M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z"},
		{"model-field-overrides", "Model Field Overrides", "Field overrides scoped to models with refs", "M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"},
		{"collection-field-overrides", "Collection Field Overrides", "Field overrides scoped to collections with refs", "M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"},
		{"base-field-overrides", "Base Field Overrides", "Project-level base field overrides with refs", "M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"},
		{"ontologies", "Linked Ontologies", "Ontology versions linked to this project", "M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"},
	}
}

// DownloadsPage renders the project downloads index page using the
// embedded template — no dependency on the legacy templates manager.
func (s *Service) DownloadsPage(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	project, ok := s.loadAndGate(w, r, projectID)
	if !ok {
		return
	}

	lang := s.currentLang(r)
	projectName := project.UIName.Get(lang, project.SystemName, projectID)
	backLabel := s.translate(lang, "nav.back_to_project", "Back to %s", projectName)
	activeVersion := auth.ProjectVersionFromContext(r.Context())
	type exportLink struct {
		Type string
		Name string
		Desc string
		Icon string
		URL  string
	}
	exports := make([]exportLink, 0, len(exportTypeInfos()))
	for _, item := range exportTypeInfos() {
		exports = append(exports, exportLink{
			Type: item.Type,
			Name: item.Name,
			Desc: item.Desc,
			Icon: item.Icon,
			URL:  csvWithVersion(fmt.Sprintf("/projects/%s/exports/%s.csv", projectID, item.Type), activeVersion),
		})
	}

	data := struct {
		ProjectID   string
		ProjectName string
		ProjectURL  string
		BasePath    string
		ZipURL      string
		BackLabel   string
		IsRelease   bool
		Version     string
		Exports     []exportLink
	}{
		ProjectID:   projectID,
		ProjectName: projectName,
		ProjectURL:  csvWithVersion(fmt.Sprintf("/projects/%s", projectID), activeVersion),
		BasePath:    fmt.Sprintf("/projects/%s/exports", projectID),
		ZipURL:      csvWithVersion(fmt.Sprintf("/projects/%s/exports/all.zip", projectID), activeVersion),
		BackLabel:   backLabel,
		IsRelease:   activeVersion != "",
		Version:     activeVersion,
		Exports:     exports,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "downloads.gohtml", data); err != nil {
		s.logger.Error("render downloads page", "err", err, "project_id", projectID)
	}
}

func csvWithVersion(raw, activeVersion string) string {
	if activeVersion == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("version", activeVersion)
	u.RawQuery = q.Encode()
	return u.String()
}

// DownloadCSV handles individual CSV file downloads.
func (s *Service) DownloadCSV(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	exportType := chi.URLParam(r, "type")

	if !validExportTypes[exportType] {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "unknown export type")
		return
	}

	project, ok := s.loadAndGate(w, r, projectID)
	if !ok {
		return
	}

	prefix := project.SystemName
	if prefix == "" {
		prefix = projectID
	}

	fileName := fmt.Sprintf("%s-%s.csv", prefix, exportType)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, fileName))

	if err := s.writeCSVToWriter(w, r, projectID, exportType); err != nil {
		s.logger.Error("write csv", "err", err, "project_id", projectID, "type", exportType)
		// Headers already sent, nothing we can do for the client.
	}
}

// writeCSVToWriter dispatches to the correct writer based on export type.
func (s *Service) writeCSVToWriter(w io.Writer, r *http.Request, projectID, exportType string) error {
	ctx := r.Context()

	switch exportType {
	case "fields":
		return s.writeFields(ctx, w, projectID)
	case "models":
		return s.writeModels(ctx, w, projectID)
	case "collections":
		return s.writeCollections(ctx, w, projectID)
	case "categories":
		return s.writeCategories(ctx, w, projectID)
	case "model-field-overrides":
		return s.writeOverrides(ctx, w, projectID, "model")
	case "collection-field-overrides":
		return s.writeOverrides(ctx, w, projectID, "collection")
	case "base-field-overrides":
		return s.writeOverrides(ctx, w, projectID, "")
	case "ontologies":
		return s.writeOntologies(ctx, w, projectID)
	default:
		return fmt.Errorf("unknown export type: %s", exportType)
	}
}

func (s *Service) writeFields(ctx context.Context, w io.Writer, projectID string) error {
	fields, total, err := s.weave.WeaveFields().List(ctx, domain.WithProjectID(projectID), domain.WithLimit(exportListLimit))
	if err != nil {
		return fmt.Errorf("list fields: %w", err)
	}
	if total > int64(exportListLimit) {
		return fmt.Errorf("project %s has %d fields, exceeding export limit %d: results would be truncated", projectID, total, exportListLimit)
	}

	expectedRefs, err := s.resolveBaseRefs(ctx, projectID)
	if err != nil {
		s.logger.Warn("failed to resolve base refs for fields export", "err", err)
		expectedRefs = make(map[string]string)
	}

	baseMeta := make(map[string]FieldOverrideMeta, len(fields))
	bases, err := s.weave.Overrides().ListByProjectAndType(ctx, projectID, "")
	if err != nil {
		s.logger.Warn("failed to load base overrides for fields export", "err", err)
	} else {
		for i := range bases {
			b := bases[i]
			baseMeta[b.FieldID] = FieldOverrideMeta{
				CategoryID: b.CategoryID,
				SetValue:   b.SetValue,
			}
		}
	}

	return WriteFields(w, fields, expectedRefs, baseMeta)
}

func (s *Service) writeModels(ctx context.Context, w io.Writer, projectID string) error {
	models, total, err := s.weave.Models().List(ctx, domain.WithProjectID(projectID), domain.WithLimit(exportListLimit))
	if err != nil {
		return fmt.Errorf("list models: %w", err)
	}
	if total > int64(exportListLimit) {
		return fmt.Errorf("project %s has %d models, exceeding export limit %d: results would be truncated", projectID, total, exportListLimit)
	}
	return WriteModels(w, models)
}

func (s *Service) writeCollections(ctx context.Context, w io.Writer, projectID string) error {
	collections, total, err := s.weave.Collections().List(ctx, domain.WithProjectID(projectID), domain.WithLimit(exportListLimit))
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	if total > int64(exportListLimit) {
		return fmt.Errorf("project %s has %d collections, exceeding export limit %d: results would be truncated", projectID, total, exportListLimit)
	}
	return WriteCollections(w, collections)
}

func (s *Service) writeCategories(ctx context.Context, w io.Writer, projectID string) error {
	categories, err := s.weave.WeaveCategories().List(ctx, domain.WithProjectID(projectID), domain.WithLimit(exportListLimit))
	if err != nil {
		return fmt.Errorf("list categories: %w", err)
	}
	return WriteCategories(w, categories)
}

func (s *Service) writeOverrides(ctx context.Context, w io.Writer, projectID, entityType string) error {
	overrides, err := s.weave.Overrides().ListByProjectAndType(ctx, projectID, entityType)
	if err != nil {
		return fmt.Errorf("list %s overrides: %w", entityType, err)
	}

	fields, _, err := s.weave.WeaveFields().List(ctx, domain.WithProjectID(projectID), domain.WithLimit(exportListLimit))
	if err != nil {
		return fmt.Errorf("list fields for lookup: %w", err)
	}
	fieldLookup := make(map[string]string, len(fields))
	for _, f := range fields {
		fieldLookup[f.ID] = f.SemanticID
	}

	entityLookup := make(map[string]string)
	switch entityType {
	case "model":
		models, _, err := s.weave.Models().List(ctx, domain.WithProjectID(projectID), domain.WithLimit(exportListLimit))
		if err != nil {
			return fmt.Errorf("list models for lookup: %w", err)
		}
		for _, m := range models {
			entityLookup[m.ID] = m.SemanticID
		}
	case "collection":
		collections, _, err := s.weave.Collections().List(ctx, domain.WithProjectID(projectID), domain.WithLimit(exportListLimit))
		if err != nil {
			return fmt.Errorf("list collections for lookup: %w", err)
		}
		for _, c := range collections {
			entityLookup[c.ID] = c.SemanticID
		}
	}

	rows := make([]OverrideRow, len(overrides))
	for i, o := range overrides {
		refs, err := s.weave.Overrides().GetRefs(ctx, o.ID)
		if err != nil {
			return fmt.Errorf("get refs for override %d: %w", o.ID, err)
		}

		refParts := make([]string, len(refs))
		for j, ref := range refs {
			refParts[j] = ref.SemanticID
		}

		rows[i] = OverrideRow{
			EntitySemanticID:   entityLookup[o.EntityID],
			FieldSemanticID:    fieldLookup[o.FieldID],
			Position:           o.Position,
			CollectionOrder:    o.CollectionOrder,
			DisplayName:        o.DisplayName,
			Description:        o.Description,
			CollectionName:     o.CollectionName,
			CategoryID:         o.CategoryID,
			SetValue:           o.SetValue,
			PartOfCollectionID: o.PartOfCollectionID,
			IsRequired:         o.IsRequired,
			MinOccurs:          o.MinOccurs,
			MaxOccurs:          o.MaxOccurs,
			IsHidden:           o.IsHidden,
			Visibility:         o.Visibility,
			Refs:               strings.Join(refParts, ","),
		}
	}

	switch entityType {
	case "model":
		return WriteModelFieldOverrides(w, rows)
	case "collection":
		return WriteCollectionFieldOverrides(w, rows)
	default:
		return WriteBaseFieldOverrides(w, rows)
	}
}

func (s *Service) writeOntologies(ctx context.Context, w io.Writer, projectID string) error {
	links, err := s.weave.ProjectOntologyVersions().List(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list project ontology versions: %w", err)
	}

	rows := make([]OntologyRow, 0, len(links))
	for _, link := range links {
		row := OntologyRow{
			IsPrimary:  link.IsPrimary,
			AddedAt:    link.AddedAt,
			UsageNotes: link.UsageNotes,
		}

		if s.ontologyVersions != nil {
			ov, err := s.ontologyVersions.GetByID(ctx, link.OntologyVersionID)
			if err != nil || ov == nil {
				s.logger.Warn("fetch ontology version for export", "err", err, "version_id", link.OntologyVersionID)
			} else {
				row.Version = ov.VersionString
				row.OntologyURI = ov.OntologyURI
				row.VersionIRI = ov.VersionIRI
				row.IsActive = ov.IsActive
				row.ClassCount = ov.ClassCount
				row.PropertyCount = ov.PropertyCount
				row.OntologyLabel = ov.OntologyLabel.Get("en")

				if s.ontologies != nil && ov.OntologyID != "" {
					ontology, oerr := s.ontologies.GetByID(ctx, ov.OntologyID)
					if oerr == nil && ontology != nil {
						row.OntologyName = ontology.Name
					}
				}
			}
		}

		rows = append(rows, row)
	}

	return WriteOntologies(w, rows)
}

// loadAndGate loads the project and confirms the caller is a member of
// it. Returns (nil, false) and writes a 404 on either missing project
// or non-member caller. The 404 (not 403) avoids leaking the existence
// of private projects.
//
// Membership = super-admin OR project owner OR explicit project role
// OR inherited org membership. Anonymous + public-fallback "viewer"
// callers are denied — the verification CSV exposes drafts and override
// detail, so it should not be a drive-by surface.
func (s *Service) loadAndGate(w http.ResponseWriter, r *http.Request, projectID string) (*domain.Project, bool) {
	return s.loadAndGateCtx(r.Context(), w, r, projectID)
}

func (s *Service) loadAndGateCtx(ctx context.Context, w http.ResponseWriter, r *http.Request, projectID string) (*domain.Project, bool) {
	project := auth.ProjectFromContext(ctx)
	if project == nil || project.ID != projectID {
		var err error
		project, err = s.weave.Projects().GetByID(ctx, projectID)
		if err != nil || project == nil {
			s.logger.Error("fetch project for csv export", "err", err, "project_id", projectID)
			errresp.Error(w, r, http.StatusNotFound, "not_found", "project not found")
			return nil, false
		}
	}
	snap := auth.FromContext(ctx)
	if !snap.IsProjectMember(auth.ProjectResource(project)) {
		errresp.Error(w, r, http.StatusNotFound, "not_found", "project not found")
		return nil, false
	}
	return project, true
}

// resolveBaseRefs returns a map from field ID to comma-separated ref
// semantic IDs by looking up the base override (entity_type="") for
// each field in the project.
func (s *Service) resolveBaseRefs(ctx context.Context, projectID string) (map[string]string, error) {
	baseOverrides, err := s.weave.Overrides().ListByProjectAndType(ctx, projectID, "")
	if err != nil {
		return nil, fmt.Errorf("list base overrides: %w", err)
	}

	result := make(map[string]string, len(baseOverrides))
	for _, o := range baseOverrides {
		refs, err := s.weave.Overrides().GetRefs(ctx, o.ID)
		if err != nil {
			continue
		}
		if len(refs) == 0 {
			continue
		}
		parts := make([]string, len(refs))
		for i, ref := range refs {
			parts[i] = ref.SemanticID
		}
		result[o.FieldID] = strings.Join(parts, ",")
	}
	return result, nil
}
