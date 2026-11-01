package csvexport

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

// ExportTypeInfo describes a CSV export for the downloads template.
type ExportTypeInfo struct {
	Type string // URL slug (e.g., exportModelOverrides)
	Name string // Display name
	Desc string // Short description
	Icon string // SVG path for the icon
}

// exportTypeInfos returns the export types with display metadata.
func exportTypeInfos() []ExportTypeInfo {
	const overrideIcon = "M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
	return []ExportTypeInfo{
		{exportFields, "Fields", "Field definitions with ontology paths and metadata", "M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"},
		{exportModels, "Models", "Model definitions with scope classes", "M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"},
		{exportCollections, "Collections", "Collection definitions with scope and ordering", "M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"},
		{exportCategories, "Categories", "Display categories with ordering", "M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z"},
		{exportModelOverrides, "Model Field Overrides", "Field overrides scoped to models with refs", overrideIcon},
		{exportCollectionOverrides, "Collection Field Overrides", "Field overrides scoped to collections with refs", overrideIcon},
		{exportBaseOverrides, "Base Field Overrides", "Project-level base field overrides with refs", overrideIcon},
		{exportOntologies, "Linked Ontologies", "Ontology versions linked to this project", "M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"},
	}
}

// DownloadsPage renders the project downloads index page using the
// embedded template — no dependency on the legacy templates manager.
func (s *Service) DownloadsPage(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	export, ok := s.loadExport(w, r, projectID)
	if !ok {
		return
	}

	lang := s.currentLang(r)
	projectName := export.ProjectName(lang)
	backLabel := s.translate(lang, "nav.back_to_project", "Back to %s", projectName)
	activeVersion := export.identity.Version()
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

// DownloadCSV prepares the selected file before publishing success headers.
func (s *Service) DownloadCSV(w http.ResponseWriter, r *http.Request) {
	projectID, exportType := chi.URLParam(r, "projectID"), chi.URLParam(r, "type")
	if !validExportTypes[exportType] {
		errresp.Error(w, r, http.StatusBadRequest, "bad_request", "unknown export type")
		return
	}
	export, ok := s.loadExport(w, r, projectID)
	if !ok {
		return
	}
	data, err := export.CSV(exportType)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	s.sendDownload(w, r, "text/csv; charset=utf-8", export.prefix+"-"+exportType+".csv", data)
}

// loadExport is the only content-loading path, shared by the page, single files
// and ZIP. Current ownership gates access; the explicit selected value supplies
// every filename, label, row and reference.
func (s *Service) loadExport(w http.ResponseWriter, r *http.Request, projectID string) (*Export, bool) {
	if _, ok := s.loadAndGate(w, r, projectID); !ok {
		return nil, false
	}
	scope := auth.ReadScopeFromContext(r.Context())
	if !scope.Valid() {
		apierror.Write(w, r, apierror.Internal(fmt.Errorf("project CSV requires an explicit read scope")))
		return nil, false
	}
	value, err := s.source.Project(r.Context(), scope, projectID)
	if err == nil {
		root, rootErr := value.Root()
		err = rootErr
		if err == nil {
			if root.ProjectID() != projectID || root.Version() != scope.Version() || root.IsDraft() == scope.IsRelease() {
				err = fmt.Errorf("project CSV source returned a different selection")
			}
		}
	}
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return nil, false
	}
	export, err := Build(value)
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return nil, false
	}
	return export, true
}

func (s *Service) sendDownload(w http.ResponseWriter, r *http.Request, contentType, name string, data []byte) {
	if err := r.Context().Err(); err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	// #nosec G705 -- CSV/ZIP attachments only, with explicit content type and nosniff.
	if _, err := w.Write(data); err != nil {
		s.logger.Error("send project export", "err", err)
	}
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
		project, err = s.projects.GetByID(ctx, projectID)
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

// DownloadZip captures once, then publishes a completely rendered archive.
func (s *Service) DownloadZip(w http.ResponseWriter, r *http.Request) {
	export, ok := s.loadExport(w, r, chi.URLParam(r, "projectID"))
	if !ok {
		return
	}
	data, err := export.ZIP()
	if err != nil {
		apierror.Write(w, r, apierror.FromError(err))
		return
	}
	s.sendDownload(w, r, "application/zip", export.prefix+"-export.zip", data)
}
