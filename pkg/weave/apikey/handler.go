package apikey

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// Handler serves the actor-scoped API-key routes.
type Handler struct {
	svc          *Service
	log          *slog.Logger
	languages    []formschema.LanguageInfo
	langResolver func(*http.Request) string
}

// NewHandler builds the HTTP handler from a validated Host.
func NewHandler(host Host) *Handler {
	log := host.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: host.Service, log: log, languages: host.Languages, langResolver: host.LangResolver}
}

// keyRow is the list-row projection: everything the UI shows, never the hash
// or the secret. revoked_at drives the row action's visible_when.
type keyRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	KeyPrefix  string `json:"key_prefix"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	RevokedAt  string `json:"revoked_at,omitempty"`
}

func rowFromKey(k *domain.APIKey) keyRow {
	row := keyRow{
		ID:        k.ID,
		Name:      k.Name,
		KeyPrefix: k.KeyPrefix,
		Status:    "active",
		CreatedAt: k.CreatedAt.Format("2006-01-02"),
	}
	if k.LastUsedAt != nil {
		row.LastUsedAt = k.LastUsedAt.Format("2006-01-02 15:04")
	}
	if k.RevokedAt != nil {
		row.Status = "revoked"
		row.RevokedAt = k.RevokedAt.Format("2006-01-02")
	} else if !k.Active(time.Now()) {
		row.Status = "expired"
	}
	return row
}

func (h *Handler) actorID(w http.ResponseWriter, r *http.Request) (string, bool) {
	snap := auth.FromContext(r.Context())
	if snap == nil || snap.IsAnonymous || snap.ActorID == "" {
		apierror.Write(w, apierror.Unauthorized())
		return "", false
	}
	return snap.ActorID, true
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// List returns the caller's keys as a bare array (ListManager contract).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.actorID(w, r)
	if !ok {
		return
	}
	keys, err := h.svc.List(r.Context(), actorID)
	if err != nil {
		h.log.Error("list api keys", "actor_id", actorID, "err", err)
		apierror.Write(w, apierror.Internal())
		return
	}
	rows := make([]keyRow, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, rowFromKey(k))
	}
	h.writeJSON(w, http.StatusOK, rows)
}

// ListSchema serves the ListManager schema.
func (h *Handler) ListSchema(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.actorID(w, r); !ok {
		return
	}
	h.writeJSON(w, http.StatusOK, BuildListSchema(h.langResolver(r), h.languages))
}

// FormSchema serves the create form schema.
func (h *Handler) FormSchema(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.actorID(w, r); !ok {
		return
	}
	h.writeJSON(w, http.StatusOK, BuildCreateFormSchema(h.langResolver(r), h.languages))
}

type createInput struct {
	Name        string `json:"name"`
	ExpiresDays any    `json:"expires_days"`
}

// parseExpiresDays accepts the create form's expires_days value in any of
// the shapes the number widget or a JSON caller may submit: a JSON number,
// a numeric string, an empty string, or a missing/null field. It returns
// (0, true) for "no expiry" and (0, false) for anything invalid — negative,
// fractional, or non-numeric.
func parseExpiresDays(v any) (int, bool) {
	switch n := v.(type) {
	case nil:
		return 0, true
	case float64:
		if n != math.Trunc(n) || n < 1 {
			return 0, false
		}
		return int(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil || f != math.Trunc(f) || f < 1 {
			return 0, false
		}
		return int(f), true
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, true
		}
		parsed, err := strconv.Atoi(s)
		if err != nil || parsed < 1 {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

// Create mints a key for the caller and returns the plaintext secret once.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.actorID(w, r)
	if !ok {
		return
	}
	var in createInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apierror.Write(w, apierror.BadRequest("invalid json body"))
		return
	}
	fieldErrs := map[string][]string{}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		fieldErrs["name"] = append(fieldErrs["name"], "name is required")
	}
	ttlDays, ok := parseExpiresDays(in.ExpiresDays)
	if !ok {
		fieldErrs["expires_days"] = append(fieldErrs["expires_days"], "must be a positive number of days")
	}
	if len(fieldErrs) > 0 {
		apierror.Write(w, apierror.Validation(fieldErrs))
		return
	}
	secret, key, err := h.svc.Mint(r.Context(), actorID, name, ttlDays)
	if err != nil {
		h.log.Error("mint api key", "actor_id", actorID, "err", err)
		apierror.Write(w, apierror.Internal())
		return
	}
	row := rowFromKey(key)
	h.writeJSON(w, http.StatusCreated, map[string]any{
		"id":         row.ID,
		"name":       row.Name,
		"key_prefix": row.KeyPrefix,
		"status":     row.Status,
		"created_at": row.CreatedAt,
		"secret":     secret,
	})
}

// Revoke revokes the caller's key by ID; 0 rows revoked is a 404 (not
// found, not owned, and already-revoked are indistinguishable).
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.actorID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	n, err := h.svc.RevokeOwned(r.Context(), actorID, id)
	if err != nil {
		h.log.Error("revoke api key", "actor_id", actorID, "err", err)
		apierror.Write(w, apierror.Internal())
		return
	}
	if n == 0 {
		apierror.Write(w, apierror.NotFound("api key not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
