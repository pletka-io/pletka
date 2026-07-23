package testdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// The real-project fixture snapshots (test/fixtures/AME, LA, ING) reference
// their owning organization actors by id — dhi-rome, getty-digital, knaw,
// sari-ch, takin, unite — via weave_projects.owner_id (a NOT NULL, FK-enforced
// column) and via vendored parent-project owners baked into each snapshot's
// project.yaml. Snapshots restore projects, ontologies, and inheritance but
// carry no actor rows, so the template must seed these org actors before
// hydration, exactly as seedFixtureOwner (removed) did for the single
// synthetic FXORG owner.
//
// fixtureOrgs mirrors the real, public actor rows for these six institutions
// as they exist in the shared dev database (display_name, slug, type,
// visibility) — public information about real organizations, not fixture
// data invented for the snapshots.
type fixtureOrg struct {
	id          string
	displayName string
	slug        string
}

var fixtureOrgs = []fixtureOrg{
	{id: "dhi-rome", displayName: "German Institute in Rome", slug: "dhi-rome"},
	{id: "getty-digital", displayName: "Getty Digital", slug: "getty-digital"},
	{id: "knaw", displayName: "Royal Netherlands Academy of Arts and Sciences", slug: "knaw"},
	{id: "sari-ch", displayName: "Swiss Art Research Infrastructure", slug: "sari-ch"},
	{id: "takin", displayName: "Takin Solutions", slug: "takin"},
	{id: "unite", displayName: "University of Teramo", slug: "unite"},
}

// fixtureUser is a mock person actor with no PII — clearly-fake id and name —
// used to model "user belongs to org, has rights on project" against the
// real-project fixtures. Real user ids/names could substitute later if the
// repo stays private; mock keeps the fixture data public-safe.
type fixtureUser struct {
	id          string
	displayName string
	slug        string
	// orgID is the organization this user is a member of (weave_memberships,
	// scope_type "org").
	orgID string
	// orgRole is the user's org-scope role: member / admin / owner (see
	// pkg/weave/orgmembers.validRoles).
	orgRole string
}

var fixtureUsers = []fixtureUser{
	{id: "fixture-user-owner", displayName: "Fixture Owner", slug: "fixture-user-owner", orgID: "unite", orgRole: "owner"},
	{id: "fixture-user-contributor", displayName: "Fixture Contributor", slug: "fixture-user-contributor", orgID: "takin", orgRole: "member"},
	{id: "fixture-user-viewer", displayName: "Fixture Viewer", slug: "fixture-user-viewer", orgID: "takin", orgRole: "member"},
}

// fixtureProjectRight grants a mock user an explicit project-scope role
// (weave_memberships, scope_type "project") on one of the real-project
// fixtures. Unlike org membership, these reference project ids that only
// exist once the fixture snapshots are hydrated, so they must be seeded
// after hydration, not before.
type fixtureProjectRight struct {
	userID    string
	projectID string
	// role is the project-scope role: viewer / contributor / maintainer /
	// owner (see pkg/weave/members.validRoles).
	role string
}

var fixtureProjectRights = []fixtureProjectRight{
	{userID: "fixture-user-owner", projectID: "AME", role: "owner"},
	{userID: "fixture-user-contributor", projectID: "LA", role: "contributor"},
	{userID: "fixture-user-viewer", projectID: "ING", role: "viewer"},
}

// seedFixtureIdentities creates the org actors the fixture snapshots
// reference by id, the mock user actors, and their org-level memberships. It
// must run before hydration: weave_projects.owner_id is a NOT NULL column
// with a foreign key to weave_actors, so a snapshot's project row (and any
// vendored parent project row referencing one of these orgs as owner) fails
// to restore unless the owning actor already exists.
func seedFixtureIdentities(ctx context.Context, pool *pgxpool.Pool) error {
	q := sqlcgen.New(pool)

	for _, org := range fixtureOrgs {
		if _, err := q.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
			ID:          org.id,
			Type:        "organization",
			DisplayName: org.displayName,
			Slug:        org.slug,
			Role:        "",
			Visibility:  "public",
		}); err != nil {
			return fmt.Errorf("seed fixture org actor %s: %w", org.id, err)
		}
	}

	for _, user := range fixtureUsers {
		if _, err := q.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
			ID:          user.id,
			Type:        "person",
			DisplayName: user.displayName,
			Slug:        user.slug,
			Role:        "contributor",
			Visibility:  "private",
		}); err != nil {
			return fmt.Errorf("seed fixture user actor %s: %w", user.id, err)
		}
		if err := q.WeaveMembershipUpsert(ctx, sqlcgen.WeaveMembershipUpsertParams{
			ActorID:   user.id,
			ScopeType: "org",
			ScopeID:   user.orgID,
			Role:      user.orgRole,
		}); err != nil {
			return fmt.Errorf("seed fixture org membership %s -> %s: %w", user.id, user.orgID, err)
		}
	}
	return nil
}

// seedFixtureProjectRights grants the mock users their project-scope
// memberships. It must run after hydration: the scope_id values are the
// real-project fixture ids (AME, LA, ING), which only exist once the
// corresponding snapshots have been restored.
func seedFixtureProjectRights(ctx context.Context, pool *pgxpool.Pool) error {
	q := sqlcgen.New(pool)
	for _, right := range fixtureProjectRights {
		if err := q.WeaveMembershipUpsert(ctx, sqlcgen.WeaveMembershipUpsertParams{
			ActorID:   right.userID,
			ScopeType: "project",
			ScopeID:   right.projectID,
			Role:      right.role,
		}); err != nil {
			return fmt.Errorf("seed fixture project right %s -> %s: %w", right.userID, right.projectID, err)
		}
	}
	return nil
}
