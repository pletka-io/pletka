package errortracking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
)

// Source identifies the origin of an error event row.
type Source string

const (
	SourceServer Source = "server"
	SourceClient Source = "client"
)

// Category classifies the failure mode. Set at capture time so the
// canary backlog query (CategoryNoRoute) doesn't drown in handler-
// decision noise (auth gates, missing entities, validation rejects).
type Category string

const (
	// CategoryNoRoute means chi did not match any registered pattern.
	// True legacy gap. The migration backlog query filters on this.
	CategoryNoRoute Category = "no_route"

	// CategoryHandlerDecision means a handler ran and returned 4xx
	// (auth gate denied, not-found-by-id, method-not-allowed,
	// validation reject). Includes the auth-rule-bug class — anomaly
	// detection on this bucket is still useful, but it shouldn't
	// pollute the legacy-gap query.
	CategoryHandlerDecision Category = "handler_decision"

	// CategoryServerError means 5xx (or panic recovered to 5xx).
	CategoryServerError Category = "server_error"

	// CategoryClient is reserved for frontend-reported exceptions.
	CategoryClient Category = "client"
)

// Event is the row written to weave_error_events. Pointer fields are
// nullable in the table; a client row will have status / method /
// duration_ms unset, a server row will have all of them.
type Event struct {
	RequestID  string
	OccurredAt time.Time
	Source     Source
	Category   Category
	Route      string
	Method     *string
	Status     *int
	ActorID    *string
	DurationMS *int
	UserAgent  string
	Request    json.RawMessage
	Response   json.RawMessage
	Error      json.RawMessage
}

// Store persists error events.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store backed by the given pgx pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Insert writes one event. Errors here are themselves swallowed by the
// caller — the tracker must never break the request path.
func (s *Store) Insert(ctx context.Context, e Event) error {
	if s == nil || s.pool == nil {
		return nil
	}
	category := string(e.Category)
	if category == "" {
		category = string(CategoryHandlerDecision)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO weave_error_events (
			request_id, occurred_at, source, category, route, method, status,
			actor_id, duration_ms, user_agent, request, response, error
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`,
		dbutil.EmptyToNil(e.RequestID),
		e.OccurredAt,
		string(e.Source),
		category,
		e.Route,
		e.Method,
		e.Status,
		e.ActorID,
		e.DurationMS,
		dbutil.EmptyToNil(e.UserAgent),
		jsonOrEmpty(e.Request),
		jsonOrEmpty(e.Response),
		jsonOrEmpty(e.Error),
	)
	return err
}

// ListFilter narrows the viewer query.
type ListFilter struct {
	Source   string
	Category string
	Status   int
	Route    string
	Limit    int
}

// ListResult is what the viewer renders.
type ListResult struct {
	ID         int64           `json:"id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Source     string          `json:"source"`
	Category   string          `json:"category"`
	Route      string          `json:"route"`
	Method     *string         `json:"method,omitempty"`
	Status     *int            `json:"status,omitempty"`
	ActorID    *string         `json:"actor_id,omitempty"`
	DurationMS *int            `json:"duration_ms,omitempty"`
	UserAgent  string          `json:"user_agent,omitempty"`
	Request    json.RawMessage `json:"request,omitempty"`
	Response   json.RawMessage `json:"response,omitempty"`
	Error      json.RawMessage `json:"error,omitempty"`
}

// List returns events matching f, newest first. Cap the limit at 500
// regardless of caller input.
func (s *Store) List(ctx context.Context, f ListFilter) ([]ListResult, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, occurred_at, source, category, route, method, status,
		       actor_id, duration_ms, user_agent, request, response, error
		FROM weave_error_events
		WHERE ($1 = '' OR source = $1)
		  AND ($2 = '' OR category = $2)
		  AND ($3 = 0  OR status = $3)
		  AND ($4 = '' OR route ILIKE '%' || $4 || '%')
		ORDER BY occurred_at DESC
		LIMIT $5
	`, f.Source, f.Category, f.Status, f.Route, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ListResult
	for rows.Next() {
		var r ListResult
		var ua *string
		if err := rows.Scan(
			&r.ID, &r.OccurredAt, &r.Source, &r.Category, &r.Route, &r.Method, &r.Status,
			&r.ActorID, &r.DurationMS, &ua, &r.Request, &r.Response, &r.Error,
		); err != nil {
			return nil, err
		}
		if ua != nil {
			r.UserAgent = *ua
		}
		out = append(out, r)
	}
	return out, rows.Err()
}


func jsonOrEmpty(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("{}")
	}
	return b
}
