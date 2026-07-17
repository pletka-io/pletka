package gitrestoreadmin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

type PreviewResult = gitmaterializer.RestorePreview
type PreviewCounts = gitmaterializer.RestorePreviewCounts

type Previewer interface {
	Preview(rootDir string) (*PreviewResult, error)
}

type previewerFunc func(rootDir string) (*PreviewResult, error)

func (f previewerFunc) Preview(rootDir string) (*PreviewResult, error) {
	return f(rootDir)
}

func PreviewSnapshot(rootDir string) (*PreviewResult, error) {
	return gitmaterializer.PreviewRestoreSnapshot(rootDir)
}

type Handler struct {
	log *slog.Logger
	svc *Service
}

type previewRequest struct {
	SnapshotPath string `json:"snapshot_path"`
}

func NewHandler(log *slog.Logger, svc *Service) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{log: log, svc: svc}
}

func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	req.SnapshotPath = strings.TrimSpace(req.SnapshotPath)
	if req.SnapshotPath == "" {
		apierror.Write(w, apierror.Validation(map[string][]string{
			"snapshot_path": {"Snapshot path is required."},
		}))
		return
	}

	preview, err := h.svc.previewer.Preview(req.SnapshotPath)
	if err != nil {
		h.log.Warn("git restore preview failed", "snapshot_path", req.SnapshotPath, "err", err)
		apierror.Write(w, apierror.BadRequest(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var in CreateJobInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid JSON body"))
		return
	}
	job, err := h.svc.Create(r.Context(), in)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			apierror.Write(w, apierror.BadRequest("limit must be a positive integer"))
			return
		}
		limit = n
	}
	items, err := h.svc.List(r.Context(), limit)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.svc.Get(r.Context(), chi.URLParam(r, "jobID"))
	if errors.Is(err, ErrJobNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) RunJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.svc.Run(r.Context(), chi.URLParam(r, "jobID"))
	if errors.Is(err, ErrJobNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	ae := apierror.FromError(err)
	if ae.Code == apierror.CodeInternal {
		h.log.Error("git restore admin error", "err", err)
	}
	apierror.Write(w, ae)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
