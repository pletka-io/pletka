package csvexport

import (
	"archive/zip"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// DownloadZip creates a zip archive containing all CSV exports for a project.
func (s *Service) DownloadZip(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	project, ok := s.loadAndGate(w, r, projectID)
	if !ok {
		return
	}

	prefix := project.SystemName
	if prefix == "" {
		prefix = projectID
	}

	zipName := fmt.Sprintf("%s-export.zip", prefix)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, zipName))

	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, exportType := range ExportTypes() {
		fileName := fmt.Sprintf("%s-%s.csv", prefix, exportType)

		entry, err := zw.Create(fileName)
		if err != nil {
			s.logger.Error("create zip entry", "err", err, "file", fileName)
			return
		}

		if err := s.writeCSVToWriter(entry, r, projectID, exportType); err != nil {
			s.logger.Error("write csv to zip", "err", err, "project_id", projectID, "type", exportType)
			return
		}
	}
}
