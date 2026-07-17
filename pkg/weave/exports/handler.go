package exports

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// ProjectGetter is the minimal project-load surface the gate needs.
// Satisfied by domain.WeaveStore via weave.Projects().
type ProjectGetter interface {
	GetByID(ctx context.Context, id string) (*domain.Project, error)
}

type Handler struct {
	service  *generators.Service
	projects ProjectGetter
	log      *slog.Logger
}

func NewHandler(service *generators.Service, projects ProjectGetter, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{service: service, projects: projects, log: log}
}

// loadAndGate loads the project and confirms member access. 404s on
// either missing project or non-member caller. Same idiom as
// csvexport.Service.loadAndGate — the verification CSV is member-only.
func (h *Handler) loadAndGate(w http.ResponseWriter, r *http.Request, projectID string) (*domain.Project, bool) {
	ctx := r.Context()
	project := auth.ProjectFromContext(ctx)
	if project == nil || project.ID != projectID {
		if h.projects == nil {
			http.Error(w, "project not found", http.StatusNotFound)
			return nil, false
		}
		var err error
		project, err = h.projects.GetByID(ctx, projectID)
		if err != nil || project == nil {
			http.Error(w, "project not found", http.StatusNotFound)
			return nil, false
		}
	}
	if !auth.FromContext(ctx).IsProjectMember(auth.ProjectResource(project)) {
		http.Error(w, "project not found", http.StatusNotFound)
		return nil, false
	}
	return project, true
}

func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "exports unavailable", http.StatusServiceUnavailable)
		return
	}

	projectID := chi.URLParam(r, "projectID")
	if _, ok := h.loadAndGate(w, r, projectID); !ok {
		return
	}
	entityKind := chi.URLParam(r, "entityKind")
	entityID := cleanCSVParam(chi.URLParam(r, "entityID"))

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s-%s.csv"`, projectID, entityKind, entityID))

	var err error
	switch normalizeEntityKind(entityKind) {
	case generators.EntityModel:
		err = h.service.GenerateModel(r.Context(), projectID, entityID, generators.FormatCSV, w, generators.Options{})
	case generators.EntityCollection:
		err = h.service.GenerateCollection(r.Context(), projectID, entityID, generators.FormatCSV, w, generators.Options{})
	case generators.EntityField:
		err = h.service.GenerateField(r.Context(), projectID, entityID, generators.FormatCSV, w, generators.Options{})
	default:
		http.Error(w, "unknown export entity kind", http.StatusBadRequest)
		return
	}
	if err != nil {
		h.log.Error("generate weave csv export", "err", err, "project_id", projectID, "entity_kind", entityKind, "entity_id", entityID)
	}
}

func normalizeEntityKind(kind string) generators.EntityKind {
	switch strings.ToLower(strings.TrimSpace(cleanCSVParam(kind))) {
	case "model", "models":
		return generators.EntityModel
	case "collection", "collections":
		return generators.EntityCollection
	case "field", "fields":
		return generators.EntityField
	default:
		return generators.EntityKind(kind)
	}
}

func cleanCSVParam(value string) string {
	return strings.TrimSuffix(value, ".csv")
}
