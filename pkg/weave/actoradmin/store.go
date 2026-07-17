package actoradmin

import (
	"context"
	"time"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Store is the data-access contract for actoradmin. Implementation:
//   - postgresStore (this package) — pgx + sqlc, production
//
// Methods mirror the actor/count/last-login reads and the actor upsert that
// Service used to issue directly against sqlcgen.Queries and the pool.
type Store interface {
	// ListActorsByType returns every actor of the given type ("person" or
	// "organization"), ordered by display_name.
	ListActorsByType(ctx context.Context, actorType string) ([]sqlcgen.WeaveActor, error)

	// GetActorByID returns the actor with the given ULID. Returns (nil, nil)
	// when no row matches — callers treat that as "not found" rather than an
	// error.
	GetActorByID(ctx context.Context, id string) (*sqlcgen.WeaveActor, error)

	// CreateActor upserts an actor row (insert, or update on id conflict).
	// Used for both create and update flows — callers populate every field.
	CreateActor(ctx context.Context, params sqlcgen.WeaveCreateActorParams) (*sqlcgen.WeaveActor, error)

	// ProjectCounts returns, for each actor ID in ids, the number of
	// distinct projects it is linked to via weave_project_actors. IDs with
	// no rows are simply absent from the result map.
	ProjectCounts(ctx context.Context, ids []string) (map[string]int, error)

	// OwnedProjectCounts is ProjectCounts restricted to role = 'owner'.
	OwnedProjectCounts(ctx context.Context, ids []string) (map[string]int, error)

	// MemberCounts returns, for each institution ID, the count of person
	// actors whose parent_id matches.
	MemberCounts(ctx context.Context, institutionIDs []string) (map[string]int, error)

	// LastLogins returns, for each actor ID, the last_login_at timestamp
	// from weave_auth. Actors with no auth row, or a null last_login_at,
	// map to a nil *time.Time.
	LastLogins(ctx context.Context, ids []string) (map[string]*time.Time, error)
}
