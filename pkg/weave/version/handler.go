package version

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/assets"
	"github.com/pletka-io/pletka/pkg/buildinfo"
)

// Payload is the JSON body returned by GET /version. Field names
// match what the frontend footer pill expects.
type Payload struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	Branch     string `json:"branch,omitempty"`
	Dirty      bool   `json:"dirty"`
	BuiltAt    string `json:"built_at"`
	Migration  int64  `json:"migration"`
	Frontend   string `json:"frontend"`
	Instance   string `json:"instance,omitempty"`
	Go         string `json:"go"`
	ReportedAt string `json:"reported_at"`
}

// Handler builds version payloads. The migration version + frontend
// manifest fingerprint are snapshot at boot via Refresh(); buildinfo
// values come from the package vars (ldflags), so reads are lock-free
// once the snapshot is set.
type Handler struct {
	pool     *pgxpool.Pool
	logger   *slog.Logger
	instance string

	mu        sync.RWMutex
	frontend  string
	migration int64
}

// NewHandler builds a Handler and runs an initial Refresh against
// the given pool. Errors during refresh log a warning but never
// block startup — the endpoint should always answer.
func NewHandler(pool *pgxpool.Pool, logger *slog.Logger, instance string) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{pool: pool, logger: logger, instance: instance}
	h.Refresh(context.Background())
	return h
}

// Refresh re-reads the migration version from goose_db_version and
// re-fingerprints the frontend manifest. Called at boot and on
// /version?refresh=1.
func (h *Handler) Refresh(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.frontend = readFrontendFingerprint()
	h.migration = readMigrationVersion(ctx, h.pool, h.logger)
}

// Check renders the JSON payload. ?refresh=1 forces a re-read of
// migration + frontend before responding.
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("refresh") == "1" {
		h.Refresh(r.Context())
	}

	bi := buildinfo.Get()

	h.mu.RLock()
	frontend := h.frontend
	migration := h.migration
	h.mu.RUnlock()

	payload := Payload{
		Version:    bi.Version,
		Commit:     bi.Commit,
		Branch:     bi.Branch,
		Dirty:      bi.Dirty,
		BuiltAt:    bi.BuiltAt,
		Migration:  migration,
		Frontend:   frontend,
		Instance:   h.instance,
		Go:         runtime.Version(),
		ReportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	// 30s cache so the footer pill doesn't re-fetch on every page nav,
	// short enough that ?refresh=1 stays useful for testing.
	w.Header().Set("Cache-Control", "public, max-age=30")
	_ = json.NewEncoder(w).Encode(payload)
}

// readMigrationVersion queries goose_db_version for the latest applied
// migration. Returns 0 on any error so the endpoint still responds.
func readMigrationVersion(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) int64 {
	if pool == nil {
		return 0
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var version int64
	err := pool.QueryRow(cctx, `
		SELECT version_id FROM goose_db_version WHERE is_applied = true
		ORDER BY id DESC LIMIT 1
	`).Scan(&version)
	if err != nil {
		logger.Warn("version: goose lookup failed", "err", err)
		return 0
	}
	return version
}

// readFrontendFingerprint reads the embedded vite manifest and returns
// a short hash of its contents. Any change to a bundled asset shifts
// the manifest content, so this fingerprint moves whenever the
// frontend rebuilds — exactly what bisect needs.
func readFrontendFingerprint() string {
	staticFS := assets.GetStaticFS()
	if staticFS == nil {
		return "unknown"
	}
	body, err := fs.ReadFile(staticFS, "dist/.vite/manifest.json")
	if err != nil || len(body) == 0 {
		return "unknown"
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])[:12]
}
