package admin

import (
	"encoding/json"
	"net/http"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
)

// Handler serves the global admin schema shell. Slice-owned content is mounted
// elsewhere; this handler only exposes the shell contract consumed by the
// frontend island.
type Handler struct {
	extraSections []formschema.AdminSection
}

func NewHandler(extraSections ...formschema.AdminSection) *Handler {
	return &Handler{extraSections: extraSections}
}

func (h *Handler) Schema(w http.ResponseWriter, r *http.Request) {
	snap := weaveauth.FromContext(r.Context())
	if snap == nil || !snap.IsSuperAdmin {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, formschema.BuildAdminSchema(snap, h.extraSections...))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
