package csvexport

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Host struct {
	Service *Service
}

func (h Host) Validate() error {
	if h.Service == nil {
		return fmt.Errorf("csvexport host missing required dependencies: Service")
	}
	return nil
}

// Mount registers the slice routes under a /projects/{projectID}/exports
// sub-router alongside the per-entity export route:
//
//	GET /projects/{projectID}/exports/             — HTML index page
//	GET /projects/{projectID}/exports/all.zip      — every CSV bundled
//	GET /projects/{projectID}/exports/{type}.csv   — one CSV per type
//
// The .csv suffix is parsed off the path inside the dispatcher so the
// URL stays human-readable (matches the legacy URL the migration team
// already trained against). The same sub-router also receives
// /projects/{projectID}/exports/{kind}/{id}.csv from pkg/weave/exports
// (per-entity generator-csv).
func Mount(parent chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}

	parent.Get("/", host.Service.DownloadsPage)
	parent.Get("/all.zip", host.Service.DownloadZip)
	parent.Get("/{file}", csvDispatcher(host.Service))
}

// csvDispatcher accepts /downloads/{type}.csv and forwards to
// DownloadCSV with the {type} chi param populated. Anything not ending
// in .csv 404s.
func csvDispatcher(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file := chi.URLParam(r, "file")
		if !strings.HasSuffix(file, ".csv") {
			http.NotFound(w, r)
			return
		}
		exportType := strings.TrimSuffix(file, ".csv")

		// Plant the {type} param chi.URLParam will read inside DownloadCSV.
		rctx := chi.RouteContext(r.Context())
		rctx.URLParams.Add("type", exportType)

		svc.DownloadCSV(w, r)
	}
}
